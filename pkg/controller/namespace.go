package controller

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/golang/glog"
	"github.com/kolibox/koli/pkg/spec"
	"github.com/kolibox/koli/pkg/util"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/apis/rbac"
	"k8s.io/kubernetes/pkg/client/cache"
	"k8s.io/kubernetes/pkg/util/wait"

	apierrors "k8s.io/kubernetes/pkg/api/errors"
	extensions "k8s.io/kubernetes/pkg/apis/extensions"
	clientset "k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset"
	utilruntime "k8s.io/kubernetes/pkg/util/runtime"

	"k8s.io/kubernetes/pkg/api/unversioned"
)

// NamespaceController controller
type NamespaceController struct {
	kclient clientset.Interface
	nsInf   cache.SharedIndexInformer
	queue   *queue
}

var (
	roleVerbs = []string{
		"get", "watch", "list", "exec", "port-forward", "logs", "scale",
		"attach", "create", "describe", "delete", "update",
	}
	roleResources = []string{
		"pods", "deployments", "namespaces", "replicasets",
		"resourcequotas", "horizontalpodautoscalers",
	}
)

// NewNamespaceController creates a NamespaceController
func NewNamespaceController(nsInformer cache.SharedIndexInformer, client clientset.Interface) *NamespaceController {
	nc := &NamespaceController{
		kclient: client,
		nsInf:   nsInformer,
		queue:   newQueue(200),
	}
	nc.nsInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    nc.addNamespace,
		UpdateFunc: nc.updateNamespace,
		DeleteFunc: nc.deleteNamespace,
	})
	return nc
}

func (c *NamespaceController) addNamespace(n interface{}) {
	new := n.(*api.Namespace)
	glog.Infof("add-namespace - %s(%s)", new.Name, new.ResourceVersion)
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
		glog.Info("skipping internal update...")
		return
	}

	glog.Infof("update-namespace - %s(%s)", new.Name, new.ResourceVersion)
	c.queue.add(new)
}

func (c *NamespaceController) deleteNamespace(n interface{}) {
	ns := n.(*api.Namespace)
	glog.Infof("delete-namespace - %s(%s)", ns.Name, ns.ResourceVersion)
}

// Run the controller.
func (c *NamespaceController) Run(workers int, stopc <-chan struct{}) {
	// don't let panics crash the process
	defer utilruntime.HandleCrash()
	// make sure the work queue is shutdown which will trigger workers to end
	defer c.queue.close()

	glog.Info("Starting Namespace Controller...")

	if !cache.WaitForCacheSync(stopc, c.nsInf.HasSynced) {
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
		glog.Infof("%s - namespace doesn't exist, repairing ...", logHeader)
		label.Remove("default")
		nss, err := c.kclient.Core().Namespaces().List(api.ListOptions{LabelSelector: label.AsSelector()})
		if err != nil {
			return fmt.Errorf("%s - failed retrieving list of namespaces (%s)", logHeader, err)
		}
		// Updates only if there's one namespace
		if len(nss.Items) == 1 {
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
	bns, err := util.NewBrokerNamespace(ns.Name)
	if err != nil {
		glog.Info("%s - %s", logHeader, err)
		return nil
	}

	if bns.Organization != user.Organization || bns.Customer != user.Customer {
		msg := "identity mismatch. user=%s customer=%s org=%s ns=%s"
		return fmt.Errorf(msg, user.Username, user.Customer, user.Organization, bns.GetNamespace())
	}

	nsCopy, err := util.NamespaceDeepCopy(ns)
	if err != nil {
		return fmt.Errorf("%s - failed copying namespace: %s", logHeader, err)
	}

	nss, err := c.kclient.Core().Namespaces().List(api.ListOptions{LabelSelector: label.AsSelector()})
	if err != nil {
		return fmt.Errorf("%s - failed listing namespaces: %s", logHeader, err)
	}
	// Remove the key 'default' because a default namespace already exists for this customer
	if len(nss.Items) > 0 {
		label.Remove("default")
	}
	nsCopy.Annotations["net.beta.kubernetes.io/network-policy"] = `{"ingress": {"isolation": "DefaultDeny"}}`
	// Prevent infinite loop on updateFunc
	nsCopy.Annotations[spec.KoliPrefix("resourceVersion")] = nsCopy.ResourceVersion
	nsCopy.Labels = label.Set

	if _, err := c.kclient.Core().Namespaces().Update(nsCopy); err != nil {
		return fmt.Errorf("%s - failed updating namespace: %s", logHeader, err)
	}

	// TODO: requeue on errors?
	c.createPermissions(ns.Name, user)
	c.createNetworkPolicy(ns.Name)

	return nil
}

// create the permissions (role/role binding) needed for the user
// to access the resources of the namespace.
func (c *NamespaceController) createPermissions(ns string, usr *spec.User) {
	role := &rbac.Role{
		ObjectMeta: api.ObjectMeta{Name: "main"},
		Rules: []rbac.PolicyRule{
			{
				APIGroups: []string{"*"},
				Resources: roleResources,
				Verbs:     roleVerbs,
			},
		},
	}
	if _, err := c.kclient.Rbac().Roles(ns).Create(role); err != nil && !apierrors.IsAlreadyExists(err) {
		glog.Errorf("failed creating role (%s)", err)
	}

	roleBinding := &rbac.RoleBinding{
		ObjectMeta: api.ObjectMeta{Name: "main"},
		Subjects: []rbac.Subject{
			{
				Kind: "User",
				Name: usr.Username, // TODO: change to ID
			},
		},
		RoleRef: rbac.RoleRef{
			Kind: "Role",
			Name: "main", // must match role name
		},
	}
	if _, err := c.kclient.Rbac().RoleBindings(ns).Create(roleBinding); err != nil && !apierrors.IsAlreadyExists(err) {
		glog.Errorf("failed creating rolebinding: %s", err)
	}
}

func (c *NamespaceController) createNetworkPolicy(ns string) {
	// allow traffic between all pods in the namespace only
	networkPolicy := &extensions.NetworkPolicy{
		ObjectMeta: api.ObjectMeta{Name: "main"},
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
		glog.Warningf("failed creating network policy: %s", err)
	}
}
