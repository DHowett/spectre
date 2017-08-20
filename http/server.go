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

	// User-controlled: web services
	SessionService            SessionService
	RequestUserService        UserService
	RequestPermitterProvider  PermitterProvider

	// Internal: handlers
	prefixes map[string]http.Handler

	once   sync.Once
	server *http.Server
}

func (s *Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	var flusher http.Flusher

	if _, ok := w.(http.Hijacker); ok {
		bufw := &hijackableBufferedResponseWriter{bufferedResponseWriter{w: w}}
		w = bufw
		flusher = bufw
	} else {
		bufw := &bufferedResponseWriter{w: w}
		w = bufw
		flusher = bufw
	}

	defer flusher.Flush()

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
	}


	wps := &contextBindingPermitterProvider{
		Handler:           ph,
		PermitterProvider: s.RequestPermitterProvider,
	}

	ph.PermitterProvider = wps

	s.addPrefixedHandler("/paste", wps)

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
