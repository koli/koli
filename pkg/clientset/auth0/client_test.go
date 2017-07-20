package auth0

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"reflect"
	"testing"

	"kolihub.io/koli/pkg/apis/authentication"
)

type clientFunc func(req *http.Request) (*http.Response, error)

func (f clientFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func makeTestServer(t *testing.T, obj interface{}, path string, statusCode int, validateBearerToken bool) clientFunc {
	return clientFunc(func(req *http.Request) (*http.Response, error) {
		if validateBearerToken {
			if len(req.Header.Get("Authorization")) == 0 {
				t.Fatalf("Missing Authorization Header")
			}
		}
		body, err := json.Marshal(obj)
		if err != nil {
			t.Fatalf("failed encoding obj: %v", err)
		}
		if req.URL.Path != path {
			t.Fatalf("unexpected request path: %#v", req.URL)
		}
		return &http.Response{
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			StatusCode: statusCode,
			Body:       ioutil.NopCloser(bytes.NewReader(body)),
		}, nil
	})
}

func TestAuth0GetClientCredentials(t *testing.T) {
	responseToken := &authentication.Token{"access_token": "eyJz93a...k4laUWw", "token_type": "Bearer", "expires_in": 86400}
	fakeServer := makeTestServer(t, responseToken, "/oauth/token", http.StatusOK, false)
	c := &Config{Host: "https://koli.auth0.com", Client: fakeServer}
	// https://auth0.com/docs/api/authentication#get-token
	token := &authentication.Token{
		"audience":      "https://koli.auth0.com/",
		"grant_type":    "client_credentials",
		"client_id":     "AUTH0-CLIENT-ID",
		"client_secret": "AUTH0-CLIENT-SECRET",
	}
	client, err := NewForConfig(c)
	if err != nil {
		t.Fatalf("unexpected error getting client config: %v", err)
	}
	accessToken, err := client.Authentication().ClientCredentials(token)
	if err != nil {
		t.Errorf("unexpected error getting client credentials: %v", err)
	}
	if accessToken.AccessToken() != responseToken.AccessToken() {
		t.Errorf("GOT: %#v, EXPECTED: %#v", accessToken, responseToken)
	}
}

func TestAuth0GetUserById(t *testing.T) {
	var (
		accessToken    = "myaccessktoken"
		userID         = "github|4312"
		identityUserId = 4312
		expectedUser   = &authentication.User{UserID: userID, Identities: []authentication.Identity{
			{Connection: "Initial-Connection", UserID: 129082182091, Provider: "auth0", IsSocial: false},
			{Connection: "A-Connection", UserID: identityUserId, Provider: "github", IsSocial: true},
		}}
	)
	fakeServer := makeTestServer(t, expectedUser, "/api/v2/users/"+userID, http.StatusOK, true)
	c := &Config{Host: "https://koli.auth0.com", Client: fakeServer}
	client, err := NewForConfig(c)
	if err != nil {
		t.Fatalf("unexpected error getting client config: %v", err)
	}
	u, err := client.Management(accessToken).Users().Get(userID)
	if err != nil {
		t.Errorf("unexpected error getting user: %v", err)
	}
	if !reflect.DeepEqual(u, expectedUser) {
		t.Errorf("GOT: %#v, EXPECTED: %#v", u, expectedUser)
	}
}

// TODO: test route set up: client.Reset()
