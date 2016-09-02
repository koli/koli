package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/renstrom/dedent"
	"github.com/spf13/cobra"

	apierrors "k8s.io/kubernetes/pkg/api/errors"
	"k8s.io/kubernetes/pkg/api/meta"
	"k8s.io/kubernetes/pkg/kubectl"
	cmdutil "k8s.io/kubernetes/pkg/kubectl/cmd/util"
	"k8s.io/kubernetes/pkg/kubectl/resource"
	"k8s.io/kubernetes/pkg/runtime"
	utilerrors "k8s.io/kubernetes/pkg/util/errors"
)

// DescribeOptions is the start of the data required to perform the operation.  As new fields are added, add them here instead of
// referencing the cmd.Flags()
type DescribeOptions struct {
	Filenames []string
	Recursive bool
}

var (
	describeExample = dedent.Dedent(`
		# Describe a pod
		kubectl describe pods/nginx

		# Describe all pods
		kubectl describe pods

		# Describe pods by label name=myLabel
		kubectl describe po -l name=myLabel`)
)

// NewCmdDescribe .
func NewCmdDescribe(f *cmdutil.Factory, out, cmdErr io.Writer) *cobra.Command {
	options := &DescribeOptions{}
	describerSettings := &kubectl.DescriberSettings{}

	validArgs := kubectl.DescribableResources()
	argAliases := kubectl.ResourceAliases(validArgs)

	cmd := &cobra.Command{
		Use:     "describe (TYPE [NAME_PREFIX | -l label] | TYPE/NAME)",
		Short:   "Show details of a specific resource or group of resources",
		Example: describeExample,
		Run: func(cmd *cobra.Command, args []string) {
			err := RunDescribe(f, out, cmdErr, cmd, args, options, describerSettings)
			cmdutil.CheckErr(err)
		},
		ValidArgs:  validArgs,
		ArgAliases: argAliases,
	}
	usage := "Filename, directory, or URL to a file containing the resource to describe"
	kubectl.AddJsonFilenameFlag(cmd, &options.Filenames, usage)
	cmdutil.AddRecursiveFlag(cmd, &options.Recursive)
	cmd.Flags().StringP("selector", "l", "", "Selector (label query) to filter on")
	cmd.Flags().Bool("all-namespaces", false, "If present, list the requested object(s) across all namespaces. Namespace in current context is ignored even if specified with --namespace.")
	cmd.Flags().BoolVar(&describerSettings.ShowEvents, "show-events", true, "If true, display events related to the described object.")
	cmdutil.AddInclude3rdPartyFlags(cmd)
	return cmd
}

// RunDescribe .
func RunDescribe(f *cmdutil.Factory, out, cmdErr io.Writer, cmd *cobra.Command, args []string, options *DescribeOptions, describerSettings *kubectl.DescriberSettings) error {
	selector := cmdutil.GetFlagString(cmd, "selector")
	allNamespaces := cmdutil.GetFlagBool(cmd, "all-namespaces")
	cmdNamespace, enforceNamespace, err := f.DefaultNamespace()
	if err != nil {
		return err
	}
	if allNamespaces {
		enforceNamespace = false
	}
	if len(args) == 0 && len(options.Filenames) == 0 {
		fmt.Fprint(cmdErr, "You must specify the type of resource to describe. ", validResources)
		return cmdutil.UsageError(cmd, "Required resource not specified.")
	}

	mapper, typer := f.Object(cmdutil.GetIncludeThirdPartyAPIs(cmd))
	r := resource.NewBuilder(mapper, typer, resource.ClientMapperFunc(f.ClientForMapping), f.Decoder(true)).
		ContinueOnError().
		NamespaceParam(cmdNamespace).DefaultNamespace().AllNamespaces(allNamespaces).
		FilenameParam(enforceNamespace, options.Recursive, options.Filenames...).
		SelectorParam(selector).
		ResourceTypeOrNameArgs(true, args...).
		Flatten().
		Do()
	err = r.Err()
	if err != nil {
		return err
	}

	allErrs := []error{}
	infos, err := r.Infos()
	if err != nil {
		if apierrors.IsNotFound(err) && len(args) == 2 {
			return DescribeMatchingResources(mapper, typer, f, cmdNamespace, args[0], args[1], describerSettings, out, err)
		}
		allErrs = append(allErrs, err)
	}

	first := true
	for _, info := range infos {
		mapping := info.ResourceMapping()
		describer, err := f.Describer(mapping)
		if err != nil {
			allErrs = append(allErrs, err)
			continue
		}
		s, err := describer.Describe(info.Namespace, info.Name, *describerSettings)
		if err != nil {
			allErrs = append(allErrs, err)
			continue
		}
		if first {
			first = false
			fmt.Fprint(out, s)
		} else {
			fmt.Fprintf(out, "\n\n%s", s)
		}
	}

	return utilerrors.NewAggregate(allErrs)
}

// DescribeMatchingResources .
func DescribeMatchingResources(mapper meta.RESTMapper, typer runtime.ObjectTyper, f *cmdutil.Factory, namespace, rsrc, prefix string, describerSettings *kubectl.DescriberSettings, out io.Writer, originalError error) error {
	r := resource.NewBuilder(mapper, typer, resource.ClientMapperFunc(f.ClientForMapping), f.Decoder(true)).
		NamespaceParam(namespace).DefaultNamespace().
		ResourceTypeOrNameArgs(true, rsrc).
		SingleResourceType().
		Flatten().
		Do()
	mapping, err := r.ResourceMapping()
	if err != nil {
		return err
	}
	describer, err := f.Describer(mapping)
	if err != nil {
		return err
	}
	infos, err := r.Infos()
	if err != nil {
		return err
	}
	isFound := false
	for ix := range infos {
		info := infos[ix]
		if strings.HasPrefix(info.Name, prefix) {
			isFound = true
			s, err := describer.Describe(info.Namespace, info.Name, *describerSettings)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "%s\n", s)
		}
	}
	if !isFound {
		return originalError
	}
	return nil
}
