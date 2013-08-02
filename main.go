package main

import (
	"./expirator"
	"bytes"
	"encoding/gob"
	"flag"
	"github.com/golang/glog"
	"github.com/golang/groupcache/lru"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"html/template"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

const PASTE_CACHE_MAX_ENTRIES int = 1000
const MAX_EXPIRE_DURATION time.Duration = 15 * 24 * time.Hour

type PasteAccessDeniedError struct {
	action string
	ID     PasteID
}

func (e PasteAccessDeniedError) Error() string {
	return "You're not allowed to " + e.action + " paste " + e.ID.String()
}

// Make the various errors we can throw conform to HTTPError (here vs. the generic type file)
func (e PasteAccessDeniedError) StatusCode() int {
	return http.StatusForbidden
}

func (e PasteNotFoundError) StatusCode() int {
	return http.StatusNotFound
}

type GenericStringError string

func (e GenericStringError) Error() string {
	return string(e)
}

func sessionOk(r *http.Request) (b bool) {
	//ua := r.Header.Get("User-Agent")
	b = true
	//b = !strings.Contains(ua, "curl")
	return
}

func getPasteRawHandler(o Model, w http.ResponseWriter, r *http.Request) {
	p := o.(*Paste)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	reader, _ := p.Reader()
	defer reader.Close()
	io.Copy(w, reader)
}

func getPasteDownloadHandler(o Model, w http.ResponseWriter, r *http.Request) {
	p := o.(*Paste)
	lang := LanguageNamed(p.Language)
	mime := "text/plain"
	ext := "txt"
	if lang != nil {
		if len(lang.MIMETypes) > 0 {
			mime = lang.MIMETypes[0]
		}

		if len(lang.Extensions) > 0 {
			ext = lang.Extensions[0]
		}
	}

	w.Header().Set("Content-Disposition", "attachment; filename=\""+p.ID.String()+"."+ext+"\"")
	w.Header().Set("Content-Type", mime+"; charset=utf-8")
	w.Header().Set("Content-Transfer-Encoding", "binary")

	reader, _ := p.Reader()
	defer reader.Close()
	io.Copy(w, reader)
}

func isEditAllowed(p *Paste, r *http.Request) bool {
	session, _ := sessionStore.Get(r, "session")
	pastes, ok := session.Values["pastes"].([]string)
	if !ok {
		return false
	}

	for _, v := range pastes {
		if v == p.ID.String() {
			return true
		}
	}
	return false
}

func requiresEditPermission(fn ModelRenderFunc) ModelRenderFunc {
	return func(o Model, w http.ResponseWriter, r *http.Request) {
		defer errorRecoveryHandler(w)

		p := o.(*Paste)
		accerr := PasteAccessDeniedError{"modify", p.ID}
		if !isEditAllowed(p, r) {
			panic(accerr)
		}
		fn(p, w, r)
	}
}

func pasteUpdate(o Model, w http.ResponseWriter, r *http.Request) {
	p := o.(*Paste)
	body := r.FormValue("text")
	if len(strings.TrimSpace(body)) == 0 {
		w.Header().Set("Location", pasteURL("delete", p))
		w.WriteHeader(http.StatusFound)
		return
	}

	pw, _ := p.Writer()
	pw.Write([]byte(body))
	if r.FormValue("lang") != "" {
		p.Language = r.FormValue("lang")
	}
	pw.Close() // Saves p

	expireIn := r.FormValue("expire")
	if expireIn != "" {
		dur, _ := time.ParseDuration(expireIn)
		if dur > MAX_EXPIRE_DURATION {
			dur = MAX_EXPIRE_DURATION
		}
		pasteExpirator.ExpireObject(p, dur)
	} else {
		if pasteExpirator.ObjectHasExpiration(p) {
			pasteExpirator.CancelObjectExpiration(p)
		}
	}

	w.Header().Set("Location", pasteURL("show", p))
	w.WriteHeader(http.StatusSeeOther)
}

func pasteCreate(w http.ResponseWriter, r *http.Request) {
	body := r.FormValue("text")
	if len(strings.TrimSpace(body)) == 0 {
		// 400 here, 200 above (one is displayed to the user, one could be an API response.)
		RenderError(GenericStringError("Hey, put some text in that paste."), 400, w)
		return
	}

	password := r.FormValue("password")
	p, err := pasteStore.New(password != "")
	if err != nil {
		panic(err)
	}

	key := p.EncryptionKeyWithPassword(password)
	p.SetEncryptionKey(key)

	if sessionOk(r) {
		session, _ := sessionStore.Get(r, "session")
		pastes, ok := session.Values["pastes"].([]string)
		if !ok {
			pastes = []string{}
		}

		pastes = append(pastes, p.ID.String())
		session.Values["pastes"] = pastes

		if key != nil {
			cliSession, _ := clientOnlySessionStore.Get(r, "c_session")
			pasteKeys, ok := cliSession.Values["paste_keys"].(map[PasteID][]byte)
			if !ok {
				pasteKeys = map[PasteID][]byte{}
			}

			pasteKeys[p.ID] = key
			cliSession.Values["paste_keys"] = pasteKeys
		}

		sessions.Save(r, w)
	}

	pasteUpdate(p, w, r)
}

func pasteDelete(o Model, w http.ResponseWriter, r *http.Request) {
	p := o.(*Paste)
	oldId := p.ID.String()
	p.Destroy()

	session, _ := sessionStore.Get(r, "session")

	if session_pastes, ok := session.Values["pastes"].([]string); ok {
		pastes := make([]string, len(session_pastes)-1)
		n := 0
		for _, v := range session_pastes {
			if v == oldId {
				continue
			}
			pastes[n] = v
			n++
		}
		session.Values["pastes"] = pastes[:n]
		session.Save(r, w)
	}

	w.Header().Set("Location", "/")
	w.WriteHeader(http.StatusFound)
}

func lookupPasteWithRequest(r *http.Request) (Model, error) {
	id := PasteIDFromString(mux.Vars(r)["id"])
	var key []byte

	cliSession, _ := clientOnlySessionStore.Get(r, "c_session")
	if pasteKeys, ok := cliSession.Values["paste_keys"].(map[PasteID][]byte); ok {
		if _key, ok := pasteKeys[id]; ok {
			key = _key
		}
	}

	enc := false
	p, err := pasteStore.Get(id, key)
	if _, ok := err.(PasteEncryptedError); ok {
		enc = true
	}

	if _, ok := err.(PasteInvalidKeyError); ok {
		enc = true
	}

	if enc {
		url, _ := router.Get("paste_authenticate").URL("id", id.String())
		if _, ok := err.(PasteInvalidKeyError); ok {
			url.RawQuery = "i=1"
		}

		return nil, DeferLookupError{
			Interstitial: url,
		}
	}
	return p, err
}

func pasteURL(routeType string, p *Paste) string {
	url, _ := router.Get("paste_"+routeType).URL("id", p.ID.String())
	return url.String()
}

func sessionHandler(w http.ResponseWriter, r *http.Request) {
	var pastes []*Paste
	var ids []string
	session, _ := sessionStore.Get(r, "session")
	if session_pastes, ok := session.Values["pastes"].([]string); ok {
		pastes = make([]*Paste, len(session_pastes))
		ids = make([]string, len(session_pastes))
		n := 0
		for _, v := range session_pastes {
			if obj, _ := pasteStore.Get(PasteIDFromString(v), nil); obj != nil {
				pastes[n] = obj
				ids[n] = obj.ID.String()
				n++
			}
		}
		pastes = pastes[:n]
		ids = ids[:n]
	} else {
		pastes = []*Paste{}
		ids = []string{}
	}

	if strings.HasSuffix(r.URL.Path, "/raw") {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(strings.Join(ids, " ")))
	} else {
		ExecuteTemplate(w, "page_session_pastes", &RenderContext{pastes, r})
	}
}

