package main

import (
	"net/http"

	"github.com/DHowett/ghostbin/model"
)

type globalPermissionScope struct {
	pID model.PasteID
	// attempts to merge session store with user perms
	// in the new unified scope interface.

	// User's paste perm scope for this ID
	uScope model.PermissionScope

	session *ScopedSession

	v3Entries map[model.PasteID]model.Permission
}

func (g *globalPermissionScope) Has(p model.Permission) bool {
	if g.uScope != nil {
		return g.uScope.Has(p)
	}
	return g.v3Entries[g.pID]&p == p
}

func (g *globalPermissionScope) Grant(p model.Permission) error {
	if g.uScope != nil {
		return g.uScope.Grant(p)
	}
	g.v3Entries[g.pID] = g.v3Entries[g.pID] | p
	g.session.MarkDirty()
	return nil
}

func (g *globalPermissionScope) Revoke(p model.Permission) error {
	if g.uScope != nil {
		return g.uScope.Revoke(p)
	}
	g.v3Entries[g.pID] = g.v3Entries[g.pID] & (^p)
	if g.v3Entries[g.pID] == 0 {
		delete(g.v3Entries, g.pID)
	}
	g.session.MarkDirty()
	return nil
}

func GetPastePermissionScope(pID model.PasteID, r *http.Request) model.PermissionScope {
	var userScope model.PermissionScope
	user := GetLoggedInUser(r)
	if user != nil {
		userScope = user.Permissions(model.PermissionClassPaste, pID)
	}

	session := sessionBroker.Get(r).Scope(SessionScopeServer)
	v3EntriesI := session.Get("v3permissions")
	v3Entries, ok := v3EntriesI.(map[model.PasteID]model.Permission)
	if !ok || v3Entries == nil {
		v3Entries = make(map[model.PasteID]model.Permission)
		session.Set("v3permissions", v3Entries)
	}

	return &globalPermissionScope{
		pID:       pID,
		uScope:    userScope,
		session:   session,
		v3Entries: v3Entries,
	}
}

func SavePastePermissionScope(w http.ResponseWriter, r *http.Request) {
	user := GetLoggedInUser(r)
	if user == nil {
		session := sessionBroker.Get(r)
		session.Save()
	}
}
