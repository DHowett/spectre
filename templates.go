package main

import "github.com/DHowett/ghostbin/lib/templatepack"

var templatePack *templatepack.Pack

func _initTemplatePack() error {
	templatePack = templatepack.New("templates/*.tmpl")
	return nil
}

func init() {
	globalInit.Add(&InitHandler{
		Priority: 5,
		Name:     "template_init",
		Do:       _initTemplatePack,
	})
	globalInit.Add(&InitHandler{
		Priority: 90,
		Name:     "template_load",
		Do:       func() error { return templatePack.Reload() },
		Redo:     func() error { return templatePack.Reload() },
	})
}
