package main

import (
	"fmt"
	"github.com/golang/glog"
	"html/template"
	"io"
	"time"
)

var templateFunctions template.FuncMap = template.FuncMap{}
var tmpl func() *template.Template

func RegisterTemplateFunction(name string, function interface{}) {
	templateFunctions[name] = function
}

var cacheBustingNonce int64

func assetFunction(kind string, names ...string) template.HTML {
	var out string
	if kind == "js" {
		if Env() == EnvironmentProduction {
			names = []string{"all.min"}
		}

		for _, n := range names {
			out += fmt.Sprintf("<script src=\"/js/%s.js?%d\"></script>", n, cacheBustingNonce)
		}
	} else if kind == "css" {
		if Env() == EnvironmentProduction {
			names = []string{"all.min"}
		}

		for _, n := range names {
			out += fmt.Sprintf("<link rel=\"stylesheet\" href=\"/css/%s.css?%d\" type=\"text/css\" media=\"screen\">", n, cacheBustingNonce)
		}
	} else if kind == "less" {
		// Do not use less/less.js in production.
		if Env() == EnvironmentProduction {
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
	cacheBustingNonce = time.Now().Unix()
	tmpl = func() *template.Template {
		return template.Must(template.New("base").Funcs(templateFunctions).ParseGlob("templates/*"))
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
	RegisterTemplateFunction("assets", assetFunction)

	RegisterReloadFunction(InitTemplates)
}
