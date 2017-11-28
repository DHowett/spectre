package pastes

import (
	"errors"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"howett.net/spectre"
	"howett.net/spectre/internal/auth"
	ghtime "howett.net/spectre/internal/time"
	"howett.net/spectre/web"
)

type Handler struct {
	PasteService      spectre.PasteService
	PermitterProvider auth.PermitterProvider
	Renderer          web.Renderer
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

func (h *Handler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	p, err := h.getPasteFromRequest(r)
	if err != nil {
		h.Renderer.Error(w, r, err)
		return
	}

	pu, err := h.pasteUpdateFromRequest(r)
	if err != nil {
		h.Renderer.Error(w, r, err)
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

	h.Renderer.Render(w, r, 200, &PasteUpdateComplete{p.GetID()})
}

func (h *Handler) handleNew(w http.ResponseWriter, r *http.Request) {
	pu, err := h.pasteUpdateFromRequest(r)
	if err != nil {
		h.Renderer.Error(w, r, err)
		return
	}

	p, err := h.PasteService.CreatePaste(r.Context(), pu)
	if err != nil {
		h.Renderer.Error(w, r, err)
		return
	}

	permitter := h.PermitterProvider.GetPermitterForRequest(r)
	perms := permitter.Permissions(spectre.PermissionClassPaste, p.GetID())
	perms.Grant(spectre.PastePermissionAll)

	h.Renderer.Render(w, r, 200, &PasteUpdateComplete{p.GetID()})
}

func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request) {
	p, err := h.getPasteFromRequest(r)
	if err != nil {
		h.Renderer.Error(w, r, err)
		return
	}

	permitter := h.PermitterProvider.GetPermitterForRequest(r)
	perms := permitter.Permissions(spectre.PermissionClassPaste, p.GetID())
	if !perms.Has(spectre.PastePermissionEdit) {
		// TODO(DH) bounce'em
		return
	}

	h.PasteService.DestroyPaste(r.Context(), p.GetID())
	h.Renderer.Render(w, r, 200, &PasteDeleteComplete{p.GetID()})
}

func (h *Handler) handleShow(w http.ResponseWriter, r *http.Request) {
	p, err := h.getPasteFromRequest(r)
	if err != nil {
		h.Renderer.Error(w, r, err)
		return
	}

	permitted := h.PermitterProvider.GetPermitterForRequest(r).Permissions(spectre.PermissionClassPaste, p.GetID()).Has(spectre.PastePermissionEdit)

	h.Renderer.Render(w, r, 200, &PasteResponse{p, permitted})
}

func (h *Handler) handleShowEditor(w http.ResponseWriter, r *http.Request) {
}

func (h *Handler) BindRoutes(router *mux.Router) error {
	// Methods() does not open a new handling context,
	// so we can't chain path.methods->a, .methods->b
	router.Path("/").
		Methods("POST").HandlerFunc(h.handleNew)

	router.Path("/").
		Methods("GET").HandlerFunc(nil)

	// Legacy
	router.Path("/new").
		Methods("POST").HandlerFunc(h.handleNew)

	router.Path("/{id}").
		Methods("POST", "PUT").HandlerFunc(h.handleUpdate)

	router.Path("/{id}").
		Methods("GET").HandlerFunc(h.handleShow)

	router.Path("/{id}/edit").
		Methods("POST", "PUT").HandlerFunc(h.handleUpdate)

	router.Path("/{id}/edit").
		Methods("GET").HandlerFunc(h.handleShowEditor)

	router.Path("/{id}/delete").
		Methods("POST").HandlerFunc(h.handleDelete)

	return nil
}

func NewHandler(ps spectre.PasteService, perm auth.PermitterProvider, r web.Renderer) *Handler {
	return &Handler{
		PasteService:      ps,
		PermitterProvider: perm,
		Renderer:          r,
	}
}
