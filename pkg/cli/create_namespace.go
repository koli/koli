package cli

import (
	"fmt"
	"io"

	"github.com/renstrom/dedent"
	"github.com/spf13/cobra"

	"k8s.io/kubernetes/pkg/kubectl"
	kubecmd "k8s.io/kubernetes/pkg/kubectl/cmd"
	cmdutil "k8s.io/kubernetes/pkg/kubectl/cmd/util"
)

var (
	namespaceLong = dedent.Dedent(`
		Create a namespace with the specified name.`)

	namespaceExample = dedent.Dedent(`
		  # Create a new namespace named my-namespace
		  kubectl create namespace my-namespace`)
)

// NewCmdCreateNamespace is a macro command to create a new namespace
func NewCmdCreateNamespace(f *cmdutil.Factory, cmdOut io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "namespace NAME [--dry-run]",
		Aliases: []string{"ns"},
		Short:   "Create a namespace with the specified name",
		Long:    namespaceLong,
		Example: namespaceExample,
		Run: func(cmd *cobra.Command, args []string) {
			err := CreateNamespace(f, cmdOut, cmd, args)
			cmdutil.CheckErr(err)
		},
	}
	cmdutil.AddApplyAnnotationFlags(cmd)
	cmdutil.AddValidateFlags(cmd)
	cmdutil.AddPrinterFlags(cmd)
	cmdutil.AddGeneratorFlags(cmd, cmdutil.NamespaceV1GeneratorName)

	return cmd
}

// CreateNamespace implements the behavior to run the create namespace command
func CreateNamespace(f *cmdutil.Factory, cmdOut io.Writer, cmd *cobra.Command, args []string) error {
	name, err := kubecmd.NameFromCommandArgs(cmd, args)
	if err != nil {
		return err
	}
	var generator kubectl.StructuredGenerator
	switch generatorName := cmdutil.GetFlagString(cmd, "generator"); generatorName {
	case cmdutil.NamespaceV1GeneratorName:
		generator = &kubectl.NamespaceGeneratorV1{Name: name}
	default:
		return cmdutil.UsageError(cmd, fmt.Sprintf("Generator: %s not supported.", generatorName))
	}
	return kubecmd.RunCreateSubcommand(f, cmd, cmdOut, &kubecmd.CreateSubcommandOptions{
		Name:                name,
		StructuredGenerator: generator,
		DryRun:              cmdutil.GetDryRunFlag(cmd),
		OutputFormat:        cmdutil.GetFlagString(cmd, "output"),
	})
}
