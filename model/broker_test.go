package model

import (
	"database/sql"
	"flag"
	"os"
	"testing"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
)

type noopChallengeProvider struct{}

func (n *noopChallengeProvider) DeriveKey(string, []byte) []byte {
	return []byte{'a'}
}

func (n *noopChallengeProvider) RandomSalt() []byte {
	return []byte{'b'}
}

func (n *noopChallengeProvider) Challenge(message []byte, key []byte) []byte {
	return append(message, key...)
}

var gormDb *gorm.DB

var gStore Broker

func TestMain(m *testing.M) {
	flag.Parse()
	os.Remove("test.db")
	sqlDb, _ := sql.Open("sqlite3", "test.db")
	gStore, _ = NewDatabaseBroker("sqlite3", sqlDb, &noopChallengeProvider{})
	e := m.Run()
	os.Exit(e)
}

func TestStore(t *testing.T) {
	u, err := gStore.CreateUser("DHowett")
	if err != nil {
		t.Error(err)
		return
	}
	u.SetPersona(true)

	u, err = gStore.CreateUser("Timward")
	if err != nil {
		t.Error(err)
		return
	}
}

func TestGetName(t *testing.T) {
	u, err := gStore.GetUserNamed("DHowett")
	if err != nil {
		t.Error(err)
	}
	if u == nil || u.GetName() != "DHowett" {
		t.Error("Username doesn't match or user doesn't exist;", u)
	}
}

func TestGetID(t *testing.T) {
	u, err := gStore.GetUserByID(1)
	if err != nil {
		t.Error(err)
	}
	if u == nil || u.GetName() != "DHowett" {
		t.Error("Username doesn't match or user doesn't exist;", u)
	}
}

func TestGrantPermission(t *testing.T) {
	u, err := gStore.GetUserByID(1)
	if err != nil {
		t.Error(err)
	}

	err = u.Permissions(PermissionClassUser).Grant(UserPermissionAdmin)
	if err != nil {
		t.Error(err)
		return
	}

	if !u.Permissions(PermissionClassUser).Has(UserPermissionAdmin) {
		t.Fail()
	}
}

func TestRevokePermission(t *testing.T) {
	// permission was granted in the previous test.
	u, err := gStore.GetUserNamed("DHowett")
	if err != nil {
		t.Error(err)
	}

	if !u.Permissions(PermissionClassUser).Has(UserPermissionAdmin) {
		t.Error("user doesn't have admin permissions")
	}

	err = u.Permissions(PermissionClassUser).Revoke(UserPermissionAdmin)
	if err != nil {
		t.Error(err)
	}

	if u.Permissions(PermissionClassUser).Has(UserPermissionAdmin) {
		t.Error("user still has admin permissions")
	}
}

func TestPostRevocation(t *testing.T) {
	u, err := gStore.GetUserNamed("DHowett")
	if err != nil {
		t.Error(err)
	}

	if u.Permissions(PermissionClassUser).Has(UserPermissionAdmin) {
		t.Error("user still has admin permissions across reload")
	}
}

func TestUpdateChallenge(t *testing.T) {
	u, err := gStore.GetUserNamed("DHowett")
	if err != nil {
		t.Error(err)
	}

	u.UpdateChallenge("hello world")
	if !u.Check("hello world") {
		t.Fail()
	}
}

func TestGrantPastePermissions(t *testing.T) {
	u, err := gStore.GetUserNamed("DHowett")
	if err != nil {
		t.Error(err)
	}

	permScope := u.Permissions(PermissionClassPaste, "abcde")

	if permScope.Has(PastePermissionEdit) {
		t.Error("user already has edit on scope for abcde?")
	}

	err = permScope.Grant(PastePermissionEdit)
	if err != nil {
		t.Error(err)
	}

	err = permScope.Grant(PastePermissionGrant)
	if err != nil {
		t.Error(err)
	}

	if !permScope.Has(PastePermissionEdit) {
		t.Error("user can't edit abcde?")
	}

	if !permScope.Has(PastePermissionGrant) {
		t.Error("user can't grant abcde?")
	}
}

func TestRevokePastePermissions(t *testing.T) {
	u, err := gStore.GetUserNamed("DHowett")
	if err != nil {
		t.Error(err)
	}

	permScope := u.Permissions(PermissionClassPaste, "abcde")
	err = permScope.Revoke(PastePermissionEdit)
	if err != nil {
		t.Error(err)
	}

	err = permScope.Revoke(PastePermissionGrant)
	if err != nil {
		t.Error(err)
	}

	if permScope.Has(PastePermissionEdit) {
		t.Error("user still has edit on scope for abcde?")
	}

}

func TestGrantRevokeGrant(t *testing.T) {
	u, err := gStore.GetUserNamed("DHowett")
	if err != nil {
		t.Error(err)
	}

	_ = "breakpoint"
	permScope := u.Permissions(PermissionClassPaste, "grg")

	err = permScope.Grant(PastePermissionEdit)
	if err != nil {
		t.Error(err)
	}

	err = permScope.Revoke(PastePermissionAll)
	if err != nil {
		t.Error(err)
	}

	if permScope.Has(PastePermissionEdit) {
		t.Error("user still has edit on scope")
	}

	// this might trigger a reinsert/recreate in user_paste_permissions

	err = permScope.Grant(PastePermissionEdit)
	if err != nil {
		t.Error(err)
	}

	if !permScope.Has(PastePermissionEdit) {
		t.Error("user doesn't have edit on scope")
	}
}

func TestPostRevokePastePermissions(t *testing.T) {
	u, err := gStore.GetUserNamed("DHowett")
	if err != nil {
		t.Error(err)
	}

	permScope := u.Permissions(PermissionClassPaste, "abcde")

	if permScope.Has(PastePermissionEdit) {
		t.Error("user still has edit on scope for abcde?")
	}

}

func TestGetPastes(t *testing.T) {
	u, err := gStore.GetUserNamed("DHowett")
	if err != nil {
		t.Error(err)
	}
	u.Permissions(PermissionClassPaste, "12345").Grant(PastePermissionEdit)
	u.Permissions(PermissionClassPaste, "defgh").Grant(PastePermissionEdit)
	t.Log(u.GetPastes())
}
