package main

import (
	"crypto/tls"
	"database/sql"
	"encoding/gob"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"

	"github.com/jessevdk/go-flags"

	// Ghostbin
	"github.com/DHowett/ghostbin/lib/config"
	"github.com/DHowett/ghostbin/lib/formatting"
	"github.com/DHowett/ghostbin/lib/four"
	"github.com/DHowett/ghostbin/views"
	"github.com/DHowett/gotimeout"
	"github.com/facebookgo/inject"

	// Model
	"github.com/DHowett/ghostbin/model"
	_ "github.com/DHowett/ghostbin/model/postgres"

	// Logging
	"github.com/Sirupsen/logrus"

	// Web
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
)

func isEditAllowed(p model.Paste, r *http.Request) bool {
	return GetPastePermissionScope(p.GetID(), r).Has(model.PastePermissionEdit)
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

	//defer renderCache.mu.Unlock()
	//renderCache.mu.Lock()
	//if renderCache.c == nil {
	//return
	//}

	//log.Info("RENDER CACHE: Removing ", p.GetID(), " due to destruction.")
	// Clear the cached render when a paste is destroyed
	//renderCache.c.Remove(p.GetID())
}

var sessionBroker *SessionBroker

var pasteExpirator *gotimeout.Expirator
var ephStore *gotimeout.Map

func loadOrGenerateSessionKey(path string, keyLength int) (data []byte, err error) {
	data, err = ioutil.ReadFile(path)
	if err != nil {
		data = securecookie.GenerateRandomKey(keyLength)
		err = ioutil.WriteFile(path, data, 0600)
	}
	return
}

type ghostbinApplication struct {
	mutex     sync.RWMutex
	urlRoutes map[URLType]*mux.Route

	indexView *views.View
	aboutView *views.View
	errorView *views.View

	rootHandler http.Handler

	Logger        logrus.FieldLogger `inject:""`
	Configuration *config.C
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
		logrus.Error("unable to generate url type <%s> (params %v): %v", ut, params, err)

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
	case "encryption":
		return RequestIsHTTPS(r) || a.Configuration.Application.ForceInsecureEncryption
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
		a.Logger.Error("failed to render error response:", err2)
	}
}

func (a *ghostbinApplication) initSessionStore() (*SessionBroker, error) {
	sessionKeyFile := filepath.Join(arguments.Root, "session.key")
	sessionKey, err := loadOrGenerateSessionKey(sessionKeyFile, 32)
	if err != nil {
		return nil, fmt.Errorf("session.key not found, and an attempt to create one failed: %v", err)
	}

	sesdir := filepath.Join(arguments.Root, "sessions")
	os.Mkdir(sesdir, 0700)
	serverSessionStore := sessions.NewFilesystemStore(sesdir, sessionKey)
	serverSessionStore.Options.Path = "/"
	serverSessionStore.Options.MaxAge = 86400 * 365

	clientKeyFile := filepath.Join(arguments.Root, "client_session_enc.key")
	clientOnlySessionEncryptionKey, err := loadOrGenerateSessionKey(clientKeyFile, 32)
	if err != nil {
		return nil, fmt.Errorf("client_session_enc.key not found, and an attempt to create one failed: %v", err)
	}
	sensitiveSessionStore := sessions.NewCookieStore(sessionKey, clientOnlySessionEncryptionKey)
	sensitiveSessionStore.Options.Path = "/"
	sensitiveSessionStore.Options.MaxAge = 0

	clientSessionStore := sessions.NewCookieStore(sessionKey, clientOnlySessionEncryptionKey)
	clientSessionStore.Options.Path = "/"
	clientSessionStore.Options.MaxAge = 86400 * 365

	if a.Configuration.Application.ForceInsecureEncryption {
		sensitiveSessionStore.Options.Secure = true
		clientSessionStore.Options.Secure = true
	}

	return NewSessionBroker(map[SessionScope]sessions.Store{
		SessionScopeServer:    serverSessionStore,
		SessionScopeClient:    clientSessionStore,
		SessionScopeSensitive: sensitiveSessionStore,
	}), nil
}

func (a *ghostbinApplication) initModelProvider() (model.Provider, error) {
	dbDialect := a.Configuration.Database.Dialect
	sqlDb, err := sql.Open(dbDialect, a.Configuration.Database.Connection)
	if err != nil {
		return nil, err
	}

	broker, err := model.Open(
		dbDialect,
		sqlDb,
		&AuthChallengeProvider{},
		model.FieldLoggingOption(a.Logger.WithField("ctx", "model")),
	)
	if err != nil {
		return nil, err
	}

	// TODO(DH): destruction callbacks
	//pasteStore.PasteDestroyCallback = PasteCallback(pasteDestroyCallback)

	ephStore = gotimeout.NewMap()
	pasteExpirator = gotimeout.NewExpiratorWithStorage(gotimeout.NoopAdapter{}, &ExpiringPasteStore{broker})

	go func() {
		for {
			select {
			case err := <-pasteExpirator.ErrorChannel:
				a.Logger.Error("Expirator Error: ", err.Error())
			}
		}
	}()

	return &PromoteFirstUserToAdminStore{
		&ManglingUserStore{
			broker,
		},
	}, nil
}

