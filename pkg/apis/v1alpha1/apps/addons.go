package apps

import (
	"fmt"

	"kolihub.io/koli/pkg/spec"

	"k8s.io/client-go/kubernetes"
	v1beta1 "k8s.io/client-go/pkg/apis/apps/v1beta1"
	"k8s.io/client-go/tools/cache"
)

// AddonInterface represents the implementation of generic apps
type AddonInterface interface {
	CreateConfigMap() error
	CreatePetSet(sp *spec.Plan) error
	UpdatePetSet(old *v1beta1.StatefulSet, sp *spec.Plan) error
	DeleteApp() error
	GetAddon() *spec.Addon
}

// GetType retrieves the type of the add-on
func GetType(a *spec.Addon, c kubernetes.Interface, psetInf cache.SharedIndexInformer) (AddonInterface, error) {
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
