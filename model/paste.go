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

type ExpiringPaste struct {
	PasteID
	time.Time
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

func (e *encryptedPastePlaceholder) GetExpirationTime() *time.Time {
	return nil
}

func (e *encryptedPastePlaceholder) SetExpirationTime(time time.Time) {}
func (e *encryptedPastePlaceholder) ClearExpirationTime()             {}

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
