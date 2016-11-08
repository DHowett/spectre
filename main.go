package main

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/gob"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/DHowett/ghostbin/lib/formatting"
	"github.com/DHowett/ghostbin/lib/four"
	"github.com/DHowett/ghostbin/lib/templatepack"
	"github.com/DHowett/ghostbin/model"

	"github.com/DHowett/gotimeout"
	"github.com/golang/glog"
	"github.com/gorilla/mux"
	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"

	_ "github.com/jinzhu/gorm/dialects/postgres"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
)

func isEditAllowed(p model.Paste, r *http.Request) bool {
	return GetPastePermissionScope(p.GetID(), r).Has(model.PastePermissionEdit)
}

func requiresUserPermission(permission model.Permission, handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer errorRecoveryHandler(w)

		user := GetUser(r)
		if user != nil {
			if user.Permissions(model.PermissionClassUser).Has(permission) {
				handler.ServeHTTP(w, r)
				return
			}
		}

		panic(fmt.Errorf("You are not allowed to be here. >:|"))
	})
}

func pasteURL(routeType string, p model.PasteID) string {
	url, _ := pasteRouter.Get(routeType).URL("id", p.String())
	return url.String()
}

func sessionHandler(w http.ResponseWriter, r *http.Request) {
	var ids []model.PasteID

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
		v3Perms, _ := v3EntriesI.(map[model.PasteID]model.Permission)

		ids = make([]model.PasteID, len(v3Perms))
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

	sessionPastes, err := pasteStore.GetPastes(ids)
	if err != nil {
		panic(err)
	}
	templatePack.ExecutePage(w, r, "session", sessionPastes)
}

func requestVariable(rc *templatepack.Context, variable string) string {
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
	templatePack.ExecutePartial(w, r, name, nil)
}

func pasteDestroyCallback(p model.Paste) {
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

var pasteStore model.Broker
var grantStore model.Broker
var userStore model.Broker

var pasteExpirator *gotimeout.Expirator
var sessionStore *sessions.FilesystemStore
var clientOnlySessionStore *sessions.CookieStore
var clientLongtermSessionStore *sessions.CookieStore
var ephStore *gotimeout.Map
var pasteRouter *mux.Router
var router *mux.Router

var globalInit Initializer

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

func (a *args) parse() error {
	a.parseOnce.Do(func() {
		flag.Parse()
	})
	return nil
}

var arguments = &args{}

func init() {
	// N.B. this should not be necessary.
	gob.Register(map[model.PasteID][]byte(nil))
	gob.Register(map[model.PasteID]model.Permission{})
	runtime.GOMAXPROCS(runtime.NumCPU())

	arguments.register()

	globalInit.Add(&InitHandler{
		Priority: 1,
		Name:     "args",
		Do:       arguments.parse,
	})
}

func initTemplateFunctions() {
	templatePack.AddFunction("encryptionAllowed", func(ri *templatepack.Context) bool {
		return Env() == EnvironmentDevelopment || RequestIsHTTPS(ri.Request)
	})
	templatePack.AddFunction("editAllowed", func(ri *templatepack.Context) bool { return isEditAllowed(ri.Obj.(model.Paste), ri.Request) })
	// TODO(DH) MOVE
	templatePack.AddFunction("render", renderPaste)
	templatePack.AddFunction("pasteURL", func(e string, p model.Paste) string {
		return pasteURL(e, p.GetID())
	})
	templatePack.AddFunction("pasteWillExpire", func(p model.Paste) bool {
		return p.GetExpiration() != "" && p.GetExpiration() != "-1"
	})
	templatePack.AddFunction("pasteFromID", func(id model.PasteID) model.Paste {
		p, err := pasteStore.GetPaste(id, nil)
		if err != nil {
			return nil
		}
		return p
	})
	templatePack.AddFunction("truncatedPasteBody", func(p model.Paste, lines int) string {
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
	templatePack.AddFunction("pasteBody", func(p model.Paste) string {
		reader, _ := p.Reader()
		defer reader.Close()
		b := &bytes.Buffer{}
		io.Copy(b, reader)
		return b.String()
	})
	templatePack.AddFunction("requestVariable", requestVariable)
	templatePack.AddFunction("languageNamed", func(name string) *formatting.Language {
		return formatting.LanguageNamed(name)
	})
}

func loadOrGenerateSessionKey(path string, keyLength int) (data []byte, err error) {
	data, err = SlurpFile(path)
	if err != nil {
		data = securecookie.GenerateRandomKey(keyLength)
		err = ioutil.WriteFile(path, data, 0600)
	}
	return
}

func initSessionStore() {
	sessionKeyFile := filepath.Join(arguments.root, "session.key")
	sessionKey, err := loadOrGenerateSessionKey(sessionKeyFile, 32)
	if err != nil {
		glog.Fatal("session.key not found, and an attempt to create one failed: ", err)
	}

	sesdir := filepath.Join(arguments.root, "sessions")
	os.Mkdir(sesdir, 0700)
	sessionStore = sessions.NewFilesystemStore(sesdir, sessionKey)
	sessionStore.Options.Path = "/"
	sessionStore.Options.MaxAge = 86400 * 365

	clientKeyFile := filepath.Join(arguments.root, "client_session_enc.key")
	clientOnlySessionEncryptionKey, err := loadOrGenerateSessionKey(clientKeyFile, 32)
	if err != nil {
		glog.Fatal("client_session_enc.key not found, and an attempt to create one failed: ", err)
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
}

func initModelBroker() {
	dbDialect := "sqlite3"
	sqlDb, err := sql.Open(dbDialect, "ghostbin.db")
	//dbDialect := "postgres"
	//sqlDb, err := sql.Open(dbDialect, "postgres://postgres:password@antares:32768/ghostbin?sslmode=disable")
	//  "postgres://postgres:password@antares:32768/ghostbin?sslmode=disable"
	if err != nil {
		panic(err)
	}

	broker, err := model.NewDatabaseBroker(dbDialect, sqlDb, &AuthChallengeProvider{})
	if err != nil {
		panic(err)
	}

	// TODO(DH): destruction callbacks
	//pasteStore.PasteDestroyCallback = PasteCallback(pasteDestroyCallback)

	pasteExpirator = gotimeout.NewExpirator(filepath.Join(arguments.root, "expiry.gob"), &ExpiringPasteStore{pasteStore})
	ephStore = gotimeout.NewMap()

	go func() {
		for {
			select {
			case err := <-pasteExpirator.ErrorChannel:
				glog.Error("Expirator Error: ", err.Error())
			}
		}
	}()

	grantStore = broker
	pasteStore = broker
	userStore = &PromoteFirstUserToAdminStore{
		&ManglingUserStore{
			broker,
		},
	}
}

func initHandledRoutes(router *mux.Router) {
	/* ADMIN */
	router.Path("/admin").Handler(requiresUserPermission(model.UserPermissionAdmin, RenderPageHandler("admin_home")))

	router.Path("/admin/reports").Handler(requiresUserPermission(model.UserPermissionAdmin, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		templatePack.ExecutePage(w, r, "admin_reports", reportStore.Reports)
	})))

	router.Methods("POST").Path("/admin/promote").Handler(requiresUserPermission(model.UserPermissionAdmin, http.HandlerFunc(adminPromoteHandler)))

	// TODO(DH)
	/*
		router.Methods("POST").
			Path("/admin/paste/{id}/delete").
			Handler(requiresUserPermission(model.UserPermissionAdmin, RequiredModelObjectHandler(lookupPasteWithRequest, pasteDelete))).
			Name("admindelete")

		router.Methods("POST").
			Path("/admin/paste/{id}/clear_report").
			Handler(requiresUserPermission(model.UserPermissionAdmin, http.HandlerFunc(reportClear))).
			Name("reportclear")
	*/

	/* SESSION */
	router.Path("/session").Handler(http.HandlerFunc(sessionHandler))
	router.Path("/session/raw").Handler(http.HandlerFunc(sessionHandler))

	/* GENERAL */
	pasteRouter.Methods("GET").Path("/").Handler(RedirectHandler("/"))

	router.Path("/paste").Handler(RedirectHandler("/"))

	router.Path("/about").Handler(RenderPageHandler("about"))
	router.Methods("GET", "HEAD").Path("/languages.json").Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		modtime, reader := formatting.GetLanguagesJSON()
		http.ServeContent(w, r, "languages.json", modtime, reader)
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
		templatePack.ExecutePage(w, r, "stats", stats)
	}))

	/* PARTIAL */
	router.Methods("GET").
		Path("/partial/{id}").
		Handler(http.HandlerFunc(partialGetHandler))

	router.Path("/").Handler(RenderPageHandler("index"))
}

