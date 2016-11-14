package addon

import (
	"encoding/json"
	"time"

	"github.com/kolibox/koli/pkg/spec"
	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/api/unversioned"
	"k8s.io/client-go/1.5/pkg/runtime"
	"k8s.io/client-go/1.5/pkg/runtime/serializer"
	"k8s.io/client-go/1.5/pkg/watch"
	"k8s.io/client-go/1.5/rest"
	"k8s.io/client-go/1.5/tools/cache"
)

// The period that should be used to re-sync the monitored resource
const resyncPeriod = 30 * time.Second

func newSysRESTClient(c rest.Config) (*rest.RESTClient, error) {
	c.APIPath = "/apis"
	c.GroupVersion = &unversioned.GroupVersion{
		Group:   "sys.koli.io",
		Version: "v1alpha1",
	}
	c.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: api.Codecs}
	return rest.RESTClientFor(&c)
}

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

// NewSysListWatch returns a new ListWatch on System resources.
func NewSysListWatch(client *rest.RESTClient) *cache.ListWatch {
	return &cache.ListWatch{
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
	}
}
