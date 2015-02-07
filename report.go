package main

import (
	"encoding/gob"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/golang/glog"
	"github.com/gorilla/mux"
)

type ReportInfo map[string]int
type ReportStore struct {
	Reports  map[PasteID]ReportInfo
	filename string
}

func (r *ReportStore) Save() error {
	file, err := os.Create(r.filename)
	if err != nil {
		return err
	}
	defer file.Close()

	enc := gob.NewEncoder(file)

	// Returns nil if everything worked.
	return enc.Encode(r)
}

func (r *ReportStore) Add(id PasteID, kind string) {
	currentReportsForPaste, ok := r.Reports[id]

	if !ok {
		currentReportsForPaste = make(ReportInfo)
		r.Reports[id] = currentReportsForPaste
	}

	currentReportsForPaste[kind] = currentReportsForPaste[kind] + 1
	r.Save()
}

func (r *ReportStore) Delete(p PasteID) {
	delete(r.Reports, p)
	glog.Info(p, " deleted from report history.")
	r.Save()
}

func LoadReportStore(filename string) *ReportStore {
	report_file, err := os.Open(filename)
	if err == nil {
		var decoded_reports *ReportStore
		dec := gob.NewDecoder(report_file)
		err := dec.Decode(&decoded_reports)

		if err != nil {
			glog.Fatal("Failed to decode reports: ", err)
		}
		decoded_reports.filename = filename
		return decoded_reports
	}
	return &ReportStore{Reports: map[PasteID]ReportInfo{}, filename: filename}
}

var reportStore *ReportStore

func reportPaste(o Model, w http.ResponseWriter, r *http.Request) {
	if throttleAuthForRequest(r) {
		RenderError(fmt.Errorf("Cool it."), 420, w)
		return
	}

	p := o.(*Paste)
	reason := r.FormValue("reason")

	reportStore.Add(p.ID, reason)

	w.Header().Set("Location", "/")
	w.WriteHeader(http.StatusFound)
}

func reportClear(w http.ResponseWriter, r *http.Request) {
	defer errorRecoveryHandler(w)

	id := PasteIDFromString(mux.Vars(r)["id"])
	reportStore.Delete(id)

	w.Header().Set("Location", "/admin")
	w.WriteHeader(http.StatusFound)
}

func init() {
	arguments.register()
	arguments.parse()
	reportStore = LoadReportStore(filepath.Join(arguments.root, "reports.gob"))
}
