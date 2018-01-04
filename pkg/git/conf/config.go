package conf

import (
	"io/ioutil"
	"time"

	"github.com/kelseyhightower/envconfig"
	"k8s.io/client-go/rest"
)

// Auth0 hold information about auth0 authentication configuration
type Auth0 struct {
	AdminClientID        string `envconfig:"ADMIN_CLIENT_ID"`
	AdminClientSecret    string `envconfig:"ADMIN_CLIENT_SECRET"`
	AdminAudienceURL     string `envconfig:"ADMIN_AUDIENCE_URL"`
	PlatformClientSecret string `envconfig:"PLATFORM_CLIENT_SECRET" required:"true"`
	PlatformPubKeyFile   string `envconfig:"PLATFORM_JWT_PUB_KEY_FILE" required:"true"`
	PlatformPubKey       []byte
}

// Config represents the required SSH server configuration.
type Config struct {
	Auth0
	Host                        string `envconfig:"KUBERNETES_SERVICE_HOST" required:"true"`
	GitHome                     string `envconfig:"GIT_HOME" default:"/home/git"`
	CleanerPollSleepDurationSec int    `envconfig:"CLEANER_POLL_SLEEP_DURATION_SEC" default:"5"`
	LockTimeout                 int    `envconfig:"GIT_LOCK_TIMEOUT" default:"10"`
	GitAPIHostname              string
	TLSInsecure                 bool
	TLSConfig                   rest.TLSClientConfig
	// Not Implemented yet
	GitHubHookSecret   string `envconfig:"GITHUB_HOOK_SECRET"`
	HealthzBindAddress string
	HealthzPort        int32
}

// CleanerPollSleepDuration returns c.CleanerPollSleepDurationSec as a time.Duration.
func (c Config) CleanerPollSleepDuration() time.Duration {
	return time.Duration(c.CleanerPollSleepDurationSec) * time.Second
}

//GitLockTimeout return LockTimeout in minutes
func (c Config) GitLockTimeout() time.Duration {
	return time.Duration(c.LockTimeout) * time.Minute
}

// ReadPubKey read a public file from Config.Auth0.PlatformPubKeyFile and stores in Config.Auth0.PlatfromPubKey
func (c *Config) ReadPubKey() (err error) {
	if len(c.Auth0.PlatformPubKeyFile) > 0 {
		c.Auth0.PlatformPubKey, err = ioutil.ReadFile(c.Auth0.PlatformPubKeyFile)
	}
	return
}

// EnvConfig is a convenience function to process the envconfig (
// https://github.com/kelseyhightower/envconfig) based configuration environment variables into
// conf. Additional notes:
//
// - appName will be passed as the first parameter to envconfig.Process
// - conf should be a pointer to an envconfig compatible struct. If you'd like to use struct
// 	 	tags to customize your struct, see
// 		https://github.com/kelseyhightower/envconfig#struct-tag-support
func EnvConfig(appName string, conf interface{}) error {
	if err := envconfig.Process(appName, conf); err != nil {
		return err
	}
	return nil
}
