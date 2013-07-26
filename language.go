package main

import (
	"bytes"
	"html/template"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strings"
	"syscall"
)

type Language struct {
	Title, Name, Formatter string
	Names                  []string
	Extensions             []string
	MIMETypes              []string `yaml:"mimetypes"`
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

func LanguageNamed(name string) *Language {
	v, ok := languageConfig.languageMap[name]
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
	for _, group := range languageConfig.LanguageGroups {
		out += "<optgroup label=\"" + group.Title + "\">"
		for _, l := range group.Languages {
			out += "<option value=\"" + l.Name + "\">" + l.Title + "</option>"
		}
		out += "</optgroup>"
	}
	languageOptionCache = out
	return template.HTML(languageOptionCache)
}

type _LanguageConfiguration struct {
	LanguageGroups []*struct {
		Title     string
		Languages LanguageList
	} `yaml:"languageGroups"`
	Formatters map[string]*Formatter

	languageMap map[string]*Language
}

var languageConfig _LanguageConfiguration

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

func commandFormatter(stream io.Reader, args ...string) (output string, err error) {
	var outbuf, errbuf bytes.Buffer
	command := exec.Command(args[0], args[1:]...)
	command.Stdin = stream
	command.Stdout = &outbuf
	command.Stderr = &errbuf
	err = command.Run()
	output = strings.TrimSpace(outbuf.String())
	if err != nil {
		output = strings.TrimSpace(errbuf.String())
	}
	return
}

func plainTextFormatter(stream io.Reader, args ...string) (string, error) {
	buf := &bytes.Buffer{}
	io.Copy(buf, stream)
	return strings.Replace(template.HTMLEscapeString(buf.String()), "\n", "<br>", -1), nil
}

var formatFunctions map[string]FormatFunc = map[string]FormatFunc{
	"commandFormatter": commandFormatter,
	"plainText":        plainTextFormatter,
}

func FormatPaste(p *Paste) (string, error) {
	var formatter *Formatter
	var ok bool
	if formatter, ok = languageConfig.Formatters[p.Language]; !ok {
		formatter = languageConfig.Formatters["default"]
	}

	reader, _ := p.Reader()
	defer reader.Close()
	return formatter.Format(reader, p.Language)
}

func loadLanguageConfig() {
	languageConfig = _LanguageConfiguration{}

	err := YAMLUnmarshalFile("languages.yml", &languageConfig)
	if err != nil {
		panic(err)
	}

	languageConfig.languageMap = make(map[string]*Language)
	for _, g := range languageConfig.LanguageGroups {
		for _, v := range g.Languages {
			languageConfig.languageMap[v.Name] = v
			for _, langname := range v.Names {
				languageConfig.languageMap[langname] = v
			}
		}
		sort.Sort(g.Languages)
	}

	languageOptionCache = ""

	for _, v := range languageConfig.Formatters {
		v.fn = formatFunctions[v.Func]
	}
}

func init() {
	loadLanguageConfig()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGHUP)

	go func() {
		for _ = range sigChan {
			loadLanguageConfig()
		}
	}()

	RegisterTemplateFunction("langByLexer", LanguageNamed)
	RegisterTemplateFunction("languageOptionListHTML", LanguageOptionListHTML)
}
