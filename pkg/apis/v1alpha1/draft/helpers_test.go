package draft

import (
	"strconv"
	"testing"

	platform "kolihub.io/koli/pkg/apis/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

func TestDeploymentDraftMapValues(t *testing.T) {
	var (
		expNotes = map[string]string{
			platform.AnnotationSetupStorage:     "true",
			platform.AnnotationAuthToken:        "jwt.auth.token",
			platform.AnnotationAutoDeploy:       "true",
			platform.AnnotationBuild:            "true",
			platform.AnnotationBuildRevision:    "10",
			platform.AnnotationGitBranch:        "refs/heads/master",
			platform.AnnotationBuildSource:      "git",
			platform.AnnotationGitCompare:       "https://github.com/[compare-sha]",
			platform.AnnotationGitHubSecretHook: "webhook-secret",
			platform.AnnotationGitRepository:    "sandromello/python-getting-started",
			platform.AnnotationGitHubUser:       "github|120412",
			platform.AnnotationGitRevision:      "6e8d52d02062f937c3376a77d6b03eb5a1e872f2",
		}
		expLabels = map[string]string{
			platform.LabelStoragePlan: "foo-plan-5g",
			platform.LabelClusterPlan: "foo-plan",
			platform.LabelDefault:     "true",
		}
	)
	kd := newKubernetesDeployment("foo", "foo-ns", expNotes, expLabels, nil, v1.Container{})
	d := NewDeployment(kd)

	if strconv.FormatBool(d.HasSetupPVCAnnotation()) != expNotes[platform.AnnotationSetupStorage] {
		t.Errorf("GOT: %v, EXPECTED: %v", d.HasSetupPVCAnnotation(), expNotes[platform.AnnotationSetupStorage])
	}
	if d.AuthToken() != expNotes[platform.AnnotationAuthToken] {
		t.Errorf("GOT: %v, EXPECTED: %v", d.AuthToken(), expNotes[platform.AnnotationAuthToken])
	}
	if strconv.FormatBool(d.HasAutoDeployAnnotation()) != expNotes[platform.AnnotationAutoDeploy] {
		t.Errorf("GOT: %v, EXPECTED: %v", d.HasAutoDeployAnnotation(), expNotes[platform.AnnotationAutoDeploy])
	}
	if strconv.FormatBool(d.HasBuildAnnotation()) != expNotes[platform.AnnotationBuild] {
		t.Errorf("GOT: %v, EXPECTED: %v", d.HasBuildAnnotation(), expNotes[platform.AnnotationBuild])
	}
	if strconv.Itoa(d.BuildRevision()) != expNotes[platform.AnnotationBuildRevision] {
		t.Errorf("GOT: %d, EXPECTED: %v", d.BuildRevision(), expNotes[platform.AnnotationBuildRevision])
	}
	if d.GitBranch() != expNotes[platform.AnnotationGitBranch] {
		t.Errorf("GOT: %v, EXPECTED: %v", d.GitBranch(), expNotes[platform.AnnotationGitBranch])
	}
	if d.GitSource() != expNotes[platform.AnnotationBuildSource] {
		t.Errorf("GOT: %v, EXPECTED: %v", d.GitSource(), expNotes[platform.AnnotationBuildSource])
	}
	if d.GitCompare() != expNotes[platform.AnnotationGitCompare] {
		t.Errorf("GOT: %v, EXPECTED: %v", d.GitCompare(), expNotes[platform.AnnotationGitCompare])
	}
	if d.GitHubWebHookSecret() != expNotes[platform.AnnotationGitHubSecretHook] {
		t.Errorf("GOT: %v, EXPECTED: %v", d.GitHubWebHookSecret(), expNotes[platform.AnnotationGitHubSecretHook])
	}
	if d.GitRepository() != expNotes[platform.AnnotationGitRepository] {
		t.Errorf("GOT: %v, EXPECTED: %v", d.GitRepository(), expNotes[platform.AnnotationGitRepository])
	}
	if d.GitHubUser() != expNotes[platform.AnnotationGitHubUser] {
		t.Errorf("GOT: %v, EXPECTED: %v", d.GitHubUser(), expNotes[platform.AnnotationGitHubUser])
	}
	sha, err := d.GitRevision()
	if err != nil {
		t.Fatalf("unexpected error retrieving git revision: %v", err)
	}
	if sha.Full() != expNotes[platform.AnnotationGitRevision] {
		t.Errorf("GOT: %v, EXPECTED: %v", sha.Full(), expNotes[platform.AnnotationGitRevision])
	}
}

func newKubernetesDeployment(name, ns string, notes, labels, selector map[string]string, container v1.Container) *v1beta1.Deployment {
	return &v1beta1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			UID:         uuid.NewUUID(),
			Name:        name,
			Namespace:   ns,
			Annotations: notes,
			Labels:      labels,
		},
		Spec: v1beta1.DeploymentSpec{
			Strategy: v1beta1.DeploymentStrategy{
				Type: v1beta1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &v1beta1.RollingUpdateDeployment{
					MaxUnavailable: func() *intstr.IntOrString { i := intstr.FromInt(0); return &i }(),
					MaxSurge:       func() *intstr.IntOrString { i := intstr.FromInt(0); return &i }(),
				},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: selector},
				Spec:       v1.PodSpec{Containers: []v1.Container{container}},
			},
		},
	}
}
