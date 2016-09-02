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
	deploymentLong = dedent.Dedent(`
                Create a deploy with the specified name.`)

	deploymentExample = dedent.Dedent(`
                # Create a new deploy named my-dep that runs the busybox image.
                kubectl create deploy my-dep --image=busybox`)
)

// NewCmdCreateDeploy is a macro command to create a new deployment
func NewCmdCreateDeploy(f *cmdutil.Factory, cmdOut io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "deploys NAME --image=image [--dry-run]",
		Aliases: []string{"dep"},
		Short:   "Create a deploy with the specified name.",
		Long:    deploymentLong,
		Example: deploymentExample,
		Run: func(cmd *cobra.Command, args []string) {
			err := CreateDeploy(f, cmdOut, cmd, args)
			cmdutil.CheckErr(err)
		},
	}
	cmdutil.AddApplyAnnotationFlags(cmd)
	cmdutil.AddValidateFlags(cmd)
	cmdutil.AddPrinterFlags(cmd)
	cmdutil.AddGeneratorFlags(cmd, cmdutil.DeploymentBasicV1Beta1GeneratorName)
	cmd.Flags().StringSlice("image", []string{}, "Image name to run.")
	cmd.MarkFlagRequired("image")
	return cmd
}

// CreateDeploy implements the behavior to run the create deploy command
func CreateDeploy(f *cmdutil.Factory, cmdOut io.Writer, cmd *cobra.Command, args []string) error {
	name, err := kubecmd.NameFromCommandArgs(cmd, args)
	if err != nil {
		return err
	}
	var generator kubectl.StructuredGenerator
	switch generatorName := cmdutil.GetFlagString(cmd, "generator"); generatorName {
	case cmdutil.DeploymentBasicV1Beta1GeneratorName:
		generator = &kubectl.DeploymentBasicGeneratorV1{Name: name, Images: cmdutil.GetFlagStringSlice(cmd, "image")}
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
