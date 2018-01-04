package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/boltdb/bolt"
	"github.com/golang/glog"
	"github.com/gorilla/mux"
	"github.com/spf13/pflag"
	"github.com/urfave/negroni"
	gitapi "kolihub.io/koli/pkg/git/api"
	"kolihub.io/koli/pkg/git/conf"
	gitutil "kolihub.io/koli/pkg/git/util"
	"kolihub.io/koli/pkg/util/monitoring"
	"kolihub.io/koli/pkg/version"
)

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
}

// Version refers to the version of the binary
type Version struct {
	git       string
	main      string
	buildDatr string
}

var cfg conf.Config
var showVersion bool

func init() {
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.StringVar(&cfg.Host, "apiserver", "", "api server addr, e.g. 'http://127.0.0.1:8080'. Omit parameter to run in on-cluster mode and utilize the service account token.")
	pflag.StringVar(&cfg.TLSConfig.CertFile, "cert-file", "", "path to public TLS certificate file.")
	pflag.StringVar(&cfg.TLSConfig.KeyFile, "key-file", "", "path to private TLS certificate file.")
	pflag.StringVar(&cfg.TLSConfig.CAFile, "ca-file", "", "path to TLS CA file.")
	pflag.StringVar(&cfg.Auth0.AdminClientID, "auth0-id", "", "auth0 Client ID for non interactive client.")
	pflag.StringVar(&cfg.Auth0.AdminClientSecret, "auth0-secret", "", "auth0 Client Secret for non interactive client.")
	pflag.StringVar(&cfg.Auth0.AdminAudienceURL, "auth0-audience", "", "auth0 API Audience URL.")
	pflag.StringVar(&cfg.Auth0.PlatformClientSecret, "platform-secret", "", "platform jwt secret for validating tokens.")
	pflag.StringVar(&cfg.Auth0.PlatformPubKeyFile, "platform-pub-key", "", "path to jwt public key file for validating tokens.")
	pflag.StringVar(&cfg.GitHubHookSecret, "github-hook-secret", "notimplementedyet", "hook secret for validating webhooks from github.")
	pflag.StringVar(&cfg.GitAPIHostname, "gitapi-host", "", "git api host routable DNS name.")
	pflag.StringVar(&cfg.GitHome, "git-home", "/home/git", "git releases path.")
	pflag.StringVar(&cfg.HealthzBindAddress, "healthz-bind-address", "0.0.0.0", "The IP address for the healthz server to serve on. (set to 0.0.0.0 for all interfaces)")
	pflag.Int32Var(&cfg.HealthzPort, "healthz-port", 20251, "The port of the localhost healthz endpoint (set to 0 to disable)")

	pflag.BoolVar(&showVersion, "version", false, "print version information and quit.")
	pflag.BoolVar(&cfg.TLSInsecure, "tls-insecure", false, "don't verify API server's CA certificate.")
	pflag.Parse()
	// Convinces goflags that we have called Parse() to avoid noisy logs.
	// OSS Issue: kubernetes/kubernetes#17162.
	flag.CommandLine.Parse([]string{})
}

func main() {
	v := version.Get()
	if showVersion {
		b, err := json.Marshal(&v)
		if err != nil {
			log.Fatalf("failed decoding version [%v]", err)
		}
		fmt.Println(string(b))
		return
	}
	glog.Infof("Version: %s, GitCommit: %s, GoVersion: %s, BuildDate: %s", v.GitVersion, v.GitCommit, v.GoVersion, v.BuildDate)
	if err := cfg.ReadPubKey(); err != nil {
		log.Fatalf("failed reading public key [%v]", err)
	}
	kubeClient, err := gitutil.GetKubernetesClient(cfg.Host)
	if err != nil {
		log.Fatalf("failed retrieving kubernetes clientset [%v]", err)
	}

	dbPath := filepath.Join(cfg.GitHome, "bolt")
	os.MkdirAll(dbPath, 0750)
	db, err := bolt.Open(filepath.Join(dbPath, "releases.db"), 0600, &bolt.Options{Timeout: time.Second * 10})
	if err != nil {
		log.Fatalf(err.Error())
	}
	gitHandler := gitapi.NewHandler(&cfg, kubeClient, db)
	r := mux.NewRouter().PathPrefix("/").Subrouter().StrictSlash(true)
	// v1beta1 releases
	// Metadata API
	r.HandleFunc(
		"/releases/v1beta1/{namespace}/{deployName}/objects/{gitSha}",
		gitHandler.V1beta1Releases,
	).Methods("GET", "PUT")
	r.HandleFunc(
		"/releases/v1beta1/{namespace}/{deployName}/objects",
		gitHandler.V1beta1ListReleases,
	).Methods("GET", "POST")
	r.HandleFunc(
		"/releases/v1beta1/{namespace}/{deployName}/seek",
		gitHandler.V1beta1SeekReleases,
	).Methods("GET")
	// Download/Upload files
	r.HandleFunc(
		"/releases/v1beta1/{namespace}/{deployName}/objects/{gitSha}/{file}",
		gitHandler.V1beta1DownloadFile,
	).Methods("GET")
	r.HandleFunc(
		"/releases/v1beta1/{namespace}/{deployName}/objects/{gitSha}",
		gitHandler.V1beta1UploadRelease,
	).Methods("POST")

	// r.HandleFunc("/releases/{namespace}/{deployName}/{gitSha}", gitHandler.Releases).Methods("POST")
	// r.HandleFunc("/releases/{namespace}/{deployName}/{gitSha}/{file}", gitHandler.Releases).Methods("GET")
	r.HandleFunc("/github/orgs/{org}/repos", gitHandler.GitHubListOrgRepos).Methods("GET")
	r.HandleFunc("/github/user/repos", gitHandler.GitHubListUserRepos).Methods("GET")
	r.HandleFunc("/github/user/orgs", gitHandler.GitHubListUserOrgs).Methods("GET")
	r.HandleFunc("/github/search/repos", gitHandler.GitHubSearchRepos).Methods("GET")
	r.HandleFunc("/github/repos/{owner}/{repo}/hooks", gitHandler.GitHubAddHooks).Methods("POST")
	r.HandleFunc("/github/repos/{owner}/{repo}/hooks", gitHandler.GitHubHooks).Methods("DELETE")
	r.HandleFunc("/github/repos/{owner}/{repo}/hooks/{id}", gitHandler.GitHubHooks).Methods("GET")
	r.HandleFunc("/github/repos/{owner}/{repo}/branches", gitHandler.GitHubListBranches).Methods("GET")

	webhook := mux.NewRouter()
	webhook.HandleFunc("/hooks", gitHandler.Webhooks).Methods("GET", "POST")

	webhook.PathPrefix("/").Handler(negroni.New(
		negroni.HandlerFunc(gitHandler.Authenticate),
		negroni.Wrap(r),
	))
	monitoring.ListenAndServeAll(cfg.HealthzBindAddress, cfg.HealthzPort)
	log.Fatal(http.ListenAndServe(":8001", webhook))
}
