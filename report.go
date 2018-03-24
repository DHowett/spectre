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
	asideFilename := r.filename + ".atomic"
	file, err := os.Create(asideFilename)
	if err != nil {
		return err
	}
	defer file.Close()

	enc := gob.NewEncoder(file)

	err = enc.Encode(r)
	if err != nil {
		glog.Error("Failed to save reports: ", err)
		return err
	}

	return os.Rename(asideFilename, r.filename)
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

		if err == nil {
			decoded_reports.filename = filename
			return decoded_reports
		} else {
			glog.Error("Failed to decode reports: ", err)
		}
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

	SetFlash(w, "success", fmt.Sprintf("Paste %v reported.", p.ID))
	w.Header().Set("Location", pasteURL("show", p))
	w.WriteHeader(http.StatusFound)
}

func reportClear(w http.ResponseWriter, r *http.Request) {
	defer errorRecoveryHandler(w)

	id := PasteIDFromString(mux.Vars(r)["id"])
	reportStore.Delete(id)

	SetFlash(w, "success", fmt.Sprintf("Report for %v cleared.", id))
	w.Header().Set("Location", "/admin/reports")
	w.WriteHeader(http.StatusFound)
}

func init() {
	arguments.register()
	arguments.parse()
	reportStore = LoadReportStore(filepath.Join(arguments.root, "reports.gob"))
}
