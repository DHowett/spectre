package postgres

import (
	"context"
	"database/sql"

	"howett.net/spectre"
)

type userPastePermissionScope struct {
	pPerm *dbUserPastePermission
	err   error

	provider *conn
	ctx      context.Context
}

func newUserPastePermissionScope(ctx context.Context, prov *conn, u *dbUser, id spectre.PasteID) *userPastePermissionScope {
	var pPerm dbUserPastePermission
	err := u.conn.db.GetContext(ctx, &pPerm, `SELECT * FROM user_paste_permissions WHERE user_id = $1 AND paste_id = $2 LIMIT 1`, u.ID, id)
	if err == sql.ErrNoRows {
		pPerm = dbUserPastePermission{
			UserID:  u.ID,
			PasteID: string(id),
		}
		err = nil
	}

	return &userPastePermissionScope{provider: prov, ctx: ctx, pPerm: &pPerm, err: err}
}

func (s *userPastePermissionScope) Has(p spectre.Permission) bool {
	if s.err != nil || s.pPerm == nil {
		return false
	}
	return s.pPerm.Permissions&p != 0
}

func (s *userPastePermissionScope) Grant(p spectre.Permission) error {
	if s.err != nil {
		return s.err
	}

	db := s.provider.db
	row := db.QueryRowContext(s.ctx, `
	INSERT INTO user_paste_permissions(user_id, paste_id, permissions)
	VALUES($1, $2, $3)
	ON CONFLICT(user_id, paste_id)
	DO
		UPDATE SET permissions = user_paste_permissions.permissions | EXCLUDED.permissions
	RETURNING permissions
	`, s.pPerm.UserID, s.pPerm.PasteID, uint32(p))

	var newPerms uint32
	if err := row.Scan(&newPerms); err == nil {
		if s.provider.logger != nil {
			s.provider.logger.Infof("New permission set %x", newPerms)
		}
		s.pPerm.Permissions = spectre.Permission(newPerms)
	} else {
		if s.provider.logger != nil {
			s.provider.logger.Error(err)
		}
		s.err = err
	}

	return s.err
}

func (s *userPastePermissionScope) Revoke(p spectre.Permission) error {
	if s.err != nil {
		return s.err
	}

	if s.pPerm == nil {
		return nil
	}

	pPerm := s.pPerm
	newPerms := pPerm.Permissions & (^p)
	if newPerms == 0 {
		_, s.err = s.provider.db.ExecContext(s.ctx, `DELETE FROM user_paste_permissions WHERE user_id = $1 AND paste_id = $2`, pPerm.UserID, pPerm.PasteID)
	} else {
		_, s.err = s.provider.db.ExecContext(s.ctx, `UPDATE user_paste_permissions SET permissions = $1 WHERE user_id = $2 AND paste_id = $3`, newPerms, pPerm.UserID, pPerm.PasteID)
	}

	if s.err == nil {
		pPerm.Permissions = newPerms
	}
	return s.err
}
