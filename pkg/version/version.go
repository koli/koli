package version

import (
	"fmt"
	"runtime"

	k8sversion "k8s.io/kubernetes/pkg/version"
)

// Info contains versioning information.
// TODO: Add []string of api versions supported? It's still unclear
// how we'll want to distribute that information.
type Info struct {
	// KubernetesVersion string `json:"kubernetesVersion"`
	K8SVersion string `json:"k8s_lib_version"`
	GitVersion string `json:"git_version"`
	GitCommit  string `json:"git_commit"`
	BuildDate  string `json:"build_date"`
	GoVersion  string `json:"go_version"`
	Compiler   string `json:"compiler"`
	Platform   string `json:"platform"`
}

var (
	version    string
	gitVersion string
	gitCommit  = "$Format:%H$"          // sha1 from git, output of $(git rev-parse HEAD)
	buildDate  = "1970-01-01T00:00:00Z" // build date in ISO8601 format, output of $(date -u +'%Y-%m-%dT%H:%M:%SZ')
)

// Get returns the overall codebase version. It's for detecting
// what code a binary was built from.
func Get() Info {
	// These variables typically come from -ldflags settings and in
	// their absence fallback to the settings in pkg/version/base.go
	return Info{
		// KubernetesVersion: kubernetesClientVersion,
		K8SVersion: fmt.Sprintf("v%s.%s.0", k8sversion.Get().Major, k8sversion.Get().Minor),
		GitVersion: gitVersion,
		GitCommit:  gitCommit,
		BuildDate:  buildDate,
		GoVersion:  runtime.Version(),
		Compiler:   runtime.Compiler,
		Platform:   fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}
