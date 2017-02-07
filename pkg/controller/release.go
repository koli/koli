package controller

import (
	"fmt"
	"time"

	"github.com/golang/glog"

	clientset "kolihub.io/koli/pkg/clientset"
	"kolihub.io/koli/pkg/platform"
	"kolihub.io/koli/pkg/spec"
	koliutil "kolihub.io/koli/pkg/util"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/client/cache"
	"k8s.io/kubernetes/pkg/labels"
	"k8s.io/kubernetes/pkg/util/wait"

	extensions "k8s.io/kubernetes/pkg/apis/extensions"
	kclientset "k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset"
	utilruntime "k8s.io/kubernetes/pkg/util/runtime"
)

// keys required in a deployment annotation for creating a new release
var requiredKeys = []string{"gitremote", "gitrepository"}

// ReleaseController controller
type ReleaseController struct {
	kclient    kclientset.Interface
	clientset  clientset.CoreInterface
	releaseInf cache.SharedIndexInformer
	dpInf      cache.SharedIndexInformer
	queue      *queue
}

// NewReleaseController creates a new ReleaseController
func NewReleaseController(releaseInf, dpInf cache.SharedIndexInformer, sysClient clientset.CoreInterface, kclient kclientset.Interface) *ReleaseController {
	r := &ReleaseController{
		kclient:    kclient,
		clientset:  sysClient,
		releaseInf: releaseInf,
		dpInf:      dpInf,
		queue:      newQueue(200),
	}

	r.dpInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    r.addDeployment,
		UpdateFunc: r.updateDeployment,
	})
	return r
}

func (r *ReleaseController) addDeployment(obj interface{}) {
	dp := obj.(*extensions.Deployment)
	glog.Infof("add-deployment - %s/%s", dp.Namespace, dp.Name)
	r.queue.add(dp)
}

func (r *ReleaseController) updateDeployment(o, n interface{}) {
	old := o.(*extensions.Deployment)
	new := n.(*extensions.Deployment)
	if old.ResourceVersion == new.ResourceVersion {
		return
	}

	statusGen := new.Status.ObservedGeneration
	// ref: https://github.com/kubernetes/kubernetes/issues/34363#issuecomment-263336109
	// observedGeneration is intended as a way for an observer to determine how up to date the status reported by the primary controller
	// for the resource is. No more, no less. That needs to be combined with the usual resourceVersion-based optimistic concurrency
	// mechanism to ensure that controllers don't act upon stale data, and with leader-election sequence numbers in the case of HA.
	if new.Spec.Paused && old.Generation != new.Generation {
		glog.Infof("update-deployment(%d) - %s/%s - paused resource, queueing ...", statusGen, new.Namespace, new.Name)
		r.queue.add(new)
	}
	// updating a deployment triggers this function serveral times.
	// a deployment must be queued only when every generation status is synchronized -
	// when the generation and ObservedGeneration are equal for each resource object (new and old)
	if old.Generation == new.Generation && old.Status.ObservedGeneration == statusGen {
		glog.Infof("update-deployment(%d) - %s/%s - resource on sync, queueing ...", statusGen, new.Namespace, new.Name)
		r.queue.add(new)
	}
}

// Run the controller.
func (r *ReleaseController) Run(workers int, stopc <-chan struct{}) {
	// don't let panics crash the process
	defer utilruntime.HandleCrash()
	// make sure the work queue is shutdown which will trigger workers to end
	defer r.queue.close()

	glog.Info("Starting Release Controller...")

	if !cache.WaitForCacheSync(stopc, r.releaseInf.HasSynced, r.dpInf.HasSynced) {
		return
	}

	for i := 0; i < workers; i++ {
		// runWorker will loop until "something bad" happens.
		// The .Until will then rekick the worker after one second
		go wait.Until(r.runWorker, time.Second, stopc)
	}

	// wait until we're told to stop
	<-stopc
	glog.Info("shutting down Release controller...")
}

// runWorker runs a worker thread that just dequeues items, processes them, and marks them done.
// It enforces that the syncHandler is never invoked concurrently with the same key.
func (r *ReleaseController) runWorker() {
	for {
		dp, ok := r.queue.pop(&extensions.Deployment{})
		if !ok {
			return
		}

		if err := r.reconcile(dp.(*extensions.Deployment)); err != nil {
			utilruntime.HandleError(err)
		}
	}
}

