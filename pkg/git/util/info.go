package util

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"sync"
)

type info struct {
	ObjectMeta

	GitServerURL *gitURL     // The git server address
	GitAPIURL    *releaseURL // The address of the git api
	GitHome      string      // The base path of the stored files
	GitRevision  string      // Git SHA revision
	Token        string
}

// ServerTask allows starting administrative tasks on git repositories
type ServerTask interface {
	ObjectMeta

	BaseReleasePath() string
	FullReleasePath() string
	BaseRepoPath() string
	FullRepoPath() string

	InitRepository() (bool, error)
	InitRelease(revision string) (bool, error)
	RemoveBranchRef(refName string) error
	WriteBranchRef(refPath, rev string) error
}

// APIInfo exposes information about the git API
type APIInfo interface {
	ObjectMeta

	ReleaseURL() *releaseURL
}

// ServerInfo exposes information about the git server
type ServerInfo interface {
	ObjectMeta

	GetCloneURL() *gitURL
}

// NewServerTask creates a new ServerTask
func NewServerTask(gitHome string, meta ObjectMeta) ServerTask {
	return &info{GitHome: gitHome, ObjectMeta: meta}
}

// NewAPIInfo creates a new APIInfo
func NewAPIInfo(gitAPIHost string, meta ObjectMeta) APIInfo {
	return &info{GitAPIURL: &releaseURL{addr: gitAPIHost}, ObjectMeta: meta}
}

// NewServerInfo creates a new ServerInfo
func NewServerInfo(gitServerAddr string, meta ObjectMeta) (ServerInfo, error) {
	u, err := url.Parse(gitServerAddr)
	if err != nil {
		return nil, err
	}
	gURL := &gitURL{
		addr:  u,
		user:  meta.GetAuthUser(),
		token: meta.GetAuthToken(),
		repo:  meta.GetRepository(),
	}
	return &info{GitServerURL: gURL, ObjectMeta: meta}, nil
}

// BaseRepoPath returns the absolute path for the base repository on the server
func (i *info) BaseRepoPath() string {
	return filepath.Join(i.GitHome, repoPrefix)
}

// BaseReleasePath returns the absolute path for the base release folder on the server.
func (i *info) BaseReleasePath() string {
	return filepath.Join(i.GitHome, releasePrefix)
}

// GetRepository returns the full repository name. e.g.: owner/repo
func (i *info) GetRepository() string {
	return i.ObjectMeta.GetRepository()
}

// GetFullRepoPath returns the full absolute path of the repository
func (i *info) FullRepoPath() string {
	return filepath.Join(i.BaseRepoPath(), i.GetRepository())
}

// FullReleasePath returns the full absolute path of the repository
func (i *info) FullReleasePath() string {
	return filepath.Join(i.BaseReleasePath(), i.GetRepository())
}

// InitRepoPath initializes a new git bare repository
func (i *info) InitRepository() (bool, error) {
	return createRepo(i.FullRepoPath(), true, &sync.Mutex{})
}

// InitRelease initializes the repository folder where the releases are going to be stored
func (i *info) InitRelease(revision string) (bool, error) {
	return createRepo(filepath.Join(i.FullReleasePath(), revision), false, &sync.Mutex{})
}

// RemoveBranchRef exclude the target refName in a git repository
// https://git-scm.com/docs/githooks#update
func (i *info) RemoveBranchRef(refPath string) error {
	return os.RemoveAll(filepath.Join(i.FullRepoPath(), refPath))
}

// WriteBranchRef write a revision in the refPath
func (i *info) WriteBranchRef(refPath, rev string) error {
	p := filepath.Join(i.FullRepoPath(), refPath)
	return ioutil.WriteFile(p, []byte(fmt.Sprintf("%s\n", rev)), 0644)
}

// ReleaseURL returns the URL for the given repository,
// if GitRevision is set it will be used at end of URL
func (i *info) ReleaseURL() *releaseURL {
	pathURL := filepath.Join(releasePrefix, i.GetRepository(), i.GitRevision)
	return &releaseURL{addr: fmt.Sprintf("%s/%s", i.GitAPIURL, pathURL)}
}

// GetCloneURL retrieves the url for cloning a repository
func (i *info) GetCloneURL() *gitURL {
	return i.GitServerURL
}
