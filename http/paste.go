package http

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"howett.net/spectre"
	ghtime "howett.net/spectre/internal/time"
)

type pasteHandler struct {
	PasteService      spectre.PasteService
	PermitterProvider PermitterProvider
}

func (ph *pasteHandler) pasteUpdateFromRequest(r *http.Request) (*spectre.PasteUpdate, error) {
	body := r.FormValue("text")
	language := r.FormValue("lang")
	expireIn := r.FormValue("expire")
	title := r.FormValue("title")

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

	return &pu, nil
}

func (ph *pasteHandler) validMethod(w http.ResponseWriter, r *http.Request, methods ...string) bool {
	for _, m := range methods {
		if m == r.Method {
			return true
		}
	}
	return false
}

func (ph *pasteHandler) getBodyFromRequest(r *http.Request) (string, error) {
	return "", nil
}

func (ph *pasteHandler) getPasteFromRequest(r *http.Request) (spectre.Paste, error) {
	clean := path.Clean(r.URL.Path)
	clean = strings.TrimPrefix(clean, "/paste/")
	pc := strings.Split(clean, "/")
	return ph.PasteService.GetPaste(r.Context(), nil, spectre.PasteID(pc[0]))
}

// TODO(DH) Move
func Redirect(w http.ResponseWriter, status int, urlString string) {
	w.Header().Set("Location", urlString)
	w.WriteHeader(status)
}

// TODO(DH) Rename
func (ph *pasteHandler) updatePasteC(p spectre.Paste, w http.ResponseWriter, pu *spectre.PasteUpdate) {
	err := p.Update(*pu)
	if err != nil {
		// TODO(DH) bounce'em
		panic(err)
	}

	Redirect(w, http.StatusSeeOther, fmt.Sprintf("/paste/%v", p.GetID()))
}

func (ph *pasteHandler) updatePaste(w http.ResponseWriter, r *http.Request) {
	if !ph.validMethod(w, r, "POST") {
		return
	}

	p, err := ph.getPasteFromRequest(r)
	if err != nil {
		// TODO(DH) errors!
		return
	}

	pu, err := ph.pasteUpdateFromRequest(r)
	if err != nil {
		// TODO(DH) errors!
		return
	}

	permitter := ph.PermitterProvider.GetPermitterForRequest(r)
	perms := permitter.Permissions(spectre.PermissionClassPaste, p.GetID())
	if !perms.Has(spectre.PastePermissionEdit) {
		// TODO(DH) bounce'em
		return
	}

	ph.updatePasteC(p, w, pu)
}

func (ph *pasteHandler) createPaste(w http.ResponseWriter, r *http.Request) {
	if !ph.validMethod(w, r, "POST") {
		return
	}

	pu, err := ph.pasteUpdateFromRequest(r)
	if err != nil {
		// TODO(DH) errors!
		return
	}

	password := r.FormValue("password")
	encrypted := password != ""
	_ = encrypted
	// TODO(DH) disallow encryption on insecure links

	p, err := ph.PasteService.CreatePaste(r.Context(), nil /* TODO(DH) cryptor */)
	if err != nil {
		// TODO(DH) err
		return
	}

	permitter := ph.PermitterProvider.GetPermitterForRequest(r)
	perms := permitter.Permissions(spectre.PermissionClassPaste, p.GetID())
	perms.Grant(spectre.PastePermissionAll)

	ph.updatePasteC(p, w, pu)
}

func (ph *pasteHandler) deletePaste(w http.ResponseWriter, r *http.Request) {
	if !ph.validMethod(w, r, "GET", "POST") {
		return
	}
}

func (ph *pasteHandler) showPaste(w http.ResponseWriter, r *http.Request) {
	p, err := ph.getPasteFromRequest(r)
	enc := json.NewEncoder(w)
	enc.Encode(map[string]interface{}{
		"paste":     p,
		"permitted": ph.PermitterProvider.GetPermitterForRequest(r).Permissions(spectre.PermissionClassPaste, p.GetID()).Has(spectre.PastePermissionEdit),
		"err":       fmt.Sprintf("%v", err),
	})
}

func (ph *pasteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// TODO(DH): All of this is bad
	clean := path.Clean(r.URL.Path)
	clean = strings.TrimPrefix(clean, "/paste/")
	pc := strings.Split(clean, "/")

	switch pc[0] {
	case "new":
		// POST only
		ph.createPaste(w, r)
		return
	default:
		ph.showPaste(w, r)
		return
	}
}