func initAuthRoutes(router *mux.Router) {
	// Nominally mounted under /auth
	router.Methods("POST").Path("/login").Handler(http.HandlerFunc(authLoginPostHandler))
	router.Methods("POST").Path("/logout").Handler(http.HandlerFunc(authLogoutPostHandler))
	router.Methods("GET").Path("/token").Handler(http.HandlerFunc(authTokenHandler))
	router.Methods("GET").Path("/token/{token}").Handler(http.HandlerFunc(authTokenPageHandler)).Name("auth_token_login")
}

func main() {
	globalInit.Add(&InitHandler{
		Priority: 80,
		Name:     "main_template_funcs",
		Do: func() error {
			initTemplateFunctions()
			return nil
		},
	})
	if err := globalInit.Do(); err != nil {
		panic(err)
	}

	// Establish a signal handler to trigger the reinitializer.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGHUP)
	go func() {
		for _ = range sigChan {
			glog.Info("Received SIGHUP")
			globalInit.Redo()
		}
	}()

	initSessionStore()
	initModelBroker()

	router = mux.NewRouter()
	pasteRouter = router.PathPrefix("/paste").Subrouter()
	authRouter := router.PathPrefix("/auth").Subrouter()
	pasteController := &PasteController{
		PasteStore: pasteStore,
		Router:     pasteRouter,
	}
	pasteController.InitRoutes()
	initAuthRoutes(authRouter)
	initHandledRoutes(router)

	// Permission handler for all routes that may require a user context.
	router.Walk(func(route *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
		route.Handler(permissionMigrationWrapperHandler{route.GetHandler()})
		return nil
	})

	// Static file routes.
	router.PathPrefix("/").Handler(http.FileServer(http.Dir("public")))
	http.Handle("/", four.WrapHandler(router, RenderPageHandler("404")))

	var addr string = arguments.addr
	server := &http.Server{
		Addr: addr,
	}
	server.ListenAndServe()
}
