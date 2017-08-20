package http

import (
	"context"
	"net/http"
	"path"
	"strings"
)

type prefixKeyType int

var prefixRestKey prefixKeyType

type PrefixHandler struct {
	Prefix string
	http.Handler
}

func (h *PrefixHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	clean := path.Clean(r.URL.Path)
	if strings.HasPrefix(clean, h.Prefix) {
		rest := clean[len(h.Prefix):]
		ctx := context.WithValue(r.Context(), prefixRestKey, rest)
		r = r.WithContext(ctx)
		h.Handler.ServeHTTP(w, r)
	}

	Error(w, r, 500)
	// TODO(DH) error
}

func PathSuffix(r *http.Request) string {
	v, _ := r.Context().Value(prefixRestKey).(string)
	return v
}

func Redirect(w http.ResponseWriter, status int, urlString string) {
	w.Header().Set("Location", urlString)
	w.WriteHeader(status)
}

func Error(w http.ResponseWriter, r *http.Request, status int) {
	// TODO(DH) this
}
