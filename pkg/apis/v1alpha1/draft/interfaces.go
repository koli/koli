package draft

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

type Object interface {
	GetKubernetesObjectMeta() *metav1.ObjectMeta
	GetLabel(key string) *MapValue
	GetAnnotation(key string) *MapValue
	GetNamespaceMetadata() *NamespaceMeta
	SetAnnotation(key, value string)
	SetLabel(key, value string)
}

type DeploymentInterface interface {
	Object
	// Labels
	GetClusterPlan() *MapValue
	GetStoragePlan() *MapValue
	SetStoragePlan(planName string)
	SetClusterPlan(planName string)
	// Annotations
	BuildRevision() int
	HasAutoDeployAnnotation() bool
	HasSetupPVCAnnotation() bool
	HasBuildAnnotation() bool
	GitRepository() string
	GitRevision() (*SHA, error)
	GitBranch() string
	GitSource() string
	GitCompare() string
	GitHubUser() string
	GitHubWebHookSecret() string
	AuthToken() string

	GetObject() *v1beta1.Deployment
	HasMultipleReplicas() bool
	IsMarkedForDeletion() bool
	GetContainers() []v1.Container
	HasContainers() bool
	PodSpec() *v1.PodSpec
	DeepCopy() (*Deployment, error)
}
