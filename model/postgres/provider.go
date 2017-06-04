package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/DHowett/ghostbin/lib/crypto"
	"github.com/DHowett/ghostbin/model"
	"github.com/Sirupsen/logrus"
	"github.com/lib/pq"

	"github.com/GeertJohan/go.rice"
	"github.com/jmoiron/sqlx"
)

type provider struct {
	DB                *sqlx.DB
	Logger            logrus.FieldLogger
	ChallengeProvider crypto.ChallengeProvider

	GenerateNewPasteID func(bool) model.PasteID
}

// User
func (p *provider) getUserWithQuery(query string, args ...interface{}) (model.User, error) {
	var u dbUser
	if err := p.DB.GetContext(context.TODO(), &u, `SELECT * FROM users WHERE `+query+` LIMIT 1`, args...); err != nil {
		return nil, err
	}

	u.provider = p
	return &u, nil
}

func (p *provider) GetUserNamed(name string) (model.User, error) {
	u, err := p.getUserWithQuery("name = $1", name)
	return u, err
}

func (p *provider) GetUserByID(id uint) (model.User, error) {
	return p.getUserWithQuery("id = $1", id)
}

func (p *provider) CreateUser(name string) (model.User, error) {
	u := &dbUser{
		Name:     name,
		provider: p,
	}

	if _, err := p.DB.ExecContext(context.TODO(), "INSERT INTO users(name, updated_at) VALUES($1, NOW())", name); err != nil {
		return nil, err
	}

	return u, nil
}

// Paste
func defaultPasteIDGenerator(encrypted bool) model.PasteID {
	idlen := 5
	if encrypted {
		idlen = 8
	}

	for {
		s, _ := generateRandomBase32String(idlen)
		return model.PasteIDFromString(s)
	}
}

func isUniquenessError(err error) bool {
	pqe, ok := err.(*pq.Error)
	return ok && pqe.Code == pq.ErrorCode("23505")
}

func (p *provider) createPaste(method model.PasteEncryptionMethod, passphraseMaterial []byte) (model.Paste, error) {
	var salt []byte
	var key []byte
	var hmac []byte

	for {
		id := p.GenerateNewPasteID(method != model.PasteEncryptionMethodNone)
		var err error
		if method != model.PasteEncryptionMethodNone {
			if passphraseMaterial == nil {
				return nil, errors.New("model: unacceptable encryption material")
			}
			salt, _ = generateRandomBytes(16)
			codec := model.GetPasteEncryptionCodec(method)
			key, err = codec.DeriveKey(passphraseMaterial, salt)
			if err != nil {
				return nil, err
			}
			hmac = codec.GenerateHMAC(id, salt, key)
		}

		_, err = p.DB.ExecContext(context.TODO(),
			`INSERT INTO pastes(
				id,
				created_at,
				updated_at,
				encryption_salt,
				encryption_method,
				hmac
			) VALUES($1, NOW(), NOW(), $2, $3, $4)`, id, salt, method, hmac)
		if err != nil {
			if isUniquenessError(err) {
				continue
			}
			return nil, err
		}

		return &dbPaste{
			provider:         p,
			ID:               string(id),
			EncryptionSalt:   salt,
			EncryptionMethod: method,
			HMAC:             hmac,
			encryptionKey:    key,
		}, nil
	}
}

func (p *provider) CreatePaste() (model.Paste, error) {
	return p.createPaste(model.PasteEncryptionMethodNone, nil)
}

func (p *provider) CreateEncryptedPaste(method model.PasteEncryptionMethod, passphraseMaterial []byte) (model.Paste, error) {
	return p.createPaste(method, passphraseMaterial)
}

