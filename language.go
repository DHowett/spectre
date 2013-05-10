package main

var langMap map[string]*Language

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

func init() {
	langMap = make(map[string]*Language)
	for i, v := range languages {
		langMap[v.Lexer] = &languages[i]
	}

	RegisterTemplateFunction("langs", Languages)
	RegisterTemplateFunction("langByLexer", LanguageByLexer)
}
