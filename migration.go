package main

import (
	"encoding/gob"
	"net/http"

	"github.com/DHowett/ghostbin/model"
)

type PasteID string
type PastePermission map[string]bool
type PastePermissionSet struct {
	Entries map[PasteID]PastePermission
}

// migration support
// wrap all requests (after user lookup, so that we can merge into user)
// in a translation layer for legacy, v2 and v3 perms.

func getV3Perms(r *http.Request) (map[model.PasteID]model.Permission, bool) {
	session := sessionBroker.Get(r)
	v, ok := session.Get(SessionScopeServer, "v3permissions").(map[model.PasteID]model.Permission)
	return v, ok
}

func mergeLegacyPermsToV3(v3Perms map[model.PasteID]model.Permission, r *http.Request) bool {
	var hasV1, hasV2 bool
	session := sessionBroker.Get(r)

	var legacyEntries map[PasteID]PastePermission
	// Attempt to get hold of the v2-style permission set.
	if sessionPermissionSet, hasV2 := session.GetOk(SessionScopeServer, "permissions"); hasV2 {
		// presume user perms to have been migrated already. don't merge
		v2Perms := sessionPermissionSet.(*PastePermissionSet)
		legacyEntries = v2Perms.Entries
	}

	// Attempt to get hold of the original list of pastes
	if oldPasteList, hasV1 := session.GetOk(SessionScopeServer, "pastes"); hasV1 {
		if legacyEntries == nil {
			legacyEntries = make(map[PasteID]PastePermission)
		}

		for _, v := range oldPasteList.([]string) {
			id := PasteID(v)
			legacyEntries[id] = PastePermission{
				"grant": true,
				"edit":  true,
			}
		}
	}

	if (hasV1 || hasV2) && len(legacyEntries) > 0 {
		// update.
		for pid, pperm := range legacyEntries {
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

			v3Perms[model.PasteIDFromString(string(pid))] = newPerm
		}
		return true
	}

	return false

}

func mergeV3PermsToUser(v3Perms map[model.PasteID]model.Permission, user model.User) bool {
	if len(v3Perms) > 0 && user != nil {
		for pid, pperm := range v3Perms {
			user.Permissions(model.PermissionClassPaste, pid).Grant(pperm)
		}
		return true
	}
	return false
}

func MigrateLegacyPermissionsForRequest(w http.ResponseWriter, r *http.Request) {
	session := sessionBroker.Get(r)

	v3Perms, hasV3 := getV3Perms(r)
	if !hasV3 {
		v3Perms = make(map[model.PasteID]model.Permission)
	}

	merged := mergeLegacyPermsToV3(v3Perms, r)
	if merged {
		// Had v1 or v2 perms: delete them.
		session.Delete(SessionScopeServer, "permissions")
		session.Delete(SessionScopeServer, "pastes")
	}

	merged = mergeV3PermsToUser(v3Perms, GetUser(r))
	if hasV3 && merged {
		// Had a user, had v3 perms: delete them, they're on the user now.
		session.Delete(SessionScopeServer, "v3permissions")
	}

	if !hasV3 && !merged && len(v3Perms) != 0 {
		// Didn't have a user and didn't have v3 perms: store v3 perms.
		session.Set(SessionScopeServer, "v3permissions", v3Perms)
	}

	session.Save()
}

type permissionMigrationWrapperHandler struct {
	http.Handler
}

func (h permissionMigrationWrapperHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	MigrateLegacyPermissionsForRequest(w, r)
	h.Handler.ServeHTTP(w, r)
}

func init() {
	gob.Register(&PastePermissionSet{})
	gob.Register(PastePermission{})
}
