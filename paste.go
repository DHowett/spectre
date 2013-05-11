package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base32"
	"github.com/DHowett/go-xattr"
	"html/template"
	"io"
	"os"
	"strings"
)

type PasteID string
type Paste struct {
	ID           PasteID
	Body         string
	Language     string
	RenderedBody *string
	SourceIP     string
}

func (id PasteID) String() string {
	return string(id)
}

func PasteIDFromString(s string) PasteID {
	return PasteID(s)
}

func filenameForPasteID(id PasteID) string {
	return "pastes/" + id.String()
}

func (p *Paste) Filename() string {
	return filenameForPasteID(p.ID)
}

func (p *Paste) URL() string {
	return "/paste/" + p.ID.String()
}

func (p *Paste) MetadataKey() string {
	return "user.paste."
}

func (p *Paste) PutMetadata(name, value string) error {
	return xattr.Setxattr(p.Filename(), p.MetadataKey()+name, []byte(value), 0, 0)
}

func (p *Paste) GetMetadata(name string) (string, error) {
	bytes, err := xattr.Getxattr(p.Filename(), p.MetadataKey()+name, 0, 0)
	if err != nil {
		return "", err
	}

	return string(bytes), nil
}

func (p *Paste) GetMetadataWithDefault(name, dflt string) string {
	val, err := p.GetMetadata(name)
	if err != nil {
		return dflt
	}
	return val
}

func (p *Paste) Render() template.HTML {
	if p.Language == "text" {
		return template.HTML(template.HTMLEscapeString(p.Body))
	}

	if p.RenderedBody == nil {
		pygmentized, err := Pygmentize(&p.Body, p.Language)
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
	return "Paste " + e.ID.String() + " was not found."
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

func (p *Paste) Save() {
	writePasteToDisk(p)
}
func writePasteToDisk(p *Paste) {
	filename := p.Filename()
	file, err := os.Create(filename)
	if err != nil {
		panic(err)
	}
	sreader := strings.NewReader(p.Body)
	io.Copy(file, sreader)
	file.Close()

	if err := p.PutMetadata("language", p.Language); err != nil {
		panic(err)
	}

	if err := p.PutMetadata("source_ip", p.SourceIP); err != nil {
		panic(err)
	}
}

func loadPasteFromDisk(id PasteID) *Paste {
	filename := "pastes/" + id.String()
	file, err := os.Open(filename)
	if err != nil {
		panic(PasteNotFoundError{ID: id})
	}
	buf := bytes.Buffer{}
	io.Copy(&buf, file)
	file.Close()

	p := &Paste{}
	p.ID = id
	p.Body = buf.String()
	p.Language = p.GetMetadataWithDefault("language", "text")
	p.SourceIP = p.GetMetadataWithDefault("source_ip", "0.0.0.0")
	return p
}

func GetPaste(id PasteID) *Paste {
	p, exist := pastes[id]
	if !exist {
		p = loadPasteFromDisk(id)
		pastes[id] = p
		return p
	}
	return p
}

func (p *Paste) Destroy() {
	os.Remove(p.Filename())
	delete(pastes, p.ID)
}

func init() {
	pastes = make(map[PasteID]*Paste)
}
