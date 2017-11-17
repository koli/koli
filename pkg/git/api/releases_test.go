package api

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/boltdb/bolt"
	"github.com/gorilla/mux"
	"github.com/pborman/uuid"
	"k8s.io/api/core/v1"
	platform "kolihub.io/koli/pkg/apis/core/v1alpha1"
	"kolihub.io/koli/pkg/git/conf"
	"kolihub.io/koli/pkg/request"
)

func getKey(deployName, gitSha string) []byte         { return []byte(filepath.Join(deployName, gitSha)) }
func fileExistsTrueFn(basepath, filename string) bool { return true }
func getBoltDb(t *testing.T) (*bolt.DB, func()) {
	dbFile := fmt.Sprintf("/tmp/%s.db", uuid.New()[:6])
	db, err := bolt.Open(dbFile, 0600, nil)
	if err != nil {
		t.Fatalf("Failed open bolt database: %v", err)
	}
	return db, func() {
		db.Close()
		os.Remove(dbFile)
	}
}

func newGitInfo(namespace, name, id string, files map[string]int64) *platform.GitInfo {
	i := &platform.GitInfo{
		Name:       name,
		Namespace:  namespace,
		KubeRef:    "foo",
		GitBranch:  "master",
		SourceType: "github",
		HeadCommit: platform.HeadCommit{
			ID:        id,
			Author:    "Koli Inc",
			AvatarURL: "https://avatar-url.jpg",
			Compare:   "https://compare-url",
			Message:   "A good commit",
			URL:       "https://github.com/koli/go-getting-started",
		},
		Files: files,
	}
	if files == nil {
		i.Files = make(map[string]int64)
	}
	return i
}

func LoadDbWithRandomData(db *bolt.DB, namespace, deployName string, items int) {
	obj := newGitInfo(namespace, deployName, "", nil)
	db.Update(func(tx *bolt.Tx) error {
		b, _ := tx.CreateBucketIfNotExists([]byte(namespace))
		_ = b
		for i := 0; i < items; i++ {
			obj.HeadCommit.ID = randomShaHash()
			data, _ := json.Marshal(obj)
			b.Put(getKey(deployName, obj.HeadCommit.ID), data)
		}
		// random data
		for i := 0; i < 10; i++ {
			obj.HeadCommit.ID = randomShaHash()
			data, _ := json.Marshal(obj)
			b.Put(getKey(deployName+"r1", obj.HeadCommit.ID), data)
		}
		return nil
	})
}

func randomShaHash() string {
	hash := sha1.New()
	hash.Write([]byte(uuid.New()))
	return hex.EncodeToString(hash.Sum(nil))
}

func TestCreateNewReleaseMetadata(t *testing.T) {
	var (
		namespace, name, gitSha = "prod-kim-koli", "myapp", "b4b36461355c0caf16b7deb8d33ba7dc5ba7093e"
		requestPath             = fmt.Sprintf("/releases/v1beta1/%s/%s/objects/%s", namespace, name, gitSha)
		r                       = mux.NewRouter()
		gitHandler              = NewHandler(&conf.Config{GitHome: "/tmp"}, nil, nil)
		requestBody             = newGitInfo(namespace, name, gitSha, nil)
	)
	s := r.PathPrefix("/releases/v1beta1/{namespace}/{deployName}").Subrouter()
	s.HandleFunc("/objects/{gitSha}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gitHandler.V1beta1ListReleases(w, r)
	})).Methods("POST").Headers("Content-Type", "application/json")
	requestURL, ts := runHttpTestServer(s, gitHandler, nil)
	defer ts.Close()
	db, close := getBoltDb(t)
	gitHandler.boltDB = db
	gitHandler.user = &platform.User{Customer: "kim", Organization: "koli"}
	defer close()

	_, err := request.NewRequest(nil, requestURL).
		Post().
		Body(requestBody).
		RequestPath(requestPath).
		Do().
		Raw()
	if err != nil {
		t.Fatalf("Failed creating release: %#v", err)
	}
	gitHandler.boltDB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(namespace))
		key := filepath.Join(name, gitSha)
		got := &platform.GitInfo{}
		json.Unmarshal(b.Get([]byte(key)), got)
		requestBody.CreatedAt = got.CreatedAt
		if !reflect.DeepEqual(requestBody, got) {
			t.Errorf("GOT: %#v, EXPECTED: %#v", got, requestBody)
		}
		return nil
	})
}

