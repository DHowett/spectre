package main

import (
	"github.com/DHowett/ghostbin/lib/templatepack"
	"github.com/golang/glog"
)

var templatePack *templatepack.Pack

func init() {
	tpack, err := templatepack.New("templates/*.tmpl")
	if err != nil {
		panic(err)
	}
	templatePack = tpack

	RegisterReloadFunction(func() {
		err := templatePack.Reload()
		if err != nil {
			glog.Error("Error reloading templates:", err)
		}
	})
}
