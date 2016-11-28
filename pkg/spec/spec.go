package spec

import (
	"fmt"

	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/pkg/api/unversioned"
	"k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/pkg/apis/apps/v1alpha1"
	"k8s.io/client-go/1.5/pkg/labels"
	"k8s.io/client-go/1.5/tools/cache"
)

const (
	koliLabelPrefix = "sys.io"
)

// Addon defines integration with external resources
type Addon struct {
	unversioned.TypeMeta `json:",inline"`
	v1.ObjectMeta        `json:"metadata,omitempty"`
	Spec                 AddonSpec `json:"spec"`
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

// AddonInterface represents the implementation of generic apps
type AddonInterface interface {
	CreateConfigMap() error
	CreatePetSet() error
	UpdatePetSet(old *v1alpha1.PetSet) error
	DeleteApp() error
	GetAddon() *Addon
}

// DefaultComputeResources http://kubernetes.io/docs/admin/resourcequota/#compute-resource-quota
type DefaultComputeResources struct {
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
}

// DefaultResourceQuota http://kubernetes.io/docs/admin/resourcequota/#object-count-quota
type DefaultResourceQuota struct {
	ConfigMaps             string
	PersistentVolumeClaims string
	Pods                   string
	ReplicationController  string
	ResourceQuotas         string
	Services               string
	ServicesLoadBalancers  string
	ServicesNodePorts      string
	Secrets                string
}

// User identifies a user on the platform
type User struct {
	ID               string                  `json:"id"`
	Username         string                  `json:"username"`
	Organization     string                  `json:"org"`
	Customer         string                  `json:"customer"`
	ObjectResources  DefaultResourceQuota    `json:"objectresources"`
	ComputeResources DefaultComputeResources `json:"computeresources"`
}

// Label wraps a labels.Set
type Label struct {
	labels.Set
}

// Remove a key from the labels.Set using a pre-defined prefix
func (l *Label) Remove(key string) {
	delete(l.Set, fmt.Sprintf("%s/%s", koliLabelPrefix, key))
}

// Add values to a labels.Set using a pre-defined prefix
func (l *Label) Add(mapLabels map[string]string) {
	for key, value := range mapLabels {
		l.Set[fmt.Sprintf("%s/%s", koliLabelPrefix, key)] = value
	}
}

// NewLabel generates a new *spec.Label
func NewLabel() *Label {
	return &Label{Set: map[string]string{}}
}
