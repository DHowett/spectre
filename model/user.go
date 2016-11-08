package model

type User interface {
	GetID() uint
	GetName() string
	UpdateChallenge(password string)
	GetPersona() bool
	SetPersona(persona bool) error
	Check(password string) bool
	Permissions(class PermissionClass, args ...interface{}) PermissionScope
	GetPastes() ([]PasteID, error)
}
