package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"

	"github.com/golang/glog"
	"github.com/google/go-github/github"
	"github.com/gorilla/mux"
	"golang.org/x/oauth2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/pkg/api"
	platform "kolihub.io/koli/pkg/apis/v1alpha1"
	"kolihub.io/koli/pkg/apis/v1alpha1/draft"
	gitutil "kolihub.io/koli/pkg/git/util"
)

const (
	auth0AuthURL          = "https://koli.auth0.com/oauth/token"
	githubBuildSourceName = "github"
)

type githubUser struct {
	Identities []auth0Identity `json:"identities"`
}

type auth0Token struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	Audience     string `json:"audience"`
	GrantType    string `json:"grant_type"`
}

type auth0Identity struct {
	AccessToken string `json:"access_token"`
	Provider    string `json:"provider"`
	UserID      int    `json:"user_id"`
	IsSocial    bool   `json:"isSocial"`
}

// GitHubSearchRepos lookup for repositores at GitHub
func (h *Handler) GitHubSearchRepos(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	identity, err := h.gitHubAccessToken()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "failed getting admin token: %s\n", err)
		return
	}

	// TODO: blocking request, we should set a timeout for failing fast!
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: identity.AccessToken})
	githubcli := github.NewClient(oauth2.NewClient(ctx, ts))
	page, perPage := parsePages(r.URL.Query())
	gitOptions := &github.SearchOptions{
		ListOptions: github.ListOptions{PerPage: perPage, Page: page},
	}
	repos, resp, err := githubcli.Search.Repositories(ctx, qs.Get("q"), gitOptions)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "failed searching repos: %s\n", err)
		return
	}
	if resp.NextPage != 0 {
		q := fmt.Sprintf("page=%d&per_page=%d", resp.NextPage, perPage)
		nextPageURL := fmt.Sprintf("http://%s/%s?%s", r.Host, r.URL.Path, q)
		w.Header().Set("Location", nextPageURL)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(repos)
}

// GitHubListOrgRepos list repositories from organizations
func (h *Handler) GitHubListOrgRepos(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	identity, err := h.gitHubAccessToken()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "failed getting admin token: %s\n", err)
		return
	}

	// TODO: blocking request, we should set a timeout for failing fast!
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: identity.AccessToken})
	githubcli := github.NewClient(oauth2.NewClient(ctx, ts))
	qs := r.URL.Query()

	page, perPage := parsePages(qs)
	gitOptions := &github.RepositoryListByOrgOptions{
		ListOptions: github.ListOptions{PerPage: perPage, Page: page},
		Type:        qs.Get("type"),
	}

	repos, resp, err := githubcli.Repositories.ListByOrg(ctx, params["org"], gitOptions)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "failed listing organization repos: %s\n", err)
		return
	}
	if resp.NextPage != 0 {
		q := fmt.Sprintf("page=%d&per_page=%d", resp.NextPage, perPage)
		nextPageURL := fmt.Sprintf("http://%s/%s?%s", r.Host, r.URL.Path, q)
		w.Header().Set("Location", nextPageURL)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(repos)
}

// GitHubListUserRepos list repositories from users
func (h *Handler) GitHubListUserRepos(w http.ResponseWriter, r *http.Request) {
	identity, err := h.gitHubAccessToken()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "failed getting admin token: %s\n", err)
		return
	}
	// TODO: blocking request, we should set a timeout for failing fast!
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: identity.AccessToken})
	githubcli := github.NewClient(oauth2.NewClient(ctx, ts))
	qs := r.URL.Query()

	page, perPage := parsePages(qs)
	gitOptions := &github.RepositoryListOptions{
		ListOptions: github.ListOptions{PerPage: perPage, Page: page},
		Type:        qs.Get("type"),
	}

	repos, resp, err := githubcli.Repositories.List(ctx, "", gitOptions)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "failed listing user repos: %s\n", err)
		return
	}
	if resp.NextPage != 0 {
		q := fmt.Sprintf("page=%d&per_page=%d", resp.NextPage, perPage)
		nextPageURL := fmt.Sprintf("http://%s/%s?%s", r.Host, r.URL.Path, q)
		w.Header().Set("Location", nextPageURL)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(repos)
}