func TestMustNotOverrideReleaseMetadata(t *testing.T) {
	var (
		namespace, name, gitSha = "prod-kim-koli", "myapp", "b4b36461355c0caf16b7deb8d33ba7dc5ba7093e"
		requestPath             = fmt.Sprintf("/releases/v1beta1/%s/%s/objects", namespace, name)
		r                       = mux.NewRouter()
		gitHandler              = NewHandler(&conf.Config{GitHome: "/tmp"}, nil, nil)
		requestBody             = newGitInfo(namespace, name, gitSha, nil)
	)
	s := r.PathPrefix("/releases/v1beta1/{namespace}/{deployName}").Subrouter()
	s.HandleFunc("/objects", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gitHandler.V1beta1ListReleases(w, r)
	})).Methods("POST").Headers("Content-Type", "application/json")
	requestURL, ts := runHttpTestServer(s, gitHandler, nil)
	defer ts.Close()
	db, close := getBoltDb(t)
	gitHandler.boltDB = db
	gitHandler.user = &platform.User{Customer: "kim", Organization: "koli"}
	defer close()

	var response *request.Result
	for i := 0; i < 2; i++ {
		response = request.NewRequest(nil, requestURL).
			Post().
			Body(requestBody).
			RequestPath(requestPath).
			Do()
	}

	if response.StatusCode() != 409 {
		t.Fatalf("Unexpected Status Code: %v", response.Error())
	}
}

func TestUpdateMetadataFiles(t *testing.T) {
	var (
		namespace, name, gitSha = "prod-kim-koli", "myapp", "b4b36461355c0caf16b7deb8d33ba7dc5ba7093e"
		requestPath             = fmt.Sprintf("/releases/v1beta1/%s/%s/objects/%s", namespace, name, gitSha)
		r                       = mux.NewRouter()
		gitHandler              = NewHandler(&conf.Config{GitHome: "/tmp"}, nil, nil)
		expectedFiles           = map[string]int64{"slug.tgz": 324802, "build.log": 2940}
		expectedObj             = newGitInfo(namespace, name, gitSha, map[string]int64{"slug.tgz": expectedFiles["slug.tgz"]})
	)
	s := r.PathPrefix("/releases/v1beta1/{namespace}/{deployName}").Subrouter()
	s.HandleFunc("/objects/{gitSha}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gitHandler.V1beta1Releases(w, r)
	})).Methods("PUT").Headers("Content-Type", "application/json")
	requestURL, ts := runHttpTestServer(s, gitHandler, nil)
	defer ts.Close()
	db, close := getBoltDb(t)
	gitHandler.boltDB = db
	gitHandler.fileExists = fileExistsTrueFn
	gitHandler.user = &platform.User{Customer: "kim", Organization: "koli"}
	defer close()
	db.Update(func(tx *bolt.Tx) error {
		b, _ := tx.CreateBucketIfNotExists([]byte(namespace))
		key := []byte(filepath.Join(name, gitSha))
		data, _ := json.Marshal(expectedObj)
		return b.Put(key, data)
	})
	requestBody := &platform.GitInfo{Files: map[string]int64{"build.log": expectedFiles["build.log"]}}
	respBody := &platform.GitInfo{}
	err := request.NewRequest(nil, requestURL).
		Put().
		Body(requestBody).
		RequestPath(requestPath).
		Do().
		Into(respBody)

	if err != nil {
		t.Fatalf("Unexpected Response: %v", err)
	}
	if !reflect.DeepEqual(respBody.Files, expectedFiles) {
		t.Errorf("GOT: %#v, EXPECTED: %#v", respBody.Files, expectedObj.Files)
	}
}

func TestListReleases(t *testing.T) {
	var (
		namespace, name = "prod-kim-koli", "myapp"
		requestPath     = fmt.Sprintf("/releases/v1beta1/%s/%s/objects", namespace, name)
		r               = mux.NewRouter()
		gitHandler      = NewHandler(&conf.Config{GitHome: "/tmp"}, nil, nil)
	)
	s := r.PathPrefix("/releases/v1beta1/{namespace}/{deployName}").Subrouter()
	s.HandleFunc("/objects", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gitHandler.V1beta1ListReleases(w, r)
	})).Methods("GET")
	requestURL, ts := runHttpTestServer(s, gitHandler, nil)
	defer ts.Close()
	db, close := getBoltDb(t)
	defer close()
	LoadDbWithRandomData(db, namespace, name, 50)
	gitHandler.boltDB = db
	gitHandler.user = &platform.User{Customer: "kim", Organization: "koli"}

	infoList := platform.GitInfoList{}
	err := request.NewRequest(nil, requestURL).
		Get().
		RequestPath(requestPath).
		Do().
		Into(&infoList)
	if err != nil {
		t.Fatalf("Got unexpected error: %v", err)
	}
	if len(infoList.Items) != maxItems {
		t.Errorf("EXPECTED %d items. Found %d item(s)", maxItems, len(infoList.Items))
	}
}

