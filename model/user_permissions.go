package model

type dbUserPermissionScope struct {
	u   *dbUser
	err error
}

func (u *dbUserPermissionScope) Has(p Permission) bool {
	return u.u.UserPermissions&p != 0
}

func (u *dbUserPermissionScope) Grant(p Permission) error {
	if u.err != nil {
		return u.err
	}
	if err := u.u.broker.Model(u.u).Update(dbUser{UserPermissions: u.u.UserPermissions | p}).Error; err != nil {
		return err
	}
	return nil
}

func (u *dbUserPermissionScope) Revoke(p Permission) error {
	if u.err != nil {
		return u.err
	}
	newPerms := u.u.UserPermissions & (^p)
	if err := u.u.broker.Model(u.u).Update("UserPermissions", newPerms).Error; err != nil {
		return err
	}
	return nil
}
