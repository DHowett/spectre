package views

import "net/http"

// DataProvider represents a View's variable storage. It provides
// read-only access to a view-scoped variables. View executions are
// differentiated by way of the http.Request for which they were
// initiated.
type DataProvider interface {
	ViewValue(r *http.Request, name string) interface{}
}

func varFromDataProvider(dp DataProvider) func(vctx viewContext, name string) interface{} {
	return func(vctx viewContext, name string) interface{} {
		return dp.ViewValue(vctx.r, name)
	}
}
