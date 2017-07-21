package mutator

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"testing"

	"github.com/gorilla/mux"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/pkg/api"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
	fakerest "k8s.io/client-go/rest/fake"
	core "k8s.io/client-go/testing"
	platform "kolihub.io/koli/pkg/apis/v1alpha1"
	"kolihub.io/koli/pkg/util"
)

type clientFunc func(req *http.Request) (*http.Response, error)

func (f clientFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func makeTestServer(t *testing.T, statusCode int) clientFunc {
	return clientFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			StatusCode: statusCode,
			Body:       req.Body,
		}, nil
	})
}

func runIngressEndpointFakeServer(h *Handler, path string, method string) *httptest.Server {
	router := mux.NewRouter()
	switch method {
	case "POST":
		router.HandleFunc(path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h.IngressOnCreate(w, r)
		})).Methods("POST")
	case "PATCH":
		router.HandleFunc(path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h.IngressOnPatch(w, r)
		})).Methods("PATCH")
	case "DELETE":
		router.HandleFunc(path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h.IngressOnDelete(w, r)
		})).Methods("DELETE")
	default:
		panic(fmt.Sprintf("method %s not implemented", method))
	}

	return httptest.NewServer(router)
}

func newIngress(name, ns string, httpIngRuleValue *v1beta1.HTTPIngressRuleValue) *v1beta1.Ingress {
	return &v1beta1.Ingress{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Ingress",
			APIVersion: api.Registry.GroupOrDie(v1beta1.GroupName).GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: v1beta1.IngressSpec{
			Rules: []v1beta1.IngressRule{
				{Host: name, IngressRuleValue: v1beta1.IngressRuleValue{
					HTTP: httpIngRuleValue,
				}},
			},
		},
	}
}

func newIngressWithRules(name, ns string, rules []v1beta1.IngressRule) *v1beta1.Ingress {
	ing := newIngress(name, ns, nil)
	ing.Spec.Rules = rules
	return ing
}

func newIngressPath(httpIngressPath []v1beta1.HTTPIngressPath) *v1beta1.HTTPIngressRuleValue {
	// []v1beta1.httpIngressPath{}
	return &v1beta1.HTTPIngressRuleValue{
		Paths: httpIngressPath,
		// Paths: []v1beta1.HTTPIngressPath{
		// 	{Path: "/", Backend: v1beta1.IngressBackend{ServiceName: svcName, ServicePort: intstr.FromInt(svcPort)}},
		// },
	}
}

func newService(name, ns string, svcPort int32) *v1.Service {
	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{{Port: svcPort}},
		},
	}
}

// /apis/extensions/v1beta1/namespaces/{namespace}/ingresses
func TestIngressOnCreate(t *testing.T) {
	var (
		svcName, svcPort = "foo-svc", int32(8000)
		ingName, ns      = "myroute", "foo-ns"
		h                = &Handler{
			usrClientset: fake.NewSimpleClientset(),
			clientset:    fake.NewSimpleClientset([]runtime.Object{newService(svcName, ns, svcPort)}...),
		}
		ing = newIngress(ingName, ns, newIngressPath([]v1beta1.HTTPIngressPath{
			{Path: "/", Backend: v1beta1.IngressBackend{ServiceName: svcName, ServicePort: intstr.FromInt(int(svcPort))}},
		}))
	)
	ts := runIngressEndpointFakeServer(h, "/apis/extensions/v1beta1/namespaces/{namespace}/ingresses", "POST")
	defer ts.Close()

	respBody, _, err := doRequest("POST", fmt.Sprintf("%s/apis/extensions/v1beta1/namespaces/%s/ingresses", ts.URL, ns), ing)
	if err != nil {
		t.Fatalf("unexpected error executing request: %#v", err)
	}

	got := &v1beta1.Ingress{}
	runtime.DecodeInto(api.Codecs.UniversalDecoder(v1beta1.SchemeGroupVersion), respBody, got)
	if !reflect.DeepEqual(got.Spec.Rules, ing.Spec.Rules) {
		t.Errorf("GOT: %#v, EXPECTED: %#v", got.Spec, ing.Spec)
	}
}

