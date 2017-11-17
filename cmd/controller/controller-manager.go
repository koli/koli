package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/golang/glog"
	"github.com/spf13/pflag"

	_ "kolihub.io/koli/pkg/apis/core/v1alpha1/install"
	"kolihub.io/koli/pkg/clientset"
	"kolihub.io/koli/pkg/controller"
	"kolihub.io/koli/pkg/controller/informers"
	_ "kolihub.io/koli/pkg/controller/install"
	koliversion "kolihub.io/koli/pkg/version"

	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes"
)

var cfg controller.Config
var showVersion bool

// Version refers to the version of the binary
type Version struct {
	git       string
	main      string
	buildDatr string
}

func init() {
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.StringVar(&cfg.Host, "apiserver", "", "api server addr, e.g. 'http://127.0.0.1:8080'. Omit parameter to run in on-cluster mode and utilize the service account token.")
	pflag.StringVar(&cfg.TLSConfig.CertFile, "cert-file", "", "path to public TLS certificate file.")
	pflag.StringVar(&cfg.TLSConfig.KeyFile, "key-file", "", "path to private TLS certificate file.")
	pflag.StringVar(&cfg.TLSConfig.CAFile, "ca-file", "", "path to TLS CA file.")
	pflag.StringVar(&cfg.GitReleaseHost, "git-release-host", "http://git-release-server.koli-system", "the address where releases are stored")
	pflag.StringVar(&cfg.ClusterName, "cluster-name", "gaia", "the name of the cluster")
	pflag.StringVar(&cfg.PlatformJWTSecret, "platform-secret", "", "the jwt secret for creating dynamic system tokens")
	pflag.StringVar(&cfg.DefaultDomain, "default-domain", "", "if set it will create default routes (services/ingresses) for each new deployment")
	pflag.StringVar(&cfg.SlugBuildImage, "slugbuilder-image", "quay.io/koli/slugbuilder", "the name of the builder image")
	pflag.StringVar(&cfg.SlugRunnerImage, "slugrunner-image", "quay.io/koli/slugrunner", "the name of the runner image")
	pflag.BoolVar(&cfg.DebugBuild, "debug-build", false, "debug the build container")

	pflag.BoolVar(&showVersion, "version", false, "print version information and quit")
	pflag.BoolVar(&cfg.TLSInsecure, "tls-insecure", false, "don't verify API server's CA certificate.")
	pflag.Parse()
	// Convinces goflags that we have called Parse() to avoid noisy logs.
	// OSS Issue: kubernetes/kubernetes#17162.
	flag.CommandLine.Parse([]string{})
}

func printSystemVersion(kubeVersion *version.Info) {
	glog.Infof("Kubernetes -> Version: %s, GitCommit: %s, GoVersion: %s, BuildDate: %s",
		kubeVersion.GitVersion, kubeVersion.GitCommit, kubeVersion.GoVersion, kubeVersion.BuildDate)
	v := koliversion.Get()
	glog.Infof("Koli -> Version: %s, GitCommit: %s, GoVersion: %s, BuildDate: %s",
		v.GitVersion, v.GitCommit, v.GoVersion, v.BuildDate)
}

func startControllers() error {
	if len(cfg.PlatformJWTSecret) == 0 {
		return fmt.Errorf("platform secret is empty")
	}
	kcfg, err := clientset.NewClusterConfig(cfg.Host, cfg.TLSInsecure, &cfg.TLSConfig)
	if err != nil {
		return err
	}
	client, err := kubernetes.NewForConfig(kcfg)
	if err != nil {
		return err
	}

	kubeServerVersion, err := client.Discovery().ServerVersion()
	if err != nil {
		return fmt.Errorf("communicating with server failed: %s", err)
	}
	printSystemVersion(kubeServerVersion)

	// controller.CreatePlatformRoles(client)
	// Create required third party resources
	// controller.CreateAddonTPRs(cfg.Host, client)
	// controller.CreatePlan3PRs(cfg.Host, client)
	// controller.CreateReleaseTPRs(cfg.Host, client)
	sysClient, err := clientset.NewSysRESTClient(kcfg)
	if err != nil {
		return err
	}
	if err := controller.CreateCRD(apiextensionsclient.NewForConfigOrDie(kcfg)); err != nil {
		return err
	}

	sharedInformers := informers.NewSharedInformerFactory(client, 30*time.Second)
	stopC := wait.NeverStop
	// TODO: should we use the same client instance??
	go controller.NewNamespaceController(
		sharedInformers.Namespaces().Informer(),
		sharedInformers.ServicePlans().Informer(sysClient),
		client,
		sysClient,
	).Run(1, stopC)

	go controller.NewAppManagerController(
		sharedInformers.Deployments().Informer(),
		sharedInformers.ServicePlans().Informer(sysClient),
		client,
		cfg.DefaultDomain,
	).Run(1, stopC)

	go controller.NewReleaseController(
		sharedInformers.Releases().Informer(sysClient),
		sharedInformers.Deployments().Informer(),
		sysClient,
		client,
	).Run(1, stopC)

	go controller.NewBuildController(
		&cfg,
		sharedInformers.Releases().Informer(sysClient),
		sysClient,
		client,
	).Run(1, stopC)

	// TODO: hard-coded
	selector := labels.Set{"kolihub.io/type": "slugbuild"}
	go controller.NewDeployerController(
		&cfg,
		sharedInformers.Pods().Informer(selector.AsSelector()),
		sharedInformers.Deployments().Informer(),
		sharedInformers.Releases().Informer(sysClient),
		sysClient,
		client,
	).Run(1, stopC)

	go controller.NewSecretController(
		sharedInformers.Namespaces().Informer(),
		sharedInformers.Secrets().Informer(),
		client,
		cfg.PlatformJWTSecret,
	).Run(1, stopC)

	sharedInformers.Start(stopC)
	select {} // block forever
}

func main() {
	if showVersion {
		version := koliversion.Get()
		b, err := json.Marshal(&version)
		if err != nil {
			fmt.Printf("failed decoding version: %s\n", err)
			os.Exit(1)
		}
		fmt.Println(string(b))
	} else {
		err := startControllers()
		glog.Fatalf("error running controllers: %v", err)
		panic("unreachable")
	}
}
