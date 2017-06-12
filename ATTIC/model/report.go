package model

type Report interface {
	GetPasteID() PasteID
	GetCount() int
	Destroy() error
}
