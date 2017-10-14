package controller

import (
	"fmt"
	"time"

	"github.com/golang/glog"
	platform "kolihub.io/koli/pkg/apis/core/v1alpha1"
	"kolihub.io/koli/pkg/apis/core/v1alpha1/draft"
	clientset "kolihub.io/koli/pkg/clientset"
	"kolihub.io/koli/pkg/spec"
	koliutil "kolihub.io/koli/pkg/util"

	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

// BuildController controller
type BuildController struct {
	kclient    kubernetes.Interface
	clientset  clientset.CoreInterface
	releaseInf cache.SharedIndexInformer
	config     *Config

	queue    *TaskQueue
	recorder record.EventRecorder
}

// NewBuildController creates a new BuilderController
func NewBuildController(
	config *Config,
	releaseInf cache.SharedIndexInformer,
	sysClient clientset.CoreInterface,
	kclient kubernetes.Interface) *BuildController {

	b := &BuildController{
		kclient:    kclient,
		clientset:  sysClient,
		releaseInf: releaseInf,
		recorder:   newRecorder(kclient, "apps-controller"),
		config:     config,
	}
	b.queue = NewTaskQueue(b.syncHandler)

	b.releaseInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    b.addRelease,
		UpdateFunc: b.updateRelease,
		DeleteFunc: b.deleteRelease,
	})
	return b
}

func (b *BuildController) addRelease(obj interface{}) {
	release := obj.(*platform.Release)
	glog.Infof("add-release - %s/%s", release.Namespace, release.Name)

	b.queue.Add(release)
	// b.queue.add(release)
}

func (b *BuildController) updateRelease(o, n interface{}) {
	old := o.(*platform.Release)
	new := n.(*platform.Release)
	if old.ResourceVersion == new.ResourceVersion {
		return
	}

	glog.Infof("update-release - %s/%s", new.Namespace, new.Name)
	b.queue.Add(new)
	// b.queue.add(new)
}

func (b *BuildController) deleteRelease(obj interface{}) {
	release := obj.(*platform.Release)
	glog.Infof("delete-release - %s/%s", release.Namespace, release.Name)
	b.queue.Add(release)
	// b.queue.add(release)
}

// Run the controller.
func (b *BuildController) Run(workers int, stopc <-chan struct{}) {
	// don't let panics crash the process
	defer utilruntime.HandleCrash()
	// make sure the work queue is shutdown which will trigger workers to end
	defer b.queue.shutdown()

	glog.Info("Starting Build Controller...")

	if !cache.WaitForCacheSync(stopc, b.releaseInf.HasSynced) {
		return
	}

	for i := 0; i < workers; i++ {
		// runWorker will loop until "something bad" happens.
		// The .Until will then rekick the worker after one second
		go b.queue.run(time.Second, stopc)
		// go wait.Until(b.runWorker, time.Second, stopc)
	}

	// wait until we're told to stop
	<-stopc
	glog.Info("shutting down Build controller...")
}

func (b *BuildController) syncHandler(key string) error {
	obj, exists, err := b.releaseInf.GetStore().GetByKey(key)
	if err != nil {
		return err
	}
	if err != nil {
		return err
	}
	if !exists {
		glog.V(3).Infof("%s - release doesn't exists, skip ...", key)
		return nil
	}
	release := obj.(*platform.Release)
	if release.DeletionTimestamp != nil {
		// TODO: delete from the remote object store (minio/s3/gcs...)
		glog.V(3).Infof("%s - release marked for deletion, skipping ...", key)
		return nil
	}

	if !draft.NewNamespaceMetadata(release.Namespace).IsValid() {
		glog.V(3).Infof("%s - noop, it's not a valid namespace", key)
		return nil
	}

	if !release.Spec.Build {
		glog.V(3).Infof("%s - noop, isn't a build action", key)
		return nil
	}

	gitSha, err := koliutil.NewSha(release.Spec.GitRevision)
	if err != nil {
		// TODO: add an event informing the problem!
		return fmt.Errorf("%s - %s", key, err)
	}

	info := koliutil.NewSlugBuilderInfo(
		release.Namespace,
		release.Spec.DeployName,
		platform.GitReleasesPathPrefix,
		gitSha)
	sbPodName := fmt.Sprintf("sb-%s", release.Name)
	pod, err := slugbuilderPod(b.config, release, gitSha, info)
	if err != nil {
		return fmt.Errorf("%s - failed creating slugbuild pod: %s", key, err)
	}
	_, err = b.kclient.Core().Pods(release.Namespace).Create(pod)
	if err != nil {
		// TODO: add an event informing the problem!
		// TODO: requeue with backoff, got an error
		return fmt.Errorf("%s - failed creating the slubuild pod: %s", key, err)
	}

	glog.Infof("%s - build started for '%s'", key, sbPodName)
	// a build has started for this release, disable it!
	_, err = b.clientset.Release(release.Namespace).Patch(release.Name, types.MergePatchType, []byte(`{"spec": {"build": false}}`))
	if err != nil {
		return fmt.Errorf("failed updating release [%s]", err)
	}
	return nil
}

