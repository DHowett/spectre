package main

import (
	"bytes"
	"context"
	"encoding/json"
	"html/template"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/golang/glog"
)

type Language struct {
	Name                string   `json:"name,omitempty"`
	ID                  string   `json:"id,omitempty" yaml:"id"`
	Formatter           string   `json:"-"`
	AlternateIDs        []string `json:"alt_ids,omitempty" yaml:"alt_ids"`
	Extensions          []string `json:"-"`
	MIMETypes           []string `json:"-" yaml:"mimetypes"`
	DisplayStyle        string   `json:"-" yaml:"display_style"`
	SuppressLineNumbers bool     `json:"-" yaml:"suppress_line_numbers"`
}

type LanguageList []*Language

func (l LanguageList) Len() int {
	return len(l)
}

func (l LanguageList) Less(i, j int) bool {
	return l[i].Name < l[j].Name
}

func (l LanguageList) Swap(i, j int) {
	l[i], l[j] = l[j], l[i]
}

func LanguageNamed(name string) *Language {
	v, ok := languageConfig.languageMap[name]
	if !ok {
		return unknownLanguage
	}
	return v
}

type _LanguageConfiguration struct {
	LanguageGroups []*struct {
		Name      string       `json:"name,omitempty"`
		Languages LanguageList `json:"languages,omitempty"`
	} `yaml:"languageGroups"`
	Formatters map[string]*Formatter

	languageMap        map[string]*Language
	modtime            time.Time
	languageJSONReader *bytes.Reader
}

var languageConfig _LanguageConfiguration
var unknownLanguage *Language = &Language{
	Name:      "Unknown",
	ID:        "unknown",
	Formatter: "text",
}

type FormatFunc func(context.Context, *Formatter, io.Reader, ...string) (string, error)

type Formatter struct {
	Name string
	Func string
	Env  []string
	Args []string
	fn   FormatFunc
}

func (f *Formatter) Format(ctx context.Context, stream io.Reader, lang string) (string, error) {
	myargs := make([]string, len(f.Args))
	for i, v := range f.Args {
		n := v
		if n == "%LANG%" {
			n = lang
		}
		myargs[i] = n
	}
	return f.fn(ctx, f, stream, myargs...)
}

func commandFormatter(ctx context.Context, formatter *Formatter, stream io.Reader, args ...string) (output string, err error) {
	var outbuf, errbuf bytes.Buffer
	command := exec.CommandContext(ctx, args[0], args[1:]...)
	command.Stdin = stream
	command.Stdout = &outbuf
	command.Stderr = &errbuf
	command.Env = formatter.Env
	err = command.Run()
	output = strings.TrimSpace(outbuf.String())
	if err != nil {
		output = strings.TrimSpace(errbuf.String())
	}
	return
}

func plainTextFormatter(ctx context.Context, formatter *Formatter, stream io.Reader, args ...string) (string, error) {
	buf := &bytes.Buffer{}
	io.Copy(buf, stream)
	return template.HTMLEscapeString(buf.String()), nil
}

var formatFunctions map[string]FormatFunc = map[string]FormatFunc{
	"commandFormatter": commandFormatter,
	"plainText":        plainTextFormatter,
	"markdown":         markdownFormatter,
}

func FormatStream(r io.Reader, language *Language) (string, error) {
	var formatter *Formatter
	var ok bool
	if formatter, ok = languageConfig.Formatters[language.Formatter]; !ok {
		formatter = languageConfig.Formatters["default"]
	}

	timeoutContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return formatter.Format(timeoutContext, r, language.ID)
}

func FormatPaste(p *Paste) (string, error) {
	reader, _ := p.Reader()
	defer reader.Close()
	return FormatStream(reader, p.Language)
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
			languageConfig.languageMap[v.ID] = v
			for _, langname := range v.AlternateIDs {
				languageConfig.languageMap[langname] = v
			}
		}
		sort.Sort(g.Languages)
	}

	for _, v := range languageConfig.Formatters {
		v.fn = formatFunctions[v.Func]
	}
	glog.Info("Loaded ", len(languageConfig.languageMap), " languages.")
	glog.Info("Loaded ", len(languageConfig.Formatters), " formatters.")

	fi, _ := os.Stat("languages.yml")
	languageConfig.modtime = fi.ModTime()
	languageJSON, _ := json.Marshal(languageConfig.LanguageGroups)
	languageConfig.languageJSONReader = bytes.NewReader(languageJSON)
}

func init() {
	RegisterTemplateFunction("langByLexer", LanguageNamed)

	RegisterReloadFunction(loadLanguageConfig)
}
