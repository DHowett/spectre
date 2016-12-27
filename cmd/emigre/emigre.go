package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/DHowett/ghostbin/lib/config"
	"github.com/DHowett/ghostbin/model"
	_ "github.com/DHowett/ghostbin/model/postgres"
	"github.com/Sirupsen/logrus"
	"github.com/jessevdk/go-flags"
	"github.com/jmoiron/sqlx"
)

type options struct {
	Pastes   string `short:"p" long:"pastes" description:"paste store directory" default:"./pastes"`
	Accounts string `short:"a" long:"accounts" description:"account store directory" default:"./accounts"`

	// These options mirror those in spectre core.
	Environment string   `long:"env" description:"Ghostbin environment (dev/production). Influences the default configuration set by including config.$ENV.yml." default:"dev"`
	ConfigFiles []string `long:"config" short:"c" description:"A configuration file (.yml) to read; can be specified multiple times."`

	SkipPastes      bool `long:"sp" description:"skip pastes (and paste bodies, user perms)"`
	SkipUsers       bool `long:"su" description:"skip users (and user perms)"`
	SkipPasteBodies bool `long:"sb" description:"skip paste bodies"`
	SkipUserPerms   bool `long:"sup" description:"skip user perms"`
}

type migrator struct {
	logger logrus.FieldLogger

	config *config.C
	opts   options

	// source
	pasteStore *FilesystemPasteStore
	userStore  *FilesystemUserStore

	// destination
	db *sqlx.DB

	pasteHits map[model.PasteID]struct{}
	userHits  map[string]struct{}

	pendingPasteBodies chan *fsPaste
	pendingUserPerms   chan *User

	// stages 1/2 waitgroups
	// stage 1: pastes
	// stage 2: users
	s1wg, s2wg sync.WaitGroup

	finished chan bool
}

func newMigrator(opts options, config *config.C, logger logrus.FieldLogger) (*migrator, error) {
	m := &migrator{
		logger:             logger,
		config:             config,
		opts:               opts,
		pasteHits:          make(map[model.PasteID]struct{}),
		userHits:           make(map[string]struct{}),
		pendingPasteBodies: make(chan *fsPaste, 1000000),
		pendingUserPerms:   make(chan *User, 100000),
		finished:           make(chan bool),
	}
	m.pasteStore = NewFilesystemPasteStore(opts.Pastes)
	m.userStore = NewFilesystemUserStore(opts.Accounts)

	db, err := sqlx.Open(m.config.Database.Dialect, m.config.Database.Connection)
	if err != nil {
		return nil, err
	}
	m.db = db
	return m, nil
}

func (m *migrator) initSchema() error {
	_, err := model.Open(m.config.Database.Dialect, m.db.DB, &noopChallengeProvider{})
	return err
}

func (m *migrator) migratePastes(logger logrus.FieldLogger) (int, error) {
	logger = logger.WithField("m", "paste")

	defer func() {
		close(m.pendingPasteBodies)
	}()
	nPastes := 0

	inserter := NewBulkInserter(m.db, `INSERT INTO
		pastes(id, created_at, updated_at, expire_at, title, language_name, hmac, encryption_salt, encryption_method)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`)

	filepath.Walk(m.opts.Pastes, func(path string, fi os.FileInfo, err error) error {
		if fi.IsDir() {
			return nil
		}
		plog := logger.WithField("paste", fi.Name())
		fsp, err := m.pasteStore.Get(model.PasteIDFromString(fi.Name()), nil)
		if err != nil {
			plog.Error(err)
			return nil
		}
		m.pasteHits[fsp.ID] = struct{}{}
		nPastes++
		if nPastes%1000 == 0 {
			logger.Infof("%d...", nPastes)
		}

		err = inserter.Insert(fsp.ID.String(), fsp.ModTime, fsp.ModTime, fsp.ExpirationTime, fsp.Title, fsp.Language, fsp.HMAC, fsp.EncryptionSalt, fsp.EncryptionMethod)
		if err != nil {
			logger.Errorf("%d batch failed: %v", nPastes, err)
		}

		m.pendingPasteBodies <- fsp
		return nil
	})

	logger.Infof("%d...", nPastes)
	err := inserter.Flush()
	if err != nil {
		logger.Errorf("%d batch failed: %v", nPastes, err)
	}

	return nPastes, nil
}

func (m *migrator) migratePasteBody(logger logrus.FieldLogger, p *fsPaste, body []byte) error {
	_, err := m.db.Exec("INSERT INTO paste_bodies(paste_id, data) VALUES($1, $2)", p.ID.String(), body)
	return err
}

