package clientset

import (
	"encoding/json"
	"errors"

	"github.com/golang/glog"
	"github.com/kolibox/koli/pkg/spec"
	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/api/unversioned"
	"k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/pkg/runtime"
	"k8s.io/client-go/1.5/pkg/watch"
	"k8s.io/client-go/1.5/rest"
)

// ServicePlanGetter has a method to return an ServicePlanInterface.
// A group's client should implement this interface.
type ServicePlanGetter interface {
	ServicePlan(namespace string) ServicePlanInterface
}

// ServicePlanInterface has methods to work with ServicePlan resources.
type ServicePlanInterface interface {
	List(opts *api.ListOptions) (*spec.ServicePlanList, error)
	Get(name string) (*spec.ServicePlan, error)
	Delete(name string, options *v1.DeleteOptions) error
	Create(data *spec.ServicePlan) (*spec.ServicePlan, error)
	Update(data *spec.ServicePlan) (*spec.ServicePlan, error)
	Watch(opts *api.ListOptions) (watch.Interface, error)
}

// servicePlan implements ServicePlanInterface
type servicePlan struct {
	client    *rest.RESTClient
	namespace string
	resource  *unversioned.APIResource
}

// Get gets the resource with the specified name.
func (s *servicePlan) Get(name string) (*spec.ServicePlan, error) {
	sps := &spec.ServicePlan{}
	err := s.client.Get().
		NamespaceIfScoped(s.namespace, s.resource.Namespaced).
		Resource(s.resource.Name).
		Name(name).
		Do().
		Into(sps)
	return sps, err
}

// List returns a list of objects for this resource.
func (s *servicePlan) List(opts *api.ListOptions) (*spec.ServicePlanList, error) {
	if opts == nil {
		opts = &api.ListOptions{}
	}
	spList := &spec.ServicePlanList{}
	err := s.client.Get().
		NamespaceIfScoped(s.namespace, s.resource.Namespaced).
		Resource(s.resource.Name).
		// VersionedParams(opts).
		Do().
		Into(spList)
	return spList, err
}

// Delete deletes the resource with the specified name.
func (s *servicePlan) Delete(name string, opts *v1.DeleteOptions) error {
	if opts == nil {
		opts = &v1.DeleteOptions{}
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
func (s *servicePlan) Create(data *spec.ServicePlan) (*spec.ServicePlan, error) {
	sps := &spec.ServicePlan{}
	err := s.client.Post().
		NamespaceIfScoped(s.namespace, s.resource.Namespaced).
		Resource(s.resource.Name).
		Body(data).
		Do().
		Into(sps)
	return sps, err
}

// Update updates the provided resource.
func (s *servicePlan) Update(data *spec.ServicePlan) (*spec.ServicePlan, error) {
	sps := &spec.ServicePlan{}
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
func (s *servicePlan) Watch(opts *api.ListOptions) (watch.Interface, error) {
	stream, err := s.client.Get().
		Prefix("watch").
		NamespaceIfScoped(s.namespace, s.resource.Namespaced).
		Resource(s.resource.Name).
		VersionedParams(opts, api.ParameterCodec).
		Stream()
	if err != nil {
		return nil, err
	}
	return watch.NewStreamWatcher(&servicePlanDecoder{
		dec:   json.NewDecoder(stream),
		close: stream.Close,
	}), nil
}

// Patch updates the provided resource
func (s *servicePlan) Patch(name string, pt api.PatchType, data []byte) (*spec.ServicePlan, error) {
	sps := &spec.ServicePlan{}
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
type servicePlanDecoder struct {
	dec   *json.Decoder
	close func() error
}

// Close decoder
func (d *servicePlanDecoder) Close() {
	d.close()
}

// Decode data
func (d *servicePlanDecoder) Decode() (watch.EventType, runtime.Object, error) {
	var e struct {
		Type   watch.EventType
		Object spec.ServicePlan
	}
	if err := d.dec.Decode(&e); err != nil {
		glog.Errorf("failed decoding service plan '%s': %s", e.Object.Name, err)
		return watch.Error, nil, err
	}
	return e.Type, &e.Object, nil
}
