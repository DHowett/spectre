package main

import (
	"github.com/golang/glog"
	"html/template"
	"io"
)

var templateFunctions template.FuncMap = template.FuncMap{}
var tmpl func() *template.Template

func RegisterTemplateFunction(name string, function interface{}) {
	templateFunctions[name] = function
}

func InitTemplates() {
	tmpl = func() *template.Template {
		return template.Must(template.New("base").Funcs(templateFunctions).ParseGlob("templates/*.html"))
	}
	if !arguments.rebuild {
		glog.Info("Caching templates.")
		t := tmpl()
		tmpl = func() *template.Template {
			return t
		}
	}
	glog.Info("Loaded templates.")
}

func ExecuteTemplate(w io.Writer, name string, data interface{}) {
	tmpl().ExecuteTemplate(w, name, data)
}

func init() {
	RegisterTemplateFunction("equal", func(t1, t2 string) bool { return t1 == t2 })

	RegisterReloadFunction(InitTemplates)
}
