package postgres

import (
	"io"
	"time"

	"howett.net/spectre"
)

type encryptedPastePlaceholder struct {
	ID               spectre.PasteID
	EncryptionMethod spectre.EncryptionMethod
}

func (e *encryptedPastePlaceholder) GetID() spectre.PasteID {
	return e.ID
}

func (e *encryptedPastePlaceholder) GetLanguageName() string {
	return "unknown"
}

func (e *encryptedPastePlaceholder) SetLanguageName(string) {}

func (e *encryptedPastePlaceholder) IsEncrypted() bool {
	return true
}

func (e *encryptedPastePlaceholder) GetEncryptionMethod() spectre.EncryptionMethod {
	return e.EncryptionMethod
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
	return nil, spectre.ErrCryptorRequired
}

func (e *encryptedPastePlaceholder) Writer() (io.WriteCloser, error) {
	return nil, spectre.ErrCryptorRequired
}

func (e *encryptedPastePlaceholder) Commit() error {
	return spectre.ErrCryptorRequired
}

func (e *encryptedPastePlaceholder) Erase() error {
	return spectre.ErrCryptorRequired
}
