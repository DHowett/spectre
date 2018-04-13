package main

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"encoding/gob"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/DHowett/ghostbin/account"
	"github.com/DHowett/gotimeout"
	"github.com/golang/glog"
	"github.com/golang/groupcache/lru"
	"github.com/gorilla/mux"
	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
)

var VERSION string = "<local build>"

const PASTE_CACHE_MAX_ENTRIES int = 1000
const PASTE_MAXIMUM_LENGTH ByteSize = 1048576 // 1 MB
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

func (e PasteNotFoundError) ErrorTemplateName() string {
	return "paste_not_found"
}

type PasteTooLargeError ByteSize

func (e PasteTooLargeError) Error() string {
	return fmt.Sprintf("Your input (%v) exceeds the maximum paste length, which is %v.", ByteSize(e), PASTE_MAXIMUM_LENGTH)
}

func (e PasteTooLargeError) StatusCode() int {
	return http.StatusBadRequest
}

func getPasteJSONHandler(o Model, w http.ResponseWriter, r *http.Request) {
	p := o.(*Paste)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	reader, _ := p.Reader()
	defer reader.Close()
	buf := &bytes.Buffer{}
	io.Copy(buf, reader)

	pasteMap := map[string]interface{}{
		"id":         p.ID,
		"language":   p.Language,
		"encrypted":  p.Encrypted,
		"expiration": p.Expiration,
		"body":       string(buf.Bytes()),
	}

	json, _ := json.Marshal(pasteMap)
	w.Write(json)
}

func getPasteRawHandler(o Model, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "null")
	w.Header().Set("Vary", "Origin")

	w.Header().Set("Content-Security-Policy", "default-src 'none'")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-XSS-Protection", "1; mode=block")

	p := o.(*Paste)
	ext := "txt"
	if mux.CurrentRoute(r).GetName() == "download" {
		lang := p.Language
		if lang != nil {
			if len(lang.Extensions) > 0 {
				ext = lang.Extensions[0]
			}
		}

		filename := p.ID.String()
		if p.Title != "" {
			filename = p.Title
		}
		w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"."+ext+"\"")
		w.Header().Set("Content-Transfer-Encoding", "binary")
	}

	reader, _ := p.Reader()
	defer reader.Close()
	io.Copy(w, reader)
}

func pasteGrantHandler(o Model, w http.ResponseWriter, r *http.Request) {
	p := o.(*Paste)

	grantKey := grantStore.NewGrant(p.ID)

	acceptURL, _ := pasteRouter.Get("grant_accept").URL("grantkey", string(grantKey))

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.Encode(map[string]string{
		"acceptURL": BaseURLForRequest(r).ResolveReference(acceptURL).String(),
		"key":       string(grantKey),
		"id":        p.ID.String(),
	})

	healthServer.IncrementMetric("grants.generated")
}

func pasteUngrantHandler(o Model, w http.ResponseWriter, r *http.Request) {
	p := o.(*Paste)
	perms := GetPastePermissions(r)
	perms.Delete(p.ID)
	perms.Save(w, r)

	SetFlash(w, "success", fmt.Sprintf("Paste %v disavowed.", p.ID))
	w.Header().Set("Location", pasteURL("show", &Paste{ID: p.ID}))
	w.WriteHeader(http.StatusSeeOther)

	healthServer.IncrementMetric("grants.disavowed")
}

func grantAcceptHandler(w http.ResponseWriter, r *http.Request) {
	v := mux.Vars(r)
	grantKey := GrantID(v["grantkey"])
	pID, ok := grantStore.Get(grantKey)
	if !ok {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Hey."))
		return
	}

	perms := GetPastePermissions(r)
	perms.Put(pID, PastePermission{"edit": true})
	perms.Save(w, r)

	// delete(grants, grantKey)
	grantStore.Delete(grantKey)

	SetFlash(w, "success", fmt.Sprintf("You now have edit rights to Paste %v.", pID))
	w.Header().Set("Location", pasteURL("show", &Paste{ID: pID}))
	w.WriteHeader(http.StatusSeeOther)

	healthServer.IncrementMetric("grants.accepted")
}

func isEditAllowed(p *Paste, r *http.Request) bool {
	perms := GetPastePermissions(r)
	perm, ok := perms.Get(p.ID)
	if !ok {
		return false
	}

	return perm["edit"]
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

func requiresUserPermission(permission string, handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer errorRecoveryHandler(w)

		user := GetUser(r)
		if user != nil {
			if o, ok := user.Values["user.permissions"]; ok {
				if perms, ok := o.(PastePermission); ok {
					if perms[permission] {
						handler.ServeHTTP(w, r)
						return
					}
				}
			}
		}

		healthServer.IncrementMetric("permission." + permission + ".failed")
		panic(fmt.Errorf("You are not allowed to be here. >:|"))
	})
}

