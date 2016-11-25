package main

import (
	"database/sql"
	"fmt"
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

func optionalString(s *string) string {
	if s == nil {
		return "NULL"
	}
	sv := strings.Replace(*s, `"`, `\"`, -1)
	return fmt.Sprintf(`"%s"`, sv)
}

func optionalHex(b []byte) string {
	if b == nil {
		return "NULL"
	}
	return fmt.Sprintf(`X'%02x'`, b)
}

func optionalUnixTime(t *time.Time) string {
	if t == nil {
		return "NULL"
	}
	return fmt.Sprintf(`DATETIME(%d, "unixepoch")`, t.Unix())
}

func main() {
	var opts options
	_, err := flags.Parse(&opts)
	if err != nil {
		logrus.Fatal(err)
	}

	pasteStore := NewFilesystemPasteStore(opts.Pastes)
	userStore := NewFilesystemUserStore(opts.Accounts)

	sqlDb, err := sql.Open(opts.Dialect, opts.Database)
	if err != nil {
		logrus.Fatal(err)
	}

	// Populate Schema(!)
	model.NewDatabaseBroker(opts.Dialect, sqlDb, nil)

	insertPasteQueryBase := `INSERT INTO
		pastes(id, created_at, updated_at, expire_at, title, language_name, hmac, encryption_salt, encryption_method)
		VALUES`
	insertPasteQueryRepeat := `(?, ?, ?, ?, ?, ?, ?, ?, ?)`

	insertUserQuery, err := sqlDb.Prepare(`INSERT INTO
		users(updated_at, name, salt, challenge, source, permissions)
		VALUES(?, ?, ?, ?, ?, ?)`)
	if err != nil {
		logrus.Fatal(err)
	}

	insertUserPermissionQuery, err := sqlDb.Prepare(`INSERT INTO
		user_paste_permissions(user_id, paste_id, permissions)
		VALUES(?, ?, ?)`)
	if err != nil {
		logrus.Fatal(err)
	}

	nPastes := 0
	batchSz := 100
	pasteQ := make([]string, 0, batchSz)
	pasteV := make([]interface{}, 0, batchSz*9)
	filepath.Walk(opts.Pastes, func(path string, fi os.FileInfo, err error) error {
		if fi.IsDir() {
			return nil
		}
		fsp, err := pasteStore.Get(model.PasteIDFromString(fi.Name()), nil)
		if err != nil {
			logrus.WithField("paste", fi.Name()).Error(err)
			return nil
		}
		pasteQ = append(pasteQ, insertPasteQueryRepeat)
		pasteV = append(pasteV, []interface{}{fsp.ID.String(), fsp.ModTime, fsp.ModTime, fsp.ExpirationTime, fsp.Title, fsp.Language, fsp.HMAC, fsp.EncryptionSalt, fsp.EncryptionMethod}...)
		//if err != nil {
		//logrus.WithField("paste", fi.Name()).Error(err)
		//return nil
		//}
		nPastes++
		if nPastes%batchSz == 0 {
			q := insertPasteQueryBase + strings.Join(pasteQ, ",")
			_, err := sqlDb.Exec(q, pasteV...)
			if err != nil {
				logrus.Errorf("%d batch(%d) failed: %v", nPastes, batchSz, err)
			}
			logrus.Infof("%d...", nPastes)
			pasteQ = pasteQ[0:0]
			pasteV = pasteV[0:0]
		}
		return nil
	})
	if nPastes%batchSz != 0 {
		n := nPastes % batchSz
		q := insertPasteQueryBase + strings.Join(pasteQ[:n], ",")
		_, err := sqlDb.Exec(q, pasteV[:9*n]...)
		if err != nil {
			logrus.Errorf("%d batch(%d) failed: %v", nPastes, n, err)
		}
	}
	logrus.Infof("%d pastes.", nPastes)

	userHits := map[string]bool{}
	nUsers := 0
	nUserPerms := 0
	filepath.Walk(opts.Accounts, func(path string, fi os.FileInfo, err error) error {
		if fi.IsDir() {
			return nil
		}
		u := userStore.Get(fi.Name())
		if u == nil {
			logrus.WithField("user", fi.Name()).Error("couldn't load?")
			return nil
		}
		if _, ok := userHits[u.Name]; !ok {
			userHits[u.Name] = true
		} else {
			logrus.WithField("user", u.Name).Warning("skipped duplicate")
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
			logrus.WithField("user", u.Name).Error(err)
			return nil
		}
		sqlUid, err := res.LastInsertId()
		if err != nil {
			logrus.WithField("user", u.Name).Error(err)
			return nil
		}

		if userPerms, ok := u.Values["permissions"].(*PastePermissionSet); ok {
			for pid, pperm := range userPerms.Entries {
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
				_, err := insertUserPermissionQuery.Exec(sqlUid, string(pid), newPerm)
				if err != nil {
					logrus.WithField("user", u.Name).WithField("paste", string(pid)).Error(err)
				}
			}
			nUserPerms += len(userPerms.Entries)
		}

		nUsers++
		if nUsers%50 == 0 {
			logrus.Infof("%d...", nUsers)
		}
		return nil
	})
	logrus.Infof("%d users.", nUsers)
	logrus.Infof("%d user permissions.", nUserPerms)
}
