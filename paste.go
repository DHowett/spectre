package main

import (
	"code.google.com/p/go.crypto/scrypt"
	"crypto/aes"
	"crypto/cipher"
	"github.com/DHowett/go-xattr"
	"io"
	"os"
	"path/filepath"
	"time"
)

const ENCRYPTION_VERSION string = "1"

type PasteStore interface {
	GenerateNewPasteID(bool) (PasteID, error)
	New(bool) (*Paste, error)
	Get(PasteID, []byte) (*Paste, error)
	Save(*Paste) error
	Destroy(*Paste) error

	EncryptionKeyForPasteWithPassword(*Paste, string) []byte
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

type PasteEncryptedError struct {
	ID PasteID
}

func (e PasteEncryptedError) Error() string {
	return "Paste " + e.ID.String() + " is encrypted."
}

type PasteInvalidKeyError PasteEncryptedError

func (e PasteInvalidKeyError) Error() string { return "" }

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

	Encrypted      bool
	encryptionKey  []byte
	encryptionSalt string
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

func (p *Paste) SetEncryptionKey(key []byte) {
	p.encryptionKey = key
	p.Encrypted = (key != nil)
}

func (p *Paste) EncryptionKeyWithPassword(password string) []byte {
	return p.store.EncryptionKeyForPasteWithPassword(p, password)
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

func (store *FilesystemPasteStore) GenerateNewPasteID(encrypted bool) (PasteID, error) {
	nbytes, idlen := 3, 5
	if encrypted {
		nbytes, idlen = 5, 8
	}

	s, err := generateRandomBase32String(nbytes, idlen)
	return PasteIDFromString(s), err
}

func (store *FilesystemPasteStore) filenameForID(id PasteID) string {
	return filepath.Join(store.path, id.String())
}

func (store *FilesystemPasteStore) New(encrypted bool) (p *Paste, err error) {
	id, err := store.GenerateNewPasteID(encrypted)
	if err != nil {
		panic(err)
	}

	p = &Paste{ID: id, store: store}

	if encrypted {
		p.encryptionSalt, _ = generateRandomBase32String(8, -1)
	}

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

func (store *FilesystemPasteStore) Get(id PasteID, key []byte) (p *Paste, err error) {
	filename := store.filenameForID(id)
	stat, err := os.Stat(filename)
	if err != nil {
		err = PasteNotFoundError{ID: id}
		return
	}

	paste := &Paste{ID: id, store: store, mtime: stat.ModTime()}

	hmac := getMetadata(filename, "hmac", "")
	if hmac != "" {
		paste.Encrypted = true
		paste.encryptionSalt = getMetadata(filename, "encryption_salt", paste.ID.String())

		err = PasteEncryptedError{ID: id}
		if key != nil {
			err = nil

			hmacBytes, e := base32Encoder.DecodeString(hmac)
			if e != nil {
				err = e
				return
			}

			ok := checkMAC([]byte(id.String()), hmacBytes, key)

			if !ok {
				err = PasteInvalidKeyError{ID: id}
				return
			}

			paste.encryptionKey = key
			err = nil
		}
	}

	paste.Language = getMetadata(filename, "language", "text")

	store.PasteUpdateCallback(paste)

	p = paste
	return
}

func (store *FilesystemPasteStore) Save(p *Paste) error {
	filename := store.filenameForID(p.ID)
	if err := putMetadata(filename, "language", p.Language); err != nil {
		return err
	}

	if p.Encrypted {
		hmacBytes := constructMAC([]byte(p.ID.String()), p.encryptionKey)
		hmac := base32Encoder.EncodeToString(hmacBytes)
		if err := putMetadata(filename, "hmac", hmac); err != nil {
			return err
		}

		if err := putMetadata(filename, "encryption_version", ENCRYPTION_VERSION); err != nil {
			return err
		}

		if err := putMetadata(filename, "encryption_salt", p.encryptionSalt); err != nil {
			return err
		}
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

func (store *FilesystemPasteStore) EncryptionKeyForPasteWithPassword(p *Paste, password string) []byte {
	if password == "" {
		return nil
	}

	key, err := scrypt.Key([]byte(password), []byte(p.encryptionSalt), 16384, 8, 1, 32)
	if err != nil {
		panic(err)
	}

	return key
}

func (store *FilesystemPasteStore) readStream(p *Paste) (*PasteReader, error) {
	filename := store.filenameForID(p.ID)
	var r io.ReadCloser
	var err error
	if r, err = os.Open(filename); err != nil {
		return nil, err
	}

	if p.Encrypted {
		blockCipher, _ := aes.NewCipher(p.encryptionKey)
		var iv [aes.BlockSize]byte
		stream := cipher.NewOFB(blockCipher, iv[:])
		streamReader := &cipher.StreamReader{S: stream, R: r}
		r = &ReadCloser{Reader: streamReader, Closer: r}
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

	if p.Encrypted {
		blockCipher, _ := aes.NewCipher(p.encryptionKey)
		var iv [aes.BlockSize]byte
		stream := cipher.NewOFB(blockCipher, iv[:])
		streamWriter := &cipher.StreamWriter{S: stream, W: w}
		w = &WriteCloser{Writer: streamWriter, Closer: w}
	}

	return &PasteWriter{WriteCloser: w, paste: p}, nil
}