func (p *provider) GetPaste(id model.PasteID, passphraseMaterial []byte) (model.Paste, error) {
	var paste dbPaste
	if err := p.DB.GetContext(context.TODO(), &paste, `SELECT * FROM view_active_pastes WHERE id = $1 LIMIT 1`, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, model.ErrNotFound
		}
		return nil, err
	}

	paste.provider = p

	// This paste is encrypted
	if paste.IsEncrypted() {
		// If they haven't requested decryption, we can
		// still tell them that a paste exists.
		// It will be a stub/placeholder that only has an ID.
		if passphraseMaterial == nil {
			return &encryptedPastePlaceholder{
				ID: id,
			}, model.ErrPasteEncrypted
		}

		key, err := model.GetPasteEncryptionCodec(paste.EncryptionMethod).DeriveKey(passphraseMaterial, paste.EncryptionSalt)
		if err != nil {
			return nil, model.ErrPasteEncrypted
		}

		ok := model.GetPasteEncryptionCodec(paste.EncryptionMethod).Authenticate(id, paste.EncryptionSalt, key, paste.HMAC)
		if !ok {
			return nil, model.ErrInvalidKey
		}

		paste.encryptionKey = key
	}

	return &paste, nil
}

func (p *provider) GetPastes(ids []model.PasteID) ([]model.Paste, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	stringIDs := make([]string, len(ids))
	for i, v := range ids {
		stringIDs[i] = string(v)
	}

	query, args, err := sqlx.In(`SELECT * FROM view_active_pastes WHERE id IN (?)` /* .In() requires ? */, ids)
	if err != nil {
		return nil, err
	}

	query = p.DB.Rebind(query)
	rows, err := p.DB.QueryxContext(context.TODO(), query, args...)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, model.ErrNotFound
		}
		return nil, err
	}

	iPastes := make([]model.Paste, len(ids))
	i := 0
	for rows.Next() {
		paste := &dbPaste{provider: p}
		rows.StructScan(&paste)
		if paste.IsEncrypted() {
			iPastes[i] = &encryptedPastePlaceholder{
				ID: paste.GetID(),
			}
		} else {
			iPastes[i] = paste
		}
		i++
	}

	return iPastes[:i], nil
}

func (p *provider) DestroyPaste(id model.PasteID) error {
	tx, err := p.DB.BeginTxx(context.TODO(), nil)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(context.TODO(), `DELETE FROM pastes WHERE id = $1`, id)
	if err != nil {
		tx.Rollback()
		return err
	}

	err = tx.Commit()
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	return nil
}

func (p *provider) CreateGrant(paste model.Paste) (model.Grant, error) {
	for {
		id, err := generateRandomBase32String(32)
		if err != nil {
			return nil, err
		}

		_, err = p.DB.ExecContext(context.TODO(),
			`INSERT INTO grants(
				id,
				paste_id
			) VALUES($1, $2)`, id, paste.GetID())
		if err != nil {
			if isUniquenessError(err) {
				continue
			}
			return nil, err
		}

		return &dbGrant{
			provider: p,
			ID:       id,
			PasteID:  paste.GetID().String(),
		}, nil
	}
}

func (p *provider) GetGrant(id model.GrantID) (model.Grant, error) {
	var g dbGrant
	if err := p.DB.GetContext(context.TODO(), &g, `SELECT * FROM grants WHERE id = $1 LIMIT 1`, id); err != nil {
		if err == sql.ErrNoRows {
			err = model.ErrNotFound
		}
		return nil, err
	}

	g.provider = p
	return &g, nil
}

func (p *provider) ReportPaste(paste model.Paste) error {
	pID := paste.GetID()
	_, err := p.DB.ExecContext(context.TODO(), `
		INSERT INTO paste_reports(paste_id, count)
		VALUES($1, $2)
		ON CONFLICT(paste_id)
		DO
			UPDATE SET count = paste_reports.count + EXCLUDED.count
		`, pID, 1)
	return err
}

func (p *provider) GetReport(pID model.PasteID) (model.Report, error) {
	var r dbReport
	if err := p.DB.GetContext(context.TODO(), &r, `SELECT paste_id, count FROM paste_reports WHERE paste_id = ?`, pID); err != nil {
		if err == sql.ErrNoRows {
			err = model.ErrNotFound
		}
		return nil, err
	}

	r.provider = p
	return &r, nil
}

