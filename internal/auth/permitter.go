package auth

import (
	"net/http"

	"howett.net/spectre"
)

type PermitterProvider interface {
	GetPermitterForRequest(r *http.Request) spectre.Permitter
}