func authenticatePastePOSTHandler(w http.ResponseWriter, r *http.Request) {
	if throttleAuthForRequest(r) {
		w.WriteHeader(420)
		return
	}

	id := PasteIDFromString(mux.Vars(r)["id"])
	password := r.FormValue("password")

	p, _ := pasteStore.Get(id, nil)
	if p == nil {
		panic(&PasteNotFoundError{ID: id})
	}

	key := p.EncryptionKeyWithPassword(password)
	if key != nil {
		cliSession, _ := clientOnlySessionStore.Get(r, "c_session")
		pasteKeys, ok := cliSession.Values["paste_keys"].(map[PasteID][]byte)
		if !ok {
			pasteKeys = map[PasteID][]byte{}
		}

		pasteKeys[id] = key
		cliSession.Values["paste_keys"] = pasteKeys
		sessions.Save(r, w)
	}

	url, _ := router.Get("paste_show").URL("id", id.String())
	w.Header().Set("Location", url.String())
	w.WriteHeader(http.StatusSeeOther)
}

func throttleAuthForRequest(r *http.Request) bool {
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.RemoteAddr[:strings.LastIndex(r.RemoteAddr, ":")]
	}

	id := mux.Vars(r)["id"]

	tok := ip + "|" + id

	var at *AuthThrottleEntry
	v, _ := authThrottler.Store.Get(expirator.ExpirableID(tok))
	if v != nil {
		at = v.(*AuthThrottleEntry)
	} else {
		at = &AuthThrottleEntry{ID: tok, Hits: 0}
		authThrottler.Store.(*ExpiringAuthThrottleStore).Add(at)
	}

	at.Hits++

	if at.Hits >= 5 {
		// If they've tried and failed too much, renew the throttle
		// at five minutes, to make them cool off.

		authThrottler.ExpireObject(at, 5*time.Minute)
		return true
	}

	if !authThrottler.ObjectHasExpiration(at) {
		authThrottler.ExpireObject(at, 1*time.Minute)
	}

	return false
}

