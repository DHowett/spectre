package http

import (
	"context"
	"net/http"
	"sync"

	"howett.net/spectre"
)

type contextBindingUserService struct {
	http.Handler
	UserService
}

type lateUser struct {
	o sync.Once
	u spectre.User
}

func (s *contextBindingUserService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r = r.WithContext(context.WithValue(r.Context(), s, &lateUser{}))
	s.Handler.ServeHTTP(w, r)
}

func (s *contextBindingUserService) GetUserForRequest(r *http.Request) spectre.User {
	lu := r.Context().Value(s).(*lateUser)
	lu.o.Do(func() {
		lu.u = s.UserService.GetUserForRequest(r)
	})
	return lu.u
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
