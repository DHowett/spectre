package main

import (
	"github.com/DHowett/ghostbin/account"
	"github.com/gorilla/sessions"
	"net/http"
)

type PastePermission map[string]bool

type PastePermissionSet struct {
	Entries map[PasteID]PastePermission
	u       *account.User
}

func GetPastePermissions(r *http.Request) *PastePermissionSet {
	var perms *PastePermissionSet

	// Check if we have a user first.
	user := GetUser(r)
	if user != nil {
		if userPerms, ok := user.Values["permissions"]; ok {
			perms = userPerms.(*PastePermissionSet)
		}
	}

	cookieSession, _ := sessionStore.Get(r, "session")

	// Attempt to get hold of the new-style permission set.
	if sessionPermissionSet, ok := cookieSession.Values["permissions"]; ok {
		if perms != nil {
			for k, v := range sessionPermissionSet.(*PastePermissionSet).Entries {
				perms.Put(k, v)
			}
		} else {
			perms = sessionPermissionSet.(*PastePermissionSet)
		}
	}

	if perms == nil {
		perms = &PastePermissionSet{
			Entries: make(map[PasteID]PastePermission),
		}
	}

	// Attempt to get hold of the original list of pastes
	if oldPasteList, ok := cookieSession.Values["pastes"]; ok {
		for _, v := range oldPasteList.([]string) {
			perms.Put(PasteIDFromString(v), PastePermission{
				"grant": true,
				"edit":  true,
			})
		}
	}

	perms.u = user
	return perms
}

// Save emits the PastePermissionSet to disk, either as part of the anonymous
// session or as part of the authenticated user's data.
func (p *PastePermissionSet) Save(w http.ResponseWriter, r *http.Request) {
	if p.u != nil {
		p.u.Save()
	} else {
		cookieSession, _ := sessionStore.Get(r, "session")
		cookieSession.Values["permissions"] = p
		sessions.Save(r, w)
	}
}

// Put inserts a set of permissions into the permission store,
// potentially merging new permissions with existing permissions for the same paste.
func (p *PastePermissionSet) Put(id PasteID, perms PastePermission) {
	if existing, ok := p.Entries[id]; ok {
		for k, v := range perms {
			existing[k] = v
		}
	} else {
		p.Entries[id] = perms
	}
}

func (p *PastePermissionSet) Get(id PasteID) (PastePermission, bool) {
	v, ok := p.Entries[id]
	return v, ok
}

func (p *PastePermissionSet) Delete(id PasteID) {
	delete(p.Entries, id)
}
