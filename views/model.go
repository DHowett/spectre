package views

import (
	"errors"
	"fmt"
	"html/template"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
)

// FuncMap is the type of the map providing template functions to all of
// a model's derived views.
type FuncMap template.FuncMap

// FunctionProvider is the interface that allows Model consumers to
// provide their own template functions.
type FunctionProvider interface {
	GetViewFunctions() FuncMap
}

// Model represents a view model loaded from a set of files. Its
// behavior is documented in the package-level documentation above.
type Model struct {
	mu           sync.Mutex
	glob         string
	baseTemplate *template.Template
	tmpl         *template.Template
	bound        []*View
	logger       logrus.FieldLogger
}

// Bind combines a view model, a view ID, and a data provider into a
// single, durable reference to a template. The supplied data provider
// will be used for all `local` variable lookups for the durartion of the
// View's life.
func (m *Model) Bind(id interface{}, dp DataProvider) (*View, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var vid viewID
	switch tid := id.(type) {
	case string:
		vid = stringViewID(tid)
	case viewID:
		vid = tid
	default:
		return nil, fmt.Errorf("unintelligible view ID passed to Bind: %v", id)
	}

	view := &View{
		m:  m,
		id: vid,
		dp: dp,
	}
	err := view.rebind(m.tmpl)
	if err != nil {
		return nil, err
	}

	m.bound = append(m.bound, view)

	return view, nil
}

// Reload reloads the model's view templates from disk, reconstructing
// all bound views and template functions.  No views are re-evaluated or
// re-rendered.
func (m *Model) Reload() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	tmpl, err := m.baseTemplate.Clone()
	if err != nil {
		return err
	}

	tmpl, err = tmpl.ParseGlob(m.glob)
	if err != nil {
		return err
	}
	m.tmpl = tmpl

	// rebind all bound views to the new template
	// this supports the load/bind/reload scenario.
	for _, bv := range m.bound {
		err := bv.rebind(tmpl)
		if err != nil {
			return err
		}
	}
	return nil
}

// New returns a new Model bound to the supplied data provider. The data
// provider will be used for all `global` variable lookups.
func New(glob string, options ...ModelOption) (*Model, error) {
	m := &Model{
		glob: glob,
	}

	tmpl := template.New(".base").Funcs(template.FuncMap{
		// all provided functions must be defined here,
		// otherwise the global parse will fail.
		"now": time.Now,

		// rebind in subviews.
		"subtemplate": func(args ...interface{}) interface{} {
			// subtemplate is rebound for all bound views.
			panic(errors.New("unbound use of subtemplate"))
		},
		"local": func(args ...interface{}) interface{} {
			panic(errors.New("unbound use of local"))
		},
	})

	m.baseTemplate = tmpl

	for _, opt := range options {
		if err := opt(m); err != nil {
			return nil, err
		}
	}

	return m, m.Reload()
}
