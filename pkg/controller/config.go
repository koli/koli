package controller

import "k8s.io/client-go/rest"

// Config defines configuration parameters for the Operator.
type Config struct {
	Host              string
	GitReleaseHost    string
	TLSInsecure       bool
	TLSConfig         rest.TLSClientConfig
	SlugBuildImage    string
	SlugRunnerImage   string
	PlatformJWTSecret string
	ClusterName       string
	DefaultDomain     string
	DebugBuild        bool

	HealthzBindAddress string
	HealthzPort        int32
}

// IsValidStorageType check if it's valid storage type
func (c *Config) IsValidStorageType() bool {
	// TODO: check if names match with constants
	return true
}
