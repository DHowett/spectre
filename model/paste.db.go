package model

import (
	"bytes"
	"database/sql"
	"io"
	"io/ioutil"
	"time"

	"github.com/DHowett/ghostbin/lib/sql/querybuilder"
	"github.com/golang/glog"
	"github.com/jinzhu/gorm"
)

type dbPasteBody struct {
	PasteID string `gorm:"primary_key;type:varchar(256);unique"`
	Data    []byte
}

type dbPaste struct {
	ID        string `gorm:"type:varchar(256);unique"`
	CreatedAt time.Time
	UpdatedAt time.Time

	Title sql.NullString `gorm:"type:text"`

	LanguageName sql.NullString `gorm:"type:varchar(128);default:'text'"`
	Expiration   sql.NullString `gorm:"type:varchar(64);null"`

	HMAC             []byte `gorm:"null"`
	EncryptionSalt   []byte `gorm:"null"`
	EncryptionMethod PasteEncryptionMethod

	encryptionKey []byte `gorm:"-"`
	broker        *dbBroker
}

// gorm
func (p *dbPaste) BeforeCreate(scope *gorm.Scope) error {
	id := p.broker.GenerateNewPasteID(p.IsEncrypted())
	scope.SetColumn("ID", id)

	if p.IsEncrypted() {
		hmac := getPasteEncryptionCodec(p.EncryptionMethod).GenerateHMAC(id, p.EncryptionSalt, p.encryptionKey)
		scope.SetColumn("HMAC", hmac)
	}

	return nil
}

func (p *dbPaste) GetID() PasteID {
	return PasteID(p.ID)
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
	p.LanguageName.String = language
}
func (p *dbPaste) IsEncrypted() bool {
	return p.EncryptionMethod != PasteEncryptionMethodNone
}
func (p *dbPaste) GetExpiration() string {
	if p.Expiration.Valid {
		return p.Expiration.String
	}
	return ""
}
func (p *dbPaste) SetExpiration(expiration string) {
	p.Expiration.Valid = (expiration != "")
	p.Expiration.String = expiration
}

func (p *dbPaste) GetTitle() string {
	if p.Title.Valid {
		return p.Title.String
	}
	return ""
}
func (p *dbPaste) SetTitle(title string) {
	p.Title.Valid = (title != "")
	p.Title.String = title
}

func (p *dbPaste) Commit() error {
	return p.broker.DB.Save(p).Error
}

func (p *dbPaste) Erase() error {
	return p.broker.Delete(p).Error
}

func (p *dbPaste) Reader() (io.ReadCloser, error) {
	var b dbPasteBody
	if err := p.broker.Model(p).Related(&b, "PasteID").Error; err != nil {
		glog.Errorln(err)
		return devZero, nil
	}
	r := ioutil.NopCloser(bytes.NewReader(b.Data))
	if p.IsEncrypted() {
		return getPasteEncryptionCodec(p.EncryptionMethod).Reader(p.encryptionKey, r), nil
	}
	return r, nil
}

type pasteWriter struct {
	bytes.Buffer
	p      *dbPaste // for UpdatedAt
	b      *dbPasteBody
	broker *dbBroker
}

func newPasteWriter(broker *dbBroker, p *dbPaste) (*pasteWriter, error) {
	var b dbPasteBody
	err := broker.FirstOrInit(&b, dbPasteBody{PasteID: p.ID}).Error
	if err != nil {
		return nil, err
	}
	return &pasteWriter{
		p:      p,
		b:      &b,
		broker: broker,
	}, nil
}

func (pw *pasteWriter) Close() error {
	newData := pw.Buffer.Bytes()
	tx := pw.broker.Begin()

	scope := tx.NewScope(pw.b)
	modelStruct := scope.GetModelStruct()
	table := modelStruct.TableName(tx)

	query, err := pw.broker.qb.Build(&querybuilder.UpsertQuery{
		Table:        table,
		ConflictKeys: []string{"paste_id"},
		Fields:       []string{"paste_id", "data"},
	})

	if err != nil {
		tx.Rollback()
		return err
	}

	_, err = tx.CommonDB().Exec(query, pw.b.PasteID, newData)
	if err != nil {
		tx.Rollback()
		return err
	}

	tx.Save(pw.p)
	err = tx.Commit().Error
	return err
}

func (p *dbPaste) Writer() (io.WriteCloser, error) {
	w, err := newPasteWriter(p.broker, p)
	if err != nil {
		return nil, err
	}

	if p.IsEncrypted() {
		return getPasteEncryptionCodec(p.EncryptionMethod).Writer(p.encryptionKey, w), nil
	}
	return w, nil
}
