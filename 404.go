package main

import (
	"net/http"
)

type fourOhFourConsumerWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *fourOhFourConsumerWriter) WriteHeader(status int) {
	w.statusCode = status
	if status == http.StatusNotFound {
		w.ResponseWriter.Header().Set("Content-Type", "text/html; charset=utf-8")
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *fourOhFourConsumerWriter) Write(p []byte) (int, error) {
	if w.statusCode == http.StatusNotFound {
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
	if writer.statusCode == http.StatusNotFound {
		ExecuteTemplate(writer.ResponseWriter, "page_404", &RenderContext{nil, r})
	}
}

