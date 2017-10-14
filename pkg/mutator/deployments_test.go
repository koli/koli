package mutator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"testing"

	"github.com/gorilla/mux"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	fakerest "k8s.io/client-go/rest/fake"
	core "k8s.io/client-go/testing"
	platform "kolihub.io/koli/pkg/apis/core/v1alpha1"
	"kolihub.io/koli/pkg/util"
)

func newDeployment(name, ns string, notes, labels, selector map[string]string, container v1.Container) *v1beta1.Deployment {
	return &v1beta1.Deployment{
		TypeMeta: metav1.TypeMeta{APIVersion: v1beta1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   ns,
			Annotations: notes,
			Labels:      labels,
		},
		Spec: v1beta1.DeploymentSpec{
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: selector},
				Spec:       v1.PodSpec{Containers: []v1.Container{container}},
			},
		},
	}
}

func newImmutableAnnotations(value string) map[string]string {
	obj := map[string]string{}
	for _, immutableKey := range immutableAnnotations {
		obj[immutableKey] = value
	}
	return obj
}

func objBody(object interface{}) io.ReadCloser {
	output, err := json.MarshalIndent(object, "", "")
	if err != nil {
		panic(err)
	}
	return ioutil.NopCloser(bytes.NewReader([]byte(output)))
}

func newComputeResources() v1.ResourceRequirements {
	return v1.ResourceRequirements{
		Limits: v1.ResourceList{
			v1.ResourceCPU:    resource.MustParse("100"),
			v1.ResourceMemory: resource.MustParse("1Gi"),
		},
		Requests: v1.ResourceList{
			v1.ResourceCPU:    resource.MustParse("100"),
			v1.ResourceMemory: resource.MustParse("1Gi"),
		},
	}
}

