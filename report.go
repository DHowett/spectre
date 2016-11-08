package main

import (
	"encoding/gob"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/DHowett/ghostbin/model"
	"github.com/golang/glog"
	"github.com/gorilla/mux"
)

type ReportInfo map[string]int
type ReportStore struct {
	Reports  map[model.PasteID]ReportInfo
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

func (r *ReportStore) Add(id model.PasteID, kind string) {
	currentReportsForPaste, ok := r.Reports[id]

	if !ok {
		currentReportsForPaste = make(ReportInfo)
		r.Reports[id] = currentReportsForPaste
	}

	currentReportsForPaste[kind] = currentReportsForPaste[kind] + 1
	r.Save()
}

func (r *ReportStore) Delete(p model.PasteID) {
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
	return &ReportStore{Reports: map[model.PasteID]ReportInfo{}, filename: filename}
}

var reportStore *ReportStore

func reportPaste(p model.Paste, w http.ResponseWriter, r *http.Request) {
	if throttleAuthForRequest(r) {
		RenderError(fmt.Errorf("Cool it."), 420, w)
		return
	}

	reason := r.FormValue("reason")

	reportStore.Add(p.GetID(), reason)

	SetFlash(w, "success", fmt.Sprintf("Paste %v reported.", p.GetID()))
	w.Header().Set("Location", pasteURL("show", p.GetID()))
	w.WriteHeader(http.StatusFound)
}

func reportClear(w http.ResponseWriter, r *http.Request) {
	defer errorRecoveryHandler(w)

	id := model.PasteIDFromString(mux.Vars(r)["id"])
	reportStore.Delete(id)

	SetFlash(w, "success", fmt.Sprintf("Report for %v cleared.", id))
	w.Header().Set("Location", "/admin/reports")
	w.WriteHeader(http.StatusFound)
}

func initReportStore() {
	reportStore = LoadReportStore(filepath.Join(arguments.root, "reports.gob"))
}
