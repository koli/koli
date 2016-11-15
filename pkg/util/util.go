package util

import (
	"fmt"

	"k8s.io/client-go/1.5/pkg/apis/apps/v1alpha1"
	"k8s.io/kubernetes/pkg/api"
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
