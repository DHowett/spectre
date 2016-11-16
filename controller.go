package main

import "github.com/gorilla/mux"

type Controller interface {
	InitRoutes(*mux.Router)
}

type RoutedController struct {
	PathPrefix string
	Controller Controller
}
