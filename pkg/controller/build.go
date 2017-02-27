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

	apierrors "k8s.io/kubernetes/pkg/api/errors"
	extensions "k8s.io/kubernetes/pkg/apis/extensions"
	kclientset "k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset"
	utilruntime "k8s.io/kubernetes/pkg/util/runtime"
)

const (
	tprReleases = "release.platform.koli.io"
)

// BuildController controller
type BuildController struct {
	kclient    kclientset.Interface
	clientset  clientset.CoreInterface
	releaseInf cache.SharedIndexInformer
	queue      *queue
	config     *Config
}

// NewBuildController creates a new BuilderController
func NewBuildController(
	config *Config,
	releaseInf cache.SharedIndexInformer,
	sysClient clientset.CoreInterface,
	kclient kclientset.Interface) *BuildController {
	b := &BuildController{
		kclient:    kclient,
		clientset:  sysClient,
		releaseInf: releaseInf,
		queue:      newQueue(200),
		config:     config,
	}

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
	b.queue.add(release)
}

func (b *BuildController) updateRelease(o, n interface{}) {
	old := o.(*spec.Release)
	new := n.(*spec.Release)
	if old.ResourceVersion == new.ResourceVersion {
		return
	}

	glog.Infof("update-release - %s/%s", new.Namespace, new.Name)
	b.queue.add(new)
}

func (b *BuildController) deleteRelease(obj interface{}) {
	release := obj.(*spec.Release)
	glog.Infof("delete-release - %s/%s", release.Namespace, release.Name)
	b.queue.add(release)
}

// Run the controller.
func (b *BuildController) Run(workers int, stopc <-chan struct{}) {
	// don't let panics crash the process
	defer utilruntime.HandleCrash()
	// make sure the work queue is shutdown which will trigger workers to end
	defer b.queue.close()

	glog.Info("Starting Build Controller...")

	if !cache.WaitForCacheSync(stopc, b.releaseInf.HasSynced) {
		return
	}

	for i := 0; i < workers; i++ {
		// runWorker will loop until "something bad" happens.
		// The .Until will then rekick the worker after one second
		go wait.Until(b.runWorker, time.Second, stopc)
	}

	// wait until we're told to stop
	<-stopc
	glog.Info("shutting down Build controller...")
}

// runWorker runs a worker thread that just dequeues items, processes them, and marks them done.
// It enforces that the syncHandler is never invoked concurrently with the same key.
func (b *BuildController) runWorker() {
	for {
		release, ok := b.queue.pop(&spec.Release{})
		if !ok {
			return
		}

		if err := b.reconcile(release.(*spec.Release)); err != nil {
			utilruntime.HandleError(err)
		}
	}
}

func (b *BuildController) reconcile(release *spec.Release) error {
	key, err := keyFunc(release)
	if err != nil {
		return err
	}

	logHeader := fmt.Sprintf("%s/%s", release.Namespace, release.Name)
	pns, err := platform.NewNamespace(release.Namespace)
	if err != nil {
		glog.V(4).Infof("%s - noop, it's not a valid namespace", logHeader)
		return nil
	}

	_, exists, err := b.releaseInf.GetStore().GetByKey(key)
	if err != nil {
		return err
	}

	if !exists || release.DeletionTimestamp != nil {
		// TODO: delete from the remote object store (minio/s3/gcs...)
		glog.V(4).Infof("%s - release doesn't exists or was marked for deletion, skipping ...", logHeader)
		return nil
	}
	if !release.Spec.Build {
		glog.V(4).Infof("%s - noop, isn't a build action", logHeader)
		return nil
	}

	gitSha, err := koliutil.NewSha(release.Spec.GitRevision)
	if err != nil {
		// TODO: add an event informing the problem!
		return fmt.Errorf("%s - %s", logHeader, err)
	}

	info := koliutil.NewSlugBuilderInfo(pns.GetNamespace(), release.Spec.DeployName, gitSha)
	sbPodName := fmt.Sprintf("sb-%s", release.Name)
	pod := slugbuilderPod(b.config, release, gitSha, info)
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

// CreateReleaseTPRs generates the third party resource required for interacting with releases
func CreateReleaseTPRs(host string, kclient kclientset.Interface) error {
	tprs := []*extensions.ThirdPartyResource{
		{
			ObjectMeta: api.ObjectMeta{
				Name: tprReleases,
			},
			Versions: []extensions.APIVersion{
				{Name: "v1alpha1"},
			},
			Description: "Application Releases",
		},
	}
	tprClient := kclient.Extensions().ThirdPartyResources()
	for _, tpr := range tprs {
		if _, err := tprClient.Create(tpr); err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}
		glog.Infof("Third Party Resource '%s' provisioned", tpr.Name)
	}

	// We have to wait for the TPRs to be ready. Otherwise the initial watch may fail.
	return watch3PRs(host, "/apis/platform.koli.io/v1alpha1/releases", kclient)
}

func slugbuilderPod(cfg *Config, rel *spec.Release, gitSha *koliutil.SHA, info *koliutil.SlugBuilderInfo) *api.Pod {
	// TODO: get from controller startup config
	env := map[string]interface{}{
		"BUILDER_STORAGE":   cfg.ObjectStorageType,
		"ACCESS_KEY":        cfg.ObjectStorageAccessKey,
		"ACCESS_SECRET_KEY": cfg.ObjectStorageSecretKey,
		"BUCKET_NAME":       cfg.ClusterName,
		"S3_HOST":           cfg.ObjectStorageHost,
		"S3_PORT":           cfg.ObjectStoragePort,
		"GITREMOTE":         rel.Spec.GitRemote,
		"GITREVISION":       rel.Spec.GitRevision,
	}
	if cfg.DebugBuild {
		env["DEBUG"] = "TRUE"
	}
	pod := podResource(rel, gitSha, env)

	// Slugbuilder
	pod.Spec.Containers[0].Image = cfg.SlugBuildImage
	pod.Spec.Containers[0].Name = rel.Name

	addEnvToContainer(pod, "PUT_PATH", info.PushKey(), 0)
	return &pod
}

func podResource(rel *spec.Release, gitSha *koliutil.SHA, env map[string]interface{}) api.Pod {
	sbPodName := fmt.Sprintf("sb-%s", rel.Name)
	pod := api.Pod{
		Spec: api.PodSpec{
			RestartPolicy: api.RestartPolicyNever,
			Containers: []api.Container{
				{
					ImagePullPolicy: api.PullIfNotPresent,
				},
			},
		},
		ObjectMeta: api.ObjectMeta{
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
				pod.Spec.Containers[i].Env = append(pod.Spec.Containers[i].Env, api.EnvVar{
					Name:  k,
					Value: fmt.Sprintf("%v", v),
				})
			}
		}
	}
	return pod
}

func addEnvToContainer(pod api.Pod, key, value string, index int) {
	if len(pod.Spec.Containers) > 0 {
		pod.Spec.Containers[index].Env = append(pod.Spec.Containers[index].Env, api.EnvVar{
			Name:  key,
			Value: value,
		})
	}
}
