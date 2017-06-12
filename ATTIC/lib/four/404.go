// Package four provides an http.Handler that consumes "404 page not found" pages from
// go's default net/http.Error implementation and turns them into something useful.
package four

import (
	"bytes"
	"net/http"
)

type fourOhFourConsumerWriter struct {
	http.ResponseWriter
	statusCode int
	tripped    bool
}

func (w *fourOhFourConsumerWriter) WriteHeader(status int) {
	w.statusCode = status
	if status == http.StatusNotFound {
		w.ResponseWriter.Header().Set("Content-Type", "text/html; charset=utf-8")
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *fourOhFourConsumerWriter) Write(p []byte) (int, error) {
	if w.statusCode == http.StatusNotFound && bytes.Equal(p, []byte("404 page not found\n")) {
		w.tripped = true
		return len(p), nil
	}
	return w.ResponseWriter.Write(p)
}

type fourOhFourConsumerHandler struct {
	http.Handler
	errorHandler http.Handler
}

func (h *fourOhFourConsumerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	writer := &fourOhFourConsumerWriter{ResponseWriter: w}
	h.Handler.ServeHTTP(writer, r)
	if writer.tripped {
		h.errorHandler.ServeHTTP(w, r)
	}
}

// WrapHandler returns a new http.Handler that invokes errorHandler when orig would
// have rendered a 404.
func WrapHandler(orig http.Handler, errorHandler http.Handler) http.Handler {
	return &fourOhFourConsumerHandler{orig, errorHandler}
}
