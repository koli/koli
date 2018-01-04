package controller

import (
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/golang/glog"
	platform "kolihub.io/koli/pkg/apis/core/v1alpha1"
	"kolihub.io/koli/pkg/clientset"
	"kolihub.io/koli/pkg/spec"
	"kolihub.io/koli/pkg/spec/util"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"

	extensions "k8s.io/api/extensions/v1beta1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

// ResourceAllocatorCtrl controller
type ResourceAllocatorCtrl struct {
	kclient   kubernetes.Interface
	sysClient clientset.CoreInterface

	dpInf cache.SharedIndexInformer
	spInf cache.SharedIndexInformer

	queue    *TaskQueue
	recorder record.EventRecorder
}

// NewResourceAllocatorCtrl creates a ResourceAllocatorCtrl
func NewResourceAllocatorCtrl(dpInf, spInf cache.SharedIndexInformer,
	client kubernetes.Interface,
	sysClient clientset.CoreInterface) *ResourceAllocatorCtrl {

	c := &ResourceAllocatorCtrl{
		kclient:   client,
		sysClient: sysClient,
		dpInf:     dpInf,
		spInf:     spInf,
		recorder:  newRecorder(client, "allocator-controller"),
	}
	c.queue = NewTaskQueue("resource_alloc", c.syncHandler)
	c.dpInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addDeployment,
		UpdateFunc: c.updateDeployment,
	})

	c.spInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		// No-op function
		// AddFunc: func(obj interface{}) {
		// 	sp := obj.(*spec.Plan)
		// 	glog.Infof("add-service-plan(%s) - %s/%s", sp.ResourceVersion, sp.Namespace, sp.Name)
		// },
		// No-op function
		// DeleteFunc: func(obj interface{}) {
		// 	sp := obj.(*spec.Plan)
		// 	glog.Infof("delete-service-plan: %s/%s", sp.Namespace, sp.Name)
		// },
		UpdateFunc: c.updateServicePlan,
	})
	return c
}

func (c *ResourceAllocatorCtrl) addDeployment(d interface{}) {
	new := d.(*extensions.Deployment)
	glog.Infof("add-deployment(%d) - %s/%s", new.Status.ObservedGeneration, new.Namespace, new.Name)
	c.queue.Add(new)
}

func (c *ResourceAllocatorCtrl) updateDeployment(o, n interface{}) {
	old := o.(*extensions.Deployment)
	new := n.(*extensions.Deployment)

	// TODO: skip a list of namespaces
	if new.Namespace == platform.SystemNamespace || old.ResourceVersion == new.ResourceVersion {
		return
	}

	statusGen := new.Status.ObservedGeneration
	if old.Labels[spec.KoliPrefix("clusterplan")] != new.Labels[spec.KoliPrefix("clusterplan")] {
		// clusterplan label is required in all deployments
		glog.V(2).Infof("update-deployment(%d) - %s/%s - enforce label 'clusterplan', queueing ...", statusGen, new.Namespace, new.Name)
		c.queue.Add(new)
		return
	}
	// updating a deployment triggers this function serveral times.
	// a deployment must be queued only when every generation status is synchronized -
	// when the generation and ObservedGeneration are equal for each resource object (new and old)
	if old.Generation == new.Generation && old.Status.ObservedGeneration == statusGen {
		glog.V(2).Infof("update-deployment(%d) - %s/%s - resource on sync, queueing ...", statusGen, new.Namespace, new.Name)
		c.queue.Add(new)
	}
}

func (c *ResourceAllocatorCtrl) updateServicePlan(o, n interface{}) {
	old := o.(*spec.Plan)
	new := n.(*spec.Plan)

	if old.ResourceVersion == new.ResourceVersion {
		return
	}

	// When a user associates a Service Plan to a new one
	if !reflect.DeepEqual(old.Labels, new.Labels) && new.Namespace != platform.SystemNamespace {
		glog.V(2).Infof("update-plan(%d) - %s/%s", new.ResourceVersion, new.Namespace, new.Name)
		c.enqueueForNamespace(new.Namespace)
	}
}

// enqueueForNamespace enqueues all Deployments object keys that belong to the given namespace.
func (c *ResourceAllocatorCtrl) enqueueForNamespace(namespace string) {
	cache.ListAll(c.dpInf.GetStore(), labels.Everything(), func(obj interface{}) {
		d := obj.(*extensions.Deployment)
		if d.Namespace == namespace {
			c.queue.Add(d)
		}
	})
}

