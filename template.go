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

func assetFunction(kind string, names ...string) template.HTML {
	var out string
	if kind == "js" {
		if *arguments.minified {
			return template.HTML("<script src=\"/js/all.min.js\"></script>")
		}

		for _, n := range names {
			out += "<script src=\"/js/" + n + ".js\"></script>"
		}
	} else if kind == "css" {
		if *arguments.minified {
			return template.HTML("<link rel=\"stylesheet\" href=\"/css/all.min.css\" type=\"text/css\" media=\"screen\">")
		}

		for _, n := range names {
			out += "<link rel=\"stylesheet\" href=\"/css/" + n + ".css\" type=\"text/css\" media=\"screen\">"
		}
	} else if kind == "less" {
		// Do not use less/less.js in production.
		if *arguments.minified {
			return template.HTML("")
		}

		for _, n := range names {
			out += "<link rel=\"stylesheet/less\" href=\"/css/" + n + ".less\" type=\"text/css\" media=\"screen\">"
		}
		out += "<script src=\"/js/less.js\" type=\"text/javascript\"></script>"
	}
	return template.HTML(out)
}

func InitTemplates() {
	glog.Info("Loading templates.")
	tmpl = func() *template.Template {
		return template.Must(template.New("base").Funcs(templateFunctions).ParseGlob("templates/*"))
	}
	if !*arguments.rebuild {
		glog.Info("Caching templates.")
		t := tmpl()
		tmpl = func() *template.Template {
			return t
		}
	}
}

func ExecuteTemplate(w io.Writer, name string, data interface{}) {
	tmpl().ExecuteTemplate(w, name, data)
}

func init() {
	RegisterTemplateFunction("equal", func(t1, t2 string) bool { return t1 == t2 })
	RegisterTemplateFunction("assets", assetFunction)

	RegisterReloadFunction(InitTemplates)
}
