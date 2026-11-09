package main

// ---- containers ----------------------------------------------------------------------
//
// Three kinds of structure hold others:
//
//	cabinet  strictly organised: a stack for each kind (design_paper.go)
//	drawer   strictly whatever: anything, in the order it went in
//	rack     a stack of anything, a row each (its icon and name); clicking a
//	         row opens that structure inside the rack, under its row, as wide
//	         as the rack (the rack grows down to hold it); closing it (its
//	         row again, Esc, Alt tapped twice, or a click on the empty board)
//	         puts it back. Dragging it (or Alt + arrows) takes it out.
//
// Drop a page, note or shape on a drawer or rack to put it in (a cabinet
// takes pages, notes and the other sheets); drag a row out to take it out.
// Drawers used to hang under every structure; a board saved that way gets
// each such drawer as a drawer of its own, beside its structure and
// linked to it (detachDrawers).

import (
	"fmt"
	"math"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// structureDrawers: structures no longer have drawers of their own.
const structureDrawers = false

func isContainer(kind string) bool { return kind == kCabinet || kind == kDrawer || kind == kRack }

// accepts is whether container c takes o: a cabinet its kinds of sheet, a
// drawer or rack anything but another container.
func accepts(c, o *dOct) bool {
	if c == nil || o == nil || c == o || isContainer(o.Kind) {
		return false
	}
	switch c.Kind {
	case kCabinet:
		return textual(o.Kind)
	case kDrawer, kRack:
		return true
	}
	return false
}

// containerRows are the rows a container shows: a cabinet's stacks, a
// drawer's or rack's things (a rack's open when clicked). The thing open
// from a rack has a row of its own ("shut") where it was.
func (d *designState) containerRows(o *dOct) []stackRow {
	if o.Kind == kCabinet {
		return stackRows(o, false)
	}
	var rows []stackRow
	act := "item:%d"
	if o.Kind == kRack {
		act = "open:%d"
	}
	open := d.openIn(o)
	shut := func() {
		rows = append(rows, stackRow{"shut", "▾ " + rowIcon(open) + " " + itemTitle(open)})
		open = nil
	}
	for i, it := range o.Items {
		if open != nil && i == d.opened.at {
			shut()
		}
		if it != nil {
			rows = append(rows, stackRow{fmt.Sprintf(act, i), rowIcon(it) + " " + itemTitle(it)})
		}
	}
	if open != nil {
		shut()
	}
	if len(rows) == 0 {
		rows = append(rows, stackRow{"none", "drop anything here"})
	}
	return rows
}

// rowIcon is the icon a structure shows in a rack or drawer: its label's
// symbol, else its kind's.
func rowIcon(o *dOct) string {
	if o.LabelStyle != labelNone && o.LabelIcon != "" && !strings.HasPrefix(o.LabelIcon, "img:") {
		return o.LabelIcon
	}
	return kindIcons[o.Kind]
}

// containerLayers draws a drawer (a light frame with a pull at the bottom)
// or a rack (a double frame, a row per thing).
func (d *designState) containerLayers(o *dOct) []*lipgloss.Layer {
	x, y, w, h := d.rect(o)
	selected := d.sel == o.ID
	edge, fill := o.Color, mixHex(cBG, o.Color, 0.16)
	if selected {
		edge = mixHex(o.Color, cText, 0.35)
	}
	if d.dropOn == o.ID {
		edge, fill = cAccent, mixHex(cBG, cAccent, 0.2)
	}
	icon := kindIcons[o.Kind]
	if w < 6 || h < 4 {
		pill := mixHex(cBG, o.Color, 0.55)
		return []*lipgloss.Layer{lipgloss.NewLayer(st(cBG, pill).Bold(true).Width(w).MaxWidth(w).
			Render(ansi.Truncate(icon+o.Header, w, ""))).X(x).Y(y)}
	}
	tl, tr, bl, br, hz, vt, ml, mr := "┌", "┐", "└", "┘", "─", "│", "├", "┤"
	if o.Kind == kRack {
		tl, tr, bl, br, hz, vt, ml, mr = "╔", "╗", "╚", "╝", "═", "║", "╟", "╢"
	}
	iw := w - 2
	e := st(edge, fill)
	name := o.Header
	head := st(mixHex(o.Color, cText, 0.4), fill).Bold(true)
	title := centreLine(head.Render(ansi.Truncate(icon+" "+name, iw-1, "…")), iw, fill)
	switch {
	case selected && d.editing == partHeader:
		title = centreLine(head.Render(ansi.TruncateLeft(icon+" "+name, max(0, lipgloss.Width(icon+" "+name)-(iw-2)), "…"))+
			st(cAccent, fill).Render("▏"), iw, fill)
	case name == "":
		title = centreLine(st(mixHex(cDim, fill, 0.25), fill).Italic(true).Render(icon+" "+kindNames[o.Kind]), iw, fill)
	}
	lines := []string{
		e.Render(tl + strings.Repeat(hz, iw) + tr),
		e.Render(vt) + fitBlockLine(title, iw, fill) + e.Render(vt),
		e.Render(ml + strings.Repeat("─", iw) + mr),
	}
	rows := d.containerRows(o)
	at, gap := d.openGap(o)
	blank := e.Render(vt) + st(cText, fill).Render(strings.Repeat(" ", iw)) + e.Render(vt)
	for i, r := range drawRows(rows, iw, fill, edge) {
		lines = append(lines, e.Render(vt)+r+e.Render(vt))
		if i == at { // room for the thing open under its row (it is drawn over it)
			for range gap {
				lines = append(lines, blank)
			}
		}
	}
	bottom := bl + strings.Repeat(hz, iw) + br
	if o.Kind == kDrawer { // its pull
		left := (iw - 3) / 2
		bottom = bl + strings.Repeat(hz, left) + "═○═" + strings.Repeat(hz, max(iw-3-left, 0)) + br
	}
	lines = append(lines, e.Render(bottom))
	if len(lines) > h {
		lines = append(lines[:h-1], lines[len(lines)-1])
	}
	layers := []*lipgloss.Layer{lipgloss.NewLayer(strings.Join(lines, "\n")).X(x).Y(y)}
	return d.octExtras(layers, o, x, y, w, h, edge, fill, false, d.linkMode && d.linkFrom == o.ID)
}

// detachDrawers gives each structure that still holds things in a drawer
// of its own (boards saved before drawers were containers) a drawer
// beside it, linked to it, holding the same things.
func (d *designState) detachDrawers() {
	var made []*dOct
	for _, o := range d.octs {
		if o == nil || isContainer(o.Kind) || len(o.Items) == 0 {
			continue
		}
		w, _ := d.size(o)
		dr := &dOct{ID: d.nextID, Kind: kDrawer, X: o.X + w + 3, Y: o.Y, W: cabinetNewW, H: 5,
			Header: strings.TrimSpace(itemTitle(o) + " drawer"), Color: mixHex(o.Color, cBG, 0.25), Items: o.Items}
		d.nextID++
		o.Items, o.DrawerOpen = nil, false
		made = append(made, dr)
		d.links = append(d.links, dLink{o.ID, dr.ID})
	}
	if len(made) > 0 {
		d.octs = append(d.octs, made...)
		d.dirty = true
	}
}

// ---- a rack's thing, opened inside it -----------------------------------------------

// rackOpen is what is opened from a rack, and where it goes back to.
type rackOpen struct {
	id           int // the structure opened (on the board while open)
	rack         int // the rack it came from
	at           int // its place in the rack
	origW, origH int // its own size: open, it is as wide as the rack
	active       bool
}

// openIn is the thing open inside rack r, or nil.
func (d *designState) openIn(r *dOct) *dOct {
	if !d.opened.active || r == nil || r.ID != d.opened.rack {
		return nil
	}
	return d.byID(d.opened.id)
}

// openGap is the row of r the open thing sits under, and how many screen
// rows it takes (-1, 0: nothing is open in r).
func (d *designState) openGap(r *dOct) (int, int) {
	it := d.openIn(r)
	if it == nil {
		return -1, 0
	}
	for i, row := range d.containerRows(r) {
		if row.act == "shut" {
			_, _, _, h := d.rect(it)
			return i, h
		}
	}
	return -1, 0
}

// openRow is the row of rack r that the open thing sits under.
func (d *designState) openRow(r *dOct) int {
	for i, row := range d.containerRows(r) {
		if row.act == "shut" {
			return i
		}
	}
	return 0
}

// openRect is where the open thing is on the screen: inside its rack,
// under its row, as wide as the rack inside.
func (d *designState) openRect(it *dOct) (x, y, w, h int, ok bool) {
	r := d.byID(d.opened.rack)
	if r == nil {
		return 0, 0, 0, 0, false
	}
	at := d.openRow(r)
	rx, ry, rw, _ := d.rect(r)
	_, ih := d.size(it)
	return rx + 1, ry + 3 + at + 1, max(rw-2, 1), max(int(math.Round(float64(ih)*d.z())), 1), true
}

// openPlace is where the open thing is on the board (for the map): its
// own X and Y are left alone while it is open, so drawing it changes
// nothing that undo would see.
func (d *designState) openPlace(o *dOct) (x, y int, ok bool) {
	if !d.opened.active || o.ID != d.opened.id {
		return 0, 0, false
	}
	r := d.byID(d.opened.rack)
	if r == nil {
		return 0, 0, false
	}
	return r.X + 1, r.Y + 3 + d.openRow(r) + 1, true
}

// designRackOpen takes thing i out of rack r and shows it inside the rack,
// under its row, chosen, until it is closed.
func (m *Model) designRackOpen(r *dOct, i int) {
	d := m.ds()
	m.designRackClose() // one at a time
	if i < 0 || i >= len(r.Items) || r.Items[i] == nil {
		return
	}
	it := r.Items[i]
	r.Items = append(r.Items[:i:i], r.Items[i+1:]...)
	op := rackOpen{id: it.ID, rack: r.ID, at: i, origW: it.W, origH: it.H, active: true}
	if resizable(it) {
		// As wide as the rack inside, keeping its shape, and short enough
		// that the rack, with it, still fits on the screen.
		rw, rh := d.size(r)
		mw, mh := minSize(it.Kind)
		nw := max(rw-2, mw)
		nh := it.H
		if it.W > 0 {
			nh = int(math.Round(float64(it.H) * float64(nw) / float64(it.W)))
		}
		if d.maxH > 0 {
			nh = min(nh, d.maxH-rh-1)
		}
		it.W, it.H = nw, max(nh, mh)
	}
	d.octs = append(d.octs, it)
	d.sel, d.editing, d.selLink = it.ID, partNone, -1
	d.opened = op
	d.dirty = true
	m.say(kindIcons[it.Kind] + " " + itemTitle(it) + " open in " + octName(r) + " · its row again (or Alt tapped twice) puts it back · drag it to take it out")
}

// designRackClose puts the opened thing back in its rack, in its place. It
// says whether something was open.
func (m *Model) designRackClose() bool {
	d := m.ds()
	op := d.opened
	if !op.active {
		return false
	}
	d.opened = rackOpen{}
	it, r := d.byID(op.id), d.byID(op.rack)
	if it == nil || r == nil {
		return true // deleted while open, or its rack is gone: it stays where it is
	}
	d.remove(it.ID)
	it.X, it.Y = 0, 0
	it.W, it.H = op.origW, op.origH
	at := min(max(op.at, 0), len(r.Items))
	r.Items = append(r.Items[:at:at], append([]*dOct{it}, r.Items[at:]...)...)
	d.sel, d.editing = r.ID, partNone
	d.dirty = true
	m.say(kindIcons[it.Kind] + " " + itemTitle(it) + " back in " + octName(r) + ".")
	return true
}

// openPutBack is octs (copies) as they are saved: what is open in a rack
// back in it, its own size, so a board read again never has it stuck
// outside. octs itself is not changed.
func (d *designState) openPutBack(octs []*dOct) []*dOct {
	op := d.opened
	if !op.active {
		return octs
	}
	var it, r *dOct
	for _, o := range octs {
		switch {
		case o == nil:
		case o.ID == op.id:
			it = o
		case o.ID == op.rack:
			r = o
		}
	}
	if it == nil || r == nil {
		return octs
	}
	out := make([]*dOct, 0, len(octs))
	for _, o := range octs {
		switch {
		case o == it:
			continue
		case o == r:
			rc := *r
			ic := *it
			ic.X, ic.Y, ic.W, ic.H = 0, 0, op.origW, op.origH
			at := min(max(op.at, 0), len(r.Items))
			rc.Items = append(append(append([]*dOct(nil), r.Items[:at]...), &ic), r.Items[at:]...)
			o = &rc
		}
		out = append(out, o)
	}
	return out
}

// detachOpened takes o, open in a rack, out of it for good: it stays on
// the board where it shows, its own size again. It says whether o was open.
func (d *designState) detachOpened(o *dOct) bool {
	if !d.opened.active || o == nil || o.ID != d.opened.id {
		return false
	}
	x, y, _, _ := d.rect(o)
	wx, wy := d.world(x, y)
	op := d.opened
	d.opened = rackOpen{}
	o.X, o.Y = int(math.Floor(wx)), int(math.Floor(wy))
	o.W, o.H = op.origW, op.origH
	d.dirty = true
	return true
}

// rackPress is a press on a rack's row, not yet a click (it opens the
// thing) or a drag (it takes it out).
type rackPress struct {
	rack, at, sx, sy int
}