func slugbuilderPod(cfg *Config, rel *platform.Release, gitSha *koliutil.SHA, info *koliutil.SlugBuilderInfo) (*v1.Pod, error) {
	gitCloneURL := rel.Spec.GitRemote
	if !rel.IsGitHubSource() {
		var err error
		gitCloneURL, err = rel.GitCloneURL()
		if err != nil {
			return nil, err
		}
	}
	env := map[string]interface{}{
		"GIT_CLONE_URL":   gitCloneURL,
		"GIT_RELEASE_URL": rel.GitReleaseURL(cfg.GitReleaseHost),
		"GIT_REVISION":    rel.Spec.GitRevision,
		// "AUTH_TOKEN":      rel.Spec.AuthToken,
	}
	if cfg.DebugBuild {
		env["DEBUG"] = "TRUE"
	}
	pod := podResource(rel, gitSha, env)

	// Slugbuilder
	pod.Spec.Containers[0].Image = cfg.SlugBuildImage
	pod.Spec.Containers[0].Name = rel.Name

	// A dynamic JWT Token secret provisioned by a controller
	pod.Spec.Containers[0].Env = append(pod.Spec.Containers[0].Env, v1.EnvVar{
		Name: "AUTH_TOKEN",
		ValueFrom: &v1.EnvVarSource{
			SecretKeyRef: &v1.SecretKeySelector{
				LocalObjectReference: v1.LocalObjectReference{
					Name: platform.SystemSecretName,
				},
				Key: "token.jwt", // TODO: hard-coded
			},
		},
	})
	// addEnvToContainer(pod,a "PUT_PATH", info.PushKey(), 0)
	return &pod, nil
}

func podResource(rel *platform.Release, gitSha *koliutil.SHA, env map[string]interface{}) v1.Pod {
	sbPodName := fmt.Sprintf("sb-%s", rel.Name)
	pod := v1.Pod{
		Spec: v1.PodSpec{
			RestartPolicy: v1.RestartPolicyNever,
			Containers: []v1.Container{
				{
					ImagePullPolicy: v1.PullIfNotPresent,
				},
			},
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      sbPodName,
			Namespace: rel.Namespace,
			Annotations: map[string]string{
				spec.KoliPrefix("gitfullrev"):  gitSha.Full(),
				spec.KoliPrefix("releasename"): rel.Name,
			},
			Labels: map[string]string{
				// TODO: hard-coded
				spec.KoliPrefix("autodeploy"):    "true",
				spec.KoliPrefix("type"):          "slugbuild",
				spec.KoliPrefix("gitrevision"):   gitSha.Short(),
				spec.KoliPrefix("buildrevision"): rel.Spec.BuildRevision,
			},
		},
	}
	if rel.Labels != nil {
		if appName, ok := rel.Labels["kolihub.io/deploy"]; ok {
			pod.Labels[platform.AnnotationApp] = appName
		}
	}

	for k, v := range env {
		for i := range pod.Spec.Containers {
			pod.Spec.Containers[i].Env = append(pod.Spec.Containers[i].Env, v1.EnvVar{
				Name:  k,
				Value: fmt.Sprintf("%v", v),
			})
		}
	}
	return pod
}

func addEnvToContainer(pod v1.Pod, key, value string, index int) {
	if len(pod.Spec.Containers) > 0 {
		pod.Spec.Containers[index].Env = append(pod.Spec.Containers[index].Env, v1.EnvVar{
			Name:  key,
			Value: value,
		})
	}
}
