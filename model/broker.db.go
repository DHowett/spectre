package model

import (
	"database/sql"
	"errors"

	"github.com/DHowett/ghostbin/lib/crypto"
	"github.com/DHowett/ghostbin/lib/sql/querybuilder"
	"github.com/jinzhu/gorm"
)

type dbBroker struct {
	*gorm.DB
	qb querybuilder.QueryBuilder

	//db                *gorm.DB
	challengeProvider crypto.ChallengeProvider
}

// User
func (broker *dbBroker) getUserWithQuery(query string, args ...interface{}) (User, error) {
	var u dbUser
	if err := broker.Where(query, args...).First(&u).Error; err != nil {
		return nil, err
	}
	u.broker = broker
	return &u, nil
}

func (broker *dbBroker) GetUserNamed(name string) (User, error) {
	u, err := broker.getUserWithQuery("name = ?", name)
	return u, err
}

func (broker *dbBroker) GetUserByID(id uint) (User, error) {
	return broker.getUserWithQuery("id = ?", id)
}

func (broker *dbBroker) CreateUser(name string) (User, error) {
	u := &dbUser{
		Name:   name,
		broker: broker,
	}
	if err := broker.Create(u).Error; err != nil {
		return nil, err
	}
	return u, nil
}

func (broker *dbBroker) GetChallengeProvider() crypto.ChallengeProvider {
	return broker.challengeProvider
}

// Paste
func (broker *dbBroker) GenerateNewPasteID(encrypted bool) PasteID {
	nbytes, idlen := 4, 5
	if encrypted {
		nbytes, idlen = 5, 8
	}

	for {
		s, _ := generateRandomBase32String(nbytes, idlen)
		return PasteIDFromString(s)
	}
}

func (broker *dbBroker) CreatePaste() (Paste, error) {
	paste := dbPaste{broker: broker}
	for {
		if err := broker.Create(&paste).Error; err != nil {
			panic(err)
		}
		paste.broker = broker
		return &paste, nil
	}
}

func (broker *dbBroker) CreateEncryptedPaste(method PasteEncryptionMethod, passphraseMaterial []byte) (Paste, error) {
	if passphraseMaterial == nil {
		return nil, errors.New("FilesystemPasteStore: unacceptable encryption material")
	}
	paste := dbPaste{broker: broker}
	paste.EncryptionSalt, _ = generateRandomBytes(16)
	paste.EncryptionMethod = PasteEncryptionMethodAES_CTR
	key, err := getPasteEncryptionCodec(method).DeriveKey(passphraseMaterial, paste.EncryptionSalt)
	if err != nil {
		return nil, err
	}
	paste.encryptionKey = key

	for {
		if err := broker.Create(&paste).Error; err != nil {
			panic(err)
		}
		paste.broker = broker
		return &paste, nil
	}
}

func (broker *dbBroker) GetPaste(id PasteID, passphraseMaterial []byte) (Paste, error) {
	var paste dbPaste
	if err := broker.Find(&paste, "id = ?", id.String()).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, PasteNotFoundError
		}

		return nil, err
	}
	paste.broker = broker

	// This paste is encrypted
	if paste.IsEncrypted() {
		// If they haven't requested decryption, we can
		// still tell them that a paste exists.
		// It will be a stub/placeholder that only has an ID.
		if passphraseMaterial == nil {
			return &encryptedPastePlaceholder{
				ID: id,
			}, PasteEncryptedError
		}

		key, err := getPasteEncryptionCodec(paste.EncryptionMethod).DeriveKey(passphraseMaterial, paste.EncryptionSalt)
		if err != nil {
			return nil, PasteEncryptedError
		}

		ok := getPasteEncryptionCodec(paste.EncryptionMethod).Authenticate(id, paste.EncryptionSalt, key, paste.HMAC)
		if !ok {
			return nil, PasteInvalidKeyError
		}

		paste.encryptionKey = key
	}

	return &paste, nil
}

func (broker *dbBroker) GetPastes(ids []PasteID) ([]Paste, error) {
	stringIDs := make([]string, len(ids))
	for i, v := range ids {
		stringIDs[i] = string(v)
	}

	var ps []*dbPaste
	if err := broker.Find(&ps, "id in (?)", stringIDs).Error; err != nil {
		return nil, err
	}

	iPastes := make([]Paste, len(ps))
	for i, p := range ps {
		p.broker = broker
		if p.IsEncrypted() {
			iPastes[i] = &encryptedPastePlaceholder{
				ID: p.GetID(),
			}
		} else {
			iPastes[i] = p
		}
	}
	return iPastes, nil
}

func (broker *dbBroker) CreateGrant(paste Paste) (Grant, error) {
	grant := dbGrant{PasteID: paste.GetID().String(), broker: broker}
	for {
		if err := broker.Create(&grant).Error; err != nil {
			panic(err)
		}
		grant.broker = broker
		return &grant, nil
	}
}

func (broker *dbBroker) GetGrant(id GrantID) (Grant, error) {
	var grant dbGrant
	if err := broker.Find(&grant, "id = ?", string(id)).Error; err != nil {
		return nil, err
	}
	grant.broker = broker
	return &grant, nil
}

func NewDatabaseBroker(dialect string, sqlDb *sql.DB, challengeProvider crypto.ChallengeProvider) (Broker, error) {
	db, err := gorm.Open(dialect, sqlDb)
	if err != nil {
		return nil, err
	}
	db = db.Debug()

	interfacesToMigrate := []interface{}{
		&dbPaste{},
		&dbPasteBody{},
		&dbUser{},
		&dbUserPastePermission{},
		&dbGrant{},
	}

	if err := db.AutoMigrate(interfacesToMigrate...).Error; err != nil {
		return nil, err
	}

	return &dbBroker{
		qb:                querybuilder.New(dialect),
		DB:                db,
		challengeProvider: challengeProvider,
	}, nil
}
