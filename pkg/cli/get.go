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
	getLong = dedent.Dedent(`
		Display one or many resources.

		`) + PossibleResourceTypes

	getExample = dedent.Dedent(`
		# List all pods in ps output format.
		koli get pods

		# List all pods in ps output format with more information (such as node name).
		koli get pods -o wide

		# List a single pod with specified NAME in ps output format.
		koli get pods web

		# List one or more resources by their type and names.
		koli get deploys/web pods/web-pod-13je7`)
)

// NewCmdGet creates a command object for the generic "get" action, which
// retrieves one or more resources from a server.
func NewCmdGet(f *cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &kubecmd.GetOptions{}

	// retrieve a list of handled resources from printer as valid args
	validArgs, argAliases := []string{}, []string{}
	p, err := f.Printer(nil, kubectl.PrintOptions{
		ColumnLabels: []string{},
	})
	cmdutil.CheckErr(err)
	if p != nil {
		validArgs = p.HandledResources()
		argAliases = kubectl.ResourceAliases(validArgs)
	}

	cmd := &cobra.Command{
		Use:     "get [(-o|--output=)json|yaml|wide] (TYPE [NAME | -l label] | TYPE/NAME ...) [flags]",
		Short:   "Display one or many resources",
		Long:    getLong,
		Example: getExample,
		Run: func(cmd *cobra.Command, args []string) {
			err := kubecmd.RunGet(f, out, errOut, cmd, args, options)
			cmdutil.CheckErr(err)
		},
		SuggestFor: []string{"list", "ps"},
		ValidArgs:  validArgs,
		ArgAliases: argAliases,
	}
	cmdutil.AddPrinterFlags(cmd)
	//cmd.Flags().StringP("output", "o", "", "Output format. One of: json|yaml|wide|name.")
	//cmd.Flags().Bool("show-labels", false, "When printing, show all labels as the last column (default hide labels column).")
	//cmd.Flags().String("template", "", "Template string or path to template file to use.")
	//cmd.MarkFlagFilename("template")
	//cmd.Flags().BoolP("show-all", "a", false, "When printing, show all resources (default hide terminated pods.)")
	cmd.Flags().Bool("show-kind", false, "If present, list the resource type for the requested object(s).")

	cmd.Flags().StringP("selector", "l", "", "Selector (label query) to filter on")
	cmd.Flags().BoolP("watch", "w", false, "After listing/getting the requested object, watch for changes.")
	cmd.Flags().Bool("watch-only", false, "Watch for changes to the requested object(s), without listing/getting first.")

	cmd.Flags().Bool("all-namespaces", false, "If present, list the requested object(s) across all namespaces. Namespace in current context is ignored even if specified with --namespace.")
	cmd.Flags().StringSliceP("label-columns", "L", []string{}, "Accepts a comma separated list of labels that are going to be presented as columns.")
	// TODO: test in other cluster
	cmd.Flags().Bool("export", false, "If true, use 'export' for the resources.  Exported resources are stripped of cluster-specific information.")
	usage := "Filename, directory, or URL to a file identifying the resource to get from a server."
	kubectl.AddJsonFilenameFlag(cmd, &options.Filenames, usage)
	cmdutil.AddRecursiveFlag(cmd, &options.Recursive)
	cmdutil.AddInclude3rdPartyFlags(cmd)
	return cmd
}