func doRequest(method, url string, obj interface{}) ([]byte, *http.Response, error) {
	var reqBody []byte
	var err error
	if method == "PATCH" {
		var err error
		var ori runtime.Object
		var new runtime.Object
		switch t := obj.(type) {
		case *v1beta1.Deployment:
			ori = &v1beta1.Deployment{}
			new = obj.(*v1beta1.Deployment)
			reqBody, err = util.StrategicMergePatch(extensionsCodec, ori, new)
		case *v1beta1.Ingress:
			ori = &v1beta1.Ingress{}
			new = obj.(*v1beta1.Ingress)
			reqBody, err = util.StrategicMergePatch(extensionsCodec, ori, new)
		case []byte:
			reqBody = t
		default:
			return nil, nil, fmt.Errorf("found an unknown type: %T, %#v", obj, obj)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("failed generating patch diff: %v", err)
		}
	} else {
		reqBody, err = json.Marshal(obj)
		if err != nil {
			return nil, nil, fmt.Errorf("failed encoding object: %v", err)
		}
	}

	httpReq, err := http.NewRequest(method, url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, nil, fmt.Errorf("unexpected error: %#v", err)
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, nil, fmt.Errorf("unexpected error: %#v", err)
	}

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("failed reading response[%d]: %v", resp.StatusCode, err)
	}
	return respBody, resp, nil
}
func TestDeploymentOnCreate(t *testing.T) {
	var (
		deployName                 = "foo-app"
		planName                   = "foo-plan"
		storagePlan                = "foo-plan-5g"
		requestStorage             = resource.MustParse("5Gi")
		computeResources           = newComputeResources()
		expectedTemplateObjectMeta = metav1.ObjectMeta{Labels: map[string]string{"app": "foo"}}
		expectedVolumes            = []v1.Volume{
			{
				Name: fmt.Sprintf("d-%s", deployName),
				VolumeSource: v1.VolumeSource{
					PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
						ClaimName: fmt.Sprintf("d-%s", deployName),
						ReadOnly:  false, // Disk based services will always be RW
					},
				},
			},
		}
		expectedSecContext = &v1.PodSecurityContext{
			RunAsUser:    func() *int64 { i := int64(2000); return &i }(),
			RunAsNonRoot: func() *bool { i := bool(false); return &i }(),
			FSGroup:      func() *int64 { i := int64(2000); return &i }(),
		}
	)
	h := Handler{allowedImages: []string{"busybox"}}
	// Fake Clients
	responseHeader := http.Header{"Content-Type": []string{"application/json"}}
	h.tprClient = &fakerest.RESTClient{
		// APIRegistry:          api.Registry,
		NegotiatedSerializer: scheme.Codecs,
		Client: fakerest.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/namespaces/koli-system/plans" {
				t.Fatalf("unexpected url path: %v", req.URL.Path)
			}
			pList := &platform.PlanList{
				Items: []platform.Plan{
					{
						ObjectMeta: metav1.ObjectMeta{Name: planName,
							Labels: map[string]string{platform.LabelDefault: "true"},
						},
						Spec: platform.PlanSpec{Resources: computeResources},
					},
					{
						ObjectMeta: metav1.ObjectMeta{Name: storagePlan},
						Spec:       platform.PlanSpec{Storage: requestStorage, Type: platform.PlanTypeStorage},
					},
				},
			}
			return &http.Response{StatusCode: 200, Header: responseHeader, Body: objBody(pList)}, nil
		}),
	}

	var err error
	h.usrClientset, err = kubernetes.NewForConfig(&rest.Config{})
	if err != nil {
		t.Fatalf("unexpected error getting rest client: %v", err)
	}

	extensionsClient := fakerest.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
		d := &v1beta1.Deployment{}
		if err := json.NewDecoder(req.Body).Decode(d); err != nil {
			t.Fatalf("failed encoding deployment request: %v", err)
		}
		return &http.Response{StatusCode: 201, Header: responseHeader, Body: objBody(d)}, nil
	})
	h.usrClientset.Extensions().RESTClient().(*rest.RESTClient).Client = extensionsClient

	// Mux Fake Server
	r := mux.NewRouter()
	r.HandleFunc("/{namespace}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.DeploymentsOnCreate(w, r)
	})).Methods("POST")
	ts := httptest.NewServer(r)
	defer ts.Close()

	// Simulate Requests
	new := &v1beta1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Name:   deployName,
		Labels: map[string]string{platform.LabelStoragePlan: storagePlan},
	}}
	new.Spec.Template.ObjectMeta = expectedTemplateObjectMeta
	podSpec := &new.Spec.Template.Spec
	podSpec.Volumes = []v1.Volume{{Name: "avolume"}}
	podSpec.Containers = []v1.Container{{Name: "test", Image: "busybox"}}
	podSpec.SecurityContext = &v1.PodSecurityContext{}

	reqBody, err := json.Marshal(new)
	if err != nil {
		t.Fatalf("failed encoding deployment: %v", err)
	}
	req, err := http.NewRequest("POST", ts.URL+"/default", bytes.NewBuffer(reqBody))
	if err != nil {
		t.Fatalf("unexpected error: %#v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %#v", err)
	}

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed reading response[%d]: %v", resp.StatusCode, err)
	}
	if resp.StatusCode != 201 {
		t.Fatalf("got error processing request[%d]: %v", resp.StatusCode, string(respBody))
	}
	got := &v1beta1.Deployment{}
	if err := json.Unmarshal(respBody, got); err != nil {
		t.Fatalf("unexpected error unmarshaling response: %v. Obj: %v", err, string(respBody))
	}

	if got.Labels[platform.LabelClusterPlan] != planName {
		t.Errorf("GOT: %#v, EXPECTED: %v", got.Labels[platform.LabelClusterPlan], planName)
	}

	// allow .spec.template.metadata
	if !reflect.DeepEqual(got.Spec.Template.ObjectMeta, expectedTemplateObjectMeta) {
		t.Errorf("GOT: %#v, EXPECTED: %#v", got.Spec.Template.ObjectMeta, expectedTemplateObjectMeta)
	}

	podSpec = &got.Spec.Template.Spec
	if len(podSpec.Containers) != 1 {
		t.Errorf("GOT: %d, EXPECTED: 1", len(podSpec.Containers))
	}

	if !reflect.DeepEqual(podSpec.Volumes, expectedVolumes) {
		t.Errorf("GOT: %#v, EXPECTED: %#v", podSpec.Volumes, expectedVolumes)
	}

	if got.Annotations[platform.AnnotationSetupStorage] != "true" {
		t.Errorf("GOT: %#v, EXPECTED: \"true\"", got.Annotations[platform.AnnotationSetupStorage])
	}

	if !reflect.DeepEqual(podSpec.Containers[0].Resources, computeResources) {
		t.Errorf("GOT: %#v, EXPECTED: %#v", podSpec.Containers[0].Resources, computeResources)
	}

	if !reflect.DeepEqual(podSpec.SecurityContext, expectedSecContext) {
		t.Errorf("GOT: %#v, EXPECTED: %#v", podSpec.SecurityContext, expectedSecContext)
	}
}

