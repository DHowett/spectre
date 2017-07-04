package http

import (
	"encoding/json"
	"net/http"
	"path"
	"strings"

	"howett.net/spectre"
)

type pasteHandler struct {
	PasteService      spectre.PasteService
	PermitterProvider PermitterProvider
}

func (ph *pasteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// TODO(DH): All of this is bad
	clean := path.Clean(r.URL.Path)
	clean = strings.TrimPrefix(clean, "/paste/")
	pc := strings.Split(clean, "/")
	enc := json.NewEncoder(w)
	paste, err := ph.PasteService.GetPaste(r.Context(), nil, spectre.PasteID(pc[0]))
	if err != nil {
		panic(err)
	}
	enc.Encode(paste)
}
