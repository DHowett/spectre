package model

import "github.com/jinzhu/gorm"

type dbGrant struct {
	ID      string `gorm:"primary_key;type:varchar(256);unique"`
	PasteID string `gorm:"type:varchar(256);index:idx_grant_by_paste"`

	broker *dbBroker
}

func (g *dbGrant) /* gorm */ BeforeCreate(scope *gorm.Scope) error {
	id, err := generateRandomBase32String(20, 32)
	if err != nil {
		return err
	}

	scope.SetColumn("ID", id)
	return nil
}

func (g *dbGrant) GetID() GrantID {
	return GrantID(g.ID)
}

func (g *dbGrant) GetPasteID() PasteID {
	return PasteIDFromString(g.PasteID)
}

func (g *dbGrant) Destroy() error {
	return g.broker.Delete(g).Error
}
