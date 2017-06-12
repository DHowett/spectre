package postgres

import (
	"context"
	"testing"

	"howett.net/spectre"
)

func TestGrant(t *testing.T) {
	p, err := pqPasteService.CreatePaste(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
		return
	}
	pID := p.GetID()

	var g spectre.Grant
	t.Run("Create", func(t *testing.T) {
		g, err = pqGrantService.CreateGrant(context.Background(), p)
		if err != nil {
			t.Fatal(err)
		}

		if g.GetID() == "" {
			t.Fatal("Grant didn't have an ID")
		}

		if g.GetPasteID() != pID {
			t.Fatal("Grant Paste ID didn't match real paste ID (`%v` != `%v`)", g.GetPasteID(), pID)
		}
	})

	var g2 spectre.Grant
	t.Run("Lookup", func(t *testing.T) {
		g2, err = pqGrantService.GetGrant(context.Background(), g.GetID())
		if err != nil {
			t.Fatal(err)
		}

		if g2.GetID() != g.GetID() {
			t.Fatal("Grant ID didn't match after lookup (`%v` != `%v`)", g2.GetID(), g.GetID())
		}

		if g2.GetPasteID() != g.GetPasteID() {
			t.Fatal("Grant Paste ID didn't match after lookup (`%v` != `%v`)", g2.GetPasteID(), g.GetPasteID())
		}
	})

	t.Run("Erase", func(t *testing.T) {
		err := g.Erase()
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("LookupAfterErase", func(t *testing.T) {
		_, err := pqGrantService.GetGrant(context.Background(), g2.GetID())
		if err != spectre.ErrNotFound {
			t.Fatal("Expected ErrNotFound; got `%v`", err)
		}
	})

	t.Run("EraseAgain", func(t *testing.T) {
		err := g2.Erase()
		if err != nil {
			t.Fatal(err)
		}
	})
}