func TestDeploymentOnPatch(t *testing.T) {
	var (
		expectedAnnotations = newImmutableAnnotations("customvalue")
		ns, appName         = "default", "foo-app"
		expectedPlan        = "foo-plan"
		expectedStoragePlan = "foo-plan-5g"
		expectedImage       = "quay.io/koli/postgres"
		computeResources    = newComputeResources()
		h                   = Handler{allowedImages: []string{expectedImage}}
		router              = mux.NewRouter()
	)
	router.HandleFunc("/{namespace}/deployments/{deploy}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.DeploymentsOnMod(w, r)
	})).Methods("PATCH")
	ts := httptest.NewServer(router)
	defer ts.Close()

	// Fake TPR Client
	h.tprClient = &fakerest.RESTClient{
		// APIRegistry:          api.Registry,
		NegotiatedSerializer: scheme.Codecs,
		Client: fakerest.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			var plan *platform.Plan
			switch req.URL.Path {
			case "/namespaces/koli-system/plans/" + expectedStoragePlan:
				plan = &platform.Plan{
					ObjectMeta: metav1.ObjectMeta{Name: expectedStoragePlan},
					Spec: platform.PlanSpec{
						Storage: resource.MustParse("5Gi"),
						Type:    platform.PlanTypeStorage,
					},
				}
			case "/namespaces/koli-system/plans/" + expectedPlan:
				plan = &platform.Plan{
					ObjectMeta: metav1.ObjectMeta{Name: expectedPlan},
					Spec:       platform.PlanSpec{Resources: computeResources},
				}
			default:
				t.Fatalf("unexpected url path: %v", req.URL.Path)
			}
			return &http.Response{
				StatusCode: 200,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       objBody(plan),
			}, nil
		}),
	}

	original := newDeployment(appName, ns, expectedAnnotations, nil, nil, v1.Container{})
	h.clientset = fake.NewSimpleClientset([]runtime.Object{original}...)
	h.usrClientset = fake.NewSimpleClientset()

	new := newDeployment(appName, ns, nil, nil, nil, v1.Container{})
	// Add storage and a default plan
	new.Labels = map[string]string{
		platform.LabelStoragePlan: expectedStoragePlan,
		platform.LabelClusterPlan: expectedPlan,
	}

	// Mutate DeploymentSpec
	new.Spec.Paused = true
	new.Spec.MinReadySeconds = int32(250)

	podSpec := &new.Spec.Template.Spec
	// Mutate podSpec
	podSpec.Containers = []v1.Container{{
		Name:         appName,
		Args:         []string{"--args", "myargument"},
		Command:      []string{"/bin/sh", "-c", "mycommand"},
		Env:          []v1.EnvVar{{Name: "MYENV", Value: "env-value"}},
		Image:        expectedImage,
		VolumeMounts: []v1.VolumeMount{{Name: "an-existing-volume", MountPath: "/tmp"}},
	}}

	_, _, err := doRequest("PATCH", ts.URL+"/default/deployments/"+appName, new)
	if err != nil {
		t.Fatalf("unexpected error executing request: %v", err)
	}

	// setup-storage annotation will be set when a storage plan (label) exists
	new.Annotations = map[string]string{platform.AnnotationSetupStorage: "true"}
	// compute resources will be set when a cluster plan (label) exists
	podSpec.Containers[0].Resources = computeResources
	// the original annotations only have immutable keys, it must be cleared otherwise
	// the merge patch will contain the immutable keys
	original.Annotations = make(map[string]string)
	expectedPatch, err := util.StrategicMergePatch(scheme.Codecs.LegacyCodec(v1beta1.SchemeGroupVersion), original, new)
	if err != nil {
		t.Fatalf("unexpected error merging patch: %v", err)
	}
	for _, action := range h.usrClientset.(*fake.Clientset).Actions() {
		switch tp := action.(type) {
		case core.PatchActionImpl:
			if string(tp.GetPatch()) != string(expectedPatch) {
				t.Errorf("GOT: %s, EXPECTED: %s", string(tp.GetPatch()), string(expectedPatch))
			}
		default:
			t.Errorf("unexpected type of action: %T, OBJ: %s", tp, action)
		}
	}
}

