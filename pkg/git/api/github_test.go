package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"reflect"
	"strconv"
	"testing"

	"github.com/google/go-github/github"
	"github.com/gorilla/mux"

	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	core "k8s.io/client-go/testing"

	platform "kolihub.io/koli/pkg/apis/core/v1alpha1"
	"kolihub.io/koli/pkg/apis/core/v1alpha1/draft"
	"kolihub.io/koli/pkg/git/conf"
	"kolihub.io/koli/pkg/request"
	koliutil "kolihub.io/koli/pkg/util"
)

func runHttpTestServer(router *mux.Router, gitHandler *Handler, client *fake.Clientset) (*url.URL, *httptest.Server) {
	ts := httptest.NewServer(router)
	gitHandler.gitClient = github.NewClient(nil)
	url, _ := url.Parse(ts.URL)
	gitHandler.gitClient.BaseURL = url
	gitHandler.gitClient.UploadURL = url
	requestURL, _ := url.Parse(ts.URL)
	gitHandler.clientset = client
	return requestURL, ts
}

type clientFunc func(req *http.Request) (*http.Response, error)

func (f clientFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func makeAuth0Server(t *testing.T, objToken, objUser interface{}, userID string) clientFunc {
	return clientFunc(func(req *http.Request) (*http.Response, error) {
		var body []byte
		var err error
		switch req.URL.Path {
		case "/oauth/token":
			if req.Method != "POST" {
				t.Fatalf("unexpected method: %#v", req.Method)
			}
			body, err = json.Marshal(objToken)
			if err != nil {
				t.Fatalf("failed encoding obj: %v", err)
			}
		case "/api/v2/users/" + url.QueryEscape(userID):
			if req.Method != "GET" {
				t.Fatalf("unexpected method: %#v", req.Method)
			}
			if len(req.Header.Get("Authorization")) == 0 {
				t.Fatalf("Missing Authorization Header")
			}
			body, err = json.Marshal(objUser)
			if err != nil {
				t.Fatalf("failed encoding obj: %v", err)
			}
		default:
			t.Fatalf("unexpected path: %#v", req.URL.Path)
		}

		return &http.Response{
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			StatusCode: http.StatusOK,
			Body:       ioutil.NopCloser(bytes.NewReader(body)),
		}, nil
	})
}

func Int(i int) *int          { return &i }
func Bool(b bool) *bool       { return &b }
func String(s string) *string { return &s }

func TestGitHubSearchRepos(t *testing.T) {
	var (
		router                   = mux.NewRouter()
		gitHandler               = NewHandler(&conf.Config{}, nil)
		expectedRepoSearchResult = &github.RepositoriesSearchResult{
			Total:             Int(2),
			IncompleteResults: Bool(false),
			Repositories:      []github.Repository{{ID: Int(1)}, {ID: Int(2)}},
		}
	)
	router.HandleFunc("/github/user/repos", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gitHandler.GitHubSearchRepos(w, r)
	})).Methods("GET")
	router.HandleFunc("/search/repositories", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(expectedRepoSearchResult)
	})).Methods("GET")
	requestURL, ts := runHttpTestServer(router, &gitHandler, nil)
	defer ts.Close()

	result := &github.RepositoriesSearchResult{}
	if err := request.NewRequest(nil, requestURL).Get().
		RequestPath("/github/user/repos").
		Do().
		Into(result); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(expectedRepoSearchResult, result) {
		t.Errorf("GOT: %#v, EXPECTED: %#v", result, expectedRepoSearchResult)
	}
}

// TODO: test pagination
func TestGitHubListOrgRepos(t *testing.T) {
	var (
		router         = mux.NewRouter()
		gitHandler     = NewHandler(&conf.Config{}, nil)
		orgName        = "kolihub"
		expectedResult = []github.Repository{
			{ID: Int(1)}, {ID: Int(2)},
		}
	)
	router.HandleFunc("/github/orgs/{org}/repos", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gitHandler.GitHubListOrgRepos(w, r)
	})).Methods("GET")
	router.HandleFunc(fmt.Sprintf("/orgs/%s/repos", orgName), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(expectedResult)
	})).Methods("GET")
	requestURL, ts := runHttpTestServer(router, &gitHandler, nil)
	defer ts.Close()

	result := []github.Repository{}
	if err := request.NewRequest(nil, requestURL).Get().
		RequestPath("/" + path.Join("github", "orgs", orgName, "repos")).
		Do().
		Into(&result); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(expectedResult, result) {
		t.Errorf("GOT: %#v, EXPECTED: %#v", result, expectedResult)
	}
}