func TestGetRelease(t *testing.T) {
	var (
		namespace, name, gitSha = "prod-kim-koli", "myapp", "a1b12d59152d7e2a8c387a5b736efcfda46c3eef"
		requestPath             = fmt.Sprintf("/releases/v1beta1/%s/%s/objects/%s", namespace, name, gitSha)
		r                       = mux.NewRouter()
		gitHandler              = NewHandler(&conf.Config{GitHome: "/tmp"}, nil, nil)
		expectedObj             = newGitInfo(namespace, name, gitSha, map[string]int64{"slug.tgz": 120})
	)
	s := r.PathPrefix("/releases/v1beta1/{namespace}/{deployName}").Subrouter()
	s.HandleFunc("/objects/{gitSha}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gitHandler.V1beta1Releases(w, r)
	})).Methods("GET")
	requestURL, ts := runHttpTestServer(s, gitHandler, nil)
	defer ts.Close()
	db, close := getBoltDb(t)
	defer close()
	LoadDbWithRandomData(db, namespace, name, 50)

	db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(namespace))
		if b == nil {
			return nil
		}
		key := []byte(filepath.Join(name, gitSha))
		data, _ := json.Marshal(expectedObj)
		return b.Put(key, data)
	})

	gitHandler.boltDB = db
	gitHandler.user = &platform.User{Customer: "kim", Organization: "koli"}

	got := &platform.GitInfo{}
	err := request.NewRequest(nil, requestURL).
		Get().
		RequestPath(requestPath).
		Do().
		Into(got)
	if err != nil {
		t.Fatalf("Got unexpected error: %v", err)
	}
	if !reflect.DeepEqual(expectedObj, got) {
		t.Errorf("EXPECTED %v GOT %v", expectedObj, got)
	}
}

func TestSeekReleasesByAttribute(t *testing.T) {
	var (
		namespace, name, sha = "prod-kim-koli", "myapp", "087350c7edc234fdfcd7e8836a1bb6522e641568"
		requestPath          = fmt.Sprintf("/releases/v1beta1/%s/%s/seek", namespace, name)
		r                    = mux.NewRouter()
		gitHandler           = NewHandler(&conf.Config{GitHome: "/tmp"}, nil, nil)
		expectedObj          = newGitInfo(namespace, name, sha, nil)
	)
	s := r.PathPrefix("/releases/v1beta1/{namespace}/{deployName}").Subrouter()
	s.HandleFunc("/seek", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gitHandler.V1beta1SeekReleases(w, r)
	})).Methods("GET")
	requestURL, ts := runHttpTestServer(s, gitHandler, nil)
	defer ts.Close()
	db, close := getBoltDb(t)
	defer close()
	LoadDbWithRandomData(db, namespace, name, 50)
	gitHandler.boltDB = db
	gitHandler.user = &platform.User{Customer: "kim", Organization: "koli"}

	testCases := []struct {
		attr  string
		value string
	}{
		{"source", "gogs"},
		{"kubeRef", "sb-build-v15"},
		{"status", string(v1.PodFailed)},
	}
	for _, testc := range testCases {
		db.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte(namespace))
			switch testc.attr {
			case "source":
				expectedObj.SourceType = testc.value
			case "kubeRef":
				expectedObj.KubeRef = testc.value
			case "status":
				expectedObj.Status = v1.PodPhase(testc.value)
			}
			expectedObj.HeadCommit.ID = randomShaHash()
			data, _ := json.Marshal(expectedObj)
			key := []byte(filepath.Join(name, sha))
			return b.Put(key, data)
		})
	}

	for _, testc := range testCases {
		infoList := platform.GitInfoList{}
		err := request.NewRequest(nil, requestURL).
			Get().
			RequestPath(requestPath).
			AddQuery("q", testc.value).
			AddQuery("in", testc.attr).
			Do().
			Into(&infoList)
		if err != nil {
			t.Fatalf("Got unexpected error: %v", err)
		}
		if len(infoList.Items) != 1 {
			t.Fatalf("EXPECTED 1 record, GOT: %d", len(infoList.Items))
		}
		i := infoList.Items[0]
		switch testc.attr {
		case "source":
			if i.SourceType != testc.value {
				t.Errorf("GOT: %v, EXPECTED: %v", i.SourceType, testc.value)
			}
		case "kubeRef":
			if i.KubeRef != testc.value {
				t.Errorf("GOT: %v, EXPECTED: %v", i.KubeRef, testc.value)
			}
		case "status":
			if i.Status != v1.PodPhase(testc.value) {
				t.Errorf("GOT: %v, EXPECTED: %v", i.Status, testc.value)
			}
		}
	}
}
