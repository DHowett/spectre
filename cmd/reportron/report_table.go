package main

import (
	"fmt"

	"github.com/jroimartin/gocui"
)

type reportTableVC struct {
	ReportService *ReportStore

	g *gocui.Gui
	v *gocui.View

	selected      int
	sortedReports []*ReportedPaste
}

func (vc *reportTableVC) Layout(g *gocui.Gui) error {
	//////////////adapt/////////////
	return vc.LoadView(g)
}

func (vc *reportTableVC) LoadView(g *gocui.Gui) error {
	maxX, maxY := g.Size()
	if v, err := g.SetView("table", 0, 0, maxX-12, maxY-1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		vc.v = v
		vc.g = g
		vc.ReloadData()
	}
	g.SetCurrentView("table")
	return nil
}

func (vc *reportTableVC) BindKeys(g *gocui.Gui) error {
	if err := g.SetKeybinding("table", gocui.KeyArrowDown, gocui.ModNone, vc.keyDown); err != nil {
		return err
	}
	if err := g.SetKeybinding("table", gocui.KeyArrowUp, gocui.ModNone, vc.keyUp); err != nil {
		return err
	}
	if err := g.SetKeybinding("table", 'd', gocui.ModNone, vc.keyDelete); err != nil {
		return err
	}
	if err := g.SetKeybinding("table", 'i', gocui.ModNone, vc.keyIgnore); err != nil {
		return err
	}
	return nil
}

func (vc *reportTableVC) keyUp(g *gocui.Gui, v *gocui.View) error {
	ns := vc.selected - 1
	if ns <= 0 {
		ns = len(vc.sortedReports) - 1
	}
	vc.selected = ns
	vc.redraw()
	return nil
}

func (vc *reportTableVC) keyDown(g *gocui.Gui, v *gocui.View) error {
	ns := vc.selected + 1
	if ns >= len(vc.sortedReports) {
		ns = 0
	}
	vc.selected = ns
	vc.redraw()
	return nil
}

func (vc *reportTableVC) keyDelete(g *gocui.Gui, v *gocui.View) error {
	vc.ReportService.Delete(vc.sortedReports[vc.selected].ID)
	vc.ReloadData()
	return nil
}

func (vc *reportTableVC) keyIgnore(g *gocui.Gui, v *gocui.View) error {
	return nil
}

func (vc *reportTableVC) redraw() {
	_, h := vc.v.Size()
	// calculate display window; 6 lines per paste
	// draw pastes from start to end
	// etc.
	// right now this only scrolls to the middle. it'll never scroll to the bottom
	// of the screen
	first := vc.selected
	max := h/6 + 1
	if first > max/2 {
		first = first - max/2
	} else {
		first = 0
	}
	last := first + max
	if last >= len(vc.sortedReports) {
		last = len(vc.sortedReports)
	}
	vc.v.Clear()
	for i, rp := range vc.sortedReports[first:last] {
		if vc.selected == first+i {
			fmt.Fprintf(vc.v, "\033[1;30;41m")
		}
		fmt.Fprintf(vc.v, "Paste %s (%d reports)\033[0m\n", rp.ID, rp.ReportCount)
		for i := 0; i < 5; i++ {
			fmt.Fprintf(vc.v, "--- fake line %d ---\n", i+1)
		}
	}
}

func (vc *reportTableVC) ReloadData() {
	vc.sortedReports = SortReports(vc.ReportService.GetReports())
	vc.g.Update(func(g *gocui.Gui) error {
		vc.redraw()
		return nil
	})
}
