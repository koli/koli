package mutator

import (
	rbac "k8s.io/api/rbac/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	platform "kolihub.io/koli/pkg/apis/core/v1alpha1"
)

var (
	DefaultClusterRole = rbac.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: platform.DefaultClusterRole,
		},
		Rules: []rbac.PolicyRule{
			{
				APIGroups: []string{"", "extensions", platform.GroupName},
				Resources: []string{
					"deployments",
					"domains",
					"events",
					"ingresses",
					"releases",
					"replicasets",
					"resourcequotas",
				},
				Verbs: []string{"get", "watch", "list"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{
					"pods",
					"pods/attach",
					"pods/exec",
					"pods/log",
					"pods/portforward",
					"services",
				},
				Verbs: []string{
					"create",
					"delete",
					"deletecollection",
					"get",
					"list",
					"patch",
					"update",
					"watch",
				},
			},
			{
				APIGroups: []string{"extensions", platform.GroupName},
				Resources: []string{"deployments", "releases", "replicasets"},
				Verbs:     []string{"delete", "deletecollection"},
			},
		},
	}
)
