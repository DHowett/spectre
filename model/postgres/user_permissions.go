package postgres

import "github.com/DHowett/ghostbin/model"

type dbUserPermissionScope struct {
	u   *dbUser
	err error
}

func (u *dbUserPermissionScope) Has(p model.Permission) bool {
	return u.u.UserPermissions&p != 0
}

func (u *dbUserPermissionScope) Grant(p model.Permission) error {
	if u.err != nil {
		return u.err
	}
	if err := u.u.broker.Model(u.u).Update(dbUser{UserPermissions: u.u.UserPermissions | p}).Error; err != nil {
		return err
	}
	return nil
}

func (u *dbUserPermissionScope) Revoke(p model.Permission) error {
	if u.err != nil {
		return u.err
	}
	newPerms := u.u.UserPermissions & (^p)
	if err := u.u.broker.Model(u.u).Update("UserPermissions", newPerms).Error; err != nil {
		return err
	}
	return nil
}