func pasteUpdate(o Model, w http.ResponseWriter, r *http.Request) {
	pasteUpdateCore(o, w, r, false)
	healthServer.IncrementMetric("paste.updated")
}

func pasteUpdateCore(o Model, w http.ResponseWriter, r *http.Request, newPaste bool) {
	p := o.(*Paste)
	body := r.FormValue("text")
	if len(strings.TrimSpace(body)) == 0 {
		w.Header().Set("Location", pasteURL("delete", p))
		w.WriteHeader(http.StatusFound)
		return
	}

	pasteLen := ByteSize(len(body))
	if pasteLen > PASTE_MAXIMUM_LENGTH {
		panic(PasteTooLargeError(pasteLen))
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
		p.Language = LanguageNamed(r.FormValue("lang"))
	}

	if p.Language == nil {
		p.Language = unknownLanguage
	}

	expireIn := r.FormValue("expire")
	if expireIn != "" && expireIn != "-1" {
		dur, _ := ParseDuration(expireIn)
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

	p.Title = r.FormValue("title")

	pw.Close() // Saves p

	w.Header().Set("Location", pasteURL("show", p))
	w.WriteHeader(http.StatusSeeOther)
}

func pasteCreate(w http.ResponseWriter, r *http.Request) {
	body := r.FormValue("text")
	if len(strings.TrimSpace(body)) == 0 {
		// 400 here, 200 above (one is displayed to the user, one could be an API response.)
		RenderError(fmt.Errorf("Hey, put some text in that paste."), 400, w)
		return
	}

	pasteLen := ByteSize(len(body))
	if pasteLen > PASTE_MAXIMUM_LENGTH {
		err := PasteTooLargeError(pasteLen)
		RenderError(err, err.StatusCode(), w)
		return
	}

	password := r.FormValue("password")
	encrypted := password != ""

	if encrypted && (Env() != EnvironmentDevelopment && !RequestIsHTTPS(r)) {
		RenderError(fmt.Errorf("I refuse to accept passwords over HTTP."), 400, w)
		return
	}

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

	perms := GetPastePermissions(r)
	perms.Put(p.ID, PastePermission{"edit": true, "grant": true})
	perms.Save(w, r)

	if key != nil {
		cliSession, err := clientOnlySessionStore.Get(r, "c_session")
		if err != nil {
			glog.Errorln(err)
		}
		pasteKeys, ok := cliSession.Values["paste_keys"].(map[PasteID][]byte)
		if !ok {
			pasteKeys = map[PasteID][]byte{}
		}

		pasteKeys[p.ID] = key
		cliSession.Values["paste_keys"] = pasteKeys
	}

	err = sessions.Save(r, w)
	if err != nil {
		glog.Errorln(err)
	}

	pasteUpdateCore(p, w, r, true)

	healthServer.IncrementMetric("paste.created")
}

func pasteDelete(o Model, w http.ResponseWriter, r *http.Request) {
	p := o.(*Paste)

	oldId := p.ID
	p.Destroy()

	perms := GetPastePermissions(r)
	perms.Delete(oldId)
	perms.Save(w, r)

	SetFlash(w, "success", fmt.Sprintf("Paste %v deleted.", oldId))

	redir := r.FormValue("redir")
	if redir == "reports" {
		w.Header().Set("Location", "/admin/reports")
	} else {
		w.Header().Set("Location", "/")
	}

	w.WriteHeader(http.StatusFound)
}

func lookupPasteWithRequest(r *http.Request) (Model, error) {
	id := PasteIDFromString(mux.Vars(r)["id"])
	var key []byte

	cliSession, err := clientOnlySessionStore.Get(r, "c_session")
	if err != nil {
		glog.Errorln(err)
	}
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
	perms := GetPastePermissions(r)
	pastes = make([]*Paste, len(perms.Entries))
	ids = make([]string, len(perms.Entries))
	n := 0
	for k, _ := range perms.Entries {
		if obj, _ := pasteStore.Get(k, nil); obj != nil {
			pastes[n] = obj
			ids[n] = obj.ID.String()
			n++
		}
	}
	pastes = pastes[:n]
	ids = ids[:n]

	if strings.HasSuffix(r.URL.Path, "/raw") {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(strings.Join(ids, " ")))
	} else {
		RenderPage(w, r, "session", pastes)
	}
}

