package spec

import (
	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/api/unversioned"
	"k8s.io/client-go/1.5/pkg/runtime"
)

// GroupName is the group name use in this package
const GroupName = "sys.koli.io"

// SchemeGroupVersion is group version used to register these objects
// var SchemeGroupVersion = unversioned.GroupVersion{Group: GroupName, Version: runtime.APIVersionInternal}
var SchemeGroupVersion = unversioned.GroupVersion{Group: GroupName, Version: "v1alpha1"}

// Kind takes an unqualified kind and returns a Group qualified GroupKind
func Kind(kind string) unversioned.GroupKind {
	return SchemeGroupVersion.WithKind(kind).GroupKind()
}

// Resource takes an unqualified resource and returns a Group qualified GroupResource
func Resource(resource string) unversioned.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

var (
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	AddToScheme   = SchemeBuilder.AddToScheme
)

// Adds the list of known types to api.Scheme.
func addKnownTypes(scheme *runtime.Scheme) error {
	// TODO this gets cleaned up when the types are fixed
	scheme.AddKnownTypes(SchemeGroupVersion,
		&Addon{},
		&AddonList{},
		&ServicePlan{},
		&ServicePlanList{},
		&ServicePlanStatus{},
		&ServicePlanStatusList{},
		&api.ListOptions{},
	)
	return nil
}
