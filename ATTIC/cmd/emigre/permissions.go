package main

import "encoding/gob"

// compat.
type PasteID string
type PastePermission map[string]bool

type PastePermissionSet struct {
	Entries map[PasteID]PastePermission
}

func init() {
	gob.Register(map[PasteID][]byte(nil))
	gob.Register(&PastePermissionSet{})
	gob.Register(PastePermission{})

}
