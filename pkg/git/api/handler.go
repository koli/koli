package api

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/boltdb/bolt"
	"github.com/golang/glog"
	"github.com/google/go-github/github"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	platform "kolihub.io/koli/pkg/apis/core/v1alpha1"
	"kolihub.io/koli/pkg/apis/core/v1alpha1/draft"
	"kolihub.io/koli/pkg/clientset/auth0"
	"kolihub.io/koli/pkg/git/conf"
	"kolihub.io/koli/pkg/util"
)

// Handler .
type Handler struct {
	//controller controller.Client
	user      *platform.User
	cnf       *conf.Config
	clientset kubernetes.Interface
	gitClient *github.Client
	boltDB    *bolt.DB

	// rest config used for testing
	auth0RestConfig *auth0.Config
	fileExists      func(basepath, filename string) bool
}

func FileExistsFn(basepath, filename string) bool {
	if _, err := os.Stat(filepath.Join(basepath, filename)); os.IsNotExist(err) {
		return false
	}
	return true
}

// NewHandler .
func NewHandler(cnf *conf.Config, clientset kubernetes.Interface, db *bolt.DB) *Handler {
	glog.Info("Starting http server...")
	h := &Handler{cnf: cnf, clientset: clientset, fileExists: FileExistsFn}
	h.fileExists = FileExistsFn
	h.boltDB = db
	return h
}

func (h *Handler) GetUserIDSub() string {
	if h.user != nil {
		return h.user.Sub
	}
	return ""
}

func (h *Handler) validateNamespace(namespace string) *metav1.Status {
	nsMeta := draft.NewNamespaceMetadata(namespace)
	if !nsMeta.IsValid() {
		msg := fmt.Sprintf(`invalid namespace "%s"`, namespace)
		return util.StatusBadRequest(msg, nil, metav1.StatusReasonBadRequest)
	}
	if nsMeta.Customer() != h.user.Customer || nsMeta.Organization() != h.user.Organization {
		return util.StatusForbidden("the user is not the owner of the namespace", nil, metav1.StatusReasonForbidden)
	}
	return nil
}
