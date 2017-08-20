package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
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

func (ph *pasteHandler) validMethod(w http.ResponseWriter, r *http.Request, methods ...string) bool {
	for _, m := range methods {
		if m == r.Method {
			return true
		}
	}
	return false
}

func (ph *pasteHandler) getPasteFromRequest(r *http.Request) (spectre.Paste, error) {
	clean := path.Clean(r.URL.Path)
	clean = strings.TrimPrefix(clean, "/paste/")
	pc := strings.Split(clean, "/")
	return ph.PasteService.GetPaste(r.Context(), nil, spectre.PasteID(pc[0]))
}

func (ph *pasteHandler) updatePasteC(p spectre.Paste, w http.ResponseWriter) {
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

	err = p.Update(*pu)
	if err != nil {
		// TODO(DH) bounce'em
		panic(err)
	}

	ph.updatePasteC(p, w)
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

	p, err := ph.PasteService.CreatePaste(r.Context(), pu)
	if err != nil {
		// TODO(DH) err
		return
	}

	permitter := ph.PermitterProvider.GetPermitterForRequest(r)
	perms := permitter.Permissions(spectre.PermissionClassPaste, p.GetID())
	perms.Grant(spectre.PastePermissionAll)

	ph.updatePasteC(p, w)
}

func (ph *pasteHandler) deletePaste(w http.ResponseWriter, r *http.Request) {
	if !ph.validMethod(w, r, "GET", "POST") {
		return
	}

	p, err := ph.getPasteFromRequest(r)
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

	ph.PasteService.DestroyPaste(r.Context(), p.GetID())
}

func (ph *pasteHandler) showPaste(w http.ResponseWriter, r *http.Request) {
	p, err := ph.getPasteFromRequest(r)
	var b string
	var perm bool
	if p != nil {
		rdr, _ := p.Reader()
		if rdr != nil {
			bs, _ := ioutil.ReadAll(rdr)
			b = string(bs)
		}
		perm = ph.PermitterProvider.GetPermitterForRequest(r).Permissions(spectre.PermissionClassPaste, p.GetID()).Has(spectre.PastePermissionEdit)
	}
	enc := json.NewEncoder(w)
	enc.Encode(map[string]interface{}{
		"paste":     p,
		"body":      b,
		"permitted": perm,
		"err":       fmt.Sprintf("%v", err),
	})
}

func (ph *pasteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// TODO(DH): All of this is bad
	pc := strings.Split(PathSuffix(r), "/")

	switch pc[1] {
	case "new":
		// POST only
		ph.createPaste(w, r)
		return
	default:
		if len(pc) > 1 {
			switch pc[1] {
			case "edit":
				ph.updatePaste(w, r)
				return
			case "delete":
				ph.deletePaste(w, r)
				return
			case "disavow":
			case "grant":
			}
		}
		ph.showPaste(w, r)
		return
	}
}
