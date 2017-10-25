package mutator

import (
	"fmt"
	"net/http"
	"regexp"

	"github.com/golang/glog"
	"kolihub.io/koli/pkg/apis/core/v1alpha1/draft"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"kolihub.io/koli/pkg/util"
)

var regexpNamespace = regexp.MustCompile("/namespaces/[a-z0-9]([-a-z0-9]*[a-z0-9])?")

// Authorize it's a middleware to process jwt token authorizations
func (h *Handler) Authorize(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	w.Header().Add("X-Koli-Origin", "K8S-Mutator")
	if r.Method == http.MethodHead {
		next(w, r)
	}
	if errStatus := h.validateUser(r); errStatus != nil {
		glog.Infof(errStatus.Message)
		util.WriteResponseError(w, errStatus)
		return
	}
	// TODO: It could be solved more cleanly: https://github.com/urfave/negroni/issues/123
	nsGroup := regexpNamespace.FindStringSubmatch(r.URL.Path)
	if len(nsGroup) == 2 {
		ns := nsGroup[1]
		if err := h.validateNamespace(draft.NewNamespaceMetadata(ns)); err != nil {
			glog.Infof(err.Message)
			util.WriteResponseError(w, err)
			return
		}
	} else {
		msg := "isn't a namespaced request"
		glog.V(3).Info(msg)
		util.WriteResponseError(w, util.StatusNotFound(msg, &v1.Namespace{}))
		return
	}
	next(w, r)
}

// validateNamespace check if the claims in the token match with the requested namespace
func (h *Handler) validateNamespace(nsMeta *draft.NamespaceMeta) *metav1.Status {
	if !nsMeta.IsValid() {
		return util.StatusForbidden(
			fmt.Sprintf("it's not a valid namespace [%#v]", nsMeta),
			&v1.Namespace{},
			metav1.StatusReasonForbidden,
		)
	}
	if nsMeta.Customer() != h.user.Customer || nsMeta.Organization() != h.user.Organization {
		msg := "Identity mismatch: the user belongs to the customer '%s' and organization '%s', " +
			"but the request was sent to the customer '%s' and organization '%s'."
		return util.StatusForbidden(
			fmt.Sprintf(
				msg,
				h.user.Customer,
				h.user.Organization,
				nsMeta.Customer(),
				nsMeta.Organization(),
			),
			&v1.Namespace{},
			metav1.StatusReasonForbidden,
		)
	}
	return nil
}

func (h *Handler) validateUser(r *http.Request) *metav1.Status {
	// validate only RSA tokens
	user, _, err := decodeJwtToken(r.Header, h.config.PlatformPubKey)
	if err != nil {
		msg := fmt.Sprintf("failed decoding token [%s]", err)
		return util.StatusUnauthorized(msg, nil, metav1.StatusReasonUnauthorized)
	}
	if !user.IsValid() {
		return util.StatusUnauthorized("it's not a valid user", nil, metav1.StatusReasonUnauthorized)
	}
	h.user = user
	return nil
}