// GitHubListBranches list branches from a repository
func (h *Handler) GitHubListBranches(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	identity, err := h.gitHubAccessToken()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "failed getting admin token: %s\n", err)
		return
	}
	// TODO: blocking request, we should set a timeout for failing fast!
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: identity.AccessToken})
	githubcli := github.NewClient(oauth2.NewClient(ctx, ts))
	qs := r.URL.Query()

	page, perPage := parsePages(qs)
	gitOptions := &github.ListOptions{PerPage: perPage, Page: page}
	branches, resp, err := githubcli.Repositories.ListBranches(ctx, params["owner"], params["repo"], gitOptions)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "failed listing branches: %s\n", err)
		return
	}
	if resp.NextPage != 0 {
		q := fmt.Sprintf("page=%d&per_page=%d", resp.NextPage, perPage)
		nextPageURL := fmt.Sprintf("http://%s/%s?%s", r.Host, r.URL.Path, q)
		w.Header().Set("Location", nextPageURL)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(branches)
}

func (h *Handler) gitHubAccessToken() (*auth0Identity, error) {
	t := &auth0Token{
		ClientID:     h.cnf.AdminClientID,
		ClientSecret: h.cnf.AdminClientSecret,
		Audience:     h.cnf.AdminAudienceURL,
		GrantType:    "client_credentials",
	}
	data, err := json.Marshal(t)
	if err != nil {
		return nil, fmt.Errorf("failed encoding request: %s", err)
	}

	resp, err := http.DefaultClient.Post(auth0AuthURL, "application/json", bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("failed retrieving admin token: %s", err)
	}
	defer resp.Body.Close()
	var d map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&d)
	adminToken, ok := d["access_token"]
	if !ok {
		return nil, fmt.Errorf("failed retrieving 'access_token' from response: %v", d)
	}

	usersURL := h.cnf.AdminAudienceURL + "users/" + url.QueryEscape(h.user.Sub)
	// TODO: verify if adminToken is nil
	req, _ := http.NewRequest("GET", usersURL, nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", adminToken.(string)))
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed retrieving github token: %s", err)
	}
	defer resp.Body.Close()
	var u githubUser
	// var id []auth0Identity

	json.NewDecoder(resp.Body).Decode(&u)
	githubIdentity := &auth0Identity{}
	for _, i := range u.Identities {
		if i.Provider != "github" {
			continue
		}
		githubIdentity = &i
	}
	if githubIdentity == nil {
		return nil, fmt.Errorf("failed finding the 'github' identity")
	}
	return githubIdentity, nil
}

