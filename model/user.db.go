package model

import (
	"crypto/subtle"
	"time"
)

type dbUserPastePermission struct {
	UserID      uint   `gorm:"unique_index:uix_user_paste_perm"`
	PasteID     string `gorm:"unique_index:uix_user_paste_perm;type:varchar(256)"`
	Permissions Permission
}

type dbUser struct {
	ID        uint `gorm:"primary_key"`
	UpdatedAt time.Time

	Name      string `gorm:"type:varchar(512);unique_index"`
	Salt      []byte
	Challenge []byte

	Source UserSource

	UserPermissions  Permission `gorm:"column:permissions"`
	PastePermissions []*dbUserPastePermission

	broker *dbBroker
}

func (u *dbUser) GetID() uint {
	return u.ID
}

func (u *dbUser) GetName() string {
	return u.Name
}

func (u *dbUser) GetSource() UserSource {
	return u.Source
}

func (u *dbUser) SetSource(source UserSource) {
	u.Source = source
}

func (u *dbUser) UpdateChallenge(password string) {
	tx := u.broker.Begin().Model(u)
	challengeProvider := u.broker.GetChallengeProvider()

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

func (u *dbUser) GetPersona() bool {
	return u.Persona
}

func (u *dbUser) SetPersona(persona bool) error {
	return u.broker.Model(u).Update("persona", persona).Error
}

func (u *dbUser) Check(password string) bool {
	salt := u.Salt
	if salt == nil {
		return false
	}
	challengeProvider := u.broker.GetChallengeProvider()
	key := challengeProvider.DeriveKey(password, salt)
	challengeMessage := append(salt, []byte(u.Name)...)
	newChallenge := challengeProvider.Challenge(challengeMessage, key)
	return subtle.ConstantTimeCompare(newChallenge, u.Challenge) == 1
}

func (u *dbUser) Permissions(class PermissionClass, args ...interface{}) PermissionScope {
	switch class {
	case PermissionClassUser:
		return &dbUserPermissionScope{u, nil}
	case PermissionClassPaste:
		var pid PasteID
		switch idt := args[0].(type) {
		case string:
			pid = PasteIDFromString(idt)
		case PasteID:
			pid = idt
		default:
			return nil
		}
		return newUserPastePermissionScope(u.broker, u, pid)
	}
	return nil
}

func (u *dbUser) GetPastes() ([]PasteID, error) {
	var ids []string
	if err := u.broker.Model(&dbUserPastePermission{}).Where("user_id = ? AND permissions > 0", u.ID).Pluck("paste_id", &ids).Error; err != nil {
		return nil, err
	}
	pids := make([]PasteID, len(ids))
	for i, v := range ids {
		pids[i] = PasteIDFromString(v)
	}
	return pids, nil
}
