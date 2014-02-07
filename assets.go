package main

import (
	"bytes"
	"html/template"
	"net/http"
	"path/filepath"
	"time"
)

type assetFile struct {
	Path, Name string
	Mtime      time.Time
}

type assetFilesystem struct {
	http.FileSystem
	Files map[string]*assetFile
}

type AssetEntry struct {
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
	Assets  []*AssetEntry
}

var _assetConfig struct {
	Sets map[string]*AssetSet
}

func assetCategoryFunction(catname string) template.HTML {
	cat := _assetConfig.Sets[catname]
	curEnv := Env()
	// Set not found
	if cat == nil {
		return template.HTML("")
	}
	// Set has an environment, and it's not this one.
	if cat.Env != "" && cat.Env != curEnv {
		return template.HTML("")
	}

	buf := &bytes.Buffer{}
	// If the set has no environment, and we're in production
	// assume that we want the set's generated result (per assetbuild)
	if cat.Env == "" && Env() == EnvironmentProduction {
		_foundAsset := assetFs.Files[cat.Name+"."+cat.Handler]
		ExecuteTemplate(buf, "asset_"+cat.Type, &RenderContext{Obj: _foundAsset})
	} else {
		// Otherwise,
		for _, ae := range cat.Assets {
			// Only render assets that match our env (if they have an env to match)
			if ae.Env != "" && ae.Env != curEnv {
				continue
			}

			typ := ae.Type
			if typ == "" {
				typ = cat.Type
			}
			_foundAsset := assetFs.Files[ae.Name+"."+typ]
			ExecuteTemplate(buf, "asset_"+typ, &RenderContext{Obj: _foundAsset})
		}
	}

	return template.HTML(buf.String())
}

func (fs *assetFilesystem) Populate() {
	newAssetMap := make(map[string]*assetFile)
	fs.recursivelyPopulateMap(newAssetMap, "/")
	fs.Files = newAssetMap
}

func (fs *assetFilesystem) recursivelyPopulateMap(assetMap map[string]*assetFile, path string) {
	dir, _ := fs.Open(path)
	fis, _ := dir.Readdir(0)
	for _, v := range fis {
		name := v.Name()
		newPath := filepath.Join(path, name)
		if v.IsDir() {
			fs.recursivelyPopulateMap(assetMap, newPath)
		} else {
			if name != "" {
				assetMap[name] = &assetFile{
					Path:  newPath,
					Name:  name,
					Mtime: v.ModTime(),
				}
			}
		}
	}
}

func InitAssets() {
	assetFs.Populate()

	err := YAMLUnmarshalFile("assets.yml", &_assetConfig)
	if err != nil {
		panic(err)
	}
}

var assetFs *assetFilesystem

func AssetFilesystem() http.FileSystem {
	return assetFs
}

func init() {
	assetFs = &assetFilesystem{FileSystem: http.Dir("./public")}
	RegisterTemplateFunction("assets", assetCategoryFunction)
	RegisterReloadFunction(InitAssets)
}
