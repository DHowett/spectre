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

	"github.com/DHowett/ghostbin/lib/four"

	"github.com/DHowett/ghostbin/lib/accounts"
	"github.com/DHowett/ghostbin/lib/pastes"

	"github.com/DHowett/gotimeout"
	"github.com/golang/glog"
	"github.com/golang/groupcache/lru"
	"github.com/gorilla/mux"
	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
)

const PASTE_CACHE_MAX_ENTRIES int = 1000
const PASTE_MAXIMUM_LENGTH ByteSize = 1048576 // 1 MB
const MAX_EXPIRE_DURATION time.Duration = 15 * 24 * time.Hour

type PasteAccessDeniedError struct {
	action string
	ID     pastes.ID
}

func (e PasteAccessDeniedError) Error() string {
	return "You're not allowed to " + e.action + " paste " + e.ID.String()
}

// Make the various errors we can throw conform to HTTPError (here vs. the generic type file)
func (e PasteAccessDeniedError) StatusCode() int {
	return http.StatusForbidden
}

/*func (e PasteNotFoundError) StatusCode() int {
	return http.StatusNotFound
}

func (e PasteNotFoundError) ErrorTemplateName() string {
	return "paste_not_found"
} TODO(DH): Fix paste not found errors. */

type PasteTooLargeError ByteSize

func (e PasteTooLargeError) Error() string {
	return fmt.Sprintf("Your input (%v) exceeds the maximum paste length, which is %v.", ByteSize(e), PASTE_MAXIMUM_LENGTH)
}

func (e PasteTooLargeError) StatusCode() int {
	return http.StatusBadRequest
}

func getPasteJSONHandler(o Model, w http.ResponseWriter, r *http.Request) {
	p := o.(pastes.Paste)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	reader, _ := p.Reader()
	defer reader.Close()
	buf := &bytes.Buffer{}
	io.Copy(buf, reader)

	pasteMap := map[string]interface{}{
		"id":         p.GetID(),
		"language":   p.GetLanguageName(),
		"encrypted":  p.IsEncrypted(),
		"expiration": p.GetExpiration(),
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

	p := o.(pastes.Paste)
	ext := "txt"
	if mux.CurrentRoute(r).GetName() == "download" {
		lang := LanguageNamed(p.GetLanguageName())
		if lang != nil {
			if len(lang.Extensions) > 0 {
				ext = lang.Extensions[0]
			}
		}

		filename := p.GetID().String()
		if p.GetTitle() != "" {
			filename = p.GetTitle()
		}
		w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"."+ext+"\"")
		w.Header().Set("Content-Transfer-Encoding", "binary")
	}

	reader, _ := p.Reader()
	defer reader.Close()
	io.Copy(w, reader)
}

func pasteGrantHandler(o Model, w http.ResponseWriter, r *http.Request) {
	p := o.(pastes.Paste)

	grantKey := grantStore.NewGrant(p.GetID())

	acceptURL, _ := pasteRouter.Get("grant_accept").URL("grantkey", string(grantKey))

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.Encode(map[string]string{
		"acceptURL": BaseURLForRequest(r).ResolveReference(acceptURL).String(),
		"key":       string(grantKey),
		"id":        p.GetID().String(),
	})
}

func pasteUngrantHandler(o Model, w http.ResponseWriter, r *http.Request) {
	p := o.(pastes.Paste)
	GetPastePermissionScope(p.GetID(), r).Revoke(accounts.PastePermissionAll)
	SavePastePermissionScope(w, r)

	SetFlash(w, "success", fmt.Sprintf("Paste %v disavowed.", p.GetID()))
	w.Header().Set("Location", pasteURL("show", p.GetID()))
	w.WriteHeader(http.StatusSeeOther)
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

	GetPastePermissionScope(pID, r).Grant(accounts.PastePermissionEdit)
	SavePastePermissionScope(w, r)

	// delete(grants, grantKey)
	grantStore.Delete(grantKey)

	SetFlash(w, "success", fmt.Sprintf("You now have edit rights to Paste %v.", pID))
	w.Header().Set("Location", pasteURL("show", pID))
	w.WriteHeader(http.StatusSeeOther)
}

func isEditAllowed(p pastes.Paste, r *http.Request) bool {
	return GetPastePermissionScope(p.GetID(), r).Has(accounts.PastePermissionEdit)
}

func requiresEditPermission(fn ModelRenderFunc) ModelRenderFunc {
	return func(o Model, w http.ResponseWriter, r *http.Request) {
		defer errorRecoveryHandler(w)

		p := o.(pastes.Paste)
		accerr := PasteAccessDeniedError{"modify", p.GetID()}
		if !isEditAllowed(p, r) {
			panic(accerr)
		}
		fn(p, w, r)
	}
}