func authenticatePastePOSTHandler(w http.ResponseWriter, r *http.Request) {
	if throttleAuthForRequest(r) {
		RenderError(fmt.Errorf("Cool it."), 420, w)
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
		if err != nil {
			glog.Errorln(err)
		}
	}

	url, _ := pasteRouter.Get("show").URL("id", id.String())
	dest := url.String()
	if destCookie, err := r.Cookie("destination"); err != nil {
		dest = destCookie.Value
	}
	w.Header().Set("Location", dest)
	w.WriteHeader(http.StatusSeeOther)
	healthServer.IncrementMetric("paste.auth.successful")
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

func partialGetHandler(w http.ResponseWriter, r *http.Request) {
	name := mux.Vars(r)["id"]
	RenderPartial(w, r, name, nil)
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

	reportStore.Delete(p.ID)

	healthServer.IncrementMetric("paste.deleted")
}

var pasteStore *FilesystemPasteStore
var pasteExpirator *gotimeout.Expirator
var sessionStore *sessions.FilesystemStore
var clientOnlySessionStore *sessions.CookieStore
var clientLongtermSessionStore *sessions.CookieStore
var ephStore *gotimeout.Map
var userStore account.AccountStore
var pasteRouter *mux.Router
var router *mux.Router
var healthServer *HealthServer

type args struct {
	root, addr string
	rebuild    bool

	registrationOnce sync.Once
	parseOnce        sync.Once
}

func (a *args) register() {
	a.registrationOnce.Do(func() {
		flag.StringVar(&a.root, "root", "./", "path to generated file storage")
		flag.StringVar(&a.addr, "addr", "0.0.0.0:8080", "bind address and port")
		flag.BoolVar(&a.rebuild, "rebuild", false, "rebuild all templates for each request")
	})
}

func (a *args) parse() {
	a.parseOnce.Do(func() {
		flag.Parse()
	})
}

var arguments = &args{}

func init() {
	// N.B. this should not be necessary.
	gob.Register(map[PasteID][]byte(nil))
	gob.Register(&PastePermissionSet{})
	gob.Register(PastePermission{})

	arguments.register()
	arguments.parse()

	runtime.GOMAXPROCS(runtime.NumCPU())
	RegisterTemplateFunction("encryptionAllowed", func(ri *RenderContext) bool { return Env() == EnvironmentDevelopment || RequestIsHTTPS(ri.Request) })
	RegisterTemplateFunction("editAllowed", func(ri *RenderContext) bool { return isEditAllowed(ri.Obj.(*Paste), ri.Request) })
	RegisterTemplateFunction("render", renderPaste)
	RegisterTemplateFunction("pasteURL", pasteURL)
	RegisterTemplateFunction("pasteWillExpire", func(p *Paste) bool {
		return p.Expiration != "" && p.Expiration != "-1"
	})
	RegisterTemplateFunction("pasteFromID", func(id PasteID) *Paste {
		p, err := pasteStore.Get(id, nil)
		if err != nil {
			return nil
		}
		return p
	})
	RegisterTemplateFunction("truncatedPasteBody", func(p *Paste, lines int) string {
		reader, _ := p.Reader()
		defer reader.Close()
		bufReader := bufio.NewReader(reader)
		s := ""
		n := 0
		for n < lines {
			line, err := bufReader.ReadString('\n')
			if err != io.EOF && err != nil {
				break
			}
			s = s + line
			if err == io.EOF {
				break
			}
			n++
		}
		if n == lines {
			s += "..."
		}
		return s
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

	sessionKeyFile := filepath.Join(arguments.root, "session.key")
	sessionKey, err := SlurpFile(sessionKeyFile)
	if err != nil {
		sessionKey = securecookie.GenerateRandomKey(32)
		err = ioutil.WriteFile(sessionKeyFile, sessionKey, 0600)
		if err != nil {
			glog.Fatal("session.key not found, and an attempt to create one failed: ", err)
		}
	}
	sessionStore = sessions.NewFilesystemStore(sesdir, sessionKey)
	sessionStore.Options.Path = "/"
	sessionStore.Options.MaxAge = 86400 * 365

	clientKeyFile := filepath.Join(arguments.root, "client_session_enc.key")
	clientOnlySessionEncryptionKey, err := SlurpFile(clientKeyFile)
	if err != nil {
		clientOnlySessionEncryptionKey = securecookie.GenerateRandomKey(32)
		err = ioutil.WriteFile(clientKeyFile, clientOnlySessionEncryptionKey, 0600)
		if err != nil {
			glog.Fatal("client_session_enc.key not found, and an attempt to create one failed: ", err)
		}
	}
	clientOnlySessionStore = sessions.NewCookieStore(sessionKey, clientOnlySessionEncryptionKey)
	if Env() != EnvironmentDevelopment {
		clientOnlySessionStore.Options.Secure = true
	}
	clientOnlySessionStore.Options.Path = "/"
	clientOnlySessionStore.Options.MaxAge = 0

	clientLongtermSessionStore = sessions.NewCookieStore(sessionKey, clientOnlySessionEncryptionKey)
	if Env() != EnvironmentDevelopment {
		clientLongtermSessionStore.Options.Secure = true
	}
	clientLongtermSessionStore.Options.Path = "/"
	clientLongtermSessionStore.Options.MaxAge = 86400 * 365

	pastedir := filepath.Join(arguments.root, "pastes")
	os.Mkdir(pastedir, 0700)
	pasteStore = NewFilesystemPasteStore(pastedir)
	pasteStore.PasteDestroyCallback = PasteCallback(pasteDestroyCallback)

	pasteExpirator = gotimeout.NewExpirator(filepath.Join(arguments.root, "expiry.gob"), &ExpiringPasteStore{pasteStore})
	ephStore = gotimeout.NewMap()

	accountPath := filepath.Join(arguments.root, "accounts")
	os.Mkdir(accountPath, 0700)
	userStore = &PromoteFirstUserToAdminStore{
		Path: accountPath,
		AccountStore: &CachingUserStore{
			AccountStore: &ManglingUserStore{
				account.NewFilesystemStore(accountPath, &AuthChallengeProvider{}),
			},
		},
	}
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

	launchTime := time.Now()
	healthServer = &HealthServer{}

	healthServer.SetMetric("version", VERSION)

	healthServer.RegisterComputedMetric("goroutines", func() interface{} {
		return runtime.NumGoroutine()
	})
	healthServer.RegisterComputedMetric("memory.alloc", func() interface{} {
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		return ms.Alloc
	})
	healthServer.RegisterComputedMetric("paste.expiring", func() interface{} {
		return pasteExpirator.Len()
	})
	healthServer.RegisterComputedMetric("paste.cache", func() interface{} {
		if renderCache.c != nil {
			return renderCache.c.Len()
		} else {
			return 0
		}
	})
	healthServer.RegisterComputedMetric("uptime", func() interface{} {
		return int(time.Now().Sub(launchTime) / time.Second)
	})

	router = mux.NewRouter()
	pasteRouter = router.PathPrefix("/paste").Subrouter()

	pasteRouter.Methods("GET").
		Path("/new").
		Handler(RedirectHandler("/"))

	pasteRouter.Methods("POST").
		Path("/new").
		Handler(http.HandlerFunc(pasteCreate))

	pasteRouter.Methods("GET").
		Path("/{id}.json").
		Handler(RequiredModelObjectHandler(lookupPasteWithRequest, ModelRenderFunc(getPasteJSONHandler))).
		Name("show")

	pasteRouter.Methods("GET").
		Path("/{id}").
		Handler(RequiredModelObjectHandler(lookupPasteWithRequest, RenderPageForModel("paste_show"))).
		Name("show")

	pasteRouter.Methods("POST").
		Path("/{id}/grant/new").
		Handler(RequiredModelObjectHandler(lookupPasteWithRequest, requiresEditPermission(ModelRenderFunc(pasteGrantHandler)))).
		Name("grant")
	pasteRouter.Methods("GET").
		Path("/grant/{grantkey}/accept").
		Handler(http.HandlerFunc(grantAcceptHandler)).
		Name("grant_accept")
	pasteRouter.Methods("GET").
		Path("/{id}/disavow").
		Handler(RequiredModelObjectHandler(lookupPasteWithRequest, requiresEditPermission(ModelRenderFunc(pasteUngrantHandler))))

	pasteRouter.Methods("GET").
		Path("/{id}/raw").
		Handler(RequiredModelObjectHandler(lookupPasteWithRequest, ModelRenderFunc(getPasteRawHandler))).
		Name("raw")
	pasteRouter.Methods("GET").
		Path("/{id}/download").
		Handler(RequiredModelObjectHandler(lookupPasteWithRequest, ModelRenderFunc(getPasteRawHandler))).
		Name("download")

	pasteRouter.Methods("GET").
		Path("/{id}/edit").
		Handler(RequiredModelObjectHandler(lookupPasteWithRequest, requiresEditPermission(RenderPageForModel("paste_edit")))).
		Name("edit")
	pasteRouter.Methods("POST").
		Path("/{id}/edit").
		Handler(RequiredModelObjectHandler(lookupPasteWithRequest, requiresEditPermission(pasteUpdate)))

	pasteRouter.Methods("GET").
		Path("/{id}/delete").
		Handler(RequiredModelObjectHandler(lookupPasteWithRequest, requiresEditPermission(RenderPageForModel("paste_delete_confirm")))).
		Name("delete")
	pasteRouter.Methods("POST").
		Path("/{id}/delete").
		Handler(RequiredModelObjectHandler(lookupPasteWithRequest, requiresEditPermission(pasteDelete)))

	pasteRouter.Methods("POST").
		Path("/{id}/report").
		Handler(RequiredModelObjectHandler(lookupPasteWithRequest, reportPaste)).
		Name("report")

	pasteRouter.Methods("GET").
		MatcherFunc(HTTPSMuxMatcher).
		Path("/{id}/authenticate").
		Handler(RenderPageHandler("paste_authenticate")).
		Name("authenticate")
	pasteRouter.Methods("POST").
		MatcherFunc(HTTPSMuxMatcher).
		Path("/{id}/authenticate").
		Handler(http.HandlerFunc(authenticatePastePOSTHandler))

	pasteRouter.Methods("GET").
		MatcherFunc(NonHTTPSMuxMatcher).
		Path("/{id}/authenticate").
		Handler(RenderPageHandler("paste_authenticate_disallowed"))

	router.Path("/admin").Handler(requiresUserPermission("admin", RenderPageHandler("admin_home")))

	router.Path("/admin/reports").Handler(requiresUserPermission("admin", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		RenderPage(w, r, "admin_reports", reportStore.Reports)
	})))

	router.Methods("POST").Path("/admin/promote").Handler(requiresUserPermission("admin", http.HandlerFunc(adminPromoteHandler)))

	router.Methods("POST").
		Path("/admin/paste/{id}/delete").
		Handler(requiresUserPermission("admin", RequiredModelObjectHandler(lookupPasteWithRequest, pasteDelete))).
		Name("admindelete")

	router.Methods("POST").
		Path("/admin/paste/{id}/clear_report").
		Handler(requiresUserPermission("admin", http.HandlerFunc(reportClear))).
		Name("reportclear")

	pasteRouter.Methods("GET").Path("/").Handler(RedirectHandler("/"))

	router.Path("/paste").Handler(RedirectHandler("/"))
	router.Path("/session").Handler(http.HandlerFunc(sessionHandler))
	router.Path("/session/raw").Handler(http.HandlerFunc(sessionHandler))
	router.Path("/about").Handler(RenderPageHandler("about"))
	router.Methods("GET", "HEAD").Path("/languages.json").Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		http.ServeContent(w, r, "languages.json", languageConfig.modtime, languageConfig.languageJSONReader)
	}))

	router.Methods("GET").Path("/stats").Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stats := make(map[string]string)
		ms := &runtime.MemStats{}
		runtime.ReadMemStats(ms)
		stats["mem_alloc"] = fmt.Sprintf("%v", ByteSize(ms.Alloc))
		if renderCache.c == nil {
			stats["cached"] = "(no cache)"
		} else {
			stats["cached"] = fmt.Sprintf("%d", renderCache.c.Len())
		}
		dur := time.Now().Sub(launchTime)
		dur = dur - (dur % time.Second)
		stats["uptime"] = fmt.Sprintf("%v", dur)
		stats["expiring"] = fmt.Sprintf("%d", pasteExpirator.Len())
		RenderPage(w, r, "stats", stats)
	}))
	router.Methods("GET").Path("/stats.json").Handler(healthServer)

	router.Methods("GET").
		Path("/partial/{id}").
		Handler(http.HandlerFunc(partialGetHandler))

	router.Methods("POST").Path("/auth/login").Handler(http.HandlerFunc(authLoginPostHandler))
	router.Methods("POST").Path("/auth/logout").Handler(http.HandlerFunc(authLogoutPostHandler))
	router.Methods("GET").Path("/auth/token").Handler(http.HandlerFunc(authTokenHandler))
	router.Methods("GET").Path("/auth/token/{token}").Handler(http.HandlerFunc(authTokenPageHandler)).Name("auth_token_login")

	router.Path("/").Handler(RenderPageHandler("index"))
	router.PathPrefix("/").Handler(http.FileServer(http.Dir("public")))
	http.Handle("/", &fourOhFourConsumerHandler{userLookupWrapper{router}})

	var addr string = arguments.addr
	server := &http.Server{
		Addr: addr,
	}
	server.ListenAndServe()
}
