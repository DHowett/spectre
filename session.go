package main

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/golang/glog"
	"github.com/gorilla/sessions"
)

type SessionScope int

const (
	// SessionScopeServer is the session scope for all server-backed sessions.
	// Server-backed sessions are long-lived and can store any amount of data.
	SessionScopeServer SessionScope = iota

	// SessionScopeClient is the session scope for all long-term client-backed sessions.
	// Since client sessions are included in every request, please use them sparingly.
	SessionScopeClient

	// SessionScopeSensitive is the session scope for all short-term client data.
	// Such sessions are short-lived and cannot be trusted for long-term storage of data.
	// Like the client scope, it is sent in every request.
	SessionScopeSensitive
)

var scopeCookieName = map[SessionScope]string{
	SessionScopeServer:    "session",
	SessionScopeClient:    "c_session",
	SessionScopeSensitive: "authentication",
}

type SessionBroker struct {
	stores map[SessionScope]sessions.Store
}

func (b *SessionBroker) getSessionStore(scope SessionScope) (sessions.Store, bool) {
	store, ok := b.stores[scope]
	return store, ok
}

func (b *SessionBroker) Handler(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session := &Session{
			broker: b,
			writer: w,
		}
		r = r.WithContext(context.WithValue(r.Context(), b, session))
		session.request = r

		handler.ServeHTTP(w, r)
	})

}

func (b *SessionBroker) Get(r *http.Request) *Session {
	if ses, ok := r.Context().Value(b).(*Session); ok {
		return ses
	}
	return nil
}

func NewSessionBroker(stores map[SessionScope]sessions.Store) *SessionBroker {
	return &SessionBroker{
		stores: stores,
	}
}

type Session struct {
	mutex sync.RWMutex

	broker *SessionBroker

	sessions map[SessionScope]*sessions.Session

	dirty map[SessionScope]bool

	writer  http.ResponseWriter
	request *http.Request
}

func (s *Session) Save() {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	for scope, dirty := range s.dirty {
		if !dirty {
			continue
		}

		ses, ok := s.sessions[scope]
		if !ok {
			// we can get here if a session was marked dirty without being loaded.
			continue
		}
		ses.Save(s.request, s.writer)
		s.dirty[scope] = false
	}
}

func (s *Session) getGorillaSession(scope SessionScope, create bool) (*sessions.Session, error) {
	s.mutex.RLock()
	session, ok := s.sessions[scope]
	s.mutex.RUnlock()

	if !ok {
		s.mutex.Lock()
		defer s.mutex.Unlock()

		// Double-checked locking/promote
		session, ok = s.sessions[scope]
		if !ok {
			store, ok := s.broker.getSessionStore(scope)
			if !ok {
				return nil, fmt.Errorf("sessions: unknown scope %d", scope)
			}
			session, err := store.Get(s.request, scopeCookieName[scope])
			if err != nil {
				glog.Error(err)
				return nil, err
			}

			if session == nil {
				if !create {
					return nil, err
				}

				session, err = store.New(s.request, scopeCookieName[scope])
				if err != nil {
					return nil, err
				}
			}

			if s.sessions == nil {
				s.sessions = make(map[SessionScope]*sessions.Session)
			}

			s.sessions[scope] = session
		}
	}
	return session, nil
}

func (s *Session) GetOk(scope SessionScope, key string) (interface{}, bool) {
	store, err := s.getGorillaSession(scope, false)
	if err != nil {
		return nil, false
	}

	if store == nil {
		return nil, false
	}

	s.mutex.RLock()
	defer s.mutex.RUnlock()

	val, ok := store.Values[key]
	return val, ok
}

func (s *Session) Get(scope SessionScope, key string) interface{} {
	val, _ := s.GetOk(scope, key)
	return val
}

func (s *Session) Set(scope SessionScope, key string, val interface{}) {
	store, err := s.getGorillaSession(scope, true)
	if err != nil {
		glog.Errorf("failed to save <%s> into session(%d): %v", key, scope, err)
		return
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	store.Values[key] = val

	if s.dirty == nil {
		s.dirty = make(map[SessionScope]bool)
	}

	s.dirty[scope] = true
}

// MarkDirty will mark a session scope as dirty, forcing it to be saved.
// This is only necessary when the session is storing object references
// that can be updated without a call to Set.
func (s *Session) MarkDirty(scope SessionScope) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.dirty == nil {
		s.dirty = make(map[SessionScope]bool)
	}

	s.dirty[scope] = true
}

func (s *Session) Delete(scope SessionScope, key string) {
	// nocreate: deleting a nonexistent key from a nonexistent session is useless.
	store, err := s.getGorillaSession(scope, false)
	if err != nil {
		glog.Errorf("failed to delete <%s> from session(%d): %v", key, scope, err)
		return
	}

	if store == nil {
		return
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	_, dirty := store.Values[key]
	delete(store.Values, key)

	if s.dirty == nil {
		s.dirty = make(map[SessionScope]bool)
	}

	// If it didn't exist, don't dirty the session.
	s.dirty[scope] = s.dirty[scope] || dirty
}
