package controller

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"time"

	"github.com/golang/glog"

	platform "kolihub.io/koli/pkg/apis/core/v1alpha1"
	"kolihub.io/koli/pkg/apis/core/v1alpha1/draft"
	clientset "kolihub.io/koli/pkg/clientset"
	"kolihub.io/koli/pkg/request"
	"kolihub.io/koli/pkg/spec"
	"kolihub.io/koli/pkg/util"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"

	"k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	d.queue = NewTaskQueue("deployer", d.syncHandler)

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
	nsMeta := draft.NewNamespaceMetadata(pod.Namespace)
	if !nsMeta.IsValid() {
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
	// turn-off build, otherwise it will trigger unwanted builds
	defer func() {
		incrementBuildStatusMetric(pod.Status.Phase)
		patchData := []byte(`{"spec": {"build": false}}`)
		_, err = d.clientset.Release(pod.Namespace).Patch(release.Name, types.MergePatchType, patchData)
		if err != nil {
			glog.Warningf("%s - failed turning off build", key)
		}
	}()

	var gitcli *request.Request
	info := &platform.GitInfo{}
	// var err error
	if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
		// TODO: log the response of /dev/termination-log to events!
		// Ref: https://github.com/koli/koli/issues/149
		gitcli, err = newGitAPIClient(
			d.config.GitReleaseHost,
			release.Spec.DeployName,
			d.config.PlatformJWTSecret,
			nsMeta,
		)
		if err != nil {
			glog.Errorf("%s - failed retrieving git api client [%v]", key, err)
			return nil
		}
		respBody, err := gitcli.Resource("seek").Get().
			AddQuery("q", pod.Name).
			AddQuery("in", "kubeRef").
			Do().Raw()
		if err != nil {
			return fmt.Errorf("failed searching for release, %v", err)
		}
		infoList := &platform.GitInfoList{}
		if err := json.Unmarshal(respBody, infoList); err != nil {
			glog.Warningf("%s - failed deserializing releases: %v, [%v]", key, err, string(respBody))
			return nil
		}
		if len(infoList.Items) == 0 {
			glog.Infof(`%s - git api release not found`, key)
			return nil
		}
		// Ignore if multiple are found for now
		info = &infoList.Items[0]
	}
	if pod.Status.Phase == v1.PodSucceeded {
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
		glog.V(3).Infof("%s - Found a release candidate [%s]", key, info.HeadCommit.ID)
		if isAutoDeploy(deploy, info.HeadCommit.ID) {
			releaseURL := release.GitReleaseURL(d.config.GitReleaseHost)
			if err := d.deploySlug(releaseURL, info.HeadCommit.ID, info.Lang, deploy); err != nil {
				d.recorder.Eventf(release, v1.EventTypeWarning, "DeployError",
					"Failed deploying release [%s]", err)
				return fmt.Errorf("failed deploying release [%s]", err)
			}
			buildsDeployed.Inc()
			ref := info.HeadCommit.ID
			if len(ref) > 7 {
				ref = ref[:7]
			}
			d.recorder.Eventf(release, v1.EventTypeNormal,
				"Deployed", "Deploy '%s' updated with the new revision [%s]", deploy.Name, ref)
		}
	}
	// Update the release in Git API
	if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
		info.Name = release.Spec.DeployName
		info.Namespace = pod.Namespace
		info.Status = pod.Status.Phase
		if len(pod.Status.ContainerStatuses) > 0 {
			terminatedState := pod.Status.ContainerStatuses[0].State.Terminated
			if terminatedState != nil {
				delta := terminatedState.FinishedAt.Sub(terminatedState.StartedAt.Time)
				info.BuildDuration = delta.Round(time.Second)
			}
		}
		if len(info.HeadCommit.ID) == 0 {
			glog.Warningf("%s - Commit ID is empty, release metadata will not be updated", key)
			return nil
		}
		_, err = gitcli.Reset().Put().
			Resource("objects").
			Name(info.HeadCommit.ID).
			Body(info).
			Do().Raw()
		if err != nil {
			return fmt.Errorf("failed updating git api release [%v]", err)
		}
		glog.Infof(`%s - Removing pod and release "%s"`, key, release.Name)
		d.kclient.Core().Pods(pod.Namespace).Delete(pod.Name, &metav1.DeleteOptions{})
		d.clientset.Release(pod.Namespace).Delete(release.Name, &metav1.DeleteOptions{})
	}
	return nil
}

func (d *DeployerController) deploySlug(releaseURL, commitID, appLang string, deploy *extensions.Deployment) error {
	dpCopy := deploy.DeepCopy()
	if dpCopy == nil {
		return fmt.Errorf("failed deep copying: %v", deploy)
	}
	dpCopy.Spec.Paused = false
	if dpCopy.Annotations == nil {
		dpCopy.Annotations = make(map[string]string)
	}
	dpCopy.Annotations["kolihub.io/deployed-git-revision"] = commitID
	dpCopy.Labels["kolihub.io/lang"] = appLang
	dpCopy.Spec.Template.Labels["kolihub.io/lang"] = appLang
	dpCopy.Spec.Selector.MatchLabels["kolihub.io/lang"] = appLang
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
	slugURL := fmt.Sprintf("%s/%s/slug.tgz", releaseURL, commitID)
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

func isAutoDeploy(d *extensions.Deployment, commitID string) bool {
	return d.Annotations[platform.AnnotationAutoDeploy] == "true" &&
		d.Annotations["kolihub.io/deployed-git-revision"] != commitID
}

func newGitAPIClient(host, deployName, jwtSecret string, nsMeta *draft.NamespaceMeta) (*request.Request, error) {
	// Generate a system token based on the customer and organization of the namespace.
	// The access token has limited access to the resources of the platform
	namespace := nsMeta.KubernetesNamespace()
	systemToken, err := util.GenerateNewJwtToken(
		jwtSecret,
		nsMeta.Customer(),
		nsMeta.Organization(),
		platform.SystemTokenType,
		time.Now().UTC().Add(time.Minute*5), // hard-coded exp time
	)
	if err != nil {
		return nil, err
	}
	requestURL, err := url.Parse(host)
	if err != nil {
		return nil, err
	}
	basePath := filepath.Join("releases", "v1beta1", namespace, deployName)
	requestURL.Path = basePath
	basicAuth := base64.StdEncoding.EncodeToString([]byte("dumb:" + systemToken))
	return request.NewRequest(nil, requestURL).
		SetHeader("Authorization", fmt.Sprintf("Basic %s", basicAuth)), nil
}

func incrementBuildStatusMetric(phase v1.PodPhase) {
	switch phase {
	case v1.PodSucceeded:
		buildsCompleted.Inc()
	case v1.PodFailed:
		buildsCompleted.Inc()
	}
}
