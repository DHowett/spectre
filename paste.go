package main

import (
	//"fmt"
	"html/template"
	"strconv"
)

type Paste struct {
	ID           uint64
	Body         string
	RenderedBody template.HTML
}

func (p *Paste) URL() string {
	return "/paste/" + strconv.FormatUint(p.ID, 10)
}

func (p *Paste) Render() template.HTML {
	return template.HTML(p.Body)
}

type PasteNotFoundError struct {
	ID uint64
}

func (e PasteNotFoundError) Error() string {
	return "Paste " + strconv.FormatUint(e.ID, 10) + " was not found."
}

var pastes map[uint64]*Paste
var last_paste_id uint64

func NewPaste() *Paste {
	last_paste_id++
	id := last_paste_id
	p := &Paste{
		ID: id,
	}
	pastes[p.ID] = p
	return p
}

func GetPaste(id uint64) (p *Paste, err error) {
	p, exist := pastes[id]
	if !exist {
		err = PasteNotFoundError{ID: id}
	} else {
		err = nil
	}
	return
}
