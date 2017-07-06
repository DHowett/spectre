package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"io/ioutil"
	"strings"
	"time"

	"howett.net/spectre"

	"github.com/jmoiron/sqlx"
)

func sqlStringFromPtr(s *string) sql.NullString {
	return sql.NullString{
		Valid:  *s != "",
		String: *s,
	}
}

type pasteBody struct {
	PasteID string `db:"paste_id"`
	Data    []byte `db:"data"`
}

type dbPaste struct {
	ID        string
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`

	ExpireAt *time.Time `db:"expire_at"`

	Title sql.NullString

	LanguageName sql.NullString `db:"language_name"`

	HMAC             []byte                   `db:"hmac"`
	EncryptionSalt   []byte                   `db:"encryption_salt"`
	EncryptionMethod spectre.EncryptionMethod `db:"encryption_method"`

	conn    *conn
	cryptor spectre.Cryptor
	ctx     context.Context
}

func (p *dbPaste) GetID() spectre.PasteID {
	return spectre.PasteID(p.ID)
}
func (p *dbPaste) GetModificationTime() time.Time {
	return p.UpdatedAt
}
func (p *dbPaste) GetLanguageName() string {
	if p.LanguageName.Valid {
		return p.LanguageName.String
	}
	return ""
}
func (p *dbPaste) IsEncrypted() bool {
	return p.EncryptionMethod != spectre.EncryptionMethodNone
}
func (p *dbPaste) GetEncryptionMethod() spectre.EncryptionMethod {
	return p.EncryptionMethod
}
func (p *dbPaste) GetExpirationTime() *time.Time {
	return p.ExpireAt
}
func (p *dbPaste) GetTitle() string {
	if p.Title.Valid {
		return p.Title.String
	}
	return ""
}
func (p *dbPaste) Update(u spectre.PasteUpdate) error {
	clauses := make([]string, 0, 4)
	args := make([]interface{}, 0, 4)
	// resolutions is a list of functions to execute when
	// the transaction completes; this ensures that we only change
	// the actual in-memory representation of p if the transaction
	// succeeds.
	resolutions := make([]func(), 0, 4)

	if u.LanguageName != nil {
		l := sqlStringFromPtr(u.LanguageName)
		clauses = append(clauses, `language_name = ?`)
		args = append(args, l)
		resolutions = append(resolutions, func() {
			p.LanguageName = l
		})
	}

	if u.ExpirationTime != nil {
		v := u.ExpirationTime
		if *v == *spectre.ExpirationTimeNever {
			v = nil
		}

		clauses = append(clauses, `expire_at = ?`)
		args = append(args, v)
		resolutions = append(resolutions, func() {
			p.ExpireAt = v
		})
	}

	if u.Title != nil {
		t := sqlStringFromPtr(u.Title)
		clauses = append(clauses, `title = ?`)
		args = append(args, t)
		resolutions = append(resolutions, func() {
			p.Title = t
		})
	}

	if len(clauses) == 0 && u.Body == nil {
		return nil // no updates
	}

	tx, err := p.conn.db.BeginTxx(p.ctx, nil)
	if err != nil {
		return err
	}

	if len(clauses) > 0 {
		args = append(args, p.ID)
		query := fmt.Sprintf(`UPDATE pastes SET %s WHERE id = ?`, strings.Join(clauses, ", "))
		rebound := tx.Rebind(query)

		_, err := tx.ExecContext(p.ctx, rebound, args...)
		if err != nil {
			tx.Rollback()
			return err
		}
	}

	for _, f := range resolutions {
		f()
	}

	if u.Body != nil {
		w, err := p.writer(tx)
		if err != nil {
			tx.Rollback()
			return err
		}
		io.WriteString(w, *u.Body)
		w.Close() // queries against tx
	}

	return tx.Commit()
}

func (p *dbPaste) Erase() error {
	_, err := p.conn.DestroyPaste(p.ctx, spectre.PasteID(p.ID))
	return err
}

func (p *dbPaste) Reader() (io.ReadCloser, error) {
	var b pasteBody
	if err := p.conn.db.GetContext(p.ctx, &b, `SELECT * FROM paste_bodies WHERE paste_id = $1 LIMIT 1`, p.ID); err != nil {
		if err == sql.ErrNoRows {
			return devZero, nil
		}
		return nil, err
	}
	r := ioutil.NopCloser(bytes.NewReader(b.Data))

	if p.cryptor != nil {
		return p.cryptor.Reader(r)
	}
	return r, nil
}

type pasteWriter struct {
	bytes.Buffer
	tx *sqlx.Tx
	id string
}

func (pw *pasteWriter) Close() error {
	newData := pw.Buffer.Bytes()

	_, err := pw.tx.Exec(`
	INSERT INTO paste_bodies(paste_id, data)
	VALUES($1, $2)
	ON CONFLICT(paste_id)
	DO
		UPDATE SET data = EXCLUDED.data
	`, pw.id, newData)

	return err
}

func (p *dbPaste) writer(tx *sqlx.Tx) (io.WriteCloser, error) {
	w := &pasteWriter{
		tx: tx,
		id: p.ID,
	}

	if p.cryptor != nil {
		return p.cryptor.Writer(w)
	}

	return w, nil
}
