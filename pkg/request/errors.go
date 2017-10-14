package request

import (
	"fmt"
	"net/http"
)

type HTTPError struct {
	statusCode int
	message    string
}

func (h *HTTPError) Error() string {
	return fmt.Sprintf("[%d] %s", h.statusCode, h.message)
}

func NewHTTPError(statusCode int, message string, format ...interface{}) *HTTPError {
	return &HTTPError{statusCode: statusCode, message: fmt.Sprintf(message, format)}
}

func IsNotFound(err error) bool {
	switch t := err.(type) {
	case *HTTPError:
		return t.statusCode == http.StatusNotFound
	}
	return false
}
