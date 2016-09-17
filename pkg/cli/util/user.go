package util

import (
	"strings"

	"k8s.io/kubernetes/pkg/client/unversioned/clientcmd"
)

// DefaultComputeResources http://kubernetes.io/docs/admin/resourcequota/#compute-resource-quota
type DefaultComputeResources struct {
	CPU    string
	Memory string
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

// UserMeta .
type UserMeta struct {
	ID               string
	Owner            string
	ObjectResources  DefaultResourceQuota
	ComputeResources DefaultComputeResources
}

// Objects joins the DefaultResourceQuota attributes into a comma
// delimited string
func (u *UserMeta) Objects() string {
	return strings.Join([]string{
		"configmaps=" + u.ObjectResources.ConfigMaps,
		"persistentvolumeclaims=" + u.ObjectResources.PersistentVolumeClaims,
		"pods=" + u.ObjectResources.Pods,
		"replicationcontrollers=" + u.ObjectResources.ReplicationController,
		"resourcequotas=" + u.ObjectResources.ResourceQuotas,
		"services=" + u.ObjectResources.Services,
		"services.loadbalancers=" + u.ObjectResources.ServicesLoadBalancers,
		"services.nodeports=" + u.ObjectResources.ServicesLoadBalancers,
		"secrets=" + u.ObjectResources.Secrets,
	}, ",")
}

// UserConfig retrieves the user metadata from a JWT Token
func UserConfig(clientConfig clientcmd.ClientConfig) *UserMeta {
	// TODO: Parse the config file and extract it from a JWT Token
	return &UserMeta{
		ID:    "21",
		Owner: "sandromello",
		// A POD born with default compute resources
		ComputeResources: DefaultComputeResources{
			CPU:    "0.2",
			Memory: "256Mi",
		},
		// When a namespace is created it must have resources hard quota.
		ObjectResources: DefaultResourceQuota{
			ConfigMaps:             "5",
			PersistentVolumeClaims: "4",
			Pods: "40",
			ReplicationController: "0",
			ResourceQuotas:        "5",
			Services:              "5",
			ServicesLoadBalancers: "0",
			ServicesNodePorts:     "0",
			Secrets:               "5",
		},
	}
}
