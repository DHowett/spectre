package main

import (
	"bytes"
	"html/template"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

type asset struct {
	Path, Name, Kind string
	Mtime            time.Time
}

var assets map[string]*asset
var assetFilesystem http.FileSystem

func assetFunction(kind string, names ...string) template.HTML {
	if Env() == EnvironmentProduction {
		names = []string{"all.min"}
	}

	foundAssets := make([]*asset, len(names))
	for i, v := range names {
		foundAssets[i] = assets[v+"-"+kind]
	}

	buf := &bytes.Buffer{}
	ExecuteTemplate(buf, "asset_"+kind, &RenderContext{Obj: foundAssets})

	return template.HTML(buf.String())
}

func assetDirectory(assetMap map[string]*asset, path string) {
	dir, _ := assetFilesystem.Open(path)
	fis, _ := dir.Readdir(0)
	for _, v := range fis {
		name := v.Name()
		newPath := filepath.Join(path, name)
		if v.IsDir() {
			assetDirectory(assetMap, newPath)
		} else {
			bits := strings.Split(name, ".")
			kind := bits[len(bits)-1]
			name := strings.Join(bits[:len(bits)-1], ".")
			if name != "" {
				assetMap[name+"-"+kind] = &asset{
					Path:  newPath,
					Name:  name,
					Kind:  kind,
					Mtime: v.ModTime(),
				}
			}
		}
	}
}

func InitAssets() {
	newAssetMap := make(map[string]*asset)
	assetDirectory(newAssetMap, "/")
	assets = newAssetMap
}

func AssetFilesystem() http.FileSystem {
	return assetFilesystem
}

func init() {
	assetFilesystem = http.Dir("./public")
	RegisterTemplateFunction("assets", assetFunction)
	RegisterReloadFunction(InitAssets)
}
