package model

const (
	UserPermissionUnknown Permission = (0)
	UserPermissionAdmin              = (1 << (iota - 1))

	UserPermissionAll Permission = Permission(^uint32(0))
)

const (
	PastePermissionUnknown Permission = 0
	PastePermissionEdit               = (1 << (iota - 1))
	PastePermissionGrant

	PastePermissionAll Permission = Permission(^uint32(0))
)

type Permission uint64
type PermissionScope interface {
	Has(Permission) bool
	Grant(Permission) error
	Revoke(Permission) error
}

type PermissionClass int

const (
	PermissionClassUser PermissionClass = iota + 1
	PermissionClassPaste
)
