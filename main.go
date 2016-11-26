package main

import (
	"database/sql"
	"encoding/gob"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/DHowett/ghostbin/lib/formatting"
	"github.com/DHowett/ghostbin/lib/four"
	"github.com/DHowett/ghostbin/lib/templatepack"
	"github.com/DHowett/ghostbin/model"
	"github.com/DHowett/ghostbin/views"

	"github.com/DHowett/gotimeout"
	log "github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"

	_ "github.com/jinzhu/gorm/dialects/postgres"
	_ "github.com/jinzhu/gorm/dialects/sqlite"

	"github.com/facebookgo/inject"
)

func isEditAllowed(p model.Paste, r *http.Request) bool {
	return GetPastePermissionScope(p.GetID(), r).Has(model.PastePermissionEdit)
}

func requestVariable(rc *templatepack.Context, variable string) string {
	v, _ := mux.Vars(rc.Request)[variable]
	if v == "" {
		v = rc.Request.FormValue(variable)
	}
	return v
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

	log.Info("RENDER CACHE: Removing ", p.GetID(), " due to destruction.")
	// Clear the cached render when a paste is destroyed
	renderCache.c.Remove(p.GetID())
}

var sessionBroker *SessionBroker

var pasteExpirator *gotimeout.Expirator
var ephStore *gotimeout.Map

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

func loadOrGenerateSessionKey(path string, keyLength int) (data []byte, err error) {
	data, err = ioutil.ReadFile(path)
	if err != nil {
		data = securecookie.GenerateRandomKey(keyLength)
		err = ioutil.WriteFile(path, data, 0600)
	}
	return
}

func initSessionStore() *SessionBroker {
	sessionKeyFile := filepath.Join(arguments.root, "session.key")
	sessionKey, err := loadOrGenerateSessionKey(sessionKeyFile, 32)
	if err != nil {
		log.Fatal("session.key not found, and an attempt to create one failed: ", err)
	}

	sesdir := filepath.Join(arguments.root, "sessions")
	os.Mkdir(sesdir, 0700)
	serverSessionStore := sessions.NewFilesystemStore(sesdir, sessionKey)
	serverSessionStore.Options.Path = "/"
	serverSessionStore.Options.MaxAge = 86400 * 365

	clientKeyFile := filepath.Join(arguments.root, "client_session_enc.key")
	clientOnlySessionEncryptionKey, err := loadOrGenerateSessionKey(clientKeyFile, 32)
	if err != nil {
		log.Fatal("client_session_enc.key not found, and an attempt to create one failed: ", err)
	}
	sensitiveSessionStore := sessions.NewCookieStore(sessionKey, clientOnlySessionEncryptionKey)
	if Env() != EnvironmentDevelopment {
		sensitiveSessionStore.Options.Secure = true
	}
	sensitiveSessionStore.Options.Path = "/"
	sensitiveSessionStore.Options.MaxAge = 0

	clientSessionStore := sessions.NewCookieStore(sessionKey, clientOnlySessionEncryptionKey)
	if Env() != EnvironmentDevelopment {
		clientSessionStore.Options.Secure = true
	}
	clientSessionStore.Options.Path = "/"
	clientSessionStore.Options.MaxAge = 86400 * 365

	return NewSessionBroker(map[SessionScope]sessions.Store{
		SessionScopeServer:    serverSessionStore,
		SessionScopeClient:    clientSessionStore,
		SessionScopeSensitive: sensitiveSessionStore,
	})
}

