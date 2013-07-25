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
	for _, group := range languageGroups {
		out += "<optgroup label=\"" + group.Title + "\">"
		for _, l := range group.Languages {
			out += "<option value=\"" + l.Name + "\">" + l.Title + "</option>"
		}
		out += "</optgroup>"
	}
	languageOptionCache = out
	return template.HTML(languageOptionCache)
}

var languageGroups []*struct {
	Title     string
	Languages LanguageList
}

func init() {
	err := YAMLUnmarshalFile("languages.yml", &languageGroups)
	if err != nil {
		panic(err)
	}

	langMap = make(map[string]*Language)
	for _, g := range languageGroups {
		for _, v := range g.Languages {
			langMap[v.Name] = v
			for _, langname := range v.Names {
				langMap[langname] = v
			}
		}
		sort.Sort(g.Languages)
	}

	RegisterTemplateFunction("langs", Languages)
	RegisterTemplateFunction("langByLexer", LanguageNamed)
	RegisterTemplateFunction("languageOptionListHTML", LanguageOptionListHTML)
}
