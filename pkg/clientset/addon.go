package clientset

// import (
// 	"encoding/json"
// 	"errors"

// 	platform "kolihub.io/koli/pkg/apis/core/v1alpha1"

// 	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
// 	"k8s.io/apimachinery/pkg/runtime"
// 	"k8s.io/apimachinery/pkg/types"
// 	"k8s.io/apimachinery/pkg/watch"
// 	"k8s.io/client-go/rest"
// )

// // AddonGetter has a method to return an AddonInterface.
// // A group's client should implement this interface.
// type AddonGetter interface {
// 	Addon(namespace string) AddonInterface
// }

// // AddonInterface has methods to work with Addon resources.
// type AddonInterface interface {
// 	List(opts *metav1.ListOptions) (*platform.AddonList, error)
// 	Get(name string) (*platform.Addon, error)
// 	Delete(name string, options *metav1.DeleteOptions) error
// 	Create(data *platform.Addon) (*platform.Addon, error)
// 	Update(data *platform.Addon) (*platform.Addon, error)
// 	Watch(opts *metav1.ListOptions) (watch.Interface, error)
// }

// // addon implements AddonInterface
// type addon struct {
// 	client    rest.Interface
// 	namespace string
// 	resource  *metav1.APIResource
// }

// // Get gets the resource with the specified name.
// func (a *addon) Get(name string) (*platform.Addon, error) {
// 	addon := &platform.Addon{}
// 	err := a.client.Get().
// 		NamespaceIfScoped(a.namespace, a.resource.Namespaced).
// 		Resource(a.resource.Name).
// 		Name(name).
// 		Do().
// 		Into(addon)
// 	return addon, err
// }

// // List returns a list of objects for this resource.
// func (a *addon) List(opts *metav1.ListOptions) (*platform.AddonList, error) {
// 	addonList := &platform.AddonList{}
// 	err := a.client.Get().
// 		NamespaceIfScoped(a.namespace, a.resource.Namespaced).
// 		Resource(a.resource.Name).
// 		FieldsSelectorParam(nil).
// 		// VersionedParams(opts, scheme.ParameterCodec). // TODO: test this option
// 		Do().
// 		Into(addonList)
// 	return addonList, err
// }

// // Delete deletes the resource with the specified name.
// func (a *addon) Delete(name string, opts *metav1.DeleteOptions) error {
// 	return a.client.Delete().
// 		NamespaceIfScoped(a.namespace, a.resource.Namespaced).
// 		Resource(a.resource.Name).
// 		Name(name).
// 		// TODO: https://github.com/kubernetes/kubernetes/issues/37278
// 		// error: no kind "DeleteOptions" is registered for version "<3PR>"
// 		// Body(opts).
// 		Do().
// 		Error()
// }

// // Create creates the provided resource.
// func (a *addon) Create(data *platform.Addon) (*platform.Addon, error) {
// 	addon := &platform.Addon{}
// 	err := a.client.Post().
// 		NamespaceIfScoped(a.namespace, a.resource.Namespaced).
// 		Resource(a.resource.Name).
// 		Body(data).
// 		Do().
// 		Into(addon)
// 	return addon, err
// }

// // Update updates the provided resource.
// func (a *addon) Update(data *platform.Addon) (*platform.Addon, error) {
// 	addon := &platform.Addon{}
// 	if len(data.GetName()) == 0 {
// 		return data, errors.New("object missing name")
// 	}
// 	err := a.client.Put().
// 		NamespaceIfScoped(a.namespace, a.resource.Namespaced).
// 		Resource(a.resource.Name).
// 		Name(data.GetName()).
// 		Body(data).
// 		Do().
// 		Into(addon)
// 	return addon, err
// }

// // Watch returns a watch.Interface that watches the resource.
// func (a *addon) Watch(opts *metav1.ListOptions) (watch.Interface, error) {
// 	// TODO: Using Watch method gives the following error on creation and deletion of resources:
// 	// expected type X, but watch event object had type *runtime.Unstructured
// 	stream, err := a.client.Get().
// 		Prefix("watch").
// 		NamespaceIfScoped(a.namespace, a.resource.Namespaced).
// 		Resource(a.resource.Name).
// 		VersionedParams(opts, metav1.ParameterCodec).
// 		Stream()
// 	if err != nil {
// 		return nil, err
// 	}

// 	return watch.NewStreamWatcher(&addonDecoder{
// 		dec:   json.NewDecoder(stream),
// 		close: stream.Close,
// 	}), nil
// }

// // Patch updates the provided resource
// func (a *addon) Patch(name string, pt types.PatchType, data []byte) (*platform.Addon, error) {
// 	addon := &platform.Addon{}
// 	err := a.client.Patch(pt).
// 		NamespaceIfScoped(a.namespace, a.resource.Namespaced).
// 		Resource(a.resource.Name).
// 		Name(name).
// 		Body(data).
// 		Do().
// 		Into(addon)
// 	return addon, err
// }

// // addonDecoder provides a decoder for watching addon resources
// type addonDecoder struct {
// 	dec   *json.Decoder
// 	close func() error
// }

// // Close decoder
// func (d *addonDecoder) Close() {
// 	d.close()
// }

// // Decode data
// func (d *addonDecoder) Decode() (watch.EventType, runtime.Object, error) {
// 	var e struct {
// 		Type   watch.EventType
// 		Object platform.Addon
// 	}
// 	if err := d.dec.Decode(&e); err != nil {
// 		return watch.Error, nil, err
// 	}
// 	return e.Type, &e.Object, nil
// }