func TestIngressOnCreateMutatingValues(t *testing.T) {
	var (
		ingName, ns      = "myroute", "foo-ns"
		svcName, svcPort = "foo-svc", int32(8000)
		h                = &Handler{
			usrClientset: fake.NewSimpleClientset(),
			clientset:    fake.NewSimpleClientset([]runtime.Object{newService(svcName, ns, svcPort)}...),
		}

		expIng = newIngress(ingName, ns, newIngressPath([]v1beta1.HTTPIngressPath{
			{Path: "/", Backend: v1beta1.IngressBackend{ServiceName: svcName, ServicePort: intstr.FromInt(int(svcPort))}},
		}))
	)
	requestIng := func() *v1beta1.Ingress { o, _ := api.Scheme.DeepCopy(expIng); return o.(*v1beta1.Ingress) }()
	requestIng.Spec.Backend = &v1beta1.IngressBackend{ServiceName: "foo"}
	requestIng.Spec.TLS = []v1beta1.IngressTLS{{SecretName: "foo-secret"}}
	ts := runIngressEndpointFakeServer(h, "/apis/extensions/v1beta1/namespaces/{namespace}/ingresses", "POST")
	defer ts.Close()

	respBody, _, err := doRequest("POST", fmt.Sprintf("%s/apis/extensions/v1beta1/namespaces/%s/ingresses", ts.URL, ns), requestIng)
	if err != nil {
		t.Fatalf("unexpected error executing request: %#v", err)
	}

	got := &v1beta1.Ingress{}
	runtime.DecodeInto(api.Codecs.UniversalDecoder(v1beta1.SchemeGroupVersion), respBody, got)
	if !reflect.DeepEqual(got, expIng) {
		t.Errorf("GOT: %#v, EXPECTED: %#v", got, expIng)
	}
}

func TestIngressOnCreateWithMoreThanOneDomain(t *testing.T) {
	var (
		h                       = &Handler{usrClientset: fake.NewSimpleClientset()}
		ingName, domainName, ns = "myroute", "acme.org", "foo-ns"
		ing                     = newIngressWithRules(ingName, ns, []v1beta1.IngressRule{
			{Host: domainName, IngressRuleValue: v1beta1.IngressRuleValue{}},
			{Host: "domain2.tld", IngressRuleValue: v1beta1.IngressRuleValue{}},
		})
		expMessage = fmt.Sprintf(`spec.rules cannot have more than one host, found %d rules`, len(ing.Spec.Rules))
	)
	ts := runIngressEndpointFakeServer(h, "/apis/extensions/v1beta1/namespaces/{namespace}/ingresses", "POST")
	defer ts.Close()

	respBody, _, err := doRequest("POST", fmt.Sprintf("%s/apis/extensions/v1beta1/namespaces/%s/ingresses", ts.URL, ns), ing)
	if err != nil {
		t.Fatalf("unexpected error executing request: %#v", err)
	}

	got := &metav1.Status{}
	runtime.DecodeInto(api.Codecs.UniversalDecoder(metav1.SchemeGroupVersion), respBody, got)
	if got.Message != expMessage {
		t.Errorf("GOT: %#v, EXPECTED: %#v", got, expMessage)
	}
}

