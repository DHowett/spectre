package pastes

type PasteEncryptedError struct {
	ID ID
}

func (e PasteEncryptedError) Error() string {
	return "Paste " + e.ID.String() + " is encrypted."
}

type PasteInvalidKeyError PasteEncryptedError

func (e PasteInvalidKeyError) Error() string { return "" }

type PasteNotFoundError struct {
	ID ID
}

func (e PasteNotFoundError) Error() string {
	return "Paste " + e.ID.String() + " was not found."
}