func (m *migrator) migrateUserPermissions(logger logrus.FieldLogger, u *User, inserter *BulkInserter) (int, error) {
	nUserPerms := 0

	if userPerms, ok := u.Values["permissions"].(*PastePermissionSet); ok {
		nCurrentPerms := 0
		for pid, pperm := range userPerms.Entries {
			if _, ok := m.pasteHits[model.PasteID(pid)]; !ok {
				continue
			}
			var newPerm model.Permission
			if pperm["grant"] {
				newPerm |= model.PastePermissionGrant
			}
			if pperm["edit"] {
				newPerm |= model.PastePermissionEdit
			}

			// legacy grant + edit = all future permissions
			if newPerm == model.PastePermissionGrant|model.PastePermissionEdit {
				newPerm = model.PastePermissionAll
			}

			// This INSERT is prepared with a (SELECT id FROM users WHERE name ...)
			err := inserter.Insert(u.Name, string(pid), newPerm)
			if err != nil {
				logger.Errorf("%d batch failed: %v", nCurrentPerms, err)
			}
			nCurrentPerms++
		}

		nUserPerms += len(userPerms.Entries)
	}
	return nUserPerms, nil
}

func (m *migrator) migrateUsers(logger logrus.FieldLogger) (int, int, error) {
	logger = logger.WithField("m", "user")

	defer func() {
		close(m.pendingUserPerms)
	}()

	inserter := NewBulkInserter(m.db, `INSERT INTO
		users(updated_at, name, salt, challenge, source, permissions)
		VALUES(?, ?, ?, ?, ?, ?)`)

	nUsers := 0
	nUserPerms := 0
	filepath.Walk(m.opts.Accounts, func(path string, fi os.FileInfo, err error) error {
		if fi.IsDir() {
			return nil
		}
		ulog := logger.WithField("user", fi.Name())
		u := m.userStore.Get(fi.Name())
		if u == nil {
			ulog.Error("couldn't load?")
			return nil
		}
		if _, ok := m.userHits[u.Name]; !ok {
			m.userHits[u.Name] = struct{}{}
		} else {
			ulog.Warning("skipped duplicate")
			return nil
		}
		source := model.UserSourceGhostbin
		if u.Persona {
			source = model.UserSourceMozillaPersona
		}
		perms := uint64(0)
		if upp, ok := u.Values["user.permissions"].(PastePermission); ok {
			if upp["admin"] {
				perms = perms | uint64(model.UserPermissionAll)
			}
		}

		nUsers++
		if nUsers%50 == 0 {
			logger.Infof("%d...", nUsers)
		}

		err = inserter.Insert(time.Now(), u.Name, u.Salt, u.Challenge, source, perms)
		if err != nil {
			logger.Errorf("%d batch failed: %v", nUsers, err)
		}

		m.pendingUserPerms <- u
		return nil
	})
	err := inserter.Flush()
	if err != nil {
		logger.Errorf("%d batch failed: %v", nUsers, err)
	}
	return nUsers, nUserPerms, nil
}

type taskReturn struct {
	NPasteBodies           int
	NPasteBodiesFailed     int
	NUsers                 int
	NUsersFailed           int
	NUserPermissions       int
	NUserPermissionsMissed int
}

func (m *migrator) pasteBodyTask(logger logrus.FieldLogger, returnCh chan taskReturn) {
	logger = logger.WithField("m", "body")

	logger.Info("waiting")
	m.s1wg.Wait()
	logger.Info("starting")

	var r taskReturn
outer:
	for {
		select {
		case paste, ok := <-m.pendingPasteBodies:
			if !ok {
				m.pendingPasteBodies = nil
				continue
			}

			if m.opts.SkipPasteBodies {
				continue
			}

			plog := logger.WithField("paste", paste.ID.String())
			rdr, err := paste.Reader()

			if rdr == nil || err != nil {
				r.NPasteBodiesFailed++
				plog.Error("failed to open; err: ", err)
				continue
			}
			buf, err := ioutil.ReadAll(rdr)
			if err != nil {
				rdr.Close()
				r.NPasteBodiesFailed++
				plog.Error("failed to read; err: ", err)
				continue
			}
			rdr.Close()
			err = m.migratePasteBody(plog, paste, buf)
			if err != nil {
				r.NPasteBodiesFailed++
				plog.Error("failed to migrate; err: ", err)
				continue
			}
			r.NPasteBodies++
		default:
			if m.pendingPasteBodies == nil {
				break outer
			}
		}
	}
	logger.Info("Done with pastes.")
	returnCh <- r
	close(returnCh)
}

