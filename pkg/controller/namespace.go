package controller

import (
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/golang/glog"
	platform "kolihub.io/koli/pkg/apis/core/v1alpha1"
	"kolihub.io/koli/pkg/apis/core/v1alpha1/draft"
	"kolihub.io/koli/pkg/clientset"

	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"

	extensions "k8s.io/api/extensions/v1beta1"
	rbac "k8s.io/api/rbac/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

// NamespaceController controller
type NamespaceController struct {
	kclient   kubernetes.Interface
	sysClient clientset.CoreInterface

	nsInf cache.SharedIndexInformer
	spInf cache.SharedIndexInformer

	queue    *TaskQueue
	recorder record.EventRecorder
}

// NewNamespaceController creates a NamespaceController
func NewNamespaceController(nsInf, spInf cache.SharedIndexInformer,
	kclient kubernetes.Interface, sysClient clientset.CoreInterface) *NamespaceController {
	nc := &NamespaceController{
		kclient:   kclient,
		sysClient: sysClient,
		nsInf:     nsInf,
		spInf:     spInf,
		recorder:  newRecorder(kclient, "namespace-controller"),
	}
	nc.queue = NewTaskQueue("namespace", nc.syncHandler)
	nc.nsInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    nc.addNamespace,
		UpdateFunc: nc.updateNamespace,
		// DeleteFunc: nc.deleteNamespace,
	})

	// TODO: do not process system broker plans
	// nc.spInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
	// 	UpdateFunc: nc.updateServicePlan,
	// })

	return nc
}

func (c *NamespaceController) addNamespace(n interface{}) {
	new := n.(*v1.Namespace)
	glog.V(2).Infof("%s(%s) - add-namespace", new.Name, new.ResourceVersion)
	c.queue.Add(new)
}

func (c *NamespaceController) updateNamespace(o, n interface{}) {
	old := o.(*v1.Namespace)
	new := n.(*v1.Namespace)

	if old.ResourceVersion == new.ResourceVersion {
		return
	}

	glog.V(2).Infof("%s(%s) - update-namespace", new.Name, new.ResourceVersion)
	c.queue.Add(new)
}

// Run the controller.
func (c *NamespaceController) Run(workers int, stopc <-chan struct{}) {
	// don't let panics crash the process
	defer utilruntime.HandleCrash()
	// make sure the work queue is shutdown which will trigger workers to end
	defer c.queue.shutdown()

	glog.Info("Starting Namespace Controller...")

	if !cache.WaitForCacheSync(stopc, c.nsInf.HasSynced, c.spInf.HasSynced) {
		return
	}

	for i := 0; i < workers; i++ {
		// runWorker will loop until "something bad" happens.
		// The .Until will then rekick the worker after one second
		go c.queue.run(time.Second, stopc)
		// go wait.Until(c.runWorker, time.Second, stopc)
	}

	// wait until we're told to stop
	<-stopc
	glog.Info("Shutting down Namespace Controller")
}

