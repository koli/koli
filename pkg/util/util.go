package util

import (
	"errors"
	"fmt"

	"strings"

	"k8s.io/kubernetes/pkg/api"
	apps "k8s.io/kubernetes/pkg/apis/apps"
	extensions "k8s.io/kubernetes/pkg/apis/extensions"
)

// StatefulSetDeepCopy creates a deep-copy from a StatefulSet
// https://github.com/kubernetes/kubernetes/blob/master/docs/devel/controllers.md
func StatefulSetDeepCopy(petset *apps.StatefulSet) (*apps.StatefulSet, error) {
	objCopy, err := api.Scheme.DeepCopy(petset)
	if err != nil {
		return nil, err
	}
	copied, ok := objCopy.(*apps.StatefulSet)
	if !ok {
		return nil, fmt.Errorf("expected StatefulSet, got %#v", objCopy)
	}
	return copied, nil
}

// NamespaceDeepCopy creates a deep-copy from a Namespace
func NamespaceDeepCopy(ns *api.Namespace) (*api.Namespace, error) {
	objCopy, err := api.Scheme.DeepCopy(ns)
	if err != nil {
		return nil, err
	}
	copied, ok := objCopy.(*api.Namespace)
	if !ok {
		return nil, fmt.Errorf("expected Namespace, got %#v", objCopy)
	}
	return copied, nil
}

// DeploymentDeepCopy creates a deep-copy from the specified resource
func DeploymentDeepCopy(d *extensions.Deployment) (*extensions.Deployment, error) {
	objCopy, err := api.Scheme.DeepCopy(d)
	if err != nil {
		return nil, err
	}
	copied, ok := objCopy.(*extensions.Deployment)
	if !ok {
		return nil, fmt.Errorf("expected Deployment, got %#v", objCopy)
	}
	return copied, nil
}

// BrokerNamespace represents the existent metadata of a broker namespace containing
// the name of the namespace, organization and the name of the customer in the form:
// [namespace]-[customer]-[organization]
type BrokerNamespace struct {
	Namespace    string
	Customer     string
	Organization string
}

// NewBrokerNamespace extract the organization, customer and the name of the namespace
func NewBrokerNamespace(namespace string) (*BrokerNamespace, error) {
	parts := strings.Split(namespace, "-")
	if len(parts) != 3 {
		return nil, errors.New("namespace in wrong format")
	}
	return &BrokerNamespace{
		Namespace:    parts[0],
		Customer:     parts[1],
		Organization: parts[2],
	}, nil
}

// IsBroker returns true if it's a broker namespace: default-main-[org]
func (b *BrokerNamespace) IsBroker() bool {
	if b.Customer == "main" && b.Namespace == "default" {
		return true
	}
	return false
}

// GetBrokerNamespace returns the broker namespace
func (b *BrokerNamespace) GetBrokerNamespace() string {
	return fmt.Sprintf("default-main-%s", b.Organization)
}

// GetNamespace retrieves the original namespace
func (b *BrokerNamespace) GetNamespace() string {
	return fmt.Sprintf("%s-%s-%s", b.Namespace, b.Customer, b.Organization)
}
