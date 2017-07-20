package auth0

import (
	"fmt"

	"kolihub.io/koli/pkg/apis/authentication"
	"kolihub.io/koli/pkg/request"
)

type management struct {
	client      request.Interface
	apiPath     string
	accessToken string
}

func (m *management) Users() UserInterface {
	return &user{
		management: m,
		resource:   "users",
	}
}

type UserInterface interface {
	Get(id string) (*authentication.User, error)
}

type user struct {
	management *management
	resource   string
}

func (u *user) Get(id string) (*authentication.User, error) {
	user := &authentication.User{}
	return user, u.management.client.Get().
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", u.management.accessToken)).
		RequestPath(u.management.apiPath).
		Resource(u.resource).
		Name(id).
		Do().
		Into(user)
}
