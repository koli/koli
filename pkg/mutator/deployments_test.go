package mutator

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"io/ioutil"

	"bytes"

	"regexp"

	"github.com/gorilla/mux"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/rest/fake"
	platform "kolihub.io/koli/pkg/apis/v1alpha1"
)

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
	reqBody, err := json.Marshal(obj)
	if err != nil {
		return nil, nil, fmt.Errorf("failed encoding object: %v", err)
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
		planName         = "foo-plan"
		storagePlan      = "foo-plan-5g"
		requestStorage   = resource.MustParse("5Gi")
		computeResources = newComputeResources()
	)
	h := Handler{}
	// Fake Clients
	responseHeader := http.Header{"Content-Type": []string{"application/json"}}
	h.tprClient = &fake.RESTClient{
		APIRegistry:          api.Registry,
		NegotiatedSerializer: serializer.DirectCodecFactory{CodecFactory: api.Codecs},
		Client: fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
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

	extensionsClient := fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
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
		Name:   "test",
		Labels: map[string]string{platform.LabelStoragePlan: storagePlan},
	}}
	podSpec := &new.Spec.Template.Spec
	podSpec.Volumes = []v1.Volume{{Name: "avolume"}}
	podSpec.Containers = []v1.Container{{Name: "test", Image: "busybox"}}

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

	podSpec = &got.Spec.Template.Spec
	if len(podSpec.Containers) != 1 {
		t.Errorf("GOT: %d, EXPECTED: 1", len(podSpec.Containers))
	}

	if podSpec.Volumes != nil {
		t.Errorf("GOT: %#v, EXPECTED: nil", podSpec.Volumes)
	}

	if got.Annotations[platform.AnnotationSetupStorage] != "true" {
		t.Errorf("GOT: %#v, EXPECTED: \"true\"", got.Annotations[platform.AnnotationSetupStorage])
	}

	if !reflect.DeepEqual(podSpec.Containers[0].Resources, computeResources) {
		t.Errorf("GOT: %#v, EXPECTED: %#v", podSpec.Containers[0].Resources, computeResources)
	}
}

