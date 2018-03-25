package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/boltdb/bolt"

	"github.com/golang/glog"
	"github.com/gorilla/mux"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	platform "kolihub.io/koli/pkg/apis/core/v1alpha1"
	"kolihub.io/koli/pkg/apis/core/v1alpha1/draft"
	gitutil "kolihub.io/koli/pkg/git/util"
	"kolihub.io/koli/pkg/util"
)

const maxItems = 30

func (h *Handler) V1beta1DownloadFile(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	namespace, deployName, gitRev := params["namespace"], params["deployName"], params["gitSha"]
	if errStatus := h.validateNamespace(namespace); errStatus != nil {
		util.WriteResponseError(w, errStatus)
		return
	}
	basePath := filepath.Join(h.cnf.GitHome, "releases", namespace, deployName, gitRev)
	switch r.Method {
	case "GET":
		if !h.fileExists(basePath, params["file"]) {
			msg := fmt.Sprintf(`File "%s" not found`, filepath.Join(basePath, params["file"]))
			util.WriteResponseError(w, util.StatusNotFound(msg, nil))
			return
		}
		// TODO: could lead to memory leak, due to the size of files
		data, err := ioutil.ReadFile(filepath.Join(basePath, params["file"]))
		if err != nil {
			msg := fmt.Sprintf(`Failed reading file "%s", [%v]`, params["file"], err)
			util.WriteResponseError(w, util.StatusBadRequest(msg, nil, metav1.StatusReasonBadRequest))
			return
		}
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", params["file"]))
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.Write(data)
	}
}

func (h *Handler) V1beta1UploadRelease(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	namespace, deployName, gitRev := params["namespace"], params["deployName"], params["gitSha"]
	if errStatus := h.validateNamespace(namespace); errStatus != nil {
		util.WriteResponseError(w, errStatus)
		return
	}
	key := fmt.Sprintf("%s/%s", namespace, deployName)
	gitTask := gitutil.NewServerTask(h.cnf.GitHome, gitutil.NewObjectMeta(deployName, namespace))
	switch r.Method {
	case "POST":
		var data bytes.Buffer
		file, header, err := r.FormFile("file")
		if err != nil {
			msg := fmt.Sprintf("error getting file, %v", err)
			util.WriteResponseError(w, util.StatusBadRequest(msg, nil, metav1.StatusReasonBadRequest))
			return
		}
		defer file.Close()
		// Open the index.json file
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
	}
}

func (h *Handler) V1beta1SeekReleases(w http.ResponseWriter, r *http.Request) {
	qs := r.URL.Query()
	params := mux.Vars(r)
	namespace, deployName := params["namespace"], params["deployName"]
	if errStatus := h.validateNamespace(namespace); errStatus != nil {
		util.WriteResponseError(w, errStatus)
		return
	}
	if r.Method == "GET" {
		infoList := &platform.GitInfoList{}
		err := h.boltDB.View(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte(namespace))
			if b == nil {
				return fmt.Errorf(`bucket "%s" not found`, namespace)
			}
			var err error
			infoList.Items, err = SeekByAttribute(b.Cursor(), []byte(deployName+"/"), qs.Get("in"), qs.Get("q"))
			infoList.Total = len(infoList.Items)
			if infoList.Total > maxItems {
				infoList.Items = infoList.Items[len(infoList.Items)-maxItems:]
			}
			return err
		})
		if err != nil {
			msg := fmt.Sprintf("Failed seeking objects [%v]", err)
			util.WriteResponseError(w, util.StatusBadRequest(msg, nil, metav1.StatusReasonBadRequest))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(infoList)
	}
}

