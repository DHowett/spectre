package main

import (
	"html/template"
	"strconv"
)

type PasteID uint64
type Paste struct {
	ID           PasteID
	Body         string
	RenderedBody template.HTML
}

func (id PasteID) ToString() string {
	return strconv.FormatUint(uint64(id), 10)
}

func PasteIDFromString(s string) PasteID {
	id, _ := strconv.ParseUint(s, 10, 64)
	return PasteID(id)
}

func (p *Paste) URL() string {
	return "/paste/" + p.ID.ToString()
}

func (p *Paste) Render() template.HTML {
	return template.HTML(p.Body)
}

type PasteNotFoundError struct {
	ID PasteID
}

func (e PasteNotFoundError) Error() string {
	return "Paste " + e.ID.ToString() + " was not found."
}

var pastes map[PasteID]*Paste
var last_paste_id PasteID

func NewPaste() *Paste {
	last_paste_id++
	id := last_paste_id
	p := &Paste{
		ID: id,
	}
	pastes[p.ID] = p
	return p
}

func GetPaste(id PasteID) (p *Paste, err error) {
	p, exist := pastes[id]
	if !exist {
		err = PasteNotFoundError{ID: id}
	} else {
		err = nil
	}
	return
}

func init() {
	pastes = make(map[PasteID]*Paste)
}
