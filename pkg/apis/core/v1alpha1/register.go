package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	GroupName = "platform.koli.io"

	PlanResourceKind   = "Plan"
	PlanResourcePlural = "plans"

	ReleaseResourceKind   = "Release"
	ReleaseResourcePlural = "releases"

	// SystemNamespace it's where the global resources are persisted
	SystemNamespace = "koli-system"
)

var (
	// SchemeGroupVersion is group version used to register these objects
	SchemeGroupVersion = schema.GroupVersion{Group: GroupName, Version: "v1"}

	PlanResourceName    = PlanResourcePlural + "." + GroupName
	ReleaseResourceName = ReleaseResourcePlural + "." + GroupName
)

// Kind takes an unqualified kind and returns a Group qualified GroupKind
func Kind(kind string) schema.GroupKind {
	return SchemeGroupVersion.WithKind(kind).GroupKind()
}

// Resource takes an unqualified resource and returns a Group qualified GroupResource
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

var (
	// SchemeBuilder collects functions that add things to a scheme. It's to allow
	// code to compile without explicitly referencing generated types. You should
	// declare one in each package that will have generated deep copy or conversion
	// functions.
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	// AddToScheme applies all the stored functions to the scheme. A non-nil error
	// indicates that one function failed and the attempt was abandoned.
	AddToScheme = SchemeBuilder.AddToScheme
)

// Adds the list of known types to api.Scheme.
func addKnownTypes(s *runtime.Scheme) error {
	s.AddKnownTypes(SchemeGroupVersion,
		&Plan{},
		&PlanList{},
		&Release{},
		&ReleaseList{},
		&Domain{},
		&DomainList{},
		// &metav1.ListOptions{},
		// &metav1.DeleteOptions{},
	)
	metav1.AddToGroupVersion(s, SchemeGroupVersion)
	return nil
}