func (h *Handler) V1beta1ListReleases(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	namespace, deployName := params["namespace"], params["deployName"]
	// key := fmt.Sprintf("%s/%s", namespace, deployName)
	// gitTask := gitutil.NewServerTask(h.cnf.GitHome, gitutil.NewObjectMeta(deployName, namespace))
	if errStatus := h.validateNamespace(namespace); errStatus != nil {
		util.WriteResponseError(w, errStatus)
		return
	}
	switch r.Method {
	case "GET":
		infoList := &platform.GitInfoList{}
		notFound := false
		err := h.boltDB.View(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte(namespace))
			if b == nil {
				notFound = true
				return fmt.Errorf(`bucket "%s" not found`, namespace)
			}
			var err error
			infoList.Items, err = ListByDeploy(b.Cursor(), []byte(deployName+"/"))
			infoList.Total = len(infoList.Items)
			if infoList.Total > maxItems {
				infoList.Items = infoList.Items[len(infoList.Items)-maxItems:]
			}
			return err
		})
		if err != nil {
			if notFound {
				w.Header().Set("Content-Type", "application/json")
				util.WriteResponseSuccess(w, []byte(`[]`))
				return
			}
			msg := fmt.Sprintf("Failed retrieving objects [%v]", err)
			util.WriteResponseError(w, util.StatusBadRequest(msg, nil, metav1.StatusReasonBadRequest))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(infoList)
	case "POST":
		defer r.Body.Close()
		var requestBody platform.GitInfo
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			msg := fmt.Sprintf("failed decoding body from request, %v", err)
			util.WriteResponseError(w, util.StatusBadRequest(msg, nil, metav1.StatusReasonBadRequest))
			return
		}
		gitSha, err := platform.NewSha(requestBody.HeadCommit.ID)
		if err != nil {
			msg := fmt.Sprintf(`invalid commit [%v]`, err)
			util.WriteResponseError(w, util.StatusUnprocessableEntity(msg, nil, nil))
			return
		}
		requestBody.CreatedAt = time.Now().UTC()
		requestBody.Files = make(map[string]int64)

		dirPath := filepath.Join(h.cnf.GitHome, "releases", namespace, deployName, gitSha.Full())
		fiList, _ := ioutil.ReadDir(dirPath)
		for _, fi := range fiList {
			if fi.IsDir() {
				continue
			}
			requestBody.Files[fi.Name()] = fi.Size()
		}
		var alreadyExists = false
		err = h.boltDB.Update(func(tx *bolt.Tx) error {
			b, err := tx.CreateBucketIfNotExists([]byte(namespace))
			if err != nil {
				return err
			}
			key := []byte(filepath.Join(deployName, gitSha.Full()))
			if b.Get(key) != nil {
				alreadyExists = true
				return fmt.Errorf(`item "%s" already exists`, string(key))
			}

			data, err := json.Marshal(&requestBody)
			if err != nil {
				return err
			}
			return b.Put(key, data)
		})
		if err != nil {
			msg := fmt.Sprintf("failed storing payload, %v", err)
			status := util.StatusBadRequest(msg, nil, metav1.StatusReasonBadRequest)
			if alreadyExists {
				status = util.StatusConflict(msg, nil, nil)
			}
			util.WriteResponseError(w, status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(&requestBody)
	}
}

