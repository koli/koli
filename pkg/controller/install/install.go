package install

import (
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/resource"
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
		api.ResourcePods:       resource.Quantity{},
		api.ResourceConfigMaps: resource.Quantity{},
		api.ResourceSecrets:    resource.Quantity{},
		// TODO: vfuture
		// api.ResourcePersistentVolumeClaims: resource.Quantity{},
		// api.ResourceRequestsCPU:            resource.Quantity{},
		// api.ResourceRequestsMemory:         resource.Quantity{},
		// ResourceLimitsCPU:                  resource.Quantity{},
		// ResourceLimitsMemory:               resource.Quantity{},
		// ResourceRequestsStorage:            resource.Quantity{},
	}
}
