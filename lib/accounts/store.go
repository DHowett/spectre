package accounts

import (
	"github.com/DHowett/ghostbin/lib/crypto"
	"github.com/DHowett/ghostbin/lib/pastes"
)

type Store interface {
	GetUserNamed(name string) (User, error)
	GetUserByID(id uint) (User, error)
	CreateUser(name string) (User, error)
	GetChallengeProvider() crypto.ChallengeProvider
}

type User interface {
	GetID() uint
	GetName() string
	UpdateChallenge(password string)
	GetPersona() bool
	SetPersona(persona bool) error
	Check(password string) bool
	Permissions(class PermissionClass, args ...interface{}) PermissionScope
	GetPastes() ([]pastes.ID, error)
}
