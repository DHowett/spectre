package main

import (
	"crypto/rand"
	"encoding/base32"
	"github.com/DHowett/go-xattr"
	"io"
	"os"
	"path/filepath"
	"time"
)

type PasteStore interface {
	New() (*Paste, error)
	Get(PasteID) (*Paste, error)
	Save(*Paste) error
	Destroy(*Paste) error

	readStream(*Paste) (*PasteReader, error)
	writeStream(*Paste) (*PasteWriter, error)
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

type PasteReader struct {
	io.ReadCloser
	paste *Paste
}

func (pr *PasteReader) Close() error {
	return pr.ReadCloser.Close()
}

type PasteWriter struct {
	io.WriteCloser
	paste *Paste
}

func (pr *PasteWriter) Close() error {
	pr.paste.Save()
	return pr.WriteCloser.Close()
}

type Paste struct {
	ID       PasteID
	Language string
	store    PasteStore
	mtime    time.Time
}

func (p *Paste) Save() error {
	return p.store.Save(p)
}

func (p *Paste) Destroy() error {
	return p.store.Destroy(p)
}

func (p *Paste) Reader() (*PasteReader, error) {
	return p.store.readStream(p)
}

func (p *Paste) Writer() (*PasteWriter, error) {
	return p.store.writeStream(p)
}

func (p *Paste) LastModified() time.Time {
	return p.mtime
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

func putMetadata(fn string, name string, value string) error {
	return xattr.Setxattr(fn, "user.paste."+name, []byte(value), 0, 0)
}

func getMetadata(fn string, name string, dflt string) string {
	bytes, err := xattr.Getxattr(fn, "user.paste."+name, 0, 0)
	if err != nil {
		return dflt
	}

	return string(bytes)
}

func (store *FilesystemPasteStore) Get(id PasteID) (p *Paste, err error) {
	filename := store.filenameForID(id)
	stat, err := os.Stat(filename)
	if err != nil {
		err = PasteNotFoundError{ID: id}
		return
	}

	p = &Paste{ID: id, store: store, mtime: stat.ModTime()}
	p.Language = getMetadata(filename, "language", "text")

	store.PasteUpdateCallback(p)
	return
}

func (store *FilesystemPasteStore) Save(p *Paste) error {
	filename := store.filenameForID(p.ID)
	if err := putMetadata(filename, "language", p.Language); err != nil {
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

func (store *FilesystemPasteStore) readStream(p *Paste) (*PasteReader, error) {
	filename := store.filenameForID(p.ID)
	var r io.ReadCloser
	var err error
	if r, err = os.Open(filename); err != nil {
		return nil, err
	}

	return &PasteReader{ReadCloser: r, paste: p}, nil
}

func (store *FilesystemPasteStore) writeStream(p *Paste) (*PasteWriter, error) {
	filename := store.filenameForID(p.ID)
	var w io.WriteCloser
	var err error
	if w, err = os.Create(filename); err != nil {
		return nil, err
	}

	return &PasteWriter{WriteCloser: w, paste: p}, nil
}
