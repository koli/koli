package util

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"strings"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/client/restclient"
	"k8s.io/kubernetes/pkg/runtime"
)

// Request allows for building up a request to a server in a chained fashion.
// Any errors are stored until the end of your call, so you only have to
// check once.
type Request struct {
	// required
	client restclient.HTTPClient
	verb   string

	baseURL     *url.URL
	serializers restclient.Serializers

	// generic components accessible via method setters
	params     url.Values
	headers    http.Header
	hostHeader string

	pathPrefix   string
	resource     string
	resourceName string
	subresource  string
	subpath      string

	// output
	err  error
	body io.Reader

	// The constructed request and the response
	req  *http.Request
	resp *http.Response
}

// Name sets the name of a resource to access (<resource>/[ns/<namespace>/]<name>)
func (r *Request) Name(resourceName string) *Request {
	if r.err != nil {
		return r
	}
	if len(resourceName) == 0 {
		r.err = fmt.Errorf("resource name may not be empty")
		return r
	}
	if len(r.resourceName) != 0 {
		r.err = fmt.Errorf("resource name already set to %q, cannot change to %q", r.resourceName, resourceName)
		return r
	}
	r.resourceName = resourceName
	return r
}

// Suffix appends segments to the end of the path. These items will be placed after the prefix and optional
// Namespace, Resource, or Name sections.
func (r *Request) Suffix(segments ...string) *Request {
	if r.err != nil {
		return r
	}
	r.subpath = path.Join(r.subpath, path.Join(segments...))
	return r
}

// Resource sets the resource to access (<resource>/[ns/<namespace>/]<name>)
func (r *Request) Resource(resource string) *Request {
	if r.err != nil {
		return r
	}
	if len(r.resource) != 0 {
		r.err = fmt.Errorf("resource already set to %q, cannot change to %q", r.resource, resource)
		return r
	}
	r.resource = resource
	return r
}

// SubResource sets a sub-resource path which can be multiple
// segments segment after the resource name but before the suffix.
func (r *Request) SubResource(subresources ...string) *Request {
	if r.err != nil {
		return r
	}
	subresource := path.Join(subresources...)
	if len(r.subresource) != 0 {
		r.err = fmt.Errorf("subresource already set to %q, cannot change to %q", r.resource, subresource)
		return r
	}
	r.subresource = subresource
	return r
}

// URL returns the current working URL.
func (r *Request) URL() *url.URL {
	p := r.pathPrefix
	if len(r.resource) != 0 {
		p = path.Join(p, strings.ToLower(r.resource))
	}
	// Join trims trailing slashes, so preserve r.pathPrefix's trailing slash for backwards compatibility if nothing was changed
	if len(r.resourceName) != 0 || len(r.subpath) != 0 || len(r.subresource) != 0 {
		p = path.Join(p, r.resourceName, r.subresource, r.subpath)
	}

	finalURL := &url.URL{}
	if r.baseURL != nil {
		*finalURL = *r.baseURL
	}
	finalURL.Path = p

	query := url.Values{}
	for key, values := range r.params {
		for _, value := range values {
			query.Add(key, value)
		}
	}

	finalURL.RawQuery = query.Encode()
	return finalURL
}

// SetHeader appends headers to the request
func (r *Request) SetHeader(key, value string) *Request {
	if r.headers == nil {
		r.headers = http.Header{}
	}
	r.headers.Set(key, value)
	return r
}

// SetHostHeader add a HOST header to the request
func (r *Request) SetHostHeader(host string) *Request {
	r.hostHeader = host
	return r
}

// SetSerializer adds a new serializer
func (r *Request) SetSerializer(gv unversioned.GroupVersion, ns runtime.NegotiatedSerializer) *Request {
	serializer, _ := ns.SerializerForMediaType(runtime.ContentTypeJSON, nil)
	streamingSerializer, _ := ns.StreamingSerializerForMediaType(runtime.ContentTypeJSON, nil)

	internalVersion := unversioned.GroupVersion{
		Group:   gv.Group,
		Version: gv.Version,
		//Version: runtime.APIVersionInternal,
	}

	r.serializers = restclient.Serializers{
		//Encoder: ns.EncoderForVersion(serializer, *testapi.Default.GroupVersion()),
		//Decoder: ns.DecoderToVersion(serializer, internalVersion),
		Encoder:             api.Codecs.EncoderForVersion(serializer, gv),
		Decoder:             ns.DecoderToVersion(serializer, internalVersion),
		StreamingSerializer: streamingSerializer,
		Framer:              streamingSerializer.Framer,
	}
	return r
}

