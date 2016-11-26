package main

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/DHowett/ghostbin/lib/formatting"
	"github.com/DHowett/ghostbin/model"
	"github.com/DHowett/ghostbin/views"

	log "github.com/Sirupsen/logrus"
	"github.com/golang/groupcache/lru"
	"github.com/gorilla/mux"
)

// paste http bindings
const CURRENT_ENCRYPTION_METHOD model.PasteEncryptionMethod = model.PasteEncryptionMethodAES_CTR
const PASTE_CACHE_MAX_ENTRIES int = 1000
const PASTE_MAXIMUM_LENGTH ByteSize = 1048576 // 1 MB
const MAX_EXPIRE_DURATION time.Duration = 15 * 24 * time.Hour

type PasteAccessDeniedError struct {
	action string
	ID     model.PasteID
}

func (e PasteAccessDeniedError) Error() string {
	return "You're not allowed to " + e.action + " paste " + e.ID.String() + "."
}

func (PasteAccessDeniedError) StatusCode() int {
	return http.StatusUnauthorized
}

type PasteTooLargeError ByteSize

func (e PasteTooLargeError) Error() string {
	return fmt.Sprintf("Your input (%v) exceeds the maximum paste length, which is %v.", ByteSize(e), PASTE_MAXIMUM_LENGTH)
}

func (PasteTooLargeError) StatusCode() int {
	return http.StatusRequestEntityTooLarge
}

// renderedPaste is stored in PasteController's renderCache.
type renderedPaste struct {
	body       template.HTML
	renderTime time.Time
}

type PasteController struct {
	App    Application    `inject:""`
	Model  model.Broker   `inject:""`
	Config *Configuration `inject:""`

	renderCacheMu sync.RWMutex
	renderCache   *lru.Cache

	// General purpose views.
	pasteShowView         *views.View
	pasteEditView         *views.View
	pasteDeleteView       *views.View
	pasteAuthenticateView *views.View

	// Error views.
	pasteAuthenticateDisallowedView *views.View
	pasteNotFoundView               *views.View
}

type pasteViewFacade struct {
	model.Paste
	c        *PasteController
	editable bool
}

func (pv *pasteViewFacade) GetRenderedBody() template.HTML {
	return pv.c.renderPaste(pv.Paste)
}

func (pc *pasteViewFacade) GetEditBody() (template.HTML, error) {
	// raw paste body for editing
	reader, err := pc.Reader()
	if err != nil {
		return "", err
	}

	buf, err := ioutil.ReadAll(reader)
	if err != nil {
		return "", err
	}

	return template.HTML(buf), nil
}

func (pv *pasteViewFacade) ExpirationTime() time.Time {
	// TODO(DH) lol
	return time.Now()
}

func (pv *pasteViewFacade) Editable() bool {
	return pv.editable
}

type pasteSessionKeyType int

const pasteSessionKey pasteSessionKeyType = 0

func (pc *PasteController) getPasteFromRequest(r *http.Request) (model.Paste, *http.Request, error) {
	p, ok := r.Context().Value(pasteSessionKey).(model.Paste)
	if !ok {
		id := model.PasteIDFromString(mux.Vars(r)["id"])
		var passphrase []byte

		session := sessionBroker.Get(r)
		if pasteKeys, ok := session.Get(SessionScopeSensitive, "paste_passphrases").(map[model.PasteID][]byte); ok {
			if _key, ok := pasteKeys[id]; ok {
				passphrase = _key
			}
		}

		p, err := pc.Model.GetPaste(id, passphrase)
		if p != nil {
			p = &pasteViewFacade{
				Paste:    p,
				c:        pc,
				editable: isEditAllowed(p, r),
			}
		}
		r = r.WithContext(context.WithValue(r.Context(), pasteSessionKey, p))
		return p, r, err
	}
	return p, r, nil
}

func (pc *PasteController) ViewValue(r *http.Request, name string) interface{} {
	if r == nil {
		return nil
	}

	switch name {
	case "paste":
		// explicitly ignoring request as it's immutable and err as we can't propagate it.
		p, _, _ := pc.getPasteFromRequest(r)
		return p
	}
	return nil
}

