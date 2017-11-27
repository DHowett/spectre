package ctxbound

import (
	"context"
	"net/http"
	"sync"

	"howett.net/spectre"
	"howett.net/spectre/internal/auth"
)

type PermitterProvider struct {
	auth.PermitterProvider
}

type latePermitter struct {
	o sync.Once
	u spectre.Permitter
}

func (s *LoginService) Middleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(context.WithValue(r.Context(), s, &latePermitter{}))
		h.ServeHTTP(w, r)
	})
}

func (s *PermitterProvider) GetPermitterForRequest(r *http.Request) spectre.Permitter {
	lu := r.Context().Value(s).(*latePermitter)
	lu.o.Do(func() {
		lu.u = s.PermitterProvider.GetPermitterForRequest(r)
	})
	return lu.u
}
