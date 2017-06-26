package controller

import (
	"fmt"
	"testing"

	platform "kolihub.io/koli/pkg/apis/v1alpha1"
	"kolihub.io/koli/pkg/controller/informers"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/pkg/api"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/tools/record"

	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	koli "kolihub.io/koli/pkg/clientset"
)

type fixture struct {
	t *testing.T

	client    *fake.Clientset
	tprClient koli.CoreInterface
	// Objects to put in the store.

	objects []runtime.Object
	plans   []runtime.Object
}

func newFixture(t *testing.T, objects, plans []runtime.Object) *fixture {
	f := &fixture{}
	f.t = t
	f.objects = objects
	f.plans = plans
	return f
}

func (f *fixture) newController() (*AppManagerController, informers.SharedInformerFactory) {
	f.client = fake.NewSimpleClientset(f.objects...)
	informers := informers.NewSharedInformerFactory(f.client, 0)
	ctrl := NewAppManagerController(
		informers.Deployments().Informer(),
		informers.ServicePlans().Informer(f.tprClient),
		f.client,
	)
	ctrl.recorder = record.NewFakeRecorder(100)
	for _, o := range f.objects {
		ctrl.dpInf.GetStore().Add(o)
	}
	for _, o := range f.plans {
		ctrl.planInf.GetStore().Add(o)
	}
	return ctrl, informers
}

func TestSyncPersistentVolume(t *testing.T) {
	var (
		expectedName   = "foo"
		expPatchDeploy = []byte(`{"metadata": {"annotations": {"kolihub.io/setup-storage": "false"}}}`)
		notes          = map[string]string{platform.AnnotationSetupStorage: "true"}
		labels         = map[string]string{platform.LabelStoragePlan: "foo-plan-5g"}
		d              = newDeployment(expectedName, "dev-coyote-acme", notes, labels, nil, v1.Container{})
		plan           = newStoragePlan("foo-plan-5g", "dev-coyote-acme", resource.MustParse("5Gi"))
	)
	f := newFixture(t, []runtime.Object{d}, []runtime.Object{plan})
	c, _ := f.newController()

	if err := c.syncHandler(getKey(d, t)); err != nil {
		t.Fatalf("unexpected error syncing: %v", err)
	}

	for _, action := range f.client.Actions() {
		switch actionT := action.(type) {
		case core.CreateAction:
			switch obj := actionT.GetObject().(type) {
			case *v1.PersistentVolumeClaim:
				if obj.Name != fmt.Sprintf("d-%s", expectedName) {
					t.Errorf("GOT NAME: %v, EXPECTED: %v", obj.Name, fmt.Sprintf("d-%s", expectedName))
				}
				if obj.Spec.Resources.Requests[v1.ResourceStorage] != plan.Spec.Storage {
					t.Errorf("GOT: %#v, EXPECTED: %#v", obj.Spec.Resources, plan.Spec.Resources)
				}
			default:
				t.Errorf("unexpected type of resource: %T, OBJ: %s", obj, obj)
			}
		case core.PatchActionImpl:
			if string(expPatchDeploy) != string(actionT.Patch) {
				t.Errorf("GOT: %s, EXPECTED: %s", string(actionT.Patch), expPatchDeploy)
			}
		default:
			t.Errorf("unexpected type of action: %T, OBJ: %s", actionT, action)
		}
	}
}

func TestSyncWithInvalidNamespace(t *testing.T) {
	d := newDeployment("foo", "invalid-namespace", nil, nil, nil, v1.Container{})
	f := newFixture(t, []runtime.Object{d}, nil)
	c, _ := f.newController()

	if err := c.syncHandler(getKey(d, t)); err != nil {
		t.Fatalf("unexpected error syncing: %v", err)
	}
}

func TestSyncWithNonExistentPlan(t *testing.T) {
	var (
		expectedPlan = "foo-plan-5g"
		expectedMsg  = fmt.Sprintf(`Storage Plan "%s" not found`, expectedPlan)
		notes        = map[string]string{platform.AnnotationSetupStorage: "true"}
		labels       = map[string]string{platform.LabelStoragePlan: expectedPlan}
		d            = newDeployment("foo", "dev-coyote-acme", notes, labels, nil, v1.Container{})
		plan         = newStoragePlan("wrong-storage-plan", "dev-coyote-acme", resource.MustParse("5Gi"))
	)
	f := newFixture(t, []runtime.Object{d}, []runtime.Object{plan})
	c, _ := f.newController()

	if err := c.syncHandler(getKey(d, t)); err != nil && err.Error() != expectedMsg {
		t.Errorf("GOT: %v, EXPECTED: %v", err, expectedMsg)
	}
}

func newStoragePlan(name, ns string, storage resource.Quantity) *platform.Plan {
	return &platform.Plan{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: platform.PlanSpec{
			Type:    platform.PlanTypeStorage,
			Storage: storage,
		},
	}
}

func newDeployment(name, ns string, notes, labels, selector map[string]string, container v1.Container) *v1beta1.Deployment {
	return &v1beta1.Deployment{
		TypeMeta: metav1.TypeMeta{APIVersion: api.Registry.GroupOrDie(v1beta1.GroupName).GroupVersion.String()},
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

func getKey(d *v1beta1.Deployment, t *testing.T) string {
	key, err := keyFunc(d)
	if err != nil {
		t.Fatalf("Unexpected error getting key for deployment %v: %v", d.Name, err)
	}
	return key
}
