package cli

import (
	"encoding/json"
	"io"

	"github.com/renstrom/dedent"
	"github.com/spf13/cobra"

	koliutil "github.com/kolibox/koli/pkg/cli/util"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/kubectl"
	kubecmd "k8s.io/kubernetes/pkg/kubectl/cmd"
	cmdutil "k8s.io/kubernetes/pkg/kubectl/cmd/util"
	"k8s.io/kubernetes/pkg/registry/thirdpartyresourcedata"
)

var (
	namespaceLong = dedent.Dedent(`
		Create a namespace with the specified name.`)

	namespaceExample = dedent.Dedent(`
		  # Create a new namespace named my-namespace
		  kubectl create namespace my-namespace`)
)

// NewCmdCreateNamespace is a macro command to create a new namespace
func NewCmdCreateNamespace(f *koliutil.Factory, cmdOut io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "namespace NAME [--dry-run]",
		Aliases: []string{"ns"},
		Short:   "Create a namespace with the specified name",
		Long:    namespaceLong,
		Example: namespaceExample,
		Run: func(cmd *cobra.Command, args []string) {
			err := CreateNamespace(f, cmdOut, cmd, args)
			cmdutil.CheckErr(err)
			// err = createDefaultQuota(namespace, f, cmdOut, cmd)
			// cmdutil.CheckErr(err)
		},
	}
	cmdutil.AddApplyAnnotationFlags(cmd)
	cmdutil.AddValidateFlags(cmd)
	cmdutil.AddPrinterFlags(cmd)
	cmdutil.AddGeneratorFlags(cmd, cmdutil.NamespaceV1GeneratorName)

	return cmd
}

// CreateNamespace adds a new namespace in the controller API
func CreateNamespace(f *koliutil.Factory, cmdOut io.Writer, cmd *cobra.Command, args []string) error {
	name, err := kubecmd.NameFromCommandArgs(cmd, args)
	if err != nil {
		return err
	}
	structuredGenerator := &kubectl.NamespaceGeneratorV1{Name: name}

	mapper, typer := f.KubeFactory.Object(cmdutil.GetIncludeThirdPartyAPIs(cmd))
	obj, err := structuredGenerator.StructuredGenerate()
	gvks, _, err := typer.ObjectKinds(obj)
	if err != nil {
		return err
	}
	gvk := gvks[0]

	mapping, err := mapper.RESTMapping(unversioned.GroupKind{Group: gvk.Group, Kind: gvk.Kind}, gvk.Version)
	if err != nil {
		return err
	}

	if !cmdutil.GetDryRunFlag(cmd) {
		gv := gvk.GroupVersion()
		serializer := thirdpartyresourcedata.NewNegotiatedSerializer(api.Codecs, gvk.Kind, gv, gv)
		f.Ctrl.Request.SetSerializer(gvk.GroupVersion(), serializer)
		ns := koliutil.Namespace{Name: name}
		data, err := json.Marshal(ns)

		if err != nil {
			return err
		}
		obj, err = f.Ctrl.Resource("namespaces").Create(data)
		if err != nil {
			return err
		}
	}

	outputFormat := cmdutil.GetFlagString(cmd, "output")
	if useShortOutput := outputFormat == "name"; useShortOutput || len(outputFormat) == 0 {
		cmdutil.PrintSuccess(mapper, useShortOutput, cmdOut, mapping.Resource, name, "created")
		return nil
	}

	return f.KubeFactory.PrintObject(cmd, mapper, obj, cmdOut)
}

/*
// CreateNamespace implements the behavior to run the create namespace command
func CreateNamespace(f *koliutil.Factory, cmdOut io.Writer, cmd *cobra.Command, args []string) (string, error) {
	name, err := kubecmd.NameFromCommandArgs(cmd, args)
	if err != nil {
		return "", err
	}

	// var generator kubectl.StructuredGenerator // TODO: Remove option "generator" cmdutil.GetFlagString(cmd, "generator"
	labels := map[string]string{ // TODO: Fix hard-coded
		"sys-id":    f.User.RootID,
		"sys-owner": f.User.Owner,
	}
	return name, kubecmd.RunCreateSubcommand(f.KubeFactory, cmd, cmdOut, &kubecmd.CreateSubcommandOptions{
		Name:                name,
		StructuredGenerator: &koliutil.NamespaceGeneratorV1{Name: name, Labels: labels},
		DryRun:              cmdutil.GetDryRunFlag(cmd),
		OutputFormat:        cmdutil.GetFlagString(cmd, "output"),
	})
}

func createDefaultQuota(userMeta koliutil.UserMeta, namespace string, f *cmdutil.Factory, cmdOut io.Writer, cmd *cobra.Command) error {
	name := "sys-default-objects" // TODO: move to a constant NAME
	options := &kubecmd.CreateSubcommandOptions{
		Name: name,
		StructuredGenerator: &kubectl.ResourceQuotaGeneratorV1{
			Name: name,
			Hard: userMeta.Objects(),
			//Scopes: "BestEffort,",
		},
		DryRun:       cmdutil.GetDryRunFlag(cmd),
		OutputFormat: cmdutil.GetFlagString(cmd, "output"),
	}

	obj, err := options.StructuredGenerator.StructuredGenerate()
	if err != nil {
		return err
	}
	mapper, typer := f.Object(cmdutil.GetIncludeThirdPartyAPIs(cmd))
	gvks, _, err := typer.ObjectKinds(obj)
	if err != nil {
		return err
	}
	gvk := gvks[0]
	mapping, err := mapper.RESTMapping(unversioned.GroupKind{Group: gvk.Group, Kind: gvk.Kind}, gvk.Version)
	if err != nil {
		return err
	}
	client, err := f.ClientForMapping(mapping)
	if err != nil {
		return err
	}
	resourceMapper := &resource.Mapper{
		ObjectTyper:  typer,
		RESTMapper:   mapper,
		ClientMapper: resource.ClientMapperFunc(f.ClientForMapping),
	}
	info, err := resourceMapper.InfoForObject(obj, nil)
	if err != nil {
		return err
	}
	if err := kubectl.UpdateApplyAnnotation(info, f.JSONEncoder()); err != nil {
		return err
	}
	if !options.DryRun {
		obj, err = resource.NewHelper(client, mapping).Create(namespace, false, info.Object)
		if err != nil {
			return err
		}
	}
	return nil
}*/
