package main

import "github.com/jroimartin/gocui"

type ViewController interface {
	LoadView(*gocui.Gui) error
	BindKeys(*gocui.Gui) error
}