func (c *NamespaceController) syncHandler(key string) error {
	obj, exists, err := c.nsInf.GetStore().GetByKey(key)
	if err != nil {
		return err
	}

	if !exists {
		glog.V(2).Infof("%s - namespace doesn't exist", key)
		return nil
	}

	ns := obj.(*v1.Namespace)
	if ns.DeletionTimestamp != nil {
		glog.V(2).Infof("%s - the namespace is being deleted", key)
		return nil
	}
	user := &platform.User{
		Customer:     ns.Labels["kolihub.io/customer"],
		Organization: ns.Labels["kolihub.io/org"],
		Username:     ns.Annotations["kolihub.io/owner"],
	}
	user.Groups = strings.Split(ns.Annotations["kolihub.io/groups"], ",")
	nsMeta := draft.NewNamespaceMetadata(ns.Name)
	if !nsMeta.IsValid() {
		glog.Infof("%s - it's not a valid resource", key)
		return nil
	}
	if len(user.Username) == 0 {
		c.recorder.Event(ns, v1.EventTypeWarning, "IdentityMismatch", "The namespace doesn't have any owner.")
		return nil
	}
	if nsMeta.Organization() != user.Organization || nsMeta.Customer() != user.Customer {
		msg := "%s - Identity mismatch: the user belongs to the customer '%s' and organization '%s', " +
			"but the namespace was created for the customer '%s' and organization '%s'."
		msg = fmt.Sprintf(msg, key, user.Customer, user.Organization, nsMeta.Customer(), nsMeta.Organization())
		c.recorder.Event(ns, v1.EventTypeWarning, "IdentityMismatch", msg)
		return nil
	}

	var plan *platform.Plan
	obj, exists, err = c.spInf.GetStore().GetByKey(path.Join(platform.SystemNamespace, ns.Annotations[platform.LabelClusterPlan]))
	if err != nil {
		return fmt.Errorf("failed retrieving plan from cache [%v]", err)
	}
	if !exists {
		glog.V(3).Infof("%s - searching for a default plan", key)
		s := labels.Set{platform.LabelDefault: "true"}
		cache.ListAll(c.spInf.GetStore(), s.AsSelector(), func(obj interface{}) {
			if plan != nil { // break
				return
			}
			p := obj.(*platform.Plan)
			if p.Namespace != platform.SystemNamespace {
				return
			}
			if _, ok := p.Labels[platform.LabelDefault]; ok {
				plan = p
			}
		})
	} else {
		plan = obj.(*platform.Plan)
	}
	if plan == nil {
		// TODO: change message output
		return fmt.Errorf("a custom or a default plan wasn't found")
	}
	glog.V(3).Infof(`%s - found plan "%s", role: %#v`, key, plan.Name, plan.Spec.DefaultClusterRole)

	// TODO: should retry
	if err := c.createDefaultRoleBinding(ns.Name, user, plan); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			c.recorder.Event(ns, v1.EventTypeWarning, "SetupRolesError", err.Error())
			return fmt.Errorf("SetupRoles [%v]", err)
		}
	}
	if err := c.enforceQuota(ns.Name, plan); err != nil {
		c.recorder.Event(ns, v1.EventTypeWarning, "SetupQuotaError", err.Error())
		return fmt.Errorf("SetupQuota [%v]", err)
	}
	// TODO: check for errors!
	// c.enforceRoleBindings(key, ns, user, sp)

	// if err := c.createDefaultPermissions(ns.Name, user); err != nil {
	// 	c.recorder.Event(ns, v1.EventTypeWarning, "SetupRolesError", err.Error())
	// 	return fmt.Errorf("SetupRoles [%s]", err)
	// }
	if err := c.createNetworkPolicy(ns.Name); err != nil {
		c.recorder.Event(ns, v1.EventTypeWarning, "SetupNetworkError", err.Error())
		return fmt.Errorf("SetupNetwork [%v]", err)
	}
	glog.V(3).Infof("%s - namespace provisioned successfully", key)
	return nil
}

// create the permissions (role/role binding) needed for the user
// to access the resources of the namespace.
// func (c *NamespaceController) createDefaultPermissions(ns string, usr *platform.User) error {
// 	role := &rbac.Role{
// 		ObjectMeta: metav1.ObjectMeta{Name: "default"},
// 		Rules: []rbac.PolicyRule{
// 			{
// 				APIGroups: []string{"*"},
// 				Resources: platform.RoleResources,
// 				Verbs:     platform.RoleVerbs,
// 			},
// 		},
// 	}
// 	if _, err := c.kclient.Rbac().Roles(ns).Create(role); err != nil && !apierrors.IsAlreadyExists(err) {
// 		return fmt.Errorf("Failed creating role: %s", err)
// 	}

// 	roleBinding := &rbac.RoleBinding{
// 		ObjectMeta: metav1.ObjectMeta{Name: "default"},
// 		Subjects: []rbac.Subject{
// 			{
// 				Kind: "User",
// 				Name: usr.Username, // TODO: change to ID
// 			},
// 		},
// 		RoleRef: rbac.RoleRef{
// 			Kind: "Role",
// 			Name: "default", // must match role name
// 		},
// 	}
// 	if _, err := c.kclient.Rbac().RoleBindings(ns).Create(roleBinding); err != nil && !apierrors.IsAlreadyExists(err) {
// 		return fmt.Errorf("Failed creating rolebinding: %s", err)
// 	}
// 	return nil
// }

func (c *NamespaceController) createDefaultRoleBinding(namespace string, u *platform.User, sp *platform.Plan) error {
	roleBinding := &rbac.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "koli:default",
			Namespace: namespace,
		},
		RoleRef: rbac.RoleRef{
			Kind: "ClusterRole",
			Name: sp.Spec.DefaultClusterRole,
		},
	}
	// Add permission to the owner of the namespace
	subjects := []rbac.Subject{{Kind: rbac.UserKind, Name: u.Username}}
	// Configure groups that this customer belongs to
	for _, group := range u.Groups {
		if len(group) == 0 {
			continue
		}
		subjects = append(subjects, rbac.Subject{Kind: rbac.GroupKind, Name: group})
	}
	roleBinding.Subjects = subjects
	_, err := c.kclient.RbacV1beta1().RoleBindings(namespace).Create(roleBinding)
	return err
}