// GitHubAddHooks create new webhooks into github
// https://developer.github.com/v3/repos/hooks/#create-a-hook
func (h *Handler) GitHubAddHooks(w http.ResponseWriter, r *http.Request) {
	p := mux.Vars(r)
	defer r.Body.Close()
	var payload map[string]string
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "failed decoding body from request: %s", err)
		return
	}

	namespace := payload["namespace"]
	deploy := payload["deploy"]

	identity, err := h.gitHubAccessToken()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "failed retrieving github access token: %s\n", err)
		return
	}
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: identity.AccessToken})
	gcli := github.NewClient(oauth2.NewClient(oauth2.NoContext, ts))

	hooks, err := h.listRepoHooks(gcli, p["owner"], p["repo"])
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s\n", err)
		return
	}
	var hook *github.Hook
	// validate if a hook already exists on github
	for _, hk := range hooks {
		_, exists := hk.Config["kolihub.io/ref"]
		if !exists {
			continue
		}
		hook = hk
		break
	}

	// any hook configured at github
	if hook == nil {
		hookName := "web"
		hookRequest := &github.Hook{
			Name: &hookName,
			// Active: &activate,
			Config: map[string]interface{}{
				"insecure_ssl":   0,
				"content_type":   "json",
				"url":            fmt.Sprintf("%s/hooks", h.cnf.GitAPIHostname),
				"secret":         h.cnf.GitHubHookSecret,
				"kolihub.io/ref": "gaia", // just a ref to know the source of the hook
			},
			Events: []string{"push"},
		}
		var resp *github.Response
		var err error
		hook, resp, err = gcli.Repositories.CreateHook(oauth2.NoContext, p["owner"], p["repo"], hookRequest)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "failed creating hook: %s\n", err)
			return
		}
		resp.Body.Close()
	}

	obj, err := h.clientset.Extensions().Deployments(namespace).Get(deploy, metav1.GetOptions{})
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "failed retrieving deployment: %s\n", err)
		return
	}
	dp := draft.NewDeployment(obj)
	dp.SetAnnotation(platform.AnnotationGitHubUser, h.user.Sub)
	dp.SetAnnotation("kolihub.io/hookid", strconv.Itoa(hook.GetID()))
	dp.SetAnnotation("kolihub.io/gitowner", p["owner"])
	dp.SetAnnotation("kolihub.io/gitrepo", p["repo"])
	// dp.Annotations[constants.GitHubUserConnection] = h.user.Sub
	// dp.Annotations["kolihub.io/hookid"] = strconv.Itoa(hook.GetID())
	// dp.Labels["kolihub.io/gitowner"] = p["owner"]
	// dp.Labels["kolihub.io/gitrepo"] = p["repo"]

	if _, err := h.clientset.Extensions().Deployments(namespace).Update(dp.GetObject()); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "failed updating deployment (%v)\n", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(hook)
}

func (h *Handler) listRepoHooks(client *github.Client, owner, repo string) ([]*github.Hook, error) {
	hooks, resp, err := client.Repositories.ListHooks(oauth2.NoContext, owner, repo, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed decoding body: %s", err)
		}
		return nil, fmt.Errorf("failed listing hooks: %s", string(data))
	}
	return hooks, nil
}

func (h *Handler) getGitHubCli() (*github.Client, error) {
	identity, err := h.gitHubAccessToken()
	if err != nil {
		return nil, fmt.Errorf("failed retrieving github access token: %s\n", err)
	}
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: identity.AccessToken})
	return github.NewClient(oauth2.NewClient(oauth2.NoContext, ts)), nil
}

