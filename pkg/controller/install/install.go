package install

import (
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	platform "kolihub.io/koli/pkg/apis/core/v1alpha1"
)

func init() {
	platform.PlatformRegisteredRoles = []platform.PlatformRole{
		platform.RoleAttachAllow,
		platform.RoleAutoScaleAllow,
		platform.RoleExecAllow,
		platform.RolePortForwardAllow,
		platform.RoleAddonManagement,
	}
	platform.PlatformRegisteredResources = &platform.ResourceList{
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
