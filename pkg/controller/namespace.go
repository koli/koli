package controller

import (
	"fmt"
	"time"

	"github.com/golang/glog"
	"kolihub.io/koli/pkg/clientset"
	"kolihub.io/koli/pkg/platform"
	"kolihub.io/koli/pkg/spec"
	"kolihub.io/koli/pkg/spec/util"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	rbac "k8s.io/client-go/pkg/apis/rbac/v1beta1"
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
	nc.queue = NewTaskQueue(nc.syncHandler)
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

// func (c *NamespaceController) deleteNamespace(n interface{}) {
// 	ns := n.(*v1.Namespace)
// 	glog.V(2).Infof("%s(%s) - delete-namespace", ns.Name, ns.ResourceVersion)
// }

func (c *NamespaceController) updateServicePlan(o, n interface{}) {
	old := o.(*spec.Plan)
	new := n.(*spec.Plan)

	if old.ResourceVersion == new.ResourceVersion {
		return
	}

	pns, err := platform.NewNamespace(new.Namespace)
	if err != nil {
		// it's not a valid namespace of the platform, skip
		return
	}
	// Process only broker serviceplans
	if pns.IsSystem() {
		glog.V(2).Infof("%s/%s - update-service-plan", new.Namespace, new.Name)
		c.enqueueForServicePlan(new.Name, pns)
	}
}

// enqueueForServicePlan queues all the namespaces that belongs to a specific service plan
func (c *NamespaceController) enqueueForServicePlan(spName string, pns *platform.Namespace) {
	options := spec.NewLabel().Add(map[string]string{
		"org":        pns.Organization,
		"brokerplan": spName,
	})
	cache.ListAll(c.nsInf.GetStore(), options.AsSelector(), func(obj interface{}) {
		// TODO: check for nil
		ns := obj.(*v1.Namespace)
		if pns.GetSystemNamespace() == ns.Name {
			return
		}
		c.queue.Add(ns)
	})
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
	user := &spec.User{
		Customer:     ns.Labels["kolihub.io/customer"],
		Organization: ns.Labels["kolihub.io/org"],
		Username:     ns.Annotations["kolihub.io/owner"],
	}
	pns, err := platform.NewNamespace(ns.Name)
	if err != nil {
		glog.Infof("%s - %s", key, err)
		return nil
	}
	if len(user.Username) == 0 {
		c.recorder.Event(ns, v1.EventTypeWarning, "IdentityMismatch", "The namespace doesn't have any owner.")
		return nil
	}
	if pns.Organization != user.Organization || pns.Customer != user.Customer {
		msg := "%s - Identity mismatch: the user belongs to the customer '%s' and organization '%s', " +
			"but the namespace was created for the customer '%s' and organization '%s'."
		msg = fmt.Sprintf(msg, key, user.Customer, user.Organization, pns.Customer, pns.Organization)
		c.recorder.Event(ns, v1.EventTypeWarning, "IdentityMismatch", msg)
		return nil
	}

	// TODO: updating a serviceplan must enqueue all namespaces (label match)
	// TODO: add a label in the namespace indicating the service plan
	var sp *spec.Plan
	options := spec.NewLabel().Add(make(map[string]string))
	planName := ns.Annotations[spec.KoliPrefix("brokerplan")]
	if planName == "" {
		options.Add(map[string]string{spec.KoliPrefix("default"): "true"})
		cache.ListAll(c.spInf.GetStore(), options.AsSelector(), func(obj interface{}) {
			// it will not handle multiple results
			if obj != nil {
				splan := obj.(*spec.Plan)
				if splan.Namespace == pns.GetSystemNamespace() {
					sp = splan
				}
			}
		})
	} else {
		spQ := &spec.Plan{}
		spQ.Name = planName
		spQ.Namespace = pns.GetSystemNamespace()
		obj, exists, err := c.spInf.GetStore().Get(spQ)
		if err != nil || !exists {
			glog.Infof("failed retrieving service plan from the store [%s]", err)
		} else {
			if obj != nil {
				sp = obj.(*spec.Plan)
			}
		}
	}
	if sp == nil {
		sp = &spec.Plan{
			ObjectMeta: metav1.ObjectMeta{
				Name: "",
			},
		}
	}
	// Deep-Copy, otherwise we're mutating our cache
	spCopy, err := util.ServicePlanDeepCopy(sp)
	if err != nil {
		c.recorder.Event(ns, v1.EventTypeWarning, "SetupPlanError", err.Error())
		return fmt.Errorf("%s - failed deep copying service plan '%s' [%s]", key, sp.Name, err)
	}
	spCopy.Spec.Hard.RemoveUnregisteredResources()

	// TODO: test resource quota, update/delete
	// TODO: enqueue all namespaces when updating a service plan - TEST
	// TODO: update or create new quotas

	// Those rules doesn't apply to system broker namespaces
	if !pns.IsSystem() {
		if err := c.enforceQuota(ns.Name, spCopy); err != nil {
			c.recorder.Event(ns, v1.EventTypeWarning, "SetupQuotaError", err.Error())
			return fmt.Errorf("SetupQuota [%s]", err)
		}
		// TODO: check for errors!
		c.enforceRoleBindings(key, ns, user, spCopy)
	}

	if err := c.createDefaultPermissions(ns.Name, user); err != nil {
		c.recorder.Event(ns, v1.EventTypeWarning, "SetupRolesError", err.Error())
		return fmt.Errorf("SetupRoles [%s]", err)
	}
	if err := c.createNetworkPolicy(ns.Name); err != nil {
		c.recorder.Event(ns, v1.EventTypeWarning, "SetupNetworkError", err.Error())
		return fmt.Errorf("SetupNetwork [%s]", err)
	}
	return nil
}

// create the permissions (role/role binding) needed for the user
// to access the resources of the namespace.
func (c *NamespaceController) createDefaultPermissions(ns string, usr *spec.User) error {
	role := &rbac.Role{
		ObjectMeta: metav1.ObjectMeta{Name: "default"},
		Rules: []rbac.PolicyRule{
			{
				APIGroups: []string{"*"},
				Resources: platform.RoleResources,
				Verbs:     platform.RoleVerbs,
			},
		},
	}
	if _, err := c.kclient.Rbac().Roles(ns).Create(role); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("Failed creating role: %s", err)
	}

	roleBinding := &rbac.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "default"},
		Subjects: []rbac.Subject{
			{
				Kind: "User",
				Name: usr.Username, // TODO: change to ID
			},
		},
		RoleRef: rbac.RoleRef{
			Kind: "Role",
			Name: "default", // must match role name
		},
	}
	if _, err := c.kclient.Rbac().RoleBindings(ns).Create(roleBinding); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("Failed creating rolebinding: %s", err)
	}
	return nil
}

