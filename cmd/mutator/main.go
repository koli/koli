package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	sysruntime "runtime"

	"github.com/golang/glog"
	"github.com/gorilla/mux"
	"github.com/spf13/pflag"
	"github.com/urfave/negroni"
	rbac "k8s.io/api/rbac/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"

	platform "kolihub.io/koli/pkg/apis/core/v1alpha1"
	"kolihub.io/koli/pkg/mutator"
	"kolihub.io/koli/pkg/request"
	"kolihub.io/koli/pkg/version"
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
	pflag.StringVar(&cfg.PlatformPubKeyFile, "platform-pub-key", "", "path to jwt public key file for validating tokens.")
	pflag.StringVar(&cfg.TLSServerConfig.CertFile, "cert-file", "", "path to public TLS certificate file")
	pflag.StringVar(&cfg.TLSServerConfig.KeyFile, "key-file", "", "path to private TLS certificate file")

	pflag.StringVar(&cfg.TLSClientConfig.CAFile, "ca-file", "", "path to TLS CA file")
	pflag.StringVar(&cfg.TLSClientConfig.KeyFile, "client-key", "", "path to private TLS client certificate file")
	pflag.StringVar(&cfg.TLSClientConfig.CertFile, "client-cert", "", "path to public TLS client certificate file")

	flag.StringVar(&cfg.KongAPIHost, "kong-api-host", "http://kong-admin:8000", "the address of Kong admin api")

	pflag.BoolVar(&showVersion, "version", false, "print version information and quit")
	pflag.BoolVar(&cfg.TLSInsecure, "tls-insecure", false, "don't verify API server's CA certificate")
	pflag.Parse()
	// Convinces goflags that we have called Parse() to avoid noisy logs.
	// OSS Issue: kubernetes/kubernetes#17162.
	flag.CommandLine.Parse([]string{})
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
			glog.Fatalf("error creating client configuration [%v]", err)
		}
	} else {
		config = &rest.Config{
			Host:            cfg.Host,
			TLSClientConfig: cfg.TLSClientConfig,
		}
		config.Insecure = cfg.TLSInsecure
	}

	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		glog.Fatalf("failed retrieving k8s client [%v]", err)
	}

	cfg.PlatformPubKey, err = ioutil.ReadFile(cfg.PlatformPubKeyFile)
	if err != nil {
		glog.Fatalf("failed reading pub key [%v]", err)
	}

	var tprConfig *rest.Config
	tprConfig = config
	tprConfig.APIPath = "/apis"
	tprConfig.GroupVersion = &platform.SchemeGroupVersion
	tprConfig.ContentType = runtime.ContentTypeJSON
	tprConfig.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: scheme.Codecs}
	metav1.AddToGroupVersion(scheme.Scheme, platform.SchemeGroupVersion)
	platform.SchemeBuilder.AddToScheme(scheme.Scheme)

	tprClient, err := rest.RESTClientFor(tprConfig)
	if err != nil {
		glog.Fatalf("failed retrieving tprclient from config [%v]", err)
	}

	kongAdminURL, err := url.Parse(cfg.KongAPIHost)
	if err != nil {
		glog.Fatalf("failed parsing kong admin api address [%v]", err)
	}
	if err := enforceClusterRole(kubeClient, &mutator.DefaultClusterRole); err != nil {
		glog.Fatalf("failed enforcing cluster role [%v]", err)
	}

	handler := mutator.NewHandler(kubeClient, tprClient, request.NewRequest(nil, kongAdminURL), &cfg)
	nonNamespaced := mux.NewRouter()

	// Namespaces mutators
	nn := nonNamespaced.PathPrefix("/api/v1/namespaces").Subrouter()
	nn.HandleFunc("", handler.NamespaceOnCreate).Methods("POST")
	nn.HandleFunc("", handler.NamespaceOnList).Methods("GET")
	nn.HandleFunc("/{name}", handler.NamespaceOnGet).Methods("GET")
	nn.HandleFunc("/{name}", handler.NamespaceOnMod).Methods("PUT", "PATCH")

	gv := platform.SchemeGroupVersion.String()
	namespaced := mux.NewRouter()
	// CRD domains (kong) mutator
	custom := namespaced.PathPrefix(fmt.Sprintf("/apis/%s/namespaces", gv)).Subrouter()
	custom.HandleFunc("/{namespace}/domains", handler.DomainsOnCreate).Methods("POST")
	custom.HandleFunc("/{namespace}/domains/{domain}", handler.DomainsOnMod).Methods("PUT", "PATCH", "DELETE")
	namespaced.HandleFunc(fmt.Sprintf("/apis/%s/domains/{fqdn}", gv), handler.DomainsOnHead).Methods("HEAD")

	// Deployment resources
	ext := namespaced.PathPrefix("/apis/extensions/v1beta1/namespaces/{namespace}").Subrouter()
	ext.HandleFunc("/deployments", handler.DeploymentsOnCreate).Methods("POST")
	ext.HandleFunc("/deployments/{deploy}", handler.DeploymentsOnMod).Methods("PUT", "PATCH")

	// Ingress resources
	ext.HandleFunc("/ingresses", handler.IngressOnCreate).Methods("POST")
	ext.HandleFunc("/ingresses/{name}", handler.IngressOnPatch).Methods("PUT", "PATCH")
	ext.HandleFunc("/ingresses/{name}", handler.IngressOnDelete).Methods("DELETE")

	nonNamespaced.PathPrefix("/").Handler(negroni.New(
		negroni.HandlerFunc(handler.Authorize),
		negroni.Wrap(namespaced),
	))

	listenAddr, isSecure := cfg.GetServeAddress()
	if isSecure {
		log.Fatal(http.ListenAndServeTLS(
			listenAddr,
			cfg.TLSServerConfig.CertFile,
			cfg.TLSServerConfig.KeyFile,
			nonNamespaced,
		))
	}
	log.Fatal(http.ListenAndServe(listenAddr, nonNamespaced))
}

func enforceClusterRole(clientset kubernetes.Interface, obj *rbac.ClusterRole) error {
	clusterRole, err := clientset.Rbac().ClusterRoles().Get(obj.Name, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	if clusterRole != nil {
		obj.ResourceVersion = clusterRole.ResourceVersion
	}
	_, err = clientset.RbacV1beta1().ClusterRoles().Update(obj)
	return err
}
