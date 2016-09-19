package cli

import (
	"fmt"

	"github.com/renstrom/dedent"
	"github.com/spf13/cobra"

	koliutil "github.com/kolibox/koli/pkg/cli/util"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/kubectl"
	cmdutil "k8s.io/kubernetes/pkg/kubectl/cmd/util"
	"k8s.io/kubernetes/pkg/kubectl/resource"
)

// ScaleOptions is the start of the data required to perform the operation.  As new fields are added, add them here instead of
// referencing the cmd.Flags()
type ScaleOptions struct {
	Filenames []string
	Recursive bool
}

var (
	scaleExample = dedent.Dedent(`
		# Scale a replicaset named 'foo' to 3.
		kubectl scale --replicas=3 deploy/foo

		# If the deployment named mysql's current size is 2, scale mysql to 3.
		kubectl scale --current-replicas=2 --replicas=3 deploy/mysql`)
)

// NewCmdScale returns a cobra command with the appropriate configuration and flags to run scale
func NewCmdScale(comm *koliutil.CommandParams) *cobra.Command {
	options := &ScaleOptions{}

	validArgs := []string{"deployment", "replicaset", "replicationcontroller", "job"}
	argAliases := kubectl.ResourceAliases(validArgs)

	cmd := &cobra.Command{
		Use:     "scale [--resource-version=version] [--current-replicas=count] --replicas=COUNT (TYPE NAME)",
		Short:   "Scale the size of pods",
		Example: scaleExample,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(cmdutil.ValidateOutputArgs(cmd))
			shortOutput := cmdutil.GetFlagString(cmd, "output") == "name"
			comm.Cmd = cmd
			err := RunScale(comm, args, shortOutput, options)
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

// RunScale executes the scaling
func RunScale(comm *koliutil.CommandParams, args []string, shortOutput bool, options *ScaleOptions) error {
	f, cmd, out := comm.KFactory(), comm.Cmd, comm.Out
	count := cmdutil.GetFlagInt(cmd, "replicas")
	if count < 0 {
		return cmdutil.UsageError(cmd, "--replicas=COUNT is required, and COUNT must be greater than or equal to 0")
	}

	err := koliutil.SetNamespacePrefix(cmd.Flag("namespace"), comm.User().ID)
	if err != nil {
		return err
	}
	cmdNamespace, err := comm.Factory.DefaultNamespace(cmd, true)
	if err != nil {
		return err
	}

	mapper, typer := f.Object(cmdutil.GetIncludeThirdPartyAPIs(cmd))
	r := resource.NewBuilder(mapper, typer, resource.ClientMapperFunc(f.ClientForMapping), f.Decoder(true)).
		ContinueOnError().
		NamespaceParam(cmdNamespace).DefaultNamespace().
		ResourceTypeOrNameArgs(false, args...).
		SingleResourceType().
		Flatten().
		Do()
	err = r.Err()
	if err != nil {
		return err
	}

	infos := []*resource.Info{}
	err = r.Visit(func(info *resource.Info, err error) error {
		if err == nil {
			infos = append(infos, info)
		}
		return nil
	})

	resourceVersion := cmdutil.GetFlagString(cmd, "resource-version")
	if len(resourceVersion) != 0 && len(infos) > 1 {
		return fmt.Errorf("cannot use --resource-version with multiple resources")
	}

	counter := 0
	err = r.Visit(func(info *resource.Info, err error) error {
		if err != nil {
			return err
		}

		mapping := info.ResourceMapping()
		scaler, err := f.Scaler(mapping)
		if err != nil {
			return err
		}

		currentSize := cmdutil.GetFlagInt(cmd, "current-replicas")
		precondition := &kubectl.ScalePrecondition{Size: currentSize, ResourceVersion: resourceVersion}
		retry := kubectl.NewRetryParams(kubectl.Interval, kubectl.Timeout)

		var waitForReplicas *kubectl.RetryParams
		if timeout := cmdutil.GetFlagDuration(cmd, "timeout"); timeout != 0 {
			waitForReplicas = kubectl.NewRetryParams(kubectl.Interval, timeout)
		}

		if err := scaler.Scale(info.Namespace, info.Name, uint(count), precondition, retry, waitForReplicas); err != nil {
			return err
		}
		if cmdutil.ShouldRecord(cmd, info) {
			patchBytes, err := cmdutil.ChangeResourcePatch(info, f.Command())
			if err != nil {
				return err
			}
			mapping := info.ResourceMapping()
			client, err := f.ClientForMapping(mapping)
			if err != nil {
				return err
			}
			helper := resource.NewHelper(client, mapping)
			_, err = helper.Patch(info.Namespace, info.Name, api.StrategicMergePatchType, patchBytes)
			if err != nil {
				return err
			}
		}
		counter++
		cmdutil.PrintSuccess(mapper, shortOutput, out, info.Mapping.Resource, info.Name, "scaled")
		return nil
	})
	if err != nil {
		return err
	}
	if counter == 0 {
		return fmt.Errorf("no objects passed to scale")
	}
	return nil
}
