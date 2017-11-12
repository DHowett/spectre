package main

import (
	"fmt"

	"github.com/jroimartin/gocui"
)

type reportTableVC struct {
	ReportService *ReportStore

	g       *gocui.Gui
	v       *gocui.View
	statusv *gocui.View

	selected      int
	sortedReports []*ReportedPaste
	marks         map[PasteID]struct{}
}

func (vc *reportTableVC) Layout(g *gocui.Gui) error {
	//////////////adapt/////////////
	return vc.LoadView(g)
}

func (vc *reportTableVC) LoadView(g *gocui.Gui) error {
	maxX, maxY := g.Size()
	if v, err := g.SetView("table", 0, 0, maxX-1, maxY-2); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		vc.v = v
		vc.g = g
	}
	if v, err := g.SetView("status", 0, maxY-2, maxX-1, maxY); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Frame = false
		vc.statusv = v
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
	if err := g.SetKeybinding("table", gocui.KeySpace, gocui.ModNone, vc.keyMark); err != nil {
		return err
	}
	if err := g.SetKeybinding("table", 'd', gocui.ModNone, vc.keyDelete); err != nil {
		return err
	}
	if err := g.SetKeybinding("table", 'i', gocui.ModNone, vc.keyIgnore); err != nil {
		return err
	}
	if err := g.SetKeybinding("table", 'D', gocui.ModNone, vc.keyDeleteMarked); err != nil {
		return err
	}
	if err := g.SetKeybinding("table", 'I', gocui.ModNone, vc.keyIgnoreMarked); err != nil {
		return err
	}
	return nil
}

func (vc *reportTableVC) keyUp(g *gocui.Gui, v *gocui.View) error {
	ns := vc.selected - 1
	if ns < 0 {
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

func (vc *reportTableVC) keyMark(g *gocui.Gui, v *gocui.View) error {
	id := vc.sortedReports[vc.selected].ID
	if _, ok := vc.marks[id]; ok {
		delete(vc.marks, id)
	} else {
		vc.marks[id] = struct{}{}
	}
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

func (vc *reportTableVC) keyDeleteMarked(g *gocui.Gui, v *gocui.View) error {
	for id, _ := range vc.marks {
		vc.ReportService.Delete(id)
	}
	vc.marks = make(map[PasteID]struct{})
	vc.ReloadData()
	// TODO: move the selection
	return nil
}

func (vc *reportTableVC) keyIgnoreMarked(g *gocui.Gui, v *gocui.View) error {
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
		bleft, bright := " ", " "
		if vc.selected == first+i {
			bright = "\033[1;33m\u258c\033[0m"
		}

		if _, ok := vc.marks[rp.ID]; ok {
			bleft = "\033[1;32m\u258c\033[0m"
		}

		fmt.Fprintf(vc.v, bleft+bright+"\033[4;37m%s (%d reports)\033[0m\n", rp.ID, rp.ReportCount)
		for i := 0; i < 5; i++ {
			fmt.Fprintf(vc.v, bleft+bright+"--- fake line %d ---\n", i+1)
		}
	}

	vc.printStatus()
}

func readPaste(id PasteID) {

}

func (vc *reportTableVC) printStatus() {
	vc.statusv.Clear()
	c := len(vc.sortedReports)
	mc := len(vc.marks)
	fmt.Fprintf(vc.statusv, "%d report", c)
	if c != 1 {
		fmt.Fprintf(vc.statusv, "s")
	}
	if mc > 0 {
		fmt.Fprintf(vc.statusv, "; %d marked (D to delete all, I to ignore all)", mc)
	} else {
		fmt.Fprintf(vc.statusv, " (d to delete, i to ignore, SPACE to mark)")
	}
}

func (vc *reportTableVC) ReloadData() {
	if vc.marks == nil {
		vc.marks = make(map[PasteID]struct{})
	}
	vc.sortedReports = SortReports(vc.ReportService.GetReports())
	vc.g.Update(func(g *gocui.Gui) error {
		vc.redraw()
		return nil
	})
}
