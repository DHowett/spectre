package main

import "github.com/gorilla/mux"

type Controller interface {
	InitRoutes(*mux.Router)
}

type ControllerRoute struct {
	PathPrefix string
	Controller Controller
}
