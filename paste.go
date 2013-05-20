package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base32"
	"github.com/DHowett/go-xattr"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type PasteStore interface {
	New() (*Paste, error)
	Get(PasteID) (*Paste, error)
	Save(*Paste) error
	Destroy(*Paste) error
}

type PasteID string

func (id PasteID) String() string {
	return string(id)
}

func PasteIDFromString(s string) PasteID {
	return PasteID(s)
}

type PasteNotFoundError struct {
	ID PasteID
}

func (e PasteNotFoundError) Error() string {
	return "Paste " + e.ID.String() + " was not found."
}

type Paste struct {
	ID       PasteID
	Body     string
	Language string
	store    PasteStore
	mtime    *time.Time
}

func (p *Paste) Save() error {
	return p.store.Save(p)
}

func (p *Paste) Destroy() error {
	return p.store.Destroy(p)
}

type PasteCallback func(*Paste)
type FilesystemPasteStore struct {
	PasteUpdateCallback  PasteCallback
	PasteDestroyCallback PasteCallback
	path                 string
}

func noopPasteCallback(p *Paste) {}

func NewFilesystemPasteStore(path string) *FilesystemPasteStore {
	return &FilesystemPasteStore{
		path:                 path,
		PasteUpdateCallback:  PasteCallback(noopPasteCallback),
		PasteDestroyCallback: PasteCallback(noopPasteCallback),
	}
}

func generatePasteID() (PasteID, error) {
	uuid := make([]byte, 3)
	n, err := rand.Read(uuid)
	if n != len(uuid) || err != nil {
		return "", err
	}

	return PasteIDFromString(base32.NewEncoding("abcdefghijklmnopqrstuvwxyz1234567").EncodeToString(uuid)[0:5]), nil
}

func (store *FilesystemPasteStore) filenameForID(id PasteID) string {
	return filepath.Join(store.path, id.String())
}

func (store *FilesystemPasteStore) New() (p *Paste, err error) {
	id, err := generatePasteID()
	if err != nil {
		return
	}

	p = &Paste{ID: id, store: store}
	return
}

func putMetadata(f *os.File, name string, value string) error {
	return xattr.Setxattr(f.Name(), "user.paste."+name, []byte(value), 0, 0)
}

func getMetadata(f *os.File, name string, dflt string) string {
	bytes, err := xattr.Getxattr(f.Name(), "user.paste."+name, 0, 0)
	if err != nil {
		return dflt
	}

	return string(bytes)
}

func (store *FilesystemPasteStore) Get(id PasteID) (p *Paste, err error) {
	file, err := os.Open(store.filenameForID(id))
	if err != nil {
		err = PasteNotFoundError{ID: id}
		return
	}
	buf := bytes.Buffer{}
	io.Copy(&buf, file)

	p = &Paste{ID: id, store: store}
	p.Body = buf.String()
	p.Language = getMetadata(file, "language", "text")

	file.Close()

	store.PasteUpdateCallback(p)
	return
}

func (store *FilesystemPasteStore) Save(p *Paste) error {
	file, err := os.Create(store.filenameForID(p.ID))
	if err != nil {
		return err
	}
	sreader := strings.NewReader(p.Body)
	io.Copy(file, sreader)
	file.Close()

	if err := putMetadata(file, "language", p.Language); err != nil {
		return err
	}

	store.PasteUpdateCallback(p)
	return nil
}

func (store *FilesystemPasteStore) Destroy(p *Paste) error {
	err := os.Remove(store.filenameForID(p.ID))
	if err != nil {
		return err
	}

	store.PasteDestroyCallback(p)
	return nil
}
