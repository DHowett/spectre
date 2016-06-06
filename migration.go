package main

import (
	"encoding/gob"
	"net/http"

	"github.com/DHowett/ghostbin/lib/accounts"
	"github.com/DHowett/ghostbin/lib/pastes"
	"github.com/golang/glog"
	"github.com/gorilla/sessions"
)

// legacy migration support
// wrap all requests (after user lookup, so that we can merge into user)
// in a translation layer for legacy and v2 perms.

type PasteID string
type PastePermission map[string]bool
type PastePermissionSet struct {
	Entries map[PasteID]PastePermission
}

type legacyPermWrapperHandler struct {
	http.Handler
}

func (h legacyPermWrapperHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cookieSession, _ := sessionStore.Get(r, "session")
	var legacyEntries map[PasteID]PastePermission
	// Attempt to get hold of the v2-style permission set.
	// TODO(DH): "PasteID" might be required for legacy perms :|
	if sessionPermissionSet, ok := cookieSession.Values["permissions"]; ok {
		// presume user perms to have been migrated already. don't merge
		v2Perms := sessionPermissionSet.(*PastePermissionSet)
		legacyEntries = v2Perms.Entries
	}

	if legacyEntries == nil {
		legacyEntries = make(map[PasteID]PastePermission)
	}

	// Attempt to get hold of the original list of pastes
	if oldPasteList, ok := cookieSession.Values["pastes"]; ok {
		for _, v := range oldPasteList.([]string) {
			id := PasteID(v)
			legacyEntries[id] = PastePermission{
				"grant": true,
				"edit":  true,
			}
		}
	}

	v3EntriesI := cookieSession.Values["v3permissions"]
	v3Perms, ok := v3EntriesI.(map[pastes.ID]accounts.Permission)
	if !ok || v3Perms == nil {
		v3Perms = make(map[pastes.ID]accounts.Permission)
	}
	if len(legacyEntries) > 0 {
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
			glog.Infof("Migrating %s as perm %x, used to be %v", pid, newPerm, pperm)
		}

		delete(cookieSession.Values, "permissions")
		delete(cookieSession.Values, "pastes")
	}

	_ = "breakpoint"
	if len(v3Perms) > 0 {
		user := GetUser(r)
		if user != nil {
			for pid, pperm := range v3Perms {
				glog.Infof("Granting user %d perm to %v %x", user.GetID(), pid, pperm)
				user.Permissions(accounts.PermissionClassPaste, pid).Grant(pperm)
			}
			delete(cookieSession.Values, "v3permissions")
		} else {
			cookieSession.Values["v3permissions"] = v3Perms
		}
		sessions.Save(r, w)
	}

	h.Handler.ServeHTTP(w, r)

}

func init() {
	gob.Register(&PastePermissionSet{})
	gob.Register(PastePermission{})
}
