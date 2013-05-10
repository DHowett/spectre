package main

import (
	"html/template"
	"io"
	"path/filepath"
)

var templateFunctions template.FuncMap = template.FuncMap{}
var tmpl func() *template.Template

func RegisterTemplateFunction(name string, function interface{}) {
	templateFunctions[name] = function
}

func InitTemplates(rebuild bool) {
	RegisterTemplateFunction("equal", func(t1, t2 string) bool { return t1 == t2 })

	tmpl = func() *template.Template {
		files, err := filepath.Glob("templates/*")
		if err != nil {
			panic(err)
		}
		return template.Must(template.New("base").Funcs(templateFunctions).ParseFiles(files...))
	}
	if !rebuild {
		t := tmpl()
		tmpl = func() *template.Template {
			return t
		}
	}
}

func ExecuteTemplate(w io.Writer, name string, data interface{}) {
	tmpl().ExecuteTemplate(w, name, data)
}
