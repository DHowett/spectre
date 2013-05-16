package main

import (
	"flag"
	"github.com/bmizerany/pat"
	"net/http"
	"strings"
)

type Model interface{}
type HTTPError interface {
	StatusCode() int
}
type PasteAccessDeniedError struct {
	action string
	ID     PasteID
}

func (e PasteAccessDeniedError) Error() string {
	return "You're not allowed to " + e.action + " paste " + e.ID.String()
}

func (e PasteAccessDeniedError) StatusCode() int {
	return http.StatusForbidden
}

func (e PasteNotFoundError) StatusCode() int {
	return http.StatusNotFound
}

func errorRecoveryHandler(w http.ResponseWriter) func() {
	return func() {
		if err := recover(); err != nil {
			status := http.StatusInternalServerError
			if weberr, ok := err.(HTTPError); ok {
				status = weberr.StatusCode()
			}

			RenderError(err.(error), status, w)
		}
	}
}

func isEditAllowed(p *Paste, r *http.Request) bool {
	cookie, err := r.Cookie("gb_pastes")
	if err != nil {
		return false
	}

	pastes := strings.Split(cookie.Value, "|")
	for _, v := range pastes {
		if v == p.ID.String() {
			return true
		}
	}
	return false
}

func requiresEditPermission(fn ModelRenderFunc) ModelRenderFunc {
	return func(o Model, w http.ResponseWriter, r *http.Request) {
		defer errorRecoveryHandler(w)()

		p := o.(*Paste)
		accerr := PasteAccessDeniedError{"modify", p.ID}
		if !isEditAllowed(p, r) {
			panic(accerr)
		}
		fn(p, w, r)
	}
}

func pasteUpdate(o Model, w http.ResponseWriter, r *http.Request) {
	p := o.(*Paste)
	body := r.FormValue("text")
	p.Body = body
	p.Language = r.FormValue("lang")
	if p.Language == "_auto" {
		p.Language, _ = PygmentsGuessLexer(&body)
	}
	p.SourceIP = r.RemoteAddr
	p.RenderedBody = nil
	p.Save()

	w.Header().Set("Location", p.URL())
	w.WriteHeader(http.StatusSeeOther)
}

func pasteCreate(w http.ResponseWriter, r *http.Request) {
	p := NewPaste()
	cookie, ok := r.Cookie("gb_pastes")
	if ok != nil {
		cookie = &http.Cookie{
			Name:  "gb_pastes",
			Value: p.ID.String(),
			Path:  "/",
		}
	} else {
		pastes := strings.Split(cookie.Value, "|")
		pastes = append(pastes, p.ID.String())
		cookie.Value = strings.Join(pastes, "|")
	}
	cookie.Path = "/"
	http.SetCookie(w, cookie)
	pasteUpdate(p, w, r)
}

func pasteDelete(o Model, w http.ResponseWriter, r *http.Request) {
	p := o.(*Paste)
	oldId := p.ID
	p.Destroy()

	cookie, ok := r.Cookie("gb_pastes")
	presence := make(map[string]bool)
	if ok == nil {
		for _, v := range strings.Split(cookie.Value, "|") {
			presence[v] = true
		}
	}
	delete(presence, oldId.String())
	pastes := make([]string, len(presence))
	i := 0
	for k, _ := range presence {
		pastes[i] = k
		i++
	}
	cookie.Value = strings.Join(pastes, "|")
	cookie.Path = "/"
	http.SetCookie(w, cookie)

	w.Header().Set("Location", "/")
	w.WriteHeader(http.StatusFound)
}

func lookupPasteWithRequest(r *http.Request) (p Model, err error) {
	id := PasteIDFromString(r.URL.Query().Get(":id"))
	p = GetPaste(id)
	return
}

func allPastes(w http.ResponseWriter, r *http.Request) {
	pasteList := make([]*Paste, len(pastes))
	i := 0
	for _, v := range pastes {
		pasteList[i] = v
		i++
	}
	ExecuteTemplate(w, "page_all", &RenderContext{pasteList, r})
}

func init() {
	RegisterTemplateFunction("editAllowed", func(ri *RenderContext) bool { return isEditAllowed(ri.Obj.(*Paste), ri.Request) })
}

func main() {
	port, bind := flag.String("port", "8080", "HTTP port"), flag.String("bind", "0.0.0.0", "bind address")
	rebuild := flag.Bool("rebuild", false, "rebuild all templates for each request")
	flag.Parse()

	InitTemplates(*rebuild)

	m := pat.New()
	m.Get("/paste/all", http.HandlerFunc(allPastes))
	m.Get("/paste/:id", RequiredModelObjectHandler(lookupPasteWithRequest, RenderTemplateForModel("paste_show")))
	m.Get("/paste/:id/edit", RequiredModelObjectHandler(lookupPasteWithRequest, requiresEditPermission(RenderTemplateForModel("paste_edit"))))
	m.Post("/paste/:id/edit", RequiredModelObjectHandler(lookupPasteWithRequest, requiresEditPermission(pasteUpdate)))
	m.Get("/paste/:id/delete", RequiredModelObjectHandler(lookupPasteWithRequest, requiresEditPermission(RenderTemplateForModel("paste_delete_confirm"))))
	m.Post("/paste/:id/delete", RequiredModelObjectHandler(lookupPasteWithRequest, requiresEditPermission(pasteDelete)))
	m.Post("/paste/new", http.HandlerFunc(pasteCreate))
	m.Get("/", RenderTemplateHandler("index"))
	http.Handle("/", m)
	http.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("./assets"))))

	var addr string = *bind + ":" + *port
	server := &http.Server{
		Addr:    addr,
		Handler: http.DefaultServeMux,
	}
	server.ListenAndServe()
}
