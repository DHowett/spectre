package templatepack

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"time"
)

// A Context is exposed to a rendered template on {{.}}.
type Context struct {
	Obj      interface{}
	Request  *http.Request
	Page     string
	template *template.Template
}

// A Pack represents a set of connected templates loaded from files specified by a shell glob.
type Pack struct {
	funcs        template.FuncMap
	templateRoot *template.Template

	glob string
}

func (p *Pack) AddFunction(name string, function interface{}) {
	p.funcs[name] = function
	if p.templateRoot != nil {
		p.templateRoot = p.templateRoot.Funcs(p.funcs)
	}
}

func (p *Pack) Reload() error {
	root, err := template.New("base").Funcs(p.funcs).ParseGlob(p.glob)
	if err != nil {
		return err
	}
	p.templateRoot = root
	return nil
}

// Execute renders the template named by name providing ctx as the render context.
func (p *Pack) Execute(w io.Writer, r *http.Request, name string, ctx *Context) error {
	if ctx.template == nil {
		ctx.template = p.templateRoot
	}
	if ctx.Request == nil {
		ctx.Request = r
	}
	if ctx.template.Lookup(name) == nil {
		return fmt.Errorf("templatepack: template %v not found", name)
	}
	return ctx.template.ExecuteTemplate(w, name, ctx)
}

// ExecutePartial renders the template named by "tmpl_page" providing obj as {{.Obj}} and page as {{.Page}}.
func (p *Pack) ExecutePage(w io.Writer, r *http.Request, page string, obj interface{}) error {
	return p.Execute(w, r, "tmpl_page", &Context{
		Page: page,
		Obj:  obj,
	})
}

// ExecutePartial renders the template named by "partial_" + name providing obj as {{.Obj}}.
func (p *Pack) ExecutePartial(w io.Writer, r *http.Request, name string, obj interface{}) error {
	return p.Execute(w, r, fmt.Sprintf("partial_%s", name), &Context{
		Page: name,
		Obj:  obj,
	})
}

func New(glob string) (*Pack, error) {
	pack := &Pack{
		funcs: make(template.FuncMap),
		glob:  glob,
	}

	err := pack.Reload()
	if err != nil {
		return nil, err
	}

	pack.AddFunction("equal", func(t1, t2 string) bool { return t1 == t2 })

	pack.AddFunction("subtemplate", func(ctx *Context, name string) template.HTML {
		buf := &bytes.Buffer{}
		err := ctx.template.ExecuteTemplate(buf, ctx.Page+"_"+name, ctx)
		if err != nil {
			return template.HTML("")
		}
		return template.HTML(buf.String())
	})

	pack.AddFunction("partial", func(ctx *Context, name string) template.HTML {
		buf := &bytes.Buffer{}
		err := ctx.template.ExecuteTemplate(buf, "partial_"+name, ctx)
		divId := "partial_container_" + name
		if err != nil {
			buf = &bytes.Buffer{}
			ctx.Obj = err
			ctx.template.ExecuteTemplate(buf, "partial_error", ctx)
		}
		return template.HTML(`<div id="` + divId + `">` + buf.String() + `</div>`)
	})

	pack.AddFunction("now", func() time.Time {
		return time.Now()
	})

	return pack, nil
}
