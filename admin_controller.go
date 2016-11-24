package main

import (
	"fmt"
	"net/http"

	"github.com/DHowett/ghostbin/model"
	"github.com/DHowett/ghostbin/views"
	"github.com/gorilla/mux"
)

type AdminController struct {
	App    Application    `inject:""`
	Model  model.Broker   `inject:""`
	Config *Configuration `inject:""`

	adminHomeView *views.View
}

func (ac *AdminController) loggedInUserMatcher(r *http.Request, matcher *mux.RouteMatch) bool {
	user := GetLoggedInUser(r)
	if user != nil {
		if user.Permissions(model.PermissionClassUser).Has(model.UserPermissionAdmin) {
			return true
		}
	}
	return false
}

func (ac *AdminController) notAllowedHandler(w http.ResponseWriter, r *http.Request) {
	// TODO(DH) Render a login page.
	w.Header().Set("Content-Type", "text/plain; charset=utf8")
	w.Write([]byte("Don't do that. Not yet, anyway."))
}

func (ac *AdminController) pasteDeleteHandler(w http.ResponseWriter, r *http.Request) {
	id := model.PasteIDFromString(mux.Vars(r)["id"])
	paste, err := ac.Model.GetPaste(id, nil)
	if paste != nil && err == nil {
		err = paste.Erase()
	}

	if err == model.ErrNotFound {
		SetFlash(w, "error", fmt.Sprintf("Paste %v not found.", id))
	} else if err == model.ErrPasteEncrypted {
		SetFlash(w, "error", fmt.Sprintf("Can't delete paste %v: It's encrypted.", id))
	} else if err != nil {
		SetFlash(w, "error", fmt.Sprintf("Can't delete paste %v: %v", id, err))
	} else {
		SetFlash(w, "success", fmt.Sprintf("Paste %v deleted.", id))
	}

	w.Header().Set("Location", ac.App.GenerateURL(URLTypeAdminReportList).String())
	w.WriteHeader(http.StatusSeeOther)
}

func (ac *AdminController) adminPromoteHandler(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	user, _ := ac.Model.GetUserNamed(username)
	if user != nil {
		err := user.Permissions(model.PermissionClassUser).Grant(model.UserPermissionAdmin)
		if err == nil {
			SetFlash(w, "success", "Promoted "+username+".")
		} else {
			SetFlash(w, "error", "Failed to promote "+username+".")
		}
	} else {
		SetFlash(w, "error", "Couldn't find "+username+" to promote.")
	}

	w.Header().Set("Location", "/admin")
	w.WriteHeader(http.StatusSeeOther)
}

func (ac *AdminController) reportsHandler(w http.ResponseWriter, r *http.Request) {
	reports, err := ac.Model.GetReports()
	if err != nil {
		panic(err)
	}
	templatePack.ExecutePage(w, r, "admin_reports", reports)
}

func (ac *AdminController) reportClearHandler(w http.ResponseWriter, r *http.Request) {
	pID := model.PasteIDFromString(mux.Vars(r)["id"])
	report, err := ac.Model.GetReport(pID)
	if err == nil {
		err = report.Destroy()
	}

	if err == model.ErrNotFound {
		SetFlash(w, "error", fmt.Sprintf("Report for %v didn't exist.", pID))
	} else if err != nil {
		SetFlash(w, "error", fmt.Sprintf("Report for %v couldn't be deleted: %v", pID, err))
	} else {
		SetFlash(w, "success", fmt.Sprintf("Report for %v cleared.", pID))
	}

	w.Header().Set("Location", "/admin/reports")
	w.WriteHeader(http.StatusFound)

}

func (ac *AdminController) InitRoutes(router *mux.Router) {
	adminRouter := router.MatcherFunc(ac.loggedInUserMatcher).Subrouter()

	adminRouter.Path("/").Handler(ac.adminHomeView)

	adminReportsRoute :=
		adminRouter.
			Path("/reports").
			HandlerFunc(ac.reportsHandler)

	adminRouter.
		Methods("POST").
		Path("/promote").
		HandlerFunc(ac.adminPromoteHandler)

	adminDeleteRoute :=
		adminRouter.Methods("POST").
			Path("/paste/{id}/delete").
			HandlerFunc(ac.pasteDeleteHandler)

	reportClearRoute :=
		adminRouter.Methods("POST").
			Path("/paste/{id}/clear_report").
			HandlerFunc(ac.reportClearHandler)

	// Fallback: If the previous Matcher (loggedInUserMatcher) failed,
	// render a login page.
	router.NewRoute().HandlerFunc(ac.notAllowedHandler)

	ac.App.RegisterRouteForURLType(URLTypeAdminReportList, adminReportsRoute)
	ac.App.RegisterRouteForURLType(URLTypeAdminPasteDelete, adminDeleteRoute)
	ac.App.RegisterRouteForURLType(URLTypeReportClear, reportClearRoute)
}

func (ac *AdminController) BindViews(viewModel *views.Model) error {
	return bindViews(viewModel, nil, map[interface{}]**views.View{
		views.PageID("admin_home"): &ac.adminHomeView,
	})
}
