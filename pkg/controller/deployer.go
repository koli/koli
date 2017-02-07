package controller

import (
	"fmt"
	"time"

	"github.com/golang/glog"
	clientset "kolihub.io/koli/pkg/clientset"
	"kolihub.io/koli/pkg/platform"
	"kolihub.io/koli/pkg/spec"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/cache"
	"k8s.io/kubernetes/pkg/util/wait"

	kclientset "k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset"
	utilruntime "k8s.io/kubernetes/pkg/util/runtime"
)

// DeployerController controller
type DeployerController struct {
	kclient   kclientset.Interface
	clientset clientset.CoreInterface
	podInf    cache.SharedIndexInformer
	dpInf     cache.SharedIndexInformer
	queue     *queue
}

// NewDeployerController creates a new DeployerController
func NewDeployerController(podInf, dpInf cache.SharedIndexInformer, sysClient clientset.CoreInterface, kclient kclientset.Interface) *DeployerController {
	d := &DeployerController{
		kclient:   kclient,
		clientset: sysClient,
		podInf:    podInf,
		dpInf:     dpInf,
		queue:     newQueue(200),
	}

	d.podInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    d.addPod,
		UpdateFunc: d.updatePod,
		DeleteFunc: d.deletePod,
	})
	return d
}

func (d *DeployerController) addPod(obj interface{}) {
	pod := obj.(*api.Pod)
	glog.Infof("add-pod - %s/%s", pod.Namespace, pod.Name)
	d.queue.add(pod)
}

func (d *DeployerController) updatePod(o, n interface{}) {
	old := o.(*api.Pod)
	new := n.(*api.Pod)
	if old.ResourceVersion == new.ResourceVersion {
		return
	}
	glog.Infof("update-pod - %s/%s", new.Namespace, new.Name)
	d.queue.add(new)
}

func (d *DeployerController) deletePod(obj interface{}) {
	pod := obj.(*api.Pod)
	glog.Infof("delete-pod - %s/%s", pod.Namespace, pod.Name)
	d.queue.add(pod)
}

// Run the controller.
func (d *DeployerController) Run(workers int, stopc <-chan struct{}) {
	// don't let panics crash the process
	defer utilruntime.HandleCrash()
	// make sure the work queue is shutdown which will trigger workers to end
	defer d.queue.close()

	glog.Info("Starting Deployer Controller...")

	if !cache.WaitForCacheSync(stopc, d.podInf.HasSynced, d.dpInf.HasSynced) {
		return
	}

	for i := 0; i < workers; i++ {
		// runWorker will loop until "something bad" happens.
		// The .Until will then rekick the worker after one second
		go wait.Until(d.runWorker, time.Second, stopc)
	}

	// wait until we're told to stop
	<-stopc
	glog.Info("shutting down Deployer controller...")
}

// runWorker runs a worker thread that just dequeues items, processes them, and marks them done.
// It enforces that the syncHandler is never invoked concurrently with the same key.
func (d *DeployerController) runWorker() {
	for {
		pod, ok := d.queue.pop(&api.Pod{})
		if !ok {
			return
		}

		if err := d.reconcile(pod.(*api.Pod)); err != nil {
			utilruntime.HandleError(err)
		}
	}
}

func (d *DeployerController) reconcile(pod *api.Pod) error {
	key, err := keyFunc(pod)
	if err != nil {
		return err
	}

	logHeader := fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)
	_, err = platform.NewNamespace(pod.Namespace)
	if err != nil {
		glog.Infof("%s - noop, it's not a valid namespace", logHeader)
		return nil
	}

	_, exists, err := d.podInf.GetStore().GetByKey(key)
	if err != nil {
		return err
	}

	if !exists || pod.DeletionTimestamp != nil {
		return nil
	}
	releaseName := pod.Annotations[spec.KoliPrefix("releasename")]
	if releaseName == "" {
		return fmt.Errorf("%s - missing 'releasename' annotation on pod", logHeader)
	}
	if pod.Status.Phase == api.PodSucceeded {
		glog.Infof("%s - build completed successfully")
		if pod.Labels[spec.KoliPrefix("deployrelease")] == "true" {
			// deploy it
		}
	}

	// update the status
	payload := `{"metadata": {"annotations": {"%s": "false"}, "labels": {"%s": "%s"}}}`
	payload = fmt.Sprintf(payload, spec.KoliPrefix("build"), spec.KoliPrefix("build-status"), pod.Status.Phase)
	_, err = d.clientset.Release(pod.Namespace).Patch(releaseName, api.StrategicMergePatchType, []byte(payload))
	if err != nil {
		return fmt.Errorf("%s - failed updating pod status: %s", logHeader, err)
	}
	return nil
}
