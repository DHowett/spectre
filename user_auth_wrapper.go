package main

import (
	"context"
	"net/http"
	"sync"

	"github.com/DHowett/ghostbin/model"
)

type lateUser struct {
	m sync.RWMutex

	s    *Session
	b    model.Provider
	u    model.User
	done bool
}

func (l *lateUser) GetUser() model.User {
	l.m.RLock()
	user, done := l.u, l.done
	l.m.RUnlock()

	if !done {
		l.m.Lock()
		defer l.m.Unlock()

		// Double-checked
		user = l.u
		done = l.done
		if !done {
			l.done = true
			if uID, ok := l.s.Get(SessionScopeClient, "acct_id").(uint); ok {
				// ignoring error: l.u will be nil if this lookup fails.
				user, _ = l.b.GetUserByID(uID)
				l.u = user
			}
		}
	}

	return user
}

func (l *lateUser) SetUser(u model.User) {
	l.m.Lock()
	defer l.m.Unlock()

	l.u = u
	l.done = true
	if u != nil {
		l.s.Set(SessionScopeClient, "acct_id", l.u.GetID())
	} else {
		l.s.Delete(SessionScopeClient, "acct_id")
	}
	l.s.Save()
}

var userContextKey int = 1

// UserLookupHandler wraps and returns a http.Handler, providing a request context bound
// login manager.
func UserLookupHandler(broker model.Provider, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lu := &lateUser{
			s: sessionBroker.Get(r),
			b: broker,
		}
		r = r.WithContext(context.WithValue(r.Context(), &userContextKey, lu))
		h.ServeHTTP(w, r)
	})
}

// GetLoggedInUser will retrieve the logged in user (if one exists) from a context-wrapped http.Request.
// On the first call to GetLoggedInUser, the request's session cookie will be queried for its `acct_id` value.
func GetLoggedInUser(r *http.Request) model.User {
	if lu, ok := r.Context().Value(&userContextKey).(*lateUser); ok {
		return lu.GetUser()
	}
	return nil
}

// SetLoggedInUser will replace the logged in user on a context-wrapped http.Request and persist the user's
// authentication. The request's session cookie's `acct_id` value will be updated with the user ID.
func SetLoggedInUser(r *http.Request, u model.User) {
	if lu, ok := r.Context().Value(&userContextKey).(*lateUser); ok {
		lu.SetUser(u)
	}
}
