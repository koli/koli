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

	"github.com/gorilla/mux"
	"kolihub.io/koli/pkg/apis/v1alpha1/draft"
	gitutil "kolihub.io/koli/pkg/git/util"
)

// Releases handles upload of tarballs as new releases
func (h *Handler) Releases(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	namespace, deployName, gitRev := params["namespace"], params["deployName"], params["gitSha"]
	if _, err := draft.NewSha(gitRev); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Invalid git SHA format\n")
		return
	}
	// path := filepath.Join(h.cnf.GitHome, constants.GitReleasePath, namespace, deployName)
	gitTask := gitutil.NewServerTask(h.cnf.GitHome, gitutil.NewObjectMeta(deployName, namespace))
	switch r.Method {
	case "GET":
		// TODO: could lead to memory leak, due to the size of files
		data, err := ioutil.ReadFile(filepath.Join(gitTask.FullReleasePath(), gitRev, params["file"]))
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, "%s\n", err.Error())
			return
		}
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", params["file"]))
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.Write(data)
	case "POST":
		// os.Stat(filepath.Join(h.cnf.GitHome, constants.GitReleasePath))
		var data bytes.Buffer
		file, header, err := r.FormFile("file")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "Error getting file: %s\n", err)
			return
		}
		defer file.Close()
		if _, err := gitTask.InitRelease(gitRev); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "Failed creating releases directory: %s", err)
			return
		}

		filePath := filepath.Join(gitTask.FullReleasePath(), gitRev, header.Filename)
		if _, err := os.Stat(filePath); !os.IsNotExist(err) {
			w.WriteHeader(http.StatusConflict)
			fmt.Fprintf(w, "The file %s already exists for this revision\n", header.Filename)
			return
		}
		io.Copy(&data, file)
		if err := ioutil.WriteFile(filePath, data.Bytes(), 0644); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "Failed storing file %s: %s\n", header.Filename, err)
			return
		}
		data.Reset()
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
