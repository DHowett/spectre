package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"
)

func errorRecoveryHandler(w http.ResponseWriter) {
	if err := recover(); err != nil {
		//status := http.StatusInternalServerError
		//if weberr, ok := err.(WebError); ok {
		//status = weberr.StatusCode()
		//}

		//TODO(DH) Render errors.
		//RenderError(err.(error), status, w)
		fmt.Println(string(debug.Stack()))
	}
}

func SetFlash(w http.ResponseWriter, kind, body string) {
	flashBody, err := json.Marshal(map[string]string{
		"type": kind,
		"body": body,
	})
	if err != nil {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:   "flash",
		Value:  base64.URLEncoding.EncodeToString(flashBody),
		Path:   "/",
		MaxAge: 60,
	})
}

var ghosts []string

func _loadGhosts() error {
	ghosts = []string{}
	err := YAMLUnmarshalFile("ghosts.yml", &ghosts)
	if err != nil {
		return err
	}
	for i, v := range ghosts {
		ghosts[i] = " " + v[1:]
	}
	return nil
}

/*
TODO(DH) remove
func init() {
	globalInit.Add(&InitHandler{
		Priority: 20,
		Name:     "ghosts",
		Do: func() error {
			templatePack.AddFunction("randomGhost", func() string {
				if len(ghosts) == 0 {
					return "[no ghosts found :(]"
				}
				return ghosts[rand.Intn(len(ghosts))]
			})
			return _loadGhosts()
		},
		Redo: _loadGhosts,
	})
}
*/
