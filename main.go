package main

import (
	"bytes"
	"flag"
	"github.com/bmizerany/pat"
	"github.com/gorilla/sessions"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
)

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

func isEditAllowed(p *Paste, r *http.Request) bool {
	session, _ := sessionStore.Get(r, "session")
	pastes, ok := session.Values["pastes"].([]string)
	if !ok {
		return false
	}

	for _, v := range pastes {
		if v == p.ID.String() {
			return true
		}
	}
	return false
}

func requiresEditPermission(fn ModelRenderFunc) ModelRenderFunc {
	return func(o Model, w http.ResponseWriter, r *http.Request) {
		defer errorRecoveryHandler(w)

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
	session, _ := sessionStore.Get(r, "session")
	pastes, ok := session.Values["pastes"].([]string)
	if !ok {
		pastes = []string{}
	}

	pastes = append(pastes, p.ID.String())
	session.Values["pastes"] = pastes
	session.Save(r, w)
	pasteUpdate(p, w, r)
}

func pasteDelete(o Model, w http.ResponseWriter, r *http.Request) {
	p := o.(*Paste)
	oldId := p.ID
	p.Destroy()

	session, _ := sessionStore.Get(r, "session")
	pastes, ok := session.Values["pastes"].([]string)

	presence := make(map[string]bool)
	if ok {
		for _, v := range pastes {
			presence[v] = true
		}
	}
	delete(presence, oldId.String())
	pastes = make([]string, len(presence))
	i := 0
	for k, _ := range presence {
		pastes[i] = k
		i++
	}

	session.Values["pastes"] = pastes
	session.Save(r, w)

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

var sessionStore *sessions.FilesystemStore

type args struct {
	port, bind *string
	rebuild    *bool
}

func (a *args) register() {
	a.port, a.bind = flag.String("port", "8080", "HTTP port"), flag.String("bind", "0.0.0.0", "bind address")
	a.rebuild = flag.Bool("rebuild", false, "rebuild all templates for each request")
}

func (a *args) parse() {
	flag.Parse()
}

var arguments = &args{}

func init() {
	arguments.register()
	arguments.parse()

	runtime.GOMAXPROCS(runtime.NumCPU())
	RegisterTemplateFunction("editAllowed", func(ri *RenderContext) bool { return isEditAllowed(ri.Obj.(*Paste), ri.Request) })

	os.Mkdir("./sessions", 0700)
	var sessionKey []byte = nil
	if sessionKeyFile, err := os.Open("session.key"); err == nil {
		buf := &bytes.Buffer{}
		io.Copy(buf, sessionKeyFile)
		sessionKey = buf.Bytes()
		sessionKeyFile.Close()
	} else {
		log.Fatalln("session.key not found. make one with seskey.go?")
	}
	sessionStore = sessions.NewFilesystemStore("./sessions", sessionKey)
	sessionStore.Options.Path = "/"
}

func main() {
	InitTemplates(*arguments.rebuild)

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

	var addr string = *arguments.bind + ":" + *arguments.port
	server := &http.Server{
		Addr:    addr,
		Handler: http.DefaultServeMux,
	}
	server.ListenAndServe()
}
