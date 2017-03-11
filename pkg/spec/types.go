package spec

import (
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/unversioned"
)

// ResourceList is a set of (resource name, quantity) pairs.
type ResourceList api.ResourceList

// ServicePlan defines how resources could be managed and distributed
type ServicePlan struct {
	unversioned.TypeMeta `json:",inline"`
	api.ObjectMeta       `json:"metadata,omitempty"`

	Spec ServicePlanSpec `json:"spec"`
}

// ServicePlanList is a list of ServicePlans
type ServicePlanList struct {
	unversioned.TypeMeta `json:",inline"`
	unversioned.ListMeta `json:"metadata,omitempty"`

	Items []ServicePlan `json:"items"`
}

// ServicePlanSpec holds specification parameters of an ServicePlan
type ServicePlanSpec struct {
	// Compute Resources required by containers.
	Resources api.ResourceRequirements `json:"resources,omitempty"`
	// Hard is the set of desired hard limits for each named resource.
	Hard  ResourceList   `json:"hard,omitempty"`
	Roles []PlatformRole `json:"roles,omitempty"`
}

const (
	// ResourceNamespace , number
	ResourceNamespace api.ResourceName = "namespaces"
)

// PlatformRole is the name identifying various roles in a PlatformRoleList.
type PlatformRole string

const (
	// RoleExecAllow cluster role name
	RoleExecAllow PlatformRole = "exec-allow"
	// RolePortForwardAllow cluster role name
	RolePortForwardAllow PlatformRole = "portforward-allow"
	// RoleAutoScaleAllow cluster role name
	RoleAutoScaleAllow PlatformRole = "autoscale-allow"
	// RoleAttachAllow cluster role name
	RoleAttachAllow PlatformRole = "attach-allow"
	// RoleAddonManagement cluster role name
	RoleAddonManagement PlatformRole = "addon-management"
)

// ServicePlanStatus is information about the current status of a ServicePlan.
type ServicePlanStatus struct {
	unversioned.TypeMeta `json:",inline"`
	api.ObjectMeta       `json:"metadata,omitempty"`

	// Phase is the current lifecycle phase of the namespace.
	Phase ServicePlanPhase `json:"phase"`
}

// ServicePlanStatusList is a list of ServicePlanStatus
type ServicePlanStatusList struct {
	unversioned.TypeMeta `json:",inline"`
	unversioned.ListMeta `json:"metadata,omitempty"`

	Items []ServicePlanStatus `json:"items"`
}

// ServicePlanPhase is the current lifecycle phase of the Service Plan.
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
	api.ObjectMeta       `json:"metadata,omitempty"`
	Spec                 AddonSpec `json:"spec"`
}

// AddonList is a list of Addons.
type AddonList struct {
	unversioned.TypeMeta `json:",inline"`
	unversioned.ListMeta `json:"metadata,omitempty"`

	Items []Addon `json:"items"`
}

// AddonSpec holds specification parameters of an addon
type AddonSpec struct {
	Type      string       `json:"type"`
	BaseImage string       `json:"baseImage"`
	Version   string       `json:"version"`
	Replicas  int32        `json:"replicas"`
	Port      int32        `json:"port"`
	Env       []api.EnvVar `json:"env"`
	// More info: http://releases.k8s.io/HEAD/docs/user-guide/containers.md#containers-and-commands
	Args []string `json:"args,omitempty"`
}

// Release refers to compiled slug file versions
type Release struct {
	unversioned.TypeMeta `json:",inline"`
	api.ObjectMeta       `json:"metadata,omitempty"`
	Spec                 ReleaseSpec `json:"spec"`
}

// ReleaseSpec holds specification parameters of a release
type ReleaseSpec struct {
	// The URL of the git remote server to download the git revision tarball
	GitRemote     string `json:"gitRemote"`
	GitRevision   string `json:"gitRevision"`
	GitRepository string `json:"gitRepository"`
	BuildRevision string `json:"buildRevision"`
	AutoDeploy    bool   `json:"autoDeploy"`
	ExpireAfter   int32  `json:"expireAfter"`
	DeployName    string `json:"deployName"`
	Build         bool   `json:"build"`
	AuthToken     string `json:"auth_token"` // expirable token
}

// ReleaseList is a list of Release
type ReleaseList struct {
	unversioned.TypeMeta `json:",inline"`
	unversioned.ListMeta `json:"metadata,omitempty"`
	Items                []Release `json:"items"`
}

// User identifies an user on the platform
type User struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	Customer     string `json:"customer"`
	Organization string `json:"org"`
	// Groups are a set of strings which associate users with as set of commonly grouped users.
	// A group name is unique in the cluster and it's formed by it's namespace, customer or the organization name:
	// [org] - Matches all the namespaces of the broker
	// [customer]-[org] - Matches all namespaces from the customer broker
	// [name]-[customer]-[org] - Matches a specific namespace
	// http://kubernetes.io/docs/admin/authentication/
	Groups []string `json:"groups"`
}
