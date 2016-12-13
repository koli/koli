package controller

import (
	"fmt"
	"reflect"
	"time"

	"github.com/golang/glog"
	"github.com/kolibox/koli/pkg/clientset"
	"github.com/kolibox/koli/pkg/spec"
	"github.com/kolibox/koli/pkg/util"

	"k8s.io/client-go/1.5/kubernetes"
	extensions "k8s.io/client-go/1.5/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/1.5/pkg/labels"
	utilruntime "k8s.io/client-go/1.5/pkg/util/runtime"
	"k8s.io/client-go/1.5/pkg/util/wait"
	"k8s.io/client-go/1.5/tools/cache"
)

// ResourceAllocatorCtrl controller
type ResourceAllocatorCtrl struct {
	kclient   *kubernetes.Clientset
	sysClient clientset.CoreInterface

	dpInf cache.SharedIndexInformer
	spInf cache.SharedIndexInformer

	queue *queue
}

// NewResourceAllocatorCtrl creates a ResourceAllocatorCtrl
func NewResourceAllocatorCtrl(dpInf, spInf cache.SharedIndexInformer,
	client *kubernetes.Clientset,
	sysClient clientset.CoreInterface) *ResourceAllocatorCtrl {

	c := &ResourceAllocatorCtrl{
		kclient:   client,
		sysClient: sysClient,
		dpInf:     dpInf,
		spInf:     spInf,
		queue:     newQueue(200),
	}
	c.dpInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addDeployment,
		UpdateFunc: c.updateDeployment,
	})

	c.spInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addServicePlan,
		UpdateFunc: c.updateServicePlan,
		DeleteFunc: c.deleteServicePlan,
	})
	return c
}

func (c *ResourceAllocatorCtrl) addDeployment(d interface{}) {
	new := d.(*extensions.Deployment)
	glog.Infof("add-deployment(%d) - %s/%s", new.Status.ObservedGeneration, new.Namespace, new.Name)
	c.queue.add(new)
}

func (c *ResourceAllocatorCtrl) updateDeployment(o, n interface{}) {
	old := o.(*extensions.Deployment)
	new := n.(*extensions.Deployment)

	// TODO: skip a list of namespaces
	if new.Namespace == systemNamespace || old.ResourceVersion == new.ResourceVersion {
		return
	}

	statusGen := new.Status.ObservedGeneration
	if old.Labels[spec.KoliPrefix("clusterplan")] != new.Labels[spec.KoliPrefix("clusterplan")] {
		// clusterplan label is required in all deployments
		glog.Infof("update-deployment(%d) - %s/%s - enforce label 'clusterplan', queueing ...", statusGen, new.Namespace, new.Name)
		c.queue.add(new)
		return
	}
	// msg := "update-deployment meta-gen(%d/%d) obs-gen(%d/%d) - %s/%s"
	// glog.Infof(msg, old.Generation, new.Generation, old.Status.ObservedGeneration, new.Status.ObservedGeneration, new.Namespace, new.Name)

	// updating a deployment triggers this function serveral times.
	// a deployment must be queued only when every generation status is synchronized -
	// when the generation and ObservedGeneration are equal for each resource object (new and old)
	if old.Generation == new.Generation && old.Status.ObservedGeneration == statusGen {
		glog.Infof("update-deployment(%d) - %s/%s - resource on sync, queueing ...", statusGen, new.Namespace, new.Name)
		c.queue.add(new)
	}
}

func (c *ResourceAllocatorCtrl) addServicePlan(s interface{}) {
	sp := s.(*spec.ServicePlan)
	glog.Infof("add-service-plan: %s/%s", sp.Namespace, sp.Name)
	// TODO: ensure that exists only one default service plan for the namespace
	if sp.Labels["koli.io/default"] == "true" && sp.Namespace != systemNamespace {
		// TODO: enqueue all deployments because the user changed the
		// default service plan on his scope.
		c.enqueueForNamespace(sp.Namespace)
	}
}

func (c *ResourceAllocatorCtrl) updateServicePlan(o, n interface{}) {
	old := o.(*spec.ServicePlan)
	new := n.(*spec.ServicePlan)

	if old.ResourceVersion == new.ResourceVersion {
		return
	}

	// When a user associates a Service Plan to a new one
	if !reflect.DeepEqual(old.Labels, new.Labels) && new.Namespace != systemNamespace {
		c.enqueueForNamespace(new.Namespace)
	}
}

func (c *ResourceAllocatorCtrl) deleteServicePlan(s interface{}) {
	sp := s.(*spec.ServicePlan)
	glog.Infof("delete-service-plan: %s/%s", sp.Namespace, sp.Name)

	if sp.Namespace != systemNamespace {
		c.enqueueForNamespace(sp.Namespace)
	}
}

// enqueueForNamespace enqueues all Deployments object keys that belong to the given namespace.
func (c *ResourceAllocatorCtrl) enqueueForNamespace(namespace string) {
	cache.ListAll(c.dpInf.GetStore(), labels.Everything(), func(obj interface{}) {
		d := obj.(*extensions.Deployment)
		if d.Namespace == namespace {
			c.queue.add(d)
		}
	})
}