func TestIngressOnCreateWithMissingService(t *testing.T) {
	var (
		unknownSvc       = "unknown-svc"
		ingName, ns      = "myroute", "foo-ns"
		svcName, svcPort = "foo-svc", int32(8000)
		h                = &Handler{
			usrClientset: fake.NewSimpleClientset(),
			clientset:    fake.NewSimpleClientset(newService(svcName, ns, svcPort)),
		}
		ing = newIngress(ingName, ns, newIngressPath([]v1beta1.HTTPIngressPath{
			{Path: "/", Backend: v1beta1.IngressBackend{ServiceName: unknownSvc, ServicePort: intstr.FromInt(int(svcPort))}},
		}))
		expectedStatus = &metav1.Status{
			TypeMeta: metav1.TypeMeta{
				APIVersion: metav1.SchemeGroupVersion.Version,
				Kind:       "Status",
			},
			Status:  metav1.StatusFailure,
			Message: fmt.Sprintf(`failed retrieving service [Service "%s" not found]`, unknownSvc),
			Reason:  metav1.StatusReasonBadRequest,
			Code:    int32(http.StatusBadRequest),
			Details: &metav1.StatusDetails{},
		}
	)
	ts := runIngressEndpointFakeServer(h, "/apis/extensions/v1beta1/namespaces/{namespace}/ingresses", "POST")
	defer ts.Close()

	respBody, _, err := doRequest("POST", fmt.Sprintf("%s/apis/extensions/v1beta1/namespaces/%s/ingresses", ts.URL, ns), ing)
	if err != nil {
		t.Fatalf("unexpected error executing request: %#v", err)
	}

	got := &metav1.Status{}
	runtime.DecodeInto(api.Codecs.UniversalDecoder(metav1.SchemeGroupVersion), respBody, got)
	if !reflect.DeepEqual(got, expectedStatus) {
		t.Errorf("GOT: %#v, EXPECTED: %#v", got, expectedStatus)
	}
}

func TestIngressOnPatch(t *testing.T) {
	var (
		ingName, ns, svcName, svcPort = "foo.tld", "foo-ns", "foo-svc", intstr.FromInt(8000)
		expObj                        = newIngressWithRules(ingName, ns, []v1beta1.IngressRule{
			{Host: ingName, IngressRuleValue: v1beta1.IngressRuleValue{}},
		})
		existentIng = newIngress(ingName, ns, newIngressPath([]v1beta1.HTTPIngressPath{
			{Path: "/", Backend: v1beta1.IngressBackend{ServiceName: svcName, ServicePort: svcPort}},
		}))
		svc           = newService(svcName, ns, svcPort.IntVal)
		kongClient, _ = NewKongClient(makeTestServer(t, http.StatusNoContent), "")
		h             = &Handler{clientset: fake.NewSimpleClientset(existentIng, svc), usrClientset: fake.NewSimpleClientset(), kongClient: kongClient}
	)
	ts := runIngressEndpointFakeServer(h, "/apis/extensions/v1beta1/namespaces/{namespace}/ingresses/{name}", "PATCH")
	defer ts.Close()

	expObj.Annotations = map[string]string{"kolihub.io/foo.tld": "primary"}
	expObj.Annotations["kolihub.io/parent"] = "fake-ns"
	expObj.Spec.Rules[0].HTTP = newIngressPath([]v1beta1.HTTPIngressPath{
		{Path: "/my-custom-path", Backend: v1beta1.IngressBackend{ServiceName: svcName, ServicePort: svcPort}},
		{Path: "/my-custom-path-02", Backend: v1beta1.IngressBackend{ServiceName: svcName, ServicePort: svcPort}},
	})

	respBody, _, err := doRequest("PATCH", fmt.Sprintf("%s/apis/extensions/v1beta1/namespaces/%s/ingresses/%s", ts.URL, ns, ingName), expObj)
	if err != nil {
		t.Fatalf("unexpected error executing request: %#v", err)
	}

	t.Logf("RESPBODY: %s", string(respBody))

	fakeUsrClientset := h.usrClientset.(*fake.Clientset)
	if len(fakeUsrClientset.Actions()) != 1 {
		t.Errorf("GOT %d action(s), EXPECTED 1 action", len(fakeUsrClientset.Actions()))
	}
	for _, action := range fakeUsrClientset.Actions() {
		switch tp := action.(type) {
		case core.PatchActionImpl:
			expPatch, _ := util.StrategicMergePatch(extensionsCodec, existentIng, expObj)
			if !reflect.DeepEqual(tp.Patch, expPatch) {
				t.Errorf("GOT: %s, EXPECTED: %s", string(tp.Patch), string(expPatch))
			}

		default:
			t.Fatalf("unexpected action %#v", tp)

		}
	}
}

