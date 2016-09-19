package cli

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/renstrom/dedent"
	"github.com/spf13/cobra"

	koliutil "github.com/kolibox/koli/pkg/cli/util"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/errors"
	"k8s.io/kubernetes/pkg/api/meta"
	"k8s.io/kubernetes/pkg/kubectl"
	cmdutil "k8s.io/kubernetes/pkg/kubectl/cmd/util"
	"k8s.io/kubernetes/pkg/kubectl/resource"
)

// DeleteOptions is the start of the data required to perform the operation.
// As new fields are added, add them here instead of referencing the cmd.Flags()
type DeleteOptions struct {
	Filenames         []string
	Recursive         bool
	IsNamespaced      bool
	IsResourceSlashed bool
}

var (
	deleteExample = dedent.Dedent(`
		# Delete pods
		kubectl delete pod foo

		# Delete pods and services with label name=myLabel.
		kubectl delete pods -l name=myLabel

		# Delete a pod immediately (no graceful shutdown)
		kubectl delete pods foo --now

		# Delete a pod with UID 1234-56-7890-234234-456456.
		kubectl delete pod 1234-56-7890-234234-456456`)
)

// NewCmdDelete .
func NewCmdDelete(comm *koliutil.CommandParams) *cobra.Command {
	options := &DeleteOptions{IsNamespaced: true}

	// retrieve a list of handled resources from printer as valid args
	validArgs, argAliases := []string{}, []string{}
	p, err := comm.KFactory().Printer(nil, kubectl.PrintOptions{
		ColumnLabels: []string{},
	})
	cmdutil.CheckErr(err)
	if p != nil {
		validArgs = p.HandledResources()
		argAliases = kubectl.ResourceAliases(validArgs)
	}

	cmd := &cobra.Command{
		Use:     "delete ([TYPE [(NAME | -l label)])",
		Short:   "Delete resources by names",
		Example: deleteExample,
		Run: func(cmd *cobra.Command, args []string) {
			comm.Cmd = cmd
			cmdutil.CheckErr(cmdutil.ValidateOutputArgs(cmd))
			hasNS, _ := regexp.MatchString(`(namespace[s]?|ns)[/]?`, args[0])
			if hasNS {
				options.IsNamespaced = false
				if strings.Contains(args[0], "/") {
					koliutil.PrefixResourceNames(args, comm.User().ID)
				} else {
					koliutil.PrefixResourceNames(args[1:], comm.User().ID)
				}
			}
			err := RunDelete(comm, args, options)
			cmdutil.CheckErr(err)
		},
		SuggestFor: []string{"rm", "stop"},
		ValidArgs:  validArgs,
		ArgAliases: argAliases,
	}
	usage := "Filename, directory, or URL to a file containing the resource to delete."
	kubectl.AddJsonFilenameFlag(cmd, &options.Filenames, usage)
	cmdutil.AddRecursiveFlag(cmd, &options.Recursive)
	cmd.Flags().StringP("selector", "l", "", "Selector (label query) to filter on.")
	cmd.Flags().Bool("all", false, "[-all] to select all the specified resources.")
	cmd.Flags().Bool("ignore-not-found", false, "Treat \"resource not found\" as a successful delete. Defaults to \"true\" when --all is specified.")
	cmd.Flags().Bool("cascade", true, "If true, cascade the deletion of the resources managed by this resource (e.g. Pods created by a ReplicationController).  Default true.")
	cmd.Flags().Int("grace-period", -1, "Period of time in seconds given to the resource to terminate gracefully. Ignored if negative.")
	cmd.Flags().Bool("now", false, "If true, resources are force terminated without graceful deletion (same as --grace-period=0).")
	cmd.Flags().Duration("timeout", 0, "The length of time to wait before giving up on a delete, zero means determine a timeout from the size of the object")
	cmdutil.AddOutputFlagsForMutation(cmd)
	cmdutil.AddInclude3rdPartyFlags(cmd)
	return cmd
}

