package spectre

import "io"

// Challenger embeds a key or passphrase material.
type Challenger interface {
	// Authenticate should do what its name says it should do.
	Authenticate(salt []byte, challenge []byte) (bool, error)

	// Challenge should return an encryption challenge and a salt; usually, this challenge is an HMAC.
	Challenge() ([]byte, []byte, error)
}

type Cryptor interface {
	Challenger

	// Reader should be implemented by cryptors that can decrypt arbitrary data.
	Reader(io.ReadCloser) (io.ReadCloser, error)

	// Writer should be implemented by cryptors that can encrypt arbitrary data.
	Writer(io.WriteCloser) (io.WriteCloser, error)
}
