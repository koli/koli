package util

import (
	"fmt"

	"kolihub.io/koli/pkg/spec"

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

// ServicePlanDeepCopy creates a deep-copy from the specified resource
func ServicePlanDeepCopy(sp *spec.ServicePlan) (*spec.ServicePlan, error) {
	objCopy, err := api.Scheme.DeepCopy(sp)
	if err != nil {
		return nil, err
	}
	copied, ok := objCopy.(*spec.ServicePlan)
	if !ok {
		return nil, fmt.Errorf("expected Service Plan, got %#v", objCopy)
	}
	return copied, nil
}
