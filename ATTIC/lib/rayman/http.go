package rayman

import (
	"net/http"
)

func RequestWithRay(r *http.Request) *http.Request {
	return r.WithContext(ContextWithRay(r.Context()))
}

func FromRequest(r *http.Request) (ID, bool) {
	return FromContext(r.Context())
}

func Handler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = RequestWithRay(r)
		h.ServeHTTP(w, r)
	})
}
