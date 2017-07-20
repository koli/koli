package v1alpha1

import (
	"fmt"

	"k8s.io/client-go/pkg/api/v1"
	v1beta1 "k8s.io/client-go/pkg/apis/apps/v1beta1"
	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/kubernetes/pkg/api"
)

// StatefulSetDeepCopy creates a deep-copy from a StatefulSet
// https://github.com/kubernetes/kubernetes/blob/master/docs/devel/controllers.md
func StatefulSetDeepCopy(petset *v1beta1.StatefulSet) (*v1beta1.StatefulSet, error) {
	objCopy, err := api.Scheme.DeepCopy(petset)
	if err != nil {
		return nil, err
	}
	copied, ok := objCopy.(*v1beta1.StatefulSet)
	if !ok {
		return nil, fmt.Errorf("expected StatefulSet, got %#v", objCopy)
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

// ServicePlanDeepCopy creates a deep-copy from the specified resource
func ServicePlanDeepCopy(sp *Plan) (*Plan, error) {
	objCopy, err := api.Scheme.DeepCopy(sp)
	if err != nil {
		return nil, err
	}
	copied, ok := objCopy.(*Plan)
	if !ok {
		return nil, fmt.Errorf("expected Service Plan, got %#v", objCopy)
	}
	return copied, nil
}

// ReleaseDeepCopy creates a deep-copy from the specified resource
func ReleaseDeepCopy(r *Release) (*Release, error) {
	objCopy, err := api.Scheme.DeepCopy(r)
	if err != nil {
		return nil, err
	}
	copied, ok := objCopy.(*Release)
	if !ok {
		return nil, fmt.Errorf("expected Release, got %#v", objCopy)
	}
	return copied, nil
}
