package pastes

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"io"

	"golang.org/x/crypto/scrypt"
)

type EncryptionMethod uint

const (
	EncryptionMethodNone    EncryptionMethod = iota
	EncryptionMethodAES_OFB                  // deprecated
	EncryptionMethodAES_CTR
)

type MACMessage interface {
	HMAC([]byte) []byte
}

type EncryptionHandler interface {
	Authenticate(ID, []byte, []byte, []byte) bool
	GenerateHMAC(ID, []byte, []byte) []byte
	Reader([]byte, io.ReadCloser) io.ReadCloser
	Writer([]byte, io.WriteCloser) io.WriteCloser
	DeriveKey([]byte, []byte) ([]byte, error)
}

type noopEncryptionHandler struct{}

func (eh *noopEncryptionHandler) Authenticate(id ID, salt []byte, key []byte, hmac []byte) bool {
	return true
}

func (eh *noopEncryptionHandler) GenerateHMAC(id ID, salt []byte, key []byte) []byte {
	return nil
}

func (eh *noopEncryptionHandler) Reader(key []byte, r io.ReadCloser) io.ReadCloser {
	return r
}

func (eh *noopEncryptionHandler) Writer(key []byte, w io.WriteCloser) io.WriteCloser {
	return w
}

func (eh *noopEncryptionHandler) DeriveKey(material []byte, salt []byte) ([]byte, error) {
	return nil, nil
}

func _scryptDeriveKey(material []byte, salt []byte) ([]byte, error) {
	return scrypt.Key(material, salt, 16384, 8, 1, 32)
}

type ghostbinLegacyEncryptionHandler struct{}

func (eh *ghostbinLegacyEncryptionHandler) Authenticate(id ID, salt []byte, key []byte, messageMAC []byte) bool {
	return hmac.Equal(messageMAC, eh.GenerateHMAC(id, salt, key))
}
func (eh *ghostbinLegacyEncryptionHandler) GenerateHMAC(id ID, salt []byte, key []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(id.String()))
	return mac.Sum(nil)
}

func (eh *ghostbinLegacyEncryptionHandler) Reader(key []byte, r io.ReadCloser) io.ReadCloser {
	blockCipher, _ := aes.NewCipher(key)
	var iv [aes.BlockSize]byte
	stream := cipher.NewOFB(blockCipher, iv[:])
	streamReader := &cipher.StreamReader{S: stream, R: r}
	return &readCloser{Reader: streamReader, Closer: r}
}

func (eh *ghostbinLegacyEncryptionHandler) Writer(key []byte, w io.WriteCloser) io.WriteCloser {
	blockCipher, _ := aes.NewCipher(key)
	var iv [aes.BlockSize]byte
	stream := cipher.NewOFB(blockCipher, iv[:])
	streamWriter := &cipher.StreamWriter{S: stream, W: w}
	return &writeCloser{Writer: streamWriter, Closer: w}
}

func (eh *ghostbinLegacyEncryptionHandler) DeriveKey(material []byte, salt []byte) ([]byte, error) {
	return _scryptDeriveKey(material, salt)
}

type aesCtrEncryptionHandler struct{}

func (eh *aesCtrEncryptionHandler) Authenticate(id ID, salt []byte, key []byte, messageMAC []byte) bool {
	return hmac.Equal(messageMAC, eh.GenerateHMAC(id, salt, key))
}
func (eh *aesCtrEncryptionHandler) GenerateHMAC(id ID, salt []byte, key []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(id.String()))
	mac.Write(salt)
	return mac.Sum(nil)
}

func (eh *aesCtrEncryptionHandler) Reader(key []byte, r io.ReadCloser) io.ReadCloser {
	blockCipher, _ := aes.NewCipher(key)
	var iv [aes.BlockSize]byte
	stream := cipher.NewCTR(blockCipher, iv[:])
	streamReader := &cipher.StreamReader{S: stream, R: r}
	return &readCloser{Reader: streamReader, Closer: r}
}

func (eh *aesCtrEncryptionHandler) Writer(key []byte, w io.WriteCloser) io.WriteCloser {
	blockCipher, _ := aes.NewCipher(key)
	var iv [aes.BlockSize]byte
	stream := cipher.NewCTR(blockCipher, iv[:])
	streamWriter := &cipher.StreamWriter{S: stream, W: w}
	return &writeCloser{Writer: streamWriter, Closer: w}
}

func (eh *aesCtrEncryptionHandler) DeriveKey(material []byte, salt []byte) ([]byte, error) {
	return _scryptDeriveKey(material, salt)
}

var encryptionHandlers = map[EncryptionMethod]EncryptionHandler{
	EncryptionMethodAES_OFB: &ghostbinLegacyEncryptionHandler{},
	EncryptionMethodAES_CTR: &aesCtrEncryptionHandler{},
}

func GetEncryptionHandler(e EncryptionMethod) EncryptionHandler {
	eh, ok := encryptionHandlers[e]
	if !ok {
		return &noopEncryptionHandler{}
	}
	return eh
}
