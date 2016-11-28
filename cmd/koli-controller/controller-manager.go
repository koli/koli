package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/golang/glog"
	"github.com/kolibox/koli/pkg/controller"
	"github.com/kolibox/koli/pkg/controller/informers"
	"github.com/spf13/pflag"

	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/api/unversioned"
	"k8s.io/client-go/1.5/pkg/runtime/serializer"
	"k8s.io/client-go/1.5/pkg/util/wait"
	"k8s.io/client-go/1.5/rest"
)

// Config defines configuration parameters for the Operator.
type Config struct {
	Host        string
	TLSInsecure bool
	TLSConfig   rest.TLSClientConfig
}

var cfg Config

func init() {
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.StringVar(&cfg.Host, "apiserver", "", "API Server addr, e.g. 'http://127.0.0.1:8080'. Omit parameter to run in on-cluster mode and utilize the service account token.")
	pflag.StringVar(&cfg.TLSConfig.CertFile, "cert-file", "", "Path to public TLS certificate file.")
	pflag.StringVar(&cfg.TLSConfig.KeyFile, "key-file", "", " Path to private TLS certificate file.")
	pflag.StringVar(&cfg.TLSConfig.CAFile, "ca-file", "", " Path to TLS CA file.")
	pflag.BoolVar(&cfg.TLSInsecure, "tls-insecure", false, " Don't verify API server's CA certificate.")
	pflag.Parse()
}

func newSysRESTClient(c *rest.Config) (*rest.RESTClient, error) {
	c.APIPath = "/apis"
	c.GroupVersion = &unversioned.GroupVersion{
		Group:   "sys.koli.io",
		Version: "v1alpha1",
	}
	c.NegotiatedSerializer = serializer.DirectCodecFactory{CodecFactory: api.Codecs}
	return rest.RESTClientFor(c)
}

func startControllers(stop <-chan struct{}) error {
	cfg, err := newClusterConfig(cfg.Host, cfg.TLSInsecure, &cfg.TLSConfig)
	if err != nil {
		return err
	}
	if os.Getenv("SUPER_USER_TOKEN") == "" {
		return fmt.Errorf("SUPER_USER_TOKEN env not defined")
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return err
	}

	_, err = client.Discovery().ServerVersion()
	if err != nil {
		return fmt.Errorf("communicating with server failed: %s", err)
	}

	// Provision third party resources
	controller.CreateTPRs(cfg.Host, client)

	sysclient, err := newSysRESTClient(cfg)
	if err != nil {
		return err
	}

	sharedInformers := informers.NewSharedInformerFactory(client, 30*time.Second)

	go controller.NewAddonController(
		sharedInformers.Addons().Informer(sysclient),
		sharedInformers.PetSets().Informer(),
		client,
	).Run(1, wait.NeverStop)
	// TODO: should we use the same client instance??
	go controller.NewNamespaceController(
		sharedInformers.Namespaces().Informer(),
		client,
	).Run(1, wait.NeverStop)
	sharedInformers.Start(stop)

	select {} // block forever
}

func newClusterConfig(host string, tlsInsecure bool, tlsConfig *rest.TLSClientConfig) (*rest.Config, error) {
	var cfg *rest.Config
	var err error

	if len(host) == 0 {
		if cfg, err = rest.InClusterConfig(); err != nil {
			return nil, err
		}
	} else {
		cfg = &rest.Config{
			Host: host,
		}
		hostURL, err := url.Parse(host)
		if err != nil {
			return nil, fmt.Errorf("error parsing host url %s : %v", host, err)
		}
		if hostURL.Scheme == "https" {
			cfg.TLSClientConfig = *tlsConfig
			cfg.Insecure = tlsInsecure
		}
	}
	// cfg.BearerToken = os.Getenv("SUPER_USER_TOKEN")
	cfg.QPS = 100
	cfg.Burst = 100

	return cfg, nil
}

func main() {
	err := startControllers(make(chan struct{}))
	glog.Fatalf("error running controllers: %v", err)
}
