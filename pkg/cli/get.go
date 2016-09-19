package cli

import (
	"fmt"
	"regexp"
	"strings"

	koliutil "github.com/kolibox/koli/pkg/cli/util"
	"github.com/renstrom/dedent"
	"github.com/spf13/cobra"
	"k8s.io/kubernetes/pkg/api/meta"
	"k8s.io/kubernetes/pkg/kubectl"
	cmdutil "k8s.io/kubernetes/pkg/kubectl/cmd/util"
	"k8s.io/kubernetes/pkg/kubectl/resource"
	"k8s.io/kubernetes/pkg/runtime"
	utilerrors "k8s.io/kubernetes/pkg/util/errors"
	"k8s.io/kubernetes/pkg/watch"
	// kubecmd "k8s.io/kubernetes/pkg/kubectl/cmd"
)

// GetOptions is the start of the data required to perform the operation.
// As new fields are added, add them here instead of referencing the cmd.Flags()
type GetOptions struct {
	Filenames            []string
	Recursive            bool
	IsNamespaced         bool
	IsSingleResourceType bool
	IsResourceSlashed    bool

	Raw string
}

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
func NewCmdGet(comm *koliutil.CommandParams) *cobra.Command {
	options := &GetOptions{IsNamespaced: true}

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
		Use:     "get [(-o|--output=)json|yaml|wide] (TYPE [NAME | -l label] | TYPE/NAME ...) [flags]",
		Short:   "Display one or many resources",
		Long:    getLong,
		Example: getExample,
		Run: func(cmd *cobra.Command, args []string) {
			comm.Cmd = cmd
			if len(args) == 0 {
				fmt.Fprint(comm.Out, "You must specify the type of resource to get. ", validResources)
				cmdutil.CheckErr(cmdutil.UsageError(cmd, "Required resource not specified."))
			}
			// Match namespace resource: 'resourcetype' or 'resourcetype/'
			hasNS, _ := regexp.MatchString(`(namespace[s]?|ns)[/]?`, args[0])
			if hasNS {
				options.IsNamespaced = false
				options.IsSingleResourceType = true
				if strings.Contains(args[0], "/") {
					options.IsResourceSlashed = true
				}
				err := RunGet(comm, args, options)
				cmdutil.CheckErr(err)
				return
			}
			for _, arg := range args[1:] {
				// Match namespace resource: 'resourcetype/'.
				// Prevent the user querying multiple resources with namespaces.
				// E.G.: $ command get svc/default ns/default
				hasNS, _ = regexp.MatchString(`(namespace[s]?|ns)/`, arg)
				if hasNS {
					cmdutil.CheckErr(cmdutil.UsageError(cmd, "Could not query multiple resources with namespaces."))
				}
			}
			err = RunGet(comm, args, options)
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

// RunGet implements the generic Get command
func RunGet(comm *koliutil.CommandParams, args []string, options *GetOptions) error {
	cmd, f, out := comm.Cmd, comm.KFactory(), comm.Out

	selector := cmdutil.GetFlagString(cmd, "selector")
	allNamespaces := cmdutil.GetFlagBool(cmd, "all-namespaces")
	showKind := cmdutil.GetFlagBool(cmd, "show-kind")

	mapper, typer := f.Object(cmdutil.GetIncludeThirdPartyAPIs(cmd))

	err := koliutil.SetNamespacePrefix(cmd.Flag("namespace"), comm.User().ID)
	if err != nil {
		return err
	}
	cmdNamespace, err := comm.Factory.DefaultNamespace(cmd, options.IsNamespaced)
	if err != nil {
		return cmdutil.UsageError(cmd,
			"Could not find a default namespace."+
				dedent.Dedent(`

			You can create a new namespace:
			>>> koli create namespace <mynamespace>

			Or configure an existent one:
			>>> koli set default ns <namespace>

			To list all created namespaces:
			>>> koli get ns

			More info: https://kolibox.io/docs/namespaces`))
	}

	// always show resources when getting by name or filename
	argsHasNames, err := resource.HasNames(args)
	if err != nil {
		return err
	}
	if len(options.Filenames) > 0 || argsHasNames {
		cmd.Flag("show-all").Value.Set("true")
	}

	if argsHasNames && !options.IsNamespaced {
		if options.IsResourceSlashed {
			koliutil.PrefixResourceNames(args, comm.User().ID)
		} else {
			koliutil.PrefixResourceNames(args[1:], comm.User().ID) // args[1:] skip <resourcetype>
		}
	}

	// TODO: querying namespaces could be insecure
	// The user could list all the namespaces in the cluster
	if !options.IsNamespaced && !options.IsResourceSlashed && !argsHasNames {
		// Override selector!
		// TODO: review! Prefix must be set in this context
		selector = fmt.Sprintf("sys.io/id=%s", comm.User().ID)
	}

	// export := cmdutil.GetFlagBool(cmd, "export")

	// handle watch separately since we cannot watch multiple resource types
	isWatch, isWatchOnly := cmdutil.GetFlagBool(cmd, "watch"), cmdutil.GetFlagBool(cmd, "watch-only")
	if isWatch || isWatchOnly {
		r := resource.NewBuilder(mapper, typer, resource.ClientMapperFunc(f.ClientForMapping), f.Decoder(true)).
			NamespaceParam(cmdNamespace).DefaultNamespace().
			SelectorParam(selector).
			ResourceTypeOrNameArgs(true, args...).
			SingleResourceType().
			Latest().
			Do()
		err := r.Err()
		if err != nil {
			return err
		}
		infos, err := r.Infos()
		if err != nil {
			return err
		}
		if len(infos) != 1 {
			return fmt.Errorf("watch is only supported on individual resources and resource collections - %d resources were found", len(infos))
		}
		info := infos[0]
		mapping := info.ResourceMapping()
		printer, err := f.PrinterForMapping(cmd, mapping, false)
		if err != nil {
			return err
		}
		obj, err := r.Object()
		if err != nil {
			return err
		}

		// watching from resourceVersion 0, starts the watch at ~now and
		// will return an initial watch event.  Starting form ~now, rather
		// the rv of the object will insure that we start the watch from
		// inside the watch window, which the rv of the object might not be.
		rv := "0"
		isList := meta.IsListType(obj)
		if isList {
			// the resourceVersion of list objects is ~now but won't return
			// an initial watch event
			rv, err = mapping.MetadataAccessor.ResourceVersion(obj)
			if err != nil {
				return err
			}
		}

		// print the current object
		if !isWatchOnly {
			if err := printer.PrintObj(obj, out); err != nil {
				return fmt.Errorf("unable to output the provided object: %v", err)
			}
			// printer.FinishPrint(errOut, mapping.Resource)
		}

		// print watched changes
		w, err := r.Watch(rv)
		if err != nil {
			return err
		}

		first := true
		kubectl.WatchLoop(w, func(e watch.Event) error {
			if !isList && first {
				// drop the initial watch event in the single resource case
				first = false
				return nil
			}
			err := printer.PrintObj(e.Object, out)
			// if err == nil {
			// printer.FinishPrint(errOut, mapping.Resource)
			// }
			return err
		})
		return nil
	}

	builder := resource.NewBuilder(mapper, typer, resource.ClientMapperFunc(f.ClientForMapping), f.Decoder(true)).
		NamespaceParam(cmdNamespace).DefaultNamespace().
		SelectorParam(selector).
		ResourceTypeOrNameArgs(true, args...).
		ContinueOnError().
		Latest().
		Flatten()

	var r *resource.Result
	if options.IsSingleResourceType {
		r = builder.SingleResourceType().Do()
	} else {
		r = builder.Do()
	}
	err = r.Err()
	if err != nil {
		return err
	}

	printer, generic, err := cmdutil.PrinterForCommand(cmd)
	if err != nil {
		return err
	}

	if generic {
		clientConfig, err := f.ClientConfig()
		if err != nil {
			return err
		}

		allErrs := []error{}
		singular := false
		infos, err := r.IntoSingular(&singular).Infos()
		if err != nil {
			if singular {
				return err
			}
			allErrs = append(allErrs, err)
		}

		// the outermost object will be converted to the output-version, but inner
		// objects can use their mappings
		version, err := cmdutil.OutputVersion(cmd, clientConfig.GroupVersion)
		if err != nil {
			return err
		}
		// res := ""
		// if len(infos) > 0 {
		// res = infos[0].ResourceMapping().Resource
		// }

		obj, err := resource.AsVersionedObject(infos, !singular, version, f.JSONEncoder())
		if err != nil {
			return err
		}

		if err := printer.PrintObj(obj, out); err != nil {
			allErrs = append(allErrs, err)
		}
		// printer.FinishPrint(errOut, res)
		return utilerrors.NewAggregate(allErrs)
	}

	allErrs := []error{}
	infos, err := r.Infos()
	if err != nil {
		allErrs = append(allErrs, err)
	}

	objs := make([]runtime.Object, len(infos))
	for ix := range infos {
		objs[ix] = infos[ix].Object
	}

	var sorter *kubectl.RuntimeSort
	// Removed sorting HERE

	// use the default printer for each object
	printer = nil
	var lastMapping *meta.RESTMapping
	w := kubectl.GetNewTabWriter(out)

	if mustPrintWithKinds(objs, infos, sorter) {
		showKind = true
	}

	for ix := range objs {
		var mapping *meta.RESTMapping
		var original runtime.Object
		if sorter != nil {
			mapping = infos[sorter.OriginalPosition(ix)].Mapping
			original = infos[sorter.OriginalPosition(ix)].Object
		} else {
			mapping = infos[ix].Mapping
			original = infos[ix].Object
		}
		if printer == nil || lastMapping == nil || mapping == nil || mapping.Resource != lastMapping.Resource {
			if printer != nil {
				w.Flush()
				// printer.FinishPrint(errOut, lastMapping.Resource)
			}
			printer, err = f.PrinterForMapping(cmd, mapping, allNamespaces)
			if err != nil {
				allErrs = append(allErrs, err)
				continue
			}
			lastMapping = mapping
		}
		if resourcePrinter, found := printer.(*kubectl.HumanReadablePrinter); found {
			resourceName := resourcePrinter.GetResourceKind()
			if mapping != nil {
				if resourceName == "" {
					resourceName = mapping.Resource
				}
				if alias, ok := kubectl.ResourceShortFormFor(mapping.Resource); ok {
					resourceName = alias
				} else if resourceName == "" {
					resourceName = "none"
				}
			} else {
				resourceName = "none"
			}

			if showKind {
				resourcePrinter.EnsurePrintWithKind(resourceName)
			}

			if err := printer.PrintObj(original, w); err != nil {
				allErrs = append(allErrs, err)
			}
			continue
		}
		if err := printer.PrintObj(original, w); err != nil {
			allErrs = append(allErrs, err)
			continue
		}
	}
	w.Flush()
	// if printer != nil {
	// printer.FinishPrint(errOut, lastMapping.Resource)
	// }
	return utilerrors.NewAggregate(allErrs)
}

// mustPrintWithKinds determines if printer is dealing
// with multiple resource kinds, in which case it will
// return true, indicating resource kind will be
// included as part of printer output
func mustPrintWithKinds(objs []runtime.Object, infos []*resource.Info, sorter *kubectl.RuntimeSort) bool {
	var lastMap *meta.RESTMapping

	for ix := range objs {
		var mapping *meta.RESTMapping
		if sorter != nil {
			mapping = infos[sorter.OriginalPosition(ix)].Mapping
		} else {
			mapping = infos[ix].Mapping
		}

		// display "kind" only if we have mixed resources
		if lastMap != nil && mapping.Resource != lastMap.Resource {
			return true
		}
		lastMap = mapping
	}

	return false
}
