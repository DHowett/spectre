package model

type dbReport struct {
	PasteID string
	Count   int

	broker *dbBroker
}

func (r *dbReport) GetPasteID() PasteID {
	return PasteIDFromString(r.PasteID)
}

func (r *dbReport) GetCount() int {
	return r.Count
}

func (r *dbReport) Destroy() error {
	_, err := r.broker.CommonDB().Exec("DELETE FROM paste_reports WHERE paste_id = ?", r.PasteID)
	return err
}