func (a *ghostbinApplication) init() error {
	viewModel, err := views.New(
		"templates/*.tmpl",
		views.FieldLoggingOption(a.Logger.WithField("ctx", "viewmodel")),
		views.GlobalDataProviderOption(a),
		views.GlobalFunctionsOption(a),
	)
	if err != nil {
		return err
	}

	// Establish a signal handler to trigger the reinitializer.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGHUP)
	go func() {
		for _ = range sigChan {
			a.Logger.Info("Received SIGHUP")
			viewModel.Reload()
			// TODO(DH) DUPED
			formatting.LoadLanguageConfig("languages.yml")
		}
	}()

	// global
	sessionBroker, err = a.initSessionStore()
	if err != nil {
		return err
	}

	modelBroker, err := a.initModelProvider()
	if err != nil {
		return err
	}

	pasteController := &PasteController{}
	adminController := &AdminController{}
	sessionController := &SessionController{}
	authController := &AuthController{}

	var graph inject.Graph
	graph.Logger = a.Logger.WithField("ctx", "inject")
	err = graph.Provide(
		&inject.Object{Value: a},
		&inject.Object{
			Complete: true,
			Value:    modelBroker,
		},
		&inject.Object{
			Complete: true,
			Value:    a.Configuration,
		},
		&inject.Object{
			Complete: true,
			Value:    a.Logger,
		},
		&inject.Object{Value: pasteController},
		&inject.Object{Value: adminController},
		&inject.Object{Value: sessionController},
		&inject.Object{Value: authController},
	)
	if err != nil {
		return err
	}

	err = graph.Populate()
	if err != nil {
		return err
	}

	controllerRoutes := map[string]Controller{
		"/paste":   pasteController,
		"/auth":    authController,
		"/session": sessionController,
		"/admin":   adminController,
		"":         a, // Application
	}

	router := mux.NewRouter()
	// Set Strict Slashes because subrouters/controller routes can register on Path("/").
	router.StrictSlash(true)
	for pathPrefix, controller := range controllerRoutes {
		l := a.Logger.WithFields(logrus.Fields{
			"controller": fmt.Sprintf("%+T", controller),
			"path":       pathPrefix,
		})

		err := controller.BindViews(viewModel)
		if err != nil {
			return fmt.Errorf("unable to bind views: %v", err)
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

	a.rootHandler = rootHandler
	return nil
}

func (a *ghostbinApplication) Run() error {
	err := a.init()
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	for _, webConfig := range a.Configuration.Web {
		logger := a.Logger.WithFields(logrus.Fields{
			"ctx":  "http",
			"addr": webConfig.Bind,
		})

		var handler http.Handler = a.rootHandler

		if webConfig.Proxied {
			handler = handlers.ProxyHeaders(handler)
			logger = logger.WithField("proxied", true)
		}

		if webConfig.SSL != nil {
			logger = logger.WithField("ssl", true)
		}

		server := &http.Server{
			Addr:      webConfig.Bind,
			Handler:   handler,
			TLSConfig: defaultTLSConfig,
		}

		wg.Add(1)
		go func() {
			var err error
			if webConfig.SSL == nil {
				err = server.ListenAndServe()
			} else {
				err = server.ListenAndServeTLS(webConfig.SSL.Certificate, webConfig.SSL.Key)
			}

			if err != nil {
				logger.Fatal(err)
			}
			wg.Done()
		}()
		logger.Info("listening")
	}
	wg.Wait()
	a.Logger.Warning("all servers terminated")
	return nil
}

func init() {
	// N.B. this should not be necessary.
	gob.Register(map[model.PasteID][]byte(nil))
	gob.Register(map[model.PasteID]model.Permission{})
	runtime.GOMAXPROCS(runtime.NumCPU())
}

var arguments struct {
	Environment string   `long:"env" description:"Ghostbin environment (dev/production). Influences the default configuration set by including config.$ENV.yml." default:"dev"`
	Root        string   `long:"root" short:"r" description:"A directory to store Slate's state in."`
	ConfigFiles []string `long:"config" short:"c" description:"A configuration file (.yml) to read; can be specified multiple times."`
}

func loadConfiguration(logger logrus.FieldLogger) *config.C {
	var c config.C
	// Base config: required
	err := c.AppendFile("config.yml")
	if err != nil {
		logger.Fatalf("failed to load base config file config.yml: %v", err)
	}

	envConfig := fmt.Sprintf("config.%s.yml", arguments.Environment)
	err = c.AppendFile(envConfig)
	if err != nil {
		logger.Fatalf("failed to load environment config file %s: %v", envConfig, err)
	}

	for _, f := range arguments.ConfigFiles {
		err = c.AppendFile(f)
		if err != nil {
			logger.Fatalf("failed to load additional config file %s: %v", f, err)
		}
	}

	return &c
}

func main() {
	_, err := flags.Parse(&arguments)
	if flagErr, ok := err.(*flags.Error); flagErr != nil && ok {
		return
	}

	//////////////////////////////////////
	// Temporarily keep lang stuff here //
	//////////////////////////////////////
	formatting.LoadLanguageConfig("languages.yml")
	//////////////////////////////////////

	logger := logrus.New()
	logger.Formatter = &logrus.TextFormatter{
		ForceColors: true,
	}

	conf := loadConfiguration(logger)

	switch conf.Logging.Destination.Type {
	case "terminal":
		// no-op: logger is terminal by default
	case "file":
		// TODO(DH): This.
	}
	logger.Level = conf.Logging.Level.LogrusLevel()

	app := &ghostbinApplication{
		Logger:        logger,
		Configuration: conf,
	}

	err = app.Run()
	if err != nil {
		logger.Fatal(err)
	}
}

var defaultTLSConfig *tls.Config = &tls.Config{
	PreferServerCipherSuites: true,
	CurvePreferences: []tls.CurveID{
		tls.CurveP256,
	},
	MinVersion: tls.VersionTLS12,
	CipherSuites: []uint16{
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		//tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305, // Go 1.8 only
		//tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,   // Go 1.8 only
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,

		// Best disabled, as they don't provide Forward Secrecy,
		// but might be necessary for some clients
		// tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
		// tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
	},
}
