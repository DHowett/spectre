package postgres

import (
	"io"
	"time"

	"github.com/DHowett/ghostbin/model"
)

type encryptedPastePlaceholder struct {
	ID model.PasteID
}

func (e *encryptedPastePlaceholder) GetID() model.PasteID {
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
	return nil, model.ErrPasteEncrypted
}

func (e *encryptedPastePlaceholder) Writer() (io.WriteCloser, error) {
	return nil, model.ErrPasteEncrypted
}

func (e *encryptedPastePlaceholder) Commit() error {
	return model.ErrPasteEncrypted
}

func (e *encryptedPastePlaceholder) Erase() error {
	return model.ErrPasteEncrypted
}