func requiresUserPermission(permission accounts.Permission, handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer errorRecoveryHandler(w)

		user := GetUser(r)
		if user != nil {
			if user.Permissions(accounts.PermissionClassUser).Has(permission) {
				handler.ServeHTTP(w, r)
				return
			}
		}

		panic(fmt.Errorf("You are not allowed to be here. >:|"))
	})
}

func pasteUpdate(o Model, w http.ResponseWriter, r *http.Request) {
	pasteUpdateCore(o, w, r, false)
}

func pasteUpdateCore(o Model, w http.ResponseWriter, r *http.Request, newPaste bool) {
	p := o.(pastes.Paste)
	body := r.FormValue("text")
	if len(strings.TrimSpace(body)) == 0 {
		w.Header().Set("Location", pasteURL("delete", p.GetID()))
		w.WriteHeader(http.StatusFound)
		return
	}

	pasteLen := ByteSize(len(body))
	if pasteLen > PASTE_MAXIMUM_LENGTH {
		panic(PasteTooLargeError(pasteLen))
	}

	if !newPaste {
		// If this is an update (instead of a new paste), blow away the hash.
		tok := "P|H|" + p.GetID().String()
		v, _ := ephStore.Get(tok)
		if hash, ok := v.(string); ok {
			ephStore.Delete(hash)
			ephStore.Delete(tok)
		}
	}

	var lang *Language = LanguageNamed(p.GetLanguageName())
	pw, _ := p.Writer()
	pw.Write([]byte(body))
	if r.FormValue("lang") != "" {
		lang = LanguageNamed(r.FormValue("lang"))
	}

	if lang != nil {
		p.SetLanguageName(lang.ID)
	}

	expireIn := r.FormValue("expire")
	ePid := ExpiringPasteID(p.GetID())
	if expireIn != "" && expireIn != "-1" {
		dur, _ := ParseDuration(expireIn)
		if dur > MAX_EXPIRE_DURATION {
			dur = MAX_EXPIRE_DURATION
		}
		pasteExpirator.ExpireObject(ePid, dur)
	} else {
		if expireIn == "-1" && pasteExpirator.ObjectHasExpiration(ePid) {
			pasteExpirator.CancelObjectExpiration(ePid)
		}
	}

	p.SetExpiration(expireIn)

	p.SetTitle(r.FormValue("title"))

	pw.Close() // Saves p

	w.Header().Set("Location", pasteURL("show", p.GetID()))
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

	var p pastes.Paste
	var err error

	if !encrypted {
		// We can only hash-dedup non-encrypted pastes.
		hasher := md5.New()
		io.WriteString(hasher, body)
		hashToken := "H|" + SourceIPForRequest(r) + "|" + base32Encoder.EncodeToString(hasher.Sum(nil))

		v, _ := ephStore.Get(hashToken)
		if hashedPaste, ok := v.(pastes.Paste); ok {
			pasteUpdateCore(hashedPaste, w, r, true)
			return
		}

		p, err = pasteStore.NewPaste()
		if err != nil {
			panic(err)
		}

		ephStore.Put(hashToken, p, 5*time.Minute)
		ephStore.Put("P|H|"+p.GetID().String(), hashToken, 5*time.Minute)
	} else {
		p, err = pasteStore.NewEncryptedPaste(CURRENT_ENCRYPTION_METHOD, []byte(password))
		if err != nil {
			panic(err)
		}

		cliSession, err := clientOnlySessionStore.Get(r, "c_session")
		if err != nil {
			glog.Errorln(err)
		}
		pasteKeys, ok := cliSession.Values["paste_passphrases"].(map[pastes.ID][]byte)
		if !ok {
			pasteKeys = map[pastes.ID][]byte{}
		}

		pasteKeys[p.GetID()] = []byte(password)
		cliSession.Values["paste_passphrases"] = pasteKeys
	}

	GetPastePermissionScope(p.GetID(), r).Grant(accounts.PastePermissionAll)
	SavePastePermissionScope(w, r)

	err = sessions.Save(r, w)
	if err != nil {
		glog.Errorln(err)
	}

	pasteUpdateCore(p, w, r, true)
}

