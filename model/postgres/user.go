package postgres

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"time"

	"github.com/DHowett/ghostbin/model"
)

type dbUserPastePermission struct {
	UserID      uint             `db:"user_id"`
	PasteID     string           `db:"paste_id"`
	Permissions model.Permission `db:"permissions"`
}

type dbUser struct {
	ID        uint
	UpdatedAt time.Time `db:"updated_at"`

	Name      string
	Salt      []byte
	Challenge []byte

	Source model.UserSource

	UserPermissions  model.Permission `db:"permissions"`
	PastePermissions []*dbUserPastePermission

	provider *provider
}

func (u *dbUser) GetID() uint {
	return u.ID
}

func (u *dbUser) GetName() string {
	return u.Name
}

func (u *dbUser) GetSource() model.UserSource {
	return u.Source
}

func (u *dbUser) SetSource(source model.UserSource) {
	tx, _ := u.provider.DB.BeginTxx(context.TODO(), nil)
	if _, err := tx.ExecContext(context.TODO(), `UPDATE users SET source = $1 WHERE id = $2`, source, u.ID); err != nil {
		tx.Rollback()
	}
	u.Source = source
	tx.Commit()
}

func (u *dbUser) UpdateChallenge(password string) {
	tx, _ := u.provider.DB.BeginTxx(context.TODO(), nil)
	challengeProvider := u.provider.ChallengeProvider

	salt := challengeProvider.RandomSalt()
	key := challengeProvider.DeriveKey(password, salt)

	challengeMessage := append(salt, []byte(u.Name)...)
	challenge := challengeProvider.Challenge(challengeMessage, key)

	if _, err := tx.ExecContext(context.TODO(), `UPDATE users SET salt = $1, challenge = $2 WHERE id = $3`, salt, challenge, u.ID); err != nil {
		tx.Rollback()
		return
	}

	u.Salt = salt
	u.Challenge = challenge

	tx.Commit()
}

func (u *dbUser) Check(password string) bool {
	salt := u.Salt
	if salt == nil {
		return false
	}
	challengeProvider := u.provider.ChallengeProvider
	key := challengeProvider.DeriveKey(password, salt)
	challengeMessage := append(salt, []byte(u.Name)...)
	newChallenge := challengeProvider.Challenge(challengeMessage, key)
	return subtle.ConstantTimeCompare(newChallenge, u.Challenge) == 1
}

func (u *dbUser) Permissions(class model.PermissionClass, args ...interface{}) model.PermissionScope {
	switch class {
	case model.PermissionClassUser:
		return &dbUserPermissionScope{u, nil}
	case model.PermissionClassPaste:
		var pid model.PasteID
		switch idt := args[0].(type) {
		case string:
			pid = model.PasteIDFromString(idt)
		case model.PasteID:
			pid = idt
		default:
			return nil
		}
		return newUserPastePermissionScope(u.provider, u, pid)
	}
	return nil
}

func (u *dbUser) GetPastes() ([]model.PasteID, error) {
	var ids []string
	if err := u.provider.DB.SelectContext(context.TODO(), &ids, `SELECT paste_id FROM user_paste_permissions WHERE user_id = $1 AND permissions > 0`, u.ID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // A user having no pastes is no error
		}
		return nil, err
	}

	pids := make([]model.PasteID, len(ids))
	for i, v := range ids {
		pids[i] = model.PasteIDFromString(v)
	}
	return pids, nil
}
