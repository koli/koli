package util

import (
	"fmt"

	"strings"

	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/pkg/apis/apps/v1alpha1"
	extensions "k8s.io/client-go/1.5/pkg/apis/extensions/v1beta1"
)

// PetSetDeepCopy creates a deep-copy from a PetSet
// https://github.com/kubernetes/kubernetes/blob/master/docs/devel/controllers.md
func PetSetDeepCopy(petset *v1alpha1.PetSet) (*v1alpha1.PetSet, error) {
	objCopy, err := api.Scheme.DeepCopy(petset)
	if err != nil {
		return nil, err
	}
	copied, ok := objCopy.(*v1alpha1.PetSet)
	if !ok {
		return nil, fmt.Errorf("expected PetSet, got %#v", objCopy)
	}
	return copied, nil
}

// NamespaceDeepCopy creates a deep-copy from a Namespace
func NamespaceDeepCopy(ns *v1.Namespace) (*v1.Namespace, error) {
	objCopy, err := api.Scheme.DeepCopy(ns)
	if err != nil {
		return nil, err
	}
	copied, ok := objCopy.(*v1.Namespace)
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
		return nil, fmt.Errorf("namespace in wrong format: %s", namespace)
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
