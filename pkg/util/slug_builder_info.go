package util

import "path/filepath"

// SlugBuilderInfo contains all of the object storage related
// information needed to pass to a slug builder.
type SlugBuilderInfo struct {
	pushKey string
	tarKey  string
}

// NewSlugBuilderInfo creates and populates a new SlugBuilderInfo based on the given data
func NewSlugBuilderInfo(namespace, deployName, prefix string, sha *SHA) *SlugBuilderInfo {
	// [namespace]-[customer]-[org]/[deployname]/[prefix]/[full-git-rev]/slug.tgz
	basePath := filepath.Join(namespace, deployName, prefix, sha.Full())
	tarKey := filepath.Join(basePath, "slug.tgz")
	// this is where the deployer controller tells slugrunner to download the slug from,
	// so we have to tell slugbuilder to upload it to here
	pushKey := basePath
	return &SlugBuilderInfo{
		pushKey: pushKey,
		tarKey:  tarKey,
	}
}

// PushKey returns the object storage key that the slug builder will store the slug in.
// The returned value only contains the path to the folder, not including the final filename.
func (s SlugBuilderInfo) PushKey() string {
	return s.pushKey
}

// TarKey returns the object storage key from which the slug builder will download for the tarball
// (from which it uses to build the slug). The returned value only contains the path to the
// folder, not including the final filename.
func (s SlugBuilderInfo) TarKey() string {
	return s.tarKey
}
