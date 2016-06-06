package main

import (
	"net/http"

	"github.com/DHowett/ghostbin/lib/accounts"
	"github.com/DHowett/ghostbin/lib/pastes"
	"github.com/gorilla/sessions"
)

type globalPermissionScope struct {
	pID pastes.ID
	// attempts to merge session store with user perms
	// in the new unified scope interface.

	// User's paste perm scope for this ID
	uScope accounts.PermissionScope

	v3Entries map[pastes.ID]accounts.Permission
}

func (g *globalPermissionScope) Has(p accounts.Permission) bool {
	if g.uScope != nil {
		return g.uScope.Has(p)
	}
	return g.v3Entries[g.pID]&p == p
}

func (g *globalPermissionScope) Grant(p accounts.Permission) error {
	if g.uScope != nil {
		return g.uScope.Grant(p)
	}
	g.v3Entries[g.pID] = g.v3Entries[g.pID] | p
	return nil
}

func (g *globalPermissionScope) Revoke(p accounts.Permission) error {
	if g.uScope != nil {
		return g.uScope.Revoke(p)
	}
	g.v3Entries[g.pID] = g.v3Entries[g.pID] & (^p)
	if g.v3Entries[g.pID] == 0 {
		delete(g.v3Entries, g.pID)
	}
	return nil
}

func GetPastePermissionScope(pID pastes.ID, r *http.Request) accounts.PermissionScope {
	var userScope accounts.PermissionScope
	user := GetUser(r)
	if user != nil {
		userScope = user.Permissions(accounts.PermissionClassPaste, pID)
	}

	cookieSession, _ := sessionStore.Get(r, "session")
	v3EntriesI := cookieSession.Values["v3permissions"]
	v3Entries, ok := v3EntriesI.(map[pastes.ID]accounts.Permission)
	if !ok || v3Entries == nil {
		v3Entries = make(map[pastes.ID]accounts.Permission)
		cookieSession.Values["v3permissions"] = v3Entries
	}

	return &globalPermissionScope{
		pID:       pID,
		uScope:    userScope,
		v3Entries: v3Entries,
	}
}

func SavePastePermissionScope(w http.ResponseWriter, r *http.Request) {
	user := GetUser(r)
	if user == nil {
		sessions.Save(r, w)
	}
}
