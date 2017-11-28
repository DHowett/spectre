package main

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/justinas/alice"
	"howett.net/spectre/api/v0/users"
	"howett.net/spectre/internal/ctxbound"
	"howett.net/spectre/web/pastes"
)

func subrouter(r *mux.Router, prefix string) *mux.Router {
	n := mux.NewRouter()
	r.PathPrefix(prefix).Handler(http.StripPrefix(prefix, n))
	return n
}

func main() {
	us := &mockUserService{}
	ps := &mockPasteService{}
	perm := &ctxbound.PermitterProvider{&loggingPermitter{}}
	login := &ctxbound.LoginService{&mockLoginService{us}}
	_, _ = us, login

	router := mux.NewRouter()
	pasteRouter := subrouter(router, "/paste")
	v0Router := subrouter(router, "/api/v0")
	usersRouter := subrouter(v0Router, "/user")

	pasteHandler := pastes.NewHandler(ps, perm, &jsonRenderer{})
	pasteHandler.BindRoutes(pasteRouter)

	userHandler := users.NewHandler(us, login)
	userHandler.BindRoutes(usersRouter)

	defaultStack := alice.New(perm.Middleware, login.Middleware)

	http.ListenAndServe(":8080", defaultStack.Then(router))
}
