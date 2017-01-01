package views

import "net/http"

type contextShared struct {
	request *http.Request
	page    string

	// unguarded vis-a-vis concurrency: a single viewContext is not used in
	// a multithreaded context.
	varCache map[string]interface{}
}

// viewContext represents the template context passed to each render.
type viewContext struct {
	parent *viewContext
	shared *contextShared

	key   interface{}
	value interface{}
}

// Exposed to templates.
func (v *viewContext) Value(key interface{}) interface{} {
	if v.key == key {
		return v.value
	}
	if v.parent == nil {
		return nil
	}
	return v.parent.Value(key)
}

// Exposed to templates.
func (v *viewContext) Obj() interface{} {
	return v.Value("object")
}

// Exposed to templates.
func (v *viewContext) Request() *http.Request {
	return v.shared.request
}

// Exposed to templates.
func (v *viewContext) With(key string, value interface{}) *viewContext {
	return &viewContext{
		parent: v,
		shared: v.shared,
		key:    key,
		value:  value,
	}
}
