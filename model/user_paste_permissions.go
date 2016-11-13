package model

import "github.com/DHowett/ghostbin/lib/sql/querybuilder"

type userPastePermissionScope struct {
	pPerm *dbUserPastePermission
	err   error

	broker *dbBroker
}

func newUserPastePermissionScope(broker *dbBroker, u *dbUser, id PasteID) *userPastePermissionScope {
	var pPerm dbUserPastePermission
	err := u.broker.FirstOrInit(&pPerm, dbUserPastePermission{UserID: u.ID, PasteID: id.String()}).Error
	return &userPastePermissionScope{broker: broker, pPerm: &pPerm, err: err}
}

func (s *userPastePermissionScope) Has(p Permission) bool {
	if s.err != nil || s.pPerm == nil {
		return false
	}
	return s.pPerm.Permissions&p != 0
}

func (s *userPastePermissionScope) Grant(p Permission) error {
	if s.err != nil {
		return s.err
	}

	newPerms := s.pPerm.Permissions | p

	db := s.broker.DB
	scope := db.NewScope(s.pPerm)
	modelStruct := scope.GetModelStruct()
	table := modelStruct.TableName(db)

	query, err := s.broker.QB.Build(&querybuilder.UpsertQuery{
		Table:        table,
		ConflictKeys: []string{"user_id", "paste_id"},
		Fields:       []string{"user_id", "paste_id", "permissions"},
	})

	if err != nil {
		s.err = err
		return err
	}

	_, s.err = db.CommonDB().Exec(query, s.pPerm.UserID, s.pPerm.PasteID, newPerms)
	if s.err == nil {
		s.pPerm.Permissions = newPerms
	}
	return s.err
}

func (s *userPastePermissionScope) Revoke(p Permission) error {
	if s.err != nil {
		return s.err
	}

	if s.pPerm == nil {
		return nil
	}

	pPerm := s.pPerm
	newPerms := pPerm.Permissions & (^p)
	if newPerms == 0 {
		s.err = s.broker.Delete(pPerm).Error
	} else {
		s.err = s.broker.Model(pPerm).Update("Permissions", newPerms).Error
	}

	if s.err == nil {
		pPerm.Permissions = newPerms
	}
	return s.err
}
