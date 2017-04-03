package clientset

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

// CoreInterface contains client third party resources
type CoreInterface interface {
	RESTClient() rest.Interface

	AddonGetter
	ServicePlanGetter
	ReleaseGetter
}

// CoreClient is used to interact with features provided by the Core group.
type CoreClient struct {
	restClient rest.Interface
}

// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *CoreClient) RESTClient() rest.Interface {
	if c == nil {
		return nil
	}
	return c.restClient
}

// ServicePlan generates a new client to communicate with ServicePlan resources
func (c *CoreClient) ServicePlan(namespace string) ServicePlanInterface {
	return newServicePlan(c, namespace)
}

// Addon generates a new client to communicate with Addon resources
func (c *CoreClient) Addon(namespace string) AddonInterface {
	return newAddon(c, namespace)
}

// Release generates a new client to communicate with Release resources
func (c *CoreClient) Release(namespace string) ReleaseInterface {
	return newRelease(c, namespace)
}

func newServicePlan(c *CoreClient, namespace string) *servicePlan {
	return &servicePlan{
		client:    c.RESTClient(),
		namespace: namespace,
		resource: &metav1.APIResource{
			Name:       "plans",
			Namespaced: true,
			Kind:       "Plan",
		},
	}
}

func newAddon(c *CoreClient, namespace string) *addon {
	return &addon{
		client:    c.RESTClient(),
		namespace: namespace,
		resource: &metav1.APIResource{
			Name:       "addons",
			Namespaced: true,
			Kind:       "Addon",
		},
	}
}

func newRelease(c *CoreClient, namespace string) *release {
	return &release{
		client:    c.RESTClient(),
		namespace: namespace,
		resource: &metav1.APIResource{
			Name:       "releases",
			Namespaced: true,
			Kind:       "release",
		},
	}
}