func TestGitHubListBranches(t *testing.T) {
	var (
		router         = mux.NewRouter()
		gitHandler     = NewHandler(&conf.Config{}, nil)
		expectedResult = []github.Branch{
			{Name: String("master"), Commit: &github.RepositoryCommit{SHA: String("a57781")}},
			{Name: String("development"), Commit: &github.RepositoryCommit{SHA: String("4c4aea")}},
		}
		repo = "owner/repo"
	)
	router.HandleFunc("/github/repos/{owner}/{repo}/branches", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gitHandler.GitHubListBranches(w, r)
	})).Methods("GET")
	router.HandleFunc(fmt.Sprintf("/repos/%s/branches", repo), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(expectedResult)
	})).Methods("GET")
	requestURL, ts := runHttpTestServer(router, &gitHandler, nil)
	defer ts.Close()

	result := []github.Branch{}
	if err := request.NewRequest(nil, requestURL).Get().
		RequestPath("/" + path.Join("github", "repos", repo, "branches")).
		Do().
		Into(&result); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(expectedResult, result) {
		t.Errorf("GOT: %#v, EXPECTED: %#v", result, expectedResult)
	}
}

// TODO: test add hook when a hook doesn't exist for a repo

func TestGitHubAddHooks(t *testing.T) {
	var (
		userID, hookID    = "github|2391", 3
		deployName, ns    = "foo-app", "foo-ns"
		gitowner, gitrepo = "foo-owner-github", "foo-repo-github"
		expectedDeploy    = &v1beta1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name: deployName, Namespace: ns,
				Annotations: map[string]string{
					"kolihub.io/hookid":  strconv.Itoa(hookID),
					"kolihub.io/gituser": userID,
				},
				Labels: map[string]string{
					"kolihub.io/gitowner": gitowner,
					"kolihub.io/gitrepo":  gitrepo,
				},
			},
		}
		client           = fake.NewSimpleClientset(runtime.Object(expectedDeploy))
		router           = mux.NewRouter()
		gitHandler       = NewHandler(&conf.Config{GitAPIHostname: "https://gitapi.kolihub.io"}, nil)
		existentHookList = []github.Hook{{ID: Int(1)}, {ID: Int(2)}}
		// https://developer.github.com/v3/repos/hooks/#create-a-hook
		expectedHook = &github.Hook{
			ID:   Int(hookID),
			Name: String("web"),
			Config: map[string]interface{}{
				"insecure_ssl":   float64(0),
				"content_type":   "json",
				"url":            fmt.Sprintf("%s/hooks", gitHandler.cnf.GitAPIHostname),
				"secret":         gitHandler.cnf.GitHubHookSecret,
				"kolihub.io/ref": "gaia", // just a ref to know the source of the hook
			},
			Events: []string{"push"},
		}
	)
	router.HandleFunc("/github/repos/{owner}/{repo}/hooks", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gitHandler.GitHubAddHooks(w, r)
	})).Methods("POST")
	router.HandleFunc("/repos/{gitowner}/{gitrepo}/hooks", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET": // list existing hooks
			json.NewEncoder(w).Encode(existentHookList)
		case "POST": // creating a new hook
			createHook := &github.Hook{}
			if err := json.NewDecoder(r.Body).Decode(createHook); err != nil {
				t.Fatalf("failed decoding hook: %v", err)
			}
			createHook.ID = &hookID
			json.NewEncoder(w).Encode(createHook)
		}

	})).Methods("POST", "GET")
	requestURL, ts := runHttpTestServer(router, &gitHandler, client)
	defer ts.Close()

	gitHandler.user = &platform.User{Sub: userID}

	result := &github.Hook{}
	if err := request.NewRequest(nil, requestURL).Post().
		RequestPath("/" + path.Join("github", "repos", gitowner, gitrepo, "hooks")).
		Body(map[string]string{"namespace": ns, "deploy": deployName}).
		Do().
		Into(&result); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !reflect.DeepEqual(result, expectedHook) {
		t.Errorf("GOT: %v, EXPECTED: %v", result, expectedHook)
	}

	if len(client.Actions()) == 0 {
		t.Errorf("GOT: 0 action, EXPECTED: 1 action")
	}
	for _, action := range client.Actions() {
		switch tp := action.(type) {
		case core.PatchActionImpl:
			codec := scheme.Codecs.LegacyCodec(v1beta1.SchemeGroupVersion)
			// clean Name and Namespace for generating the expected diff
			expectedDeploy.Name = ""
			expectedDeploy.Namespace = ""
			expectedPatchData, _ := koliutil.StrategicMergePatch(codec, &v1beta1.Deployment{}, expectedDeploy)
			if string(tp.Patch) != string(expectedPatchData) {
				t.Errorf("GOT: %s, EXPECTED: %s", string(tp.Patch), string(expectedPatchData))
			}
		default:
			t.Fatalf("unexpected action: %T, %#v", action, action)
		}
	}
}

