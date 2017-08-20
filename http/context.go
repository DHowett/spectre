package http

import (
	"context"
	"net/http"
	"sync"

	"howett.net/spectre"
)

type contextBindingLoginService struct {
	http.Handler
	LoginService
}

type lateUser struct {
	o sync.Once
	u spectre.User
}

func (s *contextBindingLoginService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r = r.WithContext(context.WithValue(r.Context(), s, &lateUser{}))
	s.Handler.ServeHTTP(w, r)
}

func (s *contextBindingLoginService) GetLoggedInUser(r *http.Request) spectre.User {
	lu := r.Context().Value(s).(*lateUser)
	lu.o.Do(func() {
		lu.u = s.LoginService.GetLoggedInUser(r)
	})
	return lu.u
}

func (s *contextBindingLoginService) SetLoggedInUser(w http.ResponseWriter, r *http.Request, u spectre.User) {
	lu := r.Context().Value(s).(*lateUser)
	lu.u = u
	s.LoginService.SetLoggedInUser(w, r, u)
}

type contextBindingPermitterProvider struct {
	http.Handler
	PermitterProvider
}

type latePermitter struct {
	o sync.Once
	u spectre.Permitter
}

func (s *contextBindingPermitterProvider) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r = r.WithContext(context.WithValue(r.Context(), s, &latePermitter{}))
	s.Handler.ServeHTTP(w, r)
}

func (s *contextBindingPermitterProvider) GetPermitterForRequest(r *http.Request) spectre.Permitter {
	lu := r.Context().Value(s).(*latePermitter)
	lu.o.Do(func() {
		lu.u = s.PermitterProvider.GetPermitterForRequest(r)
	})
	return lu.u
}
