package model

import (
	"errors"

	"github.com/jinzhu/gorm"
)

type dbGrant struct {
	ID      string `gorm:"primary_key;type:varchar(256);unique"`
	PasteID string `gorm:"primary_key;type:varchar(256)"`

	broker *dbBroker
}

// gorm
func (g *dbGrant) BeforeCreate(scope *gorm.Scope) error {
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

func (g *dbGrant) Realize() {
	panic(errors.New("NOT IMPL"))
}
