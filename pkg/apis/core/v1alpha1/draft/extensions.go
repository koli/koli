package draft

import (
	"fmt"
	"regexp"

	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/client-go/kubernetes/scheme"
	platform "kolihub.io/koli/pkg/apis/core/v1alpha1"
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

// Copy performs a deep copy of the resource
func (d *Deployment) Copy() (*Deployment, error) {
	objCopy, err := scheme.Scheme.DeepCopy(d.GetObject())
	if err != nil {
		return nil, fmt.Errorf("Failed deep copying Deployment resource")
	}
	return NewDeployment(objCopy.(*v1beta1.Deployment)), nil
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

// Copy performs a deep copy of the resource
func (d *Ingress) Copy() (*Ingress, error) {
	objCopy := d.DeepCopy()
	if objCopy == nil {
		return nil, fmt.Errorf("Failed deep copying ingress")
	}
	return NewIngress(objCopy), nil
}

// GetObject returns the original resource
func (i *Ingress) GetObject() *v1beta1.Ingress {
	return &i.Ingress
}

// DomainPrimaryKeys returns annotations matching domains, e.g.: 'kolihub.io/domain.tld'
func (i *Ingress) DomainPrimaryKeys() (m map[string]string) {
	domReg := regexp.MustCompile(`kolihub.io/.+\.+`)
	for key, value := range i.Annotations {
		if domReg.MatchString(key) {
			if m == nil {
				m = map[string]string{}
			}
			m[key] = value
		}
	}
	return
}
