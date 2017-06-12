package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"io"
	"io/ioutil"
	"time"

	"github.com/DHowett/ghostbin/model"
	"github.com/jmoiron/sqlx"
)

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

	HMAC             []byte                      `db:"hmac"`
	EncryptionSalt   []byte                      `db:"encryption_salt"`
	EncryptionMethod model.PasteEncryptionMethod `db:"encryption_method"`

	encryptionKey []byte

	provider *provider
	ctx      context.Context
	tx       *sqlx.Tx
}

func (p *dbPaste) openTx() error {
	if p.tx == nil {
		var err error
		p.tx, err = p.provider.DB.BeginTxx(p.ctx, nil)
		return err
	}
	return nil
}

func (p *dbPaste) commitTx() error {
	tx := p.tx
	p.tx = nil
	return tx.Commit()
}

func (p *dbPaste) GetID() model.PasteID {
	return model.PasteID(p.ID)
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
func (p *dbPaste) SetLanguageName(language string) {
	p.openTx()
	p.LanguageName.Valid = language != ""
	p.LanguageName.String = language
	p.tx.ExecContext(p.ctx, `UPDATE pastes SET language_name = $1 WHERE id = $2`, p.LanguageName, p.ID)
}
func (p *dbPaste) IsEncrypted() bool {
	return p.EncryptionMethod != model.PasteEncryptionMethodNone
}
func (p *dbPaste) GetExpirationTime() *time.Time {
	return p.ExpireAt
}
func (p *dbPaste) setExpirationTime(time *time.Time) {
	p.openTx()
	p.ExpireAt = time
	p.tx.ExecContext(p.ctx, `UPDATE pastes SET expire_at = $1 WHERE id = $2`, p.ExpireAt, p.ID)
}
func (p *dbPaste) SetExpirationTime(time time.Time) {
	p.setExpirationTime(&time)
}
func (p *dbPaste) ClearExpirationTime() {
	p.setExpirationTime(nil)
}

func (p *dbPaste) GetTitle() string {
	if p.Title.Valid {
		return p.Title.String
	}
	return ""
}
func (p *dbPaste) SetTitle(title string) {
	p.openTx()
	p.Title.Valid = (title != "")
	p.Title.String = title
	p.tx.ExecContext(p.ctx, `UPDATE pastes SET title = $1 WHERE id = $2`, p.Title, p.ID)
}

func (p *dbPaste) Commit() error {
	p.tx.ExecContext(p.ctx, `UPDATE pastes SET updated_at = NOW() WHERE id = $1`, p.ID)

	return p.commitTx()
}

func (p *dbPaste) Erase() error {
	if p.tx != nil {
		err := p.tx.Rollback()
		if err != nil {
			return err
		}
	}
	return p.provider.DestroyPaste(p.ctx, model.PasteID(p.ID))
}

func (p *dbPaste) Reader() (io.ReadCloser, error) {
	var b pasteBody
	if err := p.provider.DB.GetContext(p.ctx, &b, `SELECT * FROM paste_bodies WHERE paste_id = $1 LIMIT 1`, p.ID); err != nil {
		if err == sql.ErrNoRows {
			return devZero, nil
		}
		return nil, err
	}
	r := ioutil.NopCloser(bytes.NewReader(b.Data))
	if p.IsEncrypted() {
		return model.GetPasteEncryptionCodec(p.EncryptionMethod).Reader(p.encryptionKey, r), nil
	}
	return r, nil
}

type pasteWriter struct {
	bytes.Buffer
	p        *dbPaste // for UpdatedAt
	b        *pasteBody
	provider *provider
}

func newPasteWriter(p *dbPaste) (*pasteWriter, error) {
	return &pasteWriter{
		p: p,
	}, nil
}

func (pw *pasteWriter) Close() error {
	newData := pw.Buffer.Bytes()

	// TODO(DH) error
	pw.p.openTx()
	pw.p.tx.ExecContext(pw.p.ctx, `
	INSERT INTO paste_bodies(paste_id, data)
	VALUES($1, $2)
	ON CONFLICT(paste_id)
	DO
		UPDATE SET data = EXCLUDED.data
	`, pw.p.ID, newData)

	return pw.p.Commit()
}

func (p *dbPaste) Writer() (io.WriteCloser, error) {
	w, err := newPasteWriter(p)
	if err != nil {
		return nil, err
	}

	if p.IsEncrypted() {
		return model.GetPasteEncryptionCodec(p.EncryptionMethod).Writer(p.encryptionKey, w), nil
	}
	return w, nil
}
