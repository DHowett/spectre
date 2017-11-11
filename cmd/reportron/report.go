package main

import (
	"encoding/gob"
	"os"
	"sort"
)

// https://gitlab.howett.net/Ghostbin/spectre/blob/v1-stable/report.go
type PasteID string
type ReportInfo map[string]int

type ReportedPaste struct {
	ID          PasteID
	ReportCount uint
}

type ReportStore struct {
	Reports map[PasteID]ReportInfo

	ridxmap map[PasteID]int
	reports []*ReportedPaste
}

func (rs *ReportStore) Delete(id PasteID) {
	i, ok := rs.ridxmap[id]
	if !ok {
		return
	}

	delete(rs.Reports, id)
	delete(rs.ridxmap, id)

	// no need to preserve order
	l := len(rs.reports)
	if i == len(rs.reports)-1 {
		rs.reports = rs.reports[:l-1]
	} else {
		rs.reports[i] = rs.reports[l-1]
		rs.reports = rs.reports[:l-1]
		rs.ridxmap[rs.reports[i].ID] = i
	}
}

func (rs *ReportStore) GetReports() []*ReportedPaste {
	return rs.reports
}

func SortReports(i []*ReportedPaste) []*ReportedPaste {
	o := make([]*ReportedPaste, len(i))
	copy(o, i)
	sort.SliceStable(o, func(a, b int) bool {
		return o[a].ReportCount > o[b].ReportCount
	})
	return o
}

func (rs *ReportStore) init() {
	rs.ridxmap = make(map[PasteID]int, len(rs.Reports))
	rs.reports = make([]*ReportedPaste, len(rs.Reports))
	i := 0
	for id, ri := range rs.Reports {
		s := 0
		for _, c := range ri {
			s += c
		}
		rs.reports[i] = &ReportedPaste{id, uint(s)}
		rs.ridxmap[id] = i
		i++
	}
}

func LoadReportStore(path string) (*ReportStore, error) {
	var rs *ReportStore
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	dec := gob.NewDecoder(f)
	err = dec.Decode(&rs)
	if err != nil {
		return nil, err
	}

	rs.init()
	return rs, err
}
