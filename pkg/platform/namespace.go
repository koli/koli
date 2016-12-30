package platform

import (
	"errors"
	"fmt"
	"strings"
)

// Namespace represents the existent metadata of the platform namespace containing
// the name of the namespace, organization and the name of the customer in the form:
// [namespace]-[customer]-[organization]
type Namespace struct {
	Namespace    string
	Customer     string
	Organization string
}

// NewNamespace extract the organization, customer and the name of the namespace
func NewNamespace(namespace string) (*Namespace, error) {
	parts := strings.Split(namespace, "-")
	if len(parts) != 3 {
		return nil, errors.New("namespace in wrong format")
	}
	return &Namespace{
		Namespace:    parts[0],
		Customer:     parts[1],
		Organization: parts[2],
	}, nil
}

// IsSystem returns true if it's a system broker namespace.
func (n *Namespace) IsSystem() bool {
	if n.Customer == BrokerSystemCustomer && n.Namespace == BrokerSystemNamespace {
		return true
	}
	return false
}

// GetSystemNamespace returns the system broker namespace
func (n *Namespace) GetSystemNamespace() string {
	return fmt.Sprintf("%s-%s-%s",
		BrokerSystemNamespace, BrokerSystemCustomer, n.Organization)
}

// GetNamespace retrieves the original namespace
func (n *Namespace) GetNamespace() string {
	return fmt.Sprintf("%s-%s-%s", n.Namespace, n.Customer, n.Organization)
}
