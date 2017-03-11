package controller

import (
	"fmt"
	"time"

	"github.com/golang/glog"

	clientset "kolihub.io/koli/pkg/clientset"
	"kolihub.io/koli/pkg/platform"
	"kolihub.io/koli/pkg/spec"
	specutil "kolihub.io/koli/pkg/spec/util"
	koliutil "kolihub.io/koli/pkg/util"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/cache"
	"k8s.io/kubernetes/pkg/util/wait"

	extensions "k8s.io/kubernetes/pkg/apis/extensions"
	kclientset "k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset"
	utilruntime "k8s.io/kubernetes/pkg/util/runtime"
)

const (
	// ignore deploying releases that have more than X minutes of life
	autoDeployExpireInMinutes = 20
)

// DeployerController controller
type DeployerController struct {
	kclient   kclientset.Interface
	clientset clientset.CoreInterface
	podInf    cache.SharedIndexInformer
	dpInf     cache.SharedIndexInformer
	relInf    cache.SharedIndexInformer
	queue     *queue
	config    *Config
}

// NewDeployerController creates a new DeployerController
func NewDeployerController(
	config *Config, podInf,
	dpInf, relInf cache.SharedIndexInformer,
	sysClient clientset.CoreInterface,
	kclient kclientset.Interface) *DeployerController {
	d := &DeployerController{
		kclient:   kclient,
		clientset: sysClient,
		podInf:    podInf,
		dpInf:     dpInf,
		relInf:    relInf,
		queue:     newQueue(200),
		config:    config,
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
	if old.ResourceVersion == new.ResourceVersion || old.Status.Phase == new.Status.Phase {
		return
	}
	glog.Infof("update-pod - %s/%s", new.Namespace, new.Name)
	d.queue.add(new)
}

func (d *DeployerController) deletePod(obj interface{}) {
	// pod := obj.(*api.Pod)
	// glog.Infof("delete-pod - %s/%s noop", pod.Namespace, pod.Name)
	// d.queue.add(pod)
}

// Run the controller.
func (d *DeployerController) Run(workers int, stopc <-chan struct{}) {
	// don't let panics crash the process
	defer utilruntime.HandleCrash()
	// make sure the work queue is shutdown which will trigger workers to end
	defer d.queue.close()

	glog.Info("Starting Deployer Controller...")

	if !cache.WaitForCacheSync(stopc, d.podInf.HasSynced, d.dpInf.HasSynced, d.relInf.HasSynced) {
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
		glog.V(4).Infof("%s - noop, it's not a valid namespace", logHeader)
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

	releaseQ := &spec.Release{}
	releaseQ.SetName(releaseName)
	releaseQ.SetNamespace(pod.Namespace)
	releaseO, exists, err := d.relInf.GetStore().Get(releaseQ)
	if err != nil {
		return err
	}

	if !exists {
		return fmt.Errorf("%s - release '%s' doesn't exists anymore", logHeader, releaseName)
	}

	// Release expires with time, otherwise we could have wrong behavior
	// with old releases being deployed due to the async behavior of controllers.
	release := releaseO.(*spec.Release)
	if release.Expired() {
		glog.V(4).Infof("%s - auto deploy expired for release '%s'", logHeader, release.Name)
	}

	if pod.Status.Phase == api.PodSucceeded && release.Spec.AutoDeploy && !release.Expired() {
		if err := d.deploySlug(release); err != nil {
			glog.Infof("%s - failed deploying: %s", logHeader, err)
		} else {
			glog.Infof("%s - deployed successfully", logHeader)
		}
	}

	releaseCopy, err := specutil.ReleaseDeepCopy(release)
	if err != nil {
		return fmt.Errorf("%s - failed deep copying: %s", logHeader, err)
	}
	// turn-off build, otherwise it will trigger unwanted builds
	releaseCopy.Spec.Build = false
	releaseCopy.Labels[spec.KoliPrefix("buildstatus")] = string(pod.Status.Phase)
	if _, err := d.clientset.Release(pod.Namespace).Update(releaseCopy); err != nil {
		return fmt.Errorf("%s - failed updating pod status: %s", logHeader, err)
	}
	return nil
}

func (d *DeployerController) deploySlug(release *spec.Release) error {
	deploymentQ := &extensions.Deployment{}
	deploymentQ.SetName(release.Spec.DeployName)
	deploymentQ.SetNamespace(release.Namespace)
	deploymentO, exists, err := d.dpInf.GetStore().Get(deploymentQ)
	if err != nil {
		return fmt.Errorf("failed retrieving deploy from cache: %s", err)
	}
	if !exists {
		return fmt.Errorf("deploy '%s' doesn't exists", deploymentQ.Name)
	}
	dpCopy, err := specutil.DeploymentDeepCopy(deploymentO.(*extensions.Deployment))
	if err != nil {
		return fmt.Errorf("failed deep copying: %s", err)
	}
	dpCopy.Spec.Paused = false
	gitSha, err := koliutil.NewSha(release.Spec.GitRevision)
	if err != nil {
		return fmt.Errorf("wrong sha: %s", err)
	}
	info := koliutil.NewSlugBuilderInfo(
		dpCopy.Namespace,
		dpCopy.Name,
		platform.GitReleasesPathPrefix,
		gitSha)
	c := dpCopy.Spec.Template.Spec.Containers
	// TODO: hard-coded
	c[0].Ports = []api.ContainerPort{
		{
			Name:          "http",
			ContainerPort: 5000,
			Protocol:      api.ProtocolTCP,
		},
	}
	c[0].Args = []string{"start", "web"} // TODO: hard-coded, it must come from Procfile
	c[0].Image = d.config.SlugRunnerImage
	c[0].Name = dpCopy.Name
	c[0].Env = []api.EnvVar{
		{
			Name:  "SLUG_URL",
			Value: fmt.Sprintf("%s/%s", d.config.GitReleaseHost, info.TarKey()),
		},
		{
			Name:  "AUTH_TOKEN",
			Value: release.Spec.AuthToken,
		},
		{
			Name:  "DEBUG",
			Value: "TRUE",
		},
	}
	_, err = d.kclient.Extensions().Deployments(dpCopy.Namespace).Update(dpCopy)
	if err != nil {
		return fmt.Errorf("failed update deployment: %s", err)
	}
	return nil
}
