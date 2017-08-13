package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"howett.net/spectre"
)

func sqlStringFromPtr(s *string) sql.NullString {
	return sql.NullString{
		Valid:  *s != "",
		String: *s,
	}
}

func generatePasteChallenge(p *dbPaste, pass spectre.PassphraseMaterial) ([]byte, []byte, error) {
	// TODO(DH)
	return nil, nil, errors.New("x")
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

	HMAC           []byte `db:"hmac"`
	EncryptionSalt []byte `db:"encryption_salt"`

	// Used to differentiate encryption methods in postgres pastes
	EncryptionMethod encryptionMethod `db:"encryption_method"`

	conn *conn
	ctx  context.Context
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
	return p.EncryptionMethod != encryptionMethodNone
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

func (p *dbPaste) executeUpdate(ext sqlx.Ext, u *spectre.PasteUpdate) error {
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

	if u.PassphraseMaterial != nil {
		challenge, salt, err := generatePasteChallenge(p, u.PassphraseMaterial)
		if err != nil {
			return err
		}
		method := encryptionMethod(2) /* TODO(DH) ya */
		//method := u.Encryptor.EncryptionMethod()
		clauses = append(clauses, "encryption_salt = ?, hmac = ?, encryption_method = ?")
		args = append(args, salt, challenge, method)
		resolutions = append(resolutions, func() {
			p.HMAC = challenge
			p.EncryptionSalt = salt
			p.EncryptionMethod = method
		})
	}

	if len(clauses) == 0 && u.Body == nil {
		return nil // no updates
	}

	if len(clauses) > 0 {
		args = append(args, p.ID)
		query := fmt.Sprintf(`UPDATE pastes SET %s WHERE id = ?`, strings.Join(clauses, ", "))
		rebound := sqlx.Rebind(sqlx.DOLLAR, query)

		_, err := ext.Exec(rebound, args...)
		if err != nil {
			return err
		}
	}

	for _, f := range resolutions {
		f()
	}

	if u.Body != nil {
		body := []byte(*u.Body)
		/* TODO(DH)
		if u.Encryptor != nil {
			buf := &bytes.Buffer{}
			w, err := u.Encryptor.Writer(p, buf)
			if err != nil {
				return err
			}
			_, _ = w.Write(body) // TODO(DH) err
			body = buf.Bytes()
		}
		*/

		_, err := ext.Exec(`
		INSERT INTO paste_bodies(paste_id, data)
		VALUES($1, $2)
		ON CONFLICT(paste_id)
		DO
			UPDATE SET data = EXCLUDED.data
		`, p.ID, body)
		if err != nil {
			return err
		}
	}

	return nil
}

func (p *dbPaste) Update(u spectre.PasteUpdate) error {
	tx, err := p.conn.db.BeginTxx(p.ctx, nil)
	if err != nil {
		return err
	}

	err = p.executeUpdate(tx, &u)
	if err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
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

	//if p.decryptor != nil {
	//return p.decryptor.Reader(p, r)
	//}
	return r, nil
}

func (p *dbPaste) authenticate(pass spectre.PassphraseMaterial) (bool, error) {
	return false, nil // TODO(DH)
}
