package postgres

import (
	"bytes"
	"database/sql"
	"io"
	"io/ioutil"
	"time"

	"github.com/DHowett/ghostbin/model"
	log "github.com/Sirupsen/logrus"
	"github.com/jinzhu/gorm"
)

type pasteBody struct {
	PasteID string `gorm:"primary_key;type:varchar(256);unique"`
	Data    []byte
}

// gorm
func (pasteBody) TableName() string {
	return "paste_bodies"
}

type dbPaste struct {
	ID        string `gorm:"type:varchar(256);unique"`
	CreatedAt time.Time
	UpdatedAt time.Time

	ExpireAt *time.Time `gorm:"null"`

	Title sql.NullString `gorm:"type:text"`

	LanguageName sql.NullString `gorm:"type:varchar(128);default:'text'"`

	HMAC             []byte `gorm:"null"`
	EncryptionSalt   []byte `gorm:"null"`
	EncryptionMethod model.PasteEncryptionMethod

	encryptionKey []byte `gorm:"-"`
	provider      *provider
}

// gorm
func (dbPaste) TableName() string {
	return "pastes"
}

// gorm
func (p *dbPaste) BeforeCreate(scope *gorm.Scope) error {
	id := p.provider.GenerateNewPasteID(p.IsEncrypted())
	scope.SetColumn("ID", id)

	if p.IsEncrypted() {
		hmac := model.GetPasteEncryptionCodec(p.EncryptionMethod).GenerateHMAC(id, p.EncryptionSalt, p.encryptionKey)
		scope.SetColumn("HMAC", hmac)
	}

	return nil
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
	p.LanguageName.String = language
}
func (p *dbPaste) IsEncrypted() bool {
	return p.EncryptionMethod != model.PasteEncryptionMethodNone
}
func (p *dbPaste) GetExpirationTime() *time.Time {
	return p.ExpireAt
}
func (p *dbPaste) SetExpirationTime(time time.Time) {
	p.ExpireAt = &time
}
func (p *dbPaste) ClearExpirationTime() {
	p.ExpireAt = nil
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
	return p.provider.DB.Save(p).Error
}

func (p *dbPaste) Erase() error {
	return p.provider.DestroyPaste(model.PasteID(p.ID))
}

func (p *dbPaste) Reader() (io.ReadCloser, error) {
	var b pasteBody
	if err := p.provider.Model(p).Related(&b, "PasteID").Error; err != nil {
		log.Errorln(err)
		return devZero, nil
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

func newPasteWriter(pr *provider, p *dbPaste) (*pasteWriter, error) {
	var b pasteBody
	err := pr.FirstOrInit(&b, pasteBody{PasteID: p.ID}).Error
	if err != nil {
		return nil, err
	}
	return &pasteWriter{
		p:        p,
		b:        &b,
		provider: pr,
	}, nil
}

func (pw *pasteWriter) Close() error {
	newData := pw.Buffer.Bytes()
	tx := pw.provider.Begin()

	_, err := tx.CommonDB().Exec(`
	INSERT INTO paste_bodies(paste_id, data)
	VALUES($1, $2)
	ON CONFLICT(paste_id)
	DO
		UPDATE SET data = EXCLUDED.data
	`, pw.b.PasteID, newData)

	if err != nil {
		tx.Rollback()
		return err
	}

	tx.Save(pw.p)
	err = tx.Commit().Error
	return err
}

func (p *dbPaste) Writer() (io.WriteCloser, error) {
	w, err := newPasteWriter(p.provider, p)
	if err != nil {
		return nil, err
	}

	if p.IsEncrypted() {
		return model.GetPasteEncryptionCodec(p.EncryptionMethod).Writer(p.encryptionKey, w), nil
	}
	return w, nil
}
