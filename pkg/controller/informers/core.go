package informers

import (
	"encoding/json"
	"reflect"

	"github.com/kolibox/koli/pkg/spec"
	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/pkg/apis/apps/v1alpha1"
	"k8s.io/client-go/1.5/pkg/runtime"
	"k8s.io/client-go/1.5/pkg/watch"
	"k8s.io/client-go/1.5/rest"
	"k8s.io/client-go/1.5/tools/cache"
)

type sysDecoder struct {
	dec   *json.Decoder
	close func() error
}

func (d *sysDecoder) Close() {
	d.close()
}

func (d *sysDecoder) Decode() (action watch.EventType, object runtime.Object, err error) {
	var e struct {
		Type   watch.EventType
		Object spec.Addon
	}
	if err := d.dec.Decode(&e); err != nil {
		return watch.Error, nil, err
	}
	return e.Type, &e.Object, nil
}

// AddonInformer is a type of SharedIndexInformer which watches and lists all addons.
type AddonInformer interface {
	Informer(client *rest.RESTClient) cache.SharedIndexInformer
	// Lister() *cache.ListWatch
}

type addonInformer struct {
	*sharedInformerFactory
}

func (f *addonInformer) Informer(client *rest.RESTClient) cache.SharedIndexInformer {
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
				req := client.Get().
					Namespace(api.NamespaceAll).
					Resource("addons").
					// VersionedParams(&options, api.ParameterCodec)
					FieldsSelectorParam(nil)

				b, err := req.DoRaw()
				if err != nil {
					return nil, err
				}
				var p spec.AddonList
				return &p, json.Unmarshal(b, &p)
			},
			WatchFunc: func(options api.ListOptions) (watch.Interface, error) {
				r, err := client.Get().
					Prefix("watch").
					Namespace(api.NamespaceAll).
					Resource("addons").
					// VersionedParams(&options, api.ParameterCodec).
					FieldsSelectorParam(nil).
					Stream()
				if err != nil {
					return nil, err
				}
				return watch.NewStreamWatcher(&sysDecoder{
					dec:   json.NewDecoder(r),
					close: r.Close,
				}), nil
			},
		},
		&spec.Addon{},
		f.defaultResync,
		cache.Indexers{},
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

	informerType := reflect.TypeOf(&v1alpha1.PetSet{})
	informer, exists := f.informers[informerType]
	if exists {
		return informer
	}

	informer = cache.NewSharedIndexInformer(
		cache.NewListWatchFromClient(f.client.Apps().GetRESTClient(), "petsets", api.NamespaceAll, nil),
		&v1alpha1.PetSet{}, f.defaultResync, cache.Indexers{},
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
		cache.NewListWatchFromClient(f.client.Core().GetRESTClient(), "namespaces", api.NamespaceAll, nil),
		&v1.Namespace{}, f.defaultResync, cache.Indexers{},
	)
	f.informers[informerType] = informer
	return informer
}
