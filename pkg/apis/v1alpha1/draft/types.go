package draft

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

// DraftMeta contains common helper methods for all draft objects
type DraftMeta struct {
	objectMeta *metav1.ObjectMeta
}

// deployment it's a draft for composing and acessing
// platform attributes from a v1beta1.Deployment more easily
type Deployment struct {
	DraftMeta
	v1beta1.Deployment
}

// SHA is the representaton of a git sha
type SHA struct {
	full  string
	short string
}

// ErrInvalidGitSha is returned by NewSha if the given raw sha is invalid for any reason.
type ErrInvalidGitSha struct {
	sha string
}
