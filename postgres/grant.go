package postgres

import (
	"context"

	"howett.net/spectre"
)

type dbGrant struct {
	ID      string `db:"id"`
	PasteID string `db:"paste_id"`

	conn *conn
	ctx  context.Context
}

func (g *dbGrant) GetID() spectre.GrantID {
	return spectre.GrantID(g.ID)
}

func (g *dbGrant) GetPasteID() spectre.PasteID {
	return spectre.PasteID(g.PasteID)
}

func (g *dbGrant) Commit() error {
	// TODO(DH)
	// can these even be committed?
	return nil
}

func (g *dbGrant) Erase() error {
	_, err := g.conn.db.ExecContext(g.ctx, `DELETE FROM grants WHERE id = $1`, g.ID)
	return err
}
