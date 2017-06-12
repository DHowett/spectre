package main

import (
	"encoding/base32"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"howett.net/spectre"

	"github.com/DHowett/go-xattr"
	ghtime "howett.net/spectre/internal/time"
)

var base32Encoder = base32.NewEncoding("abcdefghjkmnopqrstuvwxyz23456789")

type pasteReader struct {
	io.ReadCloser
}

func (pr *pasteReader) Close() error {
	return pr.ReadCloser.Close()
}

type fsPaste struct {
	ID       spectre.PasteID
	Language string

	ModTime time.Time

	// optional fields
	Title          *string
	ExpirationTime *time.Time

	HMAC             []byte
	EncryptionSalt   []byte
	EncryptionMethod int

	store *FilesystemPasteStore
}

func (p *fsPaste) Reader() (io.ReadCloser, error) {
	return p.store.readStream(p)
}

type FilesystemPasteStore struct {
	path string
}

func NewFilesystemPasteStore(path string) *FilesystemPasteStore {
	return &FilesystemPasteStore{
		path: path,
	}
}

func (store *FilesystemPasteStore) filenameForID(id spectre.PasteID) string {
	return filepath.Join(store.path, id.String())
}

func getMetadata(fn string, name string, dflt string) string {
	bytes, err := xattr.Getxattr(fn, "user.paste."+name, 0, 0)
	if err != nil {
		return dflt
	}

	return string(bytes)
}

func (store *FilesystemPasteStore) Get(id spectre.PasteID, passphraseMaterial []byte) (*fsPaste, error) {
	filename := store.filenameForID(id)
	stat, err := os.Stat(filename)
	if err != nil {
		return nil, os.ErrNotExist
	}

	paste := &fsPaste{ID: id, store: store, ModTime: stat.ModTime()}

	hmac := getMetadata(filename, "hmac", "")

	method, _ := strconv.Atoi(getMetadata(filename, "encryption_version", ""))
	paste.EncryptionMethod = method
	if hmac != "" {
		hmacBytes, err := base32Encoder.DecodeString(hmac)
		if err != nil {
			return nil, err
		}
		paste.HMAC = hmacBytes

		//TODO(DH) if paste.EncryptionMethod == model.PasteEncryptionMethodNone {
		// Pastes with an HMAC but no encryption method use Ghostbin Legacy Enc.
		//paste.EncryptionMethod = model.PasteEncryptionMethodAES_OFB
		//}

		salt := getMetadata(filename, "encryption_salt", "")
		if salt == "" {
			paste.EncryptionSalt = []byte(paste.ID.String())
		} else {
			saltb, e := base32Encoder.DecodeString(salt)
			if e != nil {
				return nil, e
			}

			paste.EncryptionSalt = saltb
		}
	}

	paste.Language = getMetadata(filename, "language", "text")
	title := getMetadata(filename, "title", "")
	if title != "" {
		paste.Title = &title
	}

	expiration := getMetadata(filename, "expiration", "")
	if expiration != "" {
		if dur, err := ghtime.ParseDuration(expiration); err == nil {
			expTime := paste.ModTime.Add(dur)
			paste.ExpirationTime = &expTime
		}
	}

	return paste, nil
}

func (store *FilesystemPasteStore) readStream(p *fsPaste) (io.ReadCloser, error) {
	filename := store.filenameForID(p.ID)
	var r io.ReadCloser
	var err error
	if r, err = os.Open(filename); err != nil {
		return nil, err
	}

	r = &pasteReader{ReadCloser: r}
	return r, nil
}