func (c *NamespaceController) createNetworkPolicy(ns string) error {
	// allow traffic between all pods in the namespace only
	networkPolicy := &extensions.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "default"},
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
		return fmt.Errorf("Failed creating network policy: %s", err)
	}
	// client-go does not have registered client for creating networking policies
	// if _, err := c.kclient.Extensions().NetworkPolicies(ns).Create(networkPolicy); err != nil && !apierrors.IsAlreadyExists(err) {
	// 	return fmt.Errorf("failed creating network policy: %s", err)
	// }
	return nil
}

func (c *NamespaceController) enforceQuota(ns string, sp *spec.Plan) error {
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

func (c *NamespaceController) enforceRoleBindings(logHeader string, ns *v1.Namespace, user *spec.User, sp *spec.Plan) {
	// Platform Roles
	userRoles := spec.NewPlatformRoles(ns.Annotations[spec.KoliPrefix("roles")])
	hasUserRoles := false
	if len(userRoles) > 0 {
		hasUserRoles = true
	}

	if len(spec.PlatformRegisteredRoles) == 0 {
		glog.Warningf("%s - platform roles not found", logHeader)
	}
	for _, role := range spec.PlatformRegisteredRoles {
		// Manual Operation - it has an annotation
		if hasUserRoles {
			if role.Exists(userRoles) {
				subjects := []rbac.Subject{}
				for _, group := range user.Groups {
					// TODO: adding a new group will not make effect
					// if the rolebinding exists
					subjects = append(subjects, rbac.Subject{Kind: rbac.GroupKind, Name: group})
				}
				subjects = append(subjects, rbac.Subject{Kind: rbac.UserKind, Name: user.Username})
				_, err := c.kclient.Rbac().RoleBindings(ns.Name).Create(role.GetRoleBinding(subjects))
				if err != nil && !apierrors.IsAlreadyExists(err) {
					// TODO: requeue on errors?
					glog.Warningf("%s - failed creating manual role binding '%s': %s", logHeader, role, err)
					continue
				}
				glog.Infof("%s - manual role binding created '%s'", logHeader, role)
			} else {
				opts := &metav1.DeleteOptions{}
				err := c.kclient.Rbac().RoleBindings(ns.Name).Delete(string(role), opts)
				if err != nil && !apierrors.IsNotFound(err) {
					// TODO: requeue on errors?
					glog.Warningf("%s - failed removing manual role binding '%s'", logHeader, role)
					continue
				}
				glog.Infof("%s - manual role binding removed '%s'", logHeader, role)
			}
			// Go to the next platform role because it's a manual operation (has annotations)
			continue
		}

		// Automatic Operation - inherit from a service plan
		if role.Exists(sp.Spec.Roles) {
			roleBinding := role.GetRoleBinding([]rbac.Subject{{Kind: "User", Name: user.Username}})
			_, err := c.kclient.Rbac().RoleBindings(ns.Name).Create(roleBinding)
			if err != nil && !apierrors.IsAlreadyExists(err) {
				glog.Warningf("%s - failed creating role binding '%s': %s", logHeader, role, err)
				continue
			}
			glog.Infof("%s - role binding created '%s'", logHeader, role)
		} else {
			opts := &metav1.DeleteOptions{}
			err := c.kclient.Rbac().RoleBindings(ns.Name).Delete(string(role), opts)
			if err != nil && !apierrors.IsNotFound(err) {
				glog.Warningf("%s - failed removing role binding '%s'", logHeader, role)
			}
		}
	}
}
