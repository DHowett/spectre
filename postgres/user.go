package postgres

import (
	"context"
	"database/sql"
	"time"

	"howett.net/spectre"
)

type dbUserPastePermission struct {
	UserID      uint               `db:"user_id"`
	PasteID     string             `db:"paste_id"`
	Permissions spectre.Permission `db:"permissions"`
}

type dbUser struct {
	ID        uint
	UpdatedAt time.Time `db:"updated_at"`

	Name      string
	Salt      []byte
	Challenge []byte

	Source spectre.UserSource

	UserPermissions  spectre.Permission `db:"permissions"`
	PastePermissions []*dbUserPastePermission

	conn *conn
	ctx  context.Context
}

func (u *dbUser) GetID() uint {
	return u.ID
}

func (u *dbUser) GetName() string {
	return u.Name
}

func (u *dbUser) GetSource() spectre.UserSource {
	return u.Source
}

func (u *dbUser) SetSource(source spectre.UserSource) {
	tx, _ := u.conn.db.BeginTxx(u.ctx, nil)
	if _, err := tx.ExecContext(u.ctx, `UPDATE users SET source = $1 WHERE id = $2`, source, u.ID); err != nil {
		tx.Rollback()
	}
	u.Source = source
	tx.Commit()
}

func (u *dbUser) UpdateChallenge(ch spectre.Challenger) {
	tx, _ := u.conn.db.BeginTxx(u.ctx, nil)

	// TODO(DH) REMEMBER USER CHALLENGE IS SALT+USERNAME HMAC'D
	challenge, salt, err := ch.Challenge()
	if err != nil {
		panic(err) //TODO(DH)
	}

	if _, err = tx.ExecContext(u.ctx, `UPDATE users SET salt = $1, challenge = $2 WHERE id = $3`, salt, challenge, u.ID); err != nil {
		tx.Rollback()
		return
	}

	u.Salt = salt
	u.Challenge = challenge

	tx.Commit()
}

func (u *dbUser) Check(ch spectre.Challenger) bool {
	ok, _ := ch.Authenticate(u.Salt, u.Challenge)
	return ok
}

func (u *dbUser) Permissions(class spectre.PermissionClass, args ...interface{}) spectre.PermissionScope {
	switch class {
	case spectre.PermissionClassUser:
		return &dbUserPermissionScope{u, nil}
	case spectre.PermissionClassPaste:
		var pid spectre.PasteID
		switch idt := args[0].(type) {
		case string:
			pid = spectre.PasteID(idt)
		case spectre.PasteID:
			pid = idt
		default:
			return nil
		}
		return newUserPastePermissionScope(u.ctx, u.conn, u, pid)
	}
	return nil
}

func (u *dbUser) GetPastes() ([]spectre.PasteID, error) {
	var ids []string
	if err := u.conn.db.SelectContext(u.ctx, &ids, `SELECT paste_id FROM user_paste_permissions WHERE user_id = $1 AND permissions > 0`, u.ID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // A user having no pastes is no error
		}
		return nil, err
	}

	pids := make([]spectre.PasteID, len(ids))
	for i, v := range ids {
		pids[i] = spectre.PasteID(v)
	}
	return pids, nil
}

func (u *dbUser) Commit() error {
	// TODO(DH)
	return nil
}

func (u *dbUser) Erase() error {
	// TODO(DH)
	return nil
}
