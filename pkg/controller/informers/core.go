package informers

import (
	"reflect"

	"github.com/kolibox/koli/pkg/clientset"
	"github.com/kolibox/koli/pkg/spec"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/cache"
	"k8s.io/kubernetes/pkg/runtime"
	"k8s.io/kubernetes/pkg/watch"

	apps "k8s.io/kubernetes/pkg/apis/apps"
	extensions "k8s.io/kubernetes/pkg/apis/extensions"
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

	informerType := reflect.TypeOf(&spec.Addon{})
	informer, exists := f.informers[informerType]
	if exists {
		return informer
	}

	informer = cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options api.ListOptions) (runtime.Object, error) {
				return sysClient.Addon(api.NamespaceAll).List(&options)
			},
			WatchFunc: func(options api.ListOptions) (watch.Interface, error) {
				return sysClient.Addon(api.NamespaceAll).Watch(&options)
			},
		},
		&spec.Addon{},
		f.defaultResync,
		cache.Indexers{},
	)
	f.informers[informerType] = informer
	return informer
}

// ServicePlanInformer is a type of SharedIndexInformer which watches and lists all service plans.
type ServicePlanInformer interface {
	Informer(sysClient *clientset.CoreClient) cache.SharedIndexInformer
}

type servicePlanInformer struct {
	*sharedInformerFactory
}

func (f *servicePlanInformer) Informer(sysClient *clientset.CoreClient) cache.SharedIndexInformer {
	f.lock.Lock()
	defer f.lock.Unlock()

	informerType := reflect.TypeOf(&spec.ServicePlan{})
	informer, exists := f.informers[informerType]
	if exists {
		return informer
	}

	informer = cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options api.ListOptions) (runtime.Object, error) {
				return sysClient.ServicePlan(api.NamespaceAll).List(&options)
			},
			WatchFunc: func(options api.ListOptions) (watch.Interface, error) {
				return sysClient.ServicePlan(api.NamespaceAll).Watch(&options)
			},
		},
		&spec.ServicePlan{},
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
		cache.NewListWatchFromClient(f.client.Extensions().RESTClient(), "deployments", api.NamespaceAll, nil),
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

	informerType := reflect.TypeOf(&apps.StatefulSet{})
	informer, exists := f.informers[informerType]
	if exists {
		return informer
	}

	informer = cache.NewSharedIndexInformer(
		cache.NewListWatchFromClient(f.client.Apps().RESTClient(), "statefulsets", api.NamespaceAll, nil),
		&apps.StatefulSet{}, f.defaultResync, cache.Indexers{},
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

	informerType := reflect.TypeOf(&api.Namespace{})
	informer, exists := f.informers[informerType]
	if exists {
		return informer
	}

	informer = cache.NewSharedIndexInformer(
		cache.NewListWatchFromClient(f.client.Core().RESTClient(), "namespaces", api.NamespaceAll, nil),
		&api.Namespace{}, f.defaultResync, cache.Indexers{},
	)
	f.informers[informerType] = informer
	return informer
}