func (p *provider) GetReports() ([]model.Report, error) {
	reports := make([]model.Report, 0, 16)

	rows, err := p.DB.QueryxContext(context.TODO(), `SELECT paste_id, count FROM paste_reports`)
	if err != nil {
		if err == sql.ErrNoRows {
			err = model.ErrNotFound
		}
		return nil, err
	}

	defer rows.Close()
	for rows.Next() {
		r := &dbReport{provider: p}
		rows.Scan(&r.PasteID, &r.Count)
		reports = append(reports, r)
	}
	return reports, rows.Err()
}

func (p *provider) SetLoggerOption(log logrus.FieldLogger) {
	p.Logger = log
}

func (p *provider) SetDebugOption(debug bool) {
	// no-op
}

const dbV0Schema string = `
CREATE TABLE IF NOT EXISTS _schema (
	version integer UNIQUE,
	created_at timestamp with time zone DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS uix__schema_version ON _schema USING btree (version);
`

func (p *provider) migrateDb() error {
	schemaBox, err := rice.FindBox("schema")
	if err != nil {
		return err
	}

	maxVersion := -1
	schemas := make(map[int]string)
	_ = schemas
	_ = maxVersion
	err = schemaBox.Walk("" /* empty path; box is rooted at schema/ */, func(path string, fi os.FileInfo, err error) error {
		if fi.IsDir() || !strings.HasSuffix(path, ".sql") {
			return nil
		}

		var ver int
		var desc string
		n, _ := fmt.Sscanf(path, "%d_%s", &ver, &desc)
		if n != 2 {
			return fmt.Errorf("model.postgres: invalid schema migration filename %s", path)
		}
		schemas[ver] = path
		if ver > maxVersion {
			maxVersion = ver
		}
		return nil
	})
	if err != nil {
		return err
	}

	db := p.DB
	_, err = db.Exec(dbV0Schema)
	if err != nil {
		return err
	}

	schemaVersion := 0
	err = db.QueryRow("SELECT version FROM _schema ORDER BY version DESC LIMIT 1").Scan(&schemaVersion)
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	if schemaVersion > maxVersion {
		return fmt.Errorf("model.postgres: database is newer than we can support! (%d > %d)", schemaVersion, maxVersion)
	}

	for ; schemaVersion < maxVersion; schemaVersion++ {
		tx, err := db.Begin()
		if err != nil {
			// Failed to migrate!
			return err
		}

		// we use Must, as the Walk earlier proved that these files exist.
		sch := schemaBox.MustString(schemas[schemaVersion+1])
		_, err = tx.Exec(sch)
		if err != nil {
			tx.Rollback()
			// Failed to migrate!
			return err
		}

		newVersion := schemaVersion + 1
		tx.Exec("INSERT INTO _schema(version) VALUES($1)", newVersion)

		if err := tx.Commit(); err != nil {
			tx.Rollback()
			// Failed to migrate!
			return err
		}
	}

	return nil
}

type pqDriver struct{}

func (pqDriver) Open(arguments ...interface{}) (model.Provider, error) {
	p := &provider{
		GenerateNewPasteID: defaultPasteIDGenerator,
	}

	var sqlDb *sql.DB
	for _, arg := range arguments {
		switch a := arg.(type) {
		case *sql.DB:
			sqlDb = a
		case crypto.ChallengeProvider:
			p.ChallengeProvider = a
		case model.Option:
			a(p)
		default:
			return nil, fmt.Errorf("model.postgres: unknown option type %T (%v)", a, a)
		}
	}

	if sqlDb == nil {
		return nil, errors.New("model.postgres: no *sql.DB provided")
	}

	if p.ChallengeProvider == nil {
		return nil, errors.New("model.postgres: no ChallengeProvider provided")
	}

	p.DB = sqlx.NewDb(sqlDb, "postgres")

	err := p.migrateDb()
	if err != nil {
		return nil, err
	}

	res, err := p.DB.Exec(
		`DELETE FROM pastes WHERE expire_at < NOW()`,
	)
	if err != nil {
		return nil, err
	}

	if p.Logger != nil {
		nrows, _ := res.RowsAffected()
		if nrows > 0 {
			p.Logger.Infof("removed %d lingering expirees", nrows)
		}
	}

	return p, nil
}

func init() {
	model.Register("postgres", &pqDriver{})
}
