package views

import "net/http"

// DataProvider represents a View's variable storage. It provides
// read-only access to a view-scoped variables. View executions are
// differentiated by way of the http.Request for which they were
// initiated.
type DataProvider interface {
	ViewValue(r *http.Request, name string) interface{}
}

func varFromNoop(vctx viewContext, name string) interface{} {
	return nil
}

func varFromDataProvider(dp DataProvider) func(vctx viewContext, name string) interface{} {
	// nil dp is safe; if we get one, no vars exist.
	if dp == nil {
		return varFromNoop
	}

	// otherwise, capture dp and use it for view value generation.
	return func(vctx viewContext, name string) interface{} {
		return dp.ViewValue(vctx.r, name)
	}
}
