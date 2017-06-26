package http

import (
	"bufio"
	"bytes"
	"net"
	"net/http"
)

type Discarder interface {
	Discard()
}

// TODO(DH): Consider this.
// ### Pros
//  * We can buffer the response; if an error is encountered we can destroy it
//  * We can accumulate its size for logging purposes
// ### Cons
//  * Need to reimplement Hijacker and CloseNotifier
//  * Need to reimplement Headers and any other mutators
type bufferedResponseWriter struct {
	w http.ResponseWriter

	bytes.Buffer

	code    int
	written bool

	hijacked bool
}

func (w *bufferedResponseWriter) Header() http.Header {
	return w.w.Header()
}

func (w *bufferedResponseWriter) WriteHeader(code int) {
	w.code = code
	w.written = true
}

// Write is implemented by bytes.Buffer

func (w *bufferedResponseWriter) Flush() {
	if !w.hijacked {
		if w.written {
			w.w.WriteHeader(w.code)
		}
		w.Buffer.WriteTo(w.w)
	}
}

type hijackableBufferedResponseWriter struct {
	bufferedResponseWriter
}

func (w *hijackableBufferedResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h := w.bufferedResponseWriter.w.(http.Hijacker)
	w.bufferedResponseWriter.hijacked = true
	return h.Hijack()
}
