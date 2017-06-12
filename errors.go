package spectre

import "errors"

var (
	ErrChallengeRejected = errors.New("challenge rejected")
	ErrCryptorRequired   = errors.New("cryptor required")
	ErrNotFound          = errors.New("record not found")
)
