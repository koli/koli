package platform

import (
	"github.com/kolibox/koli/pkg/spec"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/apis/rbac"
)

var (
	// RoleVerbs are the registered verbs allowed by default in the platform
	RoleVerbs = []string{
		"get", "watch", "list", "exec", "port-forward", "logs", "scale",
		"attach", "create", "describe", "delete", "update",
	}

	// RoleResources are the registered resources allowed by default in the platform
	RoleResources = []string{
		"pods", "deployments", "namespaces", "replicasets",
		"resourcequotas", "horizontalpodautoscalers",
	}
)

// GetRoles returns all the roles used by the platform
func GetRoles() []*rbac.ClusterRole {
	return []*rbac.ClusterRole{
		{
			ObjectMeta: api.ObjectMeta{Name: string(spec.RoleExecAllow)},
			Rules:      GetExecRule(),
		},
		{
			ObjectMeta: api.ObjectMeta{Name: string(spec.RolePortForwardAllow)},
			Rules:      GetPortForwardRule(),
		},
		{
			ObjectMeta: api.ObjectMeta{Name: string(spec.RoleAutoScaleAllow)},
			Rules:      GetAutoScaleRule(),
		},
		{
			ObjectMeta: api.ObjectMeta{Name: string(spec.RoleAttachAllow)},
			Rules:      GetAttachRule(),
		},
		{
			ObjectMeta: api.ObjectMeta{Name: string(spec.RoleAddonManagement)},
			Rules:      GetAddonManagementRule(),
		},
	}
}

// GetExecRule returns a policy which enables the execution the exec command on pods
func GetExecRule() []rbac.PolicyRule {
	return []rbac.PolicyRule{
		{
			APIGroups: []string{"*"},
			Resources: []string{"exec"},
			Verbs:     []string{"get", "create"},
		},
	}
}

// GetPortForwardRule returns a policy which enables the execution of portforward command on pods
func GetPortForwardRule() []rbac.PolicyRule {
	return []rbac.PolicyRule{
		{
			APIGroups: []string{"*"},
			Resources: []string{"portforward"},
			Verbs:     []string{"get", "create"},
		},
	}
}

// GetAutoScaleRule returns a policy which enables the execution of autoscale command on deployments
func GetAutoScaleRule() []rbac.PolicyRule {
	return []rbac.PolicyRule{
		{
			APIGroups: []string{"*"},
			Resources: []string{"horizontalpodautoscalers"},
			Verbs:     []string{"get", "create"},
		},
	}
}

// GetAttachRule returns a policy which enables the execution of attach command on pods
func GetAttachRule() []rbac.PolicyRule {
	return []rbac.PolicyRule{
		{
			APIGroups: []string{"*"},
			Resources: []string{"attach"},
			Verbs:     []string{"get", "create"},
		},
	}
}

// GetPodManagementRule returns a policy which enables the execution of
// exec, portforward, autoscale and attach commands
func GetPodManagementRule() []rbac.PolicyRule {
	return []rbac.PolicyRule{
		{
			APIGroups: []string{"*"},
			Resources: []string{"exec", "portforward", "attach", "horizontalpodautoscalers", "attach"},
			Verbs:     []string{"get", "create"},
		},
	}
}

// GetAddonManagementRule returns a policy which enables the management of addons resources
func GetAddonManagementRule() []rbac.PolicyRule {
	return []rbac.PolicyRule{
		{
			APIGroups: []string{"platform.koli.io/v1alpha1"},
			Resources: []string{"addons"},
			Verbs:     []string{"get", "watch", "list", "create", "update", "delete"},
		},
	}
}
