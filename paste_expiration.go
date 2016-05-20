package main

import (
	"github.com/DHowett/ghostbin/lib/pastes"
	"github.com/DHowett/gotimeout"
)

type ExpiringPasteStore struct {
	pastes.PasteStore
}

type ExpiringPasteID pastes.ID

func (e *ExpiringPasteStore) GetExpirable(id gotimeout.ExpirableID) gotimeout.Expirable {
	v, _ := e.PasteStore.Get(pastes.ID(id), nil)
	if v == nil {
		return nil
	}
	return ExpiringPasteID(id)
}

func (e *ExpiringPasteStore) DestroyExpirable(ex gotimeout.Expirable) {
	v, _ := e.PasteStore.Get(pastes.ID(ex.ExpirationID()), nil)
	if v == nil {
		return
	}
	if paste, ok := v.(pastes.Paste); ok {
		paste.Erase()
	}
}

func (p ExpiringPasteID) ExpirationID() gotimeout.ExpirableID {
	return gotimeout.ExpirableID(p)
}
