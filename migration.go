package main

import (
	"encoding/gob"
	"net/http"

	"github.com/DHowett/ghostbin/lib/accounts"
	"github.com/DHowett/ghostbin/lib/pastes"
	"github.com/gorilla/sessions"
)

type PasteID string
type PastePermission map[string]bool
type PastePermissionSet struct {
	Entries map[PasteID]PastePermission
}

// migration support
// wrap all requests (after user lookup, so that we can merge into user)
// in a translation layer for legacy, v2 and v3 perms.

func getV3Perms(r *http.Request) (map[pastes.ID]accounts.Permission, bool) {
	cookieSession, _ := sessionStore.Get(r, "session")
	v, ok := cookieSession.Values["v3permissions"].(map[pastes.ID]accounts.Permission)
	return v, ok
}

func mergeLegacyPermsToV3(v3Perms map[pastes.ID]accounts.Permission, r *http.Request) bool {
	var hasV1, hasV2 bool
	cookieSession, _ := sessionStore.Get(r, "session")

	var legacyEntries map[PasteID]PastePermission
	// Attempt to get hold of the v2-style permission set.
	if sessionPermissionSet, hasV2 := cookieSession.Values["permissions"]; hasV2 {
		// presume user perms to have been migrated already. don't merge
		v2Perms := sessionPermissionSet.(*PastePermissionSet)
		legacyEntries = v2Perms.Entries
	}

	// Attempt to get hold of the original list of pastes
	if oldPasteList, hasV1 := cookieSession.Values["pastes"]; hasV1 {
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
			var newPerm accounts.Permission
			if pperm["grant"] {
				newPerm |= accounts.PastePermissionGrant
			}
			if pperm["edit"] {
				newPerm |= accounts.PastePermissionEdit
			}

			// legacy grant + edit = all future permissions
			if newPerm == accounts.PastePermissionGrant|accounts.PastePermissionEdit {
				newPerm = accounts.PastePermissionAll
			}

			v3Perms[pastes.IDFromString(string(pid))] = newPerm
		}
		return true
	}

	return false

}

func mergeV3PermsToUser(v3Perms map[pastes.ID]accounts.Permission, user accounts.User) bool {
	if len(v3Perms) > 0 && user != nil {
		for pid, pperm := range v3Perms {
			user.Permissions(accounts.PermissionClassPaste, pid).Grant(pperm)
		}
		return true
	}
	return false
}

type permissionMigrationWrapperHandler struct {
	http.Handler
}

func (h permissionMigrationWrapperHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cookieSession, _ := sessionStore.Get(r, "session")

	v3Perms, hasV3 := getV3Perms(r)
	if !hasV3 {
		v3Perms = make(map[pastes.ID]accounts.Permission)
	}

	merged := mergeLegacyPermsToV3(v3Perms, r)
	if merged {
		// Had v1 or v2 perms: delete them.
		delete(cookieSession.Values, "permissions")
		delete(cookieSession.Values, "pastes")
	}

	merged = mergeV3PermsToUser(v3Perms, GetUser(r))
	if hasV3 && merged {
		// Had a user, had v3 perms: delete them, they're on the user now.
		delete(cookieSession.Values, "v3permissions")
	}

	if !hasV3 && !merged {
		// Didn't have a user and didn't have v3 perms: store v3 perms.
		cookieSession.Values["v3permissions"] = v3Perms
	}

	sessions.Save(r, w)
	h.Handler.ServeHTTP(w, r)
}

func init() {
	gob.Register(&PastePermissionSet{})
	gob.Register(PastePermission{})
}
