package users

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"howett.net/spectre"
	"howett.net/spectre/internal/auth"
)

type Handler struct {
	UserService  spectre.UserService
	LoginService auth.LoginService
}

func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	password := r.FormValue("password")

	u, err := h.UserService.GetUserNamed(r.Context(), username)
	if err == spectre.ErrNotFound {
		//TODO(DH)
		//Error(w, r, 404)
		return
	}

	ok, err := u.TestChallenge(spectre.PassphraseMaterial(password))
	if err != nil || !ok {
		//Error(w, r, 401)
		return
	}

	h.LoginService.SetLoggedInUser(w, r, u)
	fmt.Fprintf(w, "okay")
}

func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	h.LoginService.SetLoggedInUser(w, r, nil)
}

func (h *Handler) BindRoutes(router *mux.Router) error {
	router.Path("/login").
		Methods("POST").HandlerFunc(h.handleLogin)

	router.Path("/logout").
		Methods("POST").HandlerFunc(h.handleLogout)

	return nil
}

func NewHandler(us spectre.UserService, login auth.LoginService) *Handler {
	return &Handler{
		UserService:  us,
		LoginService: login,
	}
}
