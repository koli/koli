package api

import (
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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"

	"kolihub.io/koli/pkg/apis/authentication"
	platform "kolihub.io/koli/pkg/apis/v1alpha1"
	"kolihub.io/koli/pkg/apis/v1alpha1/draft"
	"kolihub.io/koli/pkg/clientset/auth0"
	auth0clientset "kolihub.io/koli/pkg/clientset/auth0"
	gitutil "kolihub.io/koli/pkg/git/util"
	"kolihub.io/koli/pkg/util"
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
	gitclient, err := h.gitHubCli(h.GetUserIDSub())
	if err != nil {
		githubClientError(w, err)
		return
	}
	page, perPage := parsePages(r.URL.Query())
	gitOptions := &github.SearchOptions{
		ListOptions: github.ListOptions{PerPage: perPage, Page: page},
	}
	repos, resp, err := gitclient.Search.Repositories(context.Background(), qs.Get("q"), gitOptions)
	if err != nil {
		msg := fmt.Sprintf("failed searching repos, %v", err)
		util.WriteResponseError(w, util.StatusBadRequest(msg, nil, metav1.StatusReasonUnknown))
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
	githubcli, err := h.gitHubCli(h.GetUserIDSub())
	if err != nil {
		githubClientError(w, err)
		return
	}
	qs := r.URL.Query()

	page, perPage := parsePages(qs)
	gitOptions := &github.RepositoryListByOrgOptions{
		ListOptions: github.ListOptions{PerPage: perPage, Page: page},
		Type:        qs.Get("type"),
	}

	repos, resp, err := githubcli.Repositories.ListByOrg(context.Background(), params["org"], gitOptions)
	if err != nil {
		msg := fmt.Sprintf("failed listing organization repos, %v", err)
		util.WriteResponseError(w, util.StatusBadRequest(msg, nil, metav1.StatusReasonUnknown))
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
	githubcli, err := h.gitHubCli(h.GetUserIDSub())
	if err != nil {
		githubClientError(w, err)
		return
	}

	qs := r.URL.Query()
	page, perPage := parsePages(qs)
	gitOptions := &github.RepositoryListOptions{
		ListOptions: github.ListOptions{PerPage: perPage, Page: page},
		Type:        qs.Get("type"),
	}

	repos, resp, err := githubcli.Repositories.List(context.Background(), "", gitOptions)
	if err != nil {
		msg := fmt.Sprintf("failed listing user repos, %v", err)
		util.WriteResponseError(w, util.StatusBadRequest(msg, nil, metav1.StatusReasonUnknown))
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
	githubcli, err := h.gitHubCli(h.GetUserIDSub())
	if err != nil {
		githubClientError(w, err)
		return
	}
	qs := r.URL.Query()

	page, perPage := parsePages(qs)
	gitOptions := &github.ListOptions{PerPage: perPage, Page: page}
	branches, resp, err := githubcli.Repositories.ListBranches(context.Background(), params["owner"], params["repo"], gitOptions)
	if err != nil {
		msg := fmt.Sprintf("failed listing branches, %v", err)
		util.WriteResponseError(w, util.StatusBadRequest(msg, nil, metav1.StatusReasonUnknown))
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

func (h *Handler) gitHubAccessToken(userIDSub string) (*authentication.Identity, error) {
	t := &authentication.Token{
		"client_id":     h.cnf.AdminClientID,
		"client_secret": h.cnf.AdminClientSecret,
		"audience":      h.cnf.AdminAudienceURL,
		"grant_type":    "client_credentials",
	}
	auth0client, err := h.auth0Client()
	if err != nil {
		return nil, fmt.Errorf("failed retrieving auth0 client: %v", err)
	}

	responseToken, err := auth0client.Authentication().ClientCredentials(t)
	if err != nil {
		return nil, fmt.Errorf("failed retrieving admin token: %s", err)
	}
	accessToken := responseToken.AccessToken()

	if len(accessToken) == 0 {
		return nil, fmt.Errorf("failed retrieving 'access_token' from response: %#v", responseToken)
	}
	auth0User, err := auth0client.Management(accessToken).Users().Get(url.QueryEscape(userIDSub))
	if err != nil {
		return nil, fmt.Errorf("failed retrieving auth0 user info: %v", err)
	}
	var identity *authentication.Identity
	for _, obj := range auth0User.Identities {
		if obj.Provider != "github" {
			continue
		}
		identity = &obj
	}
	if identity == nil {
		return nil, fmt.Errorf("failed finding the 'github' identity")
	}
	return identity, nil
}

// GitHubAddHooks create new webhooks into github
// https://developer.github.com/v3/repos/hooks/#create-a-hook
func (h *Handler) GitHubAddHooks(w http.ResponseWriter, r *http.Request) {
	p := mux.Vars(r)
	defer r.Body.Close()
	var payload map[string]string
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		msg := fmt.Sprintf("failed decoding body from request, %v", err)
		util.WriteResponseError(w, util.StatusBadRequest(msg, nil, metav1.StatusReasonBadRequest))
		return
	}

	namespace := payload["namespace"]
	deploy := payload["deploy"]

	gcli, err := h.gitHubCli(h.GetUserIDSub())
	if err != nil {
		githubClientError(w, err)
		return
	}

	hooks, err := h.listRepoHooks(gcli, p["owner"], p["repo"])
	if err != nil {
		util.WriteResponseError(w, util.StatusBadRequest(err.Error(), nil, metav1.StatusReasonBadRequest))
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

	// there isn't a hook configured at github. Create a new one!
	if hook == nil {
		hookName := "web"
		// https://developer.github.com/v3/repos/hooks/#create-a-hook
		hookRequest := &github.Hook{
			Name: &hookName,
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
			msg := fmt.Sprintf("failed creating hook, %v", err)
			util.WriteResponseError(w, util.StatusBadRequest(msg, nil, metav1.StatusReasonBadRequest))
			return
		}
		glog.V(4).Infof("%s/%s - hook created with success for %s/%s", namespace, deploy, p["owner"], p["repo"])
		resp.Body.Close()
	}

	dp := draft.NewDeployment(&v1beta1.Deployment{})
	dp.SetAnnotation(platform.AnnotationGitHubUser, h.GetUserIDSub())
	dp.SetAnnotation("kolihub.io/hookid", strconv.Itoa(hook.GetID()))
	dp.SetLabel("kolihub.io/gitowner", p["owner"])
	dp.SetLabel("kolihub.io/gitrepo", p["repo"])

	patchData, err := util.StrategicMergePatch(scheme.Codecs.LegacyCodec(v1beta1.SchemeGroupVersion), &v1beta1.Deployment{}, dp.GetObject())
	if err != nil {
		msg := fmt.Sprintf("failed generating deployment patch diff, %v", err)
		util.WriteResponseError(w, util.StatusInternalError(msg, nil))
		return
	}

	_, err = h.clientset.Extensions().Deployments(namespace).Patch(deploy, types.StrategicMergePatchType, patchData)
	if err != nil {
		msg := fmt.Sprintf("failed updating deployment, %v", err)
		util.WriteResponseError(w, util.StatusBadRequest(msg, nil, metav1.StatusReasonUnknown))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(hook)
}

func (h *Handler) listRepoHooks(client *github.Client, owner, repo string) ([]*github.Hook, error) {
	hooks, resp, err := client.Repositories.ListHooks(context.Background(), owner, repo, nil)
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

// GitHubHooks allows read and delete github webhooks
// https://developer.github.com/v3/repos/hooks/#webhooks
func (h *Handler) GitHubHooks(w http.ResponseWriter, r *http.Request) {
	p := mux.Vars(r)
	switch r.Method {
	case "GET":
		hookID, err := strconv.Atoi(p["id"])
		if err != nil {
			msg := fmt.Sprintf("id in wrong format, %v", err)
			util.WriteResponseError(w, util.StatusBadRequest(msg, nil, metav1.StatusReasonBadRequest))
			return
		}
		gcli, err := h.gitHubCli(h.GetUserIDSub())
		if err != nil {
			githubClientError(w, err)
			return
		}
		hook, resp, err := gcli.Repositories.GetHook(oauth2.NoContext, p["owner"], p["repo"], hookID)
		if err != nil {
			statusCode := http.StatusBadRequest
			if resp != nil {
				statusCode = resp.StatusCode
			}
			util.WriteResponseError(w, &metav1.Status{
				Code:    int32(statusCode),
				Status:  metav1.StatusFailure,
				Message: fmt.Sprintf("failed retrieving hook, %v", err),
				Reason:  metav1.StatusReasonUnknown,
			})
			return
		}
		json.NewEncoder(w).Encode(hook)
	case "DELETE": // Removes deployment associations and hooks when possible
		var data map[string]string
		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			msg := fmt.Sprintf("failed decoding body from request, %v", err)
			util.WriteResponseError(w, util.StatusInternalError(msg, nil))
			return
		}
		defer r.Body.Close()
		namespace, deploy := data["namespace"], data["deploy"]
		if len(namespace) == 0 || len(deploy) == 0 {
			util.WriteResponseError(w, util.StatusBadRequest("missing 'namespace' or 'deploy' data", nil, metav1.StatusReasonBadRequest))
			return
		}

		l := labels.Set{
			"kolihub.io/gitowner": p["owner"],
			"kolihub.io/gitrepo":  p["repo"],
		}
		opts := metav1.ListOptions{LabelSelector: l.AsSelector().String()}
		// Search for all deployment references to the github hook.
		// It's important to list all namespaces, to known when to delete the hook
		// in github, otherwise it will not be possible
		dpList, err := h.clientset.Extensions().Deployments(metav1.NamespaceAll).List(opts)
		if err != nil {
			msg := fmt.Sprintf("%s/%s - failed listing deploys, %v", namespace, deploy, err)
			util.WriteResponseError(w, util.StatusBadRequest(msg, nil, metav1.StatusReasonBadRequest))
			return
		}

		gcli, err := h.gitHubCli(h.GetUserIDSub())
		if err != nil {
			githubClientError(w, err)
			return
		}

		// TODO: prevent crossing customer/org boundries,
		// an user could delete his own resources only!
		for _, dp := range dpList.Items {
			if dp.Name != deploy && dp.Namespace != namespace {
				continue
			}
			d := draft.NewDeployment(&dp)
			// if only one deployment is found, means the github hook could be deleted
			// because there is only one reference to it.
			if len(dpList.Items) == 1 {
				hookID, _ := strconv.Atoi(d.Annotations["kolihub.io/hookid"])
				// the hook ID exists, delete by ID
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
						util.WriteResponseError(w, &metav1.Status{
							Code:    int32(statusCode),
							Status:  metav1.StatusFailure,
							Message: fmt.Sprintf("failed removing hook, %v", err),
							Reason:  metav1.StatusReasonBadRequest,
						})
						return
					}
					// The hook ID doesn't exist, try to search all hooks
					// by its owner/repo
				} else {
					hooks, err := h.listRepoHooks(gcli, p["owner"], p["repo"])
					if err != nil {
						msg := fmt.Sprintf("failed listing hooks, %v", err)
						util.WriteResponseError(w, util.StatusBadRequest(msg, nil, metav1.StatusReasonBadRequest))
						return
					}
					// wipe all platform hooks from github
					for _, hook := range hooks {
						// only remove platform hooks created by the key
						// 'kolihub.io/ref'
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
								util.WriteResponseError(w, &metav1.Status{
									Code:    int32(statusCode),
									Status:  metav1.StatusFailure,
									Message: fmt.Sprintf("failed removing hook, %v", err),
									Reason:  metav1.StatusReasonBadRequest,
								})
								return
							}
						}
					}
				}
			}
			original, err := d.DeepCopy() // performs a copy of the object to create the patch diff
			if err != nil {
				msg := fmt.Sprintf("failed performing deep copy, %v", err)
				util.WriteResponseError(w, util.StatusInternalError(msg, nil))
				return
			}
			// Remove the association with the hook removing labels and annotations
			delete(d.Labels, "kolihub.io/gitowner")
			delete(d.Labels, "kolihub.io/gitrepo")

			delete(d.Annotations, platform.AnnotationGitHubUser)
			delete(d.Annotations, "kolihub.io/hookid")
			delete(d.Annotations, platform.AnnotationAuthToken)
			codec := scheme.Codecs.LegacyCodec(v1beta1.SchemeGroupVersion)
			patchData, err := util.StrategicMergePatch(codec, original.GetObject(), d.GetObject())
			if err != nil {
				msg := fmt.Sprintf("failed generating deployment patch diff, %v", err)
				util.WriteResponseError(w, util.StatusInternalError(msg, nil))
				return
			}
			_, err = h.clientset.Extensions().Deployments(namespace).Patch(d.Name, types.StrategicMergePatchType, patchData)
			if err != nil {
				msg := fmt.Sprintf("%s/%s - failed updating deploy, %v", d.Namespace, d.Name, err)
				util.WriteResponseError(w, util.StatusBadRequest(msg, nil, metav1.StatusReasonBadRequest))
				return
			}
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
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
				glog.Warningf("%s - namespace in wrong format", target)
				continue
			}

			if dp.GitBranch() != event.GetRef() {
				glog.Infof("%s - branch is not related: %s\n", target, dp.GitBranch())
				continue
			}

			// Fetch the identity only if the repository is private
			var identity *authentication.Identity
			if event.Repo.GetPrivate() {
				gitUser := dp.GitHubUser()
				if !gitUser.Exists() {
					glog.Warningf("%s - missing git user connection", target)
					continue
				}
				var err error
				identity, err = h.gitHubAccessToken(gitUser.String())
				if err != nil {
					glog.Warningf("%s - failed retrieving github access token (%s): %s", target, err)
					continue
				}
			}

			jwtSystemToken, err := gitutil.GenerateNewJwtToken(h.cnf.PlatformClientSecret, nsMeta.Customer(), nsMeta.Organization(), platform.SystemTokenType)
			if err != nil {
				glog.Infof("%s - failed generating user token: %s", target, err)
				continue
			}

			original, err := dp.DeepCopy()
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, "failed performing deep copy: %v", err)
				return
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
			dp.Annotations[platform.AnnotationAuthToken] = jwtSystemToken
			if identity != nil {
				cloneURL, err := getCloneURL(event.Repo.GetCloneURL(), identity.AccessToken, event.Repo.GetFullName())
				if err != nil {
					glog.Infof("%s - failed getting clone url: %s", target, err)
					continue
				}
				dp.Annotations[platform.AnnotationGitRemote] = cloneURL
			}
			dp.Annotations[platform.AnnotationGitCompare] = event.GetCompare()
			dp.Annotations[platform.AnnotationBuildSource] = githubBuildSourceName

			codec := scheme.Codecs.LegacyCodec(v1beta1.SchemeGroupVersion)
			patchData, err := util.StrategicMergePatch(codec, original.GetObject(), dp.GetObject())
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, "failed generating deployment patch diff: %v", err)
				return
			}
			_, err = h.clientset.Extensions().Deployments(dp.Namespace).Patch(dp.Name, types.StrategicMergePatchType, patchData)
			if err != nil {
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

func githubClientError(w http.ResponseWriter, err error) {
	msg := fmt.Sprintf("failed getting github client, %v", err)
	util.WriteResponseError(w, util.StatusBadRequest(msg, nil, metav1.StatusReasonUnknown))
}

func (h *Handler) gitHubCli(userIDSub string) (*github.Client, error) {
	if h.gitClient != nil {
		return h.gitClient, nil
	}

	id, err := h.gitHubAccessToken(userIDSub)
	if err != nil {
		return nil, fmt.Errorf("failed retrieving github access token: %v", err)
	}
	glog.V(4).Infof("GOT a token, connection %#v, IsSocial %v, Provider %#v UserID %#v ", id.Connection, id.IsSocial, id.Provider, id.UserID)
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: id.AccessToken})
	return github.NewClient(oauth2.NewClient(oauth2.NoContext, ts)), nil
}

func (h *Handler) auth0Client() (auth0.CoreInterface, error) {
	restConfig := &auth0clientset.Config{Host: auth0AuthURL, Client: &http.Client{Transport: http.DefaultTransport}}
	if h.auth0RestConfig != nil {
		restConfig = h.auth0RestConfig
	}
	auth0client, err := auth0clientset.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed retrieving auth0 client: %v", err)
	}
	return auth0client, nil
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
