package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/DHowett/go-xattr"

	"github.com/DHowett/ghostbin/lib/pastes"
)

const CURRENT_ENCRYPTION_METHOD pastes.EncryptionMethod = pastes.EncryptionMethodAES_CTR

type pasteReader struct {
	io.ReadCloser
}

func (pr *pasteReader) Close() error {
	return pr.ReadCloser.Close()
}

type pasteWriter struct {
	io.WriteCloser
	paste *fsPaste
}

func (pr *pasteWriter) Close() error {
	pr.paste.Commit()
	return pr.WriteCloser.Close()
}

type fsPaste struct {
	ID         pastes.ID
	Language   *Language
	Encrypted  bool
	Expiration string
	Title      string

	store   *FilesystemPasteStore
	mtime   time.Time
	exptime time.Time

	encryptionKey    []byte
	encryptionSalt   []byte
	encryptionMethod pastes.EncryptionMethod
}

func (p *fsPaste) GetID() pastes.ID {
	return p.ID
}
func (p *fsPaste) GetModificationTime() time.Time {
	return p.mtime
}
func (p *fsPaste) GetLanguageName() string {
	if p.Language == nil {
		return ""
	}
	return p.Language.ID
}
func (p *fsPaste) SetLanguageName(language string) {
	p.Language = LanguageNamed(language)
}
func (p *fsPaste) IsEncrypted() bool {
	return p.Encrypted
}
func (p *fsPaste) GetExpiration() string {
	return p.Expiration
}
func (p *fsPaste) SetExpiration(expiration string) {
	p.Expiration = expiration
}
func (p *fsPaste) GetTitle() string {
	return p.Title
}
func (p *fsPaste) SetTitle(title string) {
	p.Title = title
}

func (p *fsPaste) Commit() error {
	return p.store.Save(p)
}

func (p *fsPaste) Erase() error {
	return p.store.Destroy(p)
}

func (p *fsPaste) Reader() (io.ReadCloser, error) {
	return p.store.readStream(p)
}

func (p *fsPaste) Writer() (io.WriteCloser, error) {
	return p.store.writeStream(p)
}

type PasteCallback func(pastes.Paste)
type FilesystemPasteStore struct {
	PasteUpdateCallback  PasteCallback
	PasteDestroyCallback PasteCallback
	path                 string
}

func noopPasteCallback(p pastes.Paste) {}

func NewFilesystemPasteStore(path string) *FilesystemPasteStore {
	return &FilesystemPasteStore{
		path:                 path,
		PasteUpdateCallback:  PasteCallback(noopPasteCallback),
		PasteDestroyCallback: PasteCallback(noopPasteCallback),
	}
}

func (store *FilesystemPasteStore) GenerateNewPasteID(encrypted bool) pastes.ID {
	nbytes, idlen := 4, 5
	if encrypted {
		nbytes, idlen = 5, 8
	}

	for {
		s, _ := generateRandomBase32String(nbytes, idlen)
		if _, staterr := os.Stat(store.filenameForID(pastes.IDFromString(s))); os.IsNotExist(staterr) {
			return pastes.IDFromString(s)
		}
	}
}

func (store *FilesystemPasteStore) filenameForID(id pastes.ID) string {
	return filepath.Join(store.path, id.String())
}

func (store *FilesystemPasteStore) NewPaste() (pastes.Paste, error) {
	id := store.GenerateNewPasteID(false)
	return &fsPaste{ID: id, store: store}, nil
}

