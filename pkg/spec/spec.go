package spec

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/labels"

	rbac "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KoliPrefixValue is used for creating annotations and labels
const (
	KoliPrefixValue    = "kolihub.io"
	ReleaseExpireAfter = 20
)

// PlatformRegisteredRoles contains all the cluster roles provisioned on the platform
var PlatformRegisteredRoles []PlatformRole

// PlatformRegisteredResources contains all the resources allowed for a user to configure
// in resource quotas: http://kubernetes.io/docs/admin/resourcequota/#Object-Count-Quota
var PlatformRegisteredResources *ResourceList

// Label wraps a labels.Set
type Label struct {
	labels.Set
	Prefix string
}

// Remove a key from the labels.Set using a pre-defined prefix
func (l *Label) Remove(key string) *Label {
	delete(l.Set, fmt.Sprintf("%s/%s", KoliPrefixValue, key))
	return l
}

// Exists verifies if the given key exists
func (l *Label) Exists(key string) bool {
	_, hasKey := l.Set[fmt.Sprintf("%s/%s", l.Prefix, key)]
	if hasKey {
		return true
	}
	return false
}

// Add values to a labels.Set using a pre-defined prefix
func (l *Label) Add(mapLabels map[string]string) *Label {
	for key, value := range mapLabels {
		l.Set[fmt.Sprintf("%s/%s", l.Prefix, key)] = value
	}
	return l
}

// NewLabel generates a new *spec.Label, if a prefix isn't provided
// it will use the the default one: spec.KoliPrefixValue.
func NewLabel(prefixS ...string) *Label {
	var prefix string
	if len(prefixS) == 0 {
		prefix = KoliPrefixValue
	}
	if prefix == "" {
		// Default prefix if it's empty
		prefix = prefixS[0]
	}
	return &Label{Set: map[string]string{}, Prefix: prefix}
}

// KoliPrefix returns a value with the default prefix - spec.KoliPrefix
func KoliPrefix(value string) string {
	return fmt.Sprintf("%s/%s", KoliPrefixValue, value)
}

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
	repository := r.Spec.GitRepository
	if r.IsGitHubSource() {
		repository = filepath.Join(r.Namespace, r.Spec.DeployName)
	}
	urlPath := filepath.Join("releases", repository, r.Spec.GitRevision)
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
