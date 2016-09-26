package cli

import (
	koliutil "github.com/kolibox/koli/pkg/cli/util"
	"github.com/spf13/cobra"

	"k8s.io/kubernetes/pkg/kubectl"
	kubecmd "k8s.io/kubernetes/pkg/kubectl/cmd"
	cmdutil "k8s.io/kubernetes/pkg/kubectl/cmd/util"
)

// NewCmdCreate .
func NewCmdCreate(comm *koliutil.CommandParams) *cobra.Command {
	options := &kubecmd.CreateOptions{}

	cmd := &cobra.Command{
		Use:   "create [subcommand]",
		Short: "Create a new resource",
		Run: func(cmd *cobra.Command, args []string) {
			if len(options.Filenames) == 0 {
				cmd.Help()
				return
			}
			cmdutil.CheckErr(kubecmd.ValidateArgs(cmd, args))
			cmdutil.CheckErr(cmdutil.ValidateOutputArgs(cmd))
			// cmdutil.CheckErr(kubecmd.RunCreate(f.KubeFactory, cmd, out, options))
		},
	}

	usage := "Filename, directory, or URL to file to use to create the resource"
	kubectl.AddJsonFilenameFlag(cmd, &options.Filenames, usage)
	cmd.MarkFlagRequired("filename")
	cmdutil.AddValidateFlags(cmd)
	cmdutil.AddRecursiveFlag(cmd, &options.Recursive)
	cmdutil.AddOutputFlagsForMutation(cmd)
	cmdutil.AddApplyAnnotationFlags(cmd)
	cmdutil.AddRecordFlag(cmd)
	cmdutil.AddInclude3rdPartyFlags(cmd)

	// create subcommands
	cmd.AddCommand(NewCmdCreateNamespace(comm.Factory, comm.Out))
	//cmd.AddCommand(kubecmd.NewCmdCreateSecret(f, out))
	//cmd.AddCommand(kubecmd.NewCmdCreateConfigMap(f, out))
	//cmd.AddCommand(kubecmd.NewCmdCreateServiceAccount(f, out))
	//cmd.AddCommand(kubecmd.NewCmdCreateService(f, out))
	cmd.AddCommand(NewCmdCreateDeploy(comm))
	return cmd
}
