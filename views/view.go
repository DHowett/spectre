package views

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"sync"

	"github.com/Sirupsen/logrus"
)

// viewID is the internal interface that allows us to differentiate page-based
// IDs from string-based IDs
type viewID interface {
	// template returns the name of the template used to render the view
	// with this ID
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
	return &viewContext{
		shared: &contextShared{
			page: string(p),
		},
	}
}

func (p PageID) String() string {
	return "page " + string(p)
}

type stringViewID string

func (s stringViewID) template() string {
	return string(s)
}

func (s stringViewID) baseContext() *viewContext {
	return &viewContext{
		shared: &contextShared{},
	}
}

func (s stringViewID) String() string {
	return string(s)
}

// View represents an ID bound to a data provider and Model. Its behavior
// is documented in the package-level documentation above.
type View struct {
	m  *Model
	mu sync.RWMutex

	// immutable
	id viewID
	dp DataProvider

	// mutable under mu
	tmpl *template.Template
}

// Exposed to templates.
func (v *View) subexec(vctx *viewContext, name string) template.HTML {
	v.mu.RLock()
	t := v.tmpl
	v.mu.RUnlock()

	buf := &bytes.Buffer{}
	st := t.Lookup(name)
	if st == nil {
		// We return an empty snippet here, as a subtemplate failing to exist is non-fatal.
		return template.HTML("")
	}

	err := st.Execute(buf, vctx)
	if err != nil {
		if v.m.logger != nil {
			v.m.logger.WithFields(logrus.Fields{
				"id":          v.id,
				"subtemplate": name,
				"error":       err,
			}).Error("failed to service subtemplate request")
		}
	}
	return template.HTML(buf.String())
}

// Exposed to templates.
func (v *View) subtemplate(vctx *viewContext, name string) template.HTML {
	parent := vctx.shared.page
	return v.subexec(vctx, fmt.Sprintf("%s_%s", parent, name))
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
		"subexec":     v.subexec,
		"subtemplate": v.subtemplate,
	})
	v.tmpl = tmpl
	return nil
}

// Exec executes a view given a ResponseWriter and a Request. The Request
// is used as the primary key for every variable lookup during the
// template's execution.
func (v *View) Exec(w http.ResponseWriter, r *http.Request, params ...interface{}) error {
	v.mu.RLock()
	t := v.tmpl
	v.mu.RUnlock()

	vctx := v.id.baseContext()
	vctx.shared.request = r
	if len(params) == 1 {
		vctx = vctx.With("object", params[0])
	} else if len(params) > 1 {
		vctx = vctx.With("object", params)
	}
	return t.ExecuteTemplate(w, v.id.template(), vctx)
}

// ServeHTTP exists to provide conformance with http.Handler, allowing a
// view to be bound and used directly as a response handler.
func (v *View) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// TODO(DH) Error handle this.
	v.Exec(w, r)
}
