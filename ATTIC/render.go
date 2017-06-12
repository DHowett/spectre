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
