package clientset

import (
	"encoding/json"
	"errors"

	platform "kolihub.io/koli/pkg/apis/core/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
)

// ReleaseGetter has a method to return an ReleaseInterface.
// A group's client should implement this interface.
type ReleaseGetter interface {
	Release(namespace string) ReleaseInterface
}

// ReleaseInterface has methods to work with Release resources.
type ReleaseInterface interface {
	List(opts *metav1.ListOptions) (*platform.ReleaseList, error)
	Get(name string) (*platform.Release, error)
	Delete(name string, options *metav1.DeleteOptions) error
	Create(data *platform.Release) (*platform.Release, error)
	Update(data *platform.Release) (*platform.Release, error)
	Watch(opts *metav1.ListOptions) (watch.Interface, error)
	Patch(name string, pt types.PatchType, data []byte, subresources ...string) (*platform.Release, error)
}

// release implements ReleaseInterface
type release struct {
	client    rest.Interface
	namespace string
	resource  *metav1.APIResource
}

// Get gets the resource with the specified name.
func (r *release) Get(name string) (*platform.Release, error) {
	release := &platform.Release{}
	err := r.client.Get().
		NamespaceIfScoped(r.namespace, r.resource.Namespaced).
		Resource(r.resource.Name).
		Name(name).
		Do().
		Into(release)
	return release, err
}

// List returns a list of objects for this resource.
func (r *release) List(opts *metav1.ListOptions) (*platform.ReleaseList, error) {
	releaseList := &platform.ReleaseList{}
	err := r.client.Get().
		NamespaceIfScoped(r.namespace, r.resource.Namespaced).
		Resource(r.resource.Name).
		VersionedParams(opts, metav1.ParameterCodec). // TODO: test this option
		Do().
		Into(releaseList)
	return releaseList, err
}

// Delete deletes the resource with the specified name.
func (r *release) Delete(name string, opts *metav1.DeleteOptions) error {
	return r.client.Delete().
		NamespaceIfScoped(r.namespace, r.resource.Namespaced).
		Resource(r.resource.Name).
		Name(name).
		Body(opts).
		Do().
		Error()
}

// Create creates the provided resource.
func (r *release) Create(data *platform.Release) (*platform.Release, error) {
	release := &platform.Release{}
	err := r.client.Post().
		NamespaceIfScoped(r.namespace, r.resource.Namespaced).
		Resource(r.resource.Name).
		Body(data).
		Do().
		Into(release)
	return release, err
}

// Update updates the provided resource.
func (r *release) Update(data *platform.Release) (*platform.Release, error) {
	release := &platform.Release{}
	if len(data.GetName()) == 0 {
		return data, errors.New("object missing name")
	}
	err := r.client.Put().
		NamespaceIfScoped(r.namespace, r.resource.Namespaced).
		Resource(r.resource.Name).
		Name(data.GetName()).
		Body(data).
		Do().
		Into(release)
	return release, err
}

// Watch returns a watch.Interface that watches the resource.
func (r *release) Watch(opts *metav1.ListOptions) (watch.Interface, error) {
	// TODO: Using Watch method gives the following error on creation and deletion of resources:
	// expected type X, but watch event object had type *runtime.Unstructured
	stream, err := r.client.Get().
		Prefix("watch").
		NamespaceIfScoped(r.namespace, r.resource.Namespaced).
		Resource(r.resource.Name).
		// VersionedParams(opts, platform.DefaultParameterEncoder).
		VersionedParams(opts, metav1.ParameterCodec).
		Stream()
	if err != nil {
		return nil, err
	}

	return watch.NewStreamWatcher(&releaseDecoder{
		dec:   json.NewDecoder(stream),
		close: stream.Close,
	}), nil
}

// Patch applies the patch and returns the patched release.
func (r *release) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (*platform.Release, error) {
	release := &platform.Release{}
	err := r.client.Patch(pt).
		Namespace(r.namespace).
		Resource(r.resource.Name).
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(release)
	return release, err
}

// releaseDecoder provides a decoder for watching release resources
type releaseDecoder struct {
	dec   *json.Decoder
	close func() error
}

// Close decoder
func (d *releaseDecoder) Close() {
	d.close()
}

// Decode data
func (d *releaseDecoder) Decode() (watch.EventType, runtime.Object, error) {
	var e struct {
		Type   watch.EventType
		Object platform.Release
	}
	if err := d.dec.Decode(&e); err != nil {
		return watch.Error, nil, err
	}
	return e.Type, &e.Object, nil
}
