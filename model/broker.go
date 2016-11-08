package model

import "github.com/DHowett/ghostbin/lib/crypto"

type Broker interface {
	// Generic
	GetChallengeProvider() crypto.ChallengeProvider

	// User Management
	GetUserNamed(name string) (User, error)
	GetUserByID(id uint) (User, error)
	CreateUser(name string) (User, error)

	// Pastes
	GenerateNewPasteID(bool) PasteID
	CreatePaste() (Paste, error)
	CreateEncryptedPaste(PasteEncryptionMethod, []byte) (Paste, error)
	GetPaste(PasteID, []byte) (Paste, error)
	GetPastes([]PasteID) ([]Paste, error)

	// Grants
	CreateGrant(Paste) (Grant, error)
	GetGrant(GrantID) (Grant, error)
	//DestroyGrant(GrantID)
}
