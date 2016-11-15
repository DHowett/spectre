package main

import "github.com/gorilla/mux"

type Controller interface {
	InitRoutes(*mux.Router)
}
