package main

import (
	"html/template"
)

var langMap map[string]*Language
var langRenderers map[string]LanguageRenderer

type LanguageRenderer interface {
	Render(*string, string) (string, error)
}

type LanguageRenderFunc func(*string, string) (string, error)

func (fn LanguageRenderFunc) Render(text *string, language string) (string, error) {
	return fn(text, language)
}

type Language struct {
	Lexer, Title string
}

var languages []Language = []Language{
	{"_auto", "Automatically Detect"},
	{"text", "Plain Text"},
	{"logos", "Logos + Objective-C"},
	{"objective-c", "Objective-C"},
	{"c", "C"},
	{"irc", "IRC Log"},
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

func RenderForLanguage(text *string, language string) (string, error) {
	var renderer LanguageRenderer
	var ok bool
	if renderer, ok = langRenderers[language]; !ok {
		renderer = langRenderers["_default"]
	}

	return renderer.Render(text, language)
}

func init() {
	langMap = make(map[string]*Language)
	for i, v := range languages {
		langMap[v.Lexer] = &languages[i]
	}

	RegisterTemplateFunction("langs", Languages)
	RegisterTemplateFunction("langByLexer", LanguageByLexer)

	langRenderers = make(map[string]LanguageRenderer)
	langRenderers["text"] = LanguageRenderFunc(func(text *string, language string) (string, error) {
		return template.HTMLEscapeString(*text), nil
	})

	langRenderers["_default"] = LanguageRenderFunc(Pygmentize)
}