func TestIngressOnPatchMutateAnnotations(t *testing.T) {
	var (
		ingName, ns, svcName, svcPort = "foo.tld", "foo-ns", "foo-svc", intstr.FromInt(8000)
		expObj                        = newIngressWithRules(ingName, ns, nil)
		existentIng                   = newIngress(ingName, ns, newIngressPath([]v1beta1.HTTPIngressPath{
			{Path: "/", Backend: v1beta1.IngressBackend{ServiceName: svcName, ServicePort: svcPort}},
		}))
		svc = newService(svcName, ns, svcPort.IntVal)
		h   = &Handler{usrClientset: fake.NewSimpleClientset()}
	)
	ts := runIngressEndpointFakeServer(h, "/apis/extensions/v1beta1/namespaces/{namespace}/ingresses/{name}", "PATCH")
	defer ts.Close()

	existentIng.Annotations = map[string]string{"kolihub.io/bar.tld": "primary", "kolihub.io/parent": "bar-ns"}
	h.clientset = fake.NewSimpleClientset(existentIng, svc)

	// try to change the immutable annotations
	expObj.Annotations = map[string]string{"kolihub.io/bar.tld": "mutate", "kolihub.io/parent": "mutate-ns"}

	respBody, _, err := doRequest("PATCH", fmt.Sprintf("%s/apis/extensions/v1beta1/namespaces/%s/ingresses/%s", ts.URL, ns, ingName), expObj)
	if err != nil {
		t.Fatalf("unexpected error executing request: %#v", err)
	}

	t.Logf("RESPBODY: %s", string(respBody))

	fakeUsrClientset := h.usrClientset.(*fake.Clientset)
	if len(fakeUsrClientset.Actions()) != 1 {
		t.Errorf("GOT %d action(s), EXPECTED 1 action", len(fakeUsrClientset.Actions()))
	}
	for _, action := range fakeUsrClientset.Actions() {
		switch tp := action.(type) {
		case core.PatchActionImpl:
			if string(tp.Patch) != "{}" {
				t.Errorf("GOT: %s, EXPECTED: {}", string(tp.Patch))
			}

		default:
			t.Fatalf("unexpected action %#v", tp)

		}
	}
}

func TestIngressOnPatchMultipleRulesError(t *testing.T) {
	var (
		ingName, ns, svcName, svcPort = "foo.tld", "foo-ns", "foo-svc", intstr.FromInt(8000)
		expObj                        = newIngressWithRules(ingName, ns, nil)
		existentIng                   = newIngress(ingName, ns, newIngressPath([]v1beta1.HTTPIngressPath{
			{Path: "/", Backend: v1beta1.IngressBackend{ServiceName: svcName, ServicePort: svcPort}},
		}))
		expMsg = "spec.rules cannot have more than one host, found 2 rules"
		svc    = newService(svcName, ns, svcPort.IntVal)
		h      = &Handler{clientset: fake.NewSimpleClientset(existentIng, svc), usrClientset: fake.NewSimpleClientset()}
	)
	ts := runIngressEndpointFakeServer(h, "/apis/extensions/v1beta1/namespaces/{namespace}/ingresses/{name}", "PATCH")
	defer ts.Close()

	expObj.Spec.Rules = []v1beta1.IngressRule{{Host: "host01"}, {Host: "host02"}}

	respBody, _, err := doRequest("PATCH", fmt.Sprintf("%s/apis/extensions/v1beta1/namespaces/%s/ingresses/%s", ts.URL, ns, ingName), expObj)
	if err != nil {
		t.Fatalf("unexpected error executing request: %#v", err)
	}

	got := &metav1.Status{}
	runtime.DecodeInto(api.Codecs.UniversalDecoder(), respBody, got)
	if got.Message != expMsg {
		t.Errorf("GOT: %#v, EXPECTED: %#v", got.Message, expMsg)
	}
}

