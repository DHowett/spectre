package main

import (
	"crypto/aes"
	"crypto/cipher"
	"github.com/DHowett/go-xattr"
	"golang.org/x/crypto/scrypt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const CURRENT_ENCRYPTION_METHOD string = "2"

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
	ID         PasteID
	Language   *Language
	Encrypted  bool
	Expiration string
	Title      string

	store   PasteStore
	mtime   time.Time
	exptime time.Time

	encryptionKey    []byte
	encryptionSalt   []byte
	encryptionMethod string
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

func (p *Paste) ExpirationTime() time.Time {
	return p.exptime
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
	nbytes, idlen := 4, 5
	if encrypted {
		nbytes, idlen = 5, 8
	}

	for {
		s, err := generateRandomBase32String(nbytes, idlen)
		if _, staterr := os.Stat(store.filenameForID(PasteIDFromString(s))); os.IsNotExist(staterr) {
			return PasteIDFromString(s), err
		}
	}
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
		p.encryptionSalt, _ = generateRandomBytes(16)
		p.encryptionMethod = CURRENT_ENCRYPTION_METHOD
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
	paste.encryptionMethod = getMetadata(filename, "encryption_version", "")
	if hmac != "" {
		if paste.encryptionMethod == "" {
			paste.encryptionMethod = "1"
		}

		paste.Encrypted = true
		salt := getMetadata(filename, "encryption_salt", "")
		if salt == "" {
			paste.encryptionSalt = []byte(paste.ID.String())
		} else {
			saltb, e := base32Encoder.DecodeString(salt)
			if e != nil {
				err = e
				return
			}

			paste.encryptionSalt = saltb
		}

		err = PasteEncryptedError{ID: id}
		if key != nil {
			err = nil

			hmacBytes, e := base32Encoder.DecodeString(hmac)
			if e != nil {
				err = e
				return
			}

			MACMessage := encryptionMethodHandlers[paste.encryptionMethod].generateMACMessage(paste)
			ok := checkMAC(MACMessage, hmacBytes, key)

			if !ok {
				err = PasteInvalidKeyError{ID: id}
				return
			}

			paste.encryptionKey = key
			err = nil
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

	p = paste
	return
}

func (store *FilesystemPasteStore) Save(p *Paste) error {
	filename := store.filenameForID(p.ID)
	if err := putMetadata(filename, "language", p.Language.ID); err != nil {
		return err
	}

	if p.Expiration != "" {
		if err := putMetadata(filename, "expiration", p.Expiration); err != nil {
			return err
		}
	}

	if err := putMetadata(filename, "title", p.Title); err != nil {
		return err
	}

	if p.Encrypted {
		MACMessage := encryptionMethodHandlers[p.encryptionMethod].generateMACMessage(p)
		hmacBytes := constructMAC([]byte(MACMessage), p.encryptionKey)
		hmac := base32Encoder.EncodeToString(hmacBytes)
		if err := putMetadata(filename, "hmac", hmac); err != nil {
			return err
		}

		if err := putMetadata(filename, "encryption_version", p.encryptionMethod); err != nil {
			return err
		}

		if err := putMetadata(filename, "encryption_salt", base32Encoder.EncodeToString(p.encryptionSalt)); err != nil {
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
		r = encryptionMethodHandlers[p.encryptionMethod].encryptedReadWrapper(p, r)
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

	// N.B. We always write using the newest encryption method.
	if p.Encrypted {
		w = encryptionMethodHandlers[p.encryptionMethod].encryptedWriteWrapper(p, w)
	}

	return &PasteWriter{WriteCloser: w, paste: p}, nil
}

type EncryptionMethodHandlers struct {
	generateMACMessage    func(*Paste) []byte
	encryptedReadWrapper  func(*Paste, io.ReadCloser) io.ReadCloser
	encryptedWriteWrapper func(*Paste, io.WriteCloser) io.WriteCloser
}

var encryptionMethodHandlers = map[string]EncryptionMethodHandlers{
	"": EncryptionMethodHandlers{
		generateMACMessage:    func(p *Paste) []byte { return []byte{} },
		encryptedReadWrapper:  func(p *Paste, r io.ReadCloser) io.ReadCloser { return r },
		encryptedWriteWrapper: func(p *Paste, w io.WriteCloser) io.WriteCloser { return w },
	},
	"1": EncryptionMethodHandlers{
		generateMACMessage: func(p *Paste) []byte {
			return []byte(p.ID.String())
		},
		encryptedReadWrapper: func(p *Paste, r io.ReadCloser) io.ReadCloser {
			blockCipher, _ := aes.NewCipher(p.encryptionKey)
			var iv [aes.BlockSize]byte
			stream := cipher.NewOFB(blockCipher, iv[:])
			streamReader := &cipher.StreamReader{S: stream, R: r}
			return &ReadCloser{Reader: streamReader, Closer: r}
		},
		encryptedWriteWrapper: func(p *Paste, w io.WriteCloser) io.WriteCloser {
			blockCipher, _ := aes.NewCipher(p.encryptionKey)
			var iv [aes.BlockSize]byte
			stream := cipher.NewOFB(blockCipher, iv[:])
			streamWriter := &cipher.StreamWriter{S: stream, W: w}
			return &WriteCloser{Writer: streamWriter, Closer: w}
		},
	},
	"2": EncryptionMethodHandlers{
		generateMACMessage: func(p *Paste) []byte {
			return append([]byte(p.ID.String()), p.encryptionSalt...)
		},
		encryptedReadWrapper: func(p *Paste, r io.ReadCloser) io.ReadCloser {
			blockCipher, _ := aes.NewCipher(p.encryptionKey)
			var iv [aes.BlockSize]byte
			stream := cipher.NewCTR(blockCipher, iv[:])
			streamReader := &cipher.StreamReader{S: stream, R: r}
			return &ReadCloser{Reader: streamReader, Closer: r}
		},
		encryptedWriteWrapper: func(p *Paste, w io.WriteCloser) io.WriteCloser {
			blockCipher, _ := aes.NewCipher(p.encryptionKey)
			var iv [aes.BlockSize]byte
			stream := cipher.NewCTR(blockCipher, iv[:])
			streamWriter := &cipher.StreamWriter{S: stream, W: w}
			return &WriteCloser{Writer: streamWriter, Closer: w}
		},
	},
}
