package spectre

import (
	"context"
	"io"
	"time"
)

var expirationTimeNever time.Time
var ExpirationTimeNever = &expirationTimeNever

type PasteID string

func (id PasteID) String() string {
	return string(id)
}

type PasteUpdate struct {
	LanguageName       *string
	ExpirationTime     *time.Time
	Title              *string
	Body               *string
	PassphraseMaterial PassphraseMaterial
}

type Paste interface {
	GetID() PasteID

	GetLanguageName() string

	// GetExpirationTime returns the time at which this paste will
	// expire or nil if the paste does not expire.
	GetExpirationTime() *time.Time

	GetTitle() string

	IsEncrypted() bool

	GetModificationTime() time.Time

	Reader() (io.ReadCloser, error)

	Update(PasteUpdate) error
}

type PasteService interface {
	CreatePaste(context.Context, *PasteUpdate) (Paste, error)
	GetPaste(context.Context, PassphraseMaterial, PasteID) (Paste, error)
	GetPastes(context.Context, []PasteID) ([]Paste, error)
	DestroyPaste(context.Context, PasteID) (bool, error)
}
