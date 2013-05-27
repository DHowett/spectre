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
	"time"
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
	reader, _ := p.Reader()
	defer reader.Close()
	io.Copy(w, reader)
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

	pw, _ := p.Writer()
	pw.Write([]byte(body))
	p.Language = r.FormValue("lang")
	pw.Close() // Saves p

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
		session.Values["pastes"] = pastes[:n]
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
	body       template.HTML
	renderTime time.Time
}

var renderedPastes = make(map[PasteID]*RenderedPaste)

func renderPaste(p *Paste) template.HTML {
	cached, ok := renderedPastes[p.ID]
	if !ok || cached.renderTime.Before(p.LastModified()) {
		reader, err := p.Reader()
		defer reader.Close()
		out, err := RenderForLanguage(reader, p.Language)

		if err != nil {
			return template.HTML("There was an error rendering this paste.<br />" + template.HTMLEscapeString(out))
		}

		rendered := template.HTML(out)
		renderedPastes[p.ID] = &RenderedPaste{body: rendered, renderTime: time.Now()}
		return rendered
	} else {
		return cached.body
	}
}

func pasteDestroyCallback(p *Paste) {
	// Clear the cached render when a paste is destroyed
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

func init() {
	arguments.register()
	arguments.parse()

	runtime.GOMAXPROCS(runtime.NumCPU())
	RegisterTemplateFunction("editAllowed", func(ri *RenderContext) bool { return isEditAllowed(ri.Obj.(*Paste), ri.Request) })
	RegisterTemplateFunction("render", renderPaste)
	RegisterTemplateFunction("pasteURL", pasteURL)
	RegisterTemplateFunction("pasteBody", func(p *Paste) string {
		reader, _ := p.Reader()
		defer reader.Close()
		b := &bytes.Buffer{}
		io.Copy(b, reader)
		return b.String()
	})

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
	sessionStore.Options.MaxAge = 86400 * 365

	os.Mkdir("./pastes", 0700)
	pasteStore = NewFilesystemPasteStore("./pastes")
	pasteStore.PasteDestroyCallback = PasteCallback(pasteDestroyCallback)
}

func main() {
	InitTemplates(*arguments.rebuild)

	router = mux.NewRouter()

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
	router.PathPrefix("/").Handler(http.FileServer(http.Dir("./public")))

	var addr string = *arguments.bind + ":" + *arguments.port
	server := &http.Server{
		Addr:    addr,
		Handler: &fourOhFourConsumerHandler{router},
	}
	server.ListenAndServe()
}
