package auth0

import "kolihub.io/koli/pkg/request"

type CoreInterface interface {
	RESTClient() request.Interface

	Authentication() AuthenticationInterface
	Management(accessToken string) ManagementInterface
}

type CoreClient struct {
	restClient request.Interface
}

func (c *CoreClient) RESTClient() request.Interface {
	if c.restClient == nil {
		return nil
	}
	return c.restClient
}

func (c *CoreClient) Authentication() AuthenticationInterface {
	// Clean route set up
	c.restClient.Reset()
	return &auth{
		client:   c.restClient,
		resource: "/oauth/token",
	}
}

// Management represents the management api from auth0
// https://auth0.com/docs/api/management/v2
func (c *CoreClient) Management(accessToken string) ManagementInterface {
	// Clean route set up
	c.restClient.Reset()
	return &management{
		client:      c.restClient,
		apiPath:     "/api/v2",
		accessToken: accessToken,
	}
}
