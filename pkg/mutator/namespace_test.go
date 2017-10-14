package mutator

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gorilla/mux"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	fakerest "k8s.io/client-go/rest/fake"
	platform "kolihub.io/koli/pkg/apis/core/v1alpha1"
	"kolihub.io/koli/pkg/apis/core/v1alpha1/draft"
)

func newNamespace(name string) v1.Namespace {
	return v1.Namespace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1.SchemeGroupVersion.String(),
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
}

func TestOnNamespaceList(t *testing.T) {
	var (
		expCustomer, expOrg = "c1", "koli"
		expectedNs          = v1.NamespaceList{
			TypeMeta: metav1.TypeMeta{
				APIVersion: v1.SchemeGroupVersion.String(),
				Kind:       "NamespaceList",
			},
			Items: []v1.Namespace{
				newNamespace(fmt.Sprintf("prod-%s-%s", expCustomer, expOrg)),
				newNamespace(fmt.Sprintf("dev-%s-%s", expCustomer, expOrg)),
			},
		}
		expectedSelector = labels.Set{
			platform.LabelCustomer:     expCustomer,
			platform.LabelOrganization: expOrg,
		}
		h      = Handler{}
		router = mux.NewRouter()
	)
	router.HandleFunc("/api/v1/namespaces", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.NamespaceOnList(w, r)
	})).Methods("GET")
	ts := httptest.NewServer(router)
	defer ts.Close()

	h.clientset, _ = kubernetes.NewForConfig(&rest.Config{})
	nsFakeClient := fakerest.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
		if req.Method != "GET" || req.URL.Path != "/api/v1/namespaces" {
			t.Fatal("unexpected method or url path")
		}
		options := metav1.ListOptions{}
		if err := metav1.ParameterCodec.DecodeParameters(req.URL.Query(), metav1.SchemeGroupVersion, &options); err != nil {
			t.Fatalf("failed decoding parameters: %v", err)
		}

		if options.LabelSelector != expectedSelector.String() {
			t.Fatalf("GOT SELECTOR: %s, EXPECTED: %s", options.LabelSelector, expectedSelector.String())
		}

		return &http.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       objBody(expectedNs),
		}, nil
	})
	h.clientset.Core().RESTClient().(*rest.RESTClient).Client = nsFakeClient

	nsMeta := draft.NewNamespaceMetadata(expectedNs.Items[0].Name)
	h.user = &platform.User{Customer: nsMeta.Customer(), Organization: nsMeta.Organization()}

	dataBody, _, err := doGetRequest(ts.URL + "/api/v1/namespaces")
	if err != nil {
		t.Fatalf("unexpected error fetching namespaces: %v", err)
	}
	nsList := &v1.NamespaceList{}
	if err := runtime.DecodeInto(scheme.Codecs.UniversalDecoder(), dataBody, nsList); err != nil {
		t.Fatalf("unexpected error decoding to obj, %v", err)
	}
	if !reflect.DeepEqual(nsList.Items, expectedNs.Items) {
		t.Errorf("GOT: %#v, EXPECTED: %#v", nsList.Items, expectedNs.Items)
	}
}

func doGetRequest(url string) ([]byte, *http.Response, error) {
	resp, err := http.DefaultClient.Get(url)
	if err != nil {
		return nil, nil, fmt.Errorf("unexpected error executing request: %v", err)
	}
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("unexpected error reading response, %v", err)
	}
	return data, resp, nil
}
