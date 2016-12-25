package main

import (
	"github.com/DHowett/ghostbin/model"
	"github.com/DHowett/gotimeout"
)

type ExpiringPasteStore struct {
	model.Provider
}

type ExpiringPasteID model.PasteID

func (e *ExpiringPasteStore) GetExpirable(id gotimeout.ExpirableID) gotimeout.Expirable {
	return ExpiringPasteID(id)
}

func (e *ExpiringPasteStore) DestroyExpirable(ex gotimeout.Expirable) {
	e.Provider.DestroyPaste(model.PasteID(ex.ExpirationID()))
}

func (p ExpiringPasteID) ExpirationID() gotimeout.ExpirableID {
	return gotimeout.ExpirableID(p)
}
