package controller

import (
	"fmt"
	"time"

	"github.com/golang/glog"
	platform "kolihub.io/koli/pkg/apis/v1alpha1"
	clientset "kolihub.io/koli/pkg/clientset"
	"kolihub.io/koli/pkg/spec"
	specutil "kolihub.io/koli/pkg/spec/util"
	koliutil "kolihub.io/koli/pkg/util"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	release := obj.(*spec.Release)
	glog.Infof("add-release - %s/%s", release.Namespace, release.Name)

	b.queue.Add(release)
	// b.queue.add(release)
}

func (b *BuildController) updateRelease(o, n interface{}) {
	old := o.(*spec.Release)
	new := n.(*spec.Release)
	if old.ResourceVersion == new.ResourceVersion {
		return
	}

	glog.Infof("update-release - %s/%s", new.Namespace, new.Name)
	b.queue.Add(new)
	// b.queue.add(new)
}

func (b *BuildController) deleteRelease(obj interface{}) {
	release := obj.(*spec.Release)
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
	obj, _, err := b.releaseInf.GetStore().GetByKey(key)
	if err != nil {
		return err
	}
	release := obj.(*spec.Release)
	logHeader := fmt.Sprintf("%s/%s", release.Namespace, release.Name)

	if release.DeletionTimestamp != nil {
		// TODO: delete from the remote object store (minio/s3/gcs...)
		glog.V(3).Infof("%s - release marked for deletion, skipping ...", logHeader)
		return nil
	}
	pns, err := platform.NewNamespace(release.Namespace)
	if err != nil {
		glog.V(3).Infof("%s - noop, it's not a valid namespace", logHeader)
		return nil
	}

	if !release.Spec.Build {
		glog.V(3).Infof("%s - noop, isn't a build action", logHeader)
		return nil
	}

	gitSha, err := koliutil.NewSha(release.Spec.GitRevision)
	if err != nil {
		// TODO: add an event informing the problem!
		return fmt.Errorf("%s - %s", logHeader, err)
	}

	info := koliutil.NewSlugBuilderInfo(
		pns.GetNamespace(),
		release.Spec.DeployName,
		platform.GitReleasesPathPrefix,
		gitSha)
	sbPodName := fmt.Sprintf("sb-%s", release.Name)
	pod, err := slugbuilderPod(b.config, release, gitSha, info)
	if err != nil {
		return fmt.Errorf("%s - failed creating slugbuild pod: %s", logHeader, err)
	}
	_, err = b.kclient.Core().Pods(release.Namespace).Create(pod)
	if err != nil {
		// TODO: add an event informing the problem!
		// TODO: requeue with backoff, got an error
		return fmt.Errorf("%s - failed creating the slubuild pod: %s", logHeader, err)
	}

	glog.Infof("%s - build started for '%s'", logHeader, sbPodName)
	releaseCopy, err := specutil.ReleaseDeepCopy(release)
	if err != nil {
		return fmt.Errorf("%s - failed deep copying release: %s", logHeader, err)
	}
	// a build has started for this release, disable it!
	releaseCopy.Spec.Build = false
	// releaseCopy.TypeMeta = unversioned.TypeMeta{
	// 	Kind:       "Release",
	// 	APIVersion: spec.SchemeGroupVersion.String(),
	// }

	_, err = b.clientset.Release(releaseCopy.Namespace).Update(releaseCopy)
	if err != nil {
		return fmt.Errorf("%s - failed updating release: %s", logHeader, err)
	}
	return nil
}

func slugbuilderPod(cfg *Config, rel *spec.Release, gitSha *koliutil.SHA, info *koliutil.SlugBuilderInfo) (*v1.Pod, error) {
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
		"AUTH_TOKEN":      rel.Spec.AuthToken,
	}
	if cfg.DebugBuild {
		env["DEBUG"] = "TRUE"
	}
	pod := podResource(rel, gitSha, env)

	// Slugbuilder
	pod.Spec.Containers[0].Image = cfg.SlugBuildImage
	pod.Spec.Containers[0].Name = rel.Name

	// addEnvToContainer(pod,a "PUT_PATH", info.PushKey(), 0)
	return &pod, nil
}

func podResource(rel *spec.Release, gitSha *koliutil.SHA, env map[string]interface{}) v1.Pod {
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

	if len(pod.Spec.Containers) > 0 {
		for k, v := range env {
			for i := range pod.Spec.Containers {
				pod.Spec.Containers[i].Env = append(pod.Spec.Containers[i].Env, v1.EnvVar{
					Name:  k,
					Value: fmt.Sprintf("%v", v),
				})
			}
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
