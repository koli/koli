package spec

import (
	"fmt"

	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/pkg/apis/apps/v1alpha1"
	"k8s.io/client-go/1.5/pkg/labels"
	"k8s.io/client-go/1.5/tools/cache"
)

const (
	// KoliPrefixValue is used for creating annotations and labels
	KoliPrefixValue = "koli.io"
)

// AddonInterface represents the implementation of generic apps
type AddonInterface interface {
	CreateConfigMap() error
	CreatePetSet() error
	UpdatePetSet(old *v1alpha1.PetSet) error
	DeleteApp() error
	GetAddon() *Addon
}

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

// GetApp retrieves the type of the add-on
func (a *Addon) GetApp(c *kubernetes.Clientset, psetInf cache.SharedIndexInformer) (AddonInterface, error) {
	switch a.Spec.Type {
	case "redis":
		return &Redis{client: c, addon: a, psetInf: psetInf}, nil
	case "memcached":
		return &Memcached{client: c, addon: a, psetInf: psetInf}, nil
	case "mysql":
		return &MySQL{client: c, addon: a, psetInf: psetInf}, nil
	default:
		// Generic add-on
	}
	return nil, fmt.Errorf("invalid add-on type (%s)", a.Spec.Type)
}

// Label wraps a labels.Set
type Label struct {
	labels.Set
	Prefix string
}

// Remove a key from the labels.Set using a pre-defined prefix
func (l *Label) Remove(key string) {
	delete(l.Set, fmt.Sprintf("%s/%s", KoliPrefixValue, key))
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
