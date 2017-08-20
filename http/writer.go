package http

import (
	"bufio"
	"bytes"
	"errors"
	"net"
	"net/http"
	"sync"
)

type Discarder interface {
	Discard() error
}

type bufferedResponseWriter struct {
	w http.ResponseWriter
	h http.Header
	o sync.Once

	b bytes.Buffer

	code    int
	written bool
	flushed bool

	hijacked bool
}

func (w *bufferedResponseWriter) Header() http.Header {
	w.o.Do(func() {
		w.h = http.Header{}
	})
	return w.h
}

func (w *bufferedResponseWriter) WriteHeader(code int) {
	w.code = code
	w.written = true
}

func (w *bufferedResponseWriter) Write(p []byte) (int, error) {
	return w.b.Write(p)
}

func (w *bufferedResponseWriter) Flush() {
	if !w.hijacked {
		if w.h != nil {
			ph := w.w.Header()
			for k, v := range w.h {
				ph[k] = v
			}
		}
		if w.written {
			w.w.WriteHeader(w.code)
		}
		w.b.WriteTo(w.w)
		w.flushed = true
	}
}

func (w *bufferedResponseWriter) Discard() error {
	if w.hijacked {
		return errors.New("BufferedResponseWriter: Discard on hijacked connection")
	}

	if w.flushed {
		return errors.New("BufferedResponseWriter: Discard on flushed connection")
	}

	w.h = http.Header{}
	w.code = 0
	w.written = false
	w.b = bytes.Buffer{}
	return nil
}

type hijackableBufferedResponseWriter struct {
	bufferedResponseWriter
}

func (w *hijackableBufferedResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h := w.bufferedResponseWriter.w.(http.Hijacker)
	w.bufferedResponseWriter.hijacked = true
	return h.Hijack()
}
