package main

import (
	//"fmt"
	"flag"
	"github.com/bmizerany/pat"
	"html/template"
	"net/http"
	"strconv"
)

type Model interface{}

var tmpl map[string]*template.Template

// renderModelWith takes a template name and
// returns a function that takes a single model object,
// which when called will render the given template using that object.
func renderModelWith(template string) func(Model, http.ResponseWriter, *http.Request) {
	return func(o Model, w http.ResponseWriter, r *http.Request) {
		tmpl[template].ExecuteTemplate(w, "base", o)
	}
}

func pasteUpdate(o Model, w http.ResponseWriter, r *http.Request) {
	p := o.(*Paste)
	body := r.FormValue("text")
	p.Body = body

	w.Header().Set("Location", p.URL())
	w.WriteHeader(http.StatusFound)
}

func pasteCreate(w http.ResponseWriter, r *http.Request) {
	p := NewPaste()
	pasteUpdate(p, w, r)
	//p.Body = r.FormValue("text")

	//w.Header().Set("Location", p.URL())
	//w.WriteHeader(http.StatusFound)
}

func renderError(e error, statusCode int, w http.ResponseWriter) {
	w.WriteHeader(statusCode)
	tmpl["error"].ExecuteTemplate(w, "base", e)
}

func requiresModelObject(lookup func(*http.Request) (Model, error), fn func(Model, http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		obj, err := lookup(r)
		fn(obj, w, r)

		if err != nil {
			renderError(err, http.StatusNotFound, w)
			return
		}

	})
}

func lookupPasteWithRequest(r *http.Request) (p Model, err error) {
	id, err := strconv.ParseUint(r.URL.Query().Get(":id"), 10, 64)
	if err != nil {
		return nil, err
	}
	p, err = GetPaste(id)
	return
}

func indexGet(w http.ResponseWriter, r *http.Request) {
	tmpl["index"].ExecuteTemplate(w, "base", nil)
}

func allPastes(w http.ResponseWriter, r *http.Request) {
	pasteList := make([]*Paste, len(pastes))
	i := 0
	for _, v := range pastes {
		pasteList[i] = v
		i++
	}
	tmpl["all"].ExecuteTemplate(w, "base", pasteList)
}

func initTemplates() {
	withBase := func(files ...string) *template.Template {
		f := append([]string{"base.tmpl"}, files...)
		return template.Must(template.ParseFiles(f...))
	}
	tmpl = map[string]*template.Template{
		"index":      withBase("index.tmpl"),
		"paste_show": withBase("paste.tmpl"),
		"paste_edit": withBase("paste_edit.tmpl"),
		"error":      withBase("error.tmpl"),
		"all":        withBase("all.tmpl"),
	}
}

func main() {
	port, bind := flag.String("port", "8080", "HTTP port"), flag.String("bind", "0.0.0.0", "bind address")
	flag.Parse()

	pastes = make(map[uint64]*Paste)

	initTemplates()

	m := pat.New()
	m.Get("/paste/all", http.HandlerFunc(allPastes))
	m.Get("/paste/:id", requiresModelObject(lookupPasteWithRequest, renderModelWith("paste_show")))
	m.Get("/paste/:id/edit", requiresModelObject(lookupPasteWithRequest, renderModelWith("paste_edit")))
	m.Post("/paste/:id/edit", requiresModelObject(lookupPasteWithRequest, pasteUpdate))
	m.Post("/paste/new", http.HandlerFunc(pasteCreate))
	m.Get("/", http.HandlerFunc(indexGet))
	http.Handle("/", m)
	http.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("./assets"))))

	var addr string = *bind + ":" + *port
	server := &http.Server{
		Addr:    addr,
		Handler: http.DefaultServeMux,
	}
	server.ListenAndServe()
}
