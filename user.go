package spectre

import "context"

type UserSource int

const (
	UserSourceUnknown  UserSource = -1
	UserSourceGhostbin            = iota
	UserSourceMozillaPersona
)

type User interface {
	GetID() uint
	GetName() string

	GetSource() UserSource
	SetSource(UserSource)

	UpdateChallenge(challenger Challenger)
	Check(challenger Challenger) bool

	Permissions(class PermissionClass, args ...interface{}) PermissionScope

	GetPastes() ([]PasteID, error)

	Commit() error
	Erase() error
}

type UserService interface {
	GetUserNamed(context.Context, string) (User, error)
	GetUserByID(context.Context, uint) (User, error)
	CreateUser(context.Context, string) (User, error)
}
