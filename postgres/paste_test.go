package postgres

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"testing"
	"time"

	"howett.net/spectre"
)

type testReadCloser struct {
	io.ReadCloser
}

func (r *testReadCloser) Read(p []byte) (int, error) {
	b := make([]byte, len(p))
	n, err := r.ReadCloser.Read(b)
	if err != nil {
		return n, err
	}
	for i, v := range b {
		p[i] = byte((int(v) + 256 - 1) % 256)
	}
	return n, nil
}

type testWriteCloser struct {
	io.WriteCloser
}

func (r *testWriteCloser) Write(p []byte) (int, error) {
	b := make([]byte, len(p))
	copy(b, p)
	for i, v := range b {
		b[i] = byte((int(v) + 1) % 256)
	}
	return r.WriteCloser.Write(b)
}

type testCryptor struct {
	passphrase string
}

func (cr *testCryptor) Authenticate(salt []byte, challenge []byte) (bool, error) {
	return string(challenge) == (cr.passphrase + string(salt)), nil
}

func (cr *testCryptor) Challenge() ([]byte, []byte, error) {
	return []byte(cr.passphrase + "a"), []byte{'a'}, nil
}

func (cr *testCryptor) Reader(r io.ReadCloser) (io.ReadCloser, error) {
	return &testReadCloser{r}, nil
}

func (cr *testCryptor) Writer(w io.WriteCloser) (io.WriteCloser, error) {
	return &testWriteCloser{w}, nil
}

func (cr *testCryptor) EncryptionMethod() spectre.EncryptionMethod {
	return spectre.EncryptionMethod(1)
}

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
		p, err := pqPasteService.GetPaste(context.Background(), nil, pID)
		if err != nil {
			t.Fatal("failed to get", pID, ":", err)
		}
		err = p.Erase()
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

	t.Run("Create", func(t *testing.T) {
		p, err := pqPasteService.CreatePaste(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
			return
		}
		pID = p.GetID()
	})

	expTime := time.Now().Add(1 * time.Hour)

	t.Run("SetValues1", func(t *testing.T) {
		p, err := pqPasteService.GetPaste(context.Background(), nil, pID)
		if err != nil {
			t.Fatal("failed to get", pID, ":", err)
		}

		p.SetTitle("Hello World")
		p.SetLanguageName("c")
		p.SetExpirationTime(expTime)

		err = p.Commit()
		if err != nil {
			t.Fatal("failed to save", pID, err)
		}
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

		p.SetTitle("")
		p.ClearExpirationTime()

		err = p.Commit()
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

	t.Run("Create", func(t *testing.T) {
		p, err := pqPasteService.CreatePaste(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
			return
		}
		pID = p.GetID()
	})

	expTime := time.Now().Add(1 * time.Hour)

	t.Run("SetValues1", func(t *testing.T) {
		p, err := pqPasteService.GetPaste(context.Background(), nil, pID)
		if err != nil {
			t.Fatal("failed to get", pID, ":", err)
		}

		p.SetTitle("Hello World")
		p.SetLanguageName("c")
		p.SetExpirationTime(expTime)

		writer, err := p.Writer()
		if err != nil {
			t.Fatal(err)
		}
		_, _ = writer.Write([]byte{'-'})
		err = writer.Close()
		if err != nil {
			t.Fatal(err)
		}
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

		p.SetTitle("")
		p.ClearExpirationTime()

		writer, err := p.Writer()
		if err != nil {
			t.Fatal(err)
		}
		_, _ = writer.Write([]byte{'-'})
		err = writer.Close()
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
	p, err := pqPasteService.CreatePaste(context.Background(), &testCryptor{"passphrase"})
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

	pBad, err := pqPasteService.GetPaste(context.Background(), &testCryptor{"bad!"}, p.GetID())
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

	pReal, err := pqPasteService.GetPaste(context.Background(), &testCryptor{"passphrase"}, p.GetID())
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

	//p.Erase()
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
