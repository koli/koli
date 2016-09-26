package cli

import (
	"fmt"
	"reflect"

	"github.com/renstrom/dedent"
	"github.com/spf13/cobra"

	koliutil "github.com/kolibox/koli/pkg/cli/util"
	"k8s.io/kubernetes/pkg/api"
	apiresource "k8s.io/kubernetes/pkg/api/resource"
	"k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/apis/extensions"
	"k8s.io/kubernetes/pkg/kubectl"
	kubecmd "k8s.io/kubernetes/pkg/kubectl/cmd"
	cmdutil "k8s.io/kubernetes/pkg/kubectl/cmd/util"
	"k8s.io/kubernetes/pkg/kubectl/resource"
	"k8s.io/kubernetes/pkg/runtime"
	utilerrors "k8s.io/kubernetes/pkg/util/errors"
)

var (
	deploymentLong = dedent.Dedent(`
                Create a deploy with the specified name.`)

	deploymentExample = dedent.Dedent(`
		# Create a new deploy named my-dep that runs the busybox image.
		kubectl create deploy my-deploy

		# Create new deployzao
		kubectl create deploy my-deploy2 --repo mylocalrepository.`)
)

// NewCmdCreateDeploy is a macro command to create a new deployment
func NewCmdCreateDeploy(comm *koliutil.CommandParams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "deploy NAME --image=image [--dry-run]",
		Aliases: []string{"dp", "deploys"},
		Short:   "Create a deploy with the specified name.",
		Long:    deploymentLong,
		Example: deploymentExample,
		Run: func(cmd *cobra.Command, args []string) {
			comm.Cmd = cmd
			err := CreateDeploy(comm, args)
			cmdutil.CheckErr(err)
		},
	}

	cmdutil.AddApplyAnnotationFlags(cmd)
	cmdutil.AddValidateFlags(cmd)
	cmdutil.AddPrinterFlags(cmd)
	cmdutil.AddGeneratorFlags(cmd, cmdutil.DeploymentBasicV1Beta1GeneratorName)
	cmd.Flags().StringP("repository", "r", "", "Repository name. Defaults to your working git directory")
	// cmd.Flags().String("release", "", "Release version")
	// cmd.Flags().StringSlice("image", []string{}, "Image name to run.")
	// cmd.MarkFlagRequired("image")
	return cmd
}

// CreateDeploy implements the behavior to run the create deploy command
func CreateDeploy(comm *koliutil.CommandParams, args []string) error {
	git, err := koliutil.NewGitExec(cmdutil.GetFlagString(comm.Cmd, "repository"), comm.Factory.CrafterRemote)
	if err != nil {
		return err
	}

	repository, err := git.GetTopLevelRepository()
	if err != nil {
		return cmdutil.UsageError(comm.Cmd,
			err.Error()+
				dedent.Dedent(`

			The 'git' command is required for this operation.

			A deploy is associate with a local repository, make 
			sure that you're inside a git repository.

			You could specify a different repository using the flag '--repository' or '-r':
			>>> koli create deploy my-deploy --repository /path/to/your/repo

			More info: https://kolibox.io/docs/deploys`))
	}
	name, err := kubecmd.NameFromCommandArgs(comm.Cmd, args)
	if err != nil {
		return err
	}

	err = koliutil.SetNamespacePrefix(comm.Cmd.Flag("namespace"), comm.User().ID)
	if err != nil {
		return err
	}
	namespace, err := comm.Factory.DefaultNamespace(comm.Cmd, true)
	if err != nil {
		return err
	}
	exists, err := comm.Factory.RepositoryExists(comm.Cmd, repository, namespace)
	if err != nil {
		return err
	}
	if exists {
		return cmdutil.UsageError(comm.Cmd,
			fmt.Sprintf("a repository (%s) already exists with that name", repository)+
				dedent.Dedent(`

			A repository must be unique on deployment resources.

			You could verify the deployments repositories:
			>>> koli get deploy -L sys.io/repo

			More info: https://kolibox.io/docs/deploys#repositories`))
	}

	if err := git.AddRemote(namespace, repository); err != nil {
		return cmdutil.UsageError(comm.Cmd, fmt.Sprintf("couldn't add remote (%s)", err))
	}

	computeResources := api.ResourceList{
		api.ResourceCPU:    apiresource.MustParse(comm.Factory.User.ComputeResources.CPU),
		api.ResourceMemory: apiresource.MustParse(comm.Factory.User.ComputeResources.Memory),
	}

	var generator kubectl.StructuredGenerator
	generator = &DeploymentBasicGeneratorV1{
		Name:             name,
		Paused:           true,
		Repository:       repository,
		ComputeResources: computeResources,
	}
	obj, err := generator.StructuredGenerate()
	if err != nil {
		return err
	}
	mapper, typer := comm.KFactory().Object(cmdutil.GetIncludeThirdPartyAPIs(comm.Cmd))
	gvks, _, err := typer.ObjectKinds(obj)
	if err != nil {
		return err
	}
	gvk := gvks[0]
	mapping, err := mapper.RESTMapping(unversioned.GroupKind{Group: gvk.Group, Kind: gvk.Kind}, gvk.Version)
	if err != nil {
		return err
	}
	client, err := comm.KFactory().ClientForMapping(mapping)
	if err != nil {
		return err
	}
	resourceMapper := &resource.Mapper{
		ObjectTyper:  typer,
		RESTMapper:   mapper,
		ClientMapper: resource.ClientMapperFunc(comm.KFactory().ClientForMapping),
	}
	info, err := resourceMapper.InfoForObject(obj, nil)
	if err != nil {
		return err
	}
	if err := kubectl.UpdateApplyAnnotation(info, comm.KFactory().JSONEncoder()); err != nil {
		return err
	}
	if !cmdutil.GetDryRunFlag(comm.Cmd) {
		obj, err = resource.NewHelper(client, mapping).Create(namespace, false, info.Object)
		if err != nil {
			return err
		}
	}

	outputFormat := cmdutil.GetFlagString(comm.Cmd, "output")
	if useShortOutput := outputFormat == "name"; useShortOutput || len(outputFormat) == 0 {
		cmdutil.PrintSuccess(mapper, useShortOutput, comm.Out, mapping.Resource, name, "created")
		return nil
	}

	return comm.KFactory().PrintObject(comm.Cmd, mapper, obj, comm.Out)

	/*return kubecmd.RunCreateSubcommand(comm.KFactory(), comm.Cmd, comm.Out, &kubecmd.CreateSubcommandOptions{
		Name:                name,
		StructuredGenerator: generator,
		DryRun:              cmdutil.GetDryRunFlag(comm.Cmd),
		OutputFormat:        cmdutil.GetFlagString(comm.Cmd, "output"),
	})*/
}

