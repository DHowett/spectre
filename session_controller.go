package main

import (
	"context"
	"net/http"
	"strings"

	"github.com/DHowett/ghostbin/model"
	"github.com/DHowett/ghostbin/views"
	"github.com/gorilla/mux"
)

type SessionController struct {
	App   Application
	Model model.Broker

	sessionView *views.View
}

func (sc *SessionController) getPasteIDs(r *http.Request) []model.PasteID {
	var ids []model.PasteID

	// Assumption: due to the migration handler wrapper, a logged-in session will
	// never have v3 perms and user perms.
	user := GetLoggedInUser(r)
	if user != nil {
		// TODO(DH) Encrypted pastes are no longer visible...
		uPastes, err := user.GetPastes()
		if err == nil {
			ids = uPastes
		}
	} else {
		// Failed lookup is non-fatal here.
		session := sessionBroker.Get(r)
		v3EntriesI := session.Get(SessionScopeServer, "v3permissions")
		v3Perms, _ := v3EntriesI.(map[model.PasteID]model.Permission)

		ids = make([]model.PasteID, len(v3Perms))
		n := 0
		for pid, _ := range v3Perms {
			ids[n] = pid
			n++
		}
	}
	return ids
}

type sessionPastesContextKeyType int

const sessionPastesContextKey sessionPastesContextKeyType = 0

func (sc *SessionController) sessionHandlerWrapper(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ids := sc.getPasteIDs(r)
		sessionPastes, err := sc.Model.GetPastes(ids)
		if err != nil {
			// TODO(DH) no panic
			panic(err)
		}

		r = r.WithContext(context.WithValue(r.Context(), sessionPastesContextKey, sessionPastes))
		handler.ServeHTTP(w, r)
	})
}

func (sc *SessionController) sessionRawHandler(w http.ResponseWriter, r *http.Request) {
	ids := sc.getPasteIDs(r)
	stringIDs := make([]string, len(ids))
	for i, v := range ids {
		stringIDs[i] = v.String()
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(strings.Join(stringIDs, " ")))
}

func (sc *SessionController) ViewValue(r *http.Request, name string) interface{} {
	if r == nil {
		return nil
	}

	if name == "pastes" {
		return r.Context().Value(sessionPastesContextKey)
	}
	return nil
}

func (sc *SessionController) InitRoutes(router *mux.Router) {
	router.Path("/").Handler(sc.sessionHandlerWrapper(sc.sessionView))
	router.Path("/raw").HandlerFunc(sc.sessionRawHandler)
}

func (sc *SessionController) BindViews(viewModel *views.Model) error {
	var err error
	sc.sessionView, err = viewModel.Bind(views.PageID("session"), sc)
	return err
}

func NewSessionController(app Application, modelBroker model.Broker) Controller {
	return &SessionController{
		App:   app,
		Model: modelBroker,
	}
}
