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

	"github.com/DHowett/ghostbin/lib/config"
	"github.com/DHowett/ghostbin/lib/formatting"
	"github.com/DHowett/ghostbin/lib/rayman"
	ghtime "github.com/DHowett/ghostbin/lib/time"
	"github.com/DHowett/ghostbin/model"
	"github.com/DHowett/ghostbin/views"
	"github.com/DHowett/gotimeout"

	"github.com/Sirupsen/logrus"
	"github.com/dustin/go-humanize"
	"github.com/golang/groupcache/lru"
	"github.com/gorilla/mux"
)

const CURRENT_ENCRYPTION_METHOD model.PasteEncryptionMethod = model.PasteEncryptionMethodAES_CTR

type pasteHandler interface {
	ServeHTTPForPaste(model.Paste, http.ResponseWriter, *http.Request)
}

type pasteHandlerFunc func(model.Paste, http.ResponseWriter, *http.Request)

func (h pasteHandlerFunc) ServeHTTPForPaste(p model.Paste, w http.ResponseWriter, r *http.Request) {
	h(p, w, r)
}

type PasteAccessDeniedError struct {
	action string
	ID     model.PasteID
}

func (e PasteAccessDeniedError) Error() string {
	return fmt.Sprintf("You're not allowed to %s paste %v.", e.action, e.ID)
}

func (PasteAccessDeniedError) StatusCode() int {
	return http.StatusUnauthorized
}

type PasteTooLargeError struct {
	Size, Max int
}

func (e PasteTooLargeError) Error() string {
	return fmt.Sprintf("Your input (%v) exceeds the maximum paste length (%v).", humanize.IBytes(uint64(e.Size)), humanize.IBytes(uint64(e.Max)))
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
	App    Application        `inject:""`
	Model  model.Provider     `inject:""`
	Config *config.C          `inject:""`
	Logger logrus.FieldLogger `inject:""`

	renderCacheMu sync.RWMutex
	renderCache   *lru.Cache

	tempHashes gotimeout.Map

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
	Body         template.HTML
	RenderedBody template.HTML
	Editable     bool
}

func (pc *PasteController) getPasteFromRequest(r *http.Request) (model.Paste, error) {
	id := model.PasteIDFromString(mux.Vars(r)["id"])
	var passphrase []byte

	session := sessionBroker.Get(r)
	if pasteKeys, ok := session.Get(SessionScopeSensitive, "paste_passphrases").(map[model.PasteID][]byte); ok {
		if _key, ok := pasteKeys[id]; ok {
			passphrase = _key
		}
	}

	p, err := pc.Model.GetPaste(r.Context(), id, passphrase)
	return p, err
}

func (pc *PasteController) wrapPasteHandler(handler pasteHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := model.PasteIDFromString(mux.Vars(r)["id"])
		p, err := pc.getPasteFromRequest(r)

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
					pc.pasteNotFoundView.Exec(w, r, id)
				} else {
					panic(err)
				}
				return
			}

			handler.ServeHTTPForPaste(p, w, r)
		}
	})
}

func (pc *PasteController) wrapPasteEditHandler(handler pasteHandler) pasteHandler {
	return pasteHandlerFunc(func(p model.Paste, w http.ResponseWriter, r *http.Request) {
		editable := isEditAllowed(p, r)
		if !editable {
			accerr := PasteAccessDeniedError{"modify", p.GetID()}
			pc.App.RespondWithError(w, accerr)
			return
		}
		handler.ServeHTTPForPaste(p, w, r)
	})
}

func (pc *PasteController) pasteRawGetHandler(p model.Paste, w http.ResponseWriter, r *http.Request) {
	header := w.Header()
	header.Set("Access-Control-Allow-Origin", "null")
	header.Set("Vary", "Origin")

	header.Set("Content-Security-Policy", "default-src 'none'")
	header.Set("Content-Type", "text/plain; charset=utf-8")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("X-XSS-Protection", "1; mode=block")

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
		header.Set("Content-Disposition", "attachment; filename=\""+filename+"."+ext+"\"")
		header.Set("Content-Transfer-Encoding", "binary")
	}

	reader, _ := p.Reader()
	defer reader.Close()
	io.Copy(w, reader)
}

