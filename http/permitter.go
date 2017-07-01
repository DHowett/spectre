package http

import (
	"errors"

	"howett.net/spectre"
)

var (
	errPermissionNotApplicable = errors.New("permission not applicable")
)

type nullPermissionScope struct{}

func (nullPermissionScope) Has(spectre.Permission) bool {
	return false
}

func (nullPermissionScope) Grant(spectre.Permission) error {
	return errPermissionNotApplicable
}

func (nullPermissionScope) Revoke(spectre.Permission) error {
	return errPermissionNotApplicable
}

type requestPermitter struct {
	u spectre.User
	s Session
}

func (p *requestPermitter) Permissions(class spectre.PermissionClass, args ...interface{}) spectre.PermissionScope {
	if p.u != nil {
		return p.u.Permissions(class, args...)
	}

	if class != spectre.PermissionClassUser && len(args) >= 1 {
		if pID, ok := args[0].(spectre.PasteID); ok {
			return newSessionPastePermissionScope(pID, p.s)
		}
	}
	return nullPermissionScope{}
}

type sessionPastePermissionScope struct {
	id        spectre.PasteID
	s         Session
	v3Entries map[spectre.PasteID]spectre.Permission
}

func (g *sessionPastePermissionScope) Has(p spectre.Permission) bool {
	return g.v3Entries[g.id]&p == p
}

func (g *sessionPastePermissionScope) Grant(p spectre.Permission) error {
	g.v3Entries[g.id] = g.v3Entries[g.id] | p
	g.s.MarkDirty(SessionScopeServer)
	return nil
}

func (g *sessionPastePermissionScope) Revoke(p spectre.Permission) error {
	g.v3Entries[g.id] = g.v3Entries[g.id] & (^p)
	if g.v3Entries[g.id] == 0 {
		delete(g.v3Entries, g.id)
	}
	g.s.MarkDirty(SessionScopeServer)
	return nil
}

func newSessionPastePermissionScope(pID spectre.PasteID, session Session) spectre.PermissionScope {
	v3EntriesI := session.Get(SessionScopeServer, "v3permissions")
	v3Entries, ok := v3EntriesI.(map[spectre.PasteID]spectre.Permission)
	if !ok || v3Entries == nil {
		v3Entries = make(map[spectre.PasteID]spectre.Permission)
		session.Set(SessionScopeServer, "v3permissions", v3Entries)
	}

	return &sessionPastePermissionScope{
		id:        pID,
		s:         session,
		v3Entries: v3Entries,
	}
}
