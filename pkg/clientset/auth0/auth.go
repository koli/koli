package auth0

import (
	"kolihub.io/koli/pkg/apis/authentication"
	"kolihub.io/koli/pkg/request"
)

type auth struct {
	client   request.Interface
	resource string
}

func (a *auth) ClientCredentials(token *authentication.Token) (*authentication.Token, error) {
	response := &authentication.Token{}
	return response, a.client.Post().
		RequestPath(a.resource).
		Body(token).
		Do().
		Into(response)
}
