package main

import (
	"github.com/DHowett/gotimeout"
)

type ExpiringPasteStore struct {
	PasteStore
}

func (e *ExpiringPasteStore) GetExpirable(id gotimeout.ExpirableID) gotimeout.Expirable {
	v, _ := e.PasteStore.Get(PasteID(id), nil)
	if v == nil {
		return nil
	}
	return v
}

func (e *ExpiringPasteStore) DestroyExpirable(ex gotimeout.Expirable) {
	if paste, ok := ex.(*Paste); ok {
		e.PasteStore.Destroy(paste)
	}
}

func (p *Paste) ExpirationID() gotimeout.ExpirableID {
	return gotimeout.ExpirableID(p.ID)
}
