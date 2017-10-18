package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"

	"github.com/golang/glog"
	"github.com/spf13/pflag"
	"github.com/urfave/negroni"
	"kolihub.io/koli/pkg/git/conf"
	gitserver "kolihub.io/koli/pkg/git/server"
	gitutil "kolihub.io/koli/pkg/git/util"
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
	// TODO: fallback to a default secret resource
	pflag.StringVar(&cfg.PlatformClientSecret, "platform-secret", "", "platform jwt secret for validating tokens.")
	pflag.StringVar(&cfg.GitHome, "git-home", "/home/git", "git server repositories path")
	pflag.StringVar(&cfg.GitAPIHostname, "gitapi-host", "http://git-api.koli-system", "address of the git api store server")
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
			fmt.Printf("failed decoding version: %s\n", err)
			os.Exit(1)
		}
		fmt.Println(string(b))
		return
	}
	glog.Infof("Version: %s, GitCommit: %s, GoVersion: %s, BuildDate: %s", v.GitVersion, v.GitCommit, v.GoVersion, v.BuildDate)

	kubeClient, err := gitutil.GetKubernetesClient(cfg.Host)
	if err != nil {
		fmt.Printf("failed getting clientset: %s\n", err)
		os.Exit(1)
	}

	gitHandler := gitserver.NewHandler(&cfg, kubeClient)
	n := negroni.New(negroni.HandlerFunc(gitHandler.Authenticate))
	n.UseHandlerFunc(gitHandler.ServeHTTP)
	log.Fatal(http.ListenAndServe(":8000", n))
}
