package draft

import (
	"fmt"

	"k8s.io/client-go/pkg/api"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
	platform "kolihub.io/koli/pkg/apis/v1alpha1"
)

func (d *Deployment) GetClusterPlan() *MapValue {
	return &MapValue{Val: d.GetLabel(platform.LabelClusterPlan).String()}
}

func (d *Deployment) GetStoragePlan() *MapValue {
	return &MapValue{Val: d.GetLabel(platform.LabelStoragePlan).String()}
}

func (d *Deployment) SetStoragePlan(planName string) {
	d.SetLabel(platform.LabelStoragePlan, planName)
}

func (d *Deployment) SetClusterPlan(planName string) {
	d.SetLabel(platform.LabelClusterPlan, planName)
}

// GetObject returns the original resource
func (d *Deployment) GetObject() *v1beta1.Deployment {
	return &d.Deployment
}

// DeepCopy performs a deep copy of the resource
func (d *Deployment) DeepCopy() (*Deployment, error) {
	objCopy, err := api.Scheme.DeepCopy(d.GetObject())
	if err != nil {
		return nil, err
	}
	copied, ok := objCopy.(*v1beta1.Deployment)
	if !ok {
		return nil, fmt.Errorf("expected Deployment, got %#v", objCopy)
	}
	return NewDeployment(copied), nil
}

func (d Deployment) BuildRevision() int {
	return d.GetAnnotation(platform.AnnotationBuildRevision).AsInt()
}
func (d *Deployment) HasAutoDeployAnnotation() bool {
	return d.GetAnnotation(platform.AnnotationAutoDeploy).AsBool()
}
func (d *Deployment) HasSetupPVCAnnotation() bool {
	return d.GetAnnotation(platform.AnnotationSetupStorage).AsBool()
}
func (d *Deployment) HasBuildAnnotation() bool {
	return d.GetAnnotation(platform.AnnotationBuild).AsBool()
}
func (d *Deployment) GitRepository() string {
	return d.GetAnnotation(platform.AnnotationGitRepository).String()
}
func (d *Deployment) GitRevision() (*SHA, error) {
	return NewSha(d.GetAnnotation(platform.AnnotationGitRevision).String())
}
func (d *Deployment) GitBranch() string {
	return d.GetAnnotation(platform.AnnotationGitBranch).String()
}
func (d *Deployment) GitSource() string {
	return d.GetAnnotation(platform.AnnotationBuildSource).String()
}
func (d *Deployment) GitCompare() string {
	return d.GetAnnotation(platform.AnnotationGitCompare).String()
}
func (d *Deployment) GitHubUser() *MapValue {
	return d.GetAnnotation(platform.AnnotationGitHubUser)
}
func (d *Deployment) GitHubWebHookSecret() string {
	return d.GetAnnotation(platform.AnnotationGitHubSecretHook).String()
}
func (d *Deployment) AuthToken() string {
	return d.GetAnnotation(platform.AnnotationAuthToken).String()
}

func (d *Deployment) PodSpec() *v1.PodSpec          { return &d.Spec.Template.Spec }
func (d *Deployment) HasMultipleReplicas() bool     { return d.Spec.Replicas != nil && *d.Spec.Replicas > 1 }
func (d *Deployment) HasContainers() bool           { return len(d.Spec.Template.Spec.Containers) > 0 }
func (d *Deployment) GetContainers() []v1.Container { return d.Spec.Template.Spec.Containers }

// IsMarkedForDeletion verifies if the metadata.deletionTimestamp is set, meaning the resource
// is marked to be excluded
func (d *Deployment) IsMarkedForDeletion() bool { return d.DeletionTimestamp != nil }
