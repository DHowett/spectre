package main

import (
	"encoding/gob"
	"net/http"
	"os"

	"github.com/golang/glog"
	"github.com/gorilla/mux"
)

type ReportInfo map[string]int
type ReportedPasteMap map[PasteID]ReportInfo

func (r ReportedPasteMap) Save(filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	enc := gob.NewEncoder(file)

	// Returns nil if everything worked.
	return enc.Encode(r)
}

func (r ReportedPasteMap) Delete(p PasteID) {
	delete(r, p)
	glog.Info(p, " deleted from report history.")
}

// var reportedPastes = make(map[PasteID]ReportInfo)
var reportedPastes ReportedPasteMap

func reportPaste(o Model, w http.ResponseWriter, r *http.Request) {
	p := o.(*Paste)
	reason := r.FormValue("reason")

	CurrentReports, ok := reportedPastes[p.ID]

	if !ok {
		CurrentReports = make(ReportInfo)
		reportedPastes[p.ID] = CurrentReports
	}

	CurrentReports[reason] = CurrentReports[reason] + 1
	err := reportedPastes.Save("reports.gob")
	if err != nil {
		glog.Error("Error saving to reports.gob", err)
		// Should we be panicking here?
	}

	w.Header().Set("Location", "/")
	w.WriteHeader(http.StatusFound)
}

func loadReports() ReportedPasteMap {
	report_file, err := os.Open("reports.gob")
	if err == nil {
		var decoded_reports ReportedPasteMap
		dec := gob.NewDecoder(report_file)
		err := dec.Decode(&decoded_reports)

		if err != nil {
			glog.Fatal("Failed to decode report.gob :", err)
		}
		return decoded_reports
	}
	return make(ReportedPasteMap)
}

func reportClear(w http.ResponseWriter, r *http.Request) {
	defer errorRecoveryHandler(w)

	id := PasteIDFromString(mux.Vars(r)["id"])
	reportedPastes.Delete(id)
	err := reportedPastes.Save("reports.gob")

	if err != nil {
		glog.Fatal("Error saving reported posts. Error:", err)
		panic(err)
	}

	w.Header().Set("Location", "/admin")
	w.WriteHeader(http.StatusFound)
}

func init() {
	reportedPastes = loadReports()
}
