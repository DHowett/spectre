package views

import (
	"bytes"
	"html/template"
	"net/http"
	"sync"

	log "github.com/Sirupsen/logrus"
)

// viewContext represents the template context passed to each render.
type viewContext struct {
	page string
	r    *http.Request
}

// viewID is the internal interface that allows us to differentiate page-based IDs from string-based IDs
type viewID interface {
	// template returns the name of the template used to render the view with this ID
	template() string

	// baseContext creates a new viewContext with pre-seeded values.
	baseContext() *viewContext
}

// PageID represents a view identified by its page name. Views with page
// name identifiers will be treated differently from string-bound views:
//
//     * The template rendered for every Exec will be `tmpl_page`.
//     * The page name will be provided to the view's rendering context.
//     * The `subtemplate` function will render a sibling template prefixed
//       with the page's name.
type PageID string

func (p PageID) template() string {
	return "tmpl_page"
}

func (p PageID) baseContext() *viewContext {
	return &viewContext{page: string(p)}
}

type stringViewID string

func (s stringViewID) template() string {
	return string(s)
}

func (s stringViewID) baseContext() *viewContext {
	return &viewContext{}
}

// View represents an ID bound to a data provider and Model. Its behavior
// is documented in the package-level documentation above.
type View struct {
	mu sync.RWMutex

	// immutable
	id viewID
	dp DataProvider

	// mutable under mu
	tmpl *template.Template
}

func (v *View) subtemplate(vctx *viewContext, name string) template.HTML {
	buf := &bytes.Buffer{}
	st := v.tmpl.Lookup(vctx.page + "_" + name)
	if st == nil {
		// We return an empty snippet here, as a subtemplate failing to exist is non-fatal.
		return template.HTML("")
	}

	err := st.Execute(buf, vctx)
	if err != nil {
		log.WithFields(log.Fields{
			"page":        vctx.page,
			"subtemplate": name,
			"error":       err,
		}).Error("failed to service subtemplate request")
	}
	return template.HTML(buf.String())
}

func (v *View) rebind(root *template.Template) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	tmpl, err := root.Clone()
	if err != nil {
		return err
	}

	tmpl.Funcs(template.FuncMap{
		"local":       varFromDataProvider(v.dp),
		"subtemplate": v.subtemplate,
	})
	v.tmpl = tmpl
	return nil
}

// Exec executes a view given a ResponseWriter and a Request. The Request
// is used as the primary key for every variable lookup during the
// template's execution.
func (v *View) Exec(w http.ResponseWriter, r *http.Request) error {
	v.mu.RLock()
	t := v.tmpl
	v.mu.RUnlock()

	vctx := v.id.baseContext()
	vctx.r = r
	return t.ExecuteTemplate(w, v.id.template(), vctx)
}

// ServeHTTP exists to provide conformance with http.Handler, allowing a
// view to be bound and used directly as a response handler.
func (v *View) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// TODO(DH) Error handle this.
	v.Exec(w, r)
}