func TestIngressOnPatchRemoveRulesError(t *testing.T) {
	var (
		ingName, ns, svcName, svcPort = "foo.tld", "foo-ns", "foo-svc", intstr.FromInt(8000)
		existentIng                   = newIngress(ingName, ns, newIngressPath([]v1beta1.HTTPIngressPath{
			{Path: "/", Backend: v1beta1.IngressBackend{ServiceName: svcName, ServicePort: svcPort}},
		}))
		expMsg = "spec.rules cannot be removed"
		svc    = newService(svcName, ns, svcPort.IntVal)
		h      = &Handler{clientset: fake.NewSimpleClientset(existentIng, svc), usrClientset: fake.NewSimpleClientset()}
	)
	ts := runIngressEndpointFakeServer(h, "/apis/extensions/v1beta1/namespaces/{namespace}/ingresses/{name}", "PATCH")
	defer ts.Close()

	respBody, _, err := doRequest("PATCH", fmt.Sprintf("%s/apis/extensions/v1beta1/namespaces/%s/ingresses/%s", ts.URL, ns, ingName), []byte(`{"spec": {"rules": null}}`))
	if err != nil {
		t.Fatalf("unexpected error executing request: %#v", err)
	}
	t.Logf(string(respBody))

	got := &metav1.Status{}
	runtime.DecodeInto(api.Codecs.UniversalDecoder(), respBody, got)
	if got.Message != expMsg {
		t.Errorf("GOT: %#v, EXPECTED: %#v", got.Message, expMsg)
	}
}

func TestIngressOnPatchChangeHostError(t *testing.T) {
	var (
		ingName, ns, svcName, svcPort = "foo.tld", "foo-ns", "foo-svc", intstr.FromInt(8000)
		existentIng                   = newIngress(ingName, ns, newIngressPath([]v1beta1.HTTPIngressPath{
			{Path: "/", Backend: v1beta1.IngressBackend{ServiceName: svcName, ServicePort: svcPort}},
		}))
		expMsg = `cannot change host, from "foo.tld" to "mutate-host"`
		svc    = newService(svcName, ns, svcPort.IntVal)
		h      = &Handler{clientset: fake.NewSimpleClientset(existentIng, svc), usrClientset: fake.NewSimpleClientset()}
	)
	ts := runIngressEndpointFakeServer(h, "/apis/extensions/v1beta1/namespaces/{namespace}/ingresses/{name}", "PATCH")
	defer ts.Close()

	reqBody := []byte(`{"spec": {"rules": [{"host": "mutate-host"}]}}`)
	respBody, _, err := doRequest("PATCH", fmt.Sprintf("%s/apis/extensions/v1beta1/namespaces/%s/ingresses/%s", ts.URL, ns, ingName), reqBody)
	if err != nil {
		t.Fatalf("unexpected error executing request: %#v", err)
	}
	t.Logf(string(respBody))

	got := &metav1.Status{}
	runtime.DecodeInto(api.Codecs.UniversalDecoder(), respBody, got)
	if got.Message != expMsg {
		t.Errorf("GOT: %#v, EXPECTED: %#v", got.Message, expMsg)
	}
}
func TestIngressOnPatchNewPathWithWrongService(t *testing.T) {
	var (
		ingName, ns, svcName, svcPort = "foo.tld", "foo-ns", "foo-svc", intstr.FromInt(8000)
		existentIng                   = newIngress(ingName, ns, newIngressPath([]v1beta1.HTTPIngressPath{
			{Path: "/", Backend: v1beta1.IngressBackend{ServiceName: svcName, ServicePort: svcPort}},
		}))
		expMsgRgxp = `the service port "80" doesn't exists in service "foo-svc".+`
		svc        = newService(svcName, ns, svcPort.IntVal)
		h          = &Handler{clientset: fake.NewSimpleClientset(existentIng, svc), usrClientset: fake.NewSimpleClientset()}
	)
	ts := runIngressEndpointFakeServer(h, "/apis/extensions/v1beta1/namespaces/{namespace}/ingresses/{name}", "PATCH")
	defer ts.Close()

	reqBody := []byte(`{"spec": {"rules": [{"http": {"paths": [{"path": "new", "backend": {"serviceName": "foo-svc", "servicePort": 80}}]}}]}}`)
	respBody, _, err := doRequest("PATCH", fmt.Sprintf("%s/apis/extensions/v1beta1/namespaces/%s/ingresses/%s", ts.URL, ns, ingName), reqBody)
	if err != nil {
		t.Fatalf("unexpected error executing request: %#v", err)
	}
	t.Logf(string(respBody))

	got := &metav1.Status{}
	runtime.DecodeInto(api.Codecs.UniversalDecoder(), respBody, got)
	if match, _ := regexp.MatchString(expMsgRgxp, got.Message); !match {
		t.Errorf("GOT: %#v, EXPECTED REGEXP: %#v", got.Message, expMsgRgxp)
	}
}

