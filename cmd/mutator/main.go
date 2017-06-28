package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"path"
	sysruntime "runtime"

	"github.com/golang/glog"
	"github.com/gorilla/mux"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api"
	"k8s.io/client-go/rest"

	platform "kolihub.io/koli/pkg/apis/v1alpha1"
	"kolihub.io/koli/pkg/mutator"
	"kolihub.io/koli/pkg/version"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	sysruntime.GOMAXPROCS(sysruntime.NumCPU())
}

var cfg mutator.Config
var showVersion bool

func init() {
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.StringVar(&cfg.Host, "apiserver", "", "api server addr, e.g. 'http://127.0.0.1:8080'. Omit parameter to run in on-cluster mode and utilize the service account token")
	pflag.StringVar(&cfg.Serve, "serve", "", "the address to serve requests. Defaults to address ':8080' if '--cert-file' and '--key-file' is empty, otherwise ':8443'")
	pflag.StringVar(&cfg.AllowedImages, "images", "", "Comma separated list of permitted images to run in the cluster.")
	pflag.StringVar(&cfg.RegistryImages, "image-registry", "quay.io/koli", "Registry of allowed images")
	pflag.StringVar(&cfg.TLSServerConfig.CertFile, "cert-file", "", "path to public TLS certificate file")
	pflag.StringVar(&cfg.TLSServerConfig.KeyFile, "key-file", "", "path to private TLS certificate file")

	pflag.StringVar(&cfg.TLSClientConfig.CAFile, "ca-file", "", "path to TLS CA file")
	pflag.StringVar(&cfg.TLSClientConfig.KeyFile, "client-key", "", "path to private TLS client certificate file")
	pflag.StringVar(&cfg.TLSClientConfig.CertFile, "client-cert", "", "path to public TLS client certificate file")

	pflag.BoolVar(&showVersion, "version", false, "print version information and quit")
	pflag.BoolVar(&cfg.TLSInsecure, "tls-insecure", false, "don't verify API server's CA certificate")
	pflag.Parse()
}

func main() {
	version := version.Get()
	if showVersion {
		b, err := json.Marshal(&version)
		if err != nil {
			glog.Fatalf("failed decoding version: %s", err)
		}
		fmt.Println(string(b))
		return
	}
	glog.Infof("Version: %s, GitCommit: %s, GoVersion: %s, BuildDate: %s", version.GitVersion, version.GitCommit, version.GoVersion, version.BuildDate)

	var config *rest.Config
	var err error
	if len(cfg.Host) == 0 {
		config, err = rest.InClusterConfig()
		if err != nil {
			glog.Fatalf("error creating client configuration: %v", err)
		}
	} else {
		config = &rest.Config{Host: cfg.Host}
		config.TLSClientConfig = cfg.TLSClientConfig
		config.Insecure = cfg.TLSInsecure
	}

	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		glog.Fatalf("failed retrieving k8s client: %v", err)
	}

	var tprConfig *rest.Config
	tprConfig = config
	tprConfig.APIPath = "/apis"
	tprConfig.GroupVersion = &platform.SchemeGroupVersion
	tprConfig.ContentType = runtime.ContentTypeJSON
	tprConfig.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: api.Codecs}
	metav1.AddToGroupVersion(api.Scheme, platform.SchemeGroupVersion)
	platform.SchemeBuilder.AddToScheme(api.Scheme)

	tprClient, err := rest.RESTClientFor(tprConfig)
	if err != nil {
		glog.Fatalf("failed retrieving tprclient from config: %v", err)
	}

	r := mux.NewRouter()
	handler := mutator.NewHandler(kubeClient, tprClient, &cfg)

	// Namespaces mutators
	r.HandleFunc("/api/v1/namespaces", handler.NamespaceOnCreate).
		Methods("POST")
	r.HandleFunc("/api/v1/namespaces/{name}", handler.NamespaceOnMod).
		Methods("PUT", "PATCH")

	groupVersion := path.Join(platform.SchemeGroupVersion.Group, platform.SchemeGroupVersion.Version)

	// TPR domains (kong) mutator
	r.HandleFunc(fmt.Sprintf("/apis/%s/namespaces/{namespace}/domains", groupVersion), handler.DomainsOnCreate).
		Methods("POST")

	r.HandleFunc(fmt.Sprintf("/apis/%s/namespaces/{namespace}/domains/{domain}", groupVersion), handler.DomainsOnMod).
		Methods("PUT", "PATCH", "DELETE")

	// Deployment resources
	r.HandleFunc("/apis/extensions/v1beta1/namespaces/{namespace}/deployments", handler.DeploymentsOnCreate).
		Methods("POST")
	r.HandleFunc("/apis/extensions/v1beta1/namespaces/{namespace}/deployments/{deploy}", handler.DeploymentsOnMod).
		Methods("PUT", "PATCH")

	listenAddr, isSecure := cfg.GetServeAddress()
	if isSecure {
		log.Fatal(http.ListenAndServeTLS(listenAddr, cfg.TLSServerConfig.CertFile, cfg.TLSServerConfig.KeyFile, handler.Authorize(r)))
	}
	log.Fatal(http.ListenAndServe(listenAddr, handler.Authorize(r)))
}