// Run the controller.
func (c *ResourceAllocatorCtrl) Run(workers int, stopc <-chan struct{}) {
	// don't let panics crash the process
	defer utilruntime.HandleCrash()
	// make sure the work queue is shutdown which will trigger workers to end
	defer c.queue.shutdown()

	glog.Info("Starting Resource Allocator controller...")

	if !cache.WaitForCacheSync(stopc, c.dpInf.HasSynced, c.spInf.HasSynced) {
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
	glog.Info("Shutting down Resource Allocator controller")
}

func (c *ResourceAllocatorCtrl) syncHandler(key string) error {
	obj, exists, err := c.dpInf.GetStore().GetByKey(key)
	if err != nil {
		glog.Warningf("%s - failed retrieving object from store [%s]", key, err)
		return err
	}

	if !exists {
		glog.V(2).Infof("%s - the deployment doesn't exists")
		return nil
	}

	d := obj.(*extensions.Deployment)
	pns, err := platform.NewNamespace(d.Namespace)
	if err != nil {
		glog.V(2).Infof("%s - noop, it's not a valid namespace", key)
		return nil
	}

	var sp *spec.Plan
	selector := spec.NewLabel().Add(map[string]string{"default": "true"}).AsSelector()
	clusterPlanPrefix := spec.KoliPrefix("clusterplan")
	planName := d.Labels[clusterPlanPrefix]
	if planName == "" {
		glog.V(2).Infof("%s - label '%s' is empty", key, clusterPlanPrefix)
		cache.ListAll(c.spInf.GetStore(), selector, func(obj interface{}) {
			// it will not handle multiple results
			// TODO: check for nil
			splan := obj.(*spec.Plan)
			if splan.Namespace == pns.GetSystemNamespace() {
				planName = splan.Labels[clusterPlanPrefix]
			}
		})
		if planName == "" {
			glog.V(2).Infof("%s - broker service plan not found", key)
		}
	}

	// Search for the cluster plan by its name
	spQ := &spec.Plan{}
	spQ.SetName(planName)
	spQ.SetNamespace(platform.SystemNamespace)
	obj, exists, err = c.spInf.GetStore().Get(spQ)
	if err != nil {
		return err
	}
	// The broker doesn't have a service plan, search for a default one in the cluster
	if !exists {
		glog.V(2).Infof("%s - broker service plan '%s' doesn't exists, searching for a default one ...", key, planName)
		cache.ListAll(c.spInf.GetStore(), selector, func(obj interface{}) {
			// it will not handle multiple results
			// TODO: check for nil
			splan := obj.(*spec.Plan)
			if splan.Namespace == platform.SystemNamespace {
				sp = splan
			}
		})
		if sp == nil {
			// TODO: set a default ?
			return errors.New("couldn't find a default cluster plan")
		}
	} else {
		sp = obj.(*spec.Plan)
		glog.V(2).Infof("%s - found cluster service plan '%s'", key, sp.Name)
	}

	// Deep-copy otherwise we're mutating our cache.
	// TODO: Deep-copy only when needed.
	newD, err := util.DeploymentDeepCopy(d)
	if err != nil {
		return fmt.Errorf("failed copying deployment [%s]", err)
	}

	containers := newD.Spec.Template.Spec.Containers
	if err := c.validateContainers(d); err != nil {
		return err
	}

	if !reflect.DeepEqual(containers[0].Resources, sp.Spec.Resources) {
		// Enforce allocation because the resources doesn't match
		glog.V(2).Infof("%s - enforcing allocation with Plan '%s'", key, sp.Name)
		containers[0].Resources.Requests = sp.Spec.Resources.Requests
		containers[0].Resources.Limits = sp.Spec.Resources.Limits
		newD.Labels["kolihub.io/clusterplan"] = sp.Name
		if _, err := c.kclient.Extensions().Deployments(d.Namespace).Update(newD); err != nil {
			return fmt.Errorf("failed updating deployment compute resources [%s]", err)
		}
		return nil
	}

	// the resource match, update the reference plan (label)
	if d.Labels[clusterPlanPrefix] != sp.Name {
		newD.Labels[clusterPlanPrefix] = sp.Name
		glog.Infof("%s - enforcing clusterplan label '%s'", key, sp.Name)
		if _, err := c.kclient.Extensions().Deployments(d.Namespace).Update(newD); err != nil {
			return fmt.Errorf("failed updating deployment labels [%s]", err)
		}
		c.recorder.Eventf(d, v1.EventTypeNormal, "ResourceAllocation", "Successfully allocated plan '%s'", sp.Name)
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
