package pastes

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"howett.net/spectre"
	"howett.net/spectre/internal/auth"
	ghtime "howett.net/spectre/internal/time"
)

type Handler struct {
	PasteService      spectre.PasteService
	PermitterProvider auth.PermitterProvider
}

func (h *Handler) pasteUpdateFromRequest(r *http.Request) (*spectre.PasteUpdate, error) {
	body := r.FormValue("text")
	language := r.FormValue("lang")
	expireIn := r.FormValue("expire")
	title := r.FormValue("title")
	password := r.FormValue("password")

	l := len(body)
	if l == 0 || l > 1048576 /* TODO(DH) limit */ {
		return nil, errors.New("bah")
	}

	pu := spectre.PasteUpdate{}
	if body != "" {
		pu.Body = &body
	}

	if language != "" {
		pu.LanguageName = &language
	}

	if expireIn == "-1" {
		pu.ExpirationTime = spectre.ExpirationTimeNever
	} else if expireIn != "" {
		dur, _ := ghtime.ParseDuration(expireIn)
		/* TODO(DH)
		if dur > time.Duration(pc.Config.Application.Limits.PasteMaxExpiration) {
			dur = time.Duration(pc.Config.Application.Limits.PasteMaxExpiration)
		}
		*/
		expireAt := time.Now().Add(dur)
		pu.ExpirationTime = &expireAt
	}

	if title != "" {
		pu.Title = &title
	}

	if password != "" {
		pu.PassphraseMaterial = []byte(password)
	}

	return &pu, nil
}

func (h *Handler) getPasteFromRequest(r *http.Request) (spectre.Paste, error) {
	id, ok := mux.Vars(r)["id"]
	if !ok {
		return nil, spectre.ErrNotFound
	}
	return h.PasteService.GetPaste(r.Context(), nil, spectre.PasteID(id))
}

func (h *Handler) updatePasteC(p spectre.Paste, w http.ResponseWriter) {
	//Redirect(w, http.StatusSeeOther, fmt.Sprintf("/paste/%v", p.GetID()))
}

func (h *Handler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	p, err := h.getPasteFromRequest(r)
	if err != nil {
		// TODO(DH) errors!
		return
	}

	pu, err := h.pasteUpdateFromRequest(r)
	if err != nil {
		// TODO(DH) errors!
		return
	}

	permitter := h.PermitterProvider.GetPermitterForRequest(r)
	perms := permitter.Permissions(spectre.PermissionClassPaste, p.GetID())
	if !perms.Has(spectre.PastePermissionEdit) {
		// TODO(DH) bounce'em
		return
	}

	err = p.Update(*pu)
	if err != nil {
		// TODO(DH) bounce'em
		panic(err)
	}

	h.updatePasteC(p, w)
}

func (h *Handler) handleNew(w http.ResponseWriter, r *http.Request) {
	pu, err := h.pasteUpdateFromRequest(r)
	if err != nil {
		// TODO(DH) errors!
		return
	}

	p, err := h.PasteService.CreatePaste(r.Context(), pu)
	if err != nil {
		// TODO(DH) err
		return
	}

	permitter := h.PermitterProvider.GetPermitterForRequest(r)
	perms := permitter.Permissions(spectre.PermissionClassPaste, p.GetID())
	perms.Grant(spectre.PastePermissionAll)

	h.updatePasteC(p, w)
}

func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request) {
	p, err := h.getPasteFromRequest(r)
	if err != nil {
		// TODO(DH) errors!
		return
	}

	permitter := h.PermitterProvider.GetPermitterForRequest(r)
	perms := permitter.Permissions(spectre.PermissionClassPaste, p.GetID())
	if !perms.Has(spectre.PastePermissionEdit) {
		// TODO(DH) bounce'em
		return
	}

	h.PasteService.DestroyPaste(r.Context(), p.GetID())
}

func (h *Handler) handleShow(w http.ResponseWriter, r *http.Request) {
	p, err := h.getPasteFromRequest(r)
	var b string
	var perm bool
	if p != nil {
		rdr, _ := p.Reader()
		if rdr != nil {
			bs, _ := ioutil.ReadAll(rdr)
			b = string(bs)
		}
		perm = h.PermitterProvider.GetPermitterForRequest(r).Permissions(spectre.PermissionClassPaste, p.GetID()).Has(spectre.PastePermissionEdit)
	}
	enc := json.NewEncoder(w)
	enc.Encode(map[string]interface{}{
		"paste":     p,
		"body":      b,
		"permitted": perm,
		"err":       fmt.Sprintf("%v", err),
	})
}

func (h *Handler) BindRoutes(router *mux.Router) error {
	router.Path("/").
		Methods("POST").HandlerFunc(h.handleNew).
		Methods("GET").HandlerFunc(nil)

	router.Path("/{id}").
		Methods("POST", "PUT").HandlerFunc(h.handleUpdate).
		Methods("GET").HandlerFunc(h.handleShow)

	router.Path("/{id}/delete").
		Methods("POST").HandlerFunc(h.handleDelete)

	return nil
}

func NewHandler(ps spectre.PasteService, perm auth.PermitterProvider) *Handler {
	return &Handler{
		PasteService:      ps,
		PermitterProvider: perm,
	}
}