func (pc *PasteController) pasteGrantHandler(p model.Paste, w http.ResponseWriter, r *http.Request) {
	grant, _ := pc.Model.CreateGrant(r.Context(), p)

	acceptURL := pc.App.GenerateURL(URLTypePasteGrantAccept, "grantkey", string(grant.GetID()))

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.Encode(map[string]string{
		"acceptURL": BaseURLForRequest(r).ResolveReference(acceptURL).String(),
		"key":       string(grant.GetID()),
		"id":        p.GetID().String(),
	})
}

func (pc *PasteController) pasteUngrantHandler(p model.Paste, w http.ResponseWriter, r *http.Request) {
	GetPastePermissionScope(p.GetID(), r).Revoke(model.PastePermissionAll)
	SavePastePermissionScope(w, r)

	SetFlash(w, "success", fmt.Sprintf("Paste %v disavowed.", p.GetID()))
	w.Header().Set("Location", pc.App.GenerateURL(URLTypePasteShow, "id", p.GetID().String()).String())
	w.WriteHeader(http.StatusSeeOther)
}

func (pc *PasteController) grantAcceptHandler(w http.ResponseWriter, r *http.Request) {
	v := mux.Vars(r)
	grantKey := model.GrantID(v["grantkey"])
	grant, err := pc.Model.GetGrant(r.Context(), grantKey)
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

func (pc *PasteController) pasteUpdateImpl(p model.Paste, w http.ResponseWriter, r *http.Request, body string) {
	lang := formatting.LanguageNamed(p.GetLanguageName())
	if r.FormValue("lang") != "" {
		lang = formatting.LanguageNamed(r.FormValue("lang"))
	}

	expireIn := r.FormValue("expire")
	if expireIn != "" && expireIn != "-1" {
		dur, _ := ghtime.ParseDuration(expireIn)
		if dur > time.Duration(pc.Config.Application.Limits.PasteMaxExpiration) {
			dur = time.Duration(pc.Config.Application.Limits.PasteMaxExpiration)
		}
		expireAt := time.Now().Add(dur)
		p.SetExpirationTime(expireAt)
	} else {
		// Empty expireIn means "keep current expiration."
		if expireIn == "-1" {
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

func (pc *PasteController) pasteUpdateHandler(p model.Paste, w http.ResponseWriter, r *http.Request) {
	body := r.FormValue("text")
	if len(strings.TrimSpace(body)) == 0 {
		w.Header().Set("Location", pc.App.GenerateURL(URLTypePasteDelete, "id", p.GetID().String()).String())
		w.WriteHeader(http.StatusFound)
		return
	}

	pasteLen := len(body)
	if pasteLen > pc.Config.Application.Limits.PasteSize {
		pc.App.RespondWithError(w, PasteTooLargeError{pasteLen, pc.Config.Application.Limits.PasteSize})
		return
	}

	pc.pasteUpdateImpl(p, w, r, body)

	// Blow away the hashcache.
	pid := p.GetID().String()
	v, _ := pc.tempHashes.Get(pid)
	if hash, ok := v.(string); ok {
		pc.tempHashes.Delete(hash)
		pc.tempHashes.Delete(pid)
	}
}

func (pc *PasteController) pasteCreateHandler(w http.ResponseWriter, r *http.Request) {
	session := sessionBroker.Get(r)

	body := r.FormValue("text")
	if len(strings.TrimSpace(body)) == 0 {
		pc.App.RespondWithError(w, webErrEmptyPaste)
		return
	}

	pasteLen := len(body)
	if pasteLen > pc.Config.Application.Limits.PasteSize {
		pc.App.RespondWithError(w, PasteTooLargeError{pasteLen, pc.Config.Application.Limits.PasteSize})
		return
	}

	password := r.FormValue("password")
	encrypted := password != ""

	if encrypted && (!RequestIsHTTPS(r) && !pc.Config.Application.ForceInsecureEncryption) {
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

		tempPid, _ := pc.tempHashes.Get(hashToken)
		if hPid, ok := tempPid.(model.PasteID); ok {
			// update it later (and harmlessly renew permissions)
			// only do so if the user is already the de-facto owner
			if GetPastePermissionScope(hPid, r).Has(model.PastePermissionAll) {
				var err error
				p, err = pc.Model.GetPaste(r.Context(), hPid, nil)
				if err != nil {
					panic(err)
				}
			}
		}

		if p == nil {
			p, err = pc.Model.CreatePaste(r.Context())
			if err != nil {
				panic(err)
			}
		}

		// Temporarily map hash -> paste ID and paste ID -> hash (for invalidation purposes)
		pc.tempHashes.Put(hashToken, p.GetID(), 5*time.Minute)
		pc.tempHashes.Put(p.GetID().String(), hashToken, 5*time.Minute)
	} else {
		p, err = pc.Model.CreateEncryptedPaste(r.Context(), CURRENT_ENCRYPTION_METHOD, []byte(password))
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

	pc.pasteUpdateImpl(p, w, r, body)
}

func (pc *PasteController) pasteDeletePOSTHandler(p model.Paste, w http.ResponseWriter, r *http.Request) {
	oldId := p.GetID()
	p.Erase()

	GetPastePermissionScope(oldId, r).Revoke(model.PastePermissionAll)
	SavePastePermissionScope(w, r)

	SetFlash(w, "success", fmt.Sprintf("Paste %v deleted.", oldId))

	w.Header().Set("Location", "/")
	w.WriteHeader(http.StatusFound)
}

func (pc *PasteController) authenticatePOSTHandler(w http.ResponseWriter, r *http.Request) {
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

func (pc *PasteController) pasteReportHandler(p model.Paste, w http.ResponseWriter, r *http.Request) {
	if throttleAuthForRequest(r) {
		pc.App.RespondWithError(w, webErrThrottled)
		return
	}

	err := pc.Model.ReportPaste(r.Context(), p)
	if err != nil {
		SetFlash(w, "error", fmt.Sprintf("Paste %v could not be reported.", p.GetID()))
	} else {
		SetFlash(w, "success", fmt.Sprintf("Paste %v reported.", p.GetID()))
	}

	w.Header().Set("Location", pc.App.GenerateURL(URLTypePasteShow, "id", p.GetID().String()).String())
	w.WriteHeader(http.StatusFound)
}

func (pc *PasteController) pasteEditHandler(p model.Paste, w http.ResponseWriter, r *http.Request) {
	reader, err := p.Reader()
	if err != nil {
		panic(err)
	}

	buf, err := ioutil.ReadAll(reader)
	if err != nil {
		panic(err)
	}

	pc.pasteEditView.Exec(w, r, &pasteViewFacade{
		Paste:    p,
		Editable: true,
		Body:     template.HTML(buf),
	})
}

func (pc *PasteController) pasteShowHandler(p model.Paste, w http.ResponseWriter, r *http.Request) {
	body, err := pc.renderPaste(r.Context(), p)
	if err != nil {
		pc.App.RespondWithError(w, webErrFailedToRender)
		return
	}

	pc.pasteShowView.Exec(w, r, &pasteViewFacade{
		Paste:        p,
		Editable:     isEditAllowed(p, r),
		RenderedBody: body,
	})
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
func (pc *PasteController) renderPaste(ctx context.Context, p model.Paste) (template.HTML, error) {
	logger := rayman.ContextLogger(ctx).WithFields(logrus.Fields{
		"facility": "cache",
		"paste":    p.GetID(),
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
		timedCtx, cancelContext := context.WithTimeout(ctx, 5*time.Second)
		defer cancelContext()
		out, err := FormatPaste(timedCtx, p)

		if err != nil {
			logger.WithField("err", err).Errorf("render failed: %s", out)
			return template.HTML(""), err
		}

		rendered := template.HTML(out)
		if !p.IsEncrypted() {
			if pc.renderCache == nil {
				pc.renderCache = &lru.Cache{
					MaxEntries: pc.Config.Application.Limits.PasteCache,
					OnEvicted: func(key lru.Key, value interface{}) {
						pc.Logger.WithFields(logrus.Fields{
							"facility": "cache",
							"paste":    key,
						}).Info("evicted paste")
					},
				}
			}
			pc.renderCache.Add(p.GetID(), &renderedPaste{body: rendered, renderTime: time.Now()})
			logger.Info("cached")
		}

		return rendered, nil
	} else {
		return cached.body, nil
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
			Handler(pc.wrapPasteHandler(pasteHandlerFunc(pc.pasteShowHandler)))

	pasteGrantRoute :=
		router.Methods("POST").
			Path("/{id}/grant/new").
			Handler(pc.wrapPasteHandler(pc.wrapPasteEditHandler(pasteHandlerFunc(pc.pasteGrantHandler))))

	pasteGrantAcceptRoute :=
		router.Methods("GET").
			Path("/grant/{grantkey}/accept").
			HandlerFunc(pc.grantAcceptHandler)

	router.Methods("GET").
		Path("/{id}/disavow").
		Handler(pc.wrapPasteHandler(pc.wrapPasteEditHandler(pasteHandlerFunc(pc.pasteUngrantHandler))))

	pasteRawRoute :=
		router.Methods("GET").
			Path("/{id}/raw").
			Handler(pc.wrapPasteHandler(pasteHandlerFunc(pc.pasteRawGetHandler)))

	pasteDownloadRoute :=
		router.Methods("GET").
			Path("/{id}/download").
			Handler(pc.wrapPasteHandler(pasteHandlerFunc(pc.pasteRawGetHandler)))

	pasteEditRoute :=
		router.Methods("GET").
			Path("/{id}/edit").
			Handler(pc.wrapPasteHandler(pc.wrapPasteEditHandler(pasteHandlerFunc(pc.pasteEditHandler))))
	router.Methods("POST").
		Path("/{id}/edit").
		Handler(pc.wrapPasteHandler(pc.wrapPasteEditHandler(pasteHandlerFunc(pc.pasteUpdateHandler))))

		/*
			pasteDeleteRoute :=
				router.Methods("GET").
					Path("/{id}/delete").
					Handler(pc.pasteHandlerWrapper(pc.pasteEditHandlerWrapper(pc.pasteDeleteView)))
		*/

	router.Methods("POST").
		Path("/{id}/delete").
		Handler(pc.wrapPasteHandler(pc.wrapPasteEditHandler(pasteHandlerFunc(pc.pasteDeletePOSTHandler))))

	pasteReportRoute :=
		router.Methods("POST").
			Path("/{id}/report").
			Handler(pc.wrapPasteHandler(pasteHandlerFunc(pc.pasteReportHandler)))

	var pasteAuthenticateRoute *mux.Route
	if pc.Config.Application.ForceInsecureEncryption {
		// The only functional difference between this and the below is that
		// these are not guarded with the HTTPS/!HTTPS mux matchers.
		pasteAuthenticateRoute =
			router.Methods("GET").
				Path("/{id}/authenticate").
				Handler(pc.pasteAuthenticateView)

		router.Methods("POST").
			Path("/{id}/authenticate").
			HandlerFunc(pc.authenticatePOSTHandler)
	} else {
		pasteAuthenticateRoute =
			router.Methods("GET").
				MatcherFunc(HTTPSMuxMatcher).
				Path("/{id}/authenticate").
				Handler(pc.pasteAuthenticateView)

		router.Methods("POST").
			MatcherFunc(HTTPSMuxMatcher).
			Path("/{id}/authenticate").
			HandlerFunc(pc.authenticatePOSTHandler)

		router.Methods("GET").
			MatcherFunc(NonHTTPSMuxMatcher).
			Path("/{id}/authenticate").
			Handler(pc.pasteAuthenticateDisallowedView)
	}

	// catch-all rule that redirects paste/ to /
	router.Methods("GET").Path("/").Handler(RedirectHandler("/"))

	pc.App.RegisterRouteForURLType(URLTypePasteShow, pasteShowRoute)
	pc.App.RegisterRouteForURLType(URLTypePasteGrant, pasteGrantRoute)
	pc.App.RegisterRouteForURLType(URLTypePasteGrantAccept, pasteGrantAcceptRoute)
	pc.App.RegisterRouteForURLType(URLTypePasteRaw, pasteRawRoute)
	pc.App.RegisterRouteForURLType(URLTypePasteDownload, pasteDownloadRoute)
	pc.App.RegisterRouteForURLType(URLTypePasteEdit, pasteEditRoute)
	//pc.App.RegisterRouteForURLType(URLTypePasteDelete, pasteDeleteRoute)
	pc.App.RegisterRouteForURLType(URLTypePasteReport, pasteReportRoute)
	pc.App.RegisterRouteForURLType(URLTypePasteAuthenticate, pasteAuthenticateRoute)
}

func (pc *PasteController) BindViews(viewModel *views.Model) error {
	return bindViews(viewModel, nil, map[interface{}]**views.View{
		views.PageID("paste_show"):                    &pc.pasteShowView,
		views.PageID("paste_edit"):                    &pc.pasteEditView,
		views.PageID("paste_delete_confirm"):          &pc.pasteDeleteView,
		views.PageID("paste_authenticate"):            &pc.pasteAuthenticateView,
		views.PageID("paste_authenticate_disallowed"): &pc.pasteAuthenticateDisallowedView,
		views.PageID("paste_not_found"):               &pc.pasteNotFoundView,
	})
}
