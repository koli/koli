package request

import (
	"bytes"
	"errors"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"testing"
)

type clientFunc func(req *http.Request) (*http.Response, error)

func (f clientFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestRequestSetHeaders(t *testing.T) {
	server := clientFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected headers: %#v", req.Header)
		}
		return &http.Response{
			StatusCode: http.StatusForbidden,
			Body:       ioutil.NopCloser(bytes.NewReader([]byte{})),
		}, nil
	})
	NewRequest(server, &url.URL{}).Do()
}

func TestRequestURI(t *testing.T) {
	var (
		requestURL, _ = url.Parse("https://auth0.example.io/v5")
		resource      = "users"
		resourceName  = "johndoe"
		expectedURL   = requestURL.Path + "/" + path.Join(resource, resourceName)
	)
	r := NewRequest(nil, requestURL).
		Resource(resource).
		Name(resourceName)
	if r.URL().Path != expectedURL {
		t.Errorf("expected: %s, got: %s", expectedURL, r.URL().Path)
	}
}

func TestRequestDo(t *testing.T) {
	testCases := []struct {
		Request *Request
		Err     bool
		ErrFn   func(error) bool
	}{
		{
			Request: &Request{err: errors.New("an request error")},
			Err:     true,
		},
		{
			Request: &Request{
				Client: clientFunc(func(req *http.Request) (*http.Response, error) {
					return nil, errors.New("error from server")
				}),
			},
			Err: true,
		},
	}
	for i, testCase := range testCases {
		body, err := testCase.Request.Do().Raw()
		hasErr := err != nil
		if hasErr != testCase.Err {
			t.Errorf("%d: expected: %t, got: %t: %v", i, testCase.Err, hasErr, err)
		}
		if hasErr && body != nil {
			t.Errorf("%d: body should be nil when error is returned", i)
		}
	}
}
