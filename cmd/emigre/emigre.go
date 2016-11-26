package main

import (
	"database/sql"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/DHowett/ghostbin/model"
	"github.com/Sirupsen/logrus"
	"github.com/jessevdk/go-flags"

	_ "github.com/jinzhu/gorm/dialects/postgres"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
)

type options struct {
	Pastes   string `short:"p" long:"pastes" description:"paste store directory" default:"./pastes"`
	Accounts string `short:"a" long:"accounts" description:"account store directory" default:"./accounts"`

	Dialect  string `short:"D" long:"dialect" description:"database dialect for -d" default:"sqlite3"`
	Database string `short:"d" long:"db" description:"output sql file" default:"out.db"`
}

// SQLITE3
//  DATETIME(%d, "unixepoch")
// POSTGRESQL
//  TO_TIMESTAMP(%d)

func execBatchQuery(db *sql.DB, base, repeat string, per int, values ...interface{}) (sql.Result, error) {
	placeholders := make([]string, len(values)/per)
	for i, _ := range placeholders {
		placeholders[i] = repeat
	}

	query := base + strings.Join(placeholders, ", ")
	return db.Exec(query, values...)
}

type pendingDbUser struct {
	u  *User
	id int64
}

type migrator struct {
	Logger logrus.FieldLogger

	opts options

	// source
	pasteStore *FilesystemPasteStore
	userStore  *FilesystemUserStore

	// destination
	dialect string
	mu      sync.Mutex
	db      *sql.DB

	pasteHits map[model.PasteID]struct{}
	userHits  map[string]struct{}

	pendingPasteBodies chan *fsPaste
	pendingUserPerms   chan *pendingDbUser

	Finished chan bool
}

func newMigrator(opts options) (*migrator, error) {
	m := &migrator{
		opts:               opts,
		dialect:            opts.Dialect,
		pasteHits:          make(map[model.PasteID]struct{}),
		userHits:           make(map[string]struct{}),
		pendingPasteBodies: make(chan *fsPaste, 1000000),
		pendingUserPerms:   make(chan *pendingDbUser, 100000),
		Finished:           make(chan bool),
	}
	m.pasteStore = NewFilesystemPasteStore(opts.Pastes)
	m.userStore = NewFilesystemUserStore(opts.Accounts)

	db, err := sql.Open(m.dialect, opts.Database)
	if err != nil {
		return nil, err
	}
	m.db = db
	go m.runBackgroundTasks()
	return m, nil
}

func (m *migrator) InitSchema() error {
	_, err := model.NewDatabaseBroker(m.dialect, m.db, nil)
	return err
}

func (m *migrator) MigratePastes() (int, error) {
	defer func() {
		close(m.pendingPasteBodies)
	}()
	insertPasteQueryBase := `INSERT INTO
		pastes(id, created_at, updated_at, expire_at, title, language_name, hmac, encryption_salt, encryption_method)
		VALUES`
	insertPasteQueryRepeat := `(?, ?, ?, ?, ?, ?, ?, ?, ?)`
	valPerQuery := 9

	nPastes := 0

	batchSz := 999 / valPerQuery
	pasteV := make([]interface{}, 0, batchSz*valPerQuery)
	filepath.Walk(m.opts.Pastes, func(path string, fi os.FileInfo, err error) error {
		if fi.IsDir() {
			return nil
		}
		plog := m.Logger.WithField("paste", fi.Name())
		fsp, err := m.pasteStore.Get(model.PasteIDFromString(fi.Name()), nil)
		if err != nil {
			plog.Error(err)
			return nil
		}
		pasteV = append(pasteV, []interface{}{fsp.ID.String(), fsp.ModTime, fsp.ModTime, fsp.ExpirationTime, fsp.Title, fsp.Language, fsp.HMAC, fsp.EncryptionSalt, fsp.EncryptionMethod}...)
		m.pasteHits[fsp.ID] = struct{}{}
		nPastes++
		if nPastes%batchSz == 0 {
			m.mu.Lock()
			_, err := execBatchQuery(m.db, insertPasteQueryBase, insertPasteQueryRepeat, valPerQuery, pasteV...)
			if err != nil {
				m.Logger.Errorf("%d batch(%d) failed: %v", nPastes, batchSz, err)
			}
			m.Logger.Infof("%d...", nPastes)
			pasteV = pasteV[0:0]
			m.mu.Unlock()
		}
		m.pendingPasteBodies <- fsp
		return nil
	})
	if nPastes%batchSz != 0 {
		m.mu.Lock()
		n := nPastes % batchSz
		_, err := execBatchQuery(m.db, insertPasteQueryBase, insertPasteQueryRepeat, valPerQuery, pasteV[:valPerQuery*n]...)
		if err != nil {
			m.Logger.Errorf("%d batch(%d) failed: %v", nPastes, n, err)
		}
		m.mu.Unlock()
	}
	return nPastes, nil
}

func (m *migrator) migratePasteBody(p *fsPaste, body []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, err := m.db.Exec("INSERT INTO paste_bodies(paste_id, data) VALUES(?, ?)", p.ID.String(), body)
	return err
}

