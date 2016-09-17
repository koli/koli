package util

import (
	"io"

	"github.com/spf13/cobra"
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
