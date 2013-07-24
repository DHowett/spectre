package main

import (
	"bytes"
	"html/template"
	"io"
)

type FormatFunc func(io.Reader, ...string) (string, error)

type Formatter struct {
	Name string
	Func string
	Args []string
	fn   FormatFunc
}

func (f *Formatter) Format(stream io.Reader, lang string) (string, error) {
	myargs := make([]string, len(f.Args))
	for i, v := range f.Args {
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
	var f []*Formatter
	YAMLUnmarshalFile("formatters.yml", &f)

	formatters = make(map[string]*Formatter, len(f))
	for _, v := range f {
		formatters[v.Name] = v
		v.fn = formatFunctions[v.Func]
	}
}