func pasteDelete(o Model, w http.ResponseWriter, r *http.Request) {
	p := o.(pastes.Paste)

	oldId := p.GetID()
	p.Erase()

	GetPastePermissionScope(oldId, r).Revoke(accounts.PastePermissionAll)
	SavePastePermissionScope(w, r)

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
	id := pastes.IDFromString(mux.Vars(r)["id"])
	var passphrase []byte

	cliSession, err := clientOnlySessionStore.Get(r, "c_session")
	if err != nil {
		glog.Errorln(err)
	}
	if pasteKeys, ok := cliSession.Values["paste_passphrases"].(map[pastes.ID][]byte); ok {
		if _key, ok := pasteKeys[id]; ok {
			passphrase = _key
		}
	}

	enc := false
	p, err := pasteStore.Get(id, passphrase)
	if _, ok := err.(pastes.PasteEncryptedError); ok {
		enc = true
	}

	if _, ok := err.(pastes.PasteInvalidKeyError); ok {
		enc = true
	}

	if enc {
		url, _ := pasteRouter.Get("authenticate").URL("id", id.String())
		if _, ok := err.(pastes.PasteInvalidKeyError); ok {
			url.RawQuery = "i=1"
		}

		return nil, DeferLookupError{
			Interstitial: url,
		}
	}
	return p, err
}

func pasteURL(routeType string, p pastes.ID) string {
	url, _ := pasteRouter.Get(routeType).URL("id", p.String())
	return url.String()
}

func sessionHandler(w http.ResponseWriter, r *http.Request) {
	var ids []pastes.ID

	// Assumption: due to the migration handler wrapper, a logged-in session will
	// never have v3 perms and user perms.
	user := GetUser(r)
	if user != nil {
		uPastes, err := user.GetPastes()
		if err == nil {
			ids = uPastes
		}
	} else {

		// Failed lookup is non-fatal here.
		cookieSession, _ := sessionStore.Get(r, "session")
		v3EntriesI, _ := cookieSession.Values["v3permissions"]
		v3Perms, _ := v3EntriesI.(map[pastes.ID]accounts.Permission)

		ids = make([]pastes.ID, len(v3Perms))
		n := 0
		for pid, _ := range v3Perms {
			ids[n] = pid
			n++
		}
	}

	if strings.HasSuffix(r.URL.Path, "/raw") {
		stringIDs := make([]string, len(ids))
		for i, v := range ids {
			stringIDs[i] = v.String()
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(strings.Join(stringIDs, " ")))
		return
	}

	sessionPastes, err := pasteStore.GetAll(ids)
	if err != nil {
		panic(err)
	}
	RenderPage(w, r, "session", sessionPastes)
}

