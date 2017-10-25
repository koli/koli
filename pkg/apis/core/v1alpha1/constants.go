package v1alpha1

const (
	// SystemSecretName it's a secret resource created and updated dynamically by a controller.
	// It should be used for communicating between systems
	SystemSecretName   = "koli-system-token"
	DefaultClusterRole = "koli:mutator:default"
)
