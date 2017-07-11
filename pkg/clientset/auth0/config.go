package auth0

import (
	"time"

	"kolihub.io/koli/pkg/request"
)

type Config struct {
	Host   string
	Prefix string

	// Server required Basic authentication
	UserName string
	Password string

	// Server requires Bearer authentication.
	BearerToken string

	// The maximum length of time to wait before giving up on a server request. A value of zero means no timeout.
	Timeout time.Duration

	// Set specific behavior of the client.  If not set http.DefaultClient will be used.
	Client request.HTTPClient
}