func (pc *PasteController) pasteHandlerWrapper(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := model.PasteIDFromString(mux.Vars(r)["id"])
		_, r, err := pc.getPasteFromRequest(r)

		if err == model.ErrPasteEncrypted || err == model.ErrInvalidKey {
			url := pc.App.GenerateURL(URLTypePasteAuthenticate, "id", id.String())
			if err == model.ErrInvalidKey {
				url.Query().Set("i", "1")
			}

			http.SetCookie(w, &http.Cookie{
				Name:  "destination",
				Value: r.URL.String(),
				Path:  "/",
			})
			w.Header().Set("Location", url.String())
			w.WriteHeader(http.StatusFound)
		} else {
			if err != nil {
				if err == model.ErrNotFound {
					w.WriteHeader(http.StatusNotFound)
					pc.pasteNotFoundView.Exec(w, r)
				} else {
					w.WriteHeader(http.StatusInternalServerError)
					templatePack.ExecutePage(w, r, "error", err)
				}
				return
			}

			handler.ServeHTTP(w, r)
		}
	})
}

func (pc *PasteController) pasteEditHandlerWrapper(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Ignoring err; it was handled above us.
		p, r, _ := pc.getPasteFromRequest(r)
		editable := false
		if pfacade, ok := p.(*pasteViewFacade); ok {
			editable = pfacade.editable
		}
		if !editable {
			accerr := PasteAccessDeniedError{"modify", p.GetID()}
			pc.App.RespondWithError(w, accerr)
			return
		}
		handler.ServeHTTP(w, r)
	})
}

