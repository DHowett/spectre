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

func _initLanguages() error {
	return formatting.LoadLanguageConfig("languages.yml")
}

func init() {
	globalInit.Add(&InitHandler{
		Priority: 15,
		Name:     "languages",
		Do: func() error {
			templatePack.AddFunction("langByLexer", formatting.LanguageNamed)
			return _initLanguages()
		},
		Redo: _initLanguages,
	})
}