func authenticatePastePOSTHandler(w http.ResponseWriter, r *http.Request) {
	if throttleAuthForRequest(r) {
		RenderError(fmt.Errorf("Cool it."), 420, w)
		return
	}

	id := pastes.IDFromString(mux.Vars(r)["id"])
	passphrase := []byte(r.FormValue("password"))

	cliSession, _ := clientOnlySessionStore.Get(r, "c_session")
	pasteKeys, ok := cliSession.Values["paste_passphrases"].(map[pastes.ID][]byte)
	if !ok {
		pasteKeys = map[pastes.ID][]byte{}
	}

	pasteKeys[id] = passphrase
	cliSession.Values["paste_passphrases"] = pasteKeys
	sessions.Save(r, w)

	dest := pasteURL("show", id)
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

func renderPaste(p pastes.Paste) template.HTML {
	renderCache.mu.RLock()
	var cached *RenderedPaste
	var cval interface{}
	var ok bool
	if renderCache.c != nil {
		if cval, ok = renderCache.c.Get(p.GetID()); ok {
			cached = cval.(*RenderedPaste)
		}
	}
	renderCache.mu.RUnlock()

	if !ok || cached.renderTime.Before(p.GetModificationTime()) {
		defer renderCache.mu.Unlock()
		renderCache.mu.Lock()
		out, err := FormatPaste(p)

		if err != nil {
			glog.Errorf("Render for %s failed: (%s) output: %s", p.GetID(), err.Error(), out)
			return template.HTML("There was an error rendering this paste.")
		}

		rendered := template.HTML(out)
		if !p.IsEncrypted() {
			if renderCache.c == nil {
				renderCache.c = &lru.Cache{
					MaxEntries: PASTE_CACHE_MAX_ENTRIES,
					OnEvicted: func(key lru.Key, value interface{}) {
						glog.Info("RENDER CACHE: Evicted ", key)
					},
				}
			}
			renderCache.c.Add(p.GetID(), &RenderedPaste{body: rendered, renderTime: time.Now()})
			glog.Info("RENDER CACHE: Cached ", p.GetID())
		}

		return rendered
	} else {
		return cached.body
	}
}

func pasteDestroyCallback(p pastes.Paste) {
	tok := "P|H|" + p.GetID().String()
	v, _ := ephStore.Get(tok)
	if hash, ok := v.(string); ok {
		ephStore.Delete(hash)
		ephStore.Delete(tok)
	}

	pasteExpirator.CancelObjectExpiration(ExpiringPasteID(p.GetID()))

	defer renderCache.mu.Unlock()
	renderCache.mu.Lock()
	if renderCache.c == nil {
		return
	}

	glog.Info("RENDER CACHE: Removing ", p.GetID(), " due to destruction.")
	// Clear the cached render when a paste is destroyed
	renderCache.c.Remove(p.GetID())

	reportStore.Delete(p.GetID())
}

var pasteStore pastes.PasteStore
var pasteExpirator *gotimeout.Expirator
var sessionStore *sessions.FilesystemStore
var clientOnlySessionStore *sessions.CookieStore
var clientLongtermSessionStore *sessions.CookieStore
var ephStore *gotimeout.Map
var userStore accounts.Store
var pasteRouter *mux.Router
var router *mux.Router

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
	gob.Register(map[pastes.ID][]byte(nil))
	gob.Register(map[pastes.ID]accounts.Permission{})

	arguments.register()
	arguments.parse()

	runtime.GOMAXPROCS(runtime.NumCPU())
	RegisterTemplateFunction("encryptionAllowed", func(ri *RenderContext) bool { return Env() == EnvironmentDevelopment || RequestIsHTTPS(ri.Request) })
	RegisterTemplateFunction("editAllowed", func(ri *RenderContext) bool { return isEditAllowed(ri.Obj.(pastes.Paste), ri.Request) })
	RegisterTemplateFunction("render", renderPaste)
	RegisterTemplateFunction("pasteURL", func(e string, p pastes.Paste) string {
		return pasteURL(e, p.GetID())
	})
	RegisterTemplateFunction("pasteWillExpire", func(p pastes.Paste) bool {
		return p.GetExpiration() != "" && p.GetExpiration() != "-1"
	})
	RegisterTemplateFunction("pasteFromID", func(id pastes.ID) pastes.Paste {
		p, err := pasteStore.Get(id, nil)
		if err != nil {
			return nil
		}
		return p
	})
	RegisterTemplateFunction("truncatedPasteBody", func(p pastes.Paste, lines int) string {
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
	RegisterTemplateFunction("pasteBody", func(p pastes.Paste) string {
		reader, _ := p.Reader()
		defer reader.Close()
		b := &bytes.Buffer{}
		io.Copy(b, reader)
		return b.String()
	})
	RegisterTemplateFunction("requestVariable", requestVariable)
	RegisterTemplateFunction("languageNamed", func(name string) *Language {
		return LanguageNamed(name)
	})

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
	//pasteStore.PasteDestroyCallback = PasteCallback(pasteDestroyCallback) // TODO(DH): Find a good model for callbacks.

	pasteExpirator = gotimeout.NewExpirator(filepath.Join(arguments.root, "expiry.gob"), &ExpiringPasteStore{pasteStore})
	ephStore = gotimeout.NewMap()

	accountPath := filepath.Join(arguments.root, "accounts")
	os.Mkdir(accountPath, 0700)
	userStore = nil
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

	router.Path("/admin").Handler(requiresUserPermission(accounts.UserPermissionAdmin, RenderPageHandler("admin_home")))

	router.Path("/admin/reports").Handler(requiresUserPermission(accounts.UserPermissionAdmin, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		RenderPage(w, r, "admin_reports", reportStore.Reports)
	})))

	router.Methods("POST").Path("/admin/promote").Handler(requiresUserPermission(accounts.UserPermissionAdmin, http.HandlerFunc(adminPromoteHandler)))

	router.Methods("POST").
		Path("/admin/paste/{id}/delete").
		Handler(requiresUserPermission(accounts.UserPermissionAdmin, RequiredModelObjectHandler(lookupPasteWithRequest, pasteDelete))).
		Name("admindelete")

	router.Methods("POST").
		Path("/admin/paste/{id}/clear_report").
		Handler(requiresUserPermission(accounts.UserPermissionAdmin, http.HandlerFunc(reportClear))).
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

	launchTime := time.Now()
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

	router.Methods("GET").
		Path("/partial/{id}").
		Handler(http.HandlerFunc(partialGetHandler))

	router.Methods("POST").Path("/auth/login").Handler(http.HandlerFunc(authLoginPostHandler))
	router.Methods("POST").Path("/auth/logout").Handler(http.HandlerFunc(authLogoutPostHandler))
	router.Methods("GET").Path("/auth/token").Handler(http.HandlerFunc(authTokenHandler))
	router.Methods("GET").Path("/auth/token/{token}").Handler(http.HandlerFunc(authTokenPageHandler)).Name("auth_token_login")

	router.Path("/").Handler(RenderPageHandler("index"))
	router.Walk(func(route *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
		route.Handler(permissionMigrationWrapperHandler{route.GetHandler()})
		return nil
	})
	router.PathPrefix("/").Handler(http.FileServer(http.Dir("public")))
	http.Handle("/", four.WrapHandler(router, RenderPageHandler("404")))

	var addr string = arguments.addr
	server := &http.Server{
		Addr: addr,
	}
	server.ListenAndServe()
}
