package main

import (
	"bytes"
	"flag"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"
)

type PasteAccessDeniedError struct {
	action string
	ID     PasteID
}

func (e PasteAccessDeniedError) Error() string {
	return "You're not allowed to " + e.action + " paste " + e.ID.String()
}

// Make the various errors we can throw conform to HTTPError (here vs. the generic type file)
func (e PasteAccessDeniedError) StatusCode() int {
	return http.StatusForbidden
}

func (e PasteNotFoundError) StatusCode() int {
	return http.StatusNotFound
}

type GenericStringError string

func (e GenericStringError) Error() string {
	return string(e)
}

func getPasteRawHandler(o Model, w http.ResponseWriter, r *http.Request) {
	p := o.(*Paste)
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(p.Body))
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
	if len(strings.TrimSpace(body)) == 0 {
		panic(GenericStringError("Hey, put some text in that paste."))
	}

	p.Body = body
	p.Language = r.FormValue("lang")
	if p.Language == "_auto" {
		p.Language, _ = PygmentsGuessLexer(&body)
	}
	p.Save()

	w.Header().Set("Location", pasteURL("show", p))
	w.WriteHeader(http.StatusSeeOther)
}

func pasteCreate(w http.ResponseWriter, r *http.Request) {
	body := r.FormValue("text")
	if len(strings.TrimSpace(body)) == 0 {
		// 400 here, 200 above (one is displayed to the user, one could be an API response.)
		RenderError(GenericStringError("Hey, put some text in that paste."), 400, w)
		return
	}

	p, err := pasteStore.New()
	if err != nil {
		panic(err)
	}

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
	oldId := p.ID.String()
	p.Destroy()

	session, _ := sessionStore.Get(r, "session")

	if session_pastes, ok := session.Values["pastes"].([]string); ok {
		pastes := make([]string, len(session_pastes)-1)
		n := 0
		for _, v := range session_pastes {
			if v == oldId {
				continue
			}
			pastes[n] = v
			n++
		}
		session.Values["pastes"] = pastes
		session.Save(r, w)
	}

	w.Header().Set("Location", "/")
	w.WriteHeader(http.StatusFound)
}

func lookupPasteWithRequest(r *http.Request) (p Model, err error) {
	id := PasteIDFromString(mux.Vars(r)["id"])
	p, err = pasteStore.Get(id)
	return
}

func pasteURL(routeType string, p *Paste) string {
	url, _ := router.Get("paste_"+routeType).URL("id", p.ID.String())
	return url.String()
}

type RenderedPaste struct {
	body template.HTML
}

var renderedPastes = make(map[PasteID]*RenderedPaste)

func renderPaste(p *Paste) template.HTML {
	if p.Language == "text" {
		return template.HTML(template.HTMLEscapeString(p.Body))
	}

	if cached, ok := renderedPastes[p.ID]; !ok {
		pygmentized, err := Pygmentize(&p.Body, p.Language)
		if err != nil {
			return template.HTML("There was an error rendering this paste.<br />" + template.HTMLEscapeString(pygmentized))
		}

		rendered := template.HTML(pygmentized)
		renderedPastes[p.ID] = &RenderedPaste{body: rendered}
		return rendered
	} else {
		return cached.body
	}
}

func pasteMutationCallback(p *Paste) {
	// Clear the cached render when a  paste changes
	delete(renderedPastes, p.ID)
}

var pasteStore *FilesystemPasteStore
var sessionStore *sessions.FilesystemStore
var router *mux.Router

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

type fourOhFourConsumerWriter struct {
	http.ResponseWriter
}

func (w *fourOhFourConsumerWriter) WriteHeader(status int) {
	if status == http.StatusNotFound {
		w.ResponseWriter.Header().Set("Content-Type", "text/html; charset=utf-8")
	}
	w.ResponseWriter.WriteHeader(status)
	if status == http.StatusNotFound {
		ExecuteTemplate(w.ResponseWriter, "page_404", nil)
	}
}

func (w *fourOhFourConsumerWriter) Write(p []byte) (int, error) {
	if bytes.Equal(p, []byte("404 page not found\n")) {
		return len(p), nil
	} else {
		return w.ResponseWriter.Write(p)
	}
}

type fourOhFourConsumerHandler struct {
	http.Handler
}

func (h *fourOhFourConsumerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.Handler.ServeHTTP(&fourOhFourConsumerWriter{ResponseWriter: w}, r)
}

func init() {
	arguments.register()
	arguments.parse()

	runtime.GOMAXPROCS(runtime.NumCPU())
	RegisterTemplateFunction("editAllowed", func(ri *RenderContext) bool { return isEditAllowed(ri.Obj.(*Paste), ri.Request) })
	RegisterTemplateFunction("render", renderPaste)
	RegisterTemplateFunction("pasteURL", pasteURL)

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

	os.Mkdir("./pastes", 0700)
	pasteStore = NewFilesystemPasteStore("./pastes")
	pasteStore.PasteUpdateCallback = PasteCallback(pasteMutationCallback)
	pasteStore.PasteUpdateCallback = PasteCallback(pasteMutationCallback)
}

func main() {
	InitTemplates(*arguments.rebuild)

	router = mux.NewRouter()
	router.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		ExecuteTemplate(w, "page_404", &RenderContext{nil, r})
	})

	if getRouter := router.Methods("GET").Subrouter(); getRouter != nil {
		getRouter.HandleFunc("/paste/{id}", RequiredModelObjectHandler(lookupPasteWithRequest, RenderTemplateForModel("paste_show"))).Name("paste_show")
		getRouter.HandleFunc("/paste/{id}/raw", RequiredModelObjectHandler(lookupPasteWithRequest, ModelRenderFunc(getPasteRawHandler))).Name("paste_raw")
		getRouter.HandleFunc("/paste/{id}/edit", RequiredModelObjectHandler(lookupPasteWithRequest, requiresEditPermission(RenderTemplateForModel("paste_edit")))).Name("paste_edit")
		getRouter.HandleFunc("/paste/{id}/delete", RequiredModelObjectHandler(lookupPasteWithRequest, requiresEditPermission(RenderTemplateForModel("paste_delete_confirm")))).Name("paste_delete")
		getRouter.HandleFunc("/", RenderTemplateHandler("index"))
	}
	if postRouter := router.Methods("POST").Subrouter(); postRouter != nil {
		postRouter.HandleFunc("/paste/{id}/edit", RequiredModelObjectHandler(lookupPasteWithRequest, requiresEditPermission(pasteUpdate)))
		postRouter.HandleFunc("/paste/{id}/delete", RequiredModelObjectHandler(lookupPasteWithRequest, requiresEditPermission(pasteDelete)))
		postRouter.HandleFunc("/paste/new", http.HandlerFunc(pasteCreate))
	}
	router.PathPrefix("/").Handler(&fourOhFourConsumerHandler{Handler: http.FileServer(http.Dir("./public"))})

	var addr string = *arguments.bind + ":" + *arguments.port
	server := &http.Server{
		Addr:    addr,
		Handler: router,
	}
	server.ListenAndServe()
}