func (store *FilesystemPasteStore) NewEncryptedPaste(method pastes.EncryptionMethod, passphraseMaterial []byte) (pastes.Paste, error) {
	if passphraseMaterial == nil {
		return nil, errors.New("FilesystemPasteStore: unacceptable encryption material")
	}

	salt, err := generateRandomBytes(16)
	if err != nil {
		return nil, err
	}
	key, err := pastes.GetEncryptionHandler(method).DeriveKey(passphraseMaterial, salt)
	if err != nil {
		return nil, err
	}

	id := store.GenerateNewPasteID(true)
	return &fsPaste{
		ID:               id,
		Encrypted:        true,
		store:            store,
		encryptionMethod: method,
		encryptionKey:    key,
		encryptionSalt:   salt,
	}, nil
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

func (store *FilesystemPasteStore) Get(id pastes.ID, passphraseMaterial []byte) (pastes.Paste, error) {
	filename := store.filenameForID(id)
	stat, err := os.Stat(filename)
	if err != nil {
		return nil, pastes.PasteNotFoundError{ID: id}
	}

	paste := &fsPaste{ID: id, store: store, mtime: stat.ModTime()}

	hmac := getMetadata(filename, "hmac", "")
	method, _ := strconv.Atoi(getMetadata(filename, "encryption_version", ""))
	paste.encryptionMethod = pastes.EncryptionMethod(method)
	if hmac != "" {
		if paste.encryptionMethod == pastes.EncryptionMethodNone {
			// Pastes with an HMAC but no encryption method use Ghostbin Legacy Enc.
			paste.encryptionMethod = pastes.EncryptionMethodAES_OFB
		}

		paste.Encrypted = true
		salt := getMetadata(filename, "encryption_salt", "")
		if salt == "" {
			paste.encryptionSalt = []byte(paste.ID.String())
		} else {
			saltb, e := base32Encoder.DecodeString(salt)
			if e != nil {
				return nil, e
			}

			paste.encryptionSalt = saltb
		}

		err = pastes.PasteEncryptedError{ID: id}
		if passphraseMaterial != nil {
			key, err := pastes.GetEncryptionHandler(paste.encryptionMethod).DeriveKey(passphraseMaterial, paste.encryptionSalt)
			if err != nil {
				return nil, err
			}

			hmacBytes, err := base32Encoder.DecodeString(hmac)
			if err != nil {
				return nil, err
			}

			ok := pastes.GetEncryptionHandler(paste.encryptionMethod).Authenticate(id, paste.encryptionSalt, key, hmacBytes)

			if !ok {
				return nil, pastes.PasteInvalidKeyError{ID: id}
			}

			paste.encryptionKey = key
		}
	}

	paste.Language = LanguageNamed(getMetadata(filename, "language", "text"))
	paste.Expiration = getMetadata(filename, "expiration", "")
	paste.Title = getMetadata(filename, "title", "")

	if paste.Expiration != "" {
		if dur, err := ParseDuration(paste.Expiration); err == nil {
			paste.exptime = paste.mtime.Add(dur)
		}
	}

	store.PasteUpdateCallback(paste)

	return paste, nil
}

func (store *FilesystemPasteStore) Save(p pastes.Paste) error {
	fsp := p.(*fsPaste)
	filename := store.filenameForID(fsp.ID)
	if err := putMetadata(filename, "language", fsp.Language.ID); err != nil {
		return err
	}

	if fsp.Expiration != "" {
		if err := putMetadata(filename, "expiration", fsp.Expiration); err != nil {
			return err
		}
	}

	if err := putMetadata(filename, "title", fsp.Title); err != nil {
		return err
	}

	if fsp.Encrypted {
		hmacBytes := pastes.GetEncryptionHandler(fsp.encryptionMethod).GenerateHMAC(fsp.GetID(), fsp.encryptionSalt, fsp.encryptionKey)
		hmac := base32Encoder.EncodeToString(hmacBytes)
		if err := putMetadata(filename, "hmac", hmac); err != nil {
			return err
		}

		if err := putMetadata(filename, "encryption_version", strconv.Itoa(int(fsp.encryptionMethod))); err != nil {
			return err
		}

		if err := putMetadata(filename, "encryption_salt", base32Encoder.EncodeToString(fsp.encryptionSalt)); err != nil {
			return err
		}
	}

	store.PasteUpdateCallback(fsp)
	return nil
}

func (store *FilesystemPasteStore) Destroy(p pastes.Paste) error {
	fsp := p.(*fsPaste)
	err := os.Remove(store.filenameForID(fsp.ID))
	if err != nil {
		return err
	}

	store.PasteDestroyCallback(fsp)
	return nil
}

func (store *FilesystemPasteStore) readStream(p *fsPaste) (io.ReadCloser, error) {
	filename := store.filenameForID(p.ID)
	var r io.ReadCloser
	var err error
	if r, err = os.Open(filename); err != nil {
		return nil, err
	}

	r = &pasteReader{ReadCloser: r}

	if p.Encrypted {
		r = pastes.GetEncryptionHandler(p.encryptionMethod).Reader(p.encryptionKey, r)
	}
	return r, nil
}

func (store *FilesystemPasteStore) writeStream(p *fsPaste) (io.WriteCloser, error) {
	filename := store.filenameForID(p.ID)
	var w io.WriteCloser
	var err error
	if w, err = os.Create(filename); err != nil {
		return nil, err
	}

	w = &pasteWriter{WriteCloser: w, paste: p}

	if p.Encrypted {
		// N.B. We always write using the newest encryption method.
		w = pastes.GetEncryptionHandler(p.encryptionMethod).Writer(p.encryptionKey, w)
	}
	return w, nil
}
