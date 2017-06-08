package postgres

import (
	"context"

	"github.com/DHowett/ghostbin/model"
)

type dbGrant struct {
	ID      string `db:"id"`
	PasteID string `db:"paste_id"`

	provider *provider
	ctx      context.Context
}

func (g *dbGrant) GetID() model.GrantID {
	return model.GrantID(g.ID)
}

func (g *dbGrant) GetPasteID() model.PasteID {
	return model.PasteIDFromString(g.PasteID)
}

func (g *dbGrant) Destroy() error {
	_, err := g.provider.DB.ExecContext(g.ctx, `DELETE FROM grants WHERE id = $1`, g.ID)
	return err
}
