package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/golang/glog"
	"github.com/spf13/pflag"

	"github.com/kolibox/koli/pkg/clientset"
	"github.com/kolibox/koli/pkg/controller"
	"github.com/kolibox/koli/pkg/controller/informers"
	_ "github.com/kolibox/koli/pkg/spec/install"

	"k8s.io/client-go/1.5/kubernetes"
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

func startControllers(stop <-chan struct{}) error {
	cfg, err := clientset.NewClusterConfig(cfg.Host, cfg.TLSInsecure, &cfg.TLSConfig)
	if err != nil {
		return err
	}
	// if os.Getenv("SUPER_USER_TOKEN") == "" {
	// 	return fmt.Errorf("SUPER_USER_TOKEN env not defined")
	// }
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return err
	}

	_, err = client.Discovery().ServerVersion()
	if err != nil {
		return fmt.Errorf("communicating with server failed: %s", err)
	}

	// Create required third party resources
	// controller.CreateAddonTPRs(cfg.Host, client)
	// controller.CreateServicePlan3PRs(cfg.Host, client)
	// controller.CreateServicePlanStatus3PRs(cfg.Host, client)

	sysClient, err := clientset.NewSysRESTClient(cfg)
	if err != nil {
		return err
	}

	sharedInformers := informers.NewSharedInformerFactory(client, 30*time.Second)

	// TODO: should we use the same client instance??
	// go controller.NewAddonController(
	// 	sharedInformers.Addons().Informer(sysClient),
	// 	sharedInformers.PetSets().Informer(),
	// 	client,
	// ).Run(1, wait.NeverStop)

	// go controller.NewNamespaceController(
	// 	sharedInformers.Namespaces().Informer(),
	// 	client,
	// ).Run(1, wait.NeverStop)

	go controller.NewServicePlanController(
		sharedInformers.ServicePlans().Informer(sysClient),
		client,
		sysClient,
	).Run(1, wait.NeverStop)

	sharedInformers.Start(stop)

	select {} // block forever
}

func main() {
	err := startControllers(make(chan struct{}))
	glog.Fatalf("error running controllers: %v", err)
}
