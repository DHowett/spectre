package spectre

import (
	"context"
	"io"
	"time"
)

type PasteID string

func (id PasteID) String() string {
	return string(id)
}

type Paste interface {
	GetID() PasteID

	GetLanguageName() string
	SetLanguageName(string)

	IsEncrypted() bool
	GetEncryptionMethod() EncryptionMethod

	// GetExpirationTime returns the time at which this paste will
	// expire or nil if the paste does not expire.
	GetExpirationTime() *time.Time

	// SetExpirationTime will set the paste to expire at the provided time.
	SetExpirationTime(time.Time)
	ClearExpirationTime()

	GetTitle() string
	SetTitle(string)

	GetModificationTime() time.Time

	Reader() (io.ReadCloser, error)
	Writer() (io.WriteCloser, error)

	Commit() error
	Erase() error
}

type PasteService interface {
	CreatePaste(context.Context, Cryptor) (Paste, error)
	GetPaste(context.Context, Cryptor, PasteID) (Paste, error)
	GetPastes(context.Context, []PasteID) ([]Paste, error)
	DestroyPaste(context.Context, PasteID) (bool, error)
}