func (r *ReleaseController) reconcile(dp *extensions.Deployment) error {
	key, err := keyFunc(dp)
	if err != nil {
		return err
	}

	logHeader := fmt.Sprintf("%s/%s", dp.Namespace, dp.Name)
	_, err = platform.NewNamespace(dp.Namespace)
	if err != nil {
		glog.Infof("%s - noop, it's not a valid namespace", logHeader)
		return nil
	}

	_, exists, err := r.dpInf.GetStore().GetByKey(key)
	if err != nil {
		return err
	}

	if !exists || dp.DeletionTimestamp != nil {
		// TODO: delete from the remote object store (minio/s3/gcs...)
		glog.Infof("%s - deployment doesn't exists or was marked for deletion, skipping ...", logHeader)
		return nil
	}

	if dp.Annotations[spec.KoliPrefix("build")] != "true" {
		glog.Infof("%s - noop, isn't a build action", logHeader)
		return nil
	}
	// check if there's is a specific release for building it
	releaseTarget := dp.Annotations[spec.KoliPrefix("buildrelease")]
	if releaseTarget != "" {
		releaseExists := false
		cache.ListAll(r.releaseInf.GetStore(), labels.Everything(), func(obj interface{}) {
			// TODO: check for nil
			rel := obj.(*spec.Release)
			if rel.Namespace == dp.Namespace && rel.Name == releaseTarget {
				releaseExists = true
			}
		})
		// The releases exists, trigger a build on it
		if releaseExists {
			glog.Infof("%s - activating the build for release '%s'", logHeader, releaseTarget)
			activateBuild := activateBuildPayload(true)
			_, err := r.clientset.Release(dp.Namespace).Patch(releaseTarget, api.StrategicMergePatchType, activateBuild)
			if err == nil {
				// We need to update the 'build' key to false, otherwise the build will be triggered again.
				// TODO: Need other strategy for dealing with this kind of scenario
				deactivateBuild := activateBuildPayload(false)
				_, err = r.kclient.Extensions().Deployments(dp.Namespace).Patch(dp.Name, api.StrategicMergePatchType, deactivateBuild)
				if err != nil {
					return fmt.Errorf("%s - failed deactivating deployment: %s", logHeader, err)
				}
				return nil
			}
			return fmt.Errorf("%s - failed activating build for '%s'", logHeader, releaseTarget)
		}
	}
	// The release doesn't exists, create/build a new one!
	if err := validateRequiredKeys(dp); err != nil {
		return fmt.Errorf("%s - %s", logHeader, err)
	}

	gitSha, err := koliutil.NewSha(dp.Annotations[spec.KoliPrefix("gitrevision")])
	if err != nil {
		// TODO: add an event informing the problem!
		return fmt.Errorf("%s - %s", logHeader, err)
	}
	dpVersion := fmt.Sprintf("%s-v%d", dp.Name, dp.Status.ObservedGeneration)
	deployRelease := false
	if dp.Annotations[spec.KoliPrefix("deployrelease")] == "true" {
		// Deploy it after the build
		deployRelease = true
	}
	release := &spec.Release{
		TypeMeta: unversioned.TypeMeta{
			Kind:       "Release",
			APIVersion: spec.SchemeGroupVersion.String(),
		},
		ObjectMeta: api.ObjectMeta{
			Name:        dpVersion,
			Namespace:   dp.Namespace,
			Annotations: map[string]string{spec.KoliPrefix("build"): "true"},
			// Useful for filtering
			Labels: map[string]string{
				spec.KoliPrefix("deploy"):      dp.Name,
				spec.KoliPrefix("gitrevision"): gitSha.Short(),
			},
		},
		Spec: spec.ReleaseSpec{
			GitRemote:     dp.Annotations[spec.KoliPrefix("gitremote")],
			GitRepository: dp.Annotations[spec.KoliPrefix("gitrepository")],
			GitRevision:   gitSha.Full(),
			DeployRelease: deployRelease,
			Token:         dp.Annotations[spec.KoliPrefix("gittoken")],
		},
	}
	if _, err := r.clientset.Release(release.Namespace).Create(release); err != nil {
		return fmt.Errorf("%s - failed creating new release: %s", logHeader, err)
	}

	glog.Infof("%s - new release created '%s'", logHeader, release.Name)

	// We need to update the 'build' key to false, otherwise the build will be triggered again.
	// TODO: Need other strategy for dealing with this kind of scenario
	deactivateBuild := activateBuildPayload(false)
	_, err = r.kclient.Extensions().Deployments(dp.Namespace).Patch(dp.Name, api.StrategicMergePatchType, deactivateBuild)
	if err != nil {
		return fmt.Errorf("%s - failed deactivating deployment: %s", logHeader, err)
	}
	return nil
}

func validateRequiredKeys(dp *extensions.Deployment) error {
	for _, key := range requiredKeys {
		_, ok := dp.Annotations[spec.KoliPrefix(key)]
		if !ok {
			return fmt.Errorf("missing required key '%s'", key)
		}
	}
	return nil
}

func activateBuildPayload(activate bool) []byte {
	build := "false"
	if activate {
		build = "true"
	}
	payload := fmt.Sprintf(`{"metadata": {"annotations": {"%s": "%s"}}}`, spec.KoliPrefix("build"), build)
	return []byte(payload)
}
