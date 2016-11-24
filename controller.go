package main

import (
	"github.com/DHowett/ghostbin/views"
	"github.com/gorilla/mux"
)

type Controller interface {
	InitRoutes(*mux.Router)
	BindViews(*views.Model) error
}
