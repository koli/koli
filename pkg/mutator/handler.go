package mutator

import (
	"github.com/golang/glog"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	platform "kolihub.io/koli/pkg/apis/v1alpha1"
)

// Handler is the base handler for all mutators
type Handler struct {
	clientset     kubernetes.Interface
	tprClient     rest.Interface
	usrTprClient  rest.Interface
	usrClientset  kubernetes.Interface
	user          *platform.User
	config        *Config
	allowedImages []string
}

// NewHandler creates a new mutator Handler
func NewHandler(clientset kubernetes.Interface, tprClient rest.Interface, cfg *Config) *Handler {
	listenAddr, isSecure := cfg.GetServeAddress()
	serveType := "insecurely"
	if isSecure {
		serveType = "securely"
	}
	glog.Infof("Starting HTTP server %s at %s", serveType, listenAddr)
	return &Handler{clientset: clientset, tprClient: tprClient, config: cfg, allowedImages: cfg.GetImages()}
}