func (h *Handler) V1beta1Releases(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	namespace, deployName := params["namespace"], params["deployName"]
	gitSha, err := platform.NewSha(params["gitSha"])
	if err != nil {
		msg := fmt.Sprintf(`Invalid commit [%v]`, err)
		util.StatusBadRequest(msg, nil, metav1.StatusReasonBadRequest)
		return
	}
	if errStatus := h.validateNamespace(namespace); errStatus != nil {
		util.WriteResponseError(w, errStatus)
		return
	}
	basePath := filepath.Join(h.cnf.GitHome, "releases", namespace, deployName, gitSha.Full())
	switch r.Method {
	case "GET":
		info := &platform.GitInfo{}
		key := []byte(filepath.Join(deployName, gitSha.Full()))
		err := h.boltDB.View(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte(namespace))
			if b == nil {
				return fmt.Errorf(`bucket "%s" not found`, namespace)
			}
			return json.Unmarshal(b.Get(key), info)
		})
		if err != nil {
			msg := fmt.Sprintf(`Failed retrieving data for key "%s", [%v]`, string(key), err)
			util.WriteResponseError(w, util.StatusBadRequest(msg, nil, metav1.StatusReasonBadRequest))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(info)
	case "PUT":
		var requestBody platform.GitInfo
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			msg := fmt.Sprintf("failed decoding body from request, %v", err)
			util.WriteResponseError(w, util.StatusBadRequest(msg, nil, metav1.StatusReasonBadRequest))
			return
		}

		old := &platform.GitInfo{}
		err := h.boltDB.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte(namespace))
			if b == nil {
				return fmt.Errorf(`bucket "%s" not found`, namespace)
			}
			key := []byte(filepath.Join(deployName, gitSha.Full()))
			if err := json.Unmarshal(b.Get(key), old); err != nil {
				return fmt.Errorf("failed deserializing object [%v]", err)
			}
			if (int64(requestBody.BuildDuration)) > 0 {
				old.BuildDuration = requestBody.BuildDuration
			}
			if len(requestBody.Status) > 0 {
				old.Status = requestBody.Status
			}
			if len(requestBody.Namespace) > 0 {
				old.Namespace = requestBody.Namespace
			}
			if len(requestBody.Name) > 0 {
				old.Name = requestBody.Name
			}
			if len(requestBody.Lang) > 0 {
				old.Lang = requestBody.Lang
			}
			if len(requestBody.KubeRef) > 0 {
				old.KubeRef = requestBody.KubeRef
				// Reset when kubeRef changes
				// Indicating a "rebuild"
				old.BuildDuration = 0
				old.Lang = ""
				old.Status = ""
			}
			if len(requestBody.SourceType) > 0 {
				old.SourceType = requestBody.SourceType
			}
			if requestBody.Files != nil {
				for filename, size := range requestBody.Files {
					if _, ok := old.Files[filename]; !ok {
						if !h.fileExists(basePath, filename) {
							return fmt.Errorf(`file "%s" not found`, filename)
						}
						old.AddFile(filename, size)
					}
				}
			}
			data, err := json.Marshal(old)
			if err != nil {
				return fmt.Errorf("Failed serializing object [%v]", err)
			}
			return b.Put(key, data)
		})
		if err != nil {
			msg := fmt.Sprintf("Failed storing data, %v", err)
			util.WriteResponseError(w, util.StatusBadRequest(msg, nil, metav1.StatusReasonBadRequest))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(old)
	}

}

// Releases handles upload of tarballs as new releases
func (h *Handler) Releases(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	namespace, deployName, gitRev := params["namespace"], params["deployName"], params["gitSha"]
	key := fmt.Sprintf("%s/%s", namespace, deployName)
	if _, err := platform.NewSha(gitRev); err != nil {
		util.WriteResponseError(w, util.StatusBadRequest("invalid git SHA format", nil, metav1.StatusReasonBadRequest))
		return
	}
	nsMeta := draft.NewNamespaceMetadata(namespace)
	if nsMeta.Customer() != h.user.Customer || nsMeta.Organization() != h.user.Organization {
		util.WriteResponseError(w, util.StatusForbidden("the user is not the owner of the namespace", nil, metav1.StatusReasonForbidden))
		return
	}
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

func ListByDeploy(c *bolt.Cursor, prefix []byte) (infoList []platform.GitInfo, err error) {
	for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
		var info platform.GitInfo
		if err = json.Unmarshal(v, &info); err != nil {
			return
		}
		infoList = append(infoList, info)
	}
	return
}

func SeekByAttribute(c *bolt.Cursor, prefix []byte, attr, value string) (infoList []platform.GitInfo, err error) {
	if len(attr) == 0 || len(value) == 0 {
		return
	}
	for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
		var info platform.GitInfo
		if err = json.Unmarshal(v, &info); err != nil {
			return
		}
		switch attr {
		case "kubeRef":
			if info.KubeRef == value {
				infoList = append(infoList, info)
			}
		case "source":
			if info.SourceType == value {
				infoList = append(infoList, info)
			}
		case "status":
			if info.Status == v1.PodPhase(value) {
				infoList = append(infoList, info)
			}
		default:
			break
		}
	}
	return
}