// DeploymentBasicGeneratorV1 supports stable generation of a deployment
type DeploymentBasicGeneratorV1 struct {
	Name             string
	Paused           bool
	ComputeResources api.ResourceList

	Repository string
	Release    string
}

// Ensure it supports the generator pattern that uses parameters specified during construction
var _ kubectl.StructuredGenerator = &DeploymentBasicGeneratorV1{}

// ParamNames .
func (DeploymentBasicGeneratorV1) ParamNames() []kubectl.GeneratorParam {
	return []kubectl.GeneratorParam{
		{Name: "name", Required: true},
		{Name: "paused", Required: true},
		{Name: "repository", Required: false},
		{Name: "release", Required: false},
	}
}

// StructuredGenerate outputs a deployment object using the configured fields
func (s *DeploymentBasicGeneratorV1) StructuredGenerate() (runtime.Object, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}

	podSpec := api.PodSpec{Containers: []api.Container{
		api.Container{
			Name:      s.Name,
			Image:     deployImage,
			Resources: api.ResourceRequirements{Limits: s.ComputeResources},
		},
	}}

	// setup default label and selector
	labels := map[string]string{}
	if s.Repository != "" {
		labels[fmt.Sprintf("%s/repo", koliutil.PrefixLabel)] = s.Repository
	}
	if s.Release != "" {
		labels[fmt.Sprintf("%s/release", koliutil.PrefixLabel)] = s.Release
	}
	labels["app"] = s.Name
	selector := unversioned.LabelSelector{MatchLabels: labels}
	deployment := extensions.Deployment{
		ObjectMeta: api.ObjectMeta{
			Name:   s.Name,
			Labels: labels,
		},
		Spec: extensions.DeploymentSpec{
			Paused:   s.Paused,
			Replicas: 1,
			Selector: &selector,
			Template: api.PodTemplateSpec{
				ObjectMeta: api.ObjectMeta{
					Labels: labels,
				},
				Spec: podSpec,
			},
		},
	}

	return &deployment, nil
}

// validate validates required fields are set to support structured generation
func (s *DeploymentBasicGeneratorV1) validate() error {
	if len(s.Name) == 0 {
		return fmt.Errorf("name must be specified")
	}
	return nil
}

// ValidateParams ensures that all required params are present in the params map
func ValidateParams(paramSpec []kubectl.GeneratorParam, params map[string]interface{}) error {
	allErrs := []error{}
	for ix := range paramSpec {
		if paramSpec[ix].Required {
			value, found := params[paramSpec[ix].Name]
			if !found || isZero(value) {
				allErrs = append(allErrs, fmt.Errorf("Parameter: %s is required", paramSpec[ix].Name))
			}
		}
	}
	return utilerrors.NewAggregate(allErrs)
}

func isZero(i interface{}) bool {
	if i == nil {
		return true
	}
	return reflect.DeepEqual(i, reflect.Zero(reflect.TypeOf(i)).Interface())
}
