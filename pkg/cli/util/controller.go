package util

import (
	"net/http"
	"net/url"

	"k8s.io/kubernetes/pkg/runtime"
)

type Namespace struct {
	Name        string            `json:"name,omitempty"`
	Description string            `json:"description,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

const (
	controllerHost = "controller.kolihub.io"
)

// Controller represents the communication with the KOLI Controller
type Controller struct {
	// API Base URL. E.G.: https://controller.kolihub.io
	Request  *Request
	resource string
}

// NewController returns a new Controller
func NewController(url *url.URL, resource string) *Controller {
	req := NewRequest(http.DefaultClient, "", url).
		SetHostHeader(controllerHost)

	return &Controller{
		Request:  req,
		resource: resource,
	}
}

func (c *Controller) Resource(name string) *Controller {
	c.resource = name
	return c
}

// Get retrieves a single resource
func (c *Controller) Get(name string) (runtime.Object, error) {
	return c.Request.GET().
		Resource(c.resource).
		Name(name).
		Do().
		Get()
}

// List retrieves a list of resources
func (c *Controller) List() (runtime.Object, error) {
	return c.Request.GET().
		Resource(c.resource).
		Do().
		Get()
}

// Delete a single resource
func (c *Controller) Delete(name string) error {
	return c.Request.DELETE().
		Resource(c.resource).
		Name(name).
		Do().
		Error()
}

// Create a new resource
func (c *Controller) Create(data []byte) (runtime.Object, error) {
	return c.Request.POST().
		Resource(c.resource).
		Body(data).
		Do().
		Get()
}