func TestDeploymentOnPatch(t *testing.T) {
	var (
		namespace, appName      = "default", "foo-app"
		expectedAnnotationValue = "customvalue"
		expectedPlan            = "foo-plan"
		expectedStoragePlan     = "foo-plan-5g"
		expectedMinReadySeconds = 250
		expectedPaused          = true
		expectedImage           = "quay.io/koli/postgres"
		expectedEnvVars         = []v1.EnvVar{{Name: "MYENV", Value: "env-value"}}
		expectedArgs            = []string{"--args", "myargument"}
		expectedCommand         = []string{"/bin/sh", "-c", "mycommand"}
		expectedVolumeMount     = []v1.VolumeMount{{Name: "an-existing-volume", MountPath: "/tmp"}}
		computeResources        = newComputeResources()
	)
	// Mux Fake Server
	h := Handler{allowedImages: []string{"quay.io/koli/postgres"}}
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

	responseHeader := http.Header{"Content-Type": []string{"application/json"}}

	// Fake TPR Client
	h.tprClient = &fake.RESTClient{
		APIRegistry:          api.Registry,
		NegotiatedSerializer: serializer.DirectCodecFactory{CodecFactory: api.Codecs},
		Client: fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
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
			return &http.Response{StatusCode: 200, Header: responseHeader, Body: objBody(plan)}, nil
		}),
	}

	// Fake Extension Client
	extensionsClient := fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
		obj := &v1beta1.Deployment{}
		switch req.Method {
		case "GET":
			obj.ObjectMeta = metav1.ObjectMeta{Name: appName, Namespace: namespace}
			obj.Annotations = map[string]string{}
			for _, immutableKey := range immutableAnnotations {
				obj.Annotations[immutableKey] = expectedAnnotationValue
			}
		case "PATCH":
			// simulating a PATCH responses
			if err := json.NewDecoder(req.Body).Decode(obj); err != nil {
				t.Fatalf("failed encoding deployment request: %v", err)
			}
		default:
			return &http.Response{StatusCode: 501, Header: responseHeader}, nil
		}
		return &http.Response{StatusCode: 200, Header: responseHeader, Body: objBody(obj)}, nil
	})
	h.clientset.Extensions().RESTClient().(*rest.RESTClient).Client = extensionsClient
	h.usrClientset = h.clientset

	new := &v1beta1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: appName, Annotations: map[string]string{}}}
	// Try to mutate all immutable annotations
	for _, a := range immutableAnnotations {
		new.Annotations[a] = "mutate-me"
	}

	// Add storage and a default plan
	new.Labels = map[string]string{
		platform.LabelStoragePlan: expectedStoragePlan,
		platform.LabelClusterPlan: expectedPlan,
	}

	// Mutate DeploymentSpec
	new.Spec.Paused = expectedPaused
	new.Spec.MinReadySeconds = int32(expectedMinReadySeconds)

	podSpec := &new.Spec.Template.Spec

	// Mutate podSpec
	podSpec.Containers = []v1.Container{{
		Args:         expectedArgs,
		Command:      expectedCommand,
		Env:          expectedEnvVars,
		Image:        expectedImage,
		VolumeMounts: expectedVolumeMount,
	}}

	respBody, _, err := doRequest("PATCH", ts.URL+"/default/deployments/"+appName, new)
	if err != nil {
		t.Fatalf("unexpected error executing request: %v", err)
	}

	got := &v1beta1.Deployment{}
	if err := json.Unmarshal(respBody, got); err != nil {
		t.Fatalf("unexpected error unmarshaling response: %v. Obj: %v", err, string(respBody))
	}

	// Annotations mutate
	for _, immutableKey := range immutableAnnotations {
		if got.Annotations[immutableKey] != expectedAnnotationValue {
			// Skip because we're testing this annotation key above
			if immutableKey == platform.AnnotationSetupStorage {
				continue
			}
			t.Errorf("GOT: %#v, EXPECTED VALUE: %#v FOR KEY: %s", got.Annotations[immutableKey], expectedAnnotationValue, immutableKey)
		}
	}

	// Test storage plan
	if got.Annotations[platform.AnnotationSetupStorage] != "true" {
		t.Errorf("GOT: %#v, EXPECTED: %v FOR KEY: %v", got.Annotations[platform.AnnotationSetupStorage], "true", platform.AnnotationSetupStorage)
	}

	// DeploymentSpec mutate
	if got.Spec.MinReadySeconds != int32(expectedMinReadySeconds) {
		t.Errorf("GOT: %#v, EXPECTED: %#v", got.Spec.MinReadySeconds, expectedMinReadySeconds)
	}

	if got.Spec.Paused != expectedPaused {
		t.Errorf("GOT: %#v, EXPECTED: %#v", got.Spec.Paused, expectedPaused)
	}

	// Containers
	gotC := got.Spec.Template.Spec.Containers[0]
	if !reflect.DeepEqual(gotC.Args, expectedArgs) {
		t.Errorf("GOT: %#v, EXPECTED: %#v", gotC.Args, expectedArgs)
	}
	if !reflect.DeepEqual(gotC.Command, expectedCommand) {
		t.Errorf("GOT: %#v, EXPECTED: %#v", gotC.Command, expectedCommand)
	}
	if !reflect.DeepEqual(gotC.Env, expectedEnvVars) {
		t.Errorf("GOT: %#v, EXPECTED: %#v", gotC.Env, expectedEnvVars)
	}
	if gotC.Image != expectedImage {
		t.Errorf("GOT: %#v, EXPECTED: %#v", gotC.Image, expectedImage)
	}
	if !reflect.DeepEqual(gotC.VolumeMounts, expectedVolumeMount) {
		t.Errorf("GOT: %#v, EXPECTED: %#v", gotC.VolumeMounts, expectedVolumeMount)
	}

	// Test If container is in the right compute plan
	if !reflect.DeepEqual(gotC.Resources, computeResources) {
		t.Errorf("GOT: %#v, EXPECTED: %#v", gotC.Resources, computeResources)
	}
}
func TestDeploymentOnPatchImageError(t *testing.T) {
	// Mux Fake Server
	badImage := "bad-image"
	expectedMsg := fmt.Sprintf(`the image "%s" is not allowed to run in the cluster`, badImage)
	h := Handler{}
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

	extensionsClient := fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
		obj := &v1beta1.Deployment{}
		switch req.Method {
		case "GET":
			obj.ObjectMeta = metav1.ObjectMeta{Name: "myapp"}
		case "PATCH":
			if err := json.NewDecoder(req.Body).Decode(obj); err != nil {
				t.Fatalf("failed encoding deployment request: %v", err)
			}
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: objBody(obj)}, nil
	})
	h.clientset.Extensions().RESTClient().(*rest.RESTClient).Client = extensionsClient
	h.usrClientset = h.clientset

	new := &v1beta1.Deployment{}
	new.Spec.Template.Spec.Containers = []v1.Container{{Image: badImage}}
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
	image := "stateful-image"
	expectedMsg := "found a persistent volume, unable to scale"
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

	extensionsClient := fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
		obj := &v1beta1.Deployment{}
		switch req.Method {
		case "GET":
			obj.ObjectMeta = metav1.ObjectMeta{Name: "myapp"}
			obj.Spec.Template.Spec.Containers = []v1.Container{{Image: image}}
			obj.Spec.Template.Spec.Volumes = []v1.Volume{{Name: "a-volume"}}
			scaleUp := int32(3)
			obj.Spec.Replicas = &scaleUp
		case "PATCH":
			if err := json.NewDecoder(req.Body).Decode(obj); err != nil {
				t.Fatalf("failed encoding deployment request: %v", err)
			}
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: objBody(obj)}, nil
	})
	h.clientset.Extensions().RESTClient().(*rest.RESTClient).Client = extensionsClient
	h.usrClientset = h.clientset

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
	image := "valid-image"
	h := &Handler{allowedImages: []string{image}}
	computeResources := newComputeResources()
	mutateContainer := v1.Container{
		Name:                     "mutate-name",
		Resources:                computeResources,
		LivenessProbe:            &v1.Probe{PeriodSeconds: 10},
		ReadinessProbe:           &v1.Probe{SuccessThreshold: 50},
		Lifecycle:                &v1.Lifecycle{PreStop: &v1.Handler{Exec: &v1.ExecAction{Command: []string{"a-command"}}}},
		TerminationMessagePath:   v1.TerminationMessagePathDefault,
		TerminationMessagePolicy: v1.TerminationMessageReadFile,
		ImagePullPolicy:          v1.PullNever,
		Image:                    image,
		// Args:                     []string{"mycommand"},
		Stdin:     true,
		StdinOnce: true,
		TTY:       true,
	}
	expectedContainer := v1.Container{Name: "myapp", Image: image}
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

	extensionsClient := fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
		obj := &v1beta1.Deployment{}
		switch req.Method {
		case "GET":
			obj.ObjectMeta = metav1.ObjectMeta{Name: "myapp"}
			obj.Spec.Template.Spec.Containers = []v1.Container{expectedContainer}
		case "PATCH":
			if err := json.NewDecoder(req.Body).Decode(obj); err != nil {
				t.Fatalf("failed encoding deployment request: %v", err)
			}
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: objBody(obj)}, nil
	})
	h.clientset.Extensions().RESTClient().(*rest.RESTClient).Client = extensionsClient
	h.usrClientset = h.clientset

	new := &v1beta1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "myapp"}}
	new.Spec.Template.Spec.Containers = []v1.Container{mutateContainer}
	respBody, _, err := doRequest("PATCH", ts.URL+"/default/deployments/myapp", new)
	if err != nil {
		t.Fatalf("unexpected error executing request: %#v", err)
	}

	got := &v1beta1.Deployment{}
	if err := json.Unmarshal(respBody, got); err != nil {
		t.Fatalf("unexpected error unmarshaling response: %v. Obj: %v", err, string(respBody))
	}

	gotC := got.Spec.Template.Spec.Containers[0]
	if !reflect.DeepEqual(gotC, expectedContainer) {
		t.Errorf("GOT: %#v, EXPECTED: %#v", gotC, expectedContainer)
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
			extClient: fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
				return nil, fmt.Errorf("failed creating request")
			}),
			expectedMsg: "failed creating request",
		},
		{
			extClient: fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
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
			extClient: fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
				response := &http.Response{Header: responseHeaders}
				switch req.Method {
				case "GET":
					obj := v1beta1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: appName}}
					response.StatusCode = 200
					response.Body = objBody(obj)
				case "PATCH":
					obj := StatusNotFound("deployment not found", &v1beta1.Deployment{})
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
			extClient: fake.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
				response := &http.Response{Header: responseHeaders}
				switch req.Method {
				case "GET":
					obj := v1beta1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: appName}}
					response.StatusCode = 200
					response.Body = objBody(obj)
				case "PATCH":
					obj := StatusBadRequest("a bad request happened", &v1beta1.Deployment{}, metav1.StatusReasonBadRequest)
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
