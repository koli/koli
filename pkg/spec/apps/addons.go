package apps

import (
	"fmt"

	"github.com/kolibox/koli/pkg/spec"

	"k8s.io/kubernetes/pkg/apis/apps"
	"k8s.io/kubernetes/pkg/client/cache"
	clientset "k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset"
)

// AddonInterface represents the implementation of generic apps
type AddonInterface interface {
	CreateConfigMap() error
	CreatePetSet(sp *spec.ServicePlan) error
	UpdatePetSet(old *apps.StatefulSet, sp *spec.ServicePlan) error
	DeleteApp() error
	GetAddon() *spec.Addon
}

// GetType retrieves the type of the add-on
func GetType(a *spec.Addon, c clientset.Interface, psetInf cache.SharedIndexInformer) (AddonInterface, error) {
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
