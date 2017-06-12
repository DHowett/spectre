package postgres

import (
	"context"
	"testing"

	"howett.net/spectre"
)

func TestUserCreate(t *testing.T) {
	u, err := pqUserService.CreateUser(context.Background(), "DHowett")
	if err != nil {
		t.Fatal(err)
		return
	}
	u.SetSource(spectre.UserSourceMozillaPersona)

	u, err = pqUserService.CreateUser(context.Background(), "Timward")
	if err != nil {
		t.Fatal(err)
		return
	}
}

func TestUserGetByName(t *testing.T) {
	u, err := pqUserService.GetUserNamed(context.Background(), "DHowett")
	if err != nil {
		t.Fatal(err)
	}
	if u == nil || u.GetName() != "DHowett" {
		t.Error("Username doesn't match or user doesn't exist;", u)
	}
}

func TestUserGetByID(t *testing.T) {
	u, err := pqUserService.GetUserByID(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if u == nil || u.GetName() != "DHowett" {
		t.Error("Username doesn't match or user doesn't exist;", u)
	}
}

func TestUserGrantUserPermission(t *testing.T) {
	u, err := pqUserService.GetUserByID(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}

	err = u.Permissions(spectre.PermissionClassUser).Grant(spectre.UserPermissionAdmin)
	if err != nil {
		t.Fatal(err)
		return
	}

	if !u.Permissions(spectre.PermissionClassUser).Has(spectre.UserPermissionAdmin) {
		t.Fail()
	}
}

func TestUserRevokeUserPermission(t *testing.T) {
	// permission was granted in the previous test.
	u, err := pqUserService.GetUserNamed(context.Background(), "DHowett")
	if err != nil {
		t.Fatal(err)
	}

	if !u.Permissions(spectre.PermissionClassUser).Has(spectre.UserPermissionAdmin) {
		t.Error("user doesn't have admin permissions")
	}

	err = u.Permissions(spectre.PermissionClassUser).Revoke(spectre.UserPermissionAdmin)
	if err != nil {
		t.Fatal(err)
	}

	if u.Permissions(spectre.PermissionClassUser).Has(spectre.UserPermissionAdmin) {
		t.Error("user still has admin permissions")
	}

	u, err = pqUserService.GetUserNamed(context.Background(), "DHowett")
	if err != nil {
		t.Fatal(err)
	}

	if u.Permissions(spectre.PermissionClassUser).Has(spectre.UserPermissionAdmin) {
		t.Error("user still has admin permissions across reload")
	}
}

func TestUserUpdateChallenge(t *testing.T) {
	u, err := pqUserService.GetUserNamed(context.Background(), "DHowett")
	if err != nil {
		t.Fatal(err)
	}

	tc := &testCryptor{"userPassphrase"}
	u.UpdateChallenge(tc)
	if !u.Check(tc) {
		t.Fail()
	}
}

func TestUserGrantPastePermissions(t *testing.T) {
	// we must assume that the provider can create a paste safely.
	paste, err := pqPasteService.CreatePaste(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}

	u, err := pqUserService.GetUserNamed(context.Background(), "DHowett")
	if err != nil {
		t.Fatal(err)
	}

	permScope := u.Permissions(spectre.PermissionClassPaste, paste.GetID())

	if permScope.Has(spectre.PastePermissionEdit) {
		t.Error("user already has edit on scope for abcde?")
	}

	err = permScope.Grant(spectre.PastePermissionEdit)
	if err != nil {
		t.Fatal(err)
	}

	err = permScope.Grant(spectre.PastePermissionGrant)
	if err != nil {
		t.Fatal(err)
	}

	if !permScope.Has(spectre.PastePermissionEdit) {
		t.Error("user can't edit abcde?")
	}

	if !permScope.Has(spectre.PastePermissionGrant) {
		t.Error("user can't grant abcde?")
	}
}

func TestUserRevokePastePermissions(t *testing.T) {
	// we must assume that the provider can create a paste safely.
	paste, err := pqPasteService.CreatePaste(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}

	u, err := pqUserService.GetUserNamed(context.Background(), "DHowett")
	if err != nil {
		t.Fatal(err)
	}

	permScope := u.Permissions(spectre.PermissionClassPaste, paste.GetID())
	err = permScope.Revoke(spectre.PastePermissionEdit)
	if err != nil {
		t.Fatal(err)
	}

	err = permScope.Revoke(spectre.PastePermissionGrant)
	if err != nil {
		t.Fatal(err)
	}

	if permScope.Has(spectre.PastePermissionEdit) {
		t.Error("user still has edit on scope for abcde?")
	}

	// lookup anew
	u, err = pqUserService.GetUserNamed(context.Background(), "DHowett")
	if err != nil {
		t.Fatal(err)
	}

	permScope = u.Permissions(spectre.PermissionClassPaste, paste.GetID())

	if permScope.Has(spectre.PastePermissionEdit) {
		t.Error("user still has edit on scope for abcde?")
	}

}

func TestUserGrantRevokeGrant(t *testing.T) {
	// we must assume that the provider can create a paste safely.
	paste, err := pqPasteService.CreatePaste(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}

	u, err := pqUserService.GetUserNamed(context.Background(), "DHowett")
	if err != nil {
		t.Fatal(err)
	}

	permScope := u.Permissions(spectre.PermissionClassPaste, paste.GetID())

	err = permScope.Grant(spectre.PastePermissionEdit)
	if err != nil {
		t.Fatal(err)
	}

	err = permScope.Revoke(spectre.PastePermissionAll)
	if err != nil {
		t.Fatal(err)
	}

	if permScope.Has(spectre.PastePermissionEdit) {
		t.Error("user still has edit on scope")
	}

	// this might trigger a reinsert/recreate in user_paste_permissions

	err = permScope.Grant(spectre.PastePermissionEdit)
	if err != nil {
		t.Fatal(err)
	}

	if !permScope.Has(spectre.PastePermissionEdit) {
		t.Error("user doesn't have edit on scope")
	}
}

func TestUserGetPastes(t *testing.T) {
	// we must assume that the provider can create a paste safely.
	paste1, err := pqPasteService.CreatePaste(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}

	paste2, err := pqPasteService.CreatePaste(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}

	u, err := pqUserService.GetUserNamed(context.Background(), "DHowett")
	if err != nil {
		t.Fatal(err)
	}
	u.Permissions(spectre.PermissionClassPaste, paste1.GetID()).Grant(spectre.PastePermissionEdit)
	u.Permissions(spectre.PermissionClassPaste, paste2.GetID()).Grant(spectre.PastePermissionEdit)
	t.Log(u.GetPastes())
}