func establishModelConnection() model.Broker {
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

	pasteExpirator = gotimeout.NewExpiratorWithStorage(gotimeout.NoopAdapter{}, &ExpiringPasteStore{broker})
	expiringPastes, err := broker.GetExpiringPastes()
	if err != nil {
		log.Error("Error loading expiring pastes: ", err)
	}

	now := time.Now()
	for _, ep := range expiringPastes {
		pid := ep.PasteID
		t := ep.Time
		pasteExpirator.ExpireObject(ExpiringPasteID(pid), t.Sub(now))
	}
	ephStore = gotimeout.NewMap()

	go func() {
		for {
			select {
			case err := <-pasteExpirator.ErrorChannel:
				log.Error("Expirator Error: ", err.Error())
			}
		}
	}()

	return &PromoteFirstUserToAdminStore{
		&ManglingUserStore{
			broker,
		},
	}
}

type ghostbinApplication struct {
	mutex     sync.RWMutex
	urlRoutes map[URLType]*mux.Route

	indexView *views.View
	aboutView *views.View
	errorView *views.View

	Logger log.FieldLogger `inject:""`
}

func (a *ghostbinApplication) RegisterRouteForURLType(ut URLType, route *mux.Route) {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if a.urlRoutes == nil {
		a.urlRoutes = make(map[URLType]*mux.Route)
	}
	a.urlRoutes[ut] = route
}

func (a *ghostbinApplication) GenerateURL(ut URLType, params ...string) *url.URL {
	a.mutex.RLock()
	defer a.mutex.RUnlock()

	u, ok := a.urlRoutes[ut]
	err := errors.New("route doesn't exist!")
	var ret *url.URL
	if ok {
		ret, err = u.URL(params...)
	}

	if err != nil {
		log.Error("unable to generate url type <%s> (params %v): %v", ut, params, err)

		return &url.URL{
			Path: "/",
		}
	}
	return ret
}

// From views.DataProvider
func (a *ghostbinApplication) ViewValue(r *http.Request, name string) interface{} {
	if r == nil {
		return nil
	}

	switch name {
	case "request":
		return r
	case "app":
		return a
	case "user":
		return GetLoggedInUser(r)
	}
	return nil
}

func (a *ghostbinApplication) GetViewFunctions() views.FuncMap {
	return views.FuncMap{
		"generatePasteURL": func(kind string, p model.Paste) *url.URL {
			return a.GenerateURL(URLType("paste."+kind), "id", p.GetID().String())
		},
		"getLanguageNamed": func(name string) *formatting.Language {
			return formatting.LanguageNamed(name)
		},
	}
}

func (a *ghostbinApplication) InitRoutes(router *mux.Router) {
	router.Methods("GET", "HEAD").Path("/languages.json").Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		modtime, reader := formatting.GetLanguagesJSON()
		http.ServeContent(w, r, "languages.json", modtime, reader)
	}))

	/* PARTIAL */
	router.Methods("GET").
		Path("/partial/{id}").
		Handler(http.HandlerFunc(partialGetHandler))

	router.Path("/about").Handler(a.aboutView)
	router.Path("/").Handler(a.indexView)
}

func (a *ghostbinApplication) BindViews(viewModel *views.Model) error {
	return bindViews(viewModel, nil, map[interface{}]**views.View{
		views.PageID("index"): &a.indexView,
		views.PageID("about"): &a.aboutView,
		views.PageID("error"): &a.errorView,
	})
}

func (a *ghostbinApplication) RespondWithError(w http.ResponseWriter, webErr WebError) {
	w.WriteHeader(webErr.StatusCode())
	err2 := a.errorView.Exec(w, nil, webErr)
	if err2 != nil {
		log.Error("failed to render error response:", err2)
	}
}

// TODO(DH) DO NOT LEAVE GLOBAL
var ghostbin = &ghostbinApplication{}

/////////////////////////////////////
// Temporarily keep env stuff here //
/////////////////////////////////////
var environment string = EnvironmentDevelopment

func Env() string {
	return environment
}

/////////////////////////////////////

