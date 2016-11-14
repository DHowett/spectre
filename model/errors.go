package model

import "errors"

var (
	ErrInvalidKey     = errors.New("invalid password")
	ErrPasteEncrypted = errors.New("paste encrypted")
	ErrNotFound       = errors.New("record not found")
)
