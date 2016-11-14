package util

// https://github.com/kubernetes/kubernetes/blob/master/pkg/controller/deployment/util/deployment_util.go

import (
	"fmt"
	"time"

	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/api/unversioned"
	"k8s.io/client-go/1.5/pkg/apis/apps/v1alpha1"
	extensions "k8s.io/client-go/1.5/pkg/apis/extensions/v1beta1"
)

const (
	// SelectorUpdateAnnotation marks the last time deployment selector update
	// TODO: Delete this annotation when we gracefully handle overlapping selectors.
	// See https://github.com/kubernetes/kubernetes/issues/2210
	SelectorUpdateAnnotation = "replicaset.kubernetes.io/selector-updated-at"
)

// DeploymentDeepCopy creates a deep-copy from a deployment
// https://github.com/kubernetes/kubernetes/blob/master/docs/devel/controllers.md
func DeploymentDeepCopy(deployment *extensions.Deployment) (*extensions.Deployment, error) {
	objCopy, err := api.Scheme.DeepCopy(deployment)
	if err != nil {
		return nil, err
	}
	copied, ok := objCopy.(*extensions.Deployment)
	if !ok {
		return nil, fmt.Errorf("expected Deployment, got %#v", objCopy)
	}
	return copied, nil
}

// PetSetDeepCopy creates a deep-copy from a deployment
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

// LastSelectorUpdate returns the last time given replicaset selector is updated
func LastSelectorUpdate(d *extensions.ReplicaSet) unversioned.Time {
	t := d.Annotations[SelectorUpdateAnnotation]
	if len(t) > 0 {
		parsedTime, err := time.Parse(t, time.RFC3339)
		// If failed to parse the time, use creation timestamp instead
		if err != nil {
			return d.CreationTimestamp
		}
		return unversioned.Time{Time: parsedTime}
	}
	// If it's never updated, use creation timestamp instead
	return d.CreationTimestamp
}

// BySelectorLastUpdateTime sorts a list of replicasets by the last update time of their selector,
// first using their creation timestamp and then their names as a tie breaker.
type BySelectorLastUpdateTime []*extensions.ReplicaSet

func (o BySelectorLastUpdateTime) Len() int      { return len(o) }
func (o BySelectorLastUpdateTime) Swap(i, j int) { o[i], o[j] = o[j], o[i] }
func (o BySelectorLastUpdateTime) Less(i, j int) bool {
	ti, tj := LastSelectorUpdate(o[i]), LastSelectorUpdate(o[j])
	if ti.Equal(tj) {
		if o[i].CreationTimestamp.Equal(o[j].CreationTimestamp) {
			return o[i].Name < o[j].Name
		}
		return o[i].CreationTimestamp.Before(o[j].CreationTimestamp)
	}
	return ti.Before(tj)
}
