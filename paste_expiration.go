package main

import (
	"github.com/DHowett/ghostbin/model"
	"github.com/DHowett/gotimeout"
)

type ExpiringPasteStore struct {
	model.Broker
}

type ExpiringPasteID model.PasteID

func (e *ExpiringPasteStore) GetExpirable(id gotimeout.ExpirableID) gotimeout.Expirable {
	v, _ := e.Broker.GetPaste(model.PasteID(id), nil)
	if v == nil {
		return nil
	}
	return ExpiringPasteID(id)
}

func (e *ExpiringPasteStore) DestroyExpirable(ex gotimeout.Expirable) {
	v, _ := e.Broker.GetPaste(model.PasteID(ex.ExpirationID()), nil)
	if v == nil {
		return
	}
	if paste, ok := v.(model.Paste); ok {
		paste.Erase()
	}
}

func (p ExpiringPasteID) ExpirationID() gotimeout.ExpirableID {
	return gotimeout.ExpirableID(p)
}
