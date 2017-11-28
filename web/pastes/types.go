package pastes

import "howett.net/spectre"

type PasteUpdateComplete struct {
	ID spectre.PasteID
}

type PasteDeleteComplete struct {
	ID spectre.PasteID
}

type PasteResponse struct {
	spectre.Paste
	Editable bool
}

type PasteEditResponse struct {
	PasteResponse
}
