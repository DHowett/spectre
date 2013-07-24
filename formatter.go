package main

import (
	"bytes"
	"github.com/kylelemons/go-gypsy/yaml"
	"html/template"
	"io"
)

type FormatFunc func(io.Reader, ...string) (string, error)

type Formatter struct {
	name string
	fn   FormatFunc
	args []string
}

func (f *Formatter) Format(stream io.Reader, lang string) (string, error) {
	myargs := make([]string, len(f.args))
	for i, v := range f.args {
		n := v
		if n == "%LANG%" {
			n = lang
		}
		myargs[i] = n
	}
	return f.fn(stream, myargs...)
}

func plainTextFormatter(stream io.Reader, args ...string) (string, error) {
	buf := &bytes.Buffer{}
	io.Copy(buf, stream)
	return template.HTMLEscapeString(buf.String()), nil
}

var formatters map[string]*Formatter
var formatFunctions map[string]FormatFunc = map[string]FormatFunc{
	"commandFormatter": execWithStream,
	"plainText":        plainTextFormatter,
}

func FormatPaste(p *Paste) (string, error) {
	var formatter *Formatter
	var ok bool
	if formatter, ok = formatters[p.Language]; !ok {
		formatter = formatters["default"]
	}

	reader, _ := p.Reader()
	defer reader.Close()
	return formatter.Format(reader, p.Language)
}

func init() {
	yml := yaml.ConfigFile("formatters.yml")
	formatters = make(map[string]*Formatter)
	yformatters, _ := yaml.Child(yml.Root, ".formatters")
	for _, node := range yformatters.(yaml.List) {
		nmap, _ := node.(yaml.Map)
		name := nmap["name"].(yaml.Scalar).String()
		formatfn := nmap["func"].(yaml.Scalar).String()
		yargs, ok := nmap["args"].(yaml.List)
		if !ok {
			yargs = nil
		}

		args := make([]string, len(yargs))
		for i, v := range yargs {
			args[i] = v.(yaml.Scalar).String()
		}

		f := &Formatter{
			name: name,
			fn:   formatFunctions[formatfn],
			args: args,
		}

		formatters[name] = f
	}
}