func requestVariable(rc *RenderContext, variable string) string {
	v, _ := mux.Vars(rc.Request)[variable]
	if v == "" {
		v = rc.Request.FormValue(variable)
	}
	return v
}

type RedirectHandler string

func (h RedirectHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Location", string(h))
	w.WriteHeader(http.StatusFound)
}

type RenderedPaste struct {
	body       template.HTML
	renderTime time.Time
}

var renderCache struct {
	mu sync.RWMutex
	c  *lru.Cache
}

func renderPaste(p *Paste) template.HTML {
	renderCache.mu.RLock()
	var cached *RenderedPaste
	var cval interface{}
	var ok bool
	if renderCache.c != nil {
		if cval, ok = renderCache.c.Get(p.ID); ok {
			cached = cval.(*RenderedPaste)
		}
	}
	renderCache.mu.RUnlock()

	if !ok || cached.renderTime.Before(p.LastModified()) {
		defer renderCache.mu.Unlock()
		renderCache.mu.Lock()
		out, err := FormatPaste(p)

		if err != nil {
			glog.Errorf("Render for %s failed: (%s) output: %s", p.ID, err.Error(), out)
			return template.HTML("There was an error rendering this paste.")
		}

		rendered := template.HTML(out)
		if !p.Encrypted {
			if renderCache.c == nil {
				renderCache.c = &lru.Cache{
					MaxEntries: PASTE_CACHE_MAX_ENTRIES,
					OnEvicted: func(key lru.Key, value interface{}) {
						glog.Info("RENDER CACHE: Evicted ", key)
					},
				}
			}
			renderCache.c.Add(p.ID, &RenderedPaste{body: rendered, renderTime: time.Now()})
			glog.Info("RENDER CACHE: Cached ", p.ID)
		}

		return rendered
	} else {
		return cached.body
	}
}

func pasteDestroyCallback(p *Paste) {
	defer renderCache.mu.Unlock()
	renderCache.mu.Lock()
	if renderCache.c == nil {
		return
	}

	glog.Info("RENDER CACHE: Removing ", p.ID, " due to destruction.")
	// Clear the cached render when a paste is destroyed
	renderCache.c.Remove(p.ID)
}

var pasteStore *FilesystemPasteStore
var pasteExpirator *expirator.Expirator
var authThrottler *expirator.Expirator
var sessionStore *sessions.FilesystemStore
var clientOnlySessionStore *sessions.CookieStore
var router *mux.Router

type args struct {
	root, port, bind *string
	rebuild          *bool
	minified         *bool
}

func (a *args) register() {
	a.root = flag.String("root", "./", "path to generated file storage")
	a.port, a.bind = flag.String("port", "8080", "HTTP port"), flag.String("bind", "0.0.0.0", "bind address")
	a.rebuild = flag.Bool("rebuild", false, "rebuild all templates for each request")
	a.minified = flag.Bool("minified", false, "use min.js and min.css files (ala production mode)")
}

func (a *args) parse() {
	flag.Parse()
}

var arguments = &args{}

type ReloadFunction func()

var reloadFunctions = []ReloadFunction{}

func RegisterReloadFunction(f ReloadFunction) {
	reloadFunctions = append(reloadFunctions, f)
}