func TestGetGitHubHook(t *testing.T) {
	var (
		gitOwner, gitRepo = "foo-owner", "foo-repo"
		hookID            = 3
		router            = mux.NewRouter()
		gitHandler        = NewHandler(&conf.Config{}, nil)
		expectedHook      = &github.Hook{ID: &hookID, Active: Bool(true)}
		// repo = "owner/repo"
	)
	router.HandleFunc("/github/repos/{owner}/{repo}/hooks/{id}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gitHandler.GitHubHooks(w, r)
	})).Methods("GET")
	router.HandleFunc("/repos/{gitowner}/{gitrepo}/hooks/{hookid}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := mux.Vars(r)
		hookID, _ := strconv.Atoi(p["hookid"])
		getHook := &github.Hook{ID: &hookID, Active: Bool(true)}
		json.NewEncoder(w).Encode(getHook)
	})).Methods("GET")
	requestURL, ts := runHttpTestServer(router, &gitHandler, nil)
	defer ts.Close()

	got := &github.Hook{}
	if err := request.NewRequest(nil, requestURL).Get().
		RequestPath("/" + path.Join("github", "repos", gitOwner, gitRepo, "hooks", strconv.Itoa(hookID))).
		Do().
		Into(&got); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, expectedHook) {
		t.Errorf("GOT: %#v, EXPECTED: %#v", got, expectedHook)
	}
}

// TODO: test removing github hook
func TestRemoveExistentAssociationHookFromDeployment(t *testing.T) {
	var (
		gitOwner, gitRepo      = "foo-owner", "foo-repo"
		targetDeploy, targetNs = "foo", "foo-ns"
		hookID                 = 3
		labelHookRef           = map[string]string{"kolihub.io/gitowner": gitOwner, "kolihub.io/gitrepo": gitRepo}
		expectedDeploy         = draft.NewDeployment(&v1beta1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      targetDeploy,
				Namespace: targetDeploy,
				Labels:    labelHookRef,
				Annotations: map[string]string{
					platform.AnnotationAuthToken:  "a.jwt.token",
					platform.AnnotationGitHubUser: "github|3204",
					"kolihub.io/hookid":           strconv.Itoa(hookID),
				},
			},
		})
		router     = mux.NewRouter()
		gitHandler = NewHandler(&conf.Config{}, nil)
		// expectedHook = &github.Hook{ID: &hookID, Active: Bool(true)}
		client = fake.NewSimpleClientset([]runtime.Object{
			expectedDeploy.GetObject(),
			&v1beta1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "bar", Namespace: "bar-ns", Labels: labelHookRef}},
		}...)
		// repo = "owner/repo"
	)
	router.HandleFunc("/github/repos/{owner}/{repo}/hooks", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gitHandler.GitHubHooks(w, r)
	})).Methods("DELETE")
	// router.HandleFunc("/repos/{gitowner}/{gitrepo}/hooks/{hookid}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	// 	p := mux.Vars(r)
	// 	id, _ := strconv.Atoi(p["hookid"])
	// 	if id != hookID || p["gitowner"] != gitOwner || p["gitrepo"] != gitRepo {
	// 		t.Fatalf("parameters doesn't match: %#v", p)
	// 	}
	// 	w.WriteHeader(http.StatusNoContent)
	// })).Methods("DELETE")
	requestURL, ts := runHttpTestServer(router, &gitHandler, client)
	defer ts.Close()

	data := map[string]string{"namespace": targetNs, "deploy": targetDeploy}
	if res := request.NewRequest(nil, requestURL).Delete().
		RequestPath("/" + path.Join("github", "repos", gitOwner, gitRepo, "hooks")).
		Body(data).
		Do(); res.StatusCode() != http.StatusNoContent {
		t.Fatalf("unexpected status code: %#v", res)
	}

	if len(client.Actions()) == 0 {
		t.Errorf("GOT: 0 actions, EXPECTED: 1 action")
	}
	for _, action := range client.Actions() {
		switch tp := action.(type) {
		case core.PatchActionImpl:
			original, _ := expectedDeploy.Copy()
			delete(expectedDeploy.Labels, "kolihub.io/gitowner")
			delete(expectedDeploy.Labels, "kolihub.io/gitrepo")

			delete(expectedDeploy.Annotations, platform.AnnotationGitHubUser)
			delete(expectedDeploy.Annotations, "kolihub.io/hookid")
			delete(expectedDeploy.Annotations, platform.AnnotationAuthToken)

			codec := scheme.Codecs.LegacyCodec(v1beta1.SchemeGroupVersion)
			expectedPatch, _ := koliutil.StrategicMergePatch(codec, original.GetObject(), expectedDeploy.GetObject())
			if string(tp.Patch) == "{}" {
				t.Fatalf("GOT null patch: %s", string(tp.Patch))
			}
			if string(expectedPatch) != string(tp.Patch) {
				t.Errorf("GOT: %s, EXPECTED: %s", string(tp.Patch), string(expectedPatch))
			}

		case core.ListActionImpl: //no-op
		default:
			t.Fatalf("unexpected action: %T, %#v", action, action)
		}
	}
}

