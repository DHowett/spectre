package model

import (
	"testing"
)

func TestUserCreate(t *testing.T) {
	u, err := broker.CreateUser("DHowett")
	if err != nil {
		t.Error(err)
		return
	}
	u.SetSource(UserSourceMozillaPersona)

	u, err = broker.CreateUser("Timward")
	if err != nil {
		t.Error(err)
		return
	}
}

func TestUserGetByName(t *testing.T) {
	u, err := broker.GetUserNamed("DHowett")
	if err != nil {
		t.Error(err)
	}
	if u == nil || u.GetName() != "DHowett" {
		t.Error("Username doesn't match or user doesn't exist;", u)
	}
}

func TestUserGetByID(t *testing.T) {
	u, err := broker.GetUserByID(1)
	if err != nil {
		t.Error(err)
	}
	if u == nil || u.GetName() != "DHowett" {
		t.Error("Username doesn't match or user doesn't exist;", u)
	}
}

func TestUserGrantUserPermission(t *testing.T) {
	u, err := broker.GetUserByID(1)
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

func TestUserRevokeUserPermission(t *testing.T) {
	// permission was granted in the previous test.
	u, err := broker.GetUserNamed("DHowett")
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

	u, err = broker.GetUserNamed("DHowett")
	if err != nil {
		t.Error(err)
	}

	if u.Permissions(PermissionClassUser).Has(UserPermissionAdmin) {
		t.Error("user still has admin permissions across reload")
	}
}

func TestUserUpdateChallenge(t *testing.T) {
	u, err := broker.GetUserNamed("DHowett")
	if err != nil {
		t.Error(err)
	}

	u.UpdateChallenge("hello world")
	if !u.Check("hello world") {
		t.Fail()
	}
}

func TestUserGrantPastePermissions(t *testing.T) {
	u, err := broker.GetUserNamed("DHowett")
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

func TestUserRevokePastePermissions(t *testing.T) {
	u, err := broker.GetUserNamed("DHowett")
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

	// lookup anew
	u, err = broker.GetUserNamed("DHowett")
	if err != nil {
		t.Error(err)
	}

	permScope = u.Permissions(PermissionClassPaste, "abcde")

	if permScope.Has(PastePermissionEdit) {
		t.Error("user still has edit on scope for abcde?")
	}

}

func TestUserGrantRevokeGrant(t *testing.T) {
	u, err := broker.GetUserNamed("DHowett")
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

func TestUserGetPastes(t *testing.T) {
	u, err := broker.GetUserNamed("DHowett")
	if err != nil {
		t.Error(err)
	}
	u.Permissions(PermissionClassPaste, "12345").Grant(PastePermissionEdit)
	u.Permissions(PermissionClassPaste, "defgh").Grant(PastePermissionEdit)
	t.Log(u.GetPastes())
}
