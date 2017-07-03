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

var _ spectre.PasteService = &conn{}
var _ spectre.UserService = &conn{}
var _ spectre.GrantService = &conn{}
var _ spectre.ReportService = &conn{}

type conn struct {
	db     *sqlx.DB
	logger logrus.FieldLogger

	generateNewPasteID func(bool) spectre.PasteID
}

// User
func (c *conn) getUserWithQuery(ctx context.Context, query string, args ...interface{}) (spectre.User, error) {
	u := dbUser{
		conn: c,
		ctx:  ctx,
	}

	if err := c.db.GetContext(ctx, &u, `SELECT * FROM users WHERE `+query+` LIMIT 1`, args...); err != nil {
		return nil, err
	}

	return &u, nil
}

func (c *conn) GetUserNamed(ctx context.Context, name string) (spectre.User, error) {
	return c.getUserWithQuery(ctx, "name = $1", name)
}

func (c *conn) GetUserByID(ctx context.Context, id uint) (spectre.User, error) {
	return c.getUserWithQuery(ctx, "id = $1", id)
}

func (c *conn) CreateUser(ctx context.Context, name string) (spectre.User, error) {
	u := &dbUser{
		Name: name,
		conn: c,
		ctx:  ctx,
	}

	if _, err := c.db.ExecContext(ctx, "INSERT INTO users(name) VALUES($1)", name); err != nil {
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

func (c *conn) CreatePaste(ctx context.Context, cryptor spectre.Cryptor) (spectre.Paste, error) {
	var salt []byte
	var hmac []byte

	for {
		id := c.generateNewPasteID(cryptor != nil) //method != spectre.PasteEncryptionMethodNone)
		var err error
		method := spectre.EncryptionMethodNone
		if cryptor != nil {
			//TODO(DH) if passphraseMaterial == nil {
			//return nil, errors.New("model: unacceptable encryption material")
			//}
			var err error
			hmac, salt, err = cryptor.Challenge()
			if err != nil {
				return nil, err
			}

			method = cryptor.EncryptionMethod()
		}

		_, err = c.db.ExecContext(ctx,
			`INSERT INTO pastes(
				id,
				encryption_salt,
				encryption_method,
				hmac
			) VALUES($1, $2, $3, $4)`, id, salt, method, hmac)
		if err != nil {
			if isUniquenessError(err) {
				continue
			}
			return nil, err
		}

		return &dbPaste{
			conn:             c,
			ctx:              ctx,
			cryptor:          cryptor,
			ID:               string(id),
			EncryptionSalt:   salt,
			EncryptionMethod: method,
			HMAC:             hmac,
		}, nil
	}
}

func (c *conn) GetPaste(ctx context.Context, cryptor spectre.Cryptor, id spectre.PasteID) (spectre.Paste, error) {
	paste := dbPaste{
		conn: c,
		ctx:  ctx,
	}

	if err := c.db.GetContext(ctx, &paste, `SELECT * FROM view_active_pastes WHERE id = $1 LIMIT 1`, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, spectre.ErrNotFound
		}
		return nil, err
	}

	if paste.IsEncrypted() {
		// If they haven't requested decryption, we can
		// still tell them that a paste exists.
		// It will be a stub/placeholder that only has an ID.
		if cryptor == nil {
			return &encryptedPastePlaceholder{
				ID:               id,
				EncryptionMethod: paste.EncryptionMethod,
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

func (c *conn) GetPastes(ctx context.Context, ids []spectre.PasteID) ([]spectre.Paste, error) {
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

	query = c.db.Rebind(query)
	rows, err := c.db.QueryxContext(ctx, query, args...)
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
			conn: c,
			ctx:  ctx,
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

func (c *conn) DestroyPaste(ctx context.Context, id spectre.PasteID) (bool, error) {
	tx, err := c.db.BeginTxx(ctx, nil)
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

func (c *conn) CreateGrant(ctx context.Context, paste spectre.Paste) (spectre.Grant, error) {
	for {
		id, err := generateRandomBase32String(32)
		if err != nil {
			return nil, err
		}

		_, err = c.db.ExecContext(ctx,
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
			conn:    c,
			ctx:     ctx,
			ID:      id,
			PasteID: paste.GetID().String(),
		}, nil
	}
}

func (c *conn) GetGrant(ctx context.Context, id spectre.GrantID) (spectre.Grant, error) {
	g := dbGrant{
		conn: c,
		ctx:  ctx,
	}

	if err := c.db.GetContext(ctx, &g, `SELECT * FROM grants WHERE id = $1 LIMIT 1`, id); err != nil {
		if err == sql.ErrNoRows {
			err = spectre.ErrNotFound
		}
		return nil, err
	}

	return &g, nil
}

func (c *conn) ReportPaste(ctx context.Context, paste spectre.Paste) error {
	pID := paste.GetID()
	_, err := c.db.ExecContext(ctx, `
		INSERT INTO paste_reports(paste_id, count)
		VALUES($1, $2)
		ON CONFLICT(paste_id)
		DO
			UPDATE SET count = paste_reports.count + EXCLUDED.count
		`, pID, 1)
	return err
}

func (c *conn) GetReport(ctx context.Context, pID spectre.PasteID) (spectre.Report, error) {
	r := dbReport{
		conn: c,
		ctx:  ctx,
	}

	if err := c.db.GetContext(ctx, &r, `SELECT paste_id, count FROM paste_reports WHERE paste_id = ?`, pID); err != nil {
		if err == sql.ErrNoRows {
			err = spectre.ErrNotFound
		}
		return nil, err
	}

	return &r, nil
}

func (c *conn) GetReports(ctx context.Context) ([]spectre.Report, error) {
	reports := make([]spectre.Report, 0, 16)

	rows, err := c.db.QueryxContext(ctx, `SELECT paste_id, count FROM paste_reports`)
	if err != nil {
		if err == sql.ErrNoRows {
			err = spectre.ErrNotFound
		}
		return nil, err
	}

	defer rows.Close()
	for rows.Next() {
		r := &dbReport{
			conn: c,
			ctx:  ctx,
		}
		rows.Scan(&r.PasteID, &r.Count)
		reports = append(reports, r)
	}
	return reports, rows.Err()
}

func (c *conn) SetLoggerOption(log logrus.FieldLogger) {
	c.logger = log
}

func (c *conn) SetDebugOption(debug bool) {
	// no-op
}

const dbV0Schema string = `
CREATE TABLE IF NOT EXISTS _schema (
	version integer UNIQUE,
	created_at timestamp with time zone DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS uix__schema_version ON _schema USING btree (version);
`

func (c *conn) migrateDb() error {
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
			return fmt.Errorf("postgres: invalid schema migration filename %s", path)
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

	db := c.db
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
		return fmt.Errorf("postgres: database is newer than we can support! (%d > %d)", schemaVersion, maxVersion)
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

type Conn struct {
	c conn
}

func (c *Conn) PasteService() spectre.PasteService {
	return &c.c
}

func (c *Conn) UserService() spectre.UserService {
	return &c.c
}

func (c *Conn) GrantService() spectre.GrantService {
	return &c.c
}

func (c *Conn) ReportService() spectre.ReportService {
	return &c.c
}

func Open(arguments ...interface{}) (*Conn, error) {
	var wrapper Conn
	c := &wrapper.c
	c.generateNewPasteID = defaultPasteIDGenerator

	var connection *string
	for _, arg := range arguments {
		switch a := arg.(type) {
		case string:
			connection = &a
		default:
			return nil, fmt.Errorf("postgres: unknown option type %T (%v)", a, a)
		}
	}

	if connection == nil {
		return nil, errors.New("postgres: no connection string provided")
	}

	//if p.ChallengeProvider == nil {
	//return nil, errors.New("postgres: no ChallengeProvider provided")
	//}

	sqlDb, err := sqlx.Open("postgres", *connection)
	if err != nil {
		return nil, err
	}

	c.db = sqlDb

	err = c.migrateDb()
	if err != nil {
		return nil, err
	}

	res, err := c.db.Exec(
		`DELETE FROM pastes WHERE expire_at < NOW()`,
	)
	if err != nil {
		return nil, err
	}

	if c.logger != nil {
		nrows, _ := res.RowsAffected()
		if nrows > 0 {
			c.logger.Infof("removed %d lingering expirees", nrows)
		}
	}

	return &wrapper, nil
}

//func init() {
//spectre.Register("postgres", &pqDriver{})
//}
