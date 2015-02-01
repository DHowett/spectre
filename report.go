package main

import (
	"net/http"

	"github.com/golang/glog"
)

type report_info map[string]int

var reported_posts = make(map[PasteID]report_info)

func pasteReport(o Model, w http.ResponseWriter, r *http.Request) {
	p := o.(*Paste)
	reason := r.FormValue("reason")

	existing_reports, ok := reported_posts[p.ID]

	if !ok {
		existing_reports = make(report_info)
		reported_posts[p.ID] = existing_reports
	}

	existing_reports[reason] = existing_reports[reason] + 1

	w.Header().Set("Location", "/")
	w.WriteHeader(http.StatusFound)
}
