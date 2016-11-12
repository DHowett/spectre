package model

import "errors"

var (
	PasteInvalidKeyError = errors.New("invalid password")
	PasteEncryptedError  = errors.New("paste encrypted")
	PasteNotFoundError   = errors.New("paste not found")
)
