package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"

	"howett.net/spectre"

	"github.com/Sirupsen/logrus"
	"github.com/lib/pq"

	"github.com/GeertJohan/go.rice"
	"github.com/jmoiron/sqlx"
)

var _ spectre.PasteService = &provider{}
var _ spectre.UserService = &provider{}
var _ spectre.GrantService = &provider{}
var _ spectre.ReportService = &provider{}

type provider struct {
	DB     *sqlx.DB
	Logger logrus.FieldLogger

	GenerateNewPasteID func(bool) spectre.PasteID
}

// User
func (p *provider) getUserWithQuery(ctx context.Context, query string, args ...interface{}) (spectre.User, error) {
	u := dbUser{
		provider: p,
		ctx:      ctx,
	}

	if err := p.DB.GetContext(ctx, &u, `SELECT * FROM users WHERE `+query+` LIMIT 1`, args...); err != nil {
		return nil, err
	}

	return &u, nil
}

func (p *provider) GetUserNamed(ctx context.Context, name string) (spectre.User, error) {
	return p.getUserWithQuery(ctx, "name = $1", name)
}

func (p *provider) GetUserByID(ctx context.Context, id uint) (spectre.User, error) {
	return p.getUserWithQuery(ctx, "id = $1", id)
}

func (p *provider) CreateUser(ctx context.Context, name string) (spectre.User, error) {
	u := &dbUser{
		Name:     name,
		provider: p,
		ctx:      ctx,
	}

	if _, err := p.DB.ExecContext(ctx, "INSERT INTO users(name, updated_at) VALUES($1, NOW())", name); err != nil {
		return nil, err
	}

	return u, nil
}

// Paste
func defaultPasteIDGenerator(encrypted bool) spectre.PasteID {
	idlen := 5
	if encrypted {
		idlen = 8
	}

	for {
		s, _ := generateRandomBase32String(idlen)
		return spectre.PasteID(s)
	}
}

func isUniquenessError(err error) bool {
	pqe, ok := err.(*pq.Error)
	return ok && pqe.Code == pq.ErrorCode("23505")
}

func (p *provider) CreatePaste(ctx context.Context, cryptor spectre.Cryptor) (spectre.Paste, error) {
	var salt []byte
	var hmac []byte

	for {
		id := p.GenerateNewPasteID(cryptor != nil) //method != spectre.PasteEncryptionMethodNone)
		var err error
		if cryptor != nil {
			//TODO(DH) if passphraseMaterial == nil {
			//return nil, errors.New("model: unacceptable encryption material")
			//}
			var err error
			hmac, salt, err = cryptor.Challenge()
			if err != nil {
				return nil, err
			}
		}

		_, err = p.DB.ExecContext(ctx,
			`INSERT INTO pastes(
				id,
				created_at,
				updated_at,
				encryption_salt,
				encryption_method,
				hmac
			) VALUES($1, NOW(), NOW(), $2, $3, $4)`, id, salt, 0 /*TODO(DH) method*/, hmac)
		if err != nil {
			if isUniquenessError(err) {
				continue
			}
			return nil, err
		}

		return &dbPaste{
			provider:         p,
			ctx:              ctx,
			cryptor:          cryptor,
			ID:               string(id),
			EncryptionSalt:   salt,
			EncryptionMethod: 0, //TODO(DH) 1,
			HMAC:             hmac,
		}, nil
	}
}

func (p *provider) GetPaste(ctx context.Context, cryptor spectre.Cryptor, id spectre.PasteID) (spectre.Paste, error) {
	paste := dbPaste{
		provider: p,
		ctx:      ctx,
	}

	if err := p.DB.GetContext(ctx, &paste, `SELECT * FROM view_active_pastes WHERE id = $1 LIMIT 1`, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, spectre.ErrNotFound
		}
		return nil, err
	}

	// This paste is encrypted
	if len(paste.HMAC) != 0 { //paste.IsEncrypted() {
		// If they haven't requested decryption, we can
		// still tell them that a paste exists.
		// It will be a stub/placeholder that only has an ID.
		if cryptor == nil {
			return &encryptedPastePlaceholder{
				ID: id,
			}, spectre.ErrCryptorRequired
		}

		ok, err := cryptor.Authenticate(paste.EncryptionSalt, paste.HMAC)
		if !ok || err != nil {
			return nil, spectre.ErrChallengeRejected
		}

		paste.cryptor = cryptor
	}

	return &paste, nil
}

