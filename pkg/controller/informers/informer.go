package informers

import (
	"reflect"
	"sync"
	"time"

	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/tools/cache"
)

// SharedInformerFactory provides interface which holds unique informers for pods, nodes, namespaces, persistent volume
// claims and persistent volumes
type SharedInformerFactory interface {
	// Start starts informers that can start AFTER the API server and controllers have started
	Start(stopCh <-chan struct{})

	Addons() AddonInformer
	PetSets() PetSetInformer
	Namespaces() NamespaceInformer
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

// Addons returns a SharedIndexInformer that lists and watches all addons
func (f *sharedInformerFactory) Addons() AddonInformer {
	return &addonInformer{sharedInformerFactory: f}
}

// PetSets returns a SharedIndexInformer that lists and watchs all petsets
func (f *sharedInformerFactory) PetSets() PetSetInformer {
	return &petSetInformer{sharedInformerFactory: f}
}

// Namespaces returns a SharedIndexInformer that lists and watchs all namespaces
func (f *sharedInformerFactory) Namespaces() NamespaceInformer {
	return &namespaceInformer{sharedInformerFactory: f}
}
