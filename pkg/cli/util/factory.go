package util

import (
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path"
	"runtime"
	"strings"

	"github.com/imdario/mergo"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/kubernetes/pkg/client/unversioned/clientcmd"
	cmdutil "k8s.io/kubernetes/pkg/kubectl/cmd/util"
	"k8s.io/kubernetes/pkg/kubectl/resource"
	kuberuntime "k8s.io/kubernetes/pkg/runtime"
)

// Factory provides abstractions that allow the Kubectl command to be extended across multiple types
// of resources and different API sets.
type Factory struct {
	KubeFactory   *cmdutil.Factory
	User          *UserMeta
	Ctrl          *Controller
	CrafterRemote string
	Serializer    kuberuntime.NegotiatedSerializer
	flags         *pflag.FlagSet
}

// BindFlags adds any flags that are common to all kubectl sub commands.
func (f *Factory) BindFlags(flags *pflag.FlagSet) {
	// Merge factory's flags
	flags.AddFlagSet(f.flags)

	// Globally persistent flags across all subcommands.
	// TODO Change flag names to consts to allow safer lookup from subcommands.
	// TODO Add a verbose flag that turns on glog logging. Probably need a way
	// to do that automatically for every subcommand.
	//flags.BoolVar(&f.clients.matchVersion, FlagMatchBinaryVersion, false, "Require server version to match client version")

	// Normalize all flags that are coming from other packages or pre-configurations
	// a.k.a. change all "_" to "-". e.g. glog package
	//flags.SetNormalizeFunc(utilflag.WordSepNormalizeFunc)
}

// BindExternalFlags adds any flags defined by external projects (not part of pflags)
func (f *Factory) BindExternalFlags(flags *pflag.FlagSet) {
	// any flags defined by external projects (not part of pflags)
	flags.AddGoFlagSet(flag.CommandLine)
}

// DefaultNamespace filter a default namespace based on the label '<koliutil.PrefixLabel>/default=true'.
// Returns an error if none or more than one namespace is found.
func (f *Factory) DefaultNamespace(cmd *cobra.Command, isNamespaced bool) (string, error) {
	if !isNamespaced {
		return "", nil
	}
	flag := cmd.Flag("namespace")
	if flag.Value.String() != "" {
		return flag.Value.String(), nil
	}
	// The client doesn't provide any namespace, need to find a default one
	mapper, typer := f.KubeFactory.Object(cmdutil.GetIncludeThirdPartyAPIs(cmd))
	selector := fmt.Sprintf("%s/id=%s,%s/default=true", PrefixLabel, f.User.ID, PrefixLabel)
	r := resource.NewBuilder(mapper, typer,
		resource.ClientMapperFunc(f.KubeFactory.ClientForMapping), f.KubeFactory.Decoder(true)).
		SelectorParam(selector).
		ResourceTypes([]string{"namespace"}...).
		Latest().
		Flatten().
		Do()

	infos, err := r.Infos()
	if err != nil {
		return "", err
	} else if len(infos) != 1 {
		return "", fmt.Errorf("Found (%d) namespaces.", len(infos))
	}
	return infos[0].Name, nil
}

// RepositoryExists verifies if a repository already exists on deployments
func (f *Factory) RepositoryExists(cmd *cobra.Command, repository, namespace string) (bool, error) {
	// The client doesn't provide any namespace, need to find a default one
	mapper, typer := f.KubeFactory.Object(cmdutil.GetIncludeThirdPartyAPIs(cmd))
	selector := fmt.Sprintf("%s/repo=%s", PrefixLabel, repository)
	r := resource.NewBuilder(mapper, typer,
		resource.ClientMapperFunc(f.KubeFactory.ClientForMapping), f.KubeFactory.Decoder(true)).
		SelectorParam(selector).
		NamespaceParam(namespace).
		ResourceTypes([]string{"deployment"}...).
		Latest().
		Flatten().
		Do()

	infos, err := r.Infos()
	if err != nil {
		return false, fmt.Errorf("could not verify existent repositories (%s)", err)
	}

	return len(infos) >= 1, nil
}

// NewFactory creates a factory with the default Kubernetes resources defined
func NewFactory(optionalClientConfig clientcmd.ClientConfig) (*Factory, error) {
	flags := pflag.NewFlagSet("", pflag.ContinueOnError)
	// flags.SetNormalizeFunc(utilflag.WarnWordSepNormalizeFunc) // Warn for "_" flags

	clientConfig := optionalClientConfig
	if optionalClientConfig == nil {
		clientConfig = DefaultClientConfig(flags)
	}
	kfactory := cmdutil.NewFactory(clientConfig)

	cfg, err := clientConfig.ClientConfig()
	if cfg.BearerToken == "" {
		return nil, errors.New("bearer token is empty")
	}
	if err != nil {
		return nil, err
	}

	host := strings.TrimPrefix(strings.TrimPrefix(cfg.Host, "https://"), "http://")
	_ = host
	url := &url.URL{
		Scheme: "http",
		// Host:   host,
		Host: "192.168.99.100:7080",
		Path: "/",
	}

	crafterRemote := "http://crafter-orion.kolibox.io:7080" // TODO: hard-coded
	controller := NewController(url, "")
	controller.Request.SetHeader("Authorization", fmt.Sprintf("Bearer %s", cfg.BearerToken))
	plataform := path.Join(runtime.GOOS, runtime.GOARCH)
	userAgent := "koli/v0.1.0 (%s) [kubectl/v1.4.0]"
	controller.Request.SetHeader("User-Agent", fmt.Sprintf(userAgent, plataform))

	return &Factory{
		KubeFactory:   kfactory,
		User:          UserConfig(nil),
		Ctrl:          controller,
		CrafterRemote: crafterRemote,
		Serializer:    cfg.NegotiatedSerializer,
		flags:         flags,
	}, nil
}

// DefaultClientConfig from pkg/kubectl/cmd/util/factory.go:DefaultClientConfig:
func DefaultClientConfig(flags *pflag.FlagSet) clientcmd.ClientConfig {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	flags.StringVar(&loadingRules.ExplicitPath, "koliconfig", "", "Path to the koliconfig file to use for CLI requests.")

	overrides := &clientcmd.ConfigOverrides{}
	// use the standard defaults for this client config
	mergo.Merge(&overrides.ClusterDefaults, clientcmd.DefaultCluster)

	flagNames := clientcmd.RecommendedConfigOverrideFlags("")
	// short flagnames are disabled by default.  These are here for compatibility with existing scripts
	flagNames.ClusterOverrideFlags.APIServer.ShortName = "s"

	// AuthInfo Flags
	// flagNames.AuthOverrideFlags
	// Context Flags
	flagNames.ContextOverrideFlags.Namespace.BindStringFlag(flags, &overrides.Context.Namespace)

	// Cluster Flags
	flagNames.ClusterOverrideFlags.APIServer.BindStringFlag(flags, &overrides.ClusterInfo.Server)
	//clientcmd.BindOverrideFlags(overrides, flags, flagNames)
	return clientcmd.NewInteractiveDeferredLoadingClientConfig(loadingRules, overrides, os.Stdin)
}