func (p *provider) GetPastes(ctx context.Context, ids []spectre.PasteID) ([]spectre.Paste, error) {
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
	rows, err := p.DB.QueryxContext(ctx, query, args...)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, spectre.ErrNotFound
		}
		return nil, err
	}

	iPastes := make([]spectre.Paste, len(ids))
	i := 0
	for rows.Next() {
		paste := &dbPaste{
			provider: p,
			ctx:      ctx,
		}
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

func (p *provider) DestroyPaste(ctx context.Context, id spectre.PasteID) (bool, error) {
	tx, err := p.DB.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}

	_, err = tx.ExecContext(ctx, `DELETE FROM pastes WHERE id = $1`, id)
	if err != nil {
		tx.Rollback()
		return false, err
	}

	err = tx.Commit()
	if err != nil && err != sql.ErrNoRows {
		return false, err
	}

	return err != sql.ErrNoRows, nil
}

func (p *provider) CreateGrant(ctx context.Context, paste spectre.Paste) (spectre.Grant, error) {
	for {
		id, err := generateRandomBase32String(32)
		if err != nil {
			return nil, err
		}

		_, err = p.DB.ExecContext(ctx,
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
			ctx:      ctx,
			ID:       id,
			PasteID:  paste.GetID().String(),
		}, nil
	}
}

func (p *provider) GetGrant(ctx context.Context, id spectre.GrantID) (spectre.Grant, error) {
	g := dbGrant{
		provider: p,
		ctx:      ctx,
	}

	if err := p.DB.GetContext(ctx, &g, `SELECT * FROM grants WHERE id = $1 LIMIT 1`, id); err != nil {
		if err == sql.ErrNoRows {
			err = spectre.ErrNotFound
		}
		return nil, err
	}

	return &g, nil
}

func (p *provider) ReportPaste(ctx context.Context, paste spectre.Paste) error {
	pID := paste.GetID()
	_, err := p.DB.ExecContext(ctx, `
		INSERT INTO paste_reports(paste_id, count)
		VALUES($1, $2)
		ON CONFLICT(paste_id)
		DO
			UPDATE SET count = paste_reports.count + EXCLUDED.count
		`, pID, 1)
	return err
}

func (p *provider) GetReport(ctx context.Context, pID spectre.PasteID) (spectre.Report, error) {
	r := dbReport{
		provider: p,
		ctx:      ctx,
	}

	if err := p.DB.GetContext(ctx, &r, `SELECT paste_id, count FROM paste_reports WHERE paste_id = ?`, pID); err != nil {
		if err == sql.ErrNoRows {
			err = spectre.ErrNotFound
		}
		return nil, err
	}

	return &r, nil
}

func (p *provider) GetReports(ctx context.Context) ([]spectre.Report, error) {
	reports := make([]spectre.Report, 0, 16)

	rows, err := p.DB.QueryxContext(ctx, `SELECT paste_id, count FROM paste_reports`)
	if err != nil {
		if err == sql.ErrNoRows {
			err = spectre.ErrNotFound
		}
		return nil, err
	}

	defer rows.Close()
	for rows.Next() {
		r := &dbReport{
			provider: p,
			ctx:      ctx,
		}
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
			return fmt.Errorf("model/postgres: invalid schema migration filename %s", path)
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
		return fmt.Errorf("model/postgres: database is newer than we can support! (%d > %d)", schemaVersion, maxVersion)
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

func Open(arguments ...interface{}) (*provider, error) {
	p := &provider{
		GenerateNewPasteID: defaultPasteIDGenerator,
	}

	var connection *string
	for _, arg := range arguments {
		switch a := arg.(type) {
		case string:
			connection = &a
		default:
			return nil, fmt.Errorf("model/postgres: unknown option type %T (%v)", a, a)
		}
	}

	if connection == nil {
		return nil, errors.New("model/postgres: no connection string provided")
	}

	//if p.ChallengeProvider == nil {
	//return nil, errors.New("model/postgres: no ChallengeProvider provided")
	//}

	sqlDb, err := sqlx.Open("postgres", *connection)
	if err != nil {
		return nil, err
	}

	p.DB = sqlDb

	err = p.migrateDb()
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

//func init() {
//spectre.Register("postgres", &pqDriver{})
//}
