package api

import (
	"fmt"
	"net/http"

	gitutil "kolihub.io/koli/pkg/git/util"
)

// Authorize validates if the provided credentials are valid
func (h *Handler) Authorize(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
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
	next(w, r)
}
