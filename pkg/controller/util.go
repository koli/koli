package controller

import (
	"fmt"
	"time"

	"github.com/golang/glog"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"

	extensions "k8s.io/api/extensions/v1beta1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"

	platform "kolihub.io/koli/pkg/apis/core/v1alpha1"
)

const (
	tprServicePlan = "plan.platform.koli.io"
	tprAddons      = "addon.platform.koli.io"
	tprReleases    = "release.platform.koli.io"
)

var keyFunc = cache.DeletionHandlingMetaNamespaceKeyFunc

func watch3PRs(host, endpoint string, kclient kubernetes.Interface) error {
	return wait.Poll(1*time.Second, 30*time.Second, func() (bool, error) {
		_, err := kclient.Extensions().ThirdPartyResources().Get(host+endpoint, metav1.GetOptions{})
		// resp, err := kclient.Core().RESTClient().Get(host + endpoint)
		if err != nil {
			return false, err
		}
		return true, nil
	})
}

// CreatePlatformRoles initialize the needed roles for the platform
// func CreatePlatformRoles(kclient kubernetes.Interface) {
// 	for _, role := range platform.GetRoles() {
// 		if _, err := kclient.Rbac().ClusterRoles().Create(role); err != nil && !apierrors.IsAlreadyExists(err) {
// 			panic(err)
// 		}
// 		glog.Infof("provisioned role %s", role.Name)
// 	}
// }

func newRecorder(client kubernetes.Interface, component string) record.EventRecorder {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(glog.Infof)
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{
		Interface: v1core.New(client.Core().RESTClient()).Events(""),
	})
	return eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: component})
}

// TaskQueue manages a work queue through an independent worker that
// invokes the given sync function for every work item inserted.
type TaskQueue struct {
	// queue is the work queue the worker polls
	queue workqueue.RateLimitingInterface
	// sync is called for each item in the queue
	sync func(string) error
	// workerDone is closed when the worker exits
	workerDone chan struct{}
}

func (t *TaskQueue) run(period time.Duration, stopCh <-chan struct{}) {
	wait.Until(t.runWorker, period, stopCh)
}

// Len retrieves the lenght of the queue
func (t *TaskQueue) Len() int { return t.queue.Len() }

// Add enqueues ns/name of the given api object in the task queue.
func (t *TaskQueue) Add(obj interface{}) {
	key, err := keyFunc(obj)
	if err != nil {
		glog.Infof("Couldn't get key for object %+v: %v", obj, err)
		return
	}
	t.queue.Add(key)
}

func (t *TaskQueue) runWorker() {
	for {
		// hot loop until we're told to stop.  processNextWorkItem will automatically
		// wait until there's work available, so we don't worry about secondary waits
		t.processNextWorkItem()
	}
}

// worker processes work in the queue through sync.
func (t *TaskQueue) processNextWorkItem() {
	key, quit := t.queue.Get()
	if quit {
		close(t.workerDone)
		return
	}
	if key == nil {
		return
	}
	glog.V(3).Infof("Syncing %v", key)
	if err := t.sync(key.(string)); err != nil {
		glog.Errorf("Requeuing %v, err: %v", key, err)
		t.queue.AddRateLimited(key)
	} else {
		t.queue.Forget(key)
	}
	t.queue.Done(key)
}

// shutdown shuts down the work queue and waits for the worker to ACK
func (t *TaskQueue) shutdown() {
	t.queue.ShutDown()
	<-t.workerDone
}

// NewTaskQueue creates a new task queue with the given sync function.
// The sync function is called for every element inserted into the queue.
func NewTaskQueue(syncFn func(string) error) *TaskQueue {
	return &TaskQueue{
		queue:      workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
		sync:       syncFn,
		workerDone: make(chan struct{}),
	}
}

// CreateCRD provision the Custom Resource Definition and
// wait until the API is ready to interact it with
func CreateCRD(clientset apiextensionsclient.Interface) error {
	for _, r := range []struct {
		name   string
		kind   string
		plural string
	}{
		{
			platform.PlanResourceName,
			platform.PlanResourceKind,
			platform.PlanResourcePlural,
		},
		{
			platform.ReleaseResourceName,
			platform.ReleaseResourceKind,
			platform.ReleaseResourcePlural,
		},
	} {
		crd := &apiextensionsv1beta1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name: r.name,
			},
			Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
				Group:   platform.SchemeGroupVersion.Group,
				Version: platform.SchemeGroupVersion.Version,
				Scope:   apiextensionsv1beta1.NamespaceScoped,
				Names: apiextensionsv1beta1.CustomResourceDefinitionNames{
					Plural: r.plural,
					Kind:   r.kind,
					// ShortNames: []string{""},
				},
			},
		}
		_, err := clientset.ApiextensionsV1beta1().CustomResourceDefinitions().Create(crd)
		if err == nil || apierrors.IsAlreadyExists(err) {
			glog.Infof("Custom Resource Definiton '%s' provisioned, waiting to be ready ...", r.name)
			if err := waitCRDReady(clientset, r.name); err != nil {
				return err
			}
			continue
		}
		return err
	}
	return nil
}

func waitCRDReady(clientset apiextensionsclient.Interface, resourceName string) error {
	return wait.Poll(1*time.Second, 30*time.Second, func() (bool, error) {
		crd, err := clientset.
			ApiextensionsV1beta1().
			CustomResourceDefinitions().
			Get(resourceName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		for _, cond := range crd.Status.Conditions {
			switch cond.Type {
			case apiextensionsv1beta1.Established:
				if cond.Status == apiextensionsv1beta1.ConditionTrue {
					return true, nil
				}
			case apiextensionsv1beta1.NamesAccepted:
				if cond.Status == apiextensionsv1beta1.ConditionFalse {
					return false, fmt.Errorf("Name conflict: %v", cond.Reason)
				}
			}
		}
		return false, nil
	})
}

// CreatePlan3PRs generates the third party resource required for interacting with Service Plans
func CreatePlan3PRs(host string, kclient kubernetes.Interface) error {
	tprs := []*extensions.ThirdPartyResource{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: tprServicePlan,
			},
			Versions: []extensions.APIVersion{
				{Name: "v1"},
			},
			Description: "Plan resource aggregation",
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
	return watch3PRs(host, "/apis/platform.koli.io/v1/plans", kclient)
}

// CreateAddonTPRs generates the third party resource required for interacting with addons
func CreateAddonTPRs(host string, kclient kubernetes.Interface) error {
	tprs := []*extensions.ThirdPartyResource{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: tprAddons,
			},
			Versions: []extensions.APIVersion{
				{Name: "v1"},
			},
			Description: "Addon external service integration",
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
	return watch3PRs(host, "/apis/platform.koli.io/v1/addons", kclient)
}

// CreateReleaseTPRs generates the third party resource required for interacting with releases
func CreateReleaseTPRs(host string, kclient kubernetes.Interface) error {
	tprs := []*extensions.ThirdPartyResource{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: tprReleases,
			},
			Versions: []extensions.APIVersion{
				{Name: "v1"},
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
	return watch3PRs(host, "/apis/platform.koli.io/v1/releases", kclient)
}
