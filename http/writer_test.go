package http

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestBufferedResponseWriter(w http.ResponseWriter) (http.ResponseWriter, http.Flusher, Discarder) {
	brw := &bufferedResponseWriter{w: w}
	return brw, brw, brw
}

func TestBufferedResponseWriter(t *testing.T) {
	t.Run("WriteAndFlush", func(t *testing.T) {
		rr := httptest.NewRecorder()
		w, fl, _ := newTestBufferedResponseWriter(rr)

		w.Write([]byte("test"))
		fl.Flush()

		resp := rr.Result()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("unexpected response status: %d", resp.StatusCode)
		}

		body, _ := ioutil.ReadAll(resp.Body)
		if bytes.Compare(body, []byte("test")) != 0 {
			t.Fatalf("unexpected response body: %x", body)
		}
	})

	t.Run("SetHeaders", func(t *testing.T) {
		rr := httptest.NewRecorder()
		w, fl, _ := newTestBufferedResponseWriter(rr)

		w.Header().Add("X-Test", "Value1")
		w.Header().Add("X-Test", "Value2")
		w.Write([]byte("test"))
		fl.Flush()

		resp := rr.Result()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("unexpected response status: %d", resp.StatusCode)
		}

		body, _ := ioutil.ReadAll(resp.Body)
		if bytes.Compare(body, []byte("test")) != 0 {
			t.Fatalf("unexpected response body: %x", body)
		}

		h := resp.Header["X-Test"]
		if len(h) != 2 && h[0] != "Value1" && h[2] != "Value2" {
			t.Fatalf("unexpected X-Test header: %v", h)
		}
	})

	t.Run("SetStatusBefore", func(t *testing.T) {
		rr := httptest.NewRecorder()
		w, fl, _ := newTestBufferedResponseWriter(rr)

		w.WriteHeader(501)
		w.Write([]byte("test"))
		fl.Flush()

		resp := rr.Result()
		if resp.StatusCode != 501 {
			t.Fatalf("unexpected response status: %d", resp.StatusCode)
		}

		body, _ := ioutil.ReadAll(resp.Body)
		if bytes.Compare(body, []byte("test")) != 0 {
			t.Fatalf("unexpected response body: %x", body)
		}
	})

	t.Run("SetStatusAfter", func(t *testing.T) {
		rr := httptest.NewRecorder()
		w, fl, _ := newTestBufferedResponseWriter(rr)

		w.Write([]byte("test"))
		w.WriteHeader(501)
		fl.Flush()

		resp := rr.Result()
		if resp.StatusCode != 501 {
			t.Fatalf("unexpected response status: %d", resp.StatusCode)
		}

		body, _ := ioutil.ReadAll(resp.Body)
		if bytes.Compare(body, []byte("test")) != 0 {
			t.Fatalf("unexpected response body: %x", body)
		}
	})

	t.Run("Discard", func(t *testing.T) {
		rr := httptest.NewRecorder()
		w, fl, disc := newTestBufferedResponseWriter(rr)

		w.Header().Add("X-Test", "Value1")
		w.Header().Add("X-Test", "Value2")
		w.Write([]byte("test"))
		w.WriteHeader(501)
		disc.Discard()
		fl.Flush()

		resp := rr.Result()
		if resp.StatusCode != 200 {
			t.Fatalf("unexpected response status: %d", resp.StatusCode)
		}

		body, _ := ioutil.ReadAll(resp.Body)
		if len(body) != 0 {
			t.Fatalf("unexpected response body: %x", body)
		}

		if resp.Header["X-Test"] != nil {
			t.Fatalf("unexpected X-Test header: %v", resp.Header["X-Test"])
		}
	})

	t.Run("DiscardAndReuse", func(t *testing.T) {
		rr := httptest.NewRecorder()
		w, fl, disc := newTestBufferedResponseWriter(rr)

		w.Header().Add("X-Test", "Bad1")
		w.Header().Add("X-Test", "Bad2")
		w.Write([]byte("original"))
		w.WriteHeader(http.StatusOK)
		disc.Discard()

		w.Header().Add("X-Test", "Value1")
		w.Header().Add("X-Test", "Value2")
		w.Write([]byte("test"))
		w.WriteHeader(501)
		fl.Flush()

		resp := rr.Result()
		if resp.StatusCode != 501 {
			t.Fatalf("unexpected response status: %d", resp.StatusCode)
		}

		body, _ := ioutil.ReadAll(resp.Body)
		if bytes.Compare(body, []byte("test")) != 0 {
			t.Fatalf("unexpected response body: %x", body)
		}

		h := resp.Header["X-Test"]
		if len(h) != 2 && h[0] != "Value1" && h[2] != "Value2" {
			t.Fatalf("unexpected X-Test header: %v", h)
		}
	})

	t.Run("DiscardAfterFlush", func(t *testing.T) {
		rr := httptest.NewRecorder()
		w, fl, disc := newTestBufferedResponseWriter(rr)

		w.Header().Add("X-Test", "Bad1")
		w.Header().Add("X-Test", "Bad2")
		w.Write([]byte("original"))
		w.WriteHeader(http.StatusOK)
		fl.Flush()
		err := disc.Discard()
		if err == nil {
			t.Fatalf("expected error on discard; didn't get one!")
		}
	})
}
