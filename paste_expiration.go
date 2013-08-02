package main

import (
	"./expirator"
)

type ExpiringPasteStore struct {
	PasteStore
}

func (e *ExpiringPasteStore) Get(id expirator.ExpirableID) (expirator.Expirable, error) {
	v, err := e.PasteStore.Get(PasteID(id), nil)
	if v == nil {
		return nil, err
	}
	return v, err
}

func (e *ExpiringPasteStore) Destroy(ex expirator.Expirable) {
	if paste, ok := ex.(*Paste); ok {
		e.PasteStore.Destroy(paste)
	}
}

func (p *Paste) ExpirationID() expirator.ExpirableID {
	return expirator.ExpirableID(p.ID)
}
