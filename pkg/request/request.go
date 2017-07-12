package request

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/golang/glog"
)

const (
	auth0AdminURL = "https://koli.auth0.com/oauth/token"
)

type Interface interface {
	Get() *Request
	Post() *Request
	Put() *Request
	Patch() *Request
	Delete() *Request
	Verb(verb string) *Request
	Reset() *Request
}

// HTTPClient is an interface for testing a request object.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Result contains the result of calling Request.Do().
type Result struct {
	body        []byte
	contentType string
	err         error
	statusCode  int
}

func (r Result) Raw() ([]byte, error) {
	if r.err != nil {
		return nil, r.err
	}
	if !r.IsSuccess() {
		return nil, NewHTTPError(r.statusCode, "failed performing request to remote server: %s", string(r.body))
		// return nil, fmt.Errorf("response error (%d) from auth0 [%s]", r.StatusCode(), string(r.body))
	}
	return r.body, nil
}

func (r Result) Into(obj interface{}) error {
	if r.err != nil {
		return r.err
	}
	if !r.IsSuccess() {
		return NewHTTPError(r.statusCode, "failed performing request to remote server: %s", string(r.body))
		// return fmt.Errorf("response error (%d) from auth0 [%s]", r.statusCode, string(r.body))
	}
	if err := json.Unmarshal(r.body, obj); err != nil {
		return fmt.Errorf("failed decoding response [%v]", err)
	}
	return nil
}

func (r Result) StatusCode() int {
	return r.statusCode
}

func (r Result) IsSuccess() bool {
	return r.statusCode == 200 || r.statusCode == 201 || r.statusCode == 204 || r.statusCode == 202
}

func (r Result) ContentType() string {
	return r.contentType
}

func (r Result) Error() error {
	return r.err
}

type Request struct {
	Client HTTPClient

	baseURL      *url.URL
	pathPrefix   string
	resource     string
	resourceName string
	verb         string
	timeout      time.Duration

	// This is only used for per-request timeouts, deadlines, and cancellations.
	ctx context.Context

	body    io.Reader
	headers http.Header
	err     error
}

func NewRequest(client HTTPClient, baseURL *url.URL) *Request {
	if client == nil {
		client = http.DefaultClient
	}
	request := &Request{
		baseURL: baseURL,
		Client:  client,
		headers: http.Header{"Content-Type": []string{"application/json"}},
	}
	return request
}

func (r *Request) URL() *url.URL {
	p := path.Join(r.pathPrefix, r.resource, r.resourceName)
	finalURL := &url.URL{}
	if r.baseURL != nil {
		p = path.Join(r.pathPrefix, r.baseURL.Path, r.resource, r.resourceName)
		*finalURL = *r.baseURL
	}
	finalURL.Path = p
	return finalURL
}

func (r *Request) SetHeader(key, value string) *Request {
	if r.headers == nil {
		r.headers = http.Header{}
	}
	r.headers.Set(key, value)
	return r
}

// Timeout makes the request use the given duration as a timeout. Sets the "timeout"
// parameter.
func (r *Request) Timeout(d time.Duration) *Request {
	if r.err != nil {
		return r
	}
	r.timeout = d
	return r
}

// Prefix adds segments to the relative beginning to the request path. These
// items will be placed before the Resource, or Name sections.
func (r *Request) Prefix(segments ...string) *Request {
	r.pathPrefix = ""
	r.pathPrefix = path.Join(r.pathPrefix, path.Join(segments...))
	return r
}

// RequestPath override the basePath of the request
func (r *Request) RequestPath(basePath string) *Request {
	r.baseURL.Path = basePath
	r.resource = ""
	r.resourceName = ""
	return r
}

func (r *Request) Resource(name string) *Request {
	r.resource = name
	return r
}

func (r *Request) Name(resourceName string) *Request {
	r.resourceName = resourceName
	return r
}

// Context adds a context to the request. Contexts are only used for
// timeouts, deadlines, and cancellations.
func (r *Request) Context(ctx context.Context) *Request {
	r.ctx = ctx
	return r
}

func (r *Request) Patch() *Request {
	r.verb = "PATCH"
	return r
}

func (r *Request) Get() *Request {
	r.verb = "GET"
	return r
}

func (r *Request) Post() *Request {
	r.verb = "POST"
	return r
}

func (r *Request) Put() *Request {
	r.verb = "PUT"
	return r
}

func (r *Request) Delete() *Request {
	r.verb = "DELETE"
	return r
}

func (r *Request) Verb(verb string) *Request {
	r.verb = verb
	return r
}

// Reset will clean all paths
func (r *Request) Reset() *Request {
	r.resourceName = ""
	r.resource = ""
	r.pathPrefix = ""
	return r
}

func (r *Request) Body(bodyData interface{}) *Request {
	reqBody, err := json.Marshal(bodyData)
	if glog.V(6) {
		glog.Infof("Resquest Body: %s", string(reqBody))
	}
	if err != nil {
		r.err = fmt.Errorf("failed encoding body [%v]", err)
	}
	r.body = bytes.NewBuffer(reqBody)
	return r
}

func (r *Request) Error() error {
	return r.err
}

func (r *Request) Do() *Result {
	result := &Result{}
	if r.err != nil {
		result.err = r.err
		return result
	}
	client := r.Client
	if r.Client == nil {
		client = http.DefaultClient
	}
	if glog.V(4) {
		glog.Infof("Verb %#v, URL: %#v, URLPath %#v", r.verb, r.URL().String(), r.URL().Path)
	}

	request, err := http.NewRequest(r.verb, r.URL().String(), r.body)
	if err != nil {
		r.err = fmt.Errorf("failed creating request [%v]", err)
		return result
	}
	request.Header = r.headers
	resp, err := client.Do(request)
	if err != nil {
		result.err = fmt.Errorf("failed processing the request [%v]", err)
		return result
	}
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if glog.V(8) {
		glog.Infof("Response Body[%d]: %s", resp.StatusCode, string(data))
	}
	if err != nil {
		result.err = fmt.Errorf("failed reading response [%v]", err)
		result.statusCode = resp.StatusCode
		return result
	}
	result.contentType = resp.Header.Get("Content-Type")
	result.body = data
	result.statusCode = resp.StatusCode
	return result
}
