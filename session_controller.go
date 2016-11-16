package main

import (
	"net/http"
	"strings"

	"github.com/DHowett/ghostbin/model"
	"github.com/gorilla/mux"
)

type SessionController struct {
	App   Application
	Model model.Broker
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

func (sc *SessionController) sessionHandler(w http.ResponseWriter, r *http.Request) {
	ids := sc.getPasteIDs(r)
	sessionPastes, err := sc.Model.GetPastes(ids)
	if err != nil {
		panic(err)
	}
	templatePack.ExecutePage(w, r, "session", sessionPastes)
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

func (sc *SessionController) InitRoutes(router *mux.Router) {
	router.Path("/").HandlerFunc(sc.sessionHandler)
	router.Path("/raw").HandlerFunc(sc.sessionRawHandler)
}

func NewSessionController(app Application, modelBroker model.Broker) Controller {
	return &SessionController{
		App:   app,
		Model: modelBroker,
	}
}
