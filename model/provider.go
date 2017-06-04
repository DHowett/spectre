package model

type Provider interface {
	// User Management
	GetUserNamed(name string) (User, error)
	GetUserByID(id uint) (User, error)
	CreateUser(name string) (User, error)

	// Pastes
	CreatePaste() (Paste, error)
	CreateEncryptedPaste(PasteEncryptionMethod, []byte) (Paste, error)
	GetPaste(PasteID, []byte) (Paste, error)
	GetPastes([]PasteID) ([]Paste, error)
	DestroyPaste(PasteID) error

	// Grants
	CreateGrant(Paste) (Grant, error)
	GetGrant(GrantID) (Grant, error)
	//DestroyGrant(GrantID)

	// Reports
	ReportPaste(p Paste) error
	GetReport(PasteID) (Report, error)
	GetReports() ([]Report, error)
}
