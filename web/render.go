package web

import "net/http"

type Renderer interface {
	Error(w http.ResponseWriter, r *http.Request, err error)
	Render(w http.ResponseWriter, r *http.Request, status int, v interface{})
}