func TestDeploymentOnPatchImageError(t *testing.T) {
	var (
		badImage    = "bad-image"
		expectedMsg = fmt.Sprintf(`the image "%s" is not allowed to run in the cluster`, badImage)
		h           = Handler{}
		router      = mux.NewRouter()
	)
	router.HandleFunc("/{namespace}/deployments/{deploy}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.DeploymentsOnMod(w, r)
	})).Methods("PATCH")
	ts := httptest.NewServer(router)
	defer ts.Close()

	h.clientset = fake.NewSimpleClientset(newDeployment("myapp", "default", nil, nil, nil, v1.Container{}))
	h.usrClientset = fake.NewSimpleClientset()

	new := newDeployment("myapp", "default", nil, nil, nil, v1.Container{Image: badImage})
	respBody, _, err := doRequest("PATCH", ts.URL+"/default/deployments/myapp", new)
	if err != nil {
		t.Fatalf("unexpected error executing request: %v", err)
	}

	got := &metav1.Status{}
	if err := json.Unmarshal(respBody, got); err != nil {
		t.Fatalf("unexpected error unmarshaling response: %v. Obj: %v", err, string(respBody))
	}

	if got.Message != expectedMsg {
		t.Errorf("GOT: %s, EXPECTED: %s", got.Message, expectedMsg)
	}
}

func TestDeploymentOnPatchScaleError(t *testing.T) {
	var (
		image       = "stateful-image"
		expectedMsg = "found a persistent volume, unable to scale"
		h           = &Handler{allowedImages: []string{image}}
		router      = mux.NewRouter()
	)
	router.HandleFunc("/{namespace}/deployments/{deploy}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.DeploymentsOnMod(w, r)
	})).Methods("PATCH")
	ts := httptest.NewServer(router)
	defer ts.Close()

	original := newDeployment("myapp", "default", nil, nil, nil, v1.Container{Image: image})
	original.Spec.Replicas = func() *int32 { scaleUp := int32(3); return &scaleUp }()
	original.Spec.Template.Spec.Volumes = []v1.Volume{{Name: "a-volume"}}
	h.clientset = fake.NewSimpleClientset(original)
	h.usrClientset = fake.NewSimpleClientset()

	new := &v1beta1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "myapp"}}
	respBody, _, err := doRequest("PATCH", ts.URL+"/default/deployments/myapp", new)
	if err != nil {
		t.Fatalf("unexpected error executing request: %#v", err)
	}

	got := &metav1.Status{}
	if err := json.Unmarshal(respBody, got); err != nil {
		t.Fatalf("unexpected error unmarshaling response: %v. Obj: %v", err, string(respBody))
	}
	if got.Message != expectedMsg {
		t.Errorf("GOT: %s, EXPECTED: %s", got.Message, expectedMsg)
	}
}

func TestDeploymentOnPatchMutateContainers(t *testing.T) {
	var (
		image             = "valid-image"
		expectedContainer = v1.Container{Name: "myapp", Image: image}
		computeResources  = newComputeResources()
		mutateContainer   = v1.Container{
			Name:                     "myapp",
			Resources:                computeResources,
			LivenessProbe:            &v1.Probe{PeriodSeconds: 10},
			ReadinessProbe:           &v1.Probe{SuccessThreshold: 50},
			Lifecycle:                &v1.Lifecycle{PreStop: &v1.Handler{Exec: &v1.ExecAction{Command: []string{"a-command"}}}},
			TerminationMessagePath:   v1.TerminationMessagePathDefault,
			TerminationMessagePolicy: v1.TerminationMessageReadFile,
			ImagePullPolicy:          v1.PullNever,
			Image:                    image,
			Stdin:                    true,
			StdinOnce:                true,
			TTY:                      true,
		}
		h      = &Handler{allowedImages: []string{image}}
		router = mux.NewRouter()
	)
	router.HandleFunc("/{namespace}/deployments/{deploy}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.DeploymentsOnMod(w, r)
	})).Methods("PATCH")
	ts := httptest.NewServer(router)
	defer ts.Close()

	h.clientset = fake.NewSimpleClientset(newDeployment("myapp", "default", nil, nil, nil, expectedContainer))
	h.usrClientset = fake.NewSimpleClientset()

	new := newDeployment("myapp", "default", nil, nil, nil, mutateContainer)
	_, _, err := doRequest("PATCH", ts.URL+"/default/deployments/myapp", new)
	if err != nil {
		t.Fatalf("unexpected error executing request: %#v", err)
	}

	fakeclientset := h.usrClientset.(*fake.Clientset)
	if len(fakeclientset.Actions()) != 1 {
		t.Errorf("GOT: %d action(s), EXPECTED: 1 action", len(fakeclientset.Actions()))
	}
	for _, action := range fakeclientset.Actions() {
		switch tp := action.(type) {
		case core.PatchActionImpl:
			if string(tp.GetPatch()) != "{}" {
				t.Errorf("GOT: %s, EXPECTED: {}", string(tp.GetPatch()))
			}
		}
	}
}

