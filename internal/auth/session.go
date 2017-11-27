package auth

import "net/http"

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

type Session interface {
	GetOk(scope SessionScope, key string) (interface{}, bool)
	Get(scope SessionScope, key string) interface{}

	Set(scope SessionScope, key string, val interface{})

	Delete(scope SessionScope, key string)

	// MarkDirty will mark a session scope as dirty, forcing it to be saved.
	// This is only necessary when the session is storing object references
	// that can be updated without a call to Set.
	MarkDirty(scope SessionScope)

	Save()
}

type SessionService interface {
	SessionForRequest(r *http.Request) Session
}
