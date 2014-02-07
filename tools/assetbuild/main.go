package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"launchpad.net/goyaml"
	//"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func YAMLUnmarshalFile(filename string, i interface{}) error {
	yml, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}

	err = goyaml.Unmarshal(yml, i)
	if err != nil {
		return err
	}

	return nil
}

type FormatFunc func(*TypeHandler, io.Reader, ...string) (string, error)

type TypeHandler struct {
	Func string
	Env  []string
	Args []string
	fn   FormatFunc
}

func (f *TypeHandler) Format(stream io.Reader, lang string) (string, error) {
	myargs := make([]string, len(f.Args))
	for i, v := range f.Args {
		n := v
		if n == "%LANG%" {
			n = lang
		}
		myargs[i] = n
	}
	return f.fn(f, stream, myargs...)
}

func commandTypeHandler(formatter *TypeHandler, stream io.Reader, args ...string) (output string, err error) {
	var outbuf, errbuf bytes.Buffer
	command := exec.Command(args[0], args[1:]...)
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

func (f *TypeHandler) String() string {
	return strings.Join(f.Args, " ")
}

var formatFunctions map[string]FormatFunc = map[string]FormatFunc{
	"command": commandTypeHandler,
}

type Asset struct {
	Name string
	Type string
	Env  string
}

type AssetSet struct {
	Name    string
	Type    string
	Handler string
	Env     string
	Path    string
	Assets  []*Asset
}

type Configuration struct {
	Handlers map[string]*TypeHandler
	Sets     map[string]*AssetSet
}

var _assetConfig Configuration

func loadConfig(filename string) {
	err := YAMLUnmarshalFile(filename, &_assetConfig)
	if err != nil {
		panic(err)
	}

	for _, v := range _assetConfig.Handlers {
		v.fn = formatFunctions[v.Func]
	}
}

var arguments struct {
	filename string
	makedeps bool
	env      string
}

func main() {
	flag.StringVar(&arguments.filename, "file", "assets.yml", "asset config filename")
	flag.StringVar(&arguments.env, "env", "production", "environment to emit build rules for")
	flag.BoolVar(&arguments.makedeps, "md", true, "output makedeps")
	flag.Parse()
	loadConfig(arguments.filename)

	if arguments.makedeps {
		finalAssets := make([]string, len(_assetConfig.Sets))
		i := 0
		for catname, cat := range _assetConfig.Sets {
			if cat.Env != "" && cat.Env != arguments.env {
				continue
			}
			fmt.Println("#", catname, "(via "+cat.Handler+")")
			outpath := filepath.Join(cat.Path, cat.Name+"."+cat.Handler)
			fmt.Printf("%s: ", outpath)
			for _, asset := range cat.Assets {
				if asset.Env != "" && asset.Env != arguments.env {
					continue
				}
				ext := asset.Type
				if ext == "" {
					ext = cat.Type
				}
				fmt.Printf("%s ", filepath.Join(cat.Path, asset.Name+"."+ext))
			}
			fmt.Printf("\n")
			fmt.Println("\t-rm $@; cat $^ | " + _assetConfig.Handlers[cat.Handler].String() + " > $@")
			finalAssets[i] = outpath
			i++
		}
		fmt.Printf(".PHONY: _assets\n_assets: %s\n", strings.Join(finalAssets, " "))
		fmt.Printf(".PHONY: _clean_assets\n_clean_assets:\n\t-rm %s\n", strings.Join(finalAssets, " "))
		return
	}

	fmt.Printf("%+v\n", _assetConfig)
}
