package spectre

import (
	"context"
	"time"
)

type Report interface {
	GetPasteID() PasteID

	GetCount() int

	GetCreationTime() time.Time
	GetModificationTime() time.Time

	Erase() error
}

type ReportService interface {
	// Reports
	ReportPaste(context.Context, Paste) error
	GetReport(context.Context, PasteID) (Report, error)
	GetReports(context.Context) ([]Report, error)
}