func init() {
	// N.B. this should not be necessary.
	gob.Register(map[PasteID][]byte(nil))

	arguments.register()
	arguments.parse()

	runtime.GOMAXPROCS(runtime.NumCPU())
	RegisterTemplateFunction("editAllowed", func(ri *RenderContext) bool { return isEditAllowed(ri.Obj.(*Paste), ri.Request) })
	RegisterTemplateFunction("render", renderPaste)
	RegisterTemplateFunction("pasteURL", pasteURL)
	RegisterTemplateFunction("pasteBody", func(p *Paste) string {
		reader, _ := p.Reader()
		defer reader.Close()
		b := &bytes.Buffer{}
		io.Copy(b, reader)
		return b.String()
	})
	RegisterTemplateFunction("requestVariable", requestVariable)

	sesdir := filepath.Join(*arguments.root, "sessions")
	os.Mkdir(sesdir, 0700)
	var sessionKey []byte = nil
	if sessionKeyFile, err := os.Open(filepath.Join(*arguments.root, "session.key")); err == nil {
		buf := &bytes.Buffer{}
		io.Copy(buf, sessionKeyFile)
		sessionKey = buf.Bytes()
		sessionKeyFile.Close()
	} else {
		glog.Fatal("session.key not found. make one with seskey.go?")
	}
	sessionStore = sessions.NewFilesystemStore(sesdir, sessionKey)
	sessionStore.Options.Path = "/"
	sessionStore.Options.MaxAge = 86400 * 365

	var clientOnlySessionEncryptionKey []byte = nil
	if sessionKeyFile, err := os.Open(filepath.Join(*arguments.root, "client_session_enc.key")); err == nil {
		buf := &bytes.Buffer{}
		io.Copy(buf, sessionKeyFile)
		clientOnlySessionEncryptionKey = buf.Bytes()
		sessionKeyFile.Close()
	} else {
		glog.Fatal("client_session_enc.key not found. make one with seskey.go?")
	}
	clientOnlySessionStore = sessions.NewCookieStore(sessionKey, clientOnlySessionEncryptionKey)
	clientOnlySessionStore.Options.Path = "/"
	clientOnlySessionStore.Options.MaxAge = 0

	pastedir := filepath.Join(*arguments.root, "pastes")
	os.Mkdir(pastedir, 0700)
	pasteStore = NewFilesystemPasteStore(pastedir)
	pasteStore.PasteDestroyCallback = PasteCallback(pasteDestroyCallback)

	pasteExpirator = expirator.NewExpirator(filepath.Join(*arguments.root, "expiry.gob"), &ExpiringPasteStore{pasteStore})
	authThrottler = expirator.NewExpirator("", NewExpiringAuthThrottleStore())
}

func main() {
	InitTemplates()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGHUP)
	go func() {
		for _ = range sigChan {
			glog.Info("Received SIGHUP")
			for _, f := range reloadFunctions {
				f()
			}
		}
	}()

	go pasteExpirator.Run()
	go authThrottler.Run()

	router = mux.NewRouter()

	if getRouter := router.Methods("GET").Subrouter(); getRouter != nil {
		getRouter.Handle("/paste/new", RedirectHandler("/"))
		getRouter.HandleFunc("/paste/{id}", RequiredModelObjectHandler(lookupPasteWithRequest, RenderTemplateForModel("paste_show"))).Name("paste_show")
		getRouter.HandleFunc("/paste/{id}/raw", RequiredModelObjectHandler(lookupPasteWithRequest, ModelRenderFunc(getPasteRawHandler))).Name("paste_raw")
		getRouter.HandleFunc("/paste/{id}/download", RequiredModelObjectHandler(lookupPasteWithRequest, ModelRenderFunc(getPasteDownloadHandler))).Name("paste_download")
		getRouter.HandleFunc("/paste/{id}/edit", RequiredModelObjectHandler(lookupPasteWithRequest, requiresEditPermission(RenderTemplateForModel("paste_edit")))).Name("paste_edit")
		getRouter.HandleFunc("/paste/{id}/delete", RequiredModelObjectHandler(lookupPasteWithRequest, requiresEditPermission(RenderTemplateForModel("paste_delete_confirm")))).Name("paste_delete")
		getRouter.HandleFunc("/paste/{id}/authenticate", RenderTemplateHandler("paste_authenticate")).Name("paste_authenticate")
		getRouter.Handle("/paste/", RedirectHandler("/"))
		getRouter.Handle("/paste", RedirectHandler("/"))
		getRouter.HandleFunc("/session", http.HandlerFunc(sessionHandler))
		getRouter.HandleFunc("/session/raw", http.HandlerFunc(sessionHandler))
		getRouter.HandleFunc("/", RenderTemplateHandler("index"))
	}
	if postRouter := router.Methods("POST").Subrouter(); postRouter != nil {
		postRouter.HandleFunc("/paste/{id}/edit", RequiredModelObjectHandler(lookupPasteWithRequest, requiresEditPermission(pasteUpdate)))
		postRouter.HandleFunc("/paste/{id}/delete", RequiredModelObjectHandler(lookupPasteWithRequest, requiresEditPermission(pasteDelete)))
		postRouter.HandleFunc("/paste/{id}/authenticate", http.HandlerFunc(authenticatePastePOSTHandler)).Name("paste_authenticate")
		postRouter.HandleFunc("/paste/new", http.HandlerFunc(pasteCreate))
	}
	router.PathPrefix("/").Handler(http.FileServer(http.Dir("./public")))

	var addr string = *arguments.bind + ":" + *arguments.port
	server := &http.Server{
		Addr:    addr,
		Handler: &fourOhFourConsumerHandler{router},
	}
	server.ListenAndServe()
}
