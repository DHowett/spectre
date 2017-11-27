package ctxbound

import (
	"context"
	"net/http"
	"sync"

	"howett.net/spectre"
	"howett.net/spectre/internal/auth"
)

type LoginService struct {
	auth.LoginService
}

type lateUser struct {
	o sync.Once
	u spectre.User
}

func (s *LoginService) Middleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(context.WithValue(r.Context(), s, &lateUser{}))
		h.ServeHTTP(w, r)
	})
}

func (s *LoginService) GetLoggedInUser(r *http.Request) spectre.User {
	lu := r.Context().Value(s).(*lateUser)
	lu.o.Do(func() {
		lu.u = s.LoginService.GetLoggedInUser(r)
	})
	return lu.u
}

func (s *LoginService) SetLoggedInUser(w http.ResponseWriter, r *http.Request, u spectre.User) {
	lu := r.Context().Value(s).(*lateUser)
	lu.u = u
	s.LoginService.SetLoggedInUser(w, r, u)
}
