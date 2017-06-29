package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"kolihub.io/koli/pkg/git/conf"
	gitreceive "kolihub.io/koli/pkg/git/receive"
	"kolihub.io/koli/pkg/version"
)

const (
	gitReceiveConfAppName = "koli"
)

type appArgs struct {
	OldRev  string
	NewRev  string
	RefName string
}

var showVersion bool
var args appArgs

func main() {
	flag.StringVar(&args.OldRev, "oldrev", "", "the old revision sha-1")
	flag.StringVar(&args.NewRev, "newrev", "", "the new revision sha-1")
	flag.StringVar(&args.RefName, "refname", "", "the path of the ref. e.g.: refs/heads/master")
	flag.BoolVar(&showVersion, "version", false, "print version information and quit.")
	flag.Parse()
	if showVersion {
		version := version.Get()
		b, err := json.Marshal(&version)
		if err != nil {
			fmt.Printf("failed decoding version: %s\n", err)
			os.Exit(1)
		}
		fmt.Println(string(b))
		return
	}

	globalConfig := new(conf.Config)
	if err := conf.EnvConfig(gitReceiveConfAppName, globalConfig); err != nil {
		log.Printf("Error getting global config for %s [%s]", gitReceiveConfAppName, err)
		os.Exit(1)
	}

	cnf := new(gitreceive.Config)
	if err := conf.EnvConfig(gitReceiveConfAppName, cnf); err != nil {
		log.Printf("Error getting config for %s [%s]", gitReceiveConfAppName, err)
		os.Exit(1)
	}

	cnf.CheckDurations()
	if err := gitreceive.Run(cnf, args.OldRev, args.NewRev, args.RefName); err != nil {
		log.Printf("Error running git receive hook [%s]", err)
		os.Exit(1)
	}

}
