package model

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
