package cli

import (
	"io"

	"github.com/renstrom/dedent"
	"github.com/spf13/cobra"

	"k8s.io/kubernetes/pkg/kubectl"
	kubecmd "k8s.io/kubernetes/pkg/kubectl/cmd"
	cmdutil "k8s.io/kubernetes/pkg/kubectl/cmd/util"
)

var (
	scaleExample = dedent.Dedent(`
		# Scale a replicaset named 'foo' to 3.
		kubectl scale --replicas=3 deploy/foo

		# If the deployment named mysql's current size is 2, scale mysql to 3.
		kubectl scale --current-replicas=2 --replicas=3 deploy/mysql`)
)

// NewCmdScale returns a cobra command with the appropriate configuration and flags to run scale
func NewCmdScale(f *cmdutil.Factory, out io.Writer) *cobra.Command {
	options := &kubecmd.ScaleOptions{}

	validArgs := []string{"deployment", "replicaset", "replicationcontroller", "job"}
	argAliases := kubectl.ResourceAliases(validArgs)

	cmd := &cobra.Command{
		Use:     "scale [--resource-version=version] [--current-replicas=count] --replicas=COUNT (TYPE NAME)",
		Short:   "Scale the size of pods",
		Example: scaleExample,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(cmdutil.ValidateOutputArgs(cmd))
			shortOutput := cmdutil.GetFlagString(cmd, "output") == "name"
			err := kubecmd.RunScale(f, out, cmd, args, shortOutput, options)
			cmdutil.CheckErr(err)
		},
		ValidArgs:  validArgs,
		ArgAliases: argAliases,
	}
	cmd.Flags().String("resource-version", "", "Precondition for resource version. Requires that the current resource version match this value in order to scale.")
	cmd.Flags().Int("current-replicas", -1, "Precondition for current size. Requires that the current size of the resource match this value in order to scale.")
	cmd.Flags().Int("replicas", -1, "The new desired number of replicas. Required.")
	cmd.MarkFlagRequired("replicas")
	cmd.Flags().Duration("timeout", 0, "The length of time to wait before giving up on a scale operation, zero means don't wait. Any other values should contain a corresponding time unit (e.g. 1s, 2m, 3h).")
	cmdutil.AddOutputFlagsForMutation(cmd)
	cmdutil.AddRecordFlag(cmd)
	cmdutil.AddInclude3rdPartyFlags(cmd)

	usage := "Filename, directory, or URL to a file identifying the resource to set a new size"
	kubectl.AddJsonFilenameFlag(cmd, &options.Filenames, usage)
	cmdutil.AddRecursiveFlag(cmd, &options.Recursive)
	return cmd
}