// Run the controller.
func (c *ResourceAllocatorCtrl) Run(workers int, stopc <-chan struct{}) {
	// don't let panics crash the process
	defer utilruntime.HandleCrash()
	// make sure the work queue is shutdown which will trigger workers to end
	defer c.queue.close()

	glog.Info("Starting Resource Allocator controller...")

	if !cache.WaitForCacheSync(stopc, c.dpInf.HasSynced, c.spInf.HasSynced) {
		return
	}

	for i := 0; i < workers; i++ {
		// runWorker will loop until "something bad" happens.
		// The .Until will then rekick the worker after one second
		go wait.Until(c.runWorker, time.Second, stopc)
	}

	// wait until we're told to stop
	<-stopc
	glog.Info("shutting down Resource Allocator controller")
}

// var keyFunc = cache.DeletionHandlingMetaNamespaceKeyFunc

// runWorker runs a worker thread that just dequeues items, processes them, and marks them done.
// It enforces that the syncHandler is never invoked concurrently with the same key.
func (c *ResourceAllocatorCtrl) runWorker() {
	for {
		obj, ok := c.queue.pop(&extensions.Deployment{})
		if !ok {
			return
		}
		if err := c.reconcile(obj.(*extensions.Deployment)); err != nil {
			utilruntime.HandleError(err)
		}
	}
}

func (c *ResourceAllocatorCtrl) reconcile(d *extensions.Deployment) error {
	key, err := keyFunc(d)
	if err != nil {
		return err
	}
	_, exists, err := c.dpInf.GetStore().GetByKey(key)
	if err != nil {
		return err
	}
	if !exists {
		// don't do nothing because the deployment doesn't exists
		return nil
	}

	logHeader := fmt.Sprintf("%s/%s(%d)", d.Namespace, d.Name, d.Status.ObservedGeneration)
	if d.DeletionTimestamp != nil {
		glog.Infof("%s - marked for deletion, skipping ...", logHeader)
		return nil
	}

	var sp *spec.ServicePlan
	selector := spec.NewLabel().Add(map[string]string{"default": "true"}).AsSelector()
	clusterPlanPrefix := spec.KoliPrefix("clusterplan")
	planName := d.Labels[clusterPlanPrefix]
	if planName == "" {
		glog.Infof("%s - label '%s' is empty", logHeader, clusterPlanPrefix)
		cache.ListAll(c.spInf.GetStore(), selector, func(obj interface{}) {
			// it will not handle multiple results
			// TODO: check for nil
			splan := obj.(*spec.ServicePlan)
			if splan.Namespace == d.Namespace {
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
	obj, exists, err := c.spInf.GetStore().Get(spQ)
	if err != nil {
		return err
	}
	// The broker doesn't have a service plan, search for a default one in the cluster
	if !exists {
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
		glog.Infof("%s - found cluster service plan '%s'", logHeader, sp.Name)
	}

	// Deep-copy otherwise we're mutating our cache.
	// TODO: Deep-copy only when needed.
	newD, err := util.DeploymentDeepCopy(d)
	if err != nil {
		return fmt.Errorf("%s - failed copying deployment", logHeader)
	}

	containers := newD.Spec.Template.Spec.Containers
	if err := c.validateContainers(d); err != nil {
		return err
	}

	klabel := spec.NewLabel().Add(map[string]string{"clusterplan": planName})
	if !reflect.DeepEqual(containers[0].Resources, sp.Spec.Resources) {
		// TODO: Enforce allocation because the resources doesn't match
		glog.Infof("%s - enforcing allocation with SP '%s'", logHeader, sp.Name)
		containers[0].Resources.Requests = sp.Spec.Resources.Requests
		containers[0].Resources.Limits = sp.Spec.Resources.Limits
		newD.Labels = klabel.Set
		if _, err := c.kclient.Extensions().Deployments(d.Namespace).Update(newD); err != nil {
			return fmt.Errorf("%s - failed updating deployment compute resources: %s", logHeader, err)
		}
		return nil
	}

	// the resource match, update the reference plan (label)
	if d.Labels[clusterPlanPrefix] != sp.Name {
		newD.Labels[clusterPlanPrefix] = sp.Name
		glog.Infof("%s - enforcing clusterplan label '%s'", logHeader, sp.Name)
		if _, err := c.kclient.Extensions().Deployments(d.Namespace).Update(newD); err != nil {
			return fmt.Errorf("%s - failed updating deployment labels: %s", logHeader, err)
		}
	}
	return nil
}

func (c *ResourceAllocatorCtrl) validateContainers(d *extensions.Deployment) error {
	containersLength := len(d.Spec.Template.Spec.Containers)
	switch {
	case containersLength < 1:
		return fmt.Errorf("%s/%s - cannot enforce allocation, deployment doesn't have containers", d.Namespace, d.Name)
	case containersLength > 1:
		glog.Warningf("%s/%s - found more than one container", d.Namespace, d.Name)
	}
	return nil
}

// func (c *ResourceAllocatorCtrl) servicePlanForDeployment(d *extensions.Deployment) (*spec.ServicePlan, error) {
// 	spQ := &spec.ServicePlan{}
// 	spQ.Name = d.Name
// 	spQ.Namespace = d.Namespace

// 	obj, exists, err := c.spInf.GetStore().Get(spQ)
// 	if !exists {
// 		lbl := spec.NewLabel().Add(map[string]string{"default": "true"})
// 		// servicePlans := []*spec.ServicePlan{}
// 		if err := cache.ListAllByNamespace(c.spInf.GetIndexer(), d.Namespace, lbl.AsSelector(), func(obj interface{}) {
// 			// sp := obj.(*spec.ServicePlan)
// 			// append(servicePlans, sp)
// 			// fmt.Println(servicePlans)
// 		}); err != nil {
// 			return nil, fmt.Errorf("failed retrieving service plan: %s", err)
// 		}
// 	}
// 	return nil, nil

// }
