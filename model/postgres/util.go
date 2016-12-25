package postgres

import (
	"crypto/rand"
	"encoding/base32"
	"io"
)

func generateRandomBytes(nbytes int) ([]byte, error) {
	uuid := make([]byte, nbytes)
	n, err := rand.Read(uuid)
	if n != len(uuid) || err != nil {
		return []byte{}, err
	}

	return uuid, nil
}

var base32Encoder = base32.NewEncoding("abcdefghjkmnopqrstuvwxyz23456789")

func generateRandomBase32String(nbytes, outlen int) (string, error) {
	uuid, err := generateRandomBytes(nbytes)
	if err != nil {
		return "", err
	}

	s := base32Encoder.EncodeToString(uuid)
	if outlen == -1 {
		outlen = len(s)
	}

	return s[0:outlen], nil
}

type _devZero struct{}

func (z *_devZero) Read(p []byte) (n int, err error) {
	return 0, io.EOF
}

func (z *_devZero) Close() error {
	return nil
}

var devZero = &_devZero{}
