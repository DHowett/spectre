package postgres

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"math"
	"testing"
	"time"

	"howett.net/spectre"
)

func TestPaste(t *testing.T) {
	// used in each subtest to look up the paste anew.
	var pID spectre.PasteID

	t.Run("Create", func(t *testing.T) {
		p, err := pqPasteService.CreatePaste(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
			return
		}
		pID = p.GetID()
	})

	t.Run("GetValues", func(t *testing.T) {
		_, err := pqPasteService.GetPaste(context.Background(), nil, pID)
		if err != nil {
			t.Fatal("failed to get", pID, ":", err)
		}
	})

	t.Run("ReadBody1", func(t *testing.T) {
		p, err := pqPasteService.GetPaste(context.Background(), nil, pID)
		if err != nil {
			t.Fatal("failed to get", pID, ":", err)
		}
		reader, err := p.Reader()
		if err != nil {
			t.Fatal(err)
		}
		buf, err := ioutil.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		if len(buf) != 0 {
			t.Fatalf("newly-created paste had non-empty buffer: <%s>", string(buf))
		}
	})

	t.Run("CreateBody", func(t *testing.T) {
		p, err := pqPasteService.GetPaste(context.Background(), nil, pID)
		if err != nil {
			t.Fatal("failed to get", pID, ":", err)
		}
		body := "hello"
		err = p.Update(spectre.PasteUpdate{
			Body: &body,
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("ReadBody2", func(t *testing.T) {
		p, err := pqPasteService.GetPaste(context.Background(), nil, pID)
		if err != nil {
			t.Fatal("failed to get", pID, ":", err)
		}
		reader, err := p.Reader()
		if err != nil {
			t.Fatal(err)
		}
		buf, err := ioutil.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(buf, []byte("hello")) {
			t.Fatalf("paste had incomprehensible body <%s>", string(buf))
		}
	})

	t.Run("UpdateBody", func(t *testing.T) {
		p, err := pqPasteService.GetPaste(context.Background(), nil, pID)
		if err != nil {
			t.Fatal("failed to get", pID, ":", err)
		}
		body := "goodbye"
		err = p.Update(spectre.PasteUpdate{
			Body: &body,
		})
	})

	t.Run("ReadBody3", func(t *testing.T) {
		p, err := pqPasteService.GetPaste(context.Background(), nil, pID)
		if err != nil {
			t.Fatal("failed to get", pID, ":", err)
		}
		reader, err := p.Reader()
		if err != nil {
			t.Fatal(err)
		}
		buf, err := ioutil.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(buf, []byte("goodbye")) {
			t.Fatalf("paste had incomprehensible body <%s>", string(buf))
		}
	})

	t.Run("Destroy", func(t *testing.T) {
		ok, err := pqPasteService.DestroyPaste(context.Background(), pID)
		if !ok {
			t.Fatal("paste wasn't found?")
		}
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("LookupAfterDestroy", func(t *testing.T) {
		p, err := pqPasteService.GetPaste(context.Background(), nil, pID)
		if p != nil || err == nil {
			t.Fatal("got a paste back/no error?")
		}
	})
}

func expectEq(t *testing.T, field string, l, r interface{}) {
	if l == r || (l == nil && r == nil) {
		return
	}
	t.Errorf("%s mismatch (`%v` != `%v`)", field, l, r)
}

func expectTimeEq(t *testing.T, field string, l, r time.Time) {
	if math.Abs(float64(l.Local().Sub(r.Local()))) < float64(1*time.Millisecond) {
		return
	}
	t.Errorf("%s mismatch (`%v` != `%v`)", field, l, r)
}

func TestPasteMutation(t *testing.T) {
	// used in each subtest to look up the paste anew.
	var pID spectre.PasteID

	expTime := time.Now().Add(1 * time.Hour)

	t.Run("CreateConverged", func(t *testing.T) {
		title := "Hello World"
		lang := "c"
		p, err := pqPasteService.CreatePaste(context.Background(), &spectre.PasteUpdate{
			Title:          &title,
			LanguageName:   &lang,
			ExpirationTime: &expTime,
		})
		if err != nil {
			t.Fatal(err)
			return
		}
		pID = p.GetID()
	})

	t.Run("GetValues1", func(t *testing.T) {
		p, err := pqPasteService.GetPaste(context.Background(), nil, pID)
		if err != nil {
			t.Fatal("failed to get", pID, ":", err)
		}

		expectEq(t, "title", p.GetTitle(), "Hello World")
		expectEq(t, "language", p.GetLanguageName(), "c")
		expectTimeEq(t, "expiration", *p.GetExpirationTime(), expTime)
	})

	t.Run("ClearValues", func(t *testing.T) {
		p, err := pqPasteService.GetPaste(context.Background(), nil, pID)
		if err != nil {
			t.Fatal("failed to get", pID, ":", err)
		}

		title := ""
		err = p.Update(spectre.PasteUpdate{
			Title:          &title,
			ExpirationTime: spectre.ExpirationTimeNever,
		})

		if err != nil {
			t.Fatal("failed to save", pID, err)
		}
	})

	t.Run("GetValues2", func(t *testing.T) {
		p, err := pqPasteService.GetPaste(context.Background(), nil, pID)
		if err != nil {
			t.Fatal("failed to get", pID, ":", err)
		}

		expectEq(t, "title", p.GetTitle(), "")
		var nilTime *time.Time
		expectEq(t, "expiration", p.GetExpirationTime(), nilTime)
	})
}

func TestPasteMutationViaWriterCommit(t *testing.T) {
	// used in each subtest to look up the paste anew.
	var pID spectre.PasteID

	expTime := time.Now().Add(1 * time.Hour)

	t.Run("CreateConverged", func(t *testing.T) {
		title := "Hello World"
		lang := "c"
		body := "-"
		p, err := pqPasteService.CreatePaste(context.Background(), &spectre.PasteUpdate{
			Title:          &title,
			LanguageName:   &lang,
			ExpirationTime: &expTime,
			Body:           &body,
		})
		if err != nil {
			t.Fatal(err)
			return
		}
		pID = p.GetID()
	})

	t.Run("GetValues1", func(t *testing.T) {
		p, err := pqPasteService.GetPaste(context.Background(), nil, pID)
		if err != nil {
			t.Fatal("failed to get", pID, ":", err)
		}

		expectEq(t, "title", p.GetTitle(), "Hello World")
		expectEq(t, "language", p.GetLanguageName(), "c")
		expectTimeEq(t, "expiration", *p.GetExpirationTime(), expTime)
	})

	t.Run("ClearValues", func(t *testing.T) {
		p, err := pqPasteService.GetPaste(context.Background(), nil, pID)
		if err != nil {
			t.Fatal("failed to get", pID, ":", err)
		}

		title := ""
		err = p.Update(spectre.PasteUpdate{
			Title:          &title,
			ExpirationTime: spectre.ExpirationTimeNever,
		})

		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("GetValues2", func(t *testing.T) {
		p, err := pqPasteService.GetPaste(context.Background(), nil, pID)
		if err != nil {
			t.Fatal("failed to get", pID, ":", err)
		}

		expectEq(t, "title", p.GetTitle(), "")
		var nilTime *time.Time
		expectEq(t, "expiration", p.GetExpirationTime(), nilTime)
	})
}

func TestPasteReadAfterDestroy(t *testing.T) {
	p, err := pqPasteService.CreatePaste(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
		return
	}

	ok, err := pqPasteService.DestroyPaste(context.Background(), p.GetID())
	if !ok {
		t.Fatal("paste not found?")
	}
	if err != nil {
		t.Fatal(err)
	}

	reader, err := p.Reader()
	if err != nil {
		t.Fatal(err)
	}

	_, err = ioutil.ReadAll(reader)
	if err != nil {
		t.Fatalf("got an error reading on a deleted paste: %v", err)
	}
}

func TestPasteEncryption(t *testing.T) {
	title := "My Test Paste"
	body := "secret data!"
	p, err := pqPasteService.CreatePaste(context.Background(), &spectre.PasteUpdate{
		Title:              &title,
		Body:               &body,
		PassphraseMaterial: []byte("passphrase"),
	})
	if err != nil {
		t.Fatal(err)
		return
	}

	pBad, err := pqPasteService.GetPaste(context.Background(), []byte("bad!"), p.GetID())
	if pBad != nil || err == nil {
		t.Fatal("got back a paste with a bad passphrase!")
	}

	pFacade, err := pqPasteService.GetPaste(context.Background(), nil, p.GetID())
	if err != spectre.ErrCryptorRequired {
		t.Fatal("didn't get an error reading an encrypted paste")
	}

	if pFacade == nil {
		t.Fatal("didn't get an encrypted paste facade")
	}

	if pFacade.GetID() != p.GetID() {
		t.Fatalf("IDs didn't match! Facade %v, real %v", pFacade.GetID(), p.GetID())
	}

	if pFacade.GetTitle() == p.GetTitle() {
		t.Fatal("title still accessible")
	}

	reader, err := pFacade.Reader()
	if reader != nil || err == nil {
		t.Fatal("encrypted paste retrieved without password readable?")
	}

	pReal, err := pqPasteService.GetPaste(context.Background(), []byte("passphrase"), p.GetID())
	if err != nil {
		t.Fatal(err)
	}

	if pReal.GetID() != p.GetID() {
		t.Fatalf("IDs didn't match! Facade %v, real %v", pFacade.GetID(), p.GetID())
	}

	if pReal.GetTitle() != p.GetTitle() {
		t.Fatalf("Titles didn't match! Facade %v, real %v", pFacade.GetTitle(), p.GetTitle())
	}

	realReader, err := pReal.Reader()
	if err != nil {
		t.Fatal(err)
	}

	rereadData, err := ioutil.ReadAll(realReader)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal([]byte(body), rereadData) {
		t.Fatalf("incomprehensible paste data; real <%s>, readback <%s>", body, string(rereadData))
	}
}

func TestPasteCollision(t *testing.T) {
	pgProvider := pqPasteService.(*conn)
	old := pgProvider.generateNewPasteID
	i := 0
	pgProvider.generateNewPasteID = func(encrypted bool) spectre.PasteID {
		// This function will be called once for the first paste and twice for the second paste.
		defer func() {
			i++
		}()
		switch i {
		case 0, 1:
			return spectre.PasteID("first_collision")
		}
		return spectre.PasteID(fmt.Sprintf("uniqued %d", i))
	}

	defer func() {
		pgProvider.generateNewPasteID = old
	}()

	p1, err := pqPasteService.CreatePaste(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
		return
	}
	if p1.GetID() != spectre.PasteID("first_collision") {
		t.Fatalf("expected first paste ID to be `first_collision`; got %v", p1.GetID())
	}

	p2, err := pqPasteService.CreatePaste(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
		return
	}
	if p2.GetID() != spectre.PasteID("uniqued 2") {
		t.Fatalf("expected second paste ID to be `uniqued 2`; got %v", p2.GetID())
	}
}
