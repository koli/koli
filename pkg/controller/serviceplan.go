package controller

import (
	"fmt"
	"time"

	"github.com/golang/glog"
	"kolihub.io/koli/pkg/clientset"
	"kolihub.io/koli/pkg/platform"
	"kolihub.io/koli/pkg/spec"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/client/cache"
	"k8s.io/kubernetes/pkg/util/wait"

	apierrors "k8s.io/kubernetes/pkg/api/errors"
	extensions "k8s.io/kubernetes/pkg/apis/extensions"
	kclientset "k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset"
	utilruntime "k8s.io/kubernetes/pkg/util/runtime"
)

const (
	tprServicePlan       = "serviceplan.platform.koli.io"
	tprServicePlanStatus = "serviceplanstatus.platform.koli.io"
)

// ServicePlanController controller
type ServicePlanController struct {
	kclient   kclientset.Interface
	sysClient clientset.CoreInterface

	spInf cache.SharedIndexInformer

	queue *queue
}

// NewServicePlanController create a new ServicePlanController
func NewServicePlanController(spInf cache.SharedIndexInformer, client kclientset.Interface, sysClient clientset.CoreInterface) *ServicePlanController {
	spc := &ServicePlanController{
		kclient:   client,
		sysClient: sysClient,
		spInf:     spInf,
		queue:     newQueue(200),
	}

	spc.spInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    spc.addServicePlan,
		UpdateFunc: spc.updateServicePlan,
		DeleteFunc: spc.deleteServicePlan,
	})

	return spc
}

// Run the controller.
func (c *ServicePlanController) Run(workers int, stopc <-chan struct{}) {
	// don't let panics crash the process
	defer utilruntime.HandleCrash()
	// make sure the work queue is shutdown which will trigger workers to end
	defer c.queue.close()

	glog.Info("Starting ServicePlan controller...")

	if !cache.WaitForCacheSync(stopc, c.spInf.HasSynced) {
		return
	}

	for i := 0; i < workers; i++ {
		// runWorker will loop until "something bad" happens.
		// The .Until will then rekick the worker after one second
		go wait.Until(c.runWorker, time.Second, stopc)
	}

	// wait until we're told to stop
	<-stopc
	glog.Info("shutting down addon controller")
}

// runWorker runs a worker thread that just dequeues items, processes them, and marks them done.
// It enforces that the syncHandler is never invoked concurrently with the same key.
func (c *ServicePlanController) runWorker() {
	for {
		sp, ok := c.queue.pop(&spec.ServicePlan{})
		if !ok {
			return
		}
		if err := c.reconcile(sp.(*spec.ServicePlan)); err != nil {
			utilruntime.HandleError(err)
		}
	}
}

func (c *ServicePlanController) reconcile(sp *spec.ServicePlan) error {
	key, err := keyFunc(sp)
	if err != nil {
		return err
	}

	_, exists, err := c.spInf.GetStore().GetByKey(key)
	if err != nil {
		return err
	}

	logHeader := fmt.Sprintf("%s/%s", sp.Namespace, sp.Name)
	if sp.Namespace == platform.SystemNamespace {
		// TODO: rules for cluster service plans
		return nil
	}

	if !exists {
		glog.Infof("%s - removing status for '%s'", logHeader, key)
		// TODO: We should not rely on this behavior because is not reliable
		// the proper way to deal with this is garbage collecting orphan resources
		if err := c.sysClient.ServicePlanStatus(sp.Namespace).Delete(sp.Name, nil); err != nil {
			glog.Warningf("failed removing service plan status '%s': %s", key, err)
		}
		return nil
	}

	exists = true
	if _, err := c.sysClient.ServicePlanStatus(sp.Namespace).Get(sp.Name); err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed retrieving status for '%s': %s)", key, err)
		}
		exists = false
	}

	status := &spec.ServicePlanStatus{
		TypeMeta: unversioned.TypeMeta{
			Kind:       "Serviceplanstatus",
			APIVersion: spec.SchemeGroupVersion.String(),
		},
		ObjectMeta: api.ObjectMeta{
			Name: sp.Name,
		},
		Phase: spec.ServicePlanActive,
	}

	// the reference plan
	clusterPlan := sp.ObjectMeta.Labels["koli.io/clusterplan"]
	if c.planExists(clusterPlan) {
		label := spec.NewLabel().Add(map[string]string{
			"clusterplan": clusterPlan,
		})
		status.Labels = label.Set
	} else {
		// the cluster plan is referenced by a label,
		// change the status of the plan if isn't set
		status.Phase = spec.ServicePlanNotFound
	}

	if !exists {
		if _, err := c.sysClient.ServicePlanStatus(sp.Namespace).Create(status); err != nil {
			glog.Warningf("%s - failed generating status: %s", logHeader, err)
		}
		return nil
	}

	if _, err := c.sysClient.ServicePlanStatus(sp.Namespace).Update(status); err != nil {
		glog.Warningf("%s - failed updating status: %s", logHeader, err)
	}
	return nil
}

