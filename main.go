package main

import (
	"bytes"
	"crypto/md5"
	"encoding/gob"
	"flag"
	"fmt"
	"github.com/DHowett/gotimeout"
	"github.com/golang/glog"
	"github.com/golang/groupcache/lru"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const PASTE_CACHE_MAX_ENTRIES int = 1000
const PASTE_MAXIMUM_LENGTH int = 1048576 // 1 MB
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

type PasteTooLargeError int

func (e PasteTooLargeError) Error() string {
	return fmt.Sprintf("Paste length %d exceeds maximum length of %d.", e, PASTE_MAXIMUM_LENGTH)
}

func (e PasteTooLargeError) StatusCode() int {
	return http.StatusBadRequest
}

type GenericStringError string

func (e GenericStringError) Error() string {
	return string(e)
}

func getPasteRawHandler(o Model, w http.ResponseWriter, r *http.Request) {
	p := o.(*Paste)
	mime := "text/plain"
	ext := "txt"
	if mux.CurrentRoute(r).GetName() == "download" {
		lang := LanguageNamed(p.Language)
		if lang != nil {
			if len(lang.MIMETypes) > 0 {
				mime = lang.MIMETypes[0]
			}

			if len(lang.Extensions) > 0 {
				ext = lang.Extensions[0]
			}
		}

		w.Header().Set("Content-Disposition", "attachment; filename=\""+p.ID.String()+"."+ext+"\"")
		w.Header().Set("Content-Transfer-Encoding", "binary")
	}
	w.Header().Set("Content-Type", mime+"; charset=utf-8")

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
	pasteUpdateCore(o, w, r, false)
}

func pasteUpdateCore(o Model, w http.ResponseWriter, r *http.Request, newPaste bool) {
	p := o.(*Paste)
	body := r.FormValue("text")
	if len(strings.TrimSpace(body)) == 0 {
		w.Header().Set("Location", pasteURL("delete", p))
		w.WriteHeader(http.StatusFound)
		return
	}

	if len(body) > PASTE_MAXIMUM_LENGTH {
		panic(PasteTooLargeError(len(body)))
	}

	if !newPaste {
		// If this is an update (instead of a new paste), blow away the hash.
		tok := "P|H|" + p.ID.String()
		v, _ := ephStore.Get(tok)
		if hash, ok := v.(string); ok {
			ephStore.Delete(hash)
			ephStore.Delete(tok)
		}
	}

	pw, _ := p.Writer()
	pw.Write([]byte(body))
	if r.FormValue("lang") != "" {
		p.Language = r.FormValue("lang")
	}

	expireIn := r.FormValue("expire")
	if expireIn != "" && expireIn != "-1" {
		dur, _ := time.ParseDuration(expireIn)
		if dur > MAX_EXPIRE_DURATION {
			dur = MAX_EXPIRE_DURATION
		}
		pasteExpirator.ExpireObject(p, dur)
	} else {
		if expireIn == "-1" && pasteExpirator.ObjectHasExpiration(p) {
			pasteExpirator.CancelObjectExpiration(p)
		}
	}

	p.Expiration = expireIn

	pw.Close() // Saves p

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

	if len(body) > PASTE_MAXIMUM_LENGTH {
		err := PasteTooLargeError(len(body))
		RenderError(err, err.StatusCode(), w)
		return
	}

	password := r.FormValue("password")
	encrypted := password != ""

	hasher := md5.New()
	io.WriteString(hasher, body)
	hashToken := "H|" + SourceIPForRequest(r) + "|" + base32Encoder.EncodeToString(hasher.Sum(nil))

	if !encrypted {
		v, _ := ephStore.Get(hashToken)
		if hashedPaste, ok := v.(*Paste); ok {
			pasteUpdateCore(hashedPaste, w, r, true)
			return
		}
	}

	p, err := pasteStore.New(password != "")
	if err != nil {
		panic(err)
	}

	if !encrypted {
		ephStore.Put(hashToken, p, 5*time.Minute)
		ephStore.Put("P|H|"+p.ID.String(), hashToken, 5*time.Minute)
	}

	key := p.EncryptionKeyWithPassword(password)
	p.SetEncryptionKey(key)

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

	pasteUpdateCore(p, w, r, true)
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
		url, _ := pasteRouter.Get("authenticate").URL("id", id.String())
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
	url, _ := pasteRouter.Get(routeType).URL("id", p.ID.String())
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
		RenderError(GenericStringError("Cool it."), 420, w)
		return
	}

	id := PasteIDFromString(mux.Vars(r)["id"])
	password := r.FormValue("password")

	p, err := pasteStore.Get(id, nil)
	if p == nil {
		RenderError(err, http.StatusNotFound, w)
		return
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

	url, _ := pasteRouter.Get("show").URL("id", id.String())
	dest := url.String()
	if destCookie, err := r.Cookie("destination"); err != nil {
		dest = destCookie.Value
	}
	w.Header().Set("Location", dest)
	w.WriteHeader(http.StatusSeeOther)
}

func throttleAuthForRequest(r *http.Request) bool {
	ip := SourceIPForRequest(r)

	id := mux.Vars(r)["id"]

	tok := ip + "|" + id

	var at *int32
	v, ok := ephStore.Get(tok)
	if v != nil && ok {
		at = v.(*int32)
	} else {
		var n int32
		at = &n
		ephStore.Put(tok, at, 1*time.Minute)
	}

	*at++

	if *at >= 5 {
		// If they've tried and failed too much, renew the throttle
		// at five minutes, to make them cool off.

		ephStore.Put(tok, at, 5*time.Minute)
		return true
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
	tok := "P|H|" + p.ID.String()
	v, _ := ephStore.Get(tok)
	if hash, ok := v.(string); ok {
		ephStore.Delete(hash)
		ephStore.Delete(tok)
	}

	pasteExpirator.CancelObjectExpiration(p)

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
var pasteExpirator *gotimeout.Expirator
var sessionStore *sessions.FilesystemStore
var clientOnlySessionStore *sessions.CookieStore
var ephStore *gotimeout.Map
var pasteRouter *mux.Router

type args struct {
	root, addr string
	rebuild    bool
}

func (a *args) register() {
	flag.StringVar(&a.root, "root", "./", "path to generated file storage")
	flag.StringVar(&a.addr, "addr", "0.0.0.0:8080", "bind address and port")
	flag.BoolVar(&a.rebuild, "rebuild", false, "rebuild all templates for each request")
}

func (a *args) parse() {
	flag.Parse()
}

var arguments = &args{}

func init() {
	// N.B. this should not be necessary.
	gob.Register(map[PasteID][]byte(nil))

	arguments.register()
	arguments.parse()

	runtime.GOMAXPROCS(runtime.NumCPU())
	RegisterTemplateFunction("encryptionAllowed", func(ri *RenderContext) bool { return RequestIsHTTPS(ri.Request) })
	RegisterTemplateFunction("editAllowed", func(ri *RenderContext) bool { return isEditAllowed(ri.Obj.(*Paste), ri.Request) })
	RegisterTemplateFunction("render", renderPaste)
	RegisterTemplateFunction("pasteURL", pasteURL)
	RegisterTemplateFunction("pasteWillExpire", func(p *Paste) bool {
		return p.Expiration != "" && p.Expiration != "-1"
	})
	RegisterTemplateFunction("pasteBody", func(p *Paste) string {
		reader, _ := p.Reader()
		defer reader.Close()
		b := &bytes.Buffer{}
		io.Copy(b, reader)
		return b.String()
	})
	RegisterTemplateFunction("requestVariable", requestVariable)

	sesdir := filepath.Join(arguments.root, "sessions")
	os.Mkdir(sesdir, 0700)

	sessionKey, err := SlurpFile(filepath.Join(arguments.root, "session.key"))
	if err != nil {
		glog.Fatal("session.key not found. make one with seskey.go?")
	}
	sessionStore = sessions.NewFilesystemStore(sesdir, sessionKey)
	sessionStore.Options.Path = "/"
	sessionStore.Options.MaxAge = 86400 * 365

	clientOnlySessionEncryptionKey, err := SlurpFile(filepath.Join(arguments.root, "client_session_enc.key"))
	if err != nil {
		glog.Fatal("client_session_enc.key not found. make one with seskey.go?")
	}
	clientOnlySessionStore = sessions.NewCookieStore(sessionKey, clientOnlySessionEncryptionKey)
	if Env() != EnvironmentDevelopment {
		clientOnlySessionStore.Options.Secure = true
	}
	clientOnlySessionStore.Options.Path = "/"
	clientOnlySessionStore.Options.MaxAge = 0

	pastedir := filepath.Join(arguments.root, "pastes")
	os.Mkdir(pastedir, 0700)
	pasteStore = NewFilesystemPasteStore(pastedir)
	pasteStore.PasteDestroyCallback = PasteCallback(pasteDestroyCallback)

	pasteExpirator = gotimeout.NewExpirator(filepath.Join(arguments.root, "expiry.gob"), &ExpiringPasteStore{pasteStore})
	ephStore = gotimeout.NewMap()
}

func main() {
	ReloadAll()

	go func() {
		for {
			select {
			case err := <-pasteExpirator.ErrorChannel:
				glog.Error("Expirator Error: ", err.Error())
			}
		}
	}()

	router := mux.NewRouter()
	pasteRouter = router.PathPrefix("/paste").Subrouter()

	pasteRouter.Methods("GET").Path("/new").Handler(RedirectHandler("/"))
	pasteRouter.Methods("POST").Path("/new").Handler(http.HandlerFunc(pasteCreate))

	pasteRouter.Methods("GET").Path("/{id}").Handler(RequiredModelObjectHandler(lookupPasteWithRequest, RenderTemplateForModel("paste_show"))).Name("show")

	pasteRouter.Methods("GET").Path("/{id}/raw").Handler(RequiredModelObjectHandler(lookupPasteWithRequest, ModelRenderFunc(getPasteRawHandler))).Name("raw")

	pasteRouter.Methods("GET").Path("/{id}/download").Handler(RequiredModelObjectHandler(lookupPasteWithRequest, ModelRenderFunc(getPasteRawHandler))).Name("download")

	pasteRouter.Methods("GET").Path("/{id}/edit").Handler(RequiredModelObjectHandler(lookupPasteWithRequest, requiresEditPermission(RenderTemplateForModel("paste_edit")))).Name("edit")
	pasteRouter.Methods("POST").Path("/{id}/edit").Handler(RequiredModelObjectHandler(lookupPasteWithRequest, requiresEditPermission(pasteUpdate)))

	pasteRouter.Methods("GET").Path("/{id}/delete").Handler(RequiredModelObjectHandler(lookupPasteWithRequest, requiresEditPermission(RenderTemplateForModel("paste_delete_confirm")))).Name("delete")
	pasteRouter.Methods("POST").Path("/{id}/delete").Handler(RequiredModelObjectHandler(lookupPasteWithRequest, requiresEditPermission(pasteDelete)))

	httpsMatcher := func(r *http.Request, rm *mux.RouteMatch) bool {
		return RequestIsHTTPS(r)
	}
	pasteRouter.Methods("GET").
		MatcherFunc(httpsMatcher).
		Path("/{id}/authenticate").
		Handler(RenderTemplateHandler("paste_authenticate")).
		Name("authenticate")
	pasteRouter.Methods("POST").
		MatcherFunc(httpsMatcher).
		Path("/{id}/authenticate").
		Handler(http.HandlerFunc(authenticatePastePOSTHandler))

	nonHttpsMatcher := func(r *http.Request, rm *mux.RouteMatch) bool {
		return !RequestIsHTTPS(r)
	}
	pasteRouter.Methods("GET").
		MatcherFunc(nonHttpsMatcher).
		Path("/{id}/authenticate").
		Handler(RenderTemplateHandler("paste_authenticate_disallowed"))

	pasteRouter.Methods("GET").Path("/").Handler(RedirectHandler("/"))

	router.Path("/paste").Handler(RedirectHandler("/"))
	router.Path("/session").Handler(http.HandlerFunc(sessionHandler))
	router.Path("/session/raw").Handler(http.HandlerFunc(sessionHandler))
	router.Path("/about").Handler(RenderTemplateHandler("about"))
	router.Path("/").Handler(RenderTemplateHandler("index"))
	router.PathPrefix("/").Handler(http.FileServer(http.Dir("./public")))
	http.Handle("/", &fourOhFourConsumerHandler{router})

	var addr string = arguments.addr
	server := &http.Server{
		Addr: addr,
	}
	server.ListenAndServe()
}
