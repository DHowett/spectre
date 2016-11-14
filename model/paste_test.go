package model

import (
	"bytes"
	"io/ioutil"
	"testing"
)

func TestPaste(t *testing.T) {
	// used in each subtest to look up the paste anew.
	var pID PasteID

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

	reader, err := p.Reader()
	if err != nil {
		t.Error(err)
	}

	err = p.Erase()
	if err != nil {
		t.Error(err)
	}

	_, err = ioutil.ReadAll(reader)
	if err != nil {
		t.Errorf("got an error reading on a deleted paste: %v", err)
	}
}