func (c *ServicePlanController) addServicePlan(sp interface{}) {
	splan := sp.(*spec.ServicePlan)
	glog.Infof("add-service-plan - %s/%s", splan.Namespace, splan.Name)
	c.queue.add(sp.(*spec.ServicePlan))
}

func (c *ServicePlanController) updateServicePlan(o, n interface{}) {
	old := o.(*spec.ServicePlan)
	new := n.(*spec.ServicePlan)

	if old.ResourceVersion != new.ResourceVersion {
		glog.Infof("%s/%s - update-serviceplan, found new resource, queueing ...", new.Namespace, new.Name)
	}

	c.queue.add(new)
}

func (c *ServicePlanController) deleteServicePlan(sp interface{}) {
	splan := sp.(*spec.ServicePlan)
	glog.Infof("delete-service-plan - %s/%s", splan.Namespace, splan.Name)
	c.queue.add(splan)
}

// Verify if the reference plan exists
func (c *ServicePlanController) planExists(planName string) bool {
	if planName == "" {
		return false
	}
	if _, err := c.sysClient.ServicePlan(platform.SystemNamespace).Get(planName); err != nil {
		glog.Warningf("failed listing cluster plan '%s': %s", planName, err)
		return false
	}
	return true
}

// CreateServicePlan3PRs generates the third party resource required for interacting with Service Plans
func CreateServicePlan3PRs(host string, kclient kclientset.Interface) error {
	tprs := []*extensions.ThirdPartyResource{
		{
			ObjectMeta: api.ObjectMeta{
				Name: tprServicePlan,
			},
			Versions: []extensions.APIVersion{
				{Name: "v1alpha1"},
			},
			Description: "Service Plan resource aggregation",
		},
	}
	tprClient := kclient.Extensions().ThirdPartyResources()
	for _, tpr := range tprs {
		if _, err := tprClient.Create(tpr); err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}
		glog.Infof("third party resource '%s' provisioned", tpr.Name)
	}

	// We have to wait for the TPRs to be ready. Otherwise the initial watch may fail.
	return watch3PRs(host, "/apis/platform.koli.io/v1alpha1/serviceplans", kclient)
}

// CreateServicePlanStatus3PRs generates the third party resource required for informing
// the status of a Service Plan
func CreateServicePlanStatus3PRs(host string, kclient kclientset.Interface) error {
	tprs := []*extensions.ThirdPartyResource{
		{
			ObjectMeta: api.ObjectMeta{
				Name: tprServicePlanStatus,
			},
			Versions: []extensions.APIVersion{
				{Name: "v1alpha1"},
			},
			Description: "Service Plan Status",
		},
	}
	tprClient := kclient.Extensions().ThirdPartyResources()
	for _, tpr := range tprs {
		if _, err := tprClient.Create(tpr); err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}
		glog.Infof("third party resource '%s' provisioned", tpr.Name)
	}

	// We have to wait for the TPRs to be ready. Otherwise the initial watch may fail.
	return watch3PRs(host, "/apis/platform.koli.io/v1alpha1/serviceplanstatus", kclient)
}
