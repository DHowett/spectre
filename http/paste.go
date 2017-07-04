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

func (ph *pasteHandler) validMethod(w http.ResponseWriter, r *http.Request, methods ...string) bool {
	for _, m := range methods {
		if m == r.Method {
			return true
		}
	}
	return false
}

func (ph *pasteHandler) validPasteRequest(w http.ResponseWriter, r *http.Request) bool {
	body := r.FormValue("text") // TODO(DH) deduplicate
	l := len(body)
	if l == 0 || l > 1048576 /* TODO(DH) limit */ {
		return false
	}

	return true
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

func (ph *pasteHandler) updatePaste(p spectre.Paste, w http.ResponseWriter, r *http.Request) {
	if !ph.validMethod(w, r, "POST") {
		return
	}

	if !ph.validPasteRequest(w, r) {
		// TODO(DH) errors!
		return
	}

	body := r.FormValue("text")
	password := r.FormValue("password")
	language := r.FormValue("lang")
	expireIn := r.FormValue("expire")
	title := r.FormValue("title")
	encrypted := password != ""
	_ = encrypted

	// TODO(DH) disallow encryption on insecure links

	newPaste := p == nil
	permitter := ph.PermitterProvider.GetPermitterForRequest(r)

	if p == nil {
		var err error
		p, err = ph.PasteService.CreatePaste(r.Context(), nil /* TODO(DH) cryptor */)
		if err != nil {
			// TODO(DH) err
			return
		}
	}

	perms := permitter.Permissions(spectre.PermissionClassPaste, p.GetID())
	if newPaste {
		perms.Grant(spectre.PastePermissionAll)
	} else {
		if !perms.Has(spectre.PastePermissionEdit) {
			// TODO(DH) bounce'em
			return
		}
	}

	pw, err := p.Writer()
	if err != nil {
		// TODO(DH) err
		return
	}

	if expireIn != "" && expireIn != "-1" {
		dur, _ := ghtime.ParseDuration(expireIn)
		/* TODO(DH)
		if dur > time.Duration(pc.Config.Application.Limits.PasteMaxExpiration) {
			dur = time.Duration(pc.Config.Application.Limits.PasteMaxExpiration)
		}
		*/
		expireAt := time.Now().Add(dur)
		p.SetExpirationTime(expireAt)
	} else {
		// Empty expireIn means "keep current expiration."
		if expireIn == "-1" {
			p.ClearExpirationTime()
		}
	}

	p.SetLanguageName(language)
	p.SetTitle(title)
	io.WriteString(pw, body)
	pw.Close()

	Redirect(w, http.StatusSeeOther, fmt.Sprintf("/paste/%v", p.GetID()))
}

// TODO(DH) Move
func Redirect(w http.ResponseWriter, status int, urlString string) {
	w.Header().Set("Location", urlString)
	w.WriteHeader(status)
}

func (ph *pasteHandler) createPaste(w http.ResponseWriter, r *http.Request) {
	ph.updatePaste(nil, w, r)
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
