package main

import (
	"flag"
	"github.com/bmizerany/pat"
	"html/template"
	"net/http"
	"path/filepath"
)

type Model interface{}
type HTTPError interface {
	StatusCode() int
}

func (e PasteNotFoundError) StatusCode() int {
	return http.StatusNotFound
}

var tmpl func() *template.Template

func renderError(e error, statusCode int, w http.ResponseWriter) {
	w.WriteHeader(statusCode)
	tmpl().ExecuteTemplate(w, "page_error", e)
}

// renderModelWith takes a template name and
// returns a function that takes a single model object,
// which when called will render the given template using that object.
func renderModelWith(template string) func(Model, http.ResponseWriter, *http.Request) {
	// We don't defer the error handler here because it happened a step up
	return func(o Model, w http.ResponseWriter, r *http.Request) {
		tmpl().ExecuteTemplate(w, "page_"+template, o)
	}
}

func renderTemplate(template string) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer errorRecoveryHandler(w)()
		tmpl().ExecuteTemplate(w, "page_"+template, nil)
	})
}

func errorRecoveryHandler(w http.ResponseWriter) func() {
	return func() {
		if err := recover(); err != nil {
			status := http.StatusInternalServerError
			if weberr, ok := err.(HTTPError); ok {
				status = weberr.StatusCode()
			}

			renderError(err.(error), status, w)
		}
	}
}

func requiresModelObject(lookup func(*http.Request) (Model, error), fn func(Model, http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer errorRecoveryHandler(w)()

		obj, err := lookup(r)
		if err != nil {
			renderError(err, http.StatusNotFound, w)
			return
		}

		fn(obj, w, r)

	})
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
	w.WriteHeader(http.StatusFound)
}

func pasteCreate(w http.ResponseWriter, r *http.Request) {
	p := NewPaste()
	pasteUpdate(p, w, r)
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
	tmpl().ExecuteTemplate(w, "page_all", pasteList)
}

func initTemplates(rebuild bool) {
	templateFuncs := template.FuncMap{
		"langs": Languages,
		"equal": func(t1, t2 string) bool { return t1 == t2 },
	}

	tmpl = func() *template.Template {
		files, err := filepath.Glob("tmpl/*")
		if err != nil {
			panic(err)
		}
		return template.Must(template.New("base").Funcs(templateFuncs).ParseFiles(files...))
	}
	if !rebuild {
		t := tmpl()
		tmpl = func() *template.Template {
			return t
		}
	}
}

type LanguageMap map[string]string

var languages LanguageMap = LanguageMap{
	"_auto":  "Automatically Detect",
	"text":   "Plain Text",
}

func Languages() LanguageMap {
	return languages
}

func main() {
	port, bind := flag.String("port", "8080", "HTTP port"), flag.String("bind", "0.0.0.0", "bind address")
	rebuild := flag.Bool("rebuild", false, "rebuild all templates for each request")
	flag.Parse()

	initTemplates(*rebuild)

	m := pat.New()
	m.Get("/paste/all", http.HandlerFunc(allPastes))
	m.Get("/paste/:id", requiresModelObject(lookupPasteWithRequest, renderModelWith("paste_show")))
	m.Get("/paste/:id/edit", requiresModelObject(lookupPasteWithRequest, renderModelWith("paste_edit")))
	m.Post("/paste/:id/edit", requiresModelObject(lookupPasteWithRequest, pasteUpdate))
	m.Post("/paste/new", http.HandlerFunc(pasteCreate))
	m.Get("/", renderTemplate("index"))
	http.Handle("/", m)
	http.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("./assets"))))

	var addr string = *bind + ":" + *port
	server := &http.Server{
		Addr:    addr,
		Handler: http.DefaultServeMux,
	}
	server.ListenAndServe()
}
