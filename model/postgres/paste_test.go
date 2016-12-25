package postgres

import (
	"bytes"
	"io/ioutil"
	"testing"

	"github.com/DHowett/ghostbin/model"
)

func TestPaste(t *testing.T) {
	// used in each subtest to look up the paste anew.
	var pID model.PasteID

	t.Run("Create", func(t *testing.T) {
		p, err := broker.CreatePaste()
		if err != nil {
			t.Error(err)
			return
		}
		pID = p.GetID()
	})

	t.Run("GetValues", func(t *testing.T) {
		_, err := broker.GetPaste(pID, nil)
		if err != nil {
			t.Error("failed to get", pID, ":", err)
		}
	})

	t.Run("ReadBody1", func(t *testing.T) {
		p, err := broker.GetPaste(pID, nil)
		if err != nil {
			t.Error("failed to get", pID, ":", err)
		}
		reader, err := p.Reader()
		if err != nil {
			t.Error(err)
		}
		buf, err := ioutil.ReadAll(reader)
		if err != nil {
			t.Error(err)
		}
		if len(buf) != 0 {
			t.Errorf("newly-created paste had non-empty buffer: <%s>", string(buf))
		}
	})

	t.Run("CreateBody", func(t *testing.T) {
		p, err := broker.GetPaste(pID, nil)
		if err != nil {
			t.Error("failed to get", pID, ":", err)
		}
		writer, err := p.Writer()
		if err != nil {
			t.Error(err)
		}
		n, err := writer.Write([]byte("hello"))
		if n != 5 || err != nil {
			t.Errorf("failed to write 5 bytes; got %d: %v", n, err)
		}
		err = writer.Close()
		if err != nil {
			t.Error(err)
		}
	})

	t.Run("ReadBody2", func(t *testing.T) {
		p, err := broker.GetPaste(pID, nil)
		if err != nil {
			t.Error("failed to get", pID, ":", err)
		}
		reader, err := p.Reader()
		if err != nil {
			t.Error(err)
		}
		buf, err := ioutil.ReadAll(reader)
		if err != nil {
			t.Error(err)
		}
		if !bytes.Equal(buf, []byte("hello")) {
			t.Errorf("paste had incomprehensible body <%s>", string(buf))
		}
	})

	t.Run("UpdateBody", func(t *testing.T) {
		p, err := broker.GetPaste(pID, nil)
		if err != nil {
			t.Error("failed to get", pID, ":", err)
		}
		writer, err := p.Writer()
		if err != nil {
			t.Error(err)
		}
		n, err := writer.Write([]byte("goodbye"))
		if n != 7 || err != nil {
			t.Errorf("failed to write 7 bytes; got %d: %v", n, err)
		}
		err = writer.Close()
		if err != nil {
			t.Error(err)
		}
	})

	t.Run("ReadBody3", func(t *testing.T) {
		p, err := broker.GetPaste(pID, nil)
		if err != nil {
			t.Error("failed to get", pID, ":", err)
		}
		reader, err := p.Reader()
		if err != nil {
			t.Error(err)
		}
		buf, err := ioutil.ReadAll(reader)
		if err != nil {
			t.Error(err)
		}
		if !bytes.Equal(buf, []byte("goodbye")) {
			t.Errorf("paste had incomprehensible body <%s>", string(buf))
		}
	})

	t.Run("Destroy", func(t *testing.T) {
		p, err := broker.GetPaste(pID, nil)
		if err != nil {
			t.Error("failed to get", pID, ":", err)
		}
		err = p.Erase()
		if err != nil {
			t.Error(err)
		}
	})

	t.Run("LookupAfterDestroy", func(t *testing.T) {
		p, err := broker.GetPaste(pID, nil)
		if p != nil || err == nil {
			t.Error("got a paste back/no error?")
		}
	})
}

func TestPasteReadAfterDestroy(t *testing.T) {
	p, err := broker.CreatePaste()
	if err != nil {
		t.Error(err)
		return
	}

	err = p.Erase()
	if err != nil {
		t.Error(err)
	}

	reader, err := p.Reader()
	if err != nil {
		t.Error(err)
	}

	_, err = ioutil.ReadAll(reader)
	if err != nil {
		t.Errorf("got an error reading on a deleted paste: %v", err)
	}
}

func TestPasteEncryption(t *testing.T) {
	p, err := broker.CreateEncryptedPaste(model.PasteEncryptionMethodAES_CTR, []byte("passphrase"))
	if err != nil {
		t.Error(err)
		return
	}

	p.SetTitle("My Test Paste")

	w, err := p.Writer()
	if err != nil {
		t.Error(err)
	}

	data := []byte("secret data!")
	_, err = w.Write(data)
	if err != nil {
		t.Error(err)
	}

	err = w.Close()
	if err != nil {
		t.Error(err)
	}

	pBad, err := broker.GetPaste(p.GetID(), []byte("bad!"))
	if pBad != nil || err == nil {
		t.Error("got back a paste with a bad passphrase!")
	}

	pFacade, err := broker.GetPaste(p.GetID(), nil)
	if err != model.ErrPasteEncrypted {
		t.Error("didn't get an error reading an encrypted paste")
	}

	if pFacade == nil {
		t.Error("didn't get an encrypted paste facade")
	}

	if pFacade.GetID() != p.GetID() {
		t.Errorf("IDs didn't match! Facade %v, real %v", pFacade.GetID(), p.GetID())
	}

	if pFacade.GetTitle() == p.GetTitle() {
		t.Error("title still accessible")
	}

	reader, err := pFacade.Reader()
	if reader != nil || err == nil {
		t.Error("encrypted paste retrieved without password readable?")
	}

	pReal, err := broker.GetPaste(p.GetID(), []byte("passphrase"))
	if err != nil {
		t.Error(err)
	}

	if pReal.GetID() != p.GetID() {
		t.Errorf("IDs didn't match! Facade %v, real %v", pFacade.GetID(), p.GetID())
	}

	if pReal.GetTitle() != p.GetTitle() {
		t.Errorf("Titles didn't match! Facade %v, real %v", pFacade.GetTitle(), p.GetTitle())
	}

	realReader, err := pReal.Reader()
	if err != nil {
		t.Error(err)
	}

	rereadData, err := ioutil.ReadAll(realReader)
	if err != nil {
		t.Error(err)
	}

	if !bytes.Equal(data, rereadData) {
		t.Errorf("incomprehensible paste data; real <%s>, readback <%s>", string(data), string(rereadData))
	}

	p.Erase()
}
