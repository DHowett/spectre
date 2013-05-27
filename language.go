package main

import (
	"bytes"
	"html/template"
	"io"
)

var langMap map[string]*Language
var langRenderers map[string]LanguageRenderer

type LanguageRenderer interface {
	Render(io.Reader, string) (string, error)
}

type LanguageRenderFunc func(io.Reader, string) (string, error)

func (fn LanguageRenderFunc) Render(stream io.Reader, language string) (string, error) {
	return fn(stream, language)
}

type Language struct {
	Lexer, Title string
}

var languages []Language = []Language{
	{"text", "Plain Text"},
	{"logos", "Logos + Objective-C"},
	{"objective-c", "Objective-C"},
	{"c", "C"},
	{"c++", "C++"},
	{"irc", "IRC Log"},
	{"perl", "Perl"},
	{"go", "Go"},
	{"html", "HTML"},
	{"ansi", "ANSI"},
}

func Languages() []Language {
	return languages
}

func LanguageByLexer(name string) *Language {
	v, ok := langMap[name]
	if !ok {
		return nil
	}
	return v
}

func RenderForLanguage(stream io.Reader, language string) (string, error) {
	var renderer LanguageRenderer
	var ok bool
	if renderer, ok = langRenderers[language]; !ok {
		renderer = langRenderers["_default"]
	}

	return renderer.Render(stream, language)
}

func init() {
	langMap = make(map[string]*Language)
	for i, v := range languages {
		langMap[v.Lexer] = &languages[i]
	}

	RegisterTemplateFunction("langs", Languages)
	RegisterTemplateFunction("langByLexer", LanguageByLexer)

	langRenderers = make(map[string]LanguageRenderer)
	langRenderers["text"] = LanguageRenderFunc(func(stream io.Reader, language string) (string, error) {
		buf := &bytes.Buffer{}
		io.Copy(buf, stream)
		return template.HTMLEscapeString(buf.String()), nil
	})

	langRenderers["ansi"] = LanguageRenderFunc(func(stream io.Reader, language string) (string, error) {
		return ANSI(stream)
	})

	langRenderers["_default"] = LanguageRenderFunc(Pygmentize)
}
