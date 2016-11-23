package main

import (
	"net/http"
	"net/url"

	"github.com/gorilla/mux"
)

type URLType string

const (
	URLTypePasteShow         URLType = "paste.show"
	URLTypePasteRaw                  = "paste.raw"
	URLTypePasteDownload             = "paste.download"
	URLTypePasteEdit                 = "paste.edit"
	URLTypePasteDelete               = "paste.delete"
	URLTypePasteReport               = "paste.report"
	URLTypePasteAuthenticate         = "paste.auth"
	URLTypePasteGrant                = "paste.grant"
	URLTypePasteGrantAccept          = "paste.grant.accept"

	URLTypeAuthToken = "auth.token"

	URLTypeReportClear      = "report.clear"
	URLTypeAdminReportList  = "admin.reports"
	URLTypeAdminPasteDelete = "admin.paste.delete"
)

type Application interface {
	RegisterRouteForURLType(ut URLType, route *mux.Route)
	GenerateURL(ut URLType, params ...string) *url.URL

	RespondWithError(w http.ResponseWriter, webErr WebError)
}
