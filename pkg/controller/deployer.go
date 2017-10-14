package controller

import (
	"fmt"
	"strconv"
	"time"

	"github.com/golang/glog"

	platform "kolihub.io/koli/pkg/apis/core/v1alpha1"
	"kolihub.io/koli/pkg/apis/core/v1alpha1/draft"
	clientset "kolihub.io/koli/pkg/clientset"
	"kolihub.io/koli/pkg/spec"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"

	"k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

const (
	// ignore deploying releases that have more than X minutes of life
	autoDeployExpireInMinutes = 20
)

// DeployerController controller
type DeployerController struct {
	kclient   kubernetes.Interface
	clientset clientset.CoreInterface
	podInf    cache.SharedIndexInformer
	dpInf     cache.SharedIndexInformer
	relInf    cache.SharedIndexInformer
	config    *Config

	queue    *TaskQueue
	recorder record.EventRecorder
}

// NewDeployerController creates a new DeployerController
func NewDeployerController(
	config *Config, podInf,
	dpInf, relInf cache.SharedIndexInformer,
	sysClient clientset.CoreInterface,
	kclient kubernetes.Interface) *DeployerController {
	d := &DeployerController{
		kclient:   kclient,
		clientset: sysClient,
		podInf:    podInf,
		dpInf:     dpInf,
		relInf:    relInf,
		config:    config,
		recorder:  newRecorder(kclient, "apps-controller"),
	}
	d.queue = NewTaskQueue(d.syncHandler)

	d.podInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    d.addPod,
		UpdateFunc: d.updatePod,
		// DeleteFunc: d.deletePod,
	})
	return d
}

func (d *DeployerController) addPod(obj interface{}) {
	pod := obj.(*v1.Pod)
	glog.V(2).Infof("add-pod - %s/%s", pod.Namespace, pod.Name)
	d.queue.Add(pod)
}

func (d *DeployerController) updatePod(o, n interface{}) {
	old := o.(*v1.Pod)
	new := n.(*v1.Pod)
	if old.ResourceVersion == new.ResourceVersion || old.Status.Phase == new.Status.Phase {
		return
	}
	glog.V(2).Infof("update-pod - %s/%s", new.Namespace, new.Name)
	d.queue.Add(new)
}

func (d *DeployerController) deletePod(obj interface{}) {
	// pod := obj.(*v1.Pod)
	// glog.Infof("delete-pod - %s/%s noop", pod.Namespace, pod.Name)
	// d.queue.add(pod)
}

// Run the controller.
func (d *DeployerController) Run(workers int, stopc <-chan struct{}) {
	// don't let panics crash the process
	defer utilruntime.HandleCrash()
	// make sure the work queue is shutdown which will trigger workers to end
	defer d.queue.shutdown()

	glog.Info("Starting Deployer Controller...")

	if !cache.WaitForCacheSync(stopc, d.podInf.HasSynced, d.dpInf.HasSynced, d.relInf.HasSynced) {
		return
	}

	for i := 0; i < workers; i++ {
		// runWorker will loop until "something bad" happens.
		// The .Until will then rekick the worker after one second
		go d.queue.run(time.Second, stopc)
		// go wait.Until(d.runWorker, time.Second, stopc)
	}

	// wait until we're told to stop
	<-stopc
	glog.Info("shutting down Deployer controller...")
}

