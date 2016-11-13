package model

type UserSource int

const (
	UserSourceUnknown             = -1
	UserSourceGhostbin UserSource = iota
	UserSourceMozillaPersona
)

type User interface {
	GetID() uint
	GetName() string

	GetSource() UserSource
	SetSource(UserSource)

	UpdateChallenge(password string)
	Check(password string) bool

	Permissions(class PermissionClass, args ...interface{}) PermissionScope

	GetPastes() ([]PasteID, error)
}
