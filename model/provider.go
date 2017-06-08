package model

import "context"

type Provider interface {
	// User Management
	GetUserNamed(context.Context, string) (User, error)
	GetUserByID(context.Context, uint) (User, error)
	CreateUser(context.Context, string) (User, error)

	// Pastes
	CreatePaste(context.Context) (Paste, error)
	CreateEncryptedPaste(context.Context, PasteEncryptionMethod, []byte) (Paste, error)
	GetPaste(context.Context, PasteID, []byte) (Paste, error)
	GetPastes(context.Context, []PasteID) ([]Paste, error)
	DestroyPaste(context.Context, PasteID) error

	// Grants
	CreateGrant(context.Context, Paste) (Grant, error)
	GetGrant(context.Context, GrantID) (Grant, error)
	//DestroyGrant(GrantID)

	// Reports
	ReportPaste(context.Context, Paste) error
	GetReport(context.Context, PasteID) (Report, error)
	GetReports(context.Context) ([]Report, error)
}
