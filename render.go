package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"runtime/debug"
)

type Model interface{}

type CustomTemplateError interface {
	ErrorTemplateName() string
}

type HTTPError interface {
	StatusCode() int
}

func RenderError(e error, statusCode int, w http.ResponseWriter) {
	w.WriteHeader(statusCode)
	page := "error"
	if cte, ok := e.(CustomTemplateError); ok {
		page = cte.ErrorTemplateName()
	}
	templatePack.ExecutePage(w, nil, page, e)
}

func errorRecoveryHandler(w http.ResponseWriter) {
	if err := recover(); err != nil {
		status := http.StatusInternalServerError
		if weberr, ok := err.(HTTPError); ok {
			status = weberr.StatusCode()
		}

		RenderError(err.(error), status, w)
		fmt.Println(string(debug.Stack()))
	}
}

func RenderPageHandler(page string) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer errorRecoveryHandler(w)
		templatePack.ExecutePage(w, r, page, nil)
	})
}

func RenderPartialHandler(page string) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				templatePack.ExecutePartial(w, r, "error", err.(error))
			}
		}()
		templatePack.ExecutePartial(w, r, page, nil)
	})
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
