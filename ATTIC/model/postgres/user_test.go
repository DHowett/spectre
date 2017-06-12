package postgres

import (
	"context"
	"testing"

	"github.com/DHowett/ghostbin/model"
)

func TestUserCreate(t *testing.T) {
	u, err := gTestProvider.CreateUser(context.Background(), "DHowett")
	if err != nil {
		t.Fatal(err)
		return
	}
	u.SetSource(model.UserSourceMozillaPersona)

	u, err = gTestProvider.CreateUser(context.Background(), "Timward")
	if err != nil {
		t.Fatal(err)
		return
	}
}

func TestUserGetByName(t *testing.T) {
	u, err := gTestProvider.GetUserNamed(context.Background(), "DHowett")
	if err != nil {
		t.Fatal(err)
	}
	if u == nil || u.GetName() != "DHowett" {
		t.Error("Username doesn't match or user doesn't exist;", u)
	}
}

func TestUserGetByID(t *testing.T) {
	u, err := gTestProvider.GetUserByID(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if u == nil || u.GetName() != "DHowett" {
		t.Error("Username doesn't match or user doesn't exist;", u)
	}
}

func TestUserGrantUserPermission(t *testing.T) {
	u, err := gTestProvider.GetUserByID(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}

	err = u.Permissions(model.PermissionClassUser).Grant(model.UserPermissionAdmin)
	if err != nil {
		t.Fatal(err)
		return
	}

	if !u.Permissions(model.PermissionClassUser).Has(model.UserPermissionAdmin) {
		t.Fail()
	}
}

func TestUserRevokeUserPermission(t *testing.T) {
	// permission was granted in the previous test.
	u, err := gTestProvider.GetUserNamed(context.Background(), "DHowett")
	if err != nil {
		t.Fatal(err)
	}

	if !u.Permissions(model.PermissionClassUser).Has(model.UserPermissionAdmin) {
		t.Error("user doesn't have admin permissions")
	}

	err = u.Permissions(model.PermissionClassUser).Revoke(model.UserPermissionAdmin)
	if err != nil {
		t.Fatal(err)
	}

	if u.Permissions(model.PermissionClassUser).Has(model.UserPermissionAdmin) {
		t.Error("user still has admin permissions")
	}

	u, err = gTestProvider.GetUserNamed(context.Background(), "DHowett")
	if err != nil {
		t.Fatal(err)
	}

	if u.Permissions(model.PermissionClassUser).Has(model.UserPermissionAdmin) {
		t.Error("user still has admin permissions across reload")
	}
}

func TestUserUpdateChallenge(t *testing.T) {
	u, err := gTestProvider.GetUserNamed(context.Background(), "DHowett")
	if err != nil {
		t.Fatal(err)
	}

	u.UpdateChallenge("hello world")
	if !u.Check("hello world") {
		t.Fail()
	}
}

func TestUserGrantPastePermissions(t *testing.T) {
	// we must assume that the provider can create a paste safely.
	paste, err := gTestProvider.CreatePaste(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	u, err := gTestProvider.GetUserNamed(context.Background(), "DHowett")
	if err != nil {
		t.Fatal(err)
	}

	permScope := u.Permissions(model.PermissionClassPaste, paste.GetID())

	if permScope.Has(model.PastePermissionEdit) {
		t.Error("user already has edit on scope for abcde?")
	}

	err = permScope.Grant(model.PastePermissionEdit)
	if err != nil {
		t.Fatal(err)
	}

	err = permScope.Grant(model.PastePermissionGrant)
	if err != nil {
		t.Fatal(err)
	}

	if !permScope.Has(model.PastePermissionEdit) {
		t.Error("user can't edit abcde?")
	}

	if !permScope.Has(model.PastePermissionGrant) {
		t.Error("user can't grant abcde?")
	}
}

func TestUserRevokePastePermissions(t *testing.T) {
	// we must assume that the provider can create a paste safely.
	paste, err := gTestProvider.CreatePaste(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	u, err := gTestProvider.GetUserNamed(context.Background(), "DHowett")
	if err != nil {
		t.Fatal(err)
	}

	permScope := u.Permissions(model.PermissionClassPaste, paste.GetID())
	err = permScope.Revoke(model.PastePermissionEdit)
	if err != nil {
		t.Fatal(err)
	}

	err = permScope.Revoke(model.PastePermissionGrant)
	if err != nil {
		t.Fatal(err)
	}

	if permScope.Has(model.PastePermissionEdit) {
		t.Error("user still has edit on scope for abcde?")
	}

	// lookup anew
	u, err = gTestProvider.GetUserNamed(context.Background(), "DHowett")
	if err != nil {
		t.Fatal(err)
	}

	permScope = u.Permissions(model.PermissionClassPaste, paste.GetID())

	if permScope.Has(model.PastePermissionEdit) {
		t.Error("user still has edit on scope for abcde?")
	}

}

func TestUserGrantRevokeGrant(t *testing.T) {
	// we must assume that the provider can create a paste safely.
	paste, err := gTestProvider.CreatePaste(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	u, err := gTestProvider.GetUserNamed(context.Background(), "DHowett")
	if err != nil {
		t.Fatal(err)
	}

	permScope := u.Permissions(model.PermissionClassPaste, paste.GetID())

	err = permScope.Grant(model.PastePermissionEdit)
	if err != nil {
		t.Fatal(err)
	}

	err = permScope.Revoke(model.PastePermissionAll)
	if err != nil {
		t.Fatal(err)
	}

	if permScope.Has(model.PastePermissionEdit) {
		t.Error("user still has edit on scope")
	}

	// this might trigger a reinsert/recreate in user_paste_permissions

	err = permScope.Grant(model.PastePermissionEdit)
	if err != nil {
		t.Fatal(err)
	}

	if !permScope.Has(model.PastePermissionEdit) {
		t.Error("user doesn't have edit on scope")
	}
}

func TestUserGetPastes(t *testing.T) {
	// we must assume that the provider can create a paste safely.
	paste1, err := gTestProvider.CreatePaste(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	paste2, err := gTestProvider.CreatePaste(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	u, err := gTestProvider.GetUserNamed(context.Background(), "DHowett")
	if err != nil {
		t.Fatal(err)
	}
	u.Permissions(model.PermissionClassPaste, paste1.GetID()).Grant(model.PastePermissionEdit)
	u.Permissions(model.PermissionClassPaste, paste2.GetID()).Grant(model.PastePermissionEdit)
	t.Log(u.GetPastes())
}
