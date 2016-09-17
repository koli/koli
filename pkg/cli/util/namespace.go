package util

import (
	"fmt"

	cmdutil "k8s.io/kubernetes/pkg/kubectl/cmd/util"
	"k8s.io/kubernetes/pkg/kubectl/resource"
)

// DefaultNamespace will filter a default namespace based on a label `sys.io/default=true`.
// Returns an error if none or more than one namespace is found
// The id is an external unique identifier filter the namespaces using by label `sys.io/id`
func DefaultNamespace(comm *CommandParams) (string, error) {
	f := comm.KFactory()
	flag := comm.Cmd.Flag("namespace")
	if flag.Value.String() != "" {
		return flag.Value.String(), nil
	}
	// The client doesn't provide any namespace, need to find a default one
	mapper, typer := f.Object(cmdutil.GetIncludeThirdPartyAPIs(comm.Cmd))
	selector := fmt.Sprintf("sys.io/id=%s,sys.io/default=true", comm.User().ID)
	r := resource.NewBuilder(mapper, typer, resource.ClientMapperFunc(f.ClientForMapping), f.Decoder(true)).
		SelectorParam(selector).
		ResourceTypes([]string{"namespace"}...).
		Latest().
		Flatten().
		Do()

	infos, err := r.Infos()
	if err != nil {
		return "", err
	} else if len(infos) != 1 {
		return "", fmt.Errorf("Found (%d) namespaces.", len(infos))
	}
	return infos[0].Name, nil
}
