package web

import "github.com/gorilla/mux"

type Routable interface {
	BindRoutes(*mux.Router) error
}
