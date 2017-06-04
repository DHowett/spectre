package postgres

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/DHowett/ghostbin/model"
)

func TestPaste(t *testing.T) {
	// used in each subtest to look up the paste anew.
	var pID model.PasteID

	t.Run("Create", func(t *testing.T) {
		p, err := gTestProvider.CreatePaste()
		if err != nil {
			t.Fatal(err)
			return
		}
		pID = p.GetID()
	})

	t.Run("GetValues", func(t *testing.T) {
		_, err := gTestProvider.GetPaste(pID, nil)
		if err != nil {
			t.Fatal("failed to get", pID, ":", err)
		}
	})

	t.Run("ReadBody1", func(t *testing.T) {
		p, err := gTestProvider.GetPaste(pID, nil)
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
		p, err := gTestProvider.GetPaste(pID, nil)
		if err != nil {
			t.Fatal("failed to get", pID, ":", err)
		}
		writer, err := p.Writer()
		if err != nil {
			t.Fatal(err)
		}
		n, err := writer.Write([]byte("hello"))
		if n != 5 || err != nil {
			t.Fatalf("failed to write 5 bytes; got %d: %v", n, err)
		}
		err = writer.Close()
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("ReadBody2", func(t *testing.T) {
		p, err := gTestProvider.GetPaste(pID, nil)
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
		p, err := gTestProvider.GetPaste(pID, nil)
		if err != nil {
			t.Fatal("failed to get", pID, ":", err)
		}
		writer, err := p.Writer()
		if err != nil {
			t.Fatal(err)
		}
		n, err := writer.Write([]byte("goodbye"))
		if n != 7 || err != nil {
			t.Fatalf("failed to write 7 bytes; got %d: %v", n, err)
		}
		err = writer.Close()
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("ReadBody3", func(t *testing.T) {
		p, err := gTestProvider.GetPaste(pID, nil)
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
		p, err := gTestProvider.GetPaste(pID, nil)
		if err != nil {
			t.Fatal("failed to get", pID, ":", err)
		}
		err = p.Erase()
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("LookupAfterDestroy", func(t *testing.T) {
		p, err := gTestProvider.GetPaste(pID, nil)
		if p != nil || err == nil {
			t.Fatal("got a paste back/no error?")
		}
	})
}

func TestPasteReadAfterDestroy(t *testing.T) {
	p, err := gTestProvider.CreatePaste()
	if err != nil {
		t.Fatal(err)
		return
	}

	err = p.Erase()
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
	p, err := gTestProvider.CreateEncryptedPaste(model.PasteEncryptionMethodAES_CTR, []byte("passphrase"))
	if err != nil {
		t.Fatal(err)
		return
	}

	p.SetTitle("My Test Paste")

	w, err := p.Writer()
	if err != nil {
		t.Fatal(err)
	}

	data := []byte("secret data!")
	_, err = w.Write(data)
	if err != nil {
		t.Fatal(err)
	}

	err = w.Close()
	if err != nil {
		t.Fatal(err)
	}

	pBad, err := gTestProvider.GetPaste(p.GetID(), []byte("bad!"))
	if pBad != nil || err == nil {
		t.Fatal("got back a paste with a bad passphrase!")
	}

	pFacade, err := gTestProvider.GetPaste(p.GetID(), nil)
	if err != model.ErrPasteEncrypted {
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

	pReal, err := gTestProvider.GetPaste(p.GetID(), []byte("passphrase"))
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

	if !bytes.Equal(data, rereadData) {
		t.Fatalf("incomprehensible paste data; real <%s>, readback <%s>", string(data), string(rereadData))
	}

	p.Erase()
}

func TestPasteCollision(t *testing.T) {
	pgProvider := gTestProvider.(*provider)
	old := pgProvider.GenerateNewPasteID
	i := 0
	pgProvider.GenerateNewPasteID = func(encrypted bool) model.PasteID {
		// This function will be called once for the first paste and twice for the second paste.
		defer func() {
			i++
		}()
		switch i {
		case 0, 1:
			return model.PasteID("first_collision")
		}
		return model.PasteID(fmt.Sprintf("uniqued %d", i))
	}

	defer func() {
		pgProvider.GenerateNewPasteID = old
	}()

	p1, err := gTestProvider.CreatePaste()
	if err != nil {
		t.Fatal(err)
		return
	}
	if p1.GetID() != model.PasteID("first_collision") {
		t.Fatalf("expected first paste ID to be `first_collision`; got %v", p1.GetID())
	}

	p2, err := gTestProvider.CreatePaste()
	if err != nil {
		t.Fatal(err)
		return
	}
	if p2.GetID() != model.PasteID("uniqued 2") {
		t.Fatalf("expected second paste ID to be `uniqued 2`; got %v", p2.GetID())
	}
}
