package install

import (
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/pkg/api/v1"
	"kolihub.io/koli/pkg/spec"
)

func init() {
	spec.PlatformRegisteredRoles = []spec.PlatformRole{
		spec.RoleAttachAllow,
		spec.RoleAutoScaleAllow,
		spec.RoleExecAllow,
		spec.RolePortForwardAllow,
		spec.RoleAddonManagement,
	}
	spec.PlatformRegisteredResources = &spec.ResourceList{
		v1.ResourcePods:       resource.Quantity{},
		v1.ResourceConfigMaps: resource.Quantity{},
		v1.ResourceSecrets:    resource.Quantity{},
		// TODO: vfuture
		// api.ResourcePersistentVolumeClaims: resource.Quantity{},
		// api.ResourceRequestsCPU:            resource.Quantity{},
		// api.ResourceRequestsMemory:         resource.Quantity{},
		// ResourceLimitsCPU:                  resource.Quantity{},
		// ResourceLimitsMemory:               resource.Quantity{},
		// ResourceRequestsStorage:            resource.Quantity{},
	}
}
