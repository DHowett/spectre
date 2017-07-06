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
	LanguageName   *string
	ExpirationTime *time.Time
	Title          *string
	Body           *string
}

type Paste interface {
	GetID() PasteID

	GetLanguageName() string

	// GetExpirationTime returns the time at which this paste will
	// expire or nil if the paste does not expire.
	// SetExpirationTime will set the paste to expire at the provided time.
	GetExpirationTime() *time.Time

	GetTitle() string

	IsEncrypted() bool
	GetEncryptionMethod() EncryptionMethod

	GetModificationTime() time.Time

	Reader() (io.ReadCloser, error)

	Update(PasteUpdate) error
	Erase() error
}

type PasteService interface {
	CreatePaste(context.Context, Cryptor) (Paste, error)
	GetPaste(context.Context, Cryptor, PasteID) (Paste, error)
	GetPastes(context.Context, []PasteID) ([]Paste, error)
	DestroyPaste(context.Context, PasteID) (bool, error)
}
