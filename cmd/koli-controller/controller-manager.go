// Copyright 2016 The prometheus-operator Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"flag"
	"fmt"
	"os"

	operator "github.com/kolibox/koli/pkg/controller/addon"
	"github.com/spf13/pflag"
)

var cfg operator.Config

func init() {
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.StringVar(&cfg.Host, "apiserver", "", "API Server addr, e.g. 'http://127.0.0.1:8080'. Omit parameter to run in on-cluster mode and utilize the service account token.")
	pflag.StringVar(&cfg.TLSConfig.CertFile, "cert-file", "", "Path to public TLS certificate file.")
	pflag.StringVar(&cfg.TLSConfig.KeyFile, "key-file", "", " Path to private TLS certificate file.")
	pflag.StringVar(&cfg.TLSConfig.CAFile, "ca-file", "", " Path to TLS CA file.")
	pflag.BoolVar(&cfg.TLSInsecure, "tls-insecure", false, " Don't verify API server's CA certificate.")
	pflag.Parse()
}

func main() {
	c, err := operator.New(cfg)
	if err != nil {
		fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}
	if err := c.Run(1, make(chan struct{})); err != nil {
		fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}
}
