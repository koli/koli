package controller

import "k8s.io/kubernetes/pkg/client/restclient"

// Config defines configuration parameters for the Operator.
type Config struct {
	Host            string
	GitReleaseHost  string
	TLSInsecure     bool
	TLSConfig       restclient.TLSClientConfig
	SlugBuildImage  string
	SlugRunnerImage string
	ClusterName     string
	DebugBuild      bool
}

// IsValidStorageType check if it's valid storage type
func (c *Config) IsValidStorageType() bool {
	// TODO: check if names match with constants
	return true
}
