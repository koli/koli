package cli

import (
	"errors"
	"fmt"

	koliutil "github.com/kolibox/koli/pkg/cli/util"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/unversioned"
	kubecmd "k8s.io/kubernetes/pkg/kubectl/cmd"
	cmdutil "k8s.io/kubernetes/pkg/kubectl/cmd/util"
	"k8s.io/kubernetes/pkg/registry/extensions/thirdpartyresourcedata"

	"github.com/spf13/cobra"
)

// NewCmdCreateAddOn create pre-defined applications in the form of deployments
func NewCmdCreateAddOn(comm *koliutil.CommandParams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "addon NAME",
		Short: "Create a new addon",
		Run: func(cmd *cobra.Command, args []string) {
			comm.Cmd = cmd
			err := createAddOn(comm, args)
			cmdutil.CheckErr(err)
		},
	}
	cmdutil.AddPrinterFlags(cmd)

	return cmd
}

func createAddOn(comm *koliutil.CommandParams, args []string) error {
	name, err := kubecmd.NameFromCommandArgs(comm.Cmd, args)
	if err != nil {
		return err
	}

	mapper, _ := comm.KFactory().Object()
	gvk := unversioned.GroupVersionKind{
		Group:   "extensions",
		Kind:    "Deployment",
		Version: "v1beta1",
	}
	mapping, err := mapper.RESTMapping(unversioned.GroupKind{
		Group: gvk.Group, Kind: gvk.Kind}, gvk.Version)
	if err != nil {
		return err
	}
	gv := gvk.GroupVersion()
	serializer := thirdpartyresourcedata.NewNegotiatedSerializer(api.Codecs, gvk.Kind, gv, gv)
	comm.Controller().Request.SetSerializer(gvk.GroupVersion(), serializer)

	bodyData := []byte(fmt.Sprintf(`{"app_name": "%s"}`, name))
	result := comm.Controller().Request.POST().
		Resource("addons").
		SubResource(name).
		Body(bodyData).
		Do()

	if result.StatusCode() == 401 {
		return errors.New("wrong credentials")
	}

	obj, err := result.Get()
	if err != nil {
		return err
	}

	outputFormat := cmdutil.GetFlagString(comm.Cmd, "output")
	if useShortOutput := outputFormat == "name"; useShortOutput || len(outputFormat) == 0 {
		cmdutil.PrintSuccess(mapper, useShortOutput, comm.Out, mapping.Resource, name, false, "created")
		return nil
	}
	return comm.KFactory().PrintObject(comm.Cmd, mapper, obj, comm.Out)
}
