package postgres

import (
	"github.com/DHowett/ghostbin/model"
	"github.com/jinzhu/gorm"
)

type dbGrant struct {
	ID      string `gorm:"primary_key;type:varchar(256);unique"`
	PasteID string `gorm:"type:varchar(256);index:idx_grant_by_paste"`

	broker *dbBroker
}

// gorm
func (dbGrant) TableName() string {
	return "grants"
}

func (g *dbGrant) /* gorm */ BeforeCreate(scope *gorm.Scope) error {
	id, err := generateRandomBase32String(20, 32)
	if err != nil {
		return err
	}

	scope.SetColumn("ID", id)
	return nil
}

func (g *dbGrant) GetID() model.GrantID {
	return model.GrantID(g.ID)
}

func (g *dbGrant) GetPasteID() model.PasteID {
	return model.PasteIDFromString(g.PasteID)
}

func (g *dbGrant) Destroy() error {
	return g.broker.Delete(g).Error
}
