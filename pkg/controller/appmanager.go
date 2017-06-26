package controller

import (
	"fmt"
	"time"

	"github.com/golang/glog"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"

	platform "kolihub.io/koli/pkg/apis/v1alpha1"
	draft "kolihub.io/koli/pkg/apis/v1alpha1/draft"
)

// AppManagerController controller
type AppManagerController struct {
	kclient kubernetes.Interface
	// sysClient clientset.CoreInterface

	dpInf   cache.SharedIndexInformer
	planInf cache.SharedIndexInformer

	queue    *TaskQueue
	recorder record.EventRecorder
}

// NewAppManagerController creates a ResourceAllocatorCtrl
func NewAppManagerController(dpInf, planInf cache.SharedIndexInformer, client kubernetes.Interface) *AppManagerController {
	c := &AppManagerController{
		kclient:  client,
		dpInf:    dpInf,
		planInf:  planInf,
		recorder: newRecorder(client, "app-manager-controller"),
	}
	c.queue = NewTaskQueue(c.syncHandler)

	c.dpInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addDeployment,
		UpdateFunc: c.updateDeployment,
		DeleteFunc: c.deleteDeployment,
	})

	return c
}

func (c *AppManagerController) addDeployment(d interface{}) {
	new := d.(*v1beta1.Deployment)
	glog.Infof("add-deployment(%d) - %s/%s", new.Status.ObservedGeneration, new.Namespace, new.Name)
	c.queue.Add(new)
}

func (c *AppManagerController) updateDeployment(o, n interface{}) {
	old := o.(*v1beta1.Deployment)
	new := n.(*v1beta1.Deployment)

	if old.ResourceVersion == new.ResourceVersion {
		return
	}
	c.queue.Add(new)
}

func (c *AppManagerController) deleteDeployment(o interface{}) {
	obj := o.(*v1beta1.Deployment)
	c.queue.Add(obj)
}

// enqueueForNamespace enqueues all Deployments object keys that belong to the given namespace.
func (c *AppManagerController) enqueueForNamespace(namespace string) {
	cache.ListAll(c.dpInf.GetStore(), labels.Everything(), func(obj interface{}) {
		d := obj.(*v1beta1.Deployment)
		if d.Namespace == namespace {
			c.queue.Add(d)
		}
	})
}

// Run the controller.
func (c *AppManagerController) Run(workers int, stopc <-chan struct{}) {
	// don't let panics crash the process
	defer utilruntime.HandleCrash()
	// make sure the work queue is shutdown which will trigger workers to end
	defer c.queue.shutdown()

	glog.Info("Starting App Manager Controller...")

	if !cache.WaitForCacheSync(stopc, c.dpInf.HasSynced, c.planInf.HasSynced) {
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
	glog.Info("Shutting down App Manager controller")
}

// TODO: validate if it's a platform resource - OK
// TODO: test with an empty plan - OK
// TODO: Error when creating PVC
// TODO: test with already exist error PVC

func (c *AppManagerController) syncHandler(key string) error {
	obj, exists, err := c.dpInf.GetStore().GetByKey(key)
	if err != nil {
		glog.Warningf("%s - failed retrieving object from store [%s]", key, err)
		return err
	}
	if !exists {
		glog.V(3).Infof("%s - the deployment doesn't exists", key)
		return nil
	}

	d := draft.NewDeployment(obj.(*v1beta1.Deployment))
	if d.DeletionTimestamp != nil {
		glog.V(3).Infof("%s - object marked for deletion")
		return nil
	}

	if !d.GetNamespaceMetadata().Valid() {
		glog.V(3).Infof("%s - it's not a valid resource")
		return nil
	}

	if len(d.Spec.Template.Spec.Containers) > 0 {
		if d.Spec.Template.Spec.Containers[0].Resources.Requests == nil ||
			d.Spec.Template.Spec.Containers[0].Resources.Limits == nil {
			glog.Warningf("%s - deployment has empty 'limits' or 'requests' resources", key)
		}
	}

	planName, exists := d.GetStoragePlan().Value()
	if !exists || !d.HasSetupPVCAnnotation() {
		glog.V(3).Infof("%s - the object doesn't have a storage plan or an annotation to setup a PVC")
		return nil
	}
	var plan *platform.Plan
	cache.ListAll(c.planInf.GetStore(), labels.Everything(), func(obj interface{}) {
		p := obj.(*platform.Plan)
		if p.Name == planName && p.IsStorageType() {
			plan = p
		}
	})
	if plan == nil {
		msg := fmt.Sprintf(`Storage Plan "%s" not found`, planName)
		c.recorder.Event(&d.Deployment, v1.EventTypeWarning, "PlanNotFound", msg)
		return fmt.Errorf(msg)
	}
	_, err = c.kclient.Core().PersistentVolumeClaims(d.Namespace).Create(newPVC(d, plan))
	if err != nil && !apierrors.IsAlreadyExists(err) {
		msg := fmt.Sprintf(`Failed creating PVC [%v]`, err)
		c.recorder.Event(d, v1.EventTypeWarning, "ProvisionError", msg)
		return fmt.Errorf(msg)
	}

	glog.Infof(`%s - PVC "d-%s" created with "%s"`, key, d.Name, plan.Spec.Storage)
	patchData := []byte(fmt.Sprintf(`{"metadata": {"annotations": {"%s": "false"}}}`, platform.AnnotationSetupStorage))
	_, err = c.kclient.Extensions().Deployments(d.Namespace).Patch(d.Name, types.MergePatchType, patchData)
	if err != nil {
		return fmt.Errorf(`%s - failed updating deployment [%v]`, key, err)
	}
	return nil
}

func newPVC(d *draft.Deployment, plan *platform.Plan) *v1.PersistentVolumeClaim {
	return &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("d-%s", d.Name),
			Namespace: d.Namespace,
		},
		Spec: v1.PersistentVolumeClaimSpec{
			AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
			Resources: v1.ResourceRequirements{
				Requests: v1.ResourceList{v1.ResourceStorage: plan.Spec.Storage},
			},
		},
	}
}
