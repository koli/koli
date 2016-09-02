package main

import (
	"os"

	kolicmd "github.com/kolibox/koli/pkg/cli"
	"k8s.io/kubernetes/pkg/client/unversioned/clientcmd"
	"k8s.io/kubernetes/pkg/kubectl/cmd/util"
)

func main() {
	kubeconfig := "/Users/san/.kube/config"
	loader := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig}
	loadedConfig, err := loader.Load()
	_ = err
	clientConfig := clientcmd.NewNonInteractiveClientConfig(
		*loadedConfig,
		loadedConfig.CurrentContext,
		&clientcmd.ConfigOverrides{},
		loader,
	)

	f := util.NewFactory(clientConfig)
	cmd := kolicmd.NewKubectlCommand(f, os.Stdin, os.Stdout, os.Stderr)
	cmd.Execute()
}