func (pc *PasteController) getPasteRawHandler(w http.ResponseWriter, r *http.Request) {
	p, _, _ := pc.getPasteFromRequest(r)

	w.Header().Set("Access-Control-Allow-Origin", "null")
	w.Header().Set("Vary", "Origin")

	w.Header().Set("Content-Security-Policy", "default-src 'none'")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-XSS-Protection", "1; mode=block")

	ext := "txt"
	if mux.CurrentRoute(r).GetName() == "download" {
		lang := formatting.LanguageNamed(p.GetLanguageName())
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

func (pc *PasteController) pasteGrantHandler(w http.ResponseWriter, r *http.Request) {
	p, _, _ := pc.getPasteFromRequest(r)

	grant, _ := pc.Model.CreateGrant(p)

	acceptURL := pc.App.GenerateURL(URLTypePasteGrantAccept, "grantkey", string(grant.GetID()))

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.Encode(map[string]string{
		"acceptURL": BaseURLForRequest(r).ResolveReference(acceptURL).String(),
		"key":       string(grant.GetID()),
		"id":        p.GetID().String(),
	})
}

func (pc *PasteController) pasteUngrantHandler(w http.ResponseWriter, r *http.Request) {
	p, _, _ := pc.getPasteFromRequest(r)

	GetPastePermissionScope(p.GetID(), r).Revoke(model.PastePermissionAll)
	SavePastePermissionScope(w, r)

	SetFlash(w, "success", fmt.Sprintf("Paste %v disavowed.", p.GetID()))
	w.Header().Set("Location", pc.App.GenerateURL(URLTypePasteShow, "id", p.GetID().String()).String())
	w.WriteHeader(http.StatusSeeOther)
}

func (pc *PasteController) grantAcceptHandler(w http.ResponseWriter, r *http.Request) {
	v := mux.Vars(r)
	grantKey := model.GrantID(v["grantkey"])
	grant, err := pc.Model.GetGrant(grantKey)
	if err != nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Hey."))
		return
	}

	pID := grant.GetPasteID()
	GetPastePermissionScope(pID, r).Grant(model.PastePermissionEdit)
	SavePastePermissionScope(w, r)

	grant.Destroy()

	SetFlash(w, "success", fmt.Sprintf("You now have edit rights to Paste %v.", pID))
	w.Header().Set("Location", pc.App.GenerateURL(URLTypePasteShow, "id", pID.String()).String())
	w.WriteHeader(http.StatusSeeOther)
}

func (pc *PasteController) updateOrCreatePaste(p model.Paste, w http.ResponseWriter, r *http.Request, body string) {
	lang := formatting.LanguageNamed(p.GetLanguageName())
	if r.FormValue("lang") != "" {
		lang = formatting.LanguageNamed(r.FormValue("lang"))
	}

	expireIn := r.FormValue("expire")
	ePid := ExpiringPasteID(p.GetID())
	if expireIn != "" && expireIn != "-1" {
		dur, _ := ParseDuration(expireIn)
		if dur > MAX_EXPIRE_DURATION {
			dur = MAX_EXPIRE_DURATION
		}
		expireAt := time.Now().Add(dur)
		pasteExpirator.ExpireObject(ePid, dur)
		p.SetExpirationTime(expireAt)
	} else {
		// Empty expireIn means "keep current expiration."
		if expireIn == "-1" && pasteExpirator.ObjectHasExpiration(ePid) {
			pasteExpirator.CancelObjectExpiration(ePid)
			p.ClearExpirationTime()
		}
	}

	if lang != nil {
		p.SetLanguageName(lang.ID)
	}

	p.SetTitle(r.FormValue("title"))

	pw, _ := p.Writer()
	pw.Write([]byte(body))
	pw.Close() // Saves p

	w.Header().Set("Location", pc.App.GenerateURL(URLTypePasteShow, "id", p.GetID().String()).String())
	w.WriteHeader(http.StatusSeeOther)
}

func (pc *PasteController) pasteUpdateHandler(w http.ResponseWriter, r *http.Request) {
	p, _, _ := pc.getPasteFromRequest(r)

	body := r.FormValue("text")
	if len(strings.TrimSpace(body)) == 0 {
		w.Header().Set("Location", pc.App.GenerateURL(URLTypePasteDelete, "id", p.GetID().String()).String())
		w.WriteHeader(http.StatusFound)
		return
	}

	pasteLen := ByteSize(len(body))
	if pasteLen > PASTE_MAXIMUM_LENGTH {
		pc.App.RespondWithError(w, PasteTooLargeError(pasteLen))
		return
	}

	pc.updateOrCreatePaste(p, w, r, body)

	// Blow away the hashcache.
	tok := "P|H|" + p.GetID().String()
	v, _ := ephStore.Get(tok)
	if hash, ok := v.(string); ok {
		ephStore.Delete(hash)
		ephStore.Delete(tok)
	}
}

func (pc *PasteController) pasteCreateHandler(w http.ResponseWriter, r *http.Request) {
	session := sessionBroker.Get(r)

	body := r.FormValue("text")
	if len(strings.TrimSpace(body)) == 0 {
		pc.App.RespondWithError(w, webErrEmptyPaste)
		return
	}

	pasteLen := ByteSize(len(body))
	if pasteLen > PASTE_MAXIMUM_LENGTH {
		pc.App.RespondWithError(w, PasteTooLargeError(pasteLen))
		return
	}

	password := r.FormValue("password")
	encrypted := password != ""

	if encrypted && (Env() != EnvironmentDevelopment && !RequestIsHTTPS(r)) {
		pc.App.RespondWithError(w, webErrInsecurePassword)
		return
	}

	var p model.Paste
	var err error

	if !encrypted {
		// We can only hash-dedup non-encrypted pastes.
		hasher := md5.New()
		io.WriteString(hasher, body)
		hashToken := "H|" + SourceIPForRequest(r) + "|" + base32Encoder.EncodeToString(hasher.Sum(nil))

		v, _ := ephStore.Get(hashToken)
		if hashedPaste, ok := v.(model.Paste); ok {
			// update it later (and harmlessly renew permissions)
			p = hashedPaste
		} else {
			p, err = pc.Model.CreatePaste()
			if err != nil {
				panic(err)
			}
		}

		ephStore.Put(hashToken, p, 5*time.Minute)
		ephStore.Put("P|H|"+p.GetID().String(), hashToken, 5*time.Minute)
	} else {
		p, err = pc.Model.CreateEncryptedPaste(CURRENT_ENCRYPTION_METHOD, []byte(password))
		if err != nil {
			panic(err)
		}

		sensitiveScope := session.Scope(SessionScopeSensitive)
		pasteKeys, ok := sensitiveScope.Get("paste_passphrases").(map[model.PasteID][]byte)
		if !ok {
			pasteKeys = map[model.PasteID][]byte{}
		}

		pasteKeys[p.GetID()] = []byte(password)
		sensitiveScope.Set("paste_passphrases", pasteKeys)
	}

	GetPastePermissionScope(p.GetID(), r).Grant(model.PastePermissionAll)
	SavePastePermissionScope(w, r)

	session.Save()

	pc.updateOrCreatePaste(p, w, r, body)
}

func (pc *PasteController) pasteDelete(w http.ResponseWriter, r *http.Request) {
	p, _, _ := pc.getPasteFromRequest(r)

	oldId := p.GetID()
	p.Erase()

	GetPastePermissionScope(oldId, r).Revoke(model.PastePermissionAll)
	SavePastePermissionScope(w, r)

	SetFlash(w, "success", fmt.Sprintf("Paste %v deleted.", oldId))

	w.Header().Set("Location", "/")
	w.WriteHeader(http.StatusFound)
}

func (pc *PasteController) authenticatePastePOSTHandler(w http.ResponseWriter, r *http.Request) {
	if throttleAuthForRequest(r) {
		pc.App.RespondWithError(w, webErrThrottled)
		return
	}

	id := model.PasteIDFromString(mux.Vars(r)["id"])
	passphrase := []byte(r.FormValue("password"))

	session := sessionBroker.Get(r).Scope(SessionScopeSensitive)
	pasteKeys, ok := session.Get("paste_passphrases").(map[model.PasteID][]byte)
	if !ok {
		pasteKeys = map[model.PasteID][]byte{}
	}

	pasteKeys[id] = passphrase
	session.Set("paste_passphrases", pasteKeys)
	session.Save()

	dest := pc.App.GenerateURL(URLTypePasteShow, "id", id.String()).String()
	if destCookie, err := r.Cookie("destination"); err != nil {
		dest = destCookie.Value
	}
	w.Header().Set("Location", dest)
	w.WriteHeader(http.StatusSeeOther)
}

func (pc *PasteController) pasteReportHandler(w http.ResponseWriter, r *http.Request) {
	if throttleAuthForRequest(r) {
		pc.App.RespondWithError(w, webErrThrottled)
		return
	}

	p, _, _ := pc.getPasteFromRequest(r)

	err := pc.Model.ReportPaste(p)
	if err != nil {
		SetFlash(w, "error", fmt.Sprintf("Paste %v could not be reported.", p.GetID()))
	} else {
		SetFlash(w, "success", fmt.Sprintf("Paste %v reported.", p.GetID()))
	}

	w.Header().Set("Location", pc.App.GenerateURL(URLTypePasteShow, "id", p.GetID().String()).String())
	w.WriteHeader(http.StatusFound)
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

// TODO(DH) MOVE
func (pc *PasteController) renderPaste(p model.Paste) template.HTML {
	logger := log.WithFields(log.Fields{
		"ctx":   "cache",
		"paste": p.GetID(),
	})

	pc.renderCacheMu.RLock()

	var cached *renderedPaste
	var cval interface{}
	var ok bool
	if pc.renderCache != nil {
		if cval, ok = pc.renderCache.Get(p.GetID()); ok {
			cached = cval.(*renderedPaste)
		}
	}

	pc.renderCacheMu.RUnlock()

	if !ok || cached.renderTime.Before(p.GetModificationTime()) {
		defer pc.renderCacheMu.Unlock()
		pc.renderCacheMu.Lock()
		out, err := FormatPaste(p)

		if err != nil {
			logger.WithField("err", err).Errorf("render failed: %s", out)
			return template.HTML("There was an error rendering this paste.")
		}

		rendered := template.HTML(out)
		if !p.IsEncrypted() {
			if pc.renderCache == nil {
				pc.renderCache = &lru.Cache{
					MaxEntries: PASTE_CACHE_MAX_ENTRIES,
					OnEvicted: func(key lru.Key, value interface{}) {
						log.WithFields(log.Fields{
							"ctx":   "cache",
							"paste": key,
						}).Info("evicted paste")
					},
				}
			}
			pc.renderCache.Add(p.GetID(), &renderedPaste{body: rendered, renderTime: time.Now()})
			logger.Info("cached")
		}

		return rendered
	} else {
		return cached.body
	}
}

func (pc *PasteController) InitRoutes(router *mux.Router) {
	router.Methods("GET").
		Path("/new").
		Handler(RedirectHandler("/"))

	router.Methods("POST").
		Path("/new").
		HandlerFunc(pc.pasteCreateHandler)

	pasteShowRoute :=
		router.Methods("GET").
			Path("/{id}").
			Handler(pc.pasteHandlerWrapper(pc.pasteShowView))

	pasteGrantRoute :=
		router.Methods("POST").
			Path("/{id}/grant/new").
			Handler(pc.pasteHandlerWrapper(pc.pasteEditHandlerWrapper(http.HandlerFunc(pc.pasteGrantHandler))))

	pasteGrantAcceptRoute :=
		router.Methods("GET").
			Path("/grant/{grantkey}/accept").
			HandlerFunc(pc.grantAcceptHandler)

	router.Methods("GET").
		Path("/{id}/disavow").
		Handler(pc.pasteHandlerWrapper(pc.pasteEditHandlerWrapper(http.HandlerFunc(pc.pasteUngrantHandler))))

	pasteRawRoute :=
		router.Methods("GET").
			Path("/{id}/raw").
			Handler(pc.pasteHandlerWrapper(http.HandlerFunc(pc.getPasteRawHandler)))

	pasteDownloadRoute :=
		router.Methods("GET").
			Path("/{id}/download").
			Handler(pc.pasteHandlerWrapper(http.HandlerFunc(pc.getPasteRawHandler)))

	pasteEditRoute :=
		router.Methods("GET").
			Path("/{id}/edit").
			Handler(pc.pasteHandlerWrapper(pc.pasteEditHandlerWrapper(pc.pasteEditView)))
	router.Methods("POST").
		Path("/{id}/edit").
		Handler(pc.pasteHandlerWrapper(pc.pasteEditHandlerWrapper(http.HandlerFunc(pc.pasteUpdateHandler))))

	pasteDeleteRoute :=
		router.Methods("GET").
			Path("/{id}/delete").
			Handler(pc.pasteHandlerWrapper(pc.pasteEditHandlerWrapper(pc.pasteDeleteView)))

	router.Methods("POST").
		Path("/{id}/delete").
		Handler(pc.pasteHandlerWrapper(pc.pasteEditHandlerWrapper(http.HandlerFunc(pc.pasteDelete))))

	pasteReportRoute :=
		router.Methods("POST").
			Path("/{id}/report").
			Handler(pc.pasteHandlerWrapper(http.HandlerFunc(pc.pasteReportHandler)))

	pasteAuthenticateRoute :=
		router.Methods("GET").
			MatcherFunc(HTTPSMuxMatcher).
			Path("/{id}/authenticate").
			Handler(pc.pasteAuthenticateView)

	router.Methods("POST").
		MatcherFunc(HTTPSMuxMatcher).
		Path("/{id}/authenticate").
		HandlerFunc(pc.authenticatePastePOSTHandler)

	router.Methods("GET").
		MatcherFunc(NonHTTPSMuxMatcher).
		Path("/{id}/authenticate").
		Handler(pc.pasteAuthenticateDisallowedView)

	// catch-all rule that redirects paste/ to /
	router.Methods("GET").Path("/").Handler(RedirectHandler("/"))

	pc.App.RegisterRouteForURLType(URLTypePasteShow, pasteShowRoute)
	pc.App.RegisterRouteForURLType(URLTypePasteGrant, pasteGrantRoute)
	pc.App.RegisterRouteForURLType(URLTypePasteGrantAccept, pasteGrantAcceptRoute)
	pc.App.RegisterRouteForURLType(URLTypePasteRaw, pasteRawRoute)
	pc.App.RegisterRouteForURLType(URLTypePasteDownload, pasteDownloadRoute)
	pc.App.RegisterRouteForURLType(URLTypePasteEdit, pasteEditRoute)
	pc.App.RegisterRouteForURLType(URLTypePasteDelete, pasteDeleteRoute)
	pc.App.RegisterRouteForURLType(URLTypePasteReport, pasteReportRoute)
	pc.App.RegisterRouteForURLType(URLTypePasteAuthenticate, pasteAuthenticateRoute)
}

func (pc *PasteController) BindViews(viewModel *views.Model) error {
	return bindViews(viewModel, pc, map[interface{}]**views.View{
		views.PageID("paste_show"):                    &pc.pasteShowView,
		views.PageID("paste_edit"):                    &pc.pasteEditView,
		views.PageID("paste_delete_confirm"):          &pc.pasteDeleteView,
		views.PageID("paste_authenticate"):            &pc.pasteAuthenticateView,
		views.PageID("paste_authenticate_disallowed"): &pc.pasteAuthenticateDisallowedView,
		views.PageID("paste_not_found"):               &pc.pasteNotFoundView,
	})
}
