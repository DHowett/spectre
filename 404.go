package main

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
}

func (h *fourOhFourConsumerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	writer := &fourOhFourConsumerWriter{ResponseWriter: w}
	h.Handler.ServeHTTP(writer, r)
	if writer.tripped {
		RenderPage(writer.ResponseWriter, r, "404", nil)
	}
}
