package util

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
	kubecmd "k8s.io/kubernetes/pkg/kubectl/cmd"
	cmdutil "k8s.io/kubernetes/pkg/kubectl/cmd/util"
)

// CommandParams wraps parameters required in commands
type CommandParams struct {
	Factory *Factory
	Options *kubecmd.CreateSubcommandOptions
	Cmd     *cobra.Command
	In      io.Reader
	Out     io.Writer
	Err     io.Writer
}

// KFactory returns a cmdutil.Factory pointer
func (c *CommandParams) KFactory() *cmdutil.Factory {
	return c.Factory.KubeFactory
}

// Controller returns a Controller pointer.
func (c *CommandParams) Controller() *Controller {
	return c.Factory.Ctrl
}

// User returns a UserMeta pointer
func (c *CommandParams) User() *UserMeta {
	return c.Factory.User
}

// PrefixResourceNames will add a prefix to every resource found based on the type of arguments,
// accept args containing <resource [name1, name2, (...)]> or <resource/[name1] <resource/[name2] (...)>.
// The first form mustn't contains the resource type!
func PrefixResourceNames(args []string, prefix string) {
	for i, arg := range args {
		resource := strings.Split(arg, "/")
		if len(resource) > 1 {
			args[i] = fmt.Sprintf("%s/%s-%s", resource[0], prefix, resource[1])
		} else {
			args[i] = fmt.Sprintf("%s-%s", prefix, resource[0])
		}
	}
}

// SetNamespacePrefix will set a prefix to a namespace name: 'default' will become '<prefix>-default'
func SetNamespacePrefix(flag *flag.Flag, userID string) error {
	prefix, namespace := "", flag.Value.String()
	if namespace != "" {
		ns := strings.Split(namespace, "/")
		if len(ns) > 2 {
			msg := "invalid namespace (%s): may not contain more than two slashes ['/']"
			return fmt.Errorf(msg, namespace)
		} else if len(ns) == 2 {
			prefix, namespace = ns[0], ns[1]
			if prefix != "" {
				namespace = fmt.Sprintf("%s-%s", prefix, namespace)
			}
		} else {
			namespace = fmt.Sprintf("%s-%s", userID, namespace)
		}
		flag.Value.Set(namespace)
	}
	return nil
}
