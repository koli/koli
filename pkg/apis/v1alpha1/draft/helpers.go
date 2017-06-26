package draft

import (
	"regexp"

	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

// this constant represents the length of a shortened git sha - 8 characters long
const shortShaIdx = 8

var shaRegex = regexp.MustCompile(`^[\da-f]{40}$`)

// NewSha creates a raw string to a SHA. Returns ErrInvalidGitSha if the sha was invalid.
func NewSha(rawSha string) (*SHA, error) {
	if !shaRegex.MatchString(rawSha) {
		return nil, ErrInvalidGitSha{sha: rawSha}
	}
	return &SHA{full: rawSha, short: rawSha[0:shortShaIdx]}, nil
}

// NewDeployment generates a new draft.Deployment
func NewDeployment(obj *v1beta1.Deployment) *Deployment {
	d := &Deployment{Deployment: *obj}
	d.DraftMeta = DraftMeta{objectMeta: &d.ObjectMeta}
	return d
}
