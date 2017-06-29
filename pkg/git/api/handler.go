package api

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	platform "kolihub.io/koli/pkg/apis/v1alpha1"
	"kolihub.io/koli/pkg/git/conf"
)

// Handler .
type Handler struct {
	//controller controller.Client
	user      *platform.User
	cnf       *conf.Config
	clientset *kubernetes.Clientset
}

// NewHandler .
func NewHandler(cnf *conf.Config, clientset *kubernetes.Clientset) Handler {
	fmt.Println("Starting http server...")
	return Handler{cnf: cnf, clientset: clientset}
}
