package platform

import (
	"kolihub.io/koli/pkg/spec"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	rbac "k8s.io/client-go/pkg/apis/rbac/v1beta1"
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
			ObjectMeta: metav1.ObjectMeta{Name: string(spec.RoleExecAllow)},
			Rules:      GetExecRule(),
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: string(spec.RolePortForwardAllow)},
			Rules:      GetPortForwardRule(),
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: string(spec.RoleAutoScaleAllow)},
			Rules:      GetAutoScaleRule(),
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: string(spec.RoleAttachAllow)},
			Rules:      GetAttachRule(),
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: string(spec.RoleAddonManagement)},
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
			APIGroups: []string{"platform.koli.io/v1"},
			Resources: []string{"addons"},
			Verbs:     []string{"get", "watch", "list", "create", "update", "delete"},
		},
	}
}