func (c *NamespaceController) createNetworkPolicy(ns string) error {
	// allow traffic between all pods in the namespace only
	networkPolicy := &extensions.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "allow-traffic-between-pods"},
		Spec: extensions.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			Ingress: []extensions.NetworkPolicyIngressRule{
				{
					From: []extensions.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: nil,
							},
						},
					},
				},
			},
		},
	}
	_, err := c.kclient.Extensions().RESTClient().
		Post().
		NamespaceIfScoped(ns, true).
		Resource("networkpolicies").
		Body(networkPolicy).
		DoRaw()
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("Failed creating network policy (allow-traffic-between-pods): %s", err)
	}
	networkPolicy = &extensions.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "allow-traffic-between-kong"},
		Spec: extensions.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			Ingress: []extensions.NetworkPolicyIngressRule{
				{
					From: []extensions.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"kolihub.io/kong-system": "true"},
							},
						},
					},
				},
			},
		},
	}
	_, err = c.kclient.Extensions().RESTClient().
		Post().
		NamespaceIfScoped(ns, true).
		Resource("networkpolicies").
		Body(networkPolicy).
		DoRaw()
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("Failed creating network policy (allow-traffic-between-kong): %s", err)
	}
	return nil
}

func (c *NamespaceController) enforceQuota(ns string, sp *platform.Plan) error {
	rq := &v1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
		},
		Spec: v1.ResourceQuotaSpec{
			Hard: v1.ResourceList(sp.Spec.Hard),
		},
	}
	_, err := c.kclient.Core().ResourceQuotas(ns).Update(rq)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("Failed updating quota using service plan '%s': %s", sp.Name, err)
	}
	if apierrors.IsNotFound(err) {
		if _, err := c.kclient.Core().ResourceQuotas(ns).Create(rq); err != nil {
			return fmt.Errorf("Failed creating quota using service plan '%s': %s", sp.Name, err)
		}
	}
	return nil
}

// func (c *NamespaceController) enforceRoleBindings(logHeader string, ns *v1.Namespace, user *platform.User, sp *platform.Plan) {
// 	// Platform Roles
// 	userRoles := platform.NewPlatformRoles(ns.Annotations[spec.KoliPrefix("roles")])
// 	hasUserRoles := false
// 	if len(userRoles) > 0 {
// 		hasUserRoles = true
// 	}

// 	if len(platform.PlatformRegisteredRoles) == 0 {
// 		glog.Warningf("%s - platform roles not found", logHeader)
// 	}
// 	for _, role := range platform.PlatformRegisteredRoles {
// 		// Manual Operation - it has an annotation
// 		if hasUserRoles {
// 			if role.Exists(userRoles) {
// 				subjects := []rbac.Subject{}
// 				for _, group := range user.Groups {
// 					// TODO: adding a new group will not make effect
// 					// if the rolebinding exists
// 					subjects = append(subjects, rbac.Subject{Kind: rbac.GroupKind, Name: group})
// 				}
// 				subjects = append(subjects, rbac.Subject{Kind: rbac.UserKind, Name: user.Username})
// 				_, err := c.kclient.Rbac().RoleBindings(ns.Name).Create(role.GetRoleBinding(subjects))
// 				if err != nil && !apierrors.IsAlreadyExists(err) {
// 					// TODO: requeue on errors?
// 					glog.Warningf("%s - failed creating manual role binding '%s': %s", logHeader, role, err)
// 					continue
// 				}
// 				glog.Infof("%s - manual role binding created '%s'", logHeader, role)
// 			} else {
// 				opts := &metav1.DeleteOptions{}
// 				err := c.kclient.Rbac().RoleBindings(ns.Name).Delete(string(role), opts)
// 				if err != nil && !apierrors.IsNotFound(err) {
// 					// TODO: requeue on errors?
// 					glog.Warningf("%s - failed removing manual role binding '%s'", logHeader, role)
// 					continue
// 				}
// 				glog.Infof("%s - manual role binding removed '%s'", logHeader, role)
// 			}
// 			// Go to the next platform role because it's a manual operation (has annotations)
// 			continue
// 		}

// 		// Automatic Operation - inherit from a service plan
// 		if role.Exists(sp.Spec.Roles) {
// 			roleBinding := role.GetRoleBinding([]rbac.Subject{{Kind: "User", Name: user.Username}})
// 			_, err := c.kclient.Rbac().RoleBindings(ns.Name).Create(roleBinding)
// 			if err != nil && !apierrors.IsAlreadyExists(err) {
// 				glog.Warningf("%s - failed creating role binding '%s': %s", logHeader, role, err)
// 				continue
// 			}
// 			glog.Infof("%s - role binding created '%s'", logHeader, role)
// 		} else {
// 			opts := &metav1.DeleteOptions{}
// 			err := c.kclient.Rbac().RoleBindings(ns.Name).Delete(string(role), opts)
// 			if err != nil && !apierrors.IsNotFound(err) {
// 				glog.Warningf("%s - failed removing role binding '%s'", logHeader, role)
// 			}
// 		}
// 	}
// }
