package main

type ExpiringPasteStore struct {
	PasteStore
}

func (e *ExpiringPasteStore) Get(id ExpirableID) (Expirable, error) {
	return e.PasteStore.Get(PasteID(id), nil)
}

func (e *ExpiringPasteStore) Destroy(ex Expirable) {
	if paste, ok := ex.(*Paste); ok {
		e.PasteStore.Destroy(paste)
	}
}

func (p *Paste) ExpirationID() ExpirableID {
	return ExpirableID(p.ID)
}
