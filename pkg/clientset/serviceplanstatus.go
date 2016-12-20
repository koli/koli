package clientset

import (
	"encoding/json"
	"errors"

	"github.com/kolibox/koli/pkg/spec"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/client/restclient"
	"k8s.io/kubernetes/pkg/runtime"
	"k8s.io/kubernetes/pkg/watch"
)

// ServicePlanStatusGetter has a method to return an ServicePlanStatusInterface.
// A group's client should implement this interface.
type ServicePlanStatusGetter interface {
	ServicePlanStatus(namespace string) ServicePlanStatusInterface
}

// ServicePlanStatusInterface has methods to work with ServicePlanStatus resources.
type ServicePlanStatusInterface interface {
	List(opts *api.ListOptions) (*spec.ServicePlanStatusList, error)
	Get(name string) (*spec.ServicePlanStatus, error)
	Delete(name string, options *api.DeleteOptions) error
	Create(data *spec.ServicePlanStatus) (*spec.ServicePlanStatus, error)
	Update(data *spec.ServicePlanStatus) (*spec.ServicePlanStatus, error)
	Watch(opts *api.ListOptions) (watch.Interface, error)
}

// servicePlanStatusks implements ServicePlanStatusInterface
type servicePlanStatus struct {
	client    restclient.Interface
	namespace string
	resource  *unversioned.APIResource
}

// Get gets the resource with the specified name.
func (s *servicePlanStatus) Get(name string) (*spec.ServicePlanStatus, error) {
	sps := &spec.ServicePlanStatus{}
	err := s.client.Get().
		NamespaceIfScoped(s.namespace, s.resource.Namespaced).
		Resource(s.resource.Name).
		Name(name).
		Do().
		Into(sps)
	return sps, err
}

// List returns a list of objects for this resource.
func (s *servicePlanStatus) List(opts *api.ListOptions) (*spec.ServicePlanStatusList, error) {
	if opts == nil {
		opts = &api.ListOptions{}
	}
	spsList := &spec.ServicePlanStatusList{}
	err := s.client.Get().
		NamespaceIfScoped(s.namespace, s.resource.Namespaced).
		Resource(s.resource.Name).
		// VersionedParams(opts, defaultParameterEncoder).
		Do().
		Into(spsList)
	return spsList, err
}

// Delete deletes the resource with the specified name.
func (s *servicePlanStatus) Delete(name string, opts *api.DeleteOptions) error {
	if opts == nil {
		opts = &api.DeleteOptions{}
	}
	return s.client.Delete().
		NamespaceIfScoped(s.namespace, s.resource.Namespaced).
		Resource(s.resource.Name).
		Name(name).
		// TODO: https://github.com/kubernetes/kubernetes/issues/37278
		// error: no kind "DeleteOptions" is registered for version "<3PR>"
		// Body(opts).
		Do().
		Error()
}

// Create creates the provided resource.
func (s *servicePlanStatus) Create(data *spec.ServicePlanStatus) (*spec.ServicePlanStatus, error) {
	sps := &spec.ServicePlanStatus{}
	err := s.client.Post().
		NamespaceIfScoped(s.namespace, s.resource.Namespaced).
		Resource(s.resource.Name).
		Body(data).
		Do().
		Into(sps)
	return sps, err
}

// Update updates the provided resource.
func (s *servicePlanStatus) Update(data *spec.ServicePlanStatus) (*spec.ServicePlanStatus, error) {
	sps := &spec.ServicePlanStatus{}
	if len(data.GetName()) == 0 {
		return data, errors.New("object missing name")
	}
	err := s.client.Put().
		NamespaceIfScoped(s.namespace, s.resource.Namespaced).
		Resource(s.resource.Name).
		Name(data.GetName()).
		Body(data).
		Do().
		Into(sps)
	return sps, err
}

// Watch returns a watch.Interface that watches the resource.
func (s *servicePlanStatus) Watch(opts *api.ListOptions) (watch.Interface, error) {
	stream, err := s.client.Get().
		Prefix("watch").
		NamespaceIfScoped(s.namespace, s.resource.Namespaced).
		Resource(s.resource.Name).
		VersionedParams(opts, api.ParameterCodec).
		Stream()
	if err != nil {
		return nil, err
	}
	return watch.NewStreamWatcher(&servicePlanStatusDecoder{
		dec:   json.NewDecoder(stream),
		close: stream.Close,
	}), nil
}

// Patch updates the provided resource
func (s *servicePlanStatus) Patch(name string, pt api.PatchType, data []byte) (*spec.ServicePlanStatus, error) {
	sps := &spec.ServicePlanStatus{}
	err := s.client.Patch(pt).
		NamespaceIfScoped(s.namespace, s.resource.Namespaced).
		Resource(s.resource.Name).
		Name(name).
		Body(data).
		Do().
		Into(sps)
	return sps, err
}

// provides a decoder for watching 3PR resources
type servicePlanStatusDecoder struct {
	dec   *json.Decoder
	close func() error
}

// Close decoder
func (d *servicePlanStatusDecoder) Close() {
	d.close()
}

// Decode data
func (d *servicePlanStatusDecoder) Decode() (watch.EventType, runtime.Object, error) {
	var e struct {
		Type   watch.EventType
		Object spec.ServicePlanStatus
	}
	if err := d.dec.Decode(&e); err != nil {
		return watch.Error, nil, err
	}
	return e.Type, &e.Object, nil
}
