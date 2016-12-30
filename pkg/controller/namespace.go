package controller

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/golang/glog"
	"github.com/kolibox/koli/pkg/clientset"
	"github.com/kolibox/koli/pkg/platform"
	"github.com/kolibox/koli/pkg/spec"
	"github.com/kolibox/koli/pkg/spec/util"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/apis/rbac"
	"k8s.io/kubernetes/pkg/client/cache"
	"k8s.io/kubernetes/pkg/util/wait"

	apierrors "k8s.io/kubernetes/pkg/api/errors"
	extensions "k8s.io/kubernetes/pkg/apis/extensions"
	kclientset "k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset"
	utilruntime "k8s.io/kubernetes/pkg/util/runtime"

	"k8s.io/kubernetes/pkg/api/unversioned"
)

// NamespaceController controller
type NamespaceController struct {
	kclient   kclientset.Interface
	sysClient clientset.CoreInterface

	nsInf cache.SharedIndexInformer
	spInf cache.SharedIndexInformer

	queue *queue
}

const (
	// the ammount of namespaces which a user is able to provision
	hardLimitNamespace = 2
)

// NewNamespaceController creates a NamespaceController
func NewNamespaceController(nsInf, spInf cache.SharedIndexInformer,
	kclient kclientset.Interface, sysClient clientset.CoreInterface) *NamespaceController {
	nc := &NamespaceController{
		kclient:   kclient,
		sysClient: sysClient,
		nsInf:     nsInf,
		spInf:     spInf,
		queue:     newQueue(200),
	}
	nc.nsInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    nc.addNamespace,
		UpdateFunc: nc.updateNamespace,
		DeleteFunc: nc.deleteNamespace,
	})

	nc.spInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: nc.updateServicePlan,
	})

	return nc
}

func (c *NamespaceController) addNamespace(n interface{}) {
	new := n.(*api.Namespace)
	glog.Infof("%s(%s) - add-namespace", new.Name, new.ResourceVersion)
	c.queue.add(new)
}

func (c *NamespaceController) updateNamespace(o, n interface{}) {
	old := o.(*api.Namespace)
	new := n.(*api.Namespace)

	if old.ResourceVersion == new.ResourceVersion {
		return
	}

	// Prevent infinite loop on updateFunc
	// TODO: test this behavior with delete handler
	if old.ResourceVersion == new.Annotations[spec.KoliPrefix("resourceVersion")] {
		glog.Infof("%s(%s) - skipping internal update...", new.Name, new.ResourceVersion)
		return
	}

	glog.Infof("%s(%s) - update-namespace", new.Name, new.ResourceVersion)
	c.queue.add(new)
}

func (c *NamespaceController) deleteNamespace(n interface{}) {
	ns := n.(*api.Namespace)
	glog.Infof("%s(%s) - delete-namespace", ns.Name, ns.ResourceVersion)
}

func (c *NamespaceController) updateServicePlan(o, n interface{}) {
	old := o.(*spec.ServicePlan)
	new := n.(*spec.ServicePlan)

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
		glog.Infof("%s/%s - update-service-plan", new.Namespace, new.Name)
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
		ns := obj.(*api.Namespace)
		if pns.GetSystemNamespace() == ns.Name {
			// skip the system namespace
			return
		}
		c.queue.add(ns)
	})
}

// Run the controller.
func (c *NamespaceController) Run(workers int, stopc <-chan struct{}) {
	// don't let panics crash the process
	defer utilruntime.HandleCrash()
	// make sure the work queue is shutdown which will trigger workers to end
	defer c.queue.close()

	glog.Info("Starting Namespace Controller...")

	if !cache.WaitForCacheSync(stopc, c.nsInf.HasSynced, c.spInf.HasSynced) {
		return
	}

	for i := 0; i < workers; i++ {
		// runWorker will loop until "something bad" happens.
		// The .Until will then rekick the worker after one second
		go wait.Until(c.runWorker, time.Second, stopc)
	}

	// wait until we're told to stop
	<-stopc
	glog.Info("Shutting down Namespace Controller")
}

// var keyFunc = cache.DeletionHandlingMetaNamespaceKeyFunc

// runWorker runs a worker thread that just dequeues items, processes them, and marks them done.
// It enforces that the syncHandler is never invoked concurrently with the same key.
func (c *NamespaceController) runWorker() {
	for {
		obj, ok := c.queue.pop(&api.Namespace{})
		if !ok {
			return
		}
		if err := c.reconcile(obj.(*api.Namespace)); err != nil {
			utilruntime.HandleError(err)
		}
	}
}

