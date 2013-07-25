package main

import (
	"html/template"
	"sort"
)

type Language struct {
	Title, Name, Formatter string
	Names                  []string
}

type LanguageList []*Language

func (l LanguageList) Len() int {
	return len(l)
}

func (l LanguageList) Less(i, j int) bool {
	return l[i].Title < l[j].Title
}

func (l LanguageList) Swap(i, j int) {
	l[i], l[j] = l[j], l[i]
}

var langMap map[string]*Language
var languages []*Language

func Languages() []*Language {
	return languages
}

func LanguageNamed(name string) *Language {
	v, ok := langMap[name]
	if !ok {
		return nil
	}
	return v
}

var languageOptionCache string

func LanguageOptionListHTML() template.HTML {
	if languageOptionCache != "" {
		return template.HTML(languageOptionCache)
	}

	var out string
	out += "<optgroup label=\"Common Languages\">"
	for _, l := range languageConfig.Common {
		out += "<option value=\"" + l.Name + "\">" + l.Title + "</option>"
	}
	out += "</optgroup><optgroup label=\"Other Languages\">"
	for _, l := range languageConfig.Other {
		out += "<option value=\"" + l.Name + "\">" + l.Title + "</option>"
	}
	out += "</optgroup>>"
	languageOptionCache = out
	return template.HTML(languageOptionCache)
}

var languageConfig struct {
	Common LanguageList
	Other  LanguageList
}

func init() {
	YAMLUnmarshalFile("languages.yml", &languageConfig)

	langMap = make(map[string]*Language)
	for _, v := range languageConfig.Common {
		langMap[v.Name] = v
		for _, langname := range v.Names {
			langMap[langname] = v
		}
	}

	for _, v := range languageConfig.Other {
		langMap[v.Name] = v
		for _, langname := range v.Names {
			langMap[langname] = v
		}
	}

	sort.Sort(languageConfig.Common)
	sort.Sort(languageConfig.Other)

	RegisterTemplateFunction("langs", Languages)
	RegisterTemplateFunction("langByLexer", LanguageNamed)
	RegisterTemplateFunction("languageOptionListHTML", LanguageOptionListHTML)
}