func (m *migrator) userPermTask(logger logrus.FieldLogger, returnCh chan taskReturn) {
	logger = logger.WithField("m", "permission")

	logger.Info("waiting")
	m.s1wg.Wait()
	m.s2wg.Wait()
	logger.Info("starting")

	inserter := NewBulkInserter(m.db, `INSERT INTO
		user_paste_permissions(user_id, paste_id, permissions)
		VALUES((SELECT id FROM users WHERE name = ?), ?, ?)`)

	var r taskReturn
outer:
	for {
		select {
		case user, ok := <-m.pendingUserPerms:
			if !ok {
				m.pendingUserPerms = nil
				continue
			}

			if m.opts.SkipUserPerms || m.opts.SkipPastes {
				continue
			}

			nperms, err := m.migrateUserPermissions(logger, user, inserter)
			r.NUserPermissions += nperms
			if r.NUserPermissions%50 == 0 {
				logger.Infof("%d...", r.NUserPermissions)
			}
			if err == nil {
				r.NUsers++
				continue
			}

			logger.WithField("user", user.Name).Error("failed to migrate perms; err: ", err)
			r.NUsersFailed++

		default:
			if m.pendingUserPerms == nil {
				break outer
			}
		}
	}

	err := inserter.Flush()
	if err != nil {
		logger.Errorf("flush user perms failed: %v", err)
	}
	logger.Info("Done with users.")
	returnCh <- r
	close(returnCh)
}

func (m *migrator) runBackgroundTasks(logger logrus.FieldLogger) {
	var chs []chan taskReturn
	for i := 0; i < 5; i++ {
		taskLogger := logger.WithField("task", i)

		rch := make(chan taskReturn)
		chs = append(chs, rch)
		go m.pasteBodyTask(taskLogger, rch)

		rch = make(chan taskReturn)
		chs = append(chs, rch)
		go m.userPermTask(taskLogger, rch)
	}

	var r taskReturn
	for _, ch := range chs {
		tret, _ := <-ch
		r.NPasteBodies += tret.NPasteBodies
		r.NPasteBodiesFailed += tret.NPasteBodiesFailed
		r.NUsers += tret.NUsers
		r.NUsersFailed += tret.NUsersFailed
		r.NUserPermissions += tret.NUserPermissions
		r.NUserPermissionsMissed += tret.NUserPermissionsMissed
	}

	logger.Infof("%d pastes (%d failed).", r.NPasteBodies, r.NPasteBodiesFailed)
	logger.Infof("%d users (%d failed).", r.NUsers, r.NUsersFailed)
	logger.Infof("%d user permissions (%d skipped).", r.NUserPermissions, r.NUserPermissionsMissed)
	m.finished <- true
}

func (m *migrator) Run() {
	err := m.initSchema()
	if err != nil {
		m.logger.Fatal(err)
	}

	if !m.opts.SkipPastes {
		m.s1wg.Add(1)
		go func() {
			m.logger.Info("Migrating pastes.")
			_, err = m.migratePastes(m.logger)
			if err != nil {
				m.logger.Error(err)
			}
			m.s1wg.Done()
		}()
	}

	if !m.opts.SkipUsers {
		m.s2wg.Add(1)
		go func() {
			// Users are ungated on s1wg, but user perms require all paste metadata.
			m.logger.Info("Migrating users.")
			_, _, err = m.migrateUsers(m.logger)
			if err != nil {
				m.logger.Error(err)
			}
			m.s2wg.Done()
		}()
	}

	go m.runBackgroundTasks(m.logger)

	<-m.finished
}

func loadConfiguration(opts options, logger logrus.FieldLogger) *config.C {
	var c config.C
	// Base config: required
	err := c.AppendFile("config.yml")
	if err != nil {
		logger.Fatalf("failed to load base config file config.yml: %v", err)
	}

	envConfig := fmt.Sprintf("config.%s.yml", opts.Environment)
	err = c.AppendFile(envConfig)
	if err != nil {
		logger.Fatalf("failed to load environment config file %s: %v", envConfig, err)
	}

	for _, f := range opts.ConfigFiles {
		err = c.AppendFile(f)
		if err != nil {
			logger.Fatalf("failed to load additional config file %s: %v", f, err)
		}
	}

	return &c
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	logger := logrus.New()
	var opts options
	_, err := flags.Parse(&opts)
	if flagErr, ok := err.(*flags.Error); flagErr != nil && ok {
		return
	}

	config := loadConfiguration(opts, logger)

	m, err := newMigrator(opts, config, logger)
	if err != nil {
		logger.Fatal(err)
	}

	m.Run()
}
