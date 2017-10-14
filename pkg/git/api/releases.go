package api

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/golang/glog"
	"github.com/gorilla/mux"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"kolihub.io/koli/pkg/apis/core/v1alpha1/draft"
	gitutil "kolihub.io/koli/pkg/git/util"
	"kolihub.io/koli/pkg/util"
)

// Releases handles upload of tarballs as new releases
func (h *Handler) Releases(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	namespace, deployName, gitRev := params["namespace"], params["deployName"], params["gitSha"]
	key := fmt.Sprintf("%s/%s", namespace, deployName)
	if _, err := draft.NewSha(gitRev); err != nil {
		util.WriteResponseError(w, util.StatusBadRequest("invalid git SHA format", nil, metav1.StatusReasonBadRequest))
		return
	}
	nsMeta := draft.NewNamespaceMetadata(namespace)
	if nsMeta.Customer() != h.user.Customer || nsMeta.Organization() != h.user.Organization {
		util.WriteResponseError(w, util.StatusForbidden("the user is not the owner of the namespace", nil, metav1.StatusReasonForbidden))
		return
	}
	// path := filepath.Join(h.cnf.GitHome, constants.GitReleasePath, namespace, deployName)
	gitTask := gitutil.NewServerTask(h.cnf.GitHome, gitutil.NewObjectMeta(deployName, namespace))
	switch r.Method {
	case "GET":
		// TODO: could lead to memory leak, due to the size of files
		data, err := ioutil.ReadFile(filepath.Join(gitTask.FullReleasePath(), gitRev, params["file"]))
		if err != nil {
			util.WriteResponseError(w, util.StatusNotFound(err.Error(), nil))
			return
		}
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", params["file"]))
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.Write(data)
	case "POST":
		var data bytes.Buffer
		file, header, err := r.FormFile("file")
		if err != nil {
			msg := fmt.Sprintf("error getting file, %v", err)
			util.WriteResponseError(w, util.StatusBadRequest(msg, nil, metav1.StatusReasonBadRequest))
			return
		}
		defer file.Close()
		if _, err := gitTask.InitRelease(gitRev); err != nil {
			msg := fmt.Sprintf("failed creating releases directory, %v", err)
			util.WriteResponseError(w, util.StatusInternalError(msg, nil))
			return
		}

		filePath := filepath.Join(gitTask.FullReleasePath(), gitRev, header.Filename)
		if _, err := os.Stat(filePath); !os.IsNotExist(err) {
			glog.Infof(`%s - the file "%s" already exists for this revision, noop`, key, header.Filename)
			util.WriteResponseNoContent(w)
			return
		}
		io.Copy(&data, file)
		if err := ioutil.WriteFile(filePath, data.Bytes(), 0644); err != nil {
			msg := fmt.Sprintf("failed storing file %#v, %v", header.Filename, err)
			util.WriteResponseError(w, util.StatusBadRequest(msg, nil, metav1.StatusReasonBadRequest))
			return
		}
		data.Reset()
		util.WriteResponseNoContent(w)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
