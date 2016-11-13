package model

type GrantID string

func (id GrantID) String() string {
	return string(id)
}

type Grant interface {
	GetID() GrantID
	GetPasteID() PasteID

	Destroy() error
}