func (m *migrator) migrateUserPermissions(pu *pendingDbUser) (int, error) {
	insertUserPermissionQueryBase := `INSERT INTO
		user_paste_permissions(user_id, paste_id, permissions)
		VALUES`
	insertUserPermissionQueryRepeat := `(?, ?, ?)`
	valPerQuery := 3

	nUserPerms := 0
	batchSz := 999 / valPerQuery

	if userPerms, ok := pu.u.Values["permissions"].(*PastePermissionSet); ok {
		permsV := make([]interface{}, 0, valPerQuery*batchSz)
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

			permsV = append(permsV, pu.id)
			permsV = append(permsV, string(pid))
			permsV = append(permsV, newPerm)
			nCurrentPerms++
			if nCurrentPerms%batchSz == 0 {
				m.mu.Lock()
				_, err := execBatchQuery(m.db, insertUserPermissionQueryBase, insertUserPermissionQueryRepeat, valPerQuery, permsV...)
				if err != nil {
					m.Logger.Errorf("%d batch(%d) failed: %v", nCurrentPerms, batchSz, err)
				}
				permsV = permsV[0:0]
				m.mu.Unlock()
			}
		}
		if nCurrentPerms%batchSz != 0 {
			m.mu.Lock()
			n := nCurrentPerms % batchSz
			_, err := execBatchQuery(m.db, insertUserPermissionQueryBase, insertUserPermissionQueryRepeat, valPerQuery, permsV[:valPerQuery*n]...)
			if err != nil {
				m.Logger.Errorf("%d batch(%d) failed: %v", nCurrentPerms, n, err)
			}
			m.mu.Unlock()
		}
		nUserPerms += len(userPerms.Entries)
	}
	return nUserPerms, nil

}

func (m *migrator) MigrateUsers() (int, int, error) {
	defer func() {
		close(m.pendingUserPerms)
	}()
	insertUserQuery, err := m.db.Prepare(`INSERT INTO
		users(updated_at, name, salt, challenge, source, permissions)
		VALUES(?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return 0, 0, err
	}

	nUsers := 0
	nUserPerms := 0
	filepath.Walk(m.opts.Accounts, func(path string, fi os.FileInfo, err error) error {
		if fi.IsDir() {
			return nil
		}
		ulog := m.Logger.WithField("user", fi.Name())
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
		m.mu.Lock()
		res, err := insertUserQuery.Exec(time.Now(), u.Name, u.Salt, u.Challenge, source, perms)
		if err != nil {
			m.mu.Unlock()
			ulog.Error(err)
			return nil
		}
		sqlUid, err := res.LastInsertId()
		if err != nil {
			m.mu.Unlock()
			ulog.Error(err)
			return nil
		}
		m.mu.Unlock()

		m.pendingUserPerms <- &pendingDbUser{u: u, id: sqlUid}
		nUsers++
		if nUsers%50 == 0 {
			m.Logger.Infof("%d...", nUsers)
		}
		return nil
	})
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

func (m *migrator) backgroundTask(n int, returnCh chan taskReturn) {
	logger := m.Logger.WithField("task", n)
	var r taskReturn
	for {
		select {
		case paste, ok := <-m.pendingPasteBodies:
			if !ok {
				logger.Info("Done with pastes.")
				m.pendingPasteBodies = nil
				continue
			}

			plog := logger.WithField("paste", paste.ID.String())
			rdr, err := paste.Reader()
			// simulate load.
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
			err = m.migratePasteBody(paste, buf)
			if err != nil {
				r.NPasteBodiesFailed++
				plog.Error("failed to migrate; err: ", err)
				continue
			}
			r.NPasteBodies++
		case pendingUser, ok := <-m.pendingUserPerms:
			if !ok {
				logger.Info("Done with users.")
				m.pendingUserPerms = nil
				continue
			}

			nperms, err := m.migrateUserPermissions(pendingUser)
			r.NUserPermissions += nperms
			if err != nil {
				logger.WithField("user", pendingUser.u.Name).Error("failed to migrate perms; err: ", err)
				r.NUsersFailed++
			} else {
				r.NUsers++
			}
		default:
			if m.pendingPasteBodies == nil && m.pendingUserPerms == nil {
				returnCh <- r
				close(returnCh)
				return
			}
		}
	}
}

func (m *migrator) runBackgroundTasks() {
	var chs []chan taskReturn
	for i := 0; i < 10; i++ {
		rch := make(chan taskReturn)
		chs = append(chs, rch)
		go m.backgroundTask(i, rch)
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

	m.Logger.Infof("%d pastes (%d failed).", r.NPasteBodies, r.NPasteBodiesFailed)
	m.Logger.Infof("%d users (%d failed).", r.NUsers, r.NUsersFailed)
	m.Logger.Infof("%d user permissions (%d skipped).", r.NUserPermissions, r.NUserPermissionsMissed)
	m.Finished <- true
}

func main() {
	logger := logrus.New()
	var opts options
	_, err := flags.Parse(&opts)
	if err != nil {
		logger.Fatal(err)
	}

	m, err := newMigrator(opts)
	if err != nil {
		logger.Fatal(err)
	}
	m.Logger = logger

	err = m.InitSchema()
	if err != nil {
		logger.Fatal(err)
	}

	_, err = m.MigratePastes()
	if err != nil {
		logger.Error(err)
	}

	_, _, err = m.MigrateUsers()
	if err != nil {
		logger.Error(err)
	}

	<-m.Finished
}