func init() {
	// N.B. this should not be necessary.
	gob.Register(map[model.PasteID][]byte(nil))
	gob.Register(map[model.PasteID]model.Permission{})
	runtime.GOMAXPROCS(runtime.NumCPU())

	arguments.register()

	/////////////////////////////////////
	// Temporarily keep env stuff here //
	/////////////////////////////////////
	environment = os.Getenv("GHOSTBIN_ENV")
	if environment != EnvironmentProduction {
		environment = EnvironmentDevelopment
	}
	/////////////////////////////////////

}

func main() {
	arguments.parse()

	//////////////////////////////////////
	// Temporarily keep lang stuff here //
	//////////////////////////////////////
	formatting.LoadLanguageConfig("languages.yml")
	//////////////////////////////////////

	logger := log.New()

	var config Configuration

	viewModel, err := views.New("templates/*.tmpl", views.FieldLoggingOption(logger.WithField("ctx", "viewmodel")), views.GlobalDataProviderOption(ghostbin), views.GlobalFunctionsOption(ghostbin))
	if err != nil {
		log.Fatal(err)
	}

	// Establish a signal handler to trigger the reinitializer.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGHUP)
	go func() {
		for _ = range sigChan {
			log.Info("Received SIGHUP")
			viewModel.Reload()
			// TODO(DH) DUPED
			formatting.LoadLanguageConfig("languages.yml")
		}
	}()

	// global
	sessionBroker = initSessionStore()

	modelBroker := establishModelConnection()
	pasteController := &PasteController{}
	adminController := &AdminController{}
	sessionController := &SessionController{}
	authController := &AuthController{}

	var graph inject.Graph
	graph.Logger = logger.WithField("ctx", "inject")
	err = graph.Provide(
		&inject.Object{
			Complete: true,
			Value:    modelBroker,
		},
		&inject.Object{
			Complete: true,
			Value:    &config,
		},
		&inject.Object{
			Complete: true,
			Value:    logger,
		},
		&inject.Object{
			Value: ghostbin,
		},
		&inject.Object{
			Value: pasteController,
		},
		&inject.Object{
			Value: adminController,
		},
		&inject.Object{
			Value: sessionController,
		},
		&inject.Object{
			Value: authController,
		},
	)
	if err != nil {
		log.Fatal(err)
	}

	err = graph.Populate()
	if err != nil {
		log.Fatal(err)
	}

	controllerRoutes := map[string]Controller{
		"/paste":   pasteController,
		"/auth":    authController,
		"/session": sessionController,
		"/admin":   adminController,
		"":         ghostbin, // Application
	}

	router := mux.NewRouter()
	// Set Strict Slashes because subrouters/controller routes can register on Path("/").
	router.StrictSlash(true)
	for pathPrefix, controller := range controllerRoutes {
		l := log.WithFields(log.Fields{
			"controller": fmt.Sprintf("%+T", controller),
			"path":       pathPrefix,
		})

		err := controller.BindViews(viewModel)
		if err != nil {
			l.Fatal("unable to bind views:", err)
		}

		r := router
		if pathPrefix != "" {
			r = router.PathPrefix(pathPrefix).Subrouter()
		}
		l.Infof("registering routes")
		controller.InitRoutes(r)
	}

	// Permission handler for all routes that may require a user context.
	router.Walk(func(route *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
		route.Handler(permissionMigrationWrapperHandler{route.GetHandler()})
		return nil
	})

	// Static file routes.
	router.PathPrefix("/").Handler(http.FileServer(http.Dir("public")))

	var rootHandler http.Handler = router

	fourOhFourTemplate, _ := viewModel.Bind(views.PageID("404"), nil)
	rootHandler = four.WrapHandler(rootHandler, fourOhFourTemplate)
	rootHandler = UserLookupHandler(modelBroker, rootHandler)
	// User depends on Session, so install that handler last.
	rootHandler = sessionBroker.Handler(rootHandler)

	http.Handle("/", rootHandler)

	var addr string = arguments.addr
	server := &http.Server{
		Addr: addr,
	}
	server.ListenAndServe()
}
