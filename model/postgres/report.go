package postgres

import (
	"context"

	"github.com/DHowett/ghostbin/model"
)

type dbReport struct {
	PasteID string
	Count   int

	provider *provider
}

func (r *dbReport) GetPasteID() model.PasteID {
	return model.PasteIDFromString(r.PasteID)
}

func (r *dbReport) GetCount() int {
	return r.Count
}

func (r *dbReport) Destroy() error {
	_, err := r.provider.DB.ExecContext(context.TODO(), `DELETE FROM paste_reports WHERE paste_id = $1`, r.PasteID)
	return err
}
