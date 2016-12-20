package controller

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/kolibox/koli/pkg/spec"
	"github.com/kolibox/koli/pkg/util"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/apis/rbac"
	"k8s.io/kubernetes/pkg/client/cache"
	"k8s.io/kubernetes/pkg/util/wait"

	apierrors "k8s.io/kubernetes/pkg/api/errors"
	clientset "k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset"
	utilruntime "k8s.io/kubernetes/pkg/util/runtime"
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
	if old.ResourceVersion == new.Annotations["sys.io/resourceVersion"] {
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
	user := ns.Annotations["sys.io/identity"]

	logHeader := fmt.Sprintf("%s(%s)", ns.Name, ns.ResourceVersion)
	if user == "" {
		glog.Infof("%s - empty identity, ignoring...", logHeader)
		return nil
	}

	u := &spec.User{}
	if err := json.Unmarshal([]byte(user), u); err != nil {
		return fmt.Errorf("%s - failed decoding user (%s)", logHeader, err)
	}

	// validate if the namespace is correct
	if err := validateNamespace(ns, u); err != nil {
		return err
	}

	_, exists, err := c.nsInf.GetStore().Get(ns)
	if err != nil {
		return err
	}

	if !exists {
		glog.Infof("%s - namespace doesn't exists", logHeader)
		return nil
	}

	// &v1alpha1.Role{}
	// TODO: if it doesn't exists, set a finalizer: https://github.com/kubernetes/kubernetes/blob/master/docs/design/namespaces.md#finalizers
	// TODO: configure role and role binding - OK
	// TODO: set the proper labels - OK
	// TODO: set network policy annotation - OK
	// TODO: create a network policy allowing traffic between pods in the same namespace
	// TODO: hard-coded quota for namespaces

	label := spec.NewLabel().Add(map[string]string{
		"default":  "true",
		"customer": u.Customer,
		"org":      u.Organization,
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
			n.Labels["sys.io/default"] = "true"

			if _, err := c.kclient.Core().Namespaces().Update(n); err != nil {
				return fmt.Errorf("%s - failed updating a default namespace (%s)", logHeader, err)
			}
			glog.Infof("%s - default namespace updated for user '%s'", logHeader, u.Username)
		}
		return nil
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
	nsCopy.Annotations["net.beta.kubernetes.io/network-policy"] = `"{"ingress": {"isolation": "DefaultDeny"}}"`
	// Prevent infinite loop on updateFunc
	nsCopy.Annotations["sys.io/resourceVersion"] = nsCopy.ResourceVersion
	nsCopy.Labels = label.Set

	if _, err := c.kclient.Core().Namespaces().Update(nsCopy); err != nil {
		return fmt.Errorf("%s - failed updating namespace: %s", logHeader, err)
	}

	// TODO: requeue on errors?
	c.createPermissions(ns.Name, u)
	c.createNetworkPolicy()

	// a, exists, err := c.addonInf.GetStore().GetByKey(key)
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

	// TODO: temporary solution: v1alpha1.RoleRef doesn't have RoleRef.Namespace, needed in version 1.4
	// data := `{
	// 	"roleRef": {"kind": "Role", "namespace": "%s", "name": "main"},
	// 	"kind": "RoleBinding",
	// 	"subjects": [{"kind": "User", "name": "%s"}],
	// 	"apiVersion": "rbac.authorization.k8s.io/v1alpha1",
	// 	"metadata": {"name": "main", "namespace": "%s"}}`

	roleBinding := &rbac.RoleBinding{
		ObjectMeta: api.ObjectMeta{Name: "main"},
		Subjects: []rbac.Subject{
			{
				Kind: "User",
				Name: usr.Username, // TODO: change to ID
				// Namespace: ns,
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
	// result := c.kclient.Rbac().RESTClient().Post().
	// 	Namespace(ns).
	// 	Resource("rolebindings").
	// 	Body([]byte(fmt.Sprintf(data, ns, usr.Username, ns))).
	// 	Do()
	// if err := result.Error(); err != nil && !apierrors.IsAlreadyExists(err) {
	// 	glog.Errorf("failed creating role binding (%s)", err)
	// }

}

func (c *NamespaceController) createNetworkPolicy() {
	// TODO: change the library to kubernetes, client-go doesn't have NetworkPoliciesGetter
	// extensions.NetworkPolicy{

	// }
}

// validateNamespace check if the namespace has the proper format
// and matchs with the identity information
func validateNamespace(ns *api.Namespace, u *spec.User) error {
	parts := strings.Split(ns.Name, "-")
	if len(parts) != 3 {
		return fmt.Errorf("namespace %s is invalid, must be in the form [name]-[customer]-[org]", ns.Name)
	}
	if u.Customer != parts[1] || u.Organization != parts[2] {
		msg := "identity information mismatch. user-customer=%s user-org=%s ns-customer=%s ns-org=%s"
		return fmt.Errorf(msg, u.Customer, u.Organization, parts[1], parts[2])
	}
	return nil
}
