package mutator

import (
	"fmt"
	"net/http"

	"github.com/golang/glog"
	"k8s.io/api/core/v1"
	"kolihub.io/koli/pkg/util"
)

// Authorize it's a middleware to process jwt token authorizations
func (h *Handler) Authorize(hd http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("X-Koli-Origin", "K8S-Mutator")
		user, rawToken, err := decodeJwtToken(r.Header)
		if err != nil {
			msg := fmt.Sprintf("failed decoding token [%s]", err)
			glog.Infof(msg)
			util.WriteResponseError(w, util.StatusUnauthorized(msg, &v1.Namespace{}, "v1"))
			return
		}
		if !user.IsValid() {
			msg := fmt.Sprintf("it's not a valid user, [%s]", rawToken)
			glog.Infof(msg)
			util.WriteResponseError(w, util.StatusUnauthorized(msg, &v1.Namespace{}, "v1"))
			return
		}
		h.user = user
		h.usrClientset, h.usrTprClient, err = getKubernetesUserClients(h.config, rawToken)
		if err != nil {
			// TODO: need to pass a generic object instead of v1.Namespace
			msg := fmt.Sprintf("failed retrieving user k8s clients [%v]", err)
			glog.Infof(msg)
			util.WriteResponseError(w, util.StatusInternalError(msg, &v1.Namespace{}))
			return
		}
		hd.ServeHTTP(w, r)
	})
}
