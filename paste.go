package main

import (
	"crypto/rand"
	"encoding/base32"
	"html/template"
)

type PasteID string
type Paste struct {
	ID           PasteID
	Body         string
	RenderedBody *string
}

func (id PasteID) ToString() string {
	return string(id)
}

func PasteIDFromString(s string) PasteID {
	return PasteID(s)
}

func (p *Paste) URL() string {
	return "/paste/" + p.ID.ToString()
}

func (p *Paste) Render() template.HTML {
	if p.RenderedBody == nil {
		pygmentized, err := Pygmentize(p.Body)
		if err != nil {
			return template.HTML("There was an error rendering this paste.")
		}

		p.RenderedBody = &pygmentized
	}
	return template.HTML(*p.RenderedBody)
}

type PasteNotFoundError struct {
	ID PasteID
}

func (e PasteNotFoundError) Error() string {
	return "Paste " + e.ID.ToString() + " was not found."
}

var pastes map[PasteID]*Paste

func genPasteID() (PasteID, error) {
	uuid := make([]byte, 3)
	n, err := rand.Read(uuid)
	if n != len(uuid) || err != nil {
		return "", err
	}

	return PasteIDFromString(base32.NewEncoding("abcdefghijklmnopqrstuvwxyz1234567").EncodeToString(uuid)[0:5]), nil
}

func NewPaste() *Paste {
	id, err := genPasteID()
	if err != nil {
		panic(err)
	}

	p := &Paste{
		ID: id,
	}
	pastes[p.ID] = p
	return p
}

func GetPaste(id PasteID) *Paste {
	p, exist := pastes[id]
	if !exist {
		panic(PasteNotFoundError{ID: id})
	}
	return p
}

func init() {
	pastes = make(map[PasteID]*Paste)
}
