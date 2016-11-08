package model

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"io"

	"golang.org/x/crypto/scrypt"
)

type PasteEncryptionMethod uint

const (
	PasteEncryptionMethodNone    PasteEncryptionMethod = iota
	PasteEncryptionMethodAES_OFB                       // deprecated
	PasteEncryptionMethodAES_CTR
)

type PasteEncryptionCodec interface {
	Authenticate(PasteID, []byte, []byte, []byte) bool
	GenerateHMAC(PasteID, []byte, []byte) []byte
	Reader([]byte, io.ReadCloser) io.ReadCloser
	Writer([]byte, io.WriteCloser) io.WriteCloser
	DeriveKey([]byte, []byte) ([]byte, error)
}

type noopEncryptionCodec struct{}

func (eh *noopEncryptionCodec) Authenticate(id PasteID, salt []byte, key []byte, hmac []byte) bool {
	return true
}

func (eh *noopEncryptionCodec) GenerateHMAC(id PasteID, salt []byte, key []byte) []byte {
	return nil
}

func (eh *noopEncryptionCodec) Reader(key []byte, r io.ReadCloser) io.ReadCloser {
	return r
}

func (eh *noopEncryptionCodec) Writer(key []byte, w io.WriteCloser) io.WriteCloser {
	return w
}

func (eh *noopEncryptionCodec) DeriveKey(material []byte, salt []byte) ([]byte, error) {
	return nil, nil
}

func _scryptDeriveKey(material []byte, salt []byte) ([]byte, error) {
	return scrypt.Key(material, salt, 16384, 8, 1, 32)
}

type ghostbinLegacyEncryptionCodec struct{}

func (eh *ghostbinLegacyEncryptionCodec) Authenticate(id PasteID, salt []byte, key []byte, messageMAC []byte) bool {
	return hmac.Equal(messageMAC, eh.GenerateHMAC(id, salt, key))
}
func (eh *ghostbinLegacyEncryptionCodec) GenerateHMAC(id PasteID, salt []byte, key []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(id.String()))
	return mac.Sum(nil)
}

func (eh *ghostbinLegacyEncryptionCodec) Reader(key []byte, r io.ReadCloser) io.ReadCloser {
	blockCipher, _ := aes.NewCipher(key)
	var iv [aes.BlockSize]byte
	stream := cipher.NewOFB(blockCipher, iv[:])
	streamReader := &cipher.StreamReader{S: stream, R: r}
	return &readCloser{Reader: streamReader, Closer: r}
}

func (eh *ghostbinLegacyEncryptionCodec) Writer(key []byte, w io.WriteCloser) io.WriteCloser {
	blockCipher, _ := aes.NewCipher(key)
	var iv [aes.BlockSize]byte
	stream := cipher.NewOFB(blockCipher, iv[:])
	streamWriter := &cipher.StreamWriter{S: stream, W: w}
	return &writeCloser{Writer: streamWriter, Closer: w}
}

func (eh *ghostbinLegacyEncryptionCodec) DeriveKey(material []byte, salt []byte) ([]byte, error) {
	return _scryptDeriveKey(material, salt)
}

type aesCtrEncryptionCodec struct{}

func (eh *aesCtrEncryptionCodec) Authenticate(id PasteID, salt []byte, key []byte, messageMAC []byte) bool {
	return hmac.Equal(messageMAC, eh.GenerateHMAC(id, salt, key))
}
func (eh *aesCtrEncryptionCodec) GenerateHMAC(id PasteID, salt []byte, key []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(id.String()))
	mac.Write(salt)
	return mac.Sum(nil)
}

func (eh *aesCtrEncryptionCodec) Reader(key []byte, r io.ReadCloser) io.ReadCloser {
	blockCipher, _ := aes.NewCipher(key)
	var iv [aes.BlockSize]byte
	stream := cipher.NewCTR(blockCipher, iv[:])
	streamReader := &cipher.StreamReader{S: stream, R: r}
	return &readCloser{Reader: streamReader, Closer: r}
}

func (eh *aesCtrEncryptionCodec) Writer(key []byte, w io.WriteCloser) io.WriteCloser {
	blockCipher, _ := aes.NewCipher(key)
	var iv [aes.BlockSize]byte
	stream := cipher.NewCTR(blockCipher, iv[:])
	streamWriter := &cipher.StreamWriter{S: stream, W: w}
	return &writeCloser{Writer: streamWriter, Closer: w}
}

func (eh *aesCtrEncryptionCodec) DeriveKey(material []byte, salt []byte) ([]byte, error) {
	return _scryptDeriveKey(material, salt)
}

var pasteEncryptionCodecs = map[PasteEncryptionMethod]PasteEncryptionCodec{
	PasteEncryptionMethodAES_OFB: &ghostbinLegacyEncryptionCodec{},
	PasteEncryptionMethodAES_CTR: &aesCtrEncryptionCodec{},
}

func getPasteEncryptionCodec(e PasteEncryptionMethod) PasteEncryptionCodec {
	eh, ok := pasteEncryptionCodecs[e]
	if !ok {
		return &noopEncryptionCodec{}
	}
	return eh
}