func (c *NamespaceController) reconcile(ns *api.Namespace) error {
	key, err := keyFunc(ns)
	if err != nil {
		return err
	}
	identity := ns.Annotations[spec.KoliPrefix("identity")]
	logHeader := fmt.Sprintf("%s(%s)", ns.Name, ns.ResourceVersion)
	if identity == "" {
		glog.Infof("%s - empty identity, ignoring...", logHeader)
		return nil
	}

	_, exists, err := c.nsInf.GetStore().GetByKey(key)
	if err != nil {
		return err
	}

	// TODO: Should we manage several users using annotations?
	// A third party resource could exist to manage the users
	user := &spec.User{}
	if err := json.Unmarshal([]byte(identity), user); err != nil {
		return fmt.Errorf("%s - failed decoding user (%s)", logHeader, err)
	}

	label := spec.NewLabel().Add(map[string]string{
		"default":  "true",
		"customer": user.Customer,
		"org":      user.Organization,
	})

	// try to update the default namespace for the user
	if !exists || ns.Status.Phase == api.NamespaceTerminating {
		glog.Infof("%s - namespace doesn't exist", logHeader)
		label.Remove(spec.KoliPrefix("default"))
		// TODO: use cache to list!
		nss, err := c.kclient.Core().Namespaces().List(api.ListOptions{LabelSelector: label.AsSelector()})
		if err != nil {
			return fmt.Errorf("%s - failed retrieving list of namespaces (%s)", logHeader, err)
		}
		// Updates only if there's one namespace
		if len(nss.Items) == 1 {
			glog.Infof("%s - found one namespace, configuring it to default ...", logHeader)
			n, err := util.NamespaceDeepCopy(&nss.Items[0])
			if err != nil {
				return fmt.Errorf("%s - failed creating a namespace copy (%s)", logHeader, err)
			}
			n.Labels[spec.KoliPrefix("default")] = "true"

			if _, err := c.kclient.Core().Namespaces().Update(n); err != nil {
				return fmt.Errorf("%s - failed updating a default namespace (%s)", logHeader, err)
			}
			glog.Infof("%s - default namespace updated for user '%s'", logHeader, user.Username)
		}
		return nil
	}
	pns, err := platform.NewNamespace(ns.Name)
	if err != nil {
		glog.Info("%s - %s", logHeader, err)
		return nil
	}

	if pns.Organization != user.Organization || pns.Customer != user.Customer {
		msg := "identity mismatch. user=%s customer=%s org=%s ns=%s"
		return fmt.Errorf(msg, user.Username, user.Customer, user.Organization, pns.GetNamespace())
	}

	// TODO: updating a serviceplan must enqueue all namespaces (label match)
	// TODO: add a label in the namespace indicating the service plan

	var sp *spec.ServicePlan
	options := spec.NewLabel().Add(label.Set)
	planName := ns.Annotations[spec.KoliPrefix("brokerplan")]
	if planName == "" {
		options.Add(map[string]string{spec.KoliPrefix("default"): "true"})
		cache.ListAll(c.spInf.GetStore(), options.AsSelector(), func(obj interface{}) {
			// it will not handle multiple results
			if obj != nil {
				splan := obj.(*spec.ServicePlan)
				if splan.Namespace == pns.GetSystemNamespace() {
					sp = splan
				}
			}
		})
	} else {
		spQ := &spec.ServicePlan{}
		spQ.Name = planName
		spQ.Namespace = pns.GetSystemNamespace()
		obj, exists, err := c.spInf.GetStore().Get(spQ)
		if err != nil || !exists {
			glog.Infof("%s - failed retrieving service plan from the store. %s", err)
		} else {
			if obj != nil {
				sp = obj.(*spec.ServicePlan)
			}
		}
	}
	if sp == nil {
		sp = &spec.ServicePlan{
			ObjectMeta: api.ObjectMeta{
				Name: "",
			},
		}
	}
	// Deep-Copy, otherwise we're mutating our cache
	spCopy, err := util.ServicePlanDeepCopy(sp)
	if err != nil {
		return fmt.Errorf("%s - failed deep copying service plan '%s': %s", logHeader, sp.Name, err)
	}
	spCopy.Spec.Hard.RemoveUnregisteredResources()

	// TODO: test resource quota, update/delete
	// TODO: enqueue all namespaces when updating a service plan - TEST
	// TODO: update or create new quotas
	// TODO: requeue on errors?

	// Those rules doesn't apply to system broker namespaces
	if !pns.IsSystem() {
		if err := c.enforceQuota(ns.Name, spCopy); err != nil {
			glog.Warningf("%s - %s", logHeader, err)
		}
		c.enforceRoleBindings(logHeader, ns, user, spCopy)
	}

	if err := c.createDefaultPermissions(ns.Name, user); err != nil {
		return fmt.Errorf("%s - %s", logHeader, err)
	}
	if err := c.createNetworkPolicy(ns.Name); err != nil {
		return fmt.Errorf("%s - %s", logHeader, err)
	}

	// update the namespace
	nsCopy, err := util.NamespaceDeepCopy(ns)
	if err != nil {
		return fmt.Errorf("%s - failed copying namespace: %s", logHeader, err)
	}

	// TODO: use cache store to list
	nss, err := c.kclient.Core().Namespaces().List(api.ListOptions{LabelSelector: label.AsSelector()})
	if err != nil {
		return fmt.Errorf("%s - failed listing namespaces: %s", logHeader, err)
	}

	// Remove the key 'default' because a default namespace already exists for this customer
	if len(nss.Items) > 0 {
		label.Remove(spec.KoliPrefix("default"))
	}
	nsCopy.Annotations["net.beta.kubernetes.io/network-policy"] = `{"ingress": {"isolation": "DefaultDeny"}}`
	// Prevent infinite loop on updateFunc
	nsCopy.Annotations[spec.KoliPrefix("resourceVersion")] = nsCopy.ResourceVersion
	if sp.Name != "" && !pns.IsSystem() {
		label.Add(map[string]string{"brokerplan": sp.Name})
	}
	nsCopy.Labels = label.Set

	if _, err := c.kclient.Core().Namespaces().Update(nsCopy); err != nil {
		return fmt.Errorf("%s - failed updating namespace: %s", logHeader, err)
	}
	return nil
}

