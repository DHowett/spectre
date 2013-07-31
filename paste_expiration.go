package main

type ExpiringPasteStore struct {
	PasteStore
}

func (e *ExpiringPasteStore) Get(id ExpirableID) (Expirable, error) {
	v, err := e.PasteStore.Get(PasteID(id), nil)
	if v == nil {
		return nil, err
	}
	return v, err
}

func (e *ExpiringPasteStore) Destroy(ex Expirable) {
	if paste, ok := ex.(*Paste); ok {
		e.PasteStore.Destroy(paste)
	}
}

func (p *Paste) ExpirationID() ExpirableID {
	return ExpirableID(p.ID)
}
