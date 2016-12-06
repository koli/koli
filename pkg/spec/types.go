package spec

import (
	"k8s.io/client-go/1.5/pkg/api/unversioned"
	"k8s.io/client-go/1.5/pkg/api/v1"
)

// ServicePlan defines how resources could be managed and distributed
type ServicePlan struct {
	unversioned.TypeMeta `json:",inline"`
	v1.ObjectMeta        `json:"metadata,omitempty"`

	Spec ServicePlanSpec `json:"spec"`
}

// ServicePlanList is a list of ServicePlans
type ServicePlanList struct {
	unversioned.TypeMeta `json:",inline"`
	unversioned.ListMeta `json:"metadata,omitempty"`

	Items []*ServicePlan `json:"items"`
}

// ServicePlanSpec holds specification parameters of an ServicePlan
type ServicePlanSpec struct {
	// Compute Resources required by containers.
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
	// Hard is the set of desired hard limits for each named resource.
	Hard     v1.ResourceList     `json:"hard,omitempty"`
	Features ServicePlanFeatures `json:"features,omitempty"`
}

// ServicePlanFeatures defines the permissions for a ServicePlan
type ServicePlanFeatures struct {
	PodManagement struct {
		Exec        bool `json:"exec"`
		PortForward bool `json:"portForward"`
		AutoScale   bool `json:"autoScale"`
		Attach      bool `json:"attach"`
	} `json:"podManagement"`
	AddonManagement bool `json:"addonManagement"`
	MetricsAccess   bool `json:"metricsAccess"`
}

// ServicePlanStatus is information about the current status of a ServicePlan.
type ServicePlanStatus struct {
	unversioned.TypeMeta `json:",inline"`
	v1.ObjectMeta        `json:"metadata,omitempty"`

	// Phase is the current lifecycle phase of the namespace.
	Phase ServicePlanPhase `json:"phase"`
}

// ServicePlanStatusList is a list of ServicePlanStatus
type ServicePlanStatusList struct {
	unversioned.TypeMeta `json:",inline"`
	unversioned.ListMeta `json:"metadata,omitempty"`

	Items []*ServicePlanStatus `json:"items"`
}

type ServicePlanPhase string

const (
	// ServicePlanActive means the ServicePlan is available for use in the system
	ServicePlanActive ServicePlanPhase = "Active"
	// ServicePlanPending means the ServicePlan isn't associate with any global ServicePlan
	ServicePlanPending ServicePlanPhase = "Pending"
	// ServicePlanNotFound means the reference plan wasn't found
	ServicePlanNotFound ServicePlanPhase = "NotFound"
	// ServicePlanDisabled means the ServicePlan is disabled and cannot be associated with resources
	ServicePlanDisabled ServicePlanPhase = "Disabled"
)

// Addon defines integration with external resources
type Addon struct {
	unversioned.TypeMeta `json:",inline"`
	v1.ObjectMeta        `json:"metadata,omitempty"`
	Spec                 AddonSpec `json:"spec"`
}

// AddonList is a list of Addons.
type AddonList struct {
	unversioned.TypeMeta `json:",inline"`
	unversioned.ListMeta `json:"metadata,omitempty"`

	Items []*Addon `json:"items"`
}

// AddonSpec holds specification parameters of an addon
type AddonSpec struct {
	Type      string      `json:"type"`
	BaseImage string      `json:"baseImage"`
	Version   string      `json:"version"`
	Replicas  int32       `json:"replicas"`
	Port      int32       `json:"port"`
	Env       []v1.EnvVar `json:"env"`
	// More info: http://releases.k8s.io/HEAD/docs/user-guide/containers.md#containers-and-commands
	Args []string `json:"args,omitempty"`
}

// User identifies an user on the platform
type User struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	Organization string `json:"org"`
	Customer     string `json:"customer"`
}
