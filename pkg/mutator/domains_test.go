package mutator

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/gorilla/mux"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	fakerest "k8s.io/client-go/rest/fake"
	platform "kolihub.io/koli/pkg/apis/core/v1alpha1"
)

var (
	groupVersion = platform.SchemeGroupVersion.String()
)

func runDomainsFakeServer(h *Handler, path, method string) *httptest.Server {
	router := mux.NewRouter()
	switch method {
	case "HEAD":
		router.HandleFunc(path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h.DomainsOnHead(w, r)
		})).Methods("HEAD")
	default:
		panic(fmt.Sprintf("method %s not implemented", method))
	}
	return httptest.NewServer(router)
}

func newDomain(name, ns, prim string) *platform.Domain {
	return &platform.Domain{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: platform.DomainSpec{
			PrimaryDomain: prim,
		},
	}
}

func TestDomainOnHead(t *testing.T) {
	var (
		name, ns, fqdn = "foo", "foo-ns", "foo.tld"
		expTime        = &metav1.Time{Time: time.Now()}
		h              = &Handler{}
		expObj         = newDomain(name, ns, fqdn)
	)
	expObj.Status.Phase = platform.DomainStatusOK
	expObj.Status.LastUpdateTime = expTime
	expHeader := http.Header{
		"X-Domain-Name":               []string{expObj.Name},
		"X-Domain-Namespace":          []string{expObj.Namespace},
		"X-Domain-Status-Lastupdated": []string{expObj.Status.LastUpdateTime.UTC().Format(time.RFC3339)},
		"X-Domain-Status-Phase":       []string{string(expObj.Status.Phase)},
	}

	h.tprClient = &fakerest.RESTClient{
		APIRegistry:          registry,
		NegotiatedSerializer: scheme.Codecs,
		Client: fakerest.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			pList := &platform.DomainList{
				Items: []platform.Domain{*expObj},
			}
			return &http.Response{
				StatusCode: 200,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       objBody(pList),
			}, nil
		}),
	}
	ts := runDomainsFakeServer(h, fmt.Sprintf("/apis/%s/domains/{fqdn}", groupVersion), "HEAD")
	resp, err := http.DefaultClient.Head(ts.URL + fmt.Sprintf("/apis/%s/domains/%s", groupVersion, fqdn))
	if err != nil {
		t.Fatalf("unexpected error executing request: %#v", err)
	}

	resp.Header.Del("Date")
	if !reflect.DeepEqual(resp.Header, expHeader) {
		t.Errorf("GOT: %#v, EXPECTED: %#v, STATUSCODE: %d", resp.Header, expHeader, resp.StatusCode)
	}
}

func TestDomainOnHeadNotFound(t *testing.T) {
	var (
		name, ns, fqdn = "foo", "foo-ns", "foo.tld"
		expTime        = &metav1.Time{Time: time.Now()}
		h              = &Handler{}
		expObj         = newDomain(name, ns, fqdn)
	)
	expObj.Status.Phase = platform.DomainStatusOK
	expObj.Status.LastUpdateTime = expTime
	expHeader := http.Header{
		"X-Domain-Name":               []string{expObj.Name},
		"X-Domain-Namespace":          []string{expObj.Namespace},
		"X-Domain-Status-Lastupdated": []string{expObj.Status.LastUpdateTime.UTC().Format(time.RFC3339)},
		"X-Domain-Status-Phase":       []string{string(expObj.Status.Phase)},
	}

	h.tprClient = &fakerest.RESTClient{
		APIRegistry:          registry,
		NegotiatedSerializer: scheme.Codecs,
		Client: fakerest.CreateHTTPClient(func(req *http.Request) (*http.Response, error) {
			pList := &platform.DomainList{
				Items: []platform.Domain{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "unknown",
							Name:      "unknown",
						},
						Spec: platform.DomainSpec{
							PrimaryDomain: "unknown",
						},
					},
				},
			}
			return &http.Response{
				StatusCode: 200,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       objBody(pList),
			}, nil
		}),
	}
	ts := runDomainsFakeServer(h, fmt.Sprintf("/apis/%s/domains/{fqdn}", groupVersion), "HEAD")
	resp, err := http.DefaultClient.Head(ts.URL + fmt.Sprintf("/apis/%s/domains/%s", groupVersion, fqdn))
	if err != nil {
		t.Fatalf("unexpected error executing request: %#v", err)
	}

	if resp.StatusCode != http.StatusNotFound && resp.Header.Get("X-Domain-Http-Status") != "404" {
		t.Errorf("GOT: %#v, EXPECTED: %#v, STATUSCODE: %d", resp.Header, expHeader, resp.StatusCode)
	}
}
