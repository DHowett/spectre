package model

import (
	"database/sql"
	"errors"

	"github.com/DHowett/ghostbin/lib/crypto"
	"github.com/DHowett/ghostbin/lib/sql/querybuilder"
	"github.com/Sirupsen/logrus"
	"github.com/jinzhu/gorm"
)

type dbBroker struct {
	*gorm.DB
	Logger            logrus.FieldLogger
	QB                querybuilder.QueryBuilder
	ChallengeProvider crypto.ChallengeProvider
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
			return nil, ErrNotFound
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
			}, ErrPasteEncrypted
		}

		key, err := getPasteEncryptionCodec(paste.EncryptionMethod).DeriveKey(passphraseMaterial, paste.EncryptionSalt)
		if err != nil {
			return nil, ErrPasteEncrypted
		}

		ok := getPasteEncryptionCodec(paste.EncryptionMethod).Authenticate(id, paste.EncryptionSalt, key, paste.HMAC)
		if !ok {
			return nil, ErrInvalidKey
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

func (broker *dbBroker) GetExpiringPastes() ([]ExpiringPaste, error) {
	var ps []*dbPaste
	if err := broker.Not("expire_at", "NULL").Select("id, expire_at").Find(&ps).Error; err != nil {
		return nil, err
	}

	eps := make([]ExpiringPaste, len(ps))
	for i, p := range ps {
		eps[i] = ExpiringPaste{
			PasteID: PasteID(p.ID),
			Time:    *p.ExpireAt,
		}
	}
	return eps, nil
}

func (broker *dbBroker) DestroyPaste(id PasteID) error {
	// TODO(DH): Convert these manual cascades into FK constraints.
	tx := broker.Begin()
	if err := tx.Delete(&dbPaste{ID: id.String()}).Error; err != nil && err != gorm.ErrRecordNotFound {
		tx.Rollback()
		return err
	}

	if err := tx.Delete(&dbPasteBody{PasteID: id.String()}).Error; err != nil && err != gorm.ErrRecordNotFound {
		tx.Rollback()
		return err
	}

	userPastePermissionScope := broker.NewScope(&dbUserPastePermission{})
	userPastePermissionModelStruct := userPastePermissionScope.GetModelStruct()
	userPastePermissionTableName := userPastePermissionModelStruct.TableName(broker.DB)
	if _, err := tx.CommonDB().Exec("DELETE FROM "+userPastePermissionTableName+" WHERE paste_id = ?", id.String()); err != nil && err != sql.ErrNoRows {
		tx.Rollback()
		return err
	}

	return tx.Commit().Error
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

func (broker *dbBroker) ReportPaste(p Paste) error {
	pID := p.GetID()
	result, err := broker.CommonDB().Exec("UPDATE paste_reports SET count = count + 1 WHERE paste_id = ?", pID.String())
	if nrows, _ := result.RowsAffected(); nrows == 0 {
		_, err = broker.CommonDB().Exec("INSERT INTO paste_reports(paste_id, count) VALUES(?, 1)", pID.String())
		return err
	}

	return err
}

func (broker *dbBroker) GetReport(pID PasteID) (Report, error) {
	row := broker.CommonDB().QueryRow("SELECT count FROM paste_reports WHERE paste_id = ?", pID.String())

	var count int
	err := row.Scan(&count)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	} else if err != nil {
		// TODO(DH) errors?
		return nil, err
	}

	return &dbReport{
		PasteID: pID.String(),
		Count:   count,
		broker:  broker,
	}, nil
}

func (broker *dbBroker) GetReports() ([]Report, error) {
	reports := make([]Report, 0, 16)

	rows, err := broker.CommonDB().Query("SELECT paste_id, count FROM paste_reports")
	if err != nil {
		return nil, err
	}

	defer rows.Close()
	for rows.Next() {
		r := &dbReport{broker: broker}
		rows.Scan(&r.PasteID, &r.Count)
		reports = append(reports, r)
	}
	return reports, rows.Err()
}

func (broker *dbBroker) setLoggerOption(log logrus.FieldLogger) {
	broker.Logger = log
}

func (broker *dbBroker) setDebugOption(debug bool) {
	// no-op
}

func NewDatabaseBroker(dialect string, sqlDb *sql.DB, challengeProvider crypto.ChallengeProvider, options ...Option) (Broker, error) {
	if dialect == "sqlite" || dialect == "sqlite3" {
		sqlDb.Exec("PRAGMA foreign_keys = ON")
	}

	db, err := gorm.Open(dialect, sqlDb)
	if err != nil {
		return nil, err
	}

	broker := &dbBroker{
		DB:                db,
		QB:                querybuilder.New(dialect),
		ChallengeProvider: challengeProvider,
	}

	for _, opt := range options {
		opt(broker)
	}

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

	pasteScope := db.NewScope(&dbPaste{})
	pasteModelStruct := pasteScope.GetModelStruct()
	pasteTableName := pasteModelStruct.TableName(db)

	_, err = sqlDb.Exec(
		`CREATE TABLE IF NOT EXISTS paste_reports(
    paste_id VARCHAR(256) PRIMARY KEY REFERENCES ` + pasteTableName + `(id) ON DELETE CASCADE,
    count int DEFAULT 0
)`)
	if err != nil {
		return nil, err
	}

	res, err := sqlDb.Exec(
		`DELETE FROM pastes WHERE expire_at < CURRENT_TIMESTAMP`,
	)
	if err != nil {
		return nil, err
	}
	if broker.Logger != nil {
		nrows, _ := res.RowsAffected()
		if nrows > 0 {
			broker.Logger.Infof("removed %d lingering expirees", nrows)
		}
	}

	return broker, nil
}