// GitHubHooks allows read and delete github webhooks
// https://developer.github.com/v3/repos/hooks/#webhooks
func (h *Handler) GitHubHooks(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" || r.Method == "PUT" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	p := mux.Vars(r)

	switch r.Method {
	case "GET":
		hookID, err := strconv.Atoi(p["id"])
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "id in wrong format: %s\n", err)
			return
		}
		gcli, err := h.getGitHubCli()
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "failed getting github client: %s\n", err)
			return
		}
		hook, resp, err := gcli.Repositories.GetHook(oauth2.NoContext, p["owner"], p["repo"], hookID)
		if err != nil {
			statusCode := http.StatusBadRequest
			if resp != nil {
				statusCode = resp.StatusCode
			}
			w.WriteHeader(statusCode)
			fmt.Fprintf(w, "failed getting hook: %s\n", err)
			return
		}
		json.NewEncoder(w).Encode(hook)
	case "DELETE": // Removes deployment associations and hooks when possible
		var data map[string]string
		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "failed decoding body: %s\n", err)
			return
		}
		defer r.Body.Close()
		namespace, deploy := data["namespace"], data["deploy"]
		if len(namespace) == 0 || len(deploy) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "missing 'namespace' or 'deploy' data\n")
			return
		}

		l := labels.Set{
			"kolihub.io/gitowner": p["owner"],
			"kolihub.io/gitrepo":  p["repo"],
		}
		opts := metav1.ListOptions{LabelSelector: l.AsSelector().String()}
		dpList, err := h.clientset.Extensions().Deployments(api.NamespaceAll).List(opts)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "%s/%s - failed listing deploys\n", namespace, deploy)
			return
		}

		gcli, err := h.getGitHubCli()
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "failed getting github client: %s\n", err)
			return
		}

		// TODO: prevent crossing customer/org boundries,
		// an user could delete his own resources only!
		for _, dp := range dpList.Items {
			if dp.Name == deploy && dp.Namespace == namespace {
				// have only one deployment, remove the hook from github first.
				if len(dpList.Items) == 1 {
					hookID, _ := strconv.Atoi(dp.Annotations["kolihub.io/hookid"])
					// hook not found on annotations, proceed instead of return an error
					if hookID != 0 {
						resp, err := gcli.Repositories.DeleteHook(
							oauth2.NoContext,
							p["owner"],
							p["repo"],
							hookID,
						)
						if err != nil {
							statusCode := http.StatusBadRequest
							if resp != nil {
								statusCode = resp.StatusCode
							}
							w.WriteHeader(statusCode)
							fmt.Fprintf(w, "failed removing hook: %s\n", err)
							return
						}
					} else {
						hooks, err := h.listRepoHooks(gcli, p["owner"], p["repo"])
						if err != nil {
							w.WriteHeader(http.StatusBadRequest)
							fmt.Fprintf(w, "failed listing hooks: %s\n", err)
							return
						}
						// wipe all platform hooks from github
						for _, hook := range hooks {
							_, exists := hook.Config["kolihub.io/ref"]
							if exists {
								resp, err := gcli.Repositories.DeleteHook(
									oauth2.NoContext,
									p["owner"],
									p["repo"],
									hook.GetID(),
								)
								if err != nil {
									statusCode := http.StatusBadRequest
									if resp != nil {
										statusCode = resp.StatusCode
									}
									w.WriteHeader(statusCode)
									fmt.Fprintf(w, "failed removing hook: %s\n", err)
									return
								}
							}
						}
					}
				}
				// remove the association with the hook
				delete(dp.Labels, "kolihub.io/gitowner")
				delete(dp.Labels, "kolihub.io/gitrepo")

				delete(dp.Annotations, platform.AnnotationGitHubUser)
				delete(dp.Annotations, "kolihub.io/hookid")
				delete(dp.Annotations, platform.AnnotationAuthToken)
				_, err := h.clientset.Extensions().Deployments(namespace).Update(&dp)
				if err != nil {
					w.WriteHeader(http.StatusBadRequest)
					fmt.Fprintf(w, "%s/%s - failed updating deploy: %s\n", dp.Namespace, dp.Name, err)
					return
				}
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// Webhooks receives webhooks from github and trigger new releases/builds
func (h *Handler) Webhooks(w http.ResponseWriter, r *http.Request) {
	var respBody []byte
	if r.Body != nil {
		respBody, _ = ioutil.ReadAll(r.Body)
	}

	event, err := github.ParseWebHook(github.WebHookType(r), respBody)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "failed parsing webhook: %s\n", err)
		return
	}

	switch event := event.(type) {
	case *github.PushEvent:
		l := labels.Set{
			"kolihub.io/gitowner": event.Repo.Owner.GetName(),
			"kolihub.io/gitrepo":  event.Repo.GetName(),
		}
		opts := metav1.ListOptions{LabelSelector: l.AsSelector().String()}
		dList, err := h.clientset.Extensions().Deployments(metav1.NamespaceAll).List(opts)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "failed retrieving deploys: %s\n", err)
			return
		}

		for _, obj := range dList.Items {
			dp := draft.NewDeployment(&obj)
			target := filepath.Join(dp.Namespace, dp.Name)

			nsMeta := dp.GetNamespaceMetadata()
			if !nsMeta.IsValid() {
				glog.Infof("%s - namespace in wrong format", target)
				continue
			}

			if dp.GitBranch() != event.GetRef() {
				glog.Infof("%s - branch is not related: %s\n", target, dp.GitBranch())
				continue
			}

			gitUser := dp.GitHubUser()
			if !gitUser.Exists() {
				glog.Infof("%s - missing git user connection", target)
				continue
			}
			h.user = &platform.User{Sub: gitUser.String()}
			identity, err := h.gitHubAccessToken()
			if err != nil {
				glog.Infof("%s - failed retrieving github access token (%s): %s", target, err)
				continue
			}
			jwtUserToken, err := gitutil.GenerateNewJwtToken(h.cnf.PlatformClientSecret, nsMeta.Customer(), nsMeta.Organization())
			if err != nil {
				glog.Infof("%s - failed generating user token: %s", target, err)
				continue
			}

			// Always remove at this stage, if the repository is public
			// the token is not required for cloning.
			delete(dp.Annotations, platform.AnnotationAuthToken)
			delete(dp.Annotations, "kolihub.io/authtokentype")

			// TODO: check if kolihub.io/build == true, means that a build was already started
			// TODO: try to recover the revision number from the releases resources

			dp.SetAnnotation(platform.AnnotationBuildRevision, strconv.Itoa(dp.BuildRevision()+1))

			pushID := r.Header.Get("X-GitHub-Delivery")
			dp.Annotations[platform.AnnotationBuild] = "true"
			dp.Annotations[platform.AnnotationGitRemote] = event.Repo.GetCloneURL()
			dp.Annotations[platform.AnnotationGitRepository] = event.Repo.GetFullName()
			dp.Annotations[platform.AnnotationGitRevision] = event.HeadCommit.GetID()
			dp.Annotations[platform.AnnotationAuthToken] = jwtUserToken
			if event.Repo.GetPrivate() {
				cloneURL, err := getCloneURL(event.Repo.GetCloneURL(), identity.AccessToken, event.Repo.GetFullName())
				if err != nil {
					glog.Infof("%s - failed getting clone url: %s", target, err)
					continue
				}
				dp.Annotations[platform.AnnotationGitRemote] = cloneURL
			}
			dp.Annotations[platform.AnnotationGitCompare] = event.GetCompare()
			dp.Annotations[platform.AnnotationBuildSource] = githubBuildSourceName
			if _, err = h.clientset.Extensions().Deployments(dp.Namespace).Update(dp.GetObject()); err != nil {
				glog.Infof("%s - failed triggering release: %s", target, err)
				continue
			}
			glog.Infof("new release triggered sucessfully for %s. push-id: %s commit-id: %s repo: %s",
				target, pushID, event.HeadCommit.GetID(), event.Repo.GetFullName())

			fmt.Fprintf(w, "%s - new release triggered successfully.\n", target)
			// hookSecret := bytes.NewBufferString(dp.Annotations[constants.GitHubSecretHookKey]).Bytes()
			// if _, err := github.ValidatePayload(r, hookSecret); err != nil {
			// 	w.WriteHeader(http.StatusUnauthorized)
			// 	fmt.Fprintf(w, "%s - failed validating payload: %s\n", target, err)
			// 	return
			// }
		}
	case *github.PingEvent:
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "%s\n", event.GetZen())
	default:
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "unknown type: %#v\n", event)
	}
}

func getCloneURL(host, oauthToken, repository string) (string, error) {
	u, err := url.Parse(host)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s://%s:x-oauth-basic@%s/%s", u.Scheme, oauthToken, u.Host, repository) + ".git", nil
}

func parsePages(qs url.Values) (page int, perPage int) {
	page, _ = strconv.Atoi(qs.Get("page"))
	if page == 0 {
		page = 1
	}
	perPage, _ = strconv.Atoi(qs.Get("per_page"))
	if perPage == 0 {
		perPage = 30
	}
	return
}
