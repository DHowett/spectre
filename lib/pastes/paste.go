package pastes

import (
	"io"
	"time"
)

type ID string

func (id ID) String() string {
	return string(id)
}

func IDFromString(s string) ID {
	return ID(s)
}

type PasteIDGenerator interface {
	NewPasteID(encrypted bool) ID
}

type PasteIDGeneratorFunc func(encrypted bool) ID

func (f PasteIDGeneratorFunc) NewPasteID(encrypted bool) ID {
	return f(encrypted)
}

var DefaultPasteIDGenerator PasteIDGenerator

func GeneratePasteID(encrypted bool) ID {
	return DefaultPasteIDGenerator.NewPasteID(encrypted)
}

type PasteStore interface {
	GenerateNewPasteID(bool) ID
	NewPaste() (Paste, error)
	NewEncryptedPaste(EncryptionMethod, []byte) (Paste, error)
	Get(ID, []byte) (Paste, error)
	Save(Paste) error
	Destroy(Paste) error
}

type Paste interface {
	GetID() ID

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
