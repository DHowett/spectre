package postgres

import (
	"crypto/subtle"
	"time"

	"github.com/DHowett/ghostbin/model"
)

type dbUserPastePermission struct {
	UserID      uint   `gorm:"unique_index:uix_user_paste_perm"`
	PasteID     string `gorm:"unique_index:uix_user_paste_perm;type:varchar(256)"`
	Permissions model.Permission
}

// gorm
func (dbUserPastePermission) TableName() string {
	return "user_paste_permissions"
}

type dbUser struct {
	ID        uint `gorm:"primary_key"`
	UpdatedAt time.Time

	Name      string `gorm:"type:varchar(512);unique_index"`
	Salt      []byte
	Challenge []byte

	Source model.UserSource

	UserPermissions  model.Permission `gorm:"column:permissions"`
	PastePermissions []*dbUserPastePermission

	broker *dbBroker
}

// gorm
func (dbUser) TableName() string {
	return "users"
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
	tx := u.broker.Begin().Model(u)
	if err := tx.Updates(map[string]interface{}{"Source": source}).Error; err != nil {
		tx.Rollback()
	}
	tx.Commit()
}

func (u *dbUser) UpdateChallenge(password string) {
	tx := u.broker.Begin().Model(u)
	challengeProvider := u.broker.ChallengeProvider

	salt := challengeProvider.RandomSalt()
	key := challengeProvider.DeriveKey(password, salt)

	challengeMessage := append(salt, []byte(u.Name)...)
	challenge := challengeProvider.Challenge(challengeMessage, key)

	if err := tx.Updates(map[string]interface{}{"Salt": salt, "Challenge": challenge}).Error; err != nil {
		tx.Rollback()
		u.Salt = nil
		u.Challenge = nil
		return
	}

	tx.Commit()
}

func (u *dbUser) Check(password string) bool {
	salt := u.Salt
	if salt == nil {
		return false
	}
	challengeProvider := u.broker.ChallengeProvider
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
		return newUserPastePermissionScope(u.broker, u, pid)
	}
	return nil
}

func (u *dbUser) GetPastes() ([]model.PasteID, error) {
	var ids []string
	if err := u.broker.Model(&dbUserPastePermission{}).Where("user_id = ? AND permissions > 0", u.ID).Pluck("paste_id", &ids).Error; err != nil {
		return nil, err
	}
	pids := make([]model.PasteID, len(ids))
	for i, v := range ids {
		pids[i] = model.PasteIDFromString(v)
	}
	return pids, nil
}
