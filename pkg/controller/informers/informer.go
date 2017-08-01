package informers

import (
	"reflect"
	"sync"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// SharedInformerFactory provides interface which holds unique informers for pods, nodes, namespaces, persistent volume
// claims and persistent volumes
type SharedInformerFactory interface {
	// Start starts informers that can start AFTER the API server and controllers have started
	Start(stopCh <-chan struct{})
	WaitForCacheSync(stopCh <-chan struct{}) map[reflect.Type]bool

	Addons() AddonInformer
	ServicePlans() ServicePlanInformer
	Releases() ReleaseInformer
	PetSets() PetSetInformer
	Namespaces() NamespaceInformer
	Deployments() DeploymentInformer
	Pods() PodInformer
	Secrets() SecretInformer
}

type sharedInformerFactory struct {
	client        kubernetes.Interface
	lock          sync.Mutex
	defaultResync time.Duration

	informers map[reflect.Type]cache.SharedIndexInformer
	// startedInformers is used for tracking which informers have been started
	// this allows calling of Start method multiple times
	startedInformers map[reflect.Type]bool
}

// NewSharedInformerFactory constructs a new instance of sharedInformerFactory
func NewSharedInformerFactory(client kubernetes.Interface, defaultResync time.Duration) SharedInformerFactory {
	return &sharedInformerFactory{
		client:           client,
		defaultResync:    defaultResync,
		informers:        make(map[reflect.Type]cache.SharedIndexInformer),
		startedInformers: make(map[reflect.Type]bool),
	}
}

// Start initializes all requested informers.
func (f *sharedInformerFactory) Start(stopCh <-chan struct{}) {
	f.lock.Lock()
	defer f.lock.Unlock()

	for informerType, informer := range f.informers {
		if !f.startedInformers[informerType] {
			go informer.Run(stopCh)
			f.startedInformers[informerType] = true
		}
	}
}

// WaitForCacheSync waits for all started informers' cache were synced.
func (f *sharedInformerFactory) WaitForCacheSync(stopCh <-chan struct{}) map[reflect.Type]bool {
	informers := func() map[reflect.Type]cache.SharedIndexInformer {
		f.lock.Lock()
		defer f.lock.Unlock()

		informers := map[reflect.Type]cache.SharedIndexInformer{}
		for informerType, informer := range f.informers {
			if f.startedInformers[informerType] {
				informers[informerType] = informer
			}
		}
		return informers
	}()

	res := map[reflect.Type]bool{}
	for informType, informer := range informers {
		res[informType] = cache.WaitForCacheSync(stopCh, informer.HasSynced)
	}
	return res
}

// Addons returns a SharedIndexInformer that lists and watches all addons
func (f *sharedInformerFactory) Addons() AddonInformer {
	return &addonInformer{sharedInformerFactory: f}
}

// ServicePlans returns a SharedIndexInformer that lists and watches all service plans
func (f *sharedInformerFactory) ServicePlans() ServicePlanInformer {
	return &servicePlanInformer{sharedInformerFactory: f}
}

// Plan returns a SharedIndexInformer that lists and watches all service plans
func (f *sharedInformerFactory) Plan() ServicePlanInformer {
	return &servicePlanInformer{sharedInformerFactory: f}
}

// Releases returns a SharedIndexInformer that lists and watchs all releases
func (f *sharedInformerFactory) Releases() ReleaseInformer {
	return &releaseInformer{sharedInformerFactory: f}
}

// Deployments returns a SharedIndexInformer that lists and watchs its resources
func (f *sharedInformerFactory) Deployments() DeploymentInformer {
	return &deploymentInformer{sharedInformerFactory: f}
}

// PetSets returns a SharedIndexInformer that lists and watchs all petsets
func (f *sharedInformerFactory) PetSets() PetSetInformer {
	return &petSetInformer{sharedInformerFactory: f}
}

// Namespaces returns a SharedIndexInformer that lists and watchs all namespaces
func (f *sharedInformerFactory) Namespaces() NamespaceInformer {
	return &namespaceInformer{sharedInformerFactory: f}
}

// Pods returns a SharedIndexInformer that lists and watchs all pods
func (f *sharedInformerFactory) Pods() PodInformer {
	return &podInformer{sharedInformerFactory: f}
}

// Secrets returns a SharedIndexInformer that lists and watchs all secrets
func (f *sharedInformerFactory) Secrets() SecretInformer {
	return &secretInformer{sharedInformerFactory: f}
}
