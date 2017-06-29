package util

import "path/filepath"

// ObjectMeta maps a namespace/resource as owner/repo where
// every object has an authentication source.
type ObjectMeta interface {
	GetRepository() string
	GetName() string
	GetOwner() string
	GetAuthUser() string
	GetAuthToken() string
	WithCredentials(authUser, authToken string) *objectMeta
}

type objectMeta struct {
	name      string
	owner     string
	authUser  string
	authToken string
}

// GetRepository joins the owner and the name with a slash
func (m *objectMeta) GetRepository() string {
	return filepath.Join(m.owner, m.name)
}

// GetName returns the 'name' attribute
func (m *objectMeta) GetName() string {
	return m.name
}

// GetOwner returns the 'owner' attribute
func (m *objectMeta) GetOwner() string {
	return m.owner
}

// GetAuthUser returns the 'authUser' attribute
func (m *objectMeta) GetAuthUser() string {
	return m.authUser
}

// GetAuthToken returns the 'authToken' attribute
func (m *objectMeta) GetAuthToken() string {
	return m.authToken
}

// WithCredentials set user credentials
func (m *objectMeta) WithCredentials(authUser, authToken string) *objectMeta {
	m.authUser = authUser
	m.authToken = authToken
	return m
}

// NewObjectMeta generates a new ObjectMeta
func NewObjectMeta(name, owner string) ObjectMeta {
	return &objectMeta{name: name, owner: owner}
}
