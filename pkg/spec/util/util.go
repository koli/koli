package util

import (
	"fmt"

	apps "k8s.io/api/apps/v1beta1"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/client-go/kubernetes/scheme"
	"kolihub.io/koli/pkg/spec"
)

// StatefulSetDeepCopy creates a deep-copy from a StatefulSet
// https://github.com/kubernetes/kubernetes/blob/master/docs/devel/controllers.md
func StatefulSetDeepCopy(petset *apps.StatefulSet) (*apps.StatefulSet, error) {
	objCopy, err := scheme.Scheme.DeepCopy(petset)
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
func NamespaceDeepCopy(ns *v1.Namespace) (*v1.Namespace, error) {
	objCopy, err := scheme.Scheme.DeepCopy(ns)
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
func DeploymentDeepCopy(d *v1beta1.Deployment) (*v1beta1.Deployment, error) {
	objCopy, err := scheme.Scheme.DeepCopy(d)
	if err != nil {
		return nil, err
	}
	copied, ok := objCopy.(*v1beta1.Deployment)
	if !ok {
		return nil, fmt.Errorf("expected Deployment, got %#v", objCopy)
	}
	return copied, nil
}

// ServicePlanDeepCopy creates a deep-copy from the specified resource
func ServicePlanDeepCopy(sp *spec.Plan) (*spec.Plan, error) {
	objCopy, err := scheme.Scheme.DeepCopy(sp)
	if err != nil {
		return nil, err
	}
	copied, ok := objCopy.(*spec.Plan)
	if !ok {
		return nil, fmt.Errorf("expected Service Plan, got %#v", objCopy)
	}
	return copied, nil
}

// ReleaseDeepCopy creates a deep-copy from the specified resource
func ReleaseDeepCopy(r *spec.Release) (*spec.Release, error) {
	objCopy, err := scheme.Scheme.DeepCopy(r)
	if err != nil {
		return nil, err
	}
	copied, ok := objCopy.(*spec.Release)
	if !ok {
		return nil, fmt.Errorf("expected Release, got %#v", objCopy)
	}
	return copied, nil
}
