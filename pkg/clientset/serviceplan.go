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

// ServicePlanGetter has a method to return an ServicePlanInterface.
// A group's client should implement this interface.
type ServicePlanGetter interface {
	ServicePlan(namespace string) ServicePlanInterface
}

// ServicePlanInterface has methods to work with ServicePlan resources.
type ServicePlanInterface interface {
	List(opts *metav1.ListOptions) (*platform.PlanList, error)
	Get(name string) (*platform.Plan, error)
	Delete(name string, options *metav1.DeleteOptions) error
	Create(data *platform.Plan) (*platform.Plan, error)
	Update(data *platform.Plan) (*platform.Plan, error)
	Watch(opts *metav1.ListOptions) (watch.Interface, error)
}

// servicePlan implements ServicePlanInterface
type servicePlan struct {
	client    rest.Interface
	namespace string
	resource  *metav1.APIResource
}

// Get gets the resource with the specified name.
func (s *servicePlan) Get(name string) (*platform.Plan, error) {
	sps := &platform.Plan{}
	err := s.client.Get().
		NamespaceIfScoped(s.namespace, s.resource.Namespaced).
		Resource(s.resource.Name).
		Name(name).
		Do().
		Into(sps)
	return sps, err
}

// List returns a list of objects for this resource.
func (s *servicePlan) List(opts *metav1.ListOptions) (*platform.PlanList, error) {
	spList := &platform.PlanList{}
	err := s.client.Get().
		NamespaceIfScoped(s.namespace, s.resource.Namespaced).
		Resource(s.resource.Name).
		// VersionedParams(opts).
		Do().
		Into(spList)
	return spList, err
}

// Delete deletes the resource with the specified name.
func (s *servicePlan) Delete(name string, opts *metav1.DeleteOptions) error {
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
func (s *servicePlan) Create(data *platform.Plan) (*platform.Plan, error) {
	sps := &platform.Plan{}
	err := s.client.Post().
		NamespaceIfScoped(s.namespace, s.resource.Namespaced).
		Resource(s.resource.Name).
		Body(data).
		Do().
		Into(sps)
	return sps, err
}

// Update updates the provided resource.
func (s *servicePlan) Update(data *platform.Plan) (*platform.Plan, error) {
	sps := &platform.Plan{}
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
func (s *servicePlan) Watch(opts *metav1.ListOptions) (watch.Interface, error) {
	stream, err := s.client.Get().
		Prefix("watch").
		NamespaceIfScoped(s.namespace, s.resource.Namespaced).
		Resource(s.resource.Name).
		VersionedParams(opts, metav1.ParameterCodec).
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
func (s *servicePlan) Patch(name string, pt types.PatchType, data []byte) (*platform.Plan, error) {
	sps := &platform.Plan{}
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
		Object platform.Plan
	}
	if err := d.dec.Decode(&e); err != nil {
		return watch.Error, nil, err
	}
	return e.Type, &e.Object, nil
}
