package main

import (
	"github.com/DHowett/ghostbin/lib/formatting"
	"github.com/DHowett/ghostbin/model"
)

func FormatPaste(p model.Paste) (string, error) {
	reader, _ := p.Reader()
	defer reader.Close()
	return formatting.FormatStream(reader, formatting.LanguageNamed(p.GetLanguageName()))
}
