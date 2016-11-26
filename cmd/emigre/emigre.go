package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
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

	Dialect  string `short:"D" long:"dialect" description:"database dialect for -c" default:"sqlite3"`
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

type migrator struct {
	Logger logrus.FieldLogger

	opts options

	// source
	pasteStore *FilesystemPasteStore
	userStore  *FilesystemUserStore

	// destination
	dialect string
	db      *sql.DB

	pasteHits map[model.PasteID]struct{}
	userHits  map[string]struct{}
}

func newMigrator(opts options) (*migrator, error) {
	m := &migrator{
		opts:      opts,
		dialect:   opts.Dialect,
		pasteHits: make(map[model.PasteID]struct{}),
		userHits:  make(map[string]struct{}),
	}
	m.pasteStore = NewFilesystemPasteStore(opts.Pastes)
	m.userStore = NewFilesystemUserStore(opts.Accounts)

	db, err := sql.Open(m.dialect, opts.Database)
	if err != nil {
		return nil, err
	}
	m.db = db
	return m, nil
}

func (m *migrator) InitSchema() error {
	_, err := model.NewDatabaseBroker(m.dialect, m.db, nil)
	return err
}

func (m *migrator) MigratePastes() (int, error) {
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
			_, err := execBatchQuery(m.db, insertPasteQueryBase, insertPasteQueryRepeat, valPerQuery, pasteV...)
			if err != nil {
				m.Logger.Errorf("%d batch(%d) failed: %v", nPastes, batchSz, err)
			}
			m.Logger.Infof("%d...", nPastes)
			pasteV = pasteV[0:0]
		}
		return nil
	})
	if nPastes%batchSz != 0 {
		n := nPastes % batchSz
		_, err := execBatchQuery(m.db, insertPasteQueryBase, insertPasteQueryRepeat, valPerQuery, pasteV[:valPerQuery*n]...)
		if err != nil {
			m.Logger.Errorf("%d batch(%d) failed: %v", nPastes, n, err)
		}
	}
	return nPastes, nil
}

func (m *migrator) migrateUserPermissions(u *User, uid int64) (int, error) {
	insertUserPermissionQueryBase := `INSERT INTO
		user_paste_permissions(user_id, paste_id, permissions)
		VALUES`
	insertUserPermissionQueryRepeat := `(?, ?, ?)`
	valPerQuery := 3

	nUserPerms := 0
	batchSz := 999 / valPerQuery

	if userPerms, ok := u.Values["permissions"].(*PastePermissionSet); ok {
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

			permsV = append(permsV, uid)
			permsV = append(permsV, string(pid))
			permsV = append(permsV, newPerm)
			nCurrentPerms++
			if nCurrentPerms%batchSz == 0 {
				_, err := execBatchQuery(m.db, insertUserPermissionQueryBase, insertUserPermissionQueryRepeat, valPerQuery, permsV...)
				if err != nil {
					m.Logger.Errorf("%d batch(%d) failed: %v", nCurrentPerms, batchSz, err)
				}
				permsV = permsV[0:0]
			}
		}
		if nCurrentPerms%batchSz != 0 {
			n := nCurrentPerms % batchSz
			_, err := execBatchQuery(m.db, insertUserPermissionQueryBase, insertUserPermissionQueryRepeat, valPerQuery, permsV[:valPerQuery*n]...)
			if err != nil {
				m.Logger.Errorf("%d batch(%d) failed: %v", nCurrentPerms, n, err)
			}
		}
		nUserPerms += len(userPerms.Entries)
	}
	return nUserPerms, nil

}

func (m *migrator) MigrateUsers() (int, int, error) {
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
		res, err := insertUserQuery.Exec(time.Now(), u.Name, u.Salt, u.Challenge, source, perms)
		if err != nil {
			ulog.Error(err)
			return nil
		}
		sqlUid, err := res.LastInsertId()
		if err != nil {
			ulog.Error(err)
			return nil
		}

		nPermsMigrated, err := m.migrateUserPermissions(u, sqlUid)
		nUserPerms += nPermsMigrated
		nUsers++
		if nUsers%50 == 0 {
			m.Logger.Infof("%d...", nUsers)
		}
		return nil
	})
	return nUsers, nUserPerms, nil
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

	nPastes, err := m.MigratePastes()
	if err != nil {
		logger.Error(err)
	}
	logger.Infof("%d pastes.", nPastes)

	nUsers, nUserPermissions, err := m.MigrateUsers()
	if err != nil {
		logger.Error(err)
	}
	logger.Infof("%d users.", nUsers)
	logger.Infof("%d user permissions.", nUserPermissions)
}
