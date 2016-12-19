package controller

import (
	"fmt"
	"time"

	"github.com/golang/glog"
	"github.com/kolibox/koli/pkg/spec"
	"github.com/kolibox/koli/pkg/util"

	"k8s.io/client-go/1.5/kubernetes"
	apierrors "k8s.io/client-go/1.5/pkg/api/errors"
	"k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/pkg/apis/apps/v1alpha1"
	extensions "k8s.io/client-go/1.5/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/1.5/pkg/labels"
	utilruntime "k8s.io/client-go/1.5/pkg/util/runtime"
	"k8s.io/client-go/1.5/pkg/util/wait"
	"k8s.io/client-go/1.5/tools/cache"
)

const (
	tprAddons = "addon.sys.koli.io"
)

// AddonController controller
type AddonController struct {
	kclient *kubernetes.Clientset

	addonInf cache.SharedIndexInformer
	spInf    cache.SharedIndexInformer
	psetInf  cache.SharedIndexInformer

	queue *queue
}

// NewAddonController creates a new addon controller
func NewAddonController(addonInformer, psetInformer, spInformer cache.SharedIndexInformer, client *kubernetes.Clientset) *AddonController {
	ac := &AddonController{
		kclient:  client,
		addonInf: addonInformer,
		psetInf:  psetInformer,
		spInf:    spInformer,
		queue:    newQueue(200),
	}

	ac.addonInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    ac.addAddon,
		UpdateFunc: ac.updateAddon,
		DeleteFunc: ac.deleteAddon,
	})

	ac.psetInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: ac.updatePetSet,
		DeleteFunc: ac.deletePetSet,
	})

	// ac.spInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
	// 	UpdateFunc: ac.updateServicePlan,
	// })

	return ac
}

// Run the controller.
func (c *AddonController) Run(workers int, stopc <-chan struct{}) {
	// don't let panics crash the process
	defer utilruntime.HandleCrash()
	// make sure the work queue is shutdown which will trigger workers to end
	defer c.queue.close()

	glog.Info("Starting Addon Controller...")

	if !cache.WaitForCacheSync(stopc, c.addonInf.HasSynced, c.psetInf.HasSynced, c.spInf.HasSynced) {
		return
	}

	for i := 0; i < workers; i++ {
		// runWorker will loop until "something bad" happens.
		// The .Until will then rekick the worker after one second
		go wait.Until(c.runWorker, time.Second, stopc)
	}

	// wait until we're told to stop
	<-stopc
	glog.Info("shutting down Addon Controller")
}

// func (c *AddonController) updateServicePlan(o, n interface{}) {
// 	old := o.(*spec.ServicePlan)
// 	new := n.(*spec.ServicePlan)

// 	if old.ResourceVersion == new.ResourceVersion {
// 		return
// 	}

// 	// When a user associates a Service Plan to a new one
// 	if !reflect.DeepEqual(old.Labels, new.Labels) && new.Namespace != systemNamespace {
// 		c.enqueueForNamespace(new.Namespace)
// 	}
// }

// enqueueForNamespace enqueues all Deployments object keys that belong to the given namespace.
func (c *AddonController) enqueueForNamespace(namespace string) {
	cache.ListAll(c.psetInf.GetStore(), labels.Everything(), func(obj interface{}) {
		d := obj.(*v1alpha1.PetSet)
		if d.Namespace == namespace {
			c.queue.add(d)
		}
	})
}

func (c *AddonController) addAddon(a interface{}) {
	addon := a.(*spec.Addon)
	glog.Infof("CREATE ADDON: (%s/%s), spec.type (%s)", addon.Namespace, addon.Name, addon.Spec.Type)
	c.enqueueAddon(addon)
}

func (c *AddonController) updateAddon(o, n interface{}) {
	old := o.(*spec.Addon)
	new := n.(*spec.Addon)

	if old.ResourceVersion == new.ResourceVersion {
		return
	}

	glog.Infof("UPDATE ADDON: (%s/%s), spec.type (%s)", new.Namespace, new.Name, new.Spec.Type)
	c.enqueueAddon(new)
}

func (c *AddonController) deleteAddon(a interface{}) {
	addon := a.(*spec.Addon)
	glog.Infof("DELETE ADDON: (%s/%s), spec.type (%s)", addon.Namespace, addon.Name, addon.Spec.Type)
	c.enqueueAddon(addon)
}

func (c *AddonController) updatePetSet(o, n interface{}) {
	old := o.(*v1alpha1.PetSet)
	new := n.(*v1alpha1.PetSet)
	// Periodic resync may resend the deployment without changes in-between.
	// Also breaks loops created by updating the resource ourselves.
	if old.ResourceVersion == new.ResourceVersion {
		return
	}

	glog.Infof("updateDeployment: (%s/%s)", new.Namespace, new.Name)
	if addon := c.addonForDeployment(new); addon != nil {
		c.enqueueAddon(addon)
	}
}

func (c *AddonController) deletePetSet(a interface{}) {
	d := a.(*v1alpha1.PetSet)
	glog.Infof("deleteDeployment: (%s/%s)", d.Namespace, d.Name)
	if addon := c.addonForDeployment(d); addon != nil {
		c.enqueueAddon(addon)
	}
}

func (c *AddonController) enqueueAddon(addon *spec.Addon) {
	c.queue.add(addon)
}