func TestDeploymentOnPatchAPIErrors(t *testing.T) {
	appName, image := "stateful-image", "myapp"
	responseHeaders := http.Header{"Content-Type": []string{"application/json"}}
	h := &Handler{allowedImages: []string{image}}
	r := mux.NewRouter()
	r.HandleFunc("/{namespace}/deployments/{deploy}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.DeploymentsOnMod(w, r)
	})).Methods("PATCH")
	ts := httptest.NewServer(r)
	defer ts.Close()

	var err error
	h.clientset, err = kubernetes.NewForConfig(&rest.Config{})
	if err != nil {
		t.Fatalf("unexpected error getting rest client: %v", err)
	}

	testCases := []struct {
		extClient   *http.Client
		expectedMsg string
	}{
		{
			extClient: fakerest.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
				return nil, fmt.Errorf("failed creating request")
			}),
			expectedMsg: "failed creating request",
		},
		{
			extClient: fakerest.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
				obj := &metav1.Status{Status: "error", Message: "failed retrieving deployment"}
				return &http.Response{
					StatusCode: 404,
					Header:     responseHeaders,
					Body:       objBody(obj),
				}, nil
			}),
			expectedMsg: fmt.Sprintf(`deployment "%s" not found`, appName),
		},
		{
			extClient: fakerest.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
				response := &http.Response{Header: responseHeaders}
				switch req.Method {
				case "GET":
					obj := v1beta1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: appName}}
					response.StatusCode = 200
					response.Body = objBody(obj)
				case "PATCH":
					obj := util.StatusNotFound("deployment not found", &v1beta1.Deployment{})
					response.StatusCode = 404
					response.Body = objBody(obj)
				default:
					t.Fatalf("unexpected method: %v", req.Method)
				}
				return response, nil
			}),
			expectedMsg: "deployment not found",
		},
		{
			extClient: fakerest.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
				response := &http.Response{Header: responseHeaders}
				switch req.Method {
				case "GET":
					obj := v1beta1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: appName}}
					response.StatusCode = 200
					response.Body = objBody(obj)
				case "PATCH":
					obj := util.StatusBadRequest("a bad request happened", &v1beta1.Deployment{}, metav1.StatusReasonBadRequest)
					response.StatusCode = 400
					response.Body = objBody(obj)
				default:
					t.Fatalf("unexpected method: %v", req.Method)
				}
				return response, nil
			}),
			expectedMsg: "a bad request happened",
		},
	}

	for _, test := range testCases {
		h.clientset.Extensions().RESTClient().(*rest.RESTClient).Client = test.extClient
		h.usrClientset = h.clientset
		respBody, _, err := doRequest("PATCH", ts.URL+"/default/deployments/"+appName, &v1beta1.Deployment{})
		if err != nil {
			t.Fatalf("unexpected error executing request: %#v", err)
		}

		got := &metav1.Status{}
		if err := json.Unmarshal(respBody, got); err != nil {
			t.Fatalf("unexpected error unmarshaling response: %v. Obj: %v", err, string(respBody))
		}

		matched, err := regexp.MatchString(test.expectedMsg, got.Message)
		if err != nil {
			t.Fatalf("unexpected error matching result: %v", err)
		}
		if !matched {
			t.Errorf("GOT: %s, EXPECTED REGEXP: %s", got.Message, test.expectedMsg)
		}
	}
}