// create the permissions (role/role binding) needed for the user
// to access the resources of the namespace.
func (c *NamespaceController) createDefaultPermissions(ns string, usr *spec.User) error {
	role := &rbac.Role{
		ObjectMeta: api.ObjectMeta{Name: "default"},
		Rules: []rbac.PolicyRule{
			{
				APIGroups: []string{"*"},
				Resources: platform.RoleResources,
				Verbs:     platform.RoleVerbs,
			},
		},
	}
	if _, err := c.kclient.Rbac().Roles(ns).Create(role); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed creating role: %s", err)
	}

	roleBinding := &rbac.RoleBinding{
		ObjectMeta: api.ObjectMeta{Name: "default"},
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
		return fmt.Errorf("failed creating rolebinding: %s", err)
	}
	return nil
}

func (c *NamespaceController) createNetworkPolicy(ns string) error {
	// allow traffic between all pods in the namespace only
	networkPolicy := &extensions.NetworkPolicy{
		ObjectMeta: api.ObjectMeta{Name: "default"},
		Spec: extensions.NetworkPolicySpec{
			PodSelector: unversioned.LabelSelector{},
			Ingress: []extensions.NetworkPolicyIngressRule{
				{
					From: []extensions.NetworkPolicyPeer{
						{
							PodSelector: &unversioned.LabelSelector{
								MatchLabels: nil,
							},
						},
					},
				},
			},
		},
	}
	if _, err := c.kclient.Extensions().NetworkPolicies(ns).Create(networkPolicy); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed creating network policy: %s", err)
	}
	return nil
}

func (c *NamespaceController) enforceQuota(ns string, sp *spec.ServicePlan) error {
	rq := &api.ResourceQuota{
		ObjectMeta: api.ObjectMeta{
			Name: "default",
		},
		Spec: api.ResourceQuotaSpec{
			Hard: api.ResourceList(sp.Spec.Hard),
		},
	}
	_, err := c.kclient.Core().ResourceQuotas(ns).Update(rq)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed updating quota using service plan '%s': %s", sp.Name, err)
	}
	if apierrors.IsNotFound(err) {
		if _, err := c.kclient.Core().ResourceQuotas(ns).Create(rq); err != nil {
			return fmt.Errorf("failed creating quota using service plan '%s': %s", sp.Name, err)
		}
	}
	return nil
}

func (c *NamespaceController) enforceRoleBindings(logHeader string, ns *api.Namespace, user *spec.User, sp *spec.ServicePlan) {
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
				opts := &api.DeleteOptions{}
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
			roleBinding := role.GetRoleBinding([]rbac.Subject{{Kind: "User", Name: "sandromello"}})
			_, err := c.kclient.Rbac().RoleBindings(ns.Name).Create(roleBinding)
			if err != nil && !apierrors.IsAlreadyExists(err) {
				glog.Warningf("%s - failed creating role binding '%s': %s", logHeader, role, err)
				continue
			}
			glog.Infof("%s - role binding created '%s'", logHeader, role)
		} else {
			opts := &api.DeleteOptions{}
			err := c.kclient.Rbac().RoleBindings(ns.Name).Delete(string(role), opts)
			if err != nil && !apierrors.IsNotFound(err) {
				glog.Warningf("%s - failed removing role binding '%s'", logHeader, role)
			}
		}
	}
}
