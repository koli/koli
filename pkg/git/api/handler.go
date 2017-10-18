package api

import (
	"github.com/golang/glog"
	"github.com/google/go-github/github"

	"k8s.io/client-go/kubernetes"
	platform "kolihub.io/koli/pkg/apis/core/v1alpha1"
	"kolihub.io/koli/pkg/clientset/auth0"
	"kolihub.io/koli/pkg/git/conf"
)

// Handler .
type Handler struct {
	//controller controller.Client
	user      *platform.User
	cnf       *conf.Config
	clientset kubernetes.Interface
	gitClient *github.Client

	// rest config used for testing
	auth0RestConfig *auth0.Config
}

// NewHandler .
func NewHandler(cnf *conf.Config, clientset kubernetes.Interface) Handler {
	glog.Info("Starting http server...")
	return Handler{cnf: cnf, clientset: clientset}
}

func (h *Handler) GetUserIDSub() string {
	if h.user != nil {
		return h.user.Sub
	}
	return ""
}
