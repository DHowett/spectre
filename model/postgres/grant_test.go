package postgres

import (
	"testing"

	"github.com/DHowett/ghostbin/model"
)

func TestGrant(t *testing.T) {
	p, err := gTestProvider.CreatePaste()
	if err != nil {
		t.Fatal(err)
		return
	}
	pID := p.GetID()

	var g model.Grant
	t.Run("Create", func(t *testing.T) {
		g, err = gTestProvider.CreateGrant(p)
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

	var g2 model.Grant
	t.Run("Lookup", func(t *testing.T) {
		g2, err = gTestProvider.GetGrant(g.GetID())
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

	t.Run("Destroy", func(t *testing.T) {
		err := g.Destroy()
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("LookupAfterDestroy", func(t *testing.T) {
		_, err := gTestProvider.GetGrant(g2.GetID())
		if err != model.ErrNotFound {
			t.Fatal("Expected ErrNotFound; got `%v`", err)
		}
	})

	t.Run("DestroyAgain", func(t *testing.T) {
		err := g2.Destroy()
		if err != nil {
			t.Fatal(err)
		}
	})
}
