package main

type Language struct {
	Title, Name, Formatter string
	Names                  []string
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

func init() {
	YAMLUnmarshalFile("languages.yml", &languages)

	langMap = make(map[string]*Language)
	for _, v := range languages {
		langMap[v.Name] = v
		for _, langname := range v.Names {
			langMap[langname] = v
		}
	}

	RegisterTemplateFunction("langs", Languages)
	RegisterTemplateFunction("langByLexer", LanguageNamed)
}
