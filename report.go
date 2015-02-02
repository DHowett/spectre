package main

import (
	"encoding/gob"
	"net/http"
	"os"

	"github.com/golang/glog"
	"github.com/gorilla/mux"
)

type report_info map[string]int
type report_posts map[PasteID]report_info

func (r report_posts) Save(filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	enc := gob.NewEncoder(file)

	// Returns nil if everything worked.
	return enc.Encode(r)
}

func (r report_posts) Delete(p PasteID) {
	delete(r, p)
	glog.Info(r)
}

// var reported_posts = make(map[PasteID]report_info)
var reported_posts report_posts

func pasteReport(o Model, w http.ResponseWriter, r *http.Request) {
	p := o.(*Paste)
	reason := r.FormValue("reason")

	existing_reports, ok := reported_posts[p.ID]

	if !ok {
		existing_reports = make(report_info)
		reported_posts[p.ID] = existing_reports
	}

	existing_reports[reason] = existing_reports[reason] + 1
	err := reported_posts.Save("reports.gob")
	if err != nil {
		glog.Error("Error saving to reports.gob", err)
	}

	w.Header().Set("Location", "/")
	w.WriteHeader(http.StatusFound)
}

func loadReports() report_posts {
	report_file, err := os.Open("reports.gob")
	if err == nil {
		var decoded_reports report_posts
		dec := gob.NewDecoder(report_file)
		err := dec.Decode(&decoded_reports)

		if err != nil {
			glog.Fatal("Failed to decode report.gob :", err)
		}
		return decoded_reports
	}
	return make(report_posts)
}

func reportClear(w http.ResponseWriter, r *http.Request) {
	defer errorRecoveryHandler(w)

	id := PasteIDFromString(mux.Vars(r)["id"])
	reported_posts.Delete(id)
	err := reported_posts.Save("reports.gob")

	if err != nil {
		glog.Fatal("Error saving reported posts. Error:", err)
		panic err
	}

	w.Header().Set("Location", "/admin")
	w.WriteHeader(http.StatusFound)
}

func init() {
	reported_posts = loadReports()
}
