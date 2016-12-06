package clientset

import (
	"encoding/json"
	"errors"

	"github.com/kolibox/koli/pkg/spec"
	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/api/unversioned"
	"k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/pkg/runtime"
	"k8s.io/client-go/1.5/pkg/watch"
	"k8s.io/client-go/1.5/rest"
)

// AddonGetter has a method to return an AddonInterface.
// A group's client should implement this interface.
type AddonGetter interface {
	Addon(namespace string) AddonInterface
}

// AddonInterface has methods to work with Addon resources.
type AddonInterface interface {
	List(opts *api.ListOptions) (*spec.AddonList, error)
	Get(name string) (*spec.Addon, error)
	Delete(name string, options *v1.DeleteOptions) error
	Create(data *spec.Addon) (*spec.Addon, error)
	Update(data *spec.Addon) (*spec.Addon, error)
	Watch(opts *api.ListOptions) (watch.Interface, error)
}

// addon implements AddonInterface
type addon struct {
	client    *rest.RESTClient
	namespace string
	resource  *unversioned.APIResource
}

// Get gets the resource with the specified name.
func (a *addon) Get(name string) (*spec.Addon, error) {
	addon := &spec.Addon{}
	err := a.client.Get().
		NamespaceIfScoped(a.namespace, a.resource.Namespaced).
		Resource(a.resource.Name).
		Name(name).
		Do().
		Into(addon)
	return addon, err
}

// List returns a list of objects for this resource.
func (a *addon) List(opts *api.ListOptions) (*spec.AddonList, error) {
	if opts == nil {
		opts = &api.ListOptions{}
	}
	addonList := &spec.AddonList{}
	err := a.client.Get().
		NamespaceIfScoped(a.namespace, a.resource.Namespaced).
		Resource(a.resource.Name).
		FieldsSelectorParam(nil).
		VersionedParams(opts, api.ParameterCodec). // TODO: test this option
		Do().
		Into(addonList)
	return addonList, err
}

// Delete deletes the resource with the specified name.
func (a *addon) Delete(name string, opts *v1.DeleteOptions) error {
	if opts == nil {
		opts = &v1.DeleteOptions{}
	}
	return a.client.Delete().
		NamespaceIfScoped(a.namespace, a.resource.Namespaced).
		Resource(a.resource.Name).
		Name(name).
		// TODO: https://github.com/kubernetes/kubernetes/issues/37278
		// error: no kind "DeleteOptions" is registered for version "<3PR>"
		// Body(opts).
		Do().
		Error()
}

// Create creates the provided resource.
func (a *addon) Create(data *spec.Addon) (*spec.Addon, error) {
	addon := &spec.Addon{}
	err := a.client.Post().
		NamespaceIfScoped(a.namespace, a.resource.Namespaced).
		Resource(a.resource.Name).
		Body(data).
		Do().
		Into(addon)
	return addon, err
}

// Update updates the provided resource.
func (a *addon) Update(data *spec.Addon) (*spec.Addon, error) {
	addon := &spec.Addon{}
	if len(data.GetName()) == 0 {
		return data, errors.New("object missing name")
	}
	err := a.client.Put().
		NamespaceIfScoped(a.namespace, a.resource.Namespaced).
		Resource(a.resource.Name).
		Name(data.GetName()).
		Body(data).
		Do().
		Into(addon)
	return addon, err
}

// Watch returns a watch.Interface that watches the resource.
func (a *addon) Watch(opts *api.ListOptions) (watch.Interface, error) {
	// TODO: Using Watch method gives the following error on creation and deletion of resources:
	// expected type X, but watch event object had type *runtime.Unstructured
	stream, err := a.client.Get().
		Prefix("watch").
		NamespaceIfScoped(a.namespace, a.resource.Namespaced).
		Resource(a.resource.Name).
		// VersionedParams(opts, spec.DefaultParameterEncoder).
		VersionedParams(opts, api.ParameterCodec).
		Stream()
	if err != nil {
		return nil, err
	}

	return watch.NewStreamWatcher(&addonDecoder{
		dec:   json.NewDecoder(stream),
		close: stream.Close,
	}), nil
}

// Patch updates the provided resource
func (a *addon) Patch(name string, pt api.PatchType, data []byte) (*spec.Addon, error) {
	addon := &spec.Addon{}
	err := a.client.Patch(pt).
		NamespaceIfScoped(a.namespace, a.resource.Namespaced).
		Resource(a.resource.Name).
		Name(name).
		Body(data).
		Do().
		Into(addon)
	return addon, err
}

// addonDecoder provides a decoder for watching addon resources
type addonDecoder struct {
	dec   *json.Decoder
	close func() error
}

// Close decoder
func (d *addonDecoder) Close() {
	d.close()
}

// Decode data
func (d *addonDecoder) Decode() (watch.EventType, runtime.Object, error) {
	var e struct {
		Type   watch.EventType
		Object spec.Addon
	}
	if err := d.dec.Decode(&e); err != nil {
		return watch.Error, nil, err
	}
	return e.Type, &e.Object, nil
}
