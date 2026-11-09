package main

// ---- picking a group (Shift+G) -------------------------------------------------------
//
// Shift+G turns group mode on, with the chosen structure (if any) picked.
// In group mode:
//
//	click a structure      picks it (again: unpicks it)
//	drag on the empty board picks everything the box touches
//	G                      picks the chosen structure (the arrows choose)
//	Enter or Shift+G       gives everything picked one name plate (asked
//	                       for), and group mode ends
//	Esc, Alt tapped twice  leaves, naming nothing
//
// Picked structures have a dashed frame round them. Everything with the
// same name plate is a group: AltGr+P lists them (design_labels.go).

import (
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// groupBand is the box being dragged out on the empty board, in canvas cells.
type groupBand struct {
	x0, y0, x1, y1 int
}

// designGroupMode turns group mode on (picking the chosen structure) or off.
func (m *Model) designGroupMode(on bool) {
	d := m.ds()
	d.groupMode, d.band = on, nil
	d.picked = map[int]bool{}
	d.dirty = true
	if !on {
		m.say("Group mode off.")
		return
	}
	d.editing = partNone
	if d.byID(d.sel) != nil {
		d.picked[d.sel] = true
	}
	for _, id := range d.multiIDs() { // chosen together: all picked
		d.picked[id] = true
	}
	d.multi = nil
	m.sayPicked()
}

// sayPicked says how many are picked and what to do next.
func (m *Model) sayPicked() {
	n := len(m.ds().picked)
	m.say("Group: " + plural(n, "structure") + " picked.")
}

// designGroupKey handles a key in group mode. It says whether it did.
func (m *Model) designGroupKey(name string) (bool, tea.Cmd) {
	d := m.ds()
	switch name {
	case "g":
		if o := d.byID(d.sel); o != nil {
			d.pickToggle(o)
			m.sayPicked()
		} else {
			m.say("Group: choose a structure first (the arrows), then G picks it.")
		}
		return true, nil
	case "enter", "G", "shift+g":
		return true, m.designGroupName()
	}
	return false, nil
}

// pickToggle picks o, or unpicks it if it was picked.
func (d *designState) pickToggle(o *dOct) {
	if d.picked == nil {
		d.picked = map[int]bool{}
	}
	if d.picked[o.ID] {
		delete(d.picked, o.ID)
	} else {
		d.picked[o.ID] = true
	}
	d.dirty = true
}

// designGroupName asks for the name plate to give everything picked.
func (m *Model) designGroupName() tea.Cmd {
	d := m.ds()
	ids := d.pickedIDs()
	if len(ids) == 0 {
		m.say("Group: nothing picked yet · click structures, or drag round them · Esc leaves")
		return nil
	}
	// Offer the name they already share, if they all have the same one.
	same := d.byID(ids[0]).Plate
	for _, id := range ids[1:] {
		if d.byID(id).Plate != same {
			same = ""
			break
		}
	}
	return m.ask(pDesignGroup, "Name plate for "+plural(len(ids), "structure")+" (empty: none): ", same)
}

// pickedIDs are the picked structures still on the board, in board order.
func (d *designState) pickedIDs() []int {
	var ids []int
	for _, o := range d.octs {
		if o != nil && d.picked[o.ID] {
			ids = append(ids, o.ID)
		}
	}
	return ids
}

// designSetGroup gives everything picked the name plate typed at the
// prompt, and ends group mode.
func (m *Model) designSetGroup(s string) {
	d := m.ds()
	name := strings.TrimSpace(s)
	ids := d.pickedIDs()
	for _, id := range ids {
		d.byID(id).Plate = name
	}
	d.groupMode, d.band, d.picked = false, nil, nil
	d.dirty = true
	d.save()
	switch {
	case len(ids) == 0:
		m.say("Nothing was picked.")
	case name == "":
		m.say("Name plate taken off " + plural(len(ids), "structure") + ".")
	default:
		m.say("Name plate “" + name + "” on " + plural(len(ids), "structure") + " (" + itoa(len(d.plates()[name])) + " in the group). AltGr+P lists the groups.")
	}
}

// groupPress is a press in group mode: on a structure it picks it; on the
// empty board it starts a box.
func (m *Model) groupPress(o *dOct, sx, sy int) {
	d := m.ds()
	if o != nil {
		d.pickToggle(o)
		m.sayPicked()
		return
	}
	d.band = &groupBand{sx, sy, sx, sy}
	d.drag = dragBand
	d.dirty = true
}

// bandPick picks everything the box touches.
func (m *Model) bandPick() {
	d := m.ds()
	b := d.band
	d.band = nil
	if b == nil {
		return
	}
	x0, x1 := min(b.x0, b.x1), max(b.x0, b.x1)
	y0, y1 := min(b.y0, b.y1), max(b.y0, b.y1)
	if d.picked == nil {
		d.picked = map[int]bool{}
	}
	for _, o := range d.octs {
		if o == nil {
			continue
		}
		x, y, w, h := d.rect(o)
		if x <= x1 && x+w-1 >= x0 && y <= y1 && y+h-1 >= y0 {
			d.picked[o.ID] = true
		}
	}
	d.dirty = true
	m.sayPicked()
}

// paintPicked draws a dashed frame round each picked structure, and the
// box being dragged out.
func (m *Model) paintPicked(c *boardCanvas) {
	d := m.ds()
	if !d.groupMode {
		return
	}
	cw, ch := c.Width(), c.Height()
	put := func(x, y int, s, fg string) {
		if x < 0 || y < 0 || x >= cw || y >= ch {
			return
		}
		cell := c.Lines[y][x]
		st := cell.Style
		st.Fg, st.Attrs = rgbaOf(fg), uv.AttrBold
		if st.Bg == nil {
			st.Bg = rgbaOf(cBG)
		}
		c.Lines[y][x] = uv.Cell{Content: s, Width: 1, Style: st}
	}
	frame := func(x0, y0, x1, y1 int, fg string) {
		for x := x0 + 1; x < x1; x++ {
			put(x, y0, "┅", fg)
			put(x, y1, "┅", fg)
		}
		for y := y0 + 1; y < y1; y++ {
			put(x0, y, "┇", fg)
			put(x1, y, "┇", fg)
		}
		put(x0, y0, "┏", fg)
		put(x1, y0, "┓", fg)
		put(x0, y1, "┗", fg)
		put(x1, y1, "┛", fg)
	}
	ids := d.pickedIDs()
	sort.Ints(ids)
	for _, id := range ids {
		o := d.byID(id)
		if !d.onScreen(o, cw, ch) {
			continue
		}
		x, y, w, h := d.rect(o)
		frame(x-1, y-1, x+w, y+h, cAccent)
	}
	if b := d.band; b != nil {
		paintBand(c, b) // shaded, like the Ctrl + drag box (design_select.go)
	}
}
