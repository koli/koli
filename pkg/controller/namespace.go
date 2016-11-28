package controller

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kolibox/koli/pkg/spec"
	"github.com/kolibox/koli/pkg/util"

	"github.com/golang/glog"
	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/pkg/api"
	apierrors "k8s.io/client-go/1.5/pkg/api/errors"
	"k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/pkg/apis/rbac/v1alpha1"
	utilruntime "k8s.io/client-go/1.5/pkg/util/runtime"
	"k8s.io/client-go/1.5/pkg/util/wait"
	"k8s.io/client-go/1.5/tools/cache"
)

// NamespaceController .
type NamespaceController struct {
	kclient *kubernetes.Clientset
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
func NewNamespaceController(nsInformer cache.SharedIndexInformer, client *kubernetes.Clientset) *NamespaceController {
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
	new := n.(*v1.Namespace)
	glog.Infof("addNamespace: %s", new.Name)
	c.queue.add(new)
}

func (c *NamespaceController) updateNamespace(o, n interface{}) {
	old := o.(*v1.Namespace)
	new := n.(*v1.Namespace)

	if old.ResourceVersion == new.ResourceVersion {
		return
	}

	// Prevent infinite loop on updateFunc
	// TODO: test this behavior with delete handler
	if old.ResourceVersion == new.Annotations["sys.io/resourceVersion"] {
		glog.Info("skipping internal update...")
		return
	}

	glog.Infof("updateNamespace: %s", new.Name)
	c.queue.add(new)
}

func (c *NamespaceController) deleteNamespace(n interface{}) {
	ns := n.(*v1.Namespace)
	glog.Infof("deleteNamespace: %s", ns.Name)
}

// Run the controller.
func (c *NamespaceController) Run(workers int, stopc <-chan struct{}) {
	// don't let panics crash the process
	defer utilruntime.HandleCrash()
	// make sure the work queue is shutdown which will trigger workers to end
	defer c.queue.close()

	glog.Info("Starting namespace controller...")

	for i := 0; i < workers; i++ {
		// runWorker will loop until "something bad" happens.
		// The .Until will then rekick the worker after one second
		go wait.Until(c.runWorker, time.Second, stopc)
	}

	if !cache.WaitForCacheSync(stopc, c.nsInf.HasSynced) {
		return
	}

	// wait until we're told to stop
	<-stopc
	glog.Info("shutting down namespace controller")
}

// var keyFunc = cache.DeletionHandlingMetaNamespaceKeyFunc

// runWorker runs a worker thread that just dequeues items, processes them, and marks them done.
// It enforces that the syncHandler is never invoked concurrently with the same key.
func (c *NamespaceController) runWorker() {
	for {
		obj, ok := c.queue.pop(&v1.Namespace{})
		if !ok {
			return
		}
		if err := c.reconcile(obj.(*v1.Namespace)); err != nil {
			utilruntime.HandleError(err)
		}
	}
}

func (c *NamespaceController) reconcile(ns *v1.Namespace) error {
	user := ns.Annotations["sys.io/identity"]
	if user == "" {
		glog.Infof("empty identity (%s), ignoring...", ns.Name)
		return nil
	}

	u := &spec.User{}
	if err := json.Unmarshal([]byte(user), u); err != nil {
		return fmt.Errorf("failed decoding user (%s)", err)
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
		glog.Infof("namespace %s doesn't exists", ns.Name)
		return nil
	}

	// &v1alpha1.Role{}
	// TODO: if it doesn't exists, set a finalizer: https://github.com/kubernetes/kubernetes/blob/master/docs/design/namespaces.md#finalizers
	// TODO: configure role and role binding - OK
	// TODO: set the proper labels - OK
	// TODO: set network policy annotation - OK
	// TODO: create a network policy allowing traffic between pods in the same namespace
	// TODO: hard-coded quota for namespaces

	label := spec.NewLabel()
	label.Add(map[string]string{
		"default":  "true",
		"customer": u.Customer,
		"org":      u.Organization,
	})

	// try to update the default namespace for the user
	if !exists || ns.Status.Phase == v1.NamespaceTerminating {
		glog.Infof("namespace %s doesn't exist, repairing ...", ns.Name)
		label.Remove("default")
		nss, err := c.kclient.Core().Namespaces().List(api.ListOptions{LabelSelector: label.AsSelector()})
		if err != nil {
			return fmt.Errorf("failed retrieving list of namespaces (%s)", err)
		}
		// Updates only if there's one namespace
		if len(nss.Items) == 1 {
			n, err := util.NamespaceDeepCopy(&nss.Items[0])
			if err != nil {
				return fmt.Errorf("failed creating a namespace copy (%s)", err)
			}
			n.Labels["sys.io/default"] = "true"

			if _, err := c.kclient.Core().Namespaces().Update(n); err != nil {
				return fmt.Errorf("failed updating a default namespace (%s)", err)
			}
			glog.Infof("default namespace update (%s/%s)", n.Name, u.Username)
		}
		return nil
	}

	nsCopy, err := util.NamespaceDeepCopy(ns)
	if err != nil {
		return fmt.Errorf("failed copying namespace %s (%s)", ns.Name, err)
	}

	nss, err := c.kclient.Core().Namespaces().List(api.ListOptions{LabelSelector: label.AsSelector()})
	if err != nil {
		return fmt.Errorf("failed listing namespaces (%s)", err)
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
		return fmt.Errorf("failed updating namespace %s (%s)", ns.Name, err)
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
	role := &v1alpha1.Role{
		ObjectMeta: v1.ObjectMeta{Name: "main"},
		Rules: []v1alpha1.PolicyRule{
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
	data := `{
		"roleRef": {"kind": "Role", "namespace": "%s", "name": "main"}, 
		"kind": "RoleBinding", 
		"subjects": [{"kind": "User", "name": "%s"}], 
		"apiVersion": "rbac.authorization.k8s.io/v1alpha1", 
		"metadata": {"name": "main", "namespace": "%s"}}`
	result := c.kclient.Rbac().GetRESTClient().Post().
		Namespace(ns).
		Resource("rolebindings").
		Body([]byte(fmt.Sprintf(data, ns, usr.Username, ns))).
		Do()
	if err := result.Error(); err != nil && !apierrors.IsAlreadyExists(err) {
		glog.Errorf("failed creating role binding (%s)", err)
	}

	// roleBinding := &v1alpha1.RoleBinding{
	// 	ObjectMeta: v1.ObjectMeta{Name: "main"},
	// 	Subjects: []v1alpha1.Subject{
	// 		{
	// 			Kind: "User",
	// 			Name: usr.Username, // TODO: change to ID
	// 			// Namespace: ns,
	// 		},
	// 	},
	// 	RoleRef: v1alpha1.RoleRef{
	// 		Kind: "Role",
	// 		Name: "main", // must match role name
	// 		// Namespace: "", // TOOD: Need this field because the server version is still 1.4
	// 	},
	// }

	// c.kclient.CoreClient.Client.Post
	// client := c.kclient.Rbac().GetRESTClient()
	// if _, err := c.kclient.Rbac().RoleBindings(ns).Create(roleBinding); err != nil {
	// 	return fmt.Errorf("failed creating role binding (%s)", err)
	// }
}

func (c *NamespaceController) createNetworkPolicy() {
	// TODO: change the library to kubernetes, client-go doesn't have NetworkPoliciesGetter

}

// validateNamespace check if the namespace has the proper format
// and matchs with the identity information
func validateNamespace(ns *v1.Namespace, u *spec.User) error {
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
