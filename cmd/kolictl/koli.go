package main

import (
	"fmt"
	"os"

	kolicmd "github.com/kolibox/koli/pkg/cli"
	koliutil "github.com/kolibox/koli/pkg/cli/util"
)

func main() {
	factory, err := koliutil.NewFactory(nil)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	cmd := kolicmd.NewKubectlCommand(factory, os.Stdin, os.Stdout, os.Stderr)
	cmd.Execute()

	// cmdkube := cmd.NewKubectlCommand(cmdutil.NewFactory(nil), os.Stdin, os.Stdout, os.Stderr)
	// cmdkube.Execute()
}
