package httphelper

import (
	"bufio"
	"fmt"
	"net"
	"net/http"

	"golang.org/x/net/context"
)

// NewResponseWriter .
func NewResponseWriter(w http.ResponseWriter, ctx context.Context) *ResponseWriter {
	return &ResponseWriter{w: w, ctx: ctx}
}

// ResponseWriter .
type ResponseWriter struct {
	ctx    context.Context
	w      http.ResponseWriter
	status int
}

// Context .
func (r *ResponseWriter) Context() context.Context {
	return r.ctx
}

// Status .
func (r *ResponseWriter) Status() int {
	return r.status
}

// WriteHeader .
func (r *ResponseWriter) WriteHeader(s int) {
	r.w.WriteHeader(s)
	r.status = s
}

// Header .
func (r *ResponseWriter) Header() http.Header {
	return r.w.Header()
}

// Write .
func (r *ResponseWriter) Write(b []byte) (int, error) {
	return r.w.Write(b)
}

// Written .
func (r *ResponseWriter) Written() bool {
	return r.status != 0
}

// CloseNotify .
func (r *ResponseWriter) CloseNotify() <-chan bool {
	return r.w.(http.CloseNotifier).CloseNotify()
}

// Flush .
func (r *ResponseWriter) Flush() {
	flusher, ok := r.w.(http.Flusher)
	if ok {
		flusher.Flush()
	}
}

// Hijack .
func (r *ResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := r.w.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("the ResponseWriter doesn't support the Hijacker interface")
	}
	return hijacker.Hijack()
}