func (d *DeployerController) syncHandler(key string) error {
	obj, exists, err := d.podInf.GetStore().GetByKey(key)
	if err != nil {
		glog.Warningf("%s - failed retrieving object from store [%s]", key, err)
		return err
	}

	if !exists {
		glog.V(2).Infof("%s - the pod doesn't exists")
		return nil
	}

	pod := obj.(*v1.Pod)
	if !draft.NewNamespaceMetadata(pod.Namespace).IsValid() {
		glog.V(2).Infof("%s - noop, it's not a valid namespace", key)
		return nil
	}

	releaseName := pod.Annotations[spec.KoliPrefix("releasename")]
	if releaseName == "" {
		msg := "Couldn't find the release for the build, an annotation is missing on pod. " +
			"The build must be manually deployed."
		d.recorder.Event(pod, v1.EventTypeWarning, "ReleaseNotFound", msg)
		glog.Warningf("%s - missing 'releasename' annotation on pod", key)
		return nil
	}

	releaseQ := &platform.Release{}
	releaseQ.SetName(releaseName)
	releaseQ.SetNamespace(pod.Namespace)
	releaseO, exists, err := d.relInf.GetStore().Get(releaseQ)
	if err != nil {
		d.recorder.Eventf(pod, v1.EventTypeWarning, "DeployError",
			"Found an error retrieving the release from cache [%s].", err)
		glog.Warningf("%s - found an error retrieving the release from cache [%s]", key, err)
		return err
	}

	if !exists {
		d.recorder.Event(pod, v1.EventTypeWarning, "ReleaseNotFound",
			"The release wasn't found for this build, the build must be manually deployed.")
		glog.Warningf("%s - release '%s' doesn't exist anymore", key, releaseName)
		return nil
	}
	release := releaseO.(*platform.Release)
	if pod.Status.Phase == v1.PodSucceeded && release.Spec.AutoDeploy {
		deploymentQ := &extensions.Deployment{}
		deploymentQ.SetName(release.Spec.DeployName)
		deploymentQ.SetNamespace(release.Namespace)
		deploymentO, exists, err := d.dpInf.GetStore().Get(deploymentQ)
		if err != nil {
			return fmt.Errorf("failed retrieving deploy from cache [%s]", err)
		}
		if !exists {
			d.recorder.Eventf(release, v1.EventTypeWarning,
				"DeployNotFound", "Deploy '%s' not found", release.Spec.DeployName)
			// There's any resource to deploy, do not requeue
			return nil
		}
		deploy := deploymentO.(*extensions.Deployment)
		deployBuildRev, _ := strconv.Atoi(deploy.Annotations["kolihub.io/buildrevision"])
		releaseBuildRev := release.BuildRevision()
		// The release revision must be greater or equal than the current deployment and
		// it must have distinct git revisions, otherwise it will deploy an old
		// app version. This will prevent old pod resources (completed)
		// trigerring unwanted deploys
		if releaseBuildRev < deployBuildRev {
			glog.V(2).Infof("%s - found release revision [%d] and deploy revision as [%d], skipping autodeploy",
				key, releaseBuildRev, deployBuildRev)
			return nil
		}

		if deploy.Annotations["kolihub.io/deployed-git-revision"] == release.Spec.GitRevision {
			glog.V(2).Infof("%s - this release was already deployed [%s], skipping autodeploy", key, release.Spec.GitRevision)
			return nil
		}

		if err := d.deploySlug(release, deploy); err != nil {
			d.recorder.Eventf(release, v1.EventTypeWarning, "DeployError",
				"Failed deploying release [%s]", err)
			return fmt.Errorf("failed deploying release [%s]", err)
		}
		d.recorder.Eventf(release, v1.EventTypeNormal,
			"Deployed", "Deploy '%s' updated with the new revision [%s]", release.Spec.DeployName, release.Spec.GitRevision[:7])
	}

	// turn-off build, otherwise it will trigger unwanted builds
	patchData := []byte(fmt.Sprintf(`{"metadata": {"labels": {"kolihub.io/buildstatus": "%v"}}, "spec": {"build": false}}`, pod.Status.Phase))
	_, err = d.clientset.Release(pod.Namespace).Patch(release.Name, types.MergePatchType, patchData)
	if err != nil {
		return fmt.Errorf("failed patching pod status[%s] in release [%s]", pod.Status.Phase, err)
	}
	return nil
}

func (d *DeployerController) deploySlug(release *platform.Release, deploy *extensions.Deployment) error {
	dpCopy := deploy.DeepCopy()
	if dpCopy == nil {
		return fmt.Errorf("failed deep copying: %v", deploy)
	}
	dpCopy.Spec.Paused = false
	if dpCopy.Annotations == nil {
		dpCopy.Annotations = make(map[string]string)
	}
	dpCopy.Annotations["kolihub.io/deployed-git-revision"] = release.Spec.GitRevision
	c := dpCopy.Spec.Template.Spec.Containers
	// TODO: hard-coded
	c[0].Ports = []v1.ContainerPort{
		{
			Name:          "http",
			ContainerPort: 5000,
			Protocol:      v1.ProtocolTCP,
		},
	}
	c[0].Args = []string{"start", "web"} // TODO: hard-coded, it must come from Procfile
	c[0].Image = d.config.SlugRunnerImage
	c[0].Name = dpCopy.Name
	slugURL := release.GitReleaseURL(d.config.GitReleaseHost) + "/slug.tgz"
	c[0].Env = []v1.EnvVar{
		{
			Name:  "SLUG_URL",
			Value: slugURL,
		},
		// A dynamic JWT Token secret provisioned by a controller
		{
			Name: "AUTH_TOKEN",
			ValueFrom: &v1.EnvVarSource{
				SecretKeyRef: &v1.SecretKeySelector{
					LocalObjectReference: v1.LocalObjectReference{
						Name: platform.SystemSecretName,
					},
					Key: "token.jwt", // TODO: hard-coded
				},
			},
		},
		{
			Name:  "DEBUG",
			Value: "TRUE",
		},
	}
	_, err := d.kclient.Extensions().Deployments(dpCopy.Namespace).Update(dpCopy)
	if err != nil {
		return fmt.Errorf("failed update deployment: %s", err)
	}
	return nil
}
