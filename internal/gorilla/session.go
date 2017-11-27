package gorilla

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/gorilla/sessions"
	"howett.net/spectre/internal/auth"
)

var scopeCookieName = map[auth.SessionScope]string{
	auth.SessionScopeServer:    "session",
	auth.SessionScopeClient:    "c_session",
	auth.SessionScopeSensitive: "authentication",
}

var _ auth.SessionService = &gorillaSessionService{}

type gorillaSessionService struct {
	stores map[auth.SessionScope]sessions.Store
}

func (b *gorillaSessionService) getSessionStore(scope auth.SessionScope) (sessions.Store, bool) {
	store, ok := b.stores[scope]
	return store, ok
}

func (b *gorillaSessionService) Handler(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session := &session{
			broker: b,
			writer: w,
		}
		r = r.WithContext(context.WithValue(r.Context(), b, session))
		session.request = r // r changed with the context attach

		handler.ServeHTTP(w, r)
	})

}

func (b *gorillaSessionService) SessionForRequest(r *http.Request) auth.Session {
	if ses, ok := r.Context().Value(b).(*session); ok {
		return ses
	}
	return nil
}

func NewSessionService(stores map[auth.SessionScope]sessions.Store) auth.SessionService {
	return &gorillaSessionService{
		stores: stores,
	}
}

type session struct {
	mutex sync.RWMutex

	broker *gorillaSessionService

	sessions map[auth.SessionScope]*sessions.Session

	dirty map[auth.SessionScope]bool

	writer  http.ResponseWriter
	request *http.Request
}

func (s *session) logFailure(scope auth.SessionScope, operation, key string, err error) {
	// TODO(DH) Log
	/*
		s.logger.WithFields(logrus.Fields{
			"scope":     scope,
			"key":       key,
			"operation": operation,
		}).Error(err)
	*/
}

func (s *session) Save() {
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

func (s *session) getGorillaSession(scope auth.SessionScope, create bool) (*sessions.Session, error) {
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
			var err error // Using := below will create a new `session' in scope.
			session, err = store.Get(s.request, scopeCookieName[scope])
			if err != nil {
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
				s.sessions = make(map[auth.SessionScope]*sessions.Session)
			}

			s.sessions[scope] = session
		}
	}
	return session, nil
}

func (s *session) GetOk(scope auth.SessionScope, key string) (interface{}, bool) {
	store, err := s.getGorillaSession(scope, false)
	if err != nil {
		s.logFailure(scope, key, "get", err)
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

func (s *session) Get(scope auth.SessionScope, key string) interface{} {
	val, _ := s.GetOk(scope, key)
	return val
}

func (s *session) Set(scope auth.SessionScope, key string, val interface{}) {
	store, err := s.getGorillaSession(scope, true)
	if err != nil {
		s.logFailure(scope, key, "set", err)
		return
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	store.Values[key] = val

	if s.dirty == nil {
		s.dirty = make(map[auth.SessionScope]bool)
	}

	s.dirty[scope] = true
}

// MarkDirty will mark a session scope as dirty, forcing it to be saved.
// This is only necessary when the session is storing object references
// that can be updated without a call to Set.
func (s *session) MarkDirty(scope auth.SessionScope) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.dirty == nil {
		s.dirty = make(map[auth.SessionScope]bool)
	}

	s.dirty[scope] = true
}

func (s *session) Delete(scope auth.SessionScope, key string) {
	// nocreate: deleting a nonexistent key from a nonexistent session is useless.
	store, err := s.getGorillaSession(scope, false)
	if err != nil {
		s.logFailure(scope, key, "delete", err)
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
		s.dirty = make(map[auth.SessionScope]bool)
	}

	// If it didn't exist, don't dirty the session.
	s.dirty[scope] = s.dirty[scope] || dirty
}
