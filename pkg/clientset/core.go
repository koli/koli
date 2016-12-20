package clientset

import (
	"k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/client/restclient"
)

// CoreInterface contains client third party resources
type CoreInterface interface {
	RESTClient() restclient.Interface

	AddonGetter
	ServicePlanGetter
	ServicePlanStatusGetter
}

// CoreClient is used to interact with features provided by the Core group.
type CoreClient struct {
	restClient restclient.Interface
}

// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *CoreClient) RESTClient() restclient.Interface {
	if c == nil {
		return nil
	}
	return c.restClient
}

// ServicePlanStatus generates a new client to communicate with ServicePlanStatus resources
func (c *CoreClient) ServicePlanStatus(namespace string) ServicePlanStatusInterface {
	return newServicePlanStatus(c, namespace)
}

// ServicePlan generates a new client to communicate with ServicePlan resources
func (c *CoreClient) ServicePlan(namespace string) ServicePlanInterface {
	return newServicePlan(c, namespace)
}

// Addon generates a new client to communnicate with Addon resources
func (c *CoreClient) Addon(namespace string) AddonInterface {
	return newAddon(c, namespace)
}

func newServicePlanStatus(c *CoreClient, namespace string) *servicePlanStatus {
	return &servicePlanStatus{
		client:    c.RESTClient(),
		namespace: namespace,
		resource: &unversioned.APIResource{
			Name:       "serviceplanstatuses",
			Namespaced: true,
			Kind:       "Serviceplanstatus",
		},
	}
}

func newServicePlan(c *CoreClient, namespace string) *servicePlan {
	return &servicePlan{
		client:    c.RESTClient(),
		namespace: namespace,
		resource: &unversioned.APIResource{
			Name:       "serviceplans",
			Namespaced: true,
			Kind:       "Serviceplan",
		},
	}
}

func newAddon(c *CoreClient, namespace string) *addon {
	return &addon{
		client:    c.RESTClient(),
		namespace: namespace,
		resource: &unversioned.APIResource{
			Name:       "addons",
			Namespaced: true,
			Kind:       "Addon",
		},
	}
}
