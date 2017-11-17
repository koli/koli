package v1alpha1

import (
	"fmt"
	"regexp"

	v1beta1 "k8s.io/api/apps/v1beta1"
	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

// this constant represents the length of a shortened git sha - 8 characters long
const shortShaIdx = 8

var shaRegex = regexp.MustCompile(`^[\da-f]{40}$`)

// NewSha creates a raw string to a SHA. Returns ErrInvalidGitSha if the sha was invalid.
func NewSha(rawSha string) (*SHA, error) {
	if !shaRegex.MatchString(rawSha) {
		return nil, ErrInvalidGitSha{sha: rawSha}
	}
	return &SHA{full: rawSha, short: rawSha[0:shortShaIdx]}, nil
}

// Full returns the full git sha.
func (s SHA) Full() string { return s.full }

// Short returns the first 8 characters of the sha.
func (s SHA) Short() string { return s.short }

// Error is the error interface implementation.
func (e ErrInvalidGitSha) Error() string {
	return fmt.Sprintf("Git sha %s was invalid", e.sha)
}

// StatefulSetDeepCopy creates a deep-copy from a StatefulSet
// https://github.com/kubernetes/kubernetes/blob/master/docs/devel/controllers.md
func StatefulSetDeepCopy(petset *v1beta1.StatefulSet) (*v1beta1.StatefulSet, error) {
	objCopy, err := scheme.Scheme.DeepCopy(petset)
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
func ServicePlanDeepCopy(sp *Plan) (*Plan, error) {
	objCopy, err := scheme.Scheme.DeepCopy(sp)
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
	objCopy, err := scheme.Scheme.DeepCopy(r)
	if err != nil {
		return nil, err
	}
	copied, ok := objCopy.(*Release)
	if !ok {
		return nil, fmt.Errorf("expected Release, got %#v", objCopy)
	}
	return copied, nil
}