// runWorker runs a worker thread that just dequeues items, processes them, and marks them done.
// It enforces that the syncHandler is never invoked concurrently with the same key.
func (c *AddonController) runWorker() {
	for {
		a, ok := c.queue.pop(&spec.Addon{})
		if !ok {
			return
		}
		// Get the app based on its type
		app, err := a.(*spec.Addon).GetApp(c.kclient, c.psetInf)
		if err != nil {
			// If an add-on is provided without a known type
			utilruntime.HandleError(err)
		}
		if err := c.reconcile(app); err != nil {
			utilruntime.HandleError(err)
		}
	}
}

func (c *AddonController) reconcile(app spec.AddonInterface) error {
	key, err := keyFunc(app.GetAddon())
	if err != nil {
		return err
	}

	addon := app.GetAddon()
	logHeader := fmt.Sprintf("%s/%s(%s)", addon.Namespace, addon.Name, addon.ResourceVersion)
	bns, err := util.NewBrokerNamespace(addon.Namespace)
	if err != nil {
		// Skip only because it's not a valid namespace to process
		glog.Warningf("%s - %s. skipping ...", logHeader, err)
		return nil
	}

	_, exists, err := c.addonInf.GetStore().GetByKey(key)
	if err != nil {
		return err
	}
	if !exists {
		// TODO: we want to do server side deletion due to the variety of
		// resources we create.
		// Doing so just based on the deletion event is not reliable, so
		// we have to garbage collect the controller-created resources in some other way.
		//
		// Let's rely on the index key matching that of the created configmap and replica
		// set for now. This does not work if we delete addon resources as the
		// controller is not running – that could be solved via garbage collection later.
		glog.Infof("%s - deleting deployment (%v) ...", logHeader, key)
		return app.DeleteApp()
	}

	// Delete the auto-generate configuration.
	// TODO: add an ownerRef at creation
	if err := app.CreateConfigMap(); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}

	// expose the app
	svc := spec.MakePetSetService(addon)
	if _, err := c.kclient.Core().Services(addon.Namespace).Create(svc); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed creating service (%s)", err)
	}

	// Ensure we have a replica set running
	psetQ := &v1alpha1.PetSet{}
	psetQ.Namespace = addon.Namespace
	psetQ.Name = addon.Name

	obj, psetExists, err := c.psetInf.GetStore().Get(psetQ)
	if err != nil {
		return err
	}

	planName := ""
	var sp *spec.ServicePlan
	selector := spec.NewLabel().Add(map[string]string{"default": "true"}).AsSelector()
	clusterPlanPrefix := spec.KoliPrefix("clusterplan")
	if psetExists {
		pset := obj.(*v1alpha1.PetSet)
		if pset.DeletionTimestamp != nil {
			glog.Infof("%s - marked for deletion, skipping ...", logHeader)
			return nil
		}
		planName = pset.Labels[clusterPlanPrefix]
	}

	// Find a default broker service plan
	if planName == "" {
		cache.ListAll(c.spInf.GetStore(), selector, func(obj interface{}) {
			// it will not handle multiple results
			// TODO: check for nil
			splan := obj.(*spec.ServicePlan)
			if splan.Namespace == bns.GetBrokerNamespace() {
				planName = splan.Labels[clusterPlanPrefix]
			}
		})
		if planName == "" {
			glog.Infof("%s - broker service plan not found", logHeader)
		}
	}

	// Search for the cluster plan by its name
	spQ := &spec.ServicePlan{}
	spQ.SetName(planName)
	spQ.SetNamespace(systemNamespace)
	obj, spExists, err := c.spInf.GetStore().Get(spQ)
	if err != nil {
		return err
	}
	// The broker doesn't have a service plan, search for a default one in the cluster
	if !spExists {
		glog.Infof("%s - cluster service plan '%s' doesn't exists", logHeader, planName)
		glog.Infof("%s - searching for a default service plan in the cluster ...", logHeader)
		cache.ListAll(c.spInf.GetStore(), selector, func(obj interface{}) {
			// it will not handle multiple results
			// TODO: check for nil
			splan := obj.(*spec.ServicePlan)
			if splan.Namespace == systemNamespace {
				sp = splan
			}
		})
		if sp == nil {
			return fmt.Errorf("%s - couldn't find a default cluster plan", logHeader)
		}
	} else {
		sp = obj.(*spec.ServicePlan)
		glog.Infof("%s - found a cluster service plan: '%s'", logHeader, sp.Name)
	}

	if !psetExists {
		return app.CreatePetSet(sp)
	}
	return app.UpdatePetSet(nil, sp)
}

func (c *AddonController) addonForDeployment(p *v1alpha1.PetSet) *spec.Addon {
	key, err := keyFunc(p)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("creating key: %s", err))
		return nil
	}

	// Namespace/Name are one-to-one so the key will find the respective Addon resource.
	a, exists, err := c.addonInf.GetStore().GetByKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("get Addon resource: %s", err))
		return nil
	}
	if !exists {
		return nil
	}
	return a.(*spec.Addon)
}

// CreateAddonTPRs generates the third party resource required for interacting with addons
func CreateAddonTPRs(host string, kclient *kubernetes.Clientset) error {
	tprs := []*extensions.ThirdPartyResource{
		{
			ObjectMeta: v1.ObjectMeta{
				Name: tprAddons,
			},
			Versions: []extensions.APIVersion{
				{Name: "v1alpha1"},
			},
			Description: "Addon external service integration",
		},
	}
	tprClient := kclient.Extensions().ThirdPartyResources()
	for _, tpr := range tprs {
		if _, err := tprClient.Create(tpr); err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}
		glog.Infof("Third Party Resource '%s' provisioned", tpr.Name)
	}

	// We have to wait for the TPRs to be ready. Otherwise the initial watch may fail.
	return watch3PRs(host, "/apis/sys.koli.io/v1alpha1/addons", kclient)
}
