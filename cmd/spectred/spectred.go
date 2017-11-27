package main

import (
	"net/http"

	"github.com/gorilla/mux"
	"howett.net/spectre/web/pastes"
)

func main() {
	us := &mockUserService{}
	ps := &mockPasteService{}
	perm := &loggingPermitter{}
	login := &mockLoginService{}
	_, _ = us, login

	router := mux.NewRouter()
	pasteHandler := pastes.NewHandler(ps, perm)
	pasteHandler.BindRoutes("/pastes", router.Path("/pastes").Subrouter())

	http.ListenAndServe(":8080", router)
}
