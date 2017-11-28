package main

import (
	"encoding/json"
	"net/http"
)

type jsonRenderer struct{}

func (jsonRenderer) Error(w http.ResponseWriter, r *http.Request, err error) {
	d, _ := json.Marshal(map[string]string{
		"error": err.Error(),
	})
	w.WriteHeader(500)
	w.Write(d)
}

func (jsonRenderer) Render(w http.ResponseWriter, r *http.Request, status int, v interface{}) {
	d, _ := json.Marshal(map[string]interface{}{
		"object": v,
	})
	w.WriteHeader(status)
	w.Write(d)
}
