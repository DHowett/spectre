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

type PasteIDGenerator interface {
	NewPasteID(encrypted bool) PasteID
}

type PasteIDGeneratorFunc func(encrypted bool) PasteID

func (f PasteIDGeneratorFunc) NewPasteID(encrypted bool) PasteID {
	return f(encrypted)
}

var DefaultPasteIDGenerator PasteIDGenerator

func GeneratePasteID(encrypted bool) PasteID {
	return DefaultPasteIDGenerator.NewPasteID(encrypted)
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
