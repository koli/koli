package auth0

import (
	"fmt"

	"net/url"

	"kolihub.io/koli/pkg/apis/authentication"
	"kolihub.io/koli/pkg/request"
)

type AuthenticationInterface interface {
	ClientCredentials(token *authentication.Token) (*authentication.Token, error)
}

type ManagementInterface interface {
	Users() UserInterface
}

func NewForConfig(c *Config) (CoreInterface, error) {
	requestURL, err := url.Parse(c.Host)
	if err != nil {
		return nil, fmt.Errorf("failed parsing URL: %v", err)
	}
	client := request.NewRequest(c.Client, requestURL)
	if len(c.BearerToken) > 0 {
		client.SetHeader("Authorization", fmt.Sprintf("Bearer %s", c.BearerToken))
	}

	return &CoreClient{restClient: client}, nil
}
