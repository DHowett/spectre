package main

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/DHowett/ghostbin/lib/accounts"
	"github.com/DHowett/ghostbin/lib/pastes"
	"github.com/golang/glog"
	"github.com/golang/groupcache/lru"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
)

// paste http bindings
const PASTE_CACHE_MAX_ENTRIES int = 1000
const PASTE_MAXIMUM_LENGTH ByteSize = 1048576 // 1 MB
const MAX_EXPIRE_DURATION time.Duration = 15 * 24 * time.Hour

type PastePathHandler struct {
	Router     *mux.Router
	PasteStore pastes.PasteStore
}

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
}A*/

type PasteTooLargeError ByteSize

func (e PasteTooLargeError) Error() string {
	return fmt.Sprintf("Your input (%v) exceeds the maximum paste length, which is %v.", ByteSize(e), PASTE_MAXIMUM_LENGTH)
}

func (e PasteTooLargeError) StatusCode() int {
	return http.StatusBadRequest
}

func (pph *PastePathHandler) getPasteJSONHandler(o Model, w http.ResponseWriter, r *http.Request) {
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

func (pph *PastePathHandler) getPasteRawHandler(o Model, w http.ResponseWriter, r *http.Request) {
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

func (pph *PastePathHandler) pasteGrantHandler(o Model, w http.ResponseWriter, r *http.Request) {
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

func (pph *PastePathHandler) pasteUngrantHandler(o Model, w http.ResponseWriter, r *http.Request) {
	p := o.(pastes.Paste)
	GetPastePermissionScope(p.GetID(), r).Revoke(accounts.PastePermissionAll)
	SavePastePermissionScope(w, r)

	SetFlash(w, "success", fmt.Sprintf("Paste %v disavowed.", p.GetID()))
	w.Header().Set("Location", pasteURL("show", p.GetID()))
	w.WriteHeader(http.StatusSeeOther)
}

func (pph *PastePathHandler) grantAcceptHandler(w http.ResponseWriter, r *http.Request) {
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

// TODO(DH) MOVE
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

func (pph *PastePathHandler) pasteUpdate(o Model, w http.ResponseWriter, r *http.Request) {
	pph.pasteUpdateCore(o, w, r, false)
}

func (pph *PastePathHandler) pasteUpdateCore(o Model, w http.ResponseWriter, r *http.Request, newPaste bool) {
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

func (pph *PastePathHandler) pasteCreate(w http.ResponseWriter, r *http.Request) {
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
			pph.pasteUpdateCore(hashedPaste, w, r, true)
			// TODO(DH) EARLY RETURN
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

	pph.pasteUpdateCore(p, w, r, true)
}

func (pph *PastePathHandler) pasteDelete(o Model, w http.ResponseWriter, r *http.Request) {
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

func (pph *PastePathHandler) lookupPasteWithRequest(r *http.Request) (Model, error) {
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

func (pph *PastePathHandler) authenticatePastePOSTHandler(w http.ResponseWriter, r *http.Request) {
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

// TODO(DH) MOVE
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

type renderedPaste struct {
	body       template.HTML
	renderTime time.Time
}

var renderCache struct {
	mu sync.RWMutex
	c  *lru.Cache
}

// TODO(DH) MOVE
func renderPaste(p pastes.Paste) template.HTML {
	renderCache.mu.RLock()
	var cached *renderedPaste
	var cval interface{}
	var ok bool
	if renderCache.c != nil {
		if cval, ok = renderCache.c.Get(p.GetID()); ok {
			cached = cval.(*renderedPaste)
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
			renderCache.c.Add(p.GetID(), &renderedPaste{body: rendered, renderTime: time.Now()})
			glog.Info("RENDER CACHE: Cached ", p.GetID())
		}

		return rendered
	} else {
		return cached.body
	}
}

func (pph *PastePathHandler) InitRoutes() {
	pph.Router.Methods("GET").
		Path("/new").
		Handler(RedirectHandler("/"))

	pph.Router.Methods("POST").
		Path("/new").
		Handler(http.HandlerFunc(pph.pasteCreate))

	pph.Router.Methods("GET").
		Path("/{id}.json").
		Handler(RequiredModelObjectHandler(pph.lookupPasteWithRequest, ModelRenderFunc(pph.getPasteJSONHandler))).
		Name("show")

	pph.Router.Methods("GET").
		Path("/{id}").
		Handler(RequiredModelObjectHandler(pph.lookupPasteWithRequest, RenderPageForModel("paste_show"))).
		Name("show")

	pph.Router.Methods("POST").
		Path("/{id}/grant/new").
		Handler(RequiredModelObjectHandler(pph.lookupPasteWithRequest, requiresEditPermission(ModelRenderFunc(pph.pasteGrantHandler)))).
		Name("grant")
	pph.Router.Methods("GET").
		Path("/grant/{grantkey}/accept").
		Handler(http.HandlerFunc(pph.grantAcceptHandler)).
		Name("grant_accept")
	pph.Router.Methods("GET").
		Path("/{id}/disavow").
		Handler(RequiredModelObjectHandler(pph.lookupPasteWithRequest, requiresEditPermission(ModelRenderFunc(pph.pasteUngrantHandler))))

	pph.Router.Methods("GET").
		Path("/{id}/raw").
		Handler(RequiredModelObjectHandler(pph.lookupPasteWithRequest, ModelRenderFunc(pph.getPasteRawHandler))).
		Name("raw")
	pph.Router.Methods("GET").
		Path("/{id}/download").
		Handler(RequiredModelObjectHandler(pph.lookupPasteWithRequest, ModelRenderFunc(pph.getPasteRawHandler))).
		Name("download")

	pph.Router.Methods("GET").
		Path("/{id}/edit").
		Handler(RequiredModelObjectHandler(pph.lookupPasteWithRequest, requiresEditPermission(RenderPageForModel("paste_edit")))).
		Name("edit")
	pph.Router.Methods("POST").
		Path("/{id}/edit").
		Handler(RequiredModelObjectHandler(pph.lookupPasteWithRequest, requiresEditPermission(pph.pasteUpdate)))

	pph.Router.Methods("GET").
		Path("/{id}/delete").
		Handler(RequiredModelObjectHandler(pph.lookupPasteWithRequest, requiresEditPermission(RenderPageForModel("paste_delete_confirm")))).
		Name("delete")
	pph.Router.Methods("POST").
		Path("/{id}/delete").
		Handler(RequiredModelObjectHandler(pph.lookupPasteWithRequest, requiresEditPermission(pph.pasteDelete)))

	pph.Router.Methods("POST").
		Path("/{id}/report").
		Handler(RequiredModelObjectHandler(pph.lookupPasteWithRequest, pph.reportPaste)).
		Name("report")

	pph.Router.Methods("GET").
		MatcherFunc(HTTPSMuxMatcher).
		Path("/{id}/authenticate").
		Handler(RenderPageHandler("paste_authenticate")).
		Name("authenticate")
	pph.Router.Methods("POST").
		MatcherFunc(HTTPSMuxMatcher).
		Path("/{id}/authenticate").
		Handler(http.HandlerFunc(pph.authenticatePastePOSTHandler))

	pph.Router.Methods("GET").
		MatcherFunc(NonHTTPSMuxMatcher).
		Path("/{id}/authenticate").
		Handler(RenderPageHandler("paste_authenticate_disallowed"))
}
