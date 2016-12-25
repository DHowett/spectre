package postgres

import "github.com/DHowett/ghostbin/model"

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
	_, err := r.provider.CommonDB().Exec("DELETE FROM paste_reports WHERE paste_id = ?", r.PasteID)
	return err
}
