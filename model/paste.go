package model

import (
	"io"
	"time"
)

type PasteID string

func (id PasteID) String() string {
	return string(id)
}

func PasteIDFromString(s string) PasteID {
	return PasteID(s)
}

type Paste interface {
	GetID() PasteID

	GetLanguageName() string
	SetLanguageName(string)

	IsEncrypted() bool

	GetExpiration() string
	SetExpiration(string)

	GetTitle() string
	SetTitle(string)

	GetModificationTime() time.Time

	Reader() (io.ReadCloser, error)
	Writer() (io.WriteCloser, error)

	Commit() error
	Erase() error
}

type encryptedPastePlaceholder struct {
	ID PasteID
}

func (e *encryptedPastePlaceholder) GetID() PasteID {
	return e.ID
}

func (e *encryptedPastePlaceholder) GetLanguageName() string {
	return "unknown"
}

func (e *encryptedPastePlaceholder) SetLanguageName(string) {}

func (e *encryptedPastePlaceholder) IsEncrypted() bool {
	return true
}

func (e *encryptedPastePlaceholder) GetExpiration() string {
	return ""
}

func (e *encryptedPastePlaceholder) SetExpiration(string) {}

func (e *encryptedPastePlaceholder) GetTitle() string {
	return ""
}

func (e *encryptedPastePlaceholder) SetTitle(string) {}

func (e *encryptedPastePlaceholder) GetModificationTime() time.Time {
	var t time.Time
	return t
}

func (e *encryptedPastePlaceholder) Reader() (io.ReadCloser, error) {
	return nil, ErrPasteEncrypted
}

func (e *encryptedPastePlaceholder) Writer() (io.WriteCloser, error) {
	return nil, ErrPasteEncrypted
}

func (e *encryptedPastePlaceholder) Commit() error {
	return ErrPasteEncrypted
}

func (e *encryptedPastePlaceholder) Erase() error {
	return ErrPasteEncrypted
}
