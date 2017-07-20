package receive

import "time"

const (
	builderPodTick    = 100
	objectStorageTick = 500
)

// Config is the envconfig (http://github.com/kelseyhightower/envconfig)
// compatible struct for the builder's git-receive hook.
type Config struct {
	// k8s service discovery env vars
	Host                       string `envconfig:"KUBERNETES_SERVICE_HOST" required:"true"`
	DeployName                 string `envconfig:"DEPLOY_NAME" required:"true"`
	Namespace                  string `envconfig:"NAMESPACE" required:"true"`
	UserJwtToken               string `envconfig:"USER_JWT_TOKEN" required:"true"`
	GitHome                    string `envconfig:"GIT_HOME" default:"/home/git"`
	GitAPIHostname             string `envconfig:"GIT_API_HOSTNAME" default:"http://git-server.koli-system"`
	Debug                      bool   `envconfig:"DEBUG" default:"false"`
	BuilderPodTickDurationMSec int    `envconfig:"BUILDER_POD_TICK_DURATION" default:"100"`
	BuilderPodWaitDurationMSec int    `envconfig:"BUILDER_POD_WAIT_DURATION" default:"900000"` // 15 minutes
	SessionIdleIntervalMsec    int    `envconfig:"SESSION_IDLE_INTERVAL" default:"10000"`      // 10 seconds
}

// BuilderPodTickDuration returns the size of the interval used to check for
// the end of the execution of a Pod building an application.
func (c Config) BuilderPodTickDuration() time.Duration {
	return time.Duration(time.Duration(c.BuilderPodTickDurationMSec) * time.Millisecond)
}

// BuilderPodWaitDuration returns the maximum time to wait for the end
// of the execution of a Pod building an application.
func (c Config) BuilderPodWaitDuration() time.Duration {
	return time.Duration(time.Duration(c.BuilderPodWaitDurationMSec) * time.Millisecond)
}

// SessionIdleInterval returns the ticker interval to wait for status
func (c Config) SessionIdleInterval() time.Duration {
	return time.Duration(time.Duration(c.SessionIdleIntervalMsec) * time.Millisecond)
}

// CheckDurations checks if ticks for builder and object storage are not bigger
// than the maximum duration. In case of this it will set the tick to the default.
func (c *Config) CheckDurations() {
	if c.BuilderPodTickDurationMSec >= c.BuilderPodWaitDurationMSec {
		c.BuilderPodTickDurationMSec = builderPodTick
	}
	if c.BuilderPodTickDurationMSec < builderPodTick {
		c.BuilderPodTickDurationMSec = builderPodTick
	}
}
