package spectre

import "context"

type GrantID string

func (id GrantID) String() string {
	return string(id)
}

type Grant interface {
	GetID() GrantID
	GetPasteID() PasteID

	Commit() error
	Erase() error
}

type GrantService interface {
	CreateGrant(context.Context, Paste) (Grant, error)
	GetGrant(context.Context, GrantID) (Grant, error)
}
