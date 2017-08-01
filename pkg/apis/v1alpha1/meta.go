package v1alpha1

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	rbac "k8s.io/client-go/pkg/apis/rbac/v1beta1"
)

// KoliPrefixValue is used for creating annotations and labels
const (
	KoliPrefixValue    = "kolihub.io"
	ReleaseExpireAfter = 20
)

// IsValid validates if the user is valid verifying the email, customer and organization
func (u User) IsValid() bool {
	return len(u.Customer) > 0 && len(u.Organization) > 0 && len(u.Email) > 0
}

// PlatformRegisteredRoles contains all the cluster roles provisioned on the platform
var PlatformRegisteredRoles []PlatformRole

// PlatformRegisteredResources contains all the resources allowed for a user to configure
// in resource quotas: http://kubernetes.io/docs/admin/resourcequota/#Object-Count-Quota
var PlatformRegisteredResources *ResourceList

// GetRoleBinding retrieves a role binding for this role
func (r PlatformRole) GetRoleBinding(subjects []rbac.Subject) *rbac.RoleBinding {
	return &rbac.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: string(r)},
		Subjects:   subjects,
		RoleRef: rbac.RoleRef{
			Kind: "ClusterRole",
			Name: string(r), // must match role name
		},
	}
}

// IsRegisteredRole check if the role matches with the registered roles.
func (r PlatformRole) IsRegisteredRole() bool {
	for _, role := range PlatformRegisteredRoles {
		if r == role {
			return true
		}
	}
	return false
}

// Exists verifies if the slice contains the role
func (r PlatformRole) Exists(roles []PlatformRole) bool {
	for _, role := range roles {
		if r == role {
			return true
		}
	}
	return false
}

// NewPlatformRoles converts a string of comma separated roles to registered []PlatformRoles
func NewPlatformRoles(roles string) []PlatformRole {
	platformRoles := []PlatformRole{}
	for _, r := range strings.Split(roles, ",") {
		role := PlatformRole(r)
		if !role.IsRegisteredRole() {
			continue
		}
		platformRoles = append(platformRoles, role)
	}
	return platformRoles
}

// RemoveUnregisteredResources removes resources which are not registered on the platform
func (r *ResourceList) RemoveUnregisteredResources() {
	for resourceName := range *r {
		_, hasKey := (*PlatformRegisteredResources)[resourceName]
		if !hasKey {
			delete(*r, resourceName)
		}
	}
}

// Expired verifies if the creation time of the resource is expired.
func (r *Release) Expired() bool {
	expireAfter := r.Spec.ExpireAfter
	if expireAfter == 0 {
		expireAfter = ReleaseExpireAfter
	}
	createdAt := r.CreationTimestamp.Add(time.Duration(expireAfter) * time.Minute)
	if createdAt.Before(time.Now().UTC()) {
		return true
	}
	return false
}

// BuildRevision returns the revision as int, if the conversion fails returns 0
func (r *Release) BuildRevision() int {
	buildRev, _ := strconv.Atoi(r.Spec.BuildRevision)
	return buildRev
}

// IsGitHubSource check if the source of the build is from github
func (r *Release) IsGitHubSource() bool {
	return r.Spec.Source == GitHubSource
}

// GitCloneURL constructs the remote clone URL for the given release
func (r *Release) GitCloneURL() (string, error) {
	u, err := url.Parse(r.Spec.GitRemote)
	if err != nil {
		return "", fmt.Errorf("failed parsing url: %s", err)
	}
	gitRemoteURL := fmt.Sprintf("%s://jwt:%s@%s/%s", u.Scheme, r.Spec.AuthToken, u.Host, r.Spec.GitRepository)
	return gitRemoteURL + ".git", nil
}

// GitReleaseURL constructs the URL where the release must be stored
func (r *Release) GitReleaseURL(host string) string {
	urlPath := filepath.Join("releases", r.Namespace, r.Spec.DeployName, r.Spec.GitRevision)
	return fmt.Sprintf("%s/%s", host, urlPath)
}

func (d *Domain) HasFinalizer(finalizer string) bool {
	for _, f := range d.GetFinalizers() {
		if f == finalizer {
			return true
		}
	}
	return false
}

// IsPrimary validates if it's a primary domain
func (d *Domain) IsPrimary() bool {
	return len(d.Spec.Sub) == 0
}

// IsValidSharedDomain verifies if the shared domain it's a subdomain from the primary
func (d *Domain) IsValidSharedDomain() bool {
	return !d.IsPrimary() && d.IsValidDomain()
}

func (d *Domain) IsValidDomain() bool {
	if len(strings.Split(d.Spec.Sub, ".")) > 1 || len(strings.Split(d.Spec.PrimaryDomain, ".")) < 2 {
		return false
	}
	return true
}

func (d *Domain) GetDomain() string {
	if d.IsPrimary() {
		return d.GetPrimaryDomain()
	}
	return d.Spec.Sub + "." + d.Spec.PrimaryDomain
}

// GetDomainType returns the type of the resource: 'primary' or 'shared'
func (d *Domain) GetDomainType() string {
	if d.IsPrimary() {
		return "primary"
	}
	return "shared"
}

// GetPrimaryDomain returns the primary domain of the resource
func (d *Domain) GetPrimaryDomain() string {
	return d.Spec.PrimaryDomain
}

// HasDelegate verifies if the the resource has the target namespace in the delegates attribute
func (d *Domain) HasDelegate(namespace string) bool {
	for _, delegateNS := range d.Spec.Delegates {
		if delegateNS == namespace || delegateNS == "*" {
			return true
		}
	}
	return false
}

// IsOK verifies if the resource is in the OK state
func (d *Domain) IsOK() bool {
	if d.Status.Phase == DomainStatusOK {
		return true
	}
	return false
}

// CPU return the CPU from limits and requests respectively
func (p *Plan) CPU() (*resource.Quantity, *resource.Quantity) {
	return p.Spec.Resources.Limits.Cpu(),
		p.Spec.Resources.Requests.Cpu()
}

// Memory returns the memory from limits and requests respectively
func (p *Plan) Memory() (*resource.Quantity, *resource.Quantity) {
	return p.Spec.Resources.Limits.Memory(),
		p.Spec.Resources.Requests.Memory()
}

// Storage returns the storage ammount from the spec
func (p *Plan) Storage() *resource.Quantity {
	return &p.Spec.Storage
}

// IsDefaultType validate if the plan is PlanTypeDefault
func (p *Plan) IsDefaultType() bool {
	return p.Spec.Type == PlanTypeDefault
}

// IsStorageType validate if the plan is PlanTypeStorage
func (p *Plan) IsStorageType() bool {
	return p.Spec.Type == PlanTypeStorage
}
