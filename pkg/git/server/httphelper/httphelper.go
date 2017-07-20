package httphelper

import (
	"crypto/rand"
	"fmt"
	"io"
	"log"
	"net/http"

	"golang.org/x/net/context"
	"kolihub.io/koli/pkg/git/server/ctxhelper"
)

// Handler is an extended version of http.Handler that also takes a context
// argument ctx.
type Handler interface {
	ServeHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request)
}

// The HandlerFunc type is an adapter to allow the use of ordinary functions as
// Handlers.  If f is a function with the appropriate signature, HandlerFunc(f)
// is a Handler object that calls f.
type HandlerFunc func(context.Context, http.ResponseWriter, *http.Request)

// ServeHTTP calls f(ctx, w, r).
func (f HandlerFunc) ServeHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	f(ctx, w, r)
}

// ContextInjector .
func ContextInjector(componentName string, handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		reqID := req.Header.Get("X-Request-ID")
		if reqID == "" {
			reqID = UUID()
		}
		ctx := ctxhelper.NewContextRequestID(context.Background(), reqID)
		ctx = ctxhelper.NewContextComponentName(ctx, componentName)
		rw := NewResponseWriter(w, ctx)
		handler.ServeHTTP(rw, req)
	})
}

func contextFromResponseWriter(w http.ResponseWriter) context.Context {
	ctx := w.(*ResponseWriter).Context()
	return ctx
}

func logError(w http.ResponseWriter, err error) {
	if rw, ok := w.(*ResponseWriter); ok {
		logger, _ := ctxhelper.LoggerFromContext(rw.Context())
		logger.Error(err.Error())
	} else {
		log.Println(err)
	}
}

// Bytes .
func Bytes(n int) []byte {
	data := make([]byte, n)
	_, err := io.ReadFull(rand.Reader, data)
	if err != nil {
		panic(err)
	}
	return data
}

// UUID .
func UUID() string {
	id := Bytes(16)
	id[6] &= 0x0F // clear version
	id[6] |= 0x40 // set version to 4 (random uuid)
	id[8] &= 0x3F // clear variant
	id[8] |= 0x80 // set to IETF variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", id[0:4], id[4:6], id[6:8], id[8:10], id[10:])
}
