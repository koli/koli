package v1alpha1

import (
	jwt "github.com/dgrijalva/jwt-go"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ResourceList is a set of (resource name, quantity) pairs.
type ResourceList v1.ResourceList

// PlanType describes the rules how resources are going to be provisioned
type PlanType string

const (
	// PlanTypeDefault means a plan will consider only compute (memory, CPU) and
	// Kubernetes resources (pods, services, etc)
	PlanTypeDefault PlanType = ""
	// PlanTypeStorage means a plan will consider only storage resources
	PlanTypeStorage PlanType = "Storage"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Plan defines how resources could be managed and distributed
type Plan struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec PlanSpec `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PlanList is a list of ServicePlans
type PlanList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Plan `json:"items"`
}

// PlanSpec holds specification parameters of an Plan
type PlanSpec struct {
	// Type determine how resources are provisioned by the controller,
	// defaults to Compute and Kubernetes Object resources. Valid options are: "" and Storage.
	// "" means the spec will provision memory and CPU from the 'resources' attribute,
	// 'hard' could be used to limit the usage of Kubernetes resources
	Type PlanType `json:"type,omitempty"`
	// Compute Resources required by containers.
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
	// Hard is the set of desired hard limits for Kubernetes objects resources.
	Hard ResourceList `json:"hard,omitempty"`
	// Storage is the ammount of storage requested
	Storage resource.Quantity `json:"storage,omitempty"`
	// DefaultClusterRole is the reference cluster role name
	// to create a rolebinding for each user on the platform
	DefaultClusterRole string `json:"defaultClusterRole,omitempty"`
}

const (

	// ResourceNamespace , number
	ResourceNamespace v1.ResourceName = "namespaces"
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
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              AddonSpec `json:"spec"`
}

// AddonList is a list of Addons.
type AddonList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Addon `json:"items"`
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

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Release refers to compiled slug file versions
type Release struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ReleaseSpec `json:"spec"`
}

// SourceType refers to the source of the build
type SourceType string

const (
	// GitHubSource means the build came from a webhook
	GitHubSource SourceType = "github"
	// GitLocalSource means the build came from the git local server
	GitLocalSource SourceType = "local"
)

// ReleaseSpec holds specification parameters of a release
type ReleaseSpec struct {
	// The URL of the git remote server to download the git revision tarball
	GitRemote string `json:"gitRemote"`
	// DEPRECATED, in flavor of .commitInfo.ID
	GitRevision   string     `json:"gitRevision"`
	GitRepository string     `json:"gitRepository"`
	GitBranch     string     `json:"gitBranch"`
	BuildRevision string     `json:"buildRevision"`
	AutoDeploy    bool       `json:"autoDeploy"`
	ExpireAfter   int32      `json:"expireAfter"`
	DeployName    string     `json:"deployName"`
	Build         bool       `json:"build"`
	HeadCommit    HeadCommit `json:"headCommit"`
	// DEPRECATED, the authToken for each release is populated by a secret
	// the lifecycle of the token is managed by a controller
	AuthToken string     `json:"authToken"` // expirable token
	Source    SourceType `json:"sourceType"`
}

// HeadCommit holds information about a particular commit
type HeadCommit struct {
	ID        string `json:"id"`
	Author    string `json:"author"`
	AvatarURL string `json:"avatar-url"`
	Compare   string `json:"compare"`
	Message   string `json:"message"`
	URL       string `json:"url"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ReleaseList is a list of Release
type ReleaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Release `json:"items"`
}

// TokenType refers to a jwt token claim to specify the type of the token
// some types have restricted access and scope in the platform
type TokenType string

const (
	// AdminTokenType has unsrestricted access to all endpoints
	AdminTokenType TokenType = "admin"
	// SystemTokenType allows interaction only between machine with l
	// imited access scope to endpoints
	SystemTokenType  TokenType = "system"
	RegularTokenType TokenType = "regular"
)

// User identifies an user on the platform
type User struct {
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	Customer     string    `json:"customer"`
	Organization string    `json:"org"`
	Sub          string    `json:"sub"`
	Groups       []string  `json:"groups"`
	Type         TokenType `json:"kolihub.io/type"` // Origin refers to the type of the token (system or regular)
	jwt.StandardClaims
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Domain are a way for users to "claim" a domain and be able to create
// ingresses
type Domain struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DomainSpec   `json:"spec,omitempty"`
	Status DomainStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DomainList is a List of Domain
type DomainList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Domain `json:"items"`
}

// DomainStatus represents information about the status of a domain.
type DomainStatus struct {
	// The state of the domain, an empty state means it's a new resource
	// +optional
	Phase DomainPhase `json:"phase,omitempty"`
	// A human readable message indicating details about why the domain claim is in this state.
	// +optional
	Message string `json:"message,omitempty"`
	// A brief CamelCase message indicating details about why the domain claim is in this state. e.g. 'AlreadyClaimed'
	// +optional
	Reason string `json:"reason,omitempty"`
	// The last time the resource was updated
	LastUpdateTime *metav1.Time `json:"lastUpdateTime,omitempty"`
	// DeletionTimestamp it's a temporary field to work around the issue:
	// https://github.com/kubernetes/kubernetes/issues/40715, once it's solved,
	// remove this field and use the DeletionTimestamp from metav1.ObjectMeta
	DeletionTimestamp *metav1.Time `json:"deletionTimestamp,omitempty"`
}

// DomainSpec represents information about a domain claim
type DomainSpec struct {
	// PrimaryDomain is the name of the primary domain, to set the resource as primary,
	// 'name' and 'primary' must have the same value.
	// +required
	PrimaryDomain string `json:"primary,omitempty"`
	// Sub is the label of the Primary Domain to form a subdomain
	// +optional
	Sub string `json:"sub,omitempty"`
	// Delegates contains a list of namespaces that are allowed to use this domain.
	// New domain resources could be referenced to primary ones using the 'parent' key.
	// A wildcard ("*") allows delegate access to all namespaces in the cluster.
	// +optional
	Delegates []string `json:"delegates,omitempty"`
	// Parent refers to the namespace where the primary domain is in.
	// It only makes sense when the type of the domain is set to 'shared',
	// +optional
	Parent string `json:"parent,omitempty"`
}

// DomainPhase is a label for the condition of a domain at the current time.
type DomainPhase string

const (
	// DomainStatusNew means it's a new resource and the phase it's not set
	DomainStatusNew DomainPhase = ""
	// DomainStatusOK means the domain doesn't have no pending operations or prohibitions,
	// and new ingresses could be created using the target domain.
	DomainStatusOK DomainPhase = "OK"
	// DomainStatusPending indicates that a request to create a new domain
	// has been received and is being processed.
	DomainStatusPending DomainPhase = "Pending"
	// DomainStatusFailed means the resource has failed on claiming the domain
	DomainStatusFailed DomainPhase = "Failed"
)