func TestWebhookDeployOneApp(t *testing.T) {
	var (
		ownerRepo, repoName, ref = "acme", "foo", "refs/heads/master"
		deployName, deployNs     = "foo", "prod-c1-acme"
		commit                   = "e8cf15915443876a4d6e79cf75f41600377c49e1"
		cloneURL                 = "https://github.com/acme/foo.git"
		compareURL               = "https://api.github.com/repos/acme/foo/compare/{base}...{head}"
		labelRef                 = map[string]string{"kolihub.io/gitowner": ownerRepo, "kolihub.io/gitrepo": repoName}
		router                   = mux.NewRouter()
		gitHandler               = NewHandler(&conf.Config{}, nil)
		// expectedHook = &github.Hook{ID: &hookID, Active: Bool(true)}
		client = fake.NewSimpleClientset([]runtime.Object{
			&v1beta1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      deployName,
					Namespace: deployNs,
					Labels:    labelRef,
					Annotations: map[string]string{
						platform.AnnotationGitBranch: ref,
					},
				},
			},
			&v1beta1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "bar", Namespace: "wrong-namespace"}},
		}...)
		expectedPushEvent  = newPushEvent(ownerRepo, repoName, ref, commit, cloneURL, compareURL)
		expectedDeployMeta = metav1.ObjectMeta{
			Annotations: map[string]string{
				platform.AnnotationBuildRevision: "1",
				platform.AnnotationGitCompare:    compareURL,
				platform.AnnotationGitRemote:     cloneURL,
				platform.AnnotationGitRepository: path.Join(ownerRepo, repoName),
				platform.AnnotationGitRevision:   commit,
				platform.AnnotationBuildSource:   "github",
				platform.AnnotationBuild:         "true",
			},
		}
		// repo = "owner/repo"
	)
	router.HandleFunc("/hooks", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Add("X-Github-Event", "push") // fake a webhook push event
		gitHandler.Webhooks(w, r)
	})).Methods("POST", "GET")

	requestURL, ts := runHttpTestServer(router, &gitHandler, client)
	defer ts.Close()
	gitHandler.clientset = client

	result := request.NewRequest(nil, requestURL).Post().
		RequestPath("/hooks").
		Body(expectedPushEvent).
		Do()
	if !result.IsSuccess() {
		rawData, _ := result.Raw()
		t.Fatalf("failed requesting hooks endpoint, ERR: %#v, RESP: %s", result.Error(), string(rawData))
	}

	if len(client.Actions()) != 2 {
		t.Errorf("GOT: %d action(s), EXPECTED: 2 action", len(client.Actions()))
	}

	// var expectedDeployPatch []byte
	for _, action := range client.Actions() {
		switch tp := action.(type) {
		case core.PatchActionImpl:
			d := &v1beta1.Deployment{}
			if err := json.Unmarshal(tp.Patch, d); err != nil {
				t.Fatalf("unexpected error unmarshaling deploy: %#v", err)
			}
			if !reflect.DeepEqual(expectedDeployMeta.Annotations, d.Annotations) {
				t.Errorf("GOT: %#v, EXPECTED: %#v", d.Annotations, expectedDeployMeta.Annotations)
			}
		case core.ListActionImpl: //no-op
		default:
			t.Fatalf("unexpected action: %T, %#v", action, action)
		}
	}
}

func newPushEvent(ownerName, repoName, ref, headCommit, cloneURL, compareURL string) *github.PushEvent {
	return &github.PushEvent{
		Repo: &github.PushEventRepository{
			Name:     String(repoName),
			Owner:    &github.PushEventRepoOwner{Name: String(ownerName)},
			CloneURL: String(cloneURL),
			FullName: String(path.Join(ownerName, repoName)),
		},
		HeadCommit: &github.PushEventCommit{ID: String(headCommit)},
		Compare:    String(compareURL),
		Ref:        String(ref),
	}
}