func TestIngressOnDelete(t *testing.T) {
	var (
		ingName, ns = "foo.tld", "foo-ns"
		existentIng = newIngressWithRules(ingName, ns, []v1beta1.IngressRule{
			{Host: ingName, IngressRuleValue: v1beta1.IngressRuleValue{}},
		})
		existentDom = newDomain("foo-tld", ns, ingName)
		h           = &Handler{clientset: fake.NewSimpleClientset(existentIng), usrClientset: fake.NewSimpleClientset()}
	)
	ts := runIngressEndpointFakeServer(h, "/apis/extensions/v1beta1/namespaces/{namespace}/ingresses/{name}", "DELETE")
	defer ts.Close()
	h.usrTprClient = &fakerest.RESTClient{
		APIRegistry:          api.Registry,
		NegotiatedSerializer: serializer.DirectCodecFactory{CodecFactory: api.Codecs},
		Client: fakerest.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			var obj runtime.Object
			switch req.Method {
			case "GET":
				obj = &platform.DomainList{
					Items: []platform.Domain{*existentDom},
				}
			case "DELETE": // noop
			default:
				t.Fatalf("unexpected method: %v", req.Method)
			}

			return &http.Response{
				StatusCode: 200,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       objBody(obj),
			}, nil
		}),
	}
	req, err := http.NewRequest("DELETE", fmt.Sprintf("%s/apis/extensions/v1beta1/namespaces/%s/ingresses/%s", ts.URL, ns, ingName), nil)
	if err != nil {
		t.Fatalf("unexpected error creating request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Logf("STATUS CODE RESPONSE: %d", resp.StatusCode)

	fakeUsrClientset := h.usrClientset.(*fake.Clientset)
	if len(fakeUsrClientset.Actions()) != 1 {
		t.Errorf("GOT %d action(s), EXPECTED 1 action", len(fakeUsrClientset.Actions()))
	}
	for _, action := range fakeUsrClientset.Actions() {
		switch tp := action.(type) {
		case core.DeleteActionImpl:
			if tp.Namespace != ns || tp.Name != ingName {
				t.Errorf("GOT RESOURCE: %s/%s, EXPECTED: %s/%s", tp.Namespace, tp.Name, ns, ingName)
			}
		default:
			t.Fatalf("unexpected action %#v", tp)

		}
	}
}
