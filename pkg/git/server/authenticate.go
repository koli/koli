package server

import (
	"fmt"
	"net/http"

	platform "kolihub.io/koli/pkg/apis/v1alpha1"
	gitutil "kolihub.io/koli/pkg/git/util"
)

// Authenticate validates if the provided credentials are valid
func (h *Handler) Authenticate(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	_, jwtTokenString, ok := r.BasicAuth()
	if !ok {
		w.Header().Set("WWW-Authenticate", "Basic")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, "Authentication required (token null)\n")
		return
	}
	u, err := gitutil.DecodeUserToken(jwtTokenString, h.cnf.PlatformClientSecret, h.cnf.Auth0.PlatformPubKey)
	if err != nil {
		w.Header().Set("WWW-Authenticate", "Basic")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, "Authentication required (%s)\n", err)
		return
	}
	h.user = u
	// A system token is only allowed to download/upload releases,
	if u.Type == platform.SystemTokenType {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintf(w, "Access Denied! Not allowed to access the resource\n")
		return
	}
	next(w, r)
}
