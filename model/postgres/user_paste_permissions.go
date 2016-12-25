package postgres

import "github.com/DHowett/ghostbin/model"

type userPastePermissionScope struct {
	pPerm *dbUserPastePermission
	err   error

	provider *provider
}

func newUserPastePermissionScope(prov *provider, u *dbUser, id model.PasteID) *userPastePermissionScope {
	var pPerm dbUserPastePermission
	err := u.provider.FirstOrInit(&pPerm, dbUserPastePermission{UserID: u.ID, PasteID: id.String()}).Error
	return &userPastePermissionScope{provider: prov, pPerm: &pPerm, err: err}
}

func (s *userPastePermissionScope) Has(p model.Permission) bool {
	if s.err != nil || s.pPerm == nil {
		return false
	}
	return s.pPerm.Permissions&p != 0
}

func (s *userPastePermissionScope) Grant(p model.Permission) error {
	if s.err != nil {
		return s.err
	}

	db := s.provider.DB
	row := db.CommonDB().QueryRow(`
	INSERT INTO user_paste_permissions(user_id, paste_id, permissions)
	VALUES($1, $2, $3)
	ON CONFLICT(user_id, paste_id)
	DO
		UPDATE SET permissions = user_paste_permissions.permissions | EXCLUDED.permissions
	RETURNING permissions
	`, s.pPerm.UserID, s.pPerm.PasteID, uint32(p))

	var newPerms uint32
	if err := row.Scan(&newPerms); err == nil {
		if s.provider.Logger != nil {
			s.provider.Logger.Infof("New permission set %x", newPerms)
		}
		s.pPerm.Permissions = model.Permission(newPerms)
	} else {
		if s.provider.Logger != nil {
			s.provider.Logger.Error(err)
		}
		s.err = err
	}

	return s.err
}

func (s *userPastePermissionScope) Revoke(p model.Permission) error {
	if s.err != nil {
		return s.err
	}

	if s.pPerm == nil {
		return nil
	}

	pPerm := s.pPerm
	newPerms := pPerm.Permissions & (^p)
	if newPerms == 0 {
		s.err = s.provider.Delete(pPerm).Error
	} else {
		s.err = s.provider.Model(pPerm).Update("Permissions", newPerms).Error
	}

	if s.err == nil {
		pPerm.Permissions = newPerms
	}
	return s.err
}
