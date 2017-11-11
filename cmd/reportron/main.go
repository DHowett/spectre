package main

import (
	"log"
	"os"

	"github.com/jessevdk/go-flags"
	"github.com/jroimartin/gocui"
)

var args struct {
	ReportArchive  string `short:"r" description:"report archive (.gob) path" default:"reports.gob"`
	PasteDirectory string `short:"p" description:"paste directory" default:"pastes"`
}

/*
func layout(g *gocui.Gui) error {
	maxX, maxY := g.Size()
	if v, err := g.SetView("table", 0, 0, maxX-11, maxY-1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}

		v.Highlight = true
		v.SelBgColor = gocui.ColorBlack
		v.SelFgColor = gocui.ColorYellow | gocui.AttrBold

		drawPasteList(v)
	}
	if v, err := g.SetView("operations", maxX-10, 0, maxX-1, maxY-1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Autoscroll = true
	}
	g.SetCurrentView("table")
	return nil
}

func kDown(g *gocui.Gui, v *gocui.View) error {
	if v != nil {
		cx, cy := v.Cursor()
		cy += 6
		_, my := v.Size()
		if cy >= my-6 {
			// redline: always keep 6 lines visible on the _bottom_
			ox, oy := v.Origin()
			oy += 6
			v.SetOrigin(ox, oy)
		} else {
			v.SetCursor(cx, cy)
		}
	}
	return nil
}

func kUp(g *gocui.Gui, v *gocui.View) error {
	if v != nil {
		v.MoveCursor(0, -6, false)
		ox, oy := v.Origin()
		if oy%6 != 0 {
			oy -= (oy % 6)
			v.SetOrigin(ox, oy)
		}
		cx, cy := v.Cursor()
		if cy%6 != 0 {
			cy -= (cy % 6)
			v.SetCursor(cx, cy)
		}
	}
	return nil
}

func reportAtCursor(g *gocui.Gui, v *gocui.View) *ReportedPaste {
	_, cy := v.Cursor()
	_, oy := v.Origin()
	return sortedReports[(oy+cy)/6]
}

func kDelete(g *gocui.Gui, v *gocui.View) error {
	if v != nil {
		rp := reportAtCursor(g, v)
		ov, _ := g.View("operations")
		fmt.Fprintf(ov, "\033[1;31mD %s\033[0m\n", rp.ID)
		reports.Delete(rp.ID)
		refreshDatamodel()
		g.Update(func(g *gocui.Gui) error {
			v, _ := g.View("table")
			drawPasteList(v)
			return nil
		})
	}
	return nil
}

func kIgnore(g *gocui.Gui, v *gocui.View) error {
	if v != nil {
		rp := reportAtCursor(g, v)
		ov, _ := g.View("operations")
		ov.FgColor = gocui.ColorGreen
		fmt.Fprintf(ov, "\033[1;32mI %s\n\033[0m", rp.ID)
		reports.Delete(rp.ID)
		refreshDatamodel()
		g.Update(func(g *gocui.Gui) error {
			v, _ := g.View("table")
			drawPasteList(v)
			return nil
		})
	}
	return nil
}

func keybindings(g *gocui.Gui) error {
	if err := g.SetKeybinding("", 'q', gocui.ModNone, kQuit); err != nil {
		return err
	}
	if err := g.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, kQuit); err != nil {
		return err
	}
	if err := g.SetKeybinding("table", gocui.KeyArrowDown, gocui.ModNone, kDown); err != nil {
		return err
	}
	if err := g.SetKeybinding("table", gocui.KeyArrowUp, gocui.ModNone, kUp); err != nil {
		return err
	}
	if err := g.SetKeybinding("table", 'd', gocui.ModNone, kDelete); err != nil {
		return err
	}
	if err := g.SetKeybinding("table", 'i', gocui.ModNone, kIgnore); err != nil {
		return err
	}
	return nil
}
*/
func kQuit(g *gocui.Gui, v *gocui.View) error {
	return gocui.ErrQuit
}

func keybindings(g *gocui.Gui) error {
	if err := g.SetKeybinding("", 'q', gocui.ModNone, kQuit); err != nil {
		return err
	}
	return nil
}

var reports *ReportStore

func main() {
	_, err := flags.Parse(&args)
	if err != nil {
		// flags printed the error for us
		os.Exit(1)
	}

	reports, err = LoadReportStore(args.ReportArchive)
	if err != nil {
		log.Panicln(err)
	}

	g, err := gocui.NewGui(gocui.OutputNormal)
	if err != nil {
		log.Panicln(err)
	}
	defer g.Close()

	tableVc := &reportTableVC{
		ReportService: reports,
	}
	g.SetManager(tableVc)

	if err := keybindings(g); err != nil {
		log.Panicln(err)
	}

	tableVc.BindKeys(g)

	if err := g.MainLoop(); err != nil && err != gocui.ErrQuit {
		log.Panicln(err)
	}

}
