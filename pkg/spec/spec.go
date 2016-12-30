package spec

import (
	"fmt"
	"strings"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/apis/rbac"
	"k8s.io/kubernetes/pkg/labels"
)

// KoliPrefixValue is used for creating annotations and labels
const KoliPrefixValue = "koli.io"

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
		ObjectMeta: api.ObjectMeta{Name: string(r)},
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
