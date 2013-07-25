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
	"path/filepath"
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

func sessionOk(r *http.Request) (b bool) {
	//ua := r.Header.Get("User-Agent")
	b = true
	//b = !strings.Contains(ua, "curl")
	return
}

func getPasteRawHandler(o Model, w http.ResponseWriter, r *http.Request) {
	p := o.(*Paste)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
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
		w.Header().Set("Location", pasteURL("delete", p))
		w.WriteHeader(http.StatusFound)
		return
	}

	pw, _ := p.Writer()
	pw.Write([]byte(body))
	if r.FormValue("lang") != "" {
		p.Language = r.FormValue("lang")
	}
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

	if sessionOk(r) {
		session, _ := sessionStore.Get(r, "session")
		pastes, ok := session.Values["pastes"].([]string)
		if !ok {
			pastes = []string{}
		}

		pastes = append(pastes, p.ID.String())
		session.Values["pastes"] = pastes
		session.Save(r, w)
	}

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

func sessionHandler(w http.ResponseWriter, r *http.Request) {
	var pastes []string
	session, _ := sessionStore.Get(r, "session")
	if session_pastes, ok := session.Values["pastes"].([]string); ok {
		p := make([]string, len(session_pastes))
		n := 0
		for _, v := range session_pastes {
			if _, err := os.Stat(filepath.Join(*arguments.root, "pastes", v)); !os.IsNotExist(err) {
				p[n] = v
				n++
			}
		}
		pastes = p[:n]
	} else {
		pastes = []string{}
	}

	if strings.HasSuffix(r.URL.Path, "/raw") {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(strings.Join(pastes, " ")))
	} else {
		ExecuteTemplate(w, "page_session_pastes", &RenderContext{pastes, r})
	}
}

type RedirectHandler string

func (h RedirectHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Location", string(h))
	w.WriteHeader(http.StatusFound)
}

type RenderedPaste struct {
	body       template.HTML
	renderTime time.Time
}

var renderedPastes = make(map[PasteID]*RenderedPaste)

func renderPaste(p *Paste) template.HTML {
	cached, ok := renderedPastes[p.ID]
	if !ok || cached.renderTime.Before(p.LastModified()) {
		out, err := FormatPaste(p)

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
	root, port, bind *string
	rebuild          *bool
	minified         *bool
}

func (a *args) register() {
	a.root = flag.String("root", "./", "path to generated file storage")
	a.port, a.bind = flag.String("port", "8080", "HTTP port"), flag.String("bind", "0.0.0.0", "bind address")
	a.rebuild = flag.Bool("rebuild", false, "rebuild all templates for each request")
	a.minified = flag.Bool("minified", false, "use min.js and min.css files (ala production mode)")
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

	sesdir := filepath.Join(*arguments.root, "sessions")
	os.Mkdir(sesdir, 0700)
	var sessionKey []byte = nil
	if sessionKeyFile, err := os.Open(filepath.Join(*arguments.root, "session.key")); err == nil {
		buf := &bytes.Buffer{}
		io.Copy(buf, sessionKeyFile)
		sessionKey = buf.Bytes()
		sessionKeyFile.Close()
	} else {
		log.Fatalln("session.key not found. make one with seskey.go?")
	}
	sessionStore = sessions.NewFilesystemStore(sesdir, sessionKey)
	sessionStore.Options.Path = "/"
	sessionStore.Options.MaxAge = 86400 * 365

	pastedir := filepath.Join(*arguments.root, "pastes")
	os.Mkdir(pastedir, 0700)
	pasteStore = NewFilesystemPasteStore(pastedir)
	pasteStore.PasteDestroyCallback = PasteCallback(pasteDestroyCallback)
}

func main() {
	InitTemplates(*arguments.rebuild)

	router = mux.NewRouter()

	if getRouter := router.Methods("GET").Subrouter(); getRouter != nil {
		getRouter.Handle("/paste/new", RedirectHandler("/"))
		getRouter.HandleFunc("/paste/{id}", RequiredModelObjectHandler(lookupPasteWithRequest, RenderTemplateForModel("paste_show"))).Name("paste_show")
		getRouter.HandleFunc("/paste/{id}/raw", RequiredModelObjectHandler(lookupPasteWithRequest, ModelRenderFunc(getPasteRawHandler))).Name("paste_raw")
		getRouter.HandleFunc("/paste/{id}/edit", RequiredModelObjectHandler(lookupPasteWithRequest, requiresEditPermission(RenderTemplateForModel("paste_edit")))).Name("paste_edit")
		getRouter.HandleFunc("/paste/{id}/delete", RequiredModelObjectHandler(lookupPasteWithRequest, requiresEditPermission(RenderTemplateForModel("paste_delete_confirm")))).Name("paste_delete")
		getRouter.Handle("/paste/", RedirectHandler("/"))
		getRouter.Handle("/paste", RedirectHandler("/"))
		getRouter.HandleFunc("/session", http.HandlerFunc(sessionHandler))
		getRouter.HandleFunc("/session/raw", http.HandlerFunc(sessionHandler))
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
