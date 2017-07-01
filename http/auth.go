package http

import (
	"net/http"

	"howett.net/spectre"
)

// TODO(DH) This file should be split up and have its comments cleaned.

// Thing that takes a web request and transforms it into a permitter, perhaps?
// need httputil.sessionbinder maybe? something that takes a sessionservice
// and binds it to a web request

type UserService interface {
	GetUserForRequest(r *http.Request) spectre.User
}

type PermitterProvider interface {
	// TODO(DH): Plan is as follows
	// Sessionbinder binds a SessionService's session
	// to a request. GetPermitter returns a Permitter that
	// falls back on Session, Paste & User permitters.
	GetPermitterForRequest(r *http.Request) spectre.Permitter
}

type ChallengerProvider interface {
	GetChallengerForRequest(r *http.Request) spectre.Challenger
}

type PasteCryptorProvider interface {
	GetCryptorForRequest(r *http.Request, p spectre.Paste) spectre.Cryptor
}
