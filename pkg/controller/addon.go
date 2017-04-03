package controller

import (
	"fmt"
	"time"

	"github.com/golang/glog"
	"kolihub.io/koli/pkg/platform"
	"kolihub.io/koli/pkg/spec"
	koliapps "kolihub.io/koli/pkg/spec/apps"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/pkg/apis/apps"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	v1beta1 "k8s.io/client-go/pkg/apis/apps/v1beta1"
)

// AddonController controller
type AddonController struct {
	kclient kubernetes.Interface

	addonInf cache.SharedIndexInformer
	spInf    cache.SharedIndexInformer
	psetInf  cache.SharedIndexInformer

	queue    *TaskQueue
	recorder record.EventRecorder
}

// NewAddonController creates a new addon controller
func NewAddonController(addonInformer, psetInformer, spInformer cache.SharedIndexInformer, client kubernetes.Interface) *AddonController {
	ac := &AddonController{
		kclient:  client,
		addonInf: addonInformer,
		psetInf:  psetInformer,
		spInf:    spInformer,
		recorder: newRecorder(client, "apps-controller"),
	}
	ac.queue = NewTaskQueue(ac.syncHandler)

	ac.addonInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    ac.addAddon,
		UpdateFunc: ac.updateAddon,
		DeleteFunc: ac.deleteAddon,
	})

	ac.psetInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: ac.updateStatefulSet,
		DeleteFunc: ac.deleteStatefulSet,
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
	defer c.queue.shutdown()

	glog.Info("Starting Addon Controller...")

	if !cache.WaitForCacheSync(stopc, c.addonInf.HasSynced, c.psetInf.HasSynced, c.spInf.HasSynced) {
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
	glog.Info("shutting down Addon Controller")
}

// enqueueForNamespace enqueues all Deployments object keys that belong to the given namespace.
func (c *AddonController) enqueueForNamespace(namespace string) {
	cache.ListAll(c.psetInf.GetStore(), labels.Everything(), func(obj interface{}) {
		d := obj.(*v1beta1.StatefulSet)
		if d.Namespace == namespace {
			c.queue.Add(d)
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

func (c *AddonController) updateStatefulSet(o, n interface{}) {
	old := o.(*v1beta1.StatefulSet)
	new := n.(*v1beta1.StatefulSet)
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

func (c *AddonController) deleteStatefulSet(a interface{}) {
	d := a.(*v1beta1.StatefulSet)
	glog.Infof("deleteDeployment: (%s/%s)", d.Namespace, d.Name)
	if addon := c.addonForDeployment(d); addon != nil {
		c.enqueueAddon(addon)
	}
}

func (c *AddonController) enqueueAddon(addon *spec.Addon) {
	c.queue.Add(addon)
}

// Not implemented yet
func (c *AddonController) syncHandler(key string) error {
	return nil
}

func (c *AddonController) reconcile(app koliapps.AddonInterface) error {
	key, err := keyFunc(app.GetAddon())
	if err != nil {
		return err
	}

	addon := app.GetAddon()
	logHeader := fmt.Sprintf("%s/%s(%s)", addon.Namespace, addon.Name, addon.ResourceVersion)
	pns, err := platform.NewNamespace(addon.Namespace)
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
	svc := koliapps.MakePetSetService(addon)
	if _, err := c.kclient.Core().Services(addon.Namespace).Create(svc); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed creating service (%s)", err)
	}

	// Ensure we have a replica set running
	psetQ := &v1beta1.StatefulSet{}
	psetQ.Namespace = addon.Namespace
	psetQ.Name = addon.Name

	obj, psetExists, err := c.psetInf.GetStore().Get(psetQ)
	if err != nil {
		return err
	}

	planName := ""
	var sp *spec.Plan
	selector := spec.NewLabel().Add(map[string]string{"default": "true"}).AsSelector()
	clusterPlanPrefix := spec.KoliPrefix("clusterplan")
	if psetExists {
		pset := obj.(*apps.StatefulSet)
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
			splan := obj.(*spec.Plan)
			if splan.Namespace == pns.GetSystemNamespace() {
				planName = splan.Labels[clusterPlanPrefix]
			}
		})
		if planName == "" {
			glog.Infof("%s - broker service plan not found", logHeader)
		}
	}

	// Search for the cluster plan by its name
	spQ := &spec.Plan{}
	spQ.SetName(planName)
	spQ.SetNamespace(platform.SystemNamespace)
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
			splan := obj.(*spec.Plan)
			if splan.Namespace == platform.SystemNamespace {
				sp = splan
			}
		})
		if sp == nil {
			return fmt.Errorf("%s - couldn't find a default cluster plan", logHeader)
		}
	} else {
		sp = obj.(*spec.Plan)
		glog.Infof("%s - found a cluster service plan: '%s'", logHeader, sp.Name)
	}

	if !psetExists {
		return app.CreatePetSet(sp)
	}
	return app.UpdatePetSet(nil, sp)
}

func (c *AddonController) addonForDeployment(p *v1beta1.StatefulSet) *spec.Addon {
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
