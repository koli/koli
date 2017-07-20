package v1alpha1

import "fmt"

// GetImage gets the BaseImage + Version
func (a *Addon) GetImage() string {
	if a.Spec.Version == "" {
		a.Spec.Version = "latest"
	}
	return fmt.Sprintf("%s:%s", a.Spec.BaseImage, a.Spec.Version)
}

// GetReplicas returns the size of replicas, if is less than 1 sets a default value
func (a *Addon) GetReplicas() *int32 {
	if a.Spec.Replicas < 1 {
		a.Spec.Replicas = 1
	}
	return &a.Spec.Replicas
}