// RunDelete .
func RunDelete(comm *koliutil.CommandParams, args []string, options *DeleteOptions) error {
	cmd, out := comm.Cmd, comm.Out
	f := comm.KFactory()

	err := koliutil.SetNamespacePrefix(cmd.Flag("namespace"), comm.User().ID)
	if err != nil {
		return err
	}
	cmdNamespace, err := comm.Factory.DefaultNamespace(comm.Cmd, options.IsNamespaced)
	if err != nil {
		return err
	}
	mapper, typer := f.Object(cmdutil.GetIncludeThirdPartyAPIs(cmd))
	r := resource.NewBuilder(mapper, typer, resource.ClientMapperFunc(f.ClientForMapping), f.Decoder(true)).
		ContinueOnError().
		NamespaceParam(cmdNamespace).DefaultNamespace().
		ResourceTypeOrNameArgs(false, args...).RequireObject(false).
		SingleResourceType().
		Flatten().
		Do()
	err = r.Err()
	if err != nil {
		return err
	}

	ignoreNotFound := cmdutil.GetFlagBool(cmd, "ignore-not-found")

	gracePeriod := cmdutil.GetFlagInt(cmd, "grace-period")
	if cmdutil.GetFlagBool(cmd, "now") {
		if gracePeriod != -1 {
			return fmt.Errorf("--now and --grace-period cannot be specified together")
		}
		gracePeriod = 0
	}

	shortOutput := cmdutil.GetFlagString(cmd, "output") == "name"
	// By default use a reaper to delete all related resources.
	if cmdutil.GetFlagBool(cmd, "cascade") {
		return ReapResult(r, f, out, cmdutil.GetFlagBool(cmd, "cascade"),
			ignoreNotFound, cmdutil.GetFlagDuration(cmd, "timeout"),
			gracePeriod, shortOutput, mapper, false)
	}
	return DeleteResult(r, out, ignoreNotFound, shortOutput, mapper)
}

// ReapResult .
func ReapResult(r *resource.Result,
	f *cmdutil.Factory,
	out io.Writer, isDefaultDelete, ignoreNotFound bool,
	timeout time.Duration, gracePeriod int,
	shortOutput bool,
	mapper meta.RESTMapper, quiet bool) error {
	found := 0
	if ignoreNotFound {
		r = r.IgnoreErrors(errors.IsNotFound)
	}
	err := r.Visit(func(info *resource.Info, err error) error {
		if err != nil {
			return err
		}
		found++
		reaper, err := f.Reaper(info.Mapping)
		if err != nil {
			// If there is no reaper for this resources and the user didn't explicitly ask for stop.
			if kubectl.IsNoSuchReaperError(err) && isDefaultDelete {
				return deleteResource(info, out, shortOutput, mapper)
			}
			return cmdutil.AddSourceToErr("reaping", info.Source, err)
		}
		var options *api.DeleteOptions
		if gracePeriod >= 0 {
			options = api.NewDeleteOptions(int64(gracePeriod))
		}
		if err := reaper.Stop(info.Namespace, info.Name, timeout, options); err != nil {
			return cmdutil.AddSourceToErr("stopping", info.Source, err)
		}
		if !quiet {
			cmdutil.PrintSuccess(mapper, shortOutput, out, info.Mapping.Resource, info.Name, "deleted")
		}
		return nil
	})
	if err != nil {
		return err
	}
	if found == 0 {
		fmt.Fprintf(out, "No resources found\n")
	}
	return nil
}

// DeleteResult .
func DeleteResult(r *resource.Result, out io.Writer, ignoreNotFound bool, shortOutput bool, mapper meta.RESTMapper) error {
	found := 0
	if ignoreNotFound {
		r = r.IgnoreErrors(errors.IsNotFound)
	}
	err := r.Visit(func(info *resource.Info, err error) error {
		if err != nil {
			return err
		}
		found++
		return deleteResource(info, out, shortOutput, mapper)
	})
	if err != nil {
		return err
	}
	if found == 0 {
		fmt.Fprintf(out, "No resources found\n")
	}
	return nil
}

func deleteResource(info *resource.Info, out io.Writer, shortOutput bool, mapper meta.RESTMapper) error {
	if err := resource.NewHelper(info.Client, info.Mapping).Delete(info.Namespace, info.Name); err != nil {
		return cmdutil.AddSourceToErr("deleting", info.Source, err)
	}
	cmdutil.PrintSuccess(mapper, shortOutput, out, info.Mapping.Resource, info.Name, "deleted")
	return nil
}
