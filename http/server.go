package http

import (
	"crypto/tls"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"howett.net/spectre"

	"github.com/gorilla/handlers"
)

/*
// TODO(DH): Consider this.
// ### Pros
//  * We can buffer the response; if an error is encountered we can destroy it
//  * We can accumulate its size for logging purposes
// ### Cons
//  * Need to reimplement Hijacker and CloseNotifier
//  * Need to reimplement Headers and any other mutators
type bufferedResponseWriter struct {
	buf bytes.Buffer
	hijacked bool
}

type hijackableBufferedResponseWriter struct {
	bufferedResponseWriter
	http.Hijacker
}

func (h *hijackableBufferedResponseWriter) Hijack() (net.Conn, bufio.ReadWriter, error) {
	h.bufferedResponseWriter.hijacked = true
	return h.Hijacker.Hijack()
}
*/

/*
// TODO(DH): Consider whether we need to do this
type stripPrefixHandler struct {
	prefix string
	handler http.Handler
}

func (s *stripPrefixHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
}
*/

type Server struct {
	Addr         string
	TLSConfig    *tls.Config
	Proxied      bool
	DocumentRoot string

	// User-controlled: Services
	PasteService  spectre.PasteService
	UserService   spectre.UserService
	GrantService  spectre.GrantService
	ReportService spectre.ReportService

	// Internal: handlers
	prefixes map[string]http.Handler

	once   sync.Once
	server *http.Server
}

func (s *Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	clean := path.Clean("/" + r.URL.Path)
	if clean != "/" {
		path := filepath.Join(s.DocumentRoot, clean)
		if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
			http.ServeFile(w, r, path)
			return
		}
	}

	// now look for handler prefixes.
	pidx := strings.IndexByte(clean[1:], '/')
	prefix := clean
	if pidx != -1 {
		prefix = prefix[:pidx+1]
	}

	if handler, ok := s.prefixes[prefix]; ok {
		handler.ServeHTTP(w, r)
		return
	}
}

func (s *Server) rootHandler() http.Handler {
	return http.HandlerFunc(s.serveHTTP)
}

func (s *Server) addPrefixedHandler(prefix string, handler http.Handler) {
	s.prefixes[prefix] = handler
}

func (s *Server) init() {
	handler := s.rootHandler()

	s.prefixes = make(map[string]http.Handler)

	// TODO(DH): Killify this
	ph := &pasteHandler{
		PasteService: s.PasteService,
		Permitter:    nil,
	}

	s.addPrefixedHandler("/paste", ph)

	if s.Proxied {
		handler = handlers.ProxyHeaders(handler)
	}

	s.server = &http.Server{
		Addr:      s.Addr,
		Handler:   handler,
		TLSConfig: s.TLSConfig,
	}
}

func (s *Server) Listen() error {
	s.once.Do(func() {
		s.init()
	})

	if s.TLSConfig != nil {
		return s.server.ListenAndServeTLS("", "")
	}

	return s.server.ListenAndServe()
}
