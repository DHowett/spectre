package postgres

import "howett.net/spectre"

type dbUserPermissionScope struct {
	u   *dbUser
	err error
}

func (u *dbUserPermissionScope) Has(p spectre.Permission) bool {
	return u.u.UserPermissions&p != 0
}

func (u *dbUserPermissionScope) set(newPerms spectre.Permission) error {
	if u.err != nil {
		return u.err
	}
	if _, err := u.u.provider.DB.ExecContext(u.u.ctx, `UPDATE users SET permissions = $1 WHERE id = $2`, newPerms, u.u.ID); err != nil {
		return err
	}

	u.u.UserPermissions = newPerms
	return nil
}

func (u *dbUserPermissionScope) Grant(p spectre.Permission) error {
	return u.set(u.u.UserPermissions | p)
}

func (u *dbUserPermissionScope) Revoke(p spectre.Permission) error {
	return u.set(u.u.UserPermissions & (^p))
}
