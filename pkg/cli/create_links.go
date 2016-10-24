package cli

import (
	"errors"
	"fmt"

	koliutil "github.com/kolibox/koli/pkg/cli/util"
	"github.com/renstrom/dedent"
	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubernetes/pkg/kubectl/cmd/util"
)

var (
	linksExample = dedent.Dedent(`
		# Allow traffic from the default namespace to 'redis' addon
		koli links addons redis
		
		# Allow traffic from namespace 'prod' to 'redis' addon 
		koli links addons/redis -n dev
		
		# Allow traffic from namespace 'dev' to 'mysql' and 'redis' addon
		koli links addons mysql redis --namespace=dev
		`)
)

// NewCmdCreateLinks creates a command object for allowing traffic between namespaces
func NewCmdCreateLinks(comm *koliutil.CommandParams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "links (NAMESPACE [SERVICE]) [flags]",
		Short:   "Link addons between namespaces",
		Example: linksExample,
		Run: func(cmd *cobra.Command, args []string) {
			comm.Cmd = cmd
			if len(args) != 1 {
				cmdutil.CheckErr(cmdutil.UsageError(cmd, "Required link name, not specified."))
			}

			err := runSetLinks(comm, args[0])
			cmdutil.CheckErr(err)
		},
		ArgAliases: []string{"link"},
	}
	return cmd
}

func runSetLinks(comm *koliutil.CommandParams, linkName string) error {
	err := koliutil.SetNamespacePrefix(comm.Cmd.Flag("namespace"), comm.User().ID)
	if err != nil {
		return err
	}
	cmdNamespace, err := comm.Factory.DefaultNamespace(comm.Cmd, true)
	if err != nil {
		return cmdutil.UsageError(comm.Cmd,
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
	bodyData := []byte(fmt.Sprintf(`{"link_name": "%s"}`, linkName))
	result := comm.Controller().Request.POST().
		Resource("namespaces").
		Name(cmdNamespace).
		SubResource("links").
		Body(bodyData).
		Do()

	if result.StatusCode() == 401 {
		return errors.New("wrong credentials")
	}

	if result.StatusCode() == 201 {
		fmt.Println("link created.")
		return nil
	}
	obj, err := result.Raw()
	return fmt.Errorf("unknown error (%s) (%s)", string(obj), err)
}
