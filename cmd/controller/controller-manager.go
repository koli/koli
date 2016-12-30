package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/golang/glog"
	"github.com/spf13/pflag"

	"kolihub.io/koli/pkg/clientset"
	"kolihub.io/koli/pkg/controller"
	"kolihub.io/koli/pkg/controller/informers"
	_ "kolihub.io/koli/pkg/controller/install"
	_ "kolihub.io/koli/pkg/spec/install"
	"kolihub.io/koli/pkg/version"

	"encoding/json"

	kclientset "k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset"
	"k8s.io/kubernetes/pkg/client/restclient"
	"k8s.io/kubernetes/pkg/util/wait"
)

// Config defines configuration parameters for the Operator.
type Config struct {
	Host        string
	TLSInsecure bool
	TLSConfig   restclient.TLSClientConfig
}

var cfg Config
var showVersion bool

type Version struct {
	git       string
	main      string
	buildDatr string
}

func init() {
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.StringVar(&cfg.Host, "apiserver", "", "API Server addr, e.g. 'http://127.0.0.1:8080'. Omit parameter to run in on-cluster mode and utilize the service account token.")
	pflag.StringVar(&cfg.TLSConfig.CertFile, "cert-file", "", "Path to public TLS certificate file.")
	pflag.StringVar(&cfg.TLSConfig.KeyFile, "key-file", "", " Path to private TLS certificate file.")
	pflag.StringVar(&cfg.TLSConfig.CAFile, "ca-file", "", " Path to TLS CA file.")
	pflag.BoolVar(&showVersion, "version", true, "Print version information and quit")
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
	client, err := kclientset.NewForConfig(cfg)
	if err != nil {
		return err
	}

	_, err = client.Discovery().ServerVersion()
	if err != nil {
		return fmt.Errorf("communicating with server failed: %s", err)
	}

	controller.CreatePlatformRoles(client)
	// Create required third party resources
	controller.CreateAddonTPRs(cfg.Host, client)
	controller.CreateServicePlan3PRs(cfg.Host, client)
	controller.CreateServicePlanStatus3PRs(cfg.Host, client)

	sysClient, err := clientset.NewSysRESTClient(cfg)
	if err != nil {
		return err
	}

	sharedInformers := informers.NewSharedInformerFactory(client, 30*time.Second)

	// TODO: should we use the same client instance??
	go controller.NewAddonController(
		sharedInformers.Addons().Informer(sysClient),
		sharedInformers.PetSets().Informer(),
		sharedInformers.ServicePlans().Informer(sysClient),
		client,
	).Run(1, wait.NeverStop)

	go controller.NewNamespaceController(
		sharedInformers.Namespaces().Informer(),
		sharedInformers.ServicePlans().Informer(sysClient),
		client,
		sysClient,
	).Run(1, wait.NeverStop)

	go controller.NewServicePlanController(
		sharedInformers.ServicePlans().Informer(sysClient),
		client,
		sysClient,
	).Run(1, wait.NeverStop)

	go controller.NewResourceAllocatorCtrl(
		sharedInformers.Deployments().Informer(),
		sharedInformers.ServicePlans().Informer(sysClient),
		client,
		sysClient,
	).Run(1, wait.NeverStop)

	sharedInformers.Start(stop)

	select {} // block forever
}

func main() {
	if showVersion {
		version := version.Get()
		b, err := json.Marshal(&version)
		if err != nil {
			fmt.Printf("failed decoding version: %s\n", err)
			os.Exit(1)
		}
		fmt.Println(string(b))
	} else {
		err := startControllers(make(chan struct{}))
		glog.Fatalf("error running controllers: %v", err)
	}
}