// GET change the method of the request to GET
func (r *Request) GET() *Request {
	r.verb = "GET"
	return r
}

// POST change the method of the request to POST
func (r *Request) POST() *Request {
	r.verb = "POST"
	return r
}

// PUT change the method of the request to PUT
func (r *Request) PUT() *Request {
	r.verb = "PUT"
	return r
}

// DELETE change the method of the request to DELETE
func (r *Request) DELETE() *Request {
	r.verb = "DELETE"
	return r
}

// Do formats and executes the request. Returns a Result object for easy response
// processing.
//
// Error type:
//  * If the request can't be constructed, or an error happened earlier while building its
//    arguments: *RequestConstructionError
//  * If the server responds with a status: *errors.StatusError or *errors.UnexpectedObjectError
//  * http.Client.Do errors are returned directly.
func (r *Request) Do() Result {
	var result Result
	err := r.request(func(req *http.Request, resp *http.Response) {
		result = r.transformResponse(resp, req)
	})
	if err != nil {
		return Result{err: err}
	}
	return result
}

// request connects to the server and invokes the provided function when a server response is
// received. It handles retry behavior and up front validation of requests. It will invoke
// fn at most once. It will return an error if a problem occurred prior to connecting to the
// server - the provided function is responsible for handling server errors.
func (r *Request) request(fn func(*http.Request, *http.Response)) error {
	if r.err != nil {
		return r.err
	}

	client := r.client
	if client == nil {
		client = http.DefaultClient
	}

	url := r.URL().String()
	req, err := http.NewRequest(r.verb, url, r.body)
	if err != nil {
		return err
	}
	req.Header = r.headers
	if r.hostHeader != "" {
		req.Host = r.hostHeader
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	fn(req, resp)
	return nil

}

// transformResponse converts an API response into a structured API object
func (r *Request) transformResponse(resp *http.Response, req *http.Request) Result {
	var body []byte
	if resp.Body != nil {
		if data, err := ioutil.ReadAll(resp.Body); err == nil {
			body = data
		}
	}
	return Result{
		body:        body,
		contentType: resp.Header.Get("Content-Type"),
		statusCode:  resp.StatusCode,
		decoder:     r.serializers.Decoder,
	}
}

// Body .
func (r *Request) Body(data []byte) *Request {
	if r.err != nil {
		return r
	}
	if data == nil {
		return r
	}
	//data, err := runtime.Encode(r.serializers.Encoder, obj)
	// if err != nil {
	// r.err = err
	// return r
	// }
	r.body = bytes.NewReader(data)
	//r.body = data

	return r
}

// NewRequest creates a new request helper object for accessing runtime.Objects on a server.
func NewRequest(client restclient.HTTPClient, verb string, baseURL *url.URL) *Request {
	r := &Request{
		client:      client,
		serializers: restclient.Serializers{},
		verb:        verb,
		baseURL:     baseURL,
		pathPrefix:  baseURL.Path,
	}
	r.SetHeader("Content-Type", "application/json")
	return r
}

// Result contains the result of calling Request.Do().
type Result struct {
	body        []byte
	contentType string
	err         error
	statusCode  int

	decoder runtime.Decoder
}

// Raw returns the raw result.
func (r Result) Raw() ([]byte, error) {
	return r.body, r.err
}

// Get returns the result as an object.
func (r Result) Get() (runtime.Object, error) {
	if r.err != nil {
		return nil, r.err
	}
	if r.decoder == nil {
		return nil, fmt.Errorf("serializer for %s doesn't exist", r.contentType)
	}
	return runtime.Decode(r.decoder, r.body)
}

// StatusCode returns the HTTP status code of the request. (Only valid if no
// error was returned.)
func (r Result) StatusCode() int {
	return r.statusCode
}

// Error returns the error executing the request, nil if no error occurred.
// See the Request.Do() comment for what errors you might get.
func (r Result) Error() error {
	return r.err
}
