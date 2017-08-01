package informers

import (
	"fmt"
	"reflect"

	platform "kolihub.io/koli/pkg/apis/v1alpha1"
	"kolihub.io/koli/pkg/clientset"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/tools/cache"

	v1beta1 "k8s.io/client-go/pkg/apis/apps/v1beta1"
	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

// AddonInformer is a type of SharedIndexInformer which watches and lists all addons.
type AddonInformer interface {
	Informer(client *clientset.CoreClient) cache.SharedIndexInformer
	// Lister() *cache.ListWatch
}

type addonInformer struct {
	*sharedInformerFactory
}

func (f *addonInformer) Informer(sysClient *clientset.CoreClient) cache.SharedIndexInformer {
	f.lock.Lock()
	defer f.lock.Unlock()

	informerType := reflect.TypeOf(&platform.Addon{})
	informer, exists := f.informers[informerType]
	if exists {
		return informer
	}

	informer = cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return sysClient.Addon(metav1.NamespaceAll).List(&options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return sysClient.Addon(metav1.NamespaceAll).Watch(&options)
			},
		},
		&platform.Addon{},
		f.defaultResync,
		cache.Indexers{},
	)
	f.informers[informerType] = informer
	return informer
}

// ServicePlanInformer is a type of SharedIndexInformer which watches and lists all service plans
type ServicePlanInformer interface {
	Informer(tprClient clientset.CoreInterface) cache.SharedIndexInformer
}

type servicePlanInformer struct {
	*sharedInformerFactory
}

func (f *servicePlanInformer) Informer(tprClient clientset.CoreInterface) cache.SharedIndexInformer {
	f.lock.Lock()
	defer f.lock.Unlock()

	informerType := reflect.TypeOf(&platform.Plan{})
	informer, exists := f.informers[informerType]
	if exists {
		return informer
	}

	informer = cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return tprClient.ServicePlan(metav1.NamespaceAll).List(&options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return tprClient.ServicePlan(metav1.NamespaceAll).Watch(&options)
			},
		},
		&platform.Plan{},
		f.defaultResync,
		cache.Indexers{},
	)
	f.informers[informerType] = informer
	return informer
}

// ReleaseInformer is a type of SharedIndexInformer which watches and lists all releases.
type ReleaseInformer interface {
	Informer(client *clientset.CoreClient) cache.SharedIndexInformer
	// Lister() *cache.ListWatch
}

type releaseInformer struct {
	*sharedInformerFactory
}

func (f *releaseInformer) Informer(sysClient *clientset.CoreClient) cache.SharedIndexInformer {
	f.lock.Lock()
	defer f.lock.Unlock()

	informerType := reflect.TypeOf(&platform.Release{})
	informer, exists := f.informers[informerType]
	if exists {
		return informer
	}

	informer = cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return sysClient.Release(metav1.NamespaceAll).List(&options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return sysClient.Release(metav1.NamespaceAll).Watch(&options)
			},
		},
		&platform.Release{},
		f.defaultResync,
		cache.Indexers{},
	)
	f.informers[informerType] = informer
	return informer
}

// DeploymentInformer is a type of SharedIndexInformer which watches and lists all its resources
type DeploymentInformer interface {
	Informer() cache.SharedIndexInformer
	// Lister() *cache.ListWatch
}

type deploymentInformer struct {
	*sharedInformerFactory
}

func (f *deploymentInformer) Informer() cache.SharedIndexInformer {
	f.lock.Lock()
	defer f.lock.Unlock()

	informerType := reflect.TypeOf(&extensions.Deployment{})
	informer, exists := f.informers[informerType]
	if exists {
		return informer
	}

	informer = cache.NewSharedIndexInformer(
		cache.NewListWatchFromClient(f.client.Extensions().RESTClient(), "deployments", metav1.NamespaceAll, nil),
		&extensions.Deployment{}, f.defaultResync, cache.Indexers{},
	)
	f.informers[informerType] = informer
	return informer
}

// PetSetInformer is a type of SharedIndexInformer which watches and lists all PetSets.
type PetSetInformer interface {
	Informer() cache.SharedIndexInformer
	// Lister() *cache.ListWatch
}

type petSetInformer struct {
	*sharedInformerFactory
}

func (f *petSetInformer) Informer() cache.SharedIndexInformer {
	f.lock.Lock()
	defer f.lock.Unlock()

	informerType := reflect.TypeOf(&v1beta1.StatefulSet{})
	informer, exists := f.informers[informerType]
	if exists {
		return informer
	}

	informer = cache.NewSharedIndexInformer(
		cache.NewListWatchFromClient(f.client.Apps().RESTClient(), "statefulsets", metav1.NamespaceAll, nil),
		&v1beta1.StatefulSet{}, f.defaultResync, cache.Indexers{},
	)
	f.informers[informerType] = informer
	return informer
}

// NamespaceInformer is a type of SharedIndexInformer which watches and lists all Namespaces.
type NamespaceInformer interface {
	Informer() cache.SharedIndexInformer
	// Lister() *cache.ListWatch
}

type namespaceInformer struct {
	*sharedInformerFactory
}

func (f *namespaceInformer) Informer() cache.SharedIndexInformer {
	f.lock.Lock()
	defer f.lock.Unlock()

	informerType := reflect.TypeOf(&v1.Namespace{})
	informer, exists := f.informers[informerType]
	if exists {
		return informer
	}

	informer = cache.NewSharedIndexInformer(
		cache.NewListWatchFromClient(f.client.Core().RESTClient(), "namespaces", v1.NamespaceAll, nil),
		&v1.Namespace{}, f.defaultResync, cache.Indexers{},
	)
	f.informers[informerType] = informer
	return informer
}

// PodInformer is a type of SharedIndexInformer which watches and lists all its resources
type PodInformer interface {
	Informer(selector labels.Selector) cache.SharedIndexInformer
	// Lister() *cache.ListWatch
}

type podInformer struct {
	*sharedInformerFactory
}

func (f *podInformer) Informer(selector labels.Selector) cache.SharedIndexInformer {
	f.lock.Lock()
	defer f.lock.Unlock()

	informerType := reflect.TypeOf(&v1.Pod{})
	informer, exists := f.informers[informerType]
	if exists {
		return informer
	}

	informer = cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				options.LabelSelector = selector.String()
				return f.client.Core().Pods(metav1.NamespaceAll).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				options.LabelSelector = selector.String()
				return f.client.Core().Pods(metav1.NamespaceAll).Watch(options)
			},
		},
		&v1.Pod{},
		f.defaultResync,
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)
	f.informers[informerType] = informer
	return informer
}

// SecretInformer is a type of SharedIndexInformer which watches and lists all its resources
type SecretInformer interface {
	Informer() cache.SharedIndexInformer
}

type secretInformer struct {
	*sharedInformerFactory
}

func (f *secretInformer) Informer() cache.SharedIndexInformer {
	f.lock.Lock()
	defer f.lock.Unlock()

	informerType := reflect.TypeOf(&v1.Secret{})
	informer, exists := f.informers[informerType]
	if exists {
		return informer
	}

	informer = cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				options.LabelSelector = fmt.Sprintf("%s=true", platform.LabelSecretController)
				return f.client.Core().Secrets(metav1.NamespaceAll).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				options.LabelSelector = fmt.Sprintf("%s=true", platform.LabelSecretController)
				return f.client.Core().Secrets(metav1.NamespaceAll).Watch(options)
			},
		},
		&v1.Secret{},
		f.defaultResync,
		cache.Indexers{},
	)
	f.informers[informerType] = informer
	return informer
}
