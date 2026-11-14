package main

// ---- Tab through a container, section by section ----------------------------------------
//
// With a container chosen (a cabinet, a drawer unit, a rack, a board or a
// box), Tab steps through all of its sections in order and Shift+Tab steps
// back, on the flat board and in 3D alike. After the last section Tab goes
// round to the first again (the container's name); Tab never leaves the
// container by itself: an arrow, Esc, Alt tapped twice, a click or
// choosing another structure ends the stepping.
//
// The sections, in order:
//
//	cabinet      its name · then for each shelf that holds something (pages,
//	             books, slides, notes, drawings): the shelf, then each thing on it
//	drawer unit  its name · then for each drawer: the drawer, then each thing in it
//	rack         its name · then each row, top to bottom
//	board        its name · then each pin, in the order they were pinned
//	box          its name · then for each layer that holds something (top,
//	             middle, bottom): the layer, then each thing on it
//
// Each section is put in writing mode by itself: on the name, typing writes
// the container's name; on a thing (and a rack's row, a pin), its title.
// A drawer, a shelf or a layer has no words of its own: the view goes to it
// and the next Tab goes on. The container moves to show each one: a
// cabinet's doors open for its shelves, the drawer reached is pulled out
// (the others pushed in), a rack's row opens as a click on it does, a
// box's lid opens and the layer reached is looked at. On the name, and when
// an arrow (or choosing another structure) ends the stepping, the container
// is put back as Tab found it; Esc (or Alt tapped twice) only stops the
// writing, leaving what Tab reached open and chosen (a pin: then R, S, T …
// work on it). A key that does not type (PgUp, PgDn, Alt + arrows) stops
// the writing too and does its work. Every step moves the view to the
// section: on the flat board it is centred; in 3D the camera stands square
// in front of it.

import (
	"math"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// partThing is d.editing while writing the title of a thing inside a
// container (the section's thing, see sectionThing).
const partThing = 9

// What a section is.
const (
	secName   = iota // the container's name
	secDrawer        // a drawer of a unit (slot)
	secShelf         // a cabinet's shelf (stack)
	secLayer         // a box's layer (slot)
	secThing         // a thing in a drawer (slot), on a shelf (stack) or a layer (slot): at, in Items
	secRow           // a rack's row: at, its place in the rack
	secPin           // a board's pin: at, in Items
)

type section struct {
	kind  int
	slot  int    // the drawer, or the box's layer
	stack string // the cabinet's shelf
	at    int
}

// sectState is Tab stepping through a container.
type sectState struct {
	on    bool
	box   int      // the container
	cur   section  // the section reached
	i     int      // and its place in the list
	sel   int      // what is chosen on it (the container, or a rack's open thing)
	found struct { // the container as Tab found it, put back on its name and at the end
		open   bool
		pulled []bool
		view   int
	}
}

// sections are container o's sections, in Tab's order.
func (d *designState) sections(o *dOct) []section {
	s := []section{{kind: secName}}
	switch o.Kind {
	case kCabinet:
		for _, k := range stackKinds {
			idx := shelfThings(o, k)
			if len(idx) == 0 {
				continue
			}
			s = append(s, section{kind: secShelf, stack: k})
			for _, j := range idx {
				s = append(s, section{kind: secThing, stack: k, at: j})
			}
		}
	case kDrawer:
		for i := range drawerCount(o) {
			s = append(s, section{kind: secDrawer, slot: i})
			for _, j := range slotThings(o, i) {
				s = append(s, section{kind: secThing, slot: i, at: j})
			}
		}
	case kRack:
		n := len(o.Items)
		if d.openIn(o) != nil {
			n++ // the open one is out of Items while open
		}
		for i := range n {
			s = append(s, section{kind: secRow, at: i})
		}
	case kBoard:
		for _, j := range drawerThings(o) {
			s = append(s, section{kind: secPin, at: j})
		}
	case kBox:
		for l := range boxLayers {
			idx := boxLayerThings(o, l)
			if len(idx) == 0 {
				continue
			}
			s = append(s, section{kind: secLayer, slot: l})
			for _, j := range idx {
				s = append(s, section{kind: secThing, slot: l, at: j})
			}
		}
	}
	return s
}

// shelfThings are the indexes (in o.Items) of what is on cabinet o's shelf kind.
func shelfThings(o *dOct, kind string) []int {
	var idx []int
	for j, it := range o.Items {
		if it != nil && it.Kind == kind {
			idx = append(idx, j)
		}
	}
	return idx
}

// sectionThing is the thing whose title is being written (partThing), or nil.
func (d *designState) sectionThing() *dOct {
	o := d.byID(d.sect.box)
	if !d.sect.on || o == nil {
		return nil
	}
	if c := d.sect.cur; (c.kind == secThing || c.kind == secPin) && c.at >= 0 && c.at < len(o.Items) {
		return o.Items[c.at]
	}
	return nil
}

// sectionBox is the container Tab steps through: the one it is stepping
// through, else the chosen container (or the rack, or board, a thing is
// open from), or nil.
func (m *Model) sectionBox() *dOct {
	d := m.ds()
	if d.sect.on {
		if o := d.byID(d.sect.box); o != nil && d.sel == d.sect.sel {
			return o
		}
		m.sectionEnd(true)
	}
	if d.opened.active && d.sel == d.opened.id {
		if r := d.byID(d.opened.rack); r != nil {
			return r
		}
	}
	if o := d.byID(d.sel); o != nil && isContainer(o.Kind) {
		return o
	}
	return nil
}

// designSectionTab is Tab (back: Shift+Tab) on a container: its next
// section (or the one before). It says whether it took the key.
func (m *Model) designSectionTab(back bool) bool {
	d := m.ds()
	o := m.sectionBox()
	if o == nil {
		return false
	}
	step := 1
	if back {
		step = -1
	}
	if !d.sect.on || d.sect.box != o.ID {
		m.sectionBegin(o)
	}
	list := d.sections(o)
	n := len(list)
	i := -1
	for k, s := range list {
		if s == d.sect.cur {
			i = k
		}
	}
	var next int
	switch {
	case i >= 0:
		next = ((i+step)%n + n) % n
	case d.sect.cur.kind < 0 && back: // the first Shift+Tab: the last section
		next = n - 1
	case d.sect.cur.kind < 0: // the first Tab: its name
		next = 0
	case back: // the section reached is gone (taken out): the one before its place
		next = ((d.sect.i-1)%n + n) % n
	default: // or the one now in its place
		next = d.sect.i % n
	}
	m.sectionGo(o, list[next], next)
	return true
}

// sectionBegin starts stepping through container o, noting how it is. A
// thing open from it (a rack's row, a board's pin), a chosen pin or a
// chosen thing in a box is where the stepping starts from.
func (m *Model) sectionBegin(o *dOct) {
	d := m.ds()
	s := sectState{on: true, box: o.ID, sel: d.sel, cur: section{kind: -1}}
	switch {
	case d.opened.active && d.opened.rack == o.ID && o.Kind == kRack:
		s.cur = section{kind: secRow, at: d.opened.at}
	case d.opened.active && d.opened.rack == o.ID && o.Kind == kBoard:
		at := d.opened.at
		m.designRackClose() // back on its pin, which is chosen next
		s.cur, s.sel = section{kind: secPin, at: at}, o.ID
	case d.pin.active && d.pin.board == o.ID:
		s.cur = section{kind: secPin, at: d.pin.at}
	case d.boxPick.active && d.boxPick.box == o.ID && d.boxPick.at < len(o.Items) && o.Items[d.boxPick.at] != nil:
		s.cur = section{kind: secThing, slot: boxLayerOf(o.Items[d.boxPick.at]), at: d.boxPick.at}
	}
	s.found.open = o.DrawerOpen
	if o.Kind == kDrawer {
		for i := range drawerCount(o) {
			s.found.pulled = append(s.found.pulled, slotOpen(o, i))
		}
	}
	s.found.view = d.boxView(o)
	d.sect = s
}

// sectionGo moves to section s (the i-th) of container o: the container
// shows it, it is written in (if it has words), and the view goes to it.
func (m *Model) sectionGo(o *dOct, s section, i int) {
	d := m.ds()
	d.sect.cur, d.sect.i = s, i
	m.sectionPose(o, s)
	switch s.kind {
	case secName:
		d.sel, d.editing = o.ID, partHeader
	case secRow:
		d.editing = partNone
		if it := d.openIn(o); it != nil {
			d.sel = it.ID
			if hasPart(it, partHeader) {
				d.editing = partHeader
			}
		}
	case secThing, secPin:
		d.sel, d.editing = o.ID, partThing
		if d.sectionThing() == nil {
			d.editing = partNone
		}
	default:
		d.sel, d.editing = o.ID, partNone
	}
	d.selLink = -1
	d.sect.sel = d.sel
	m.sectionShow(o)
	m.msg = "" // the status bar says where Tab is (sectionHint)
	d.dirty = true
}

// sectionPose makes container o show section s: on its name, as Tab found
// it; inside, the doors or lid open, the drawer reached out, the row open,
// the pin or thing chosen, the layer looked at.
func (m *Model) sectionPose(o *dOct, s section) {
	d := m.ds()
	if s.kind == secName {
		m.sectionPutBack(o)
		return
	}
	switch o.Kind {
	case kCabinet, kBox:
		if !o.DrawerOpen {
			o.DrawerOpen = true
			d.startOpenAnim(o)
		}
	}
	switch o.Kind {
	case kDrawer:
		for j := range drawerCount(o) {
			if want := j == s.slot; slotOpen(o, j) != want {
				setSlotOpen(o, j, want)
				d.startSlotAnim(o, j, want)
			}
		}
		d.toFront(o)
	case kRack:
		if !(d.opened.active && d.opened.rack == o.ID && d.opened.at == s.at) {
			m.designRackClose()
			m.designRackOpen(o, s.at)
		}
	case kBoard:
		d.pin = pinSel{board: o.ID, at: s.at, active: true}
	case kBox:
		d.boxPick = boxSel{}
		if s.kind == secThing {
			d.boxPick = boxSel{box: o.ID, at: s.at, active: true}
		}
		d.setBoxView(o, s.slot+1) // its layer: the layers above it see-through
		d.toFront(o)
	case kCabinet:
		d.toFront(o)
		if s.kind == secThing { // a shelf showing other things: from this one
			m.sectionShelfShows(o, s)
		}
	}
}

// sectionPutBack puts container o back as Tab found it.
func (m *Model) sectionPutBack(o *dOct) {
	d := m.ds()
	f := d.sect.found
	switch o.Kind {
	case kCabinet, kBox:
		if o.DrawerOpen != f.open {
			o.DrawerOpen = f.open
			d.startOpenAnim(o)
		}
		if o.Kind == kBox {
			d.boxPick = boxSel{}
			d.setBoxView(o, f.view)
		}
	case kDrawer:
		for j := range drawerCount(o) {
			want := j < len(f.pulled) && f.pulled[j]
			if slotOpen(o, j) != want {
				setSlotOpen(o, j, want)
				d.startSlotAnim(o, j, want)
			}
		}
	case kRack:
		if d.opened.active && d.opened.rack == o.ID {
			m.designRackClose()
		}
	case kBoard:
		d.pin = pinSel{}
	}
	d.dirty = true
}

// sectionShelfShows makes cabinet o's shelf show thing s, starting from it
// when it is not among those showing.
func (m *Model) sectionShelfShows(o *dOct, s section) {
	d := m.ds()
	idx := shelfThings(o, s.stack)
	at := -1
	for n, j := range idx {
		if j == s.at {
			at = n
		}
	}
	if at < 0 {
		return
	}
	if _, _, _, _, ok := d.sectionRect(o, s); ok {
		return
	}
	if d.shelfAt == nil {
		d.shelfAt = map[string]int{}
	}
	d.shelfAt[shelfKey(o, s.stack)] = at
}

// sectionEnd ends Tab's stepping; restore: the container is put back as
// Tab found it (an arrow, Esc, another structure chosen), else it is left
// as it is (a click: it acts on what it sees).
func (m *Model) sectionEnd(restore bool) {
	d := m.ds()
	if !d.sect.on {
		return
	}
	if o := d.byID(d.sect.box); o != nil && restore {
		keep := d.sel
		m.sectionPutBack(o)
		if keep != d.sect.sel && d.byID(keep) != nil {
			d.sel = keep // another structure was chosen: it stays chosen
		}
	}
	d.sect = sectState{}
	if d.editing == partThing {
		d.editing = partNone
	}
	d.dirty = true
}

// sectionGuard runs before each key in Design mode: an arrow (or Alt+N,
// Ctrl+F) ends Tab's stepping, the container put back, and the key does
// its work; another structure chosen ends it the same way; Esc (and Alt
// tapped twice) only stops the writing, leaving what Tab reached chosen
// and open; undo and redo end it, leaving the board as they make it. It
// says whether it took the key.
func (m *Model) sectionGuard(name string) bool {
	d := m.ds()
	if !d.sect.on {
		return false
	}
	switch {
	case d.byID(d.sect.box) == nil:
		d.sect = sectState{}
		if d.editing == partThing {
			d.editing = partNone
		}
	case name == "ctrl+z" || name == "ctrl+y" || name == "ctrl+shift+z" || name == "ctrl+Z":
		d.sect = sectState{}
		if d.editing == partThing {
			d.editing = partNone
		}
	case d.sel != d.sect.sel:
		m.sectionEnd(true)
	case name == "esc" && d.editing != partNone:
		// Writing stops; the stepping goes on, so an arrow still puts the
		// container back as Tab found it (a pin: R S T … work on it now)
		d.editing = partNone
		m.say("Stopped writing.")
		return true
	case name == "esc":
		m.sectionEnd(false) // not writing: the stepping ends, what Tab reached stays chosen and open
		return true
	case d.editing == partNone && (name == "left" || name == "right" || name == "up" || name == "down" ||
		name == "shift+left" || name == "shift+right" || name == "shift+up" || name == "shift+down"),
		(stopsWriting(name) && !strings.HasPrefix(name, "alt+")) || name == "alt+n" || name == "ctrl+f":
		// (an arrow while writing moves the cursor: the stepping goes on)
		// (Alt + arrows move what is chosen, a pin or a thing in a box with it: the stepping goes on)
		m.sectionEnd(true)
	}
	return false
}

// sectionDrop: a click ends Tab's stepping, leaving the container as it
// shows (the click acts on what is under it).
func (m *Model) sectionDrop() {
	m.sectionEnd(false)
}

// sectionWrite types into the title of the thing reached (partThing).
// Enter goes on to the next section, as Tab does. It says whether it took
// the key: another key (PgUp, PgDn, Alt + arrows …) stops the writing and
// does its usual work, to the chosen pin or thing.
func (m *Model) sectionWrite(k tea.Key, name string) bool {
	d := m.ds()
	it := d.sectionThing()
	if it == nil {
		d.editing = partNone
		return false
	}
	switch name {
	case "enter":
		m.designSectionTab(false)
		return true
	case "backspace":
		if r := []rune(it.Header); len(r) > 0 {
			it.Header = string(r[:len(r)-1])
		}
	case "ctrl+u", "ctrl+backspace", "alt+backspace":
		it.Header = ""
	default:
		if k.Text == "" || k.Mod.Contains(tea.ModCtrl) || k.Mod.Contains(tea.ModAlt) {
			d.editing = partNone // writing stops (the thing stays chosen; Tab goes on), and the key does its work
			return false
		}
		it.Header += strings.ReplaceAll(textLike(k.Text), "\n", " ")
	}
	d.dirty = true
	return true
}

// sectionPaste puts pasted text into the title of the thing reached.
func (m *Model) sectionPaste(text string) {
	d := m.ds()
	if it := d.sectionThing(); it != nil {
		it.Header += strings.ReplaceAll(textLike(text), "\n", " ")
		d.dirty = true
	}
}

// ---- where a section is, and the view going to it ---------------------------------------

// settled runs f with every door, lid and drawer drawn where it is going,
// not where it is in its move: the view goes to where a section will be.
func (d *designState) settled(f func()) {
	a := d.anims
	d.anims = nil
	f()
	d.anims = a
}

// sectionRect is where section s of container o is on the screen (cells
// of the board area), worked out from how it is drawn. ok is false when it
// does not show (zoomed out, it may be cut).
func (d *designState) sectionRect(o *dOct, s section) (x, y, w, h int, ok bool) {
	ox, oy, ow, oh := d.rect(o)
	box := func(x0, y0, x1, y1 int) (int, int, int, int, bool) { // a box round cells x0 ≤ x ≤ x1, y0 ≤ y ≤ y1
		if x1 < x0 || y1 < y0 {
			return 0, 0, 0, 0, false
		}
		return x0, y0, x1 - x0 + 1, y1 - y0 + 1, true
	}
	switch s.kind {
	case secName:
		return ox, oy, ow, oh, true
	case secRow:
		it := d.openIn(o)
		if it == nil {
			return 0, 0, 0, 0, false
		}
		ix, iy, iw, ih := d.rect(it)
		return ix - 1, iy - 1, iw + 2, ih + 1, true // with its row, above it
	case secPin:
		if s.at < 0 || s.at >= len(o.Items) || o.Items[s.at] == nil {
			return 0, 0, 0, 0, false
		}
		p := d.pinMapOf(o, o.Items[s.at])
		x0, y0, x1, y1 := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
		for _, c := range [][2]float64{{0, 0}, {float64(p.gw), 0}, {0, float64(p.gh)}, {float64(p.gw), float64(p.gh)}} {
			sx, sy := p.toScreen(c[0], c[1])
			x0, y0, x1, y1 = math.Min(x0, sx), math.Min(y0, sy), math.Max(x1, sx), math.Max(y1, sy)
		}
		return box(ox+int(math.Floor(x0)), oy+int(math.Floor(y0)), ox+int(math.Ceil(x1))-1, oy+int(math.Ceil(y1))-1)
	}
	switch o.Kind {
	case kDrawer:
		if ow < 8 || oh < 3 {
			return 0, 0, 0, 0, false
		}
		l := d.drawerLayout(o, ow, oh)
		y0, y1 := math.MaxInt, -1
		if s.kind == secDrawer {
			for li, ln := range l.lines {
				if ln.slot == s.slot {
					y0, y1 = min(y0, li), max(y1, li)
				}
			}
			return box(ox, oy+y0, ox+ow-1, oy+y1)
		}
		if s.slot < 0 || s.slot >= len(l.slots) {
			return 0, 0, 0, 0, false
		}
		sl := l.slots[s.slot]
		k := -1
		for n, j := range sl.things {
			if j == s.at {
				k = n
			}
		}
		if k < 0 || k/l.cols >= sl.fit {
			return 0, 0, 0, 0, false
		}
		mr, mc := k/l.cols, k%l.cols
		for li, ln := range l.lines {
			if ln.kind == dlTray && ln.slot == s.slot && ln.row/sl.mh == mr {
				y0, y1 = min(y0, li), max(y1, li)
			}
		}
		x0 := ox + 1 + 2 + mc*(l.mw+1)
		return box(x0, oy+y0, x0+l.mw-1, oy+y1)
	case kCabinet:
		if ow < 10 || oh < 5 || !o.DrawerOpen {
			return 0, 0, 0, 0, false
		}
		sw := cabShelfW(ow)
		rows := d.cabinetShelvesFit(o, sw, oh-4)
		left := ox + 1 + cabDoorW
		x0, y0, x1, y1 := math.MaxInt, math.MaxInt, -1, -1
		for r, row := range rows {
			at := left
			for _, sg := range row {
				sw := lipgloss.Width(sg.text)
				mine := false
				switch {
				case s.kind == secThing:
					mine = sg.act == "item:"+itoa(s.at)
				case s.kind == secShelf:
					mine = sg.act == "new:"+s.stack || sg.act == "shelf:"+s.stack ||
						(strings.HasPrefix(sg.act, "item:") && thingKind(o, sg.act) == s.stack)
				}
				if mine {
					x0, y0, x1, y1 = min(x0, at), min(y0, oy+3+r), max(x1, at+sw-1), max(y1, oy+3+r)
				}
				at += sw
			}
		}
		if s.kind == secShelf && y1 >= 0 { // the whole shelf: its things' row and its board, as wide as the shelves
			if y0 == y1 && y0-oy-3 > 0 && rows[y0-oy-3-1] != nil && rows[y0-oy-3-1][0].look != lookBoard {
				y0-- // its things' row, above its board
			}
			x0, x1 = left, left+sw-1
		}
		return box(x0, y0, x1, y1)
	case kBox:
		if ow < 12 || oh < 5 {
			return 0, 0, 0, 0, false
		}
		g := d.boxGridOf(o, ow, oh)
		want := map[string]bool{}
		if s.kind == secThing {
			want["boxthing:"+itoa(s.at)] = true
		} else {
			for _, j := range boxLayerThings(o, s.slot) {
				want["boxthing:"+itoa(j)] = true
			}
		}
		x0, y0, x1, y1 := math.MaxInt, math.MaxInt, -1, -1
		for cy := range g.c {
			for cx := range g.c[cy] {
				if want[g.c[cy][cx].act] {
					x0, y0, x1, y1 = min(x0, cx), min(y0, cy), max(x1, cx), max(y1, cy)
				}
			}
		}
		return box(ox+x0, oy+y0, ox+x1, oy+y1)
	}
	return 0, 0, 0, 0, false
}

// thingKind is the kind of cabinet o's thing that act ("item:j") is.
func thingKind(o *dOct, act string) string {
	j := 0
	for _, c := range strings.TrimPrefix(act, "item:") {
		j = j*10 + int(c-'0')
	}
	if j < 0 || j >= len(o.Items) || o.Items[j] == nil {
		return ""
	}
	return o.Items[j].Kind
}

// sectionShow moves the view to the section reached: on the flat board
// it is centred (one taller than the screen shows from its top); in 3D the
// camera stands square in front of it.
func (m *Model) sectionShow(o *dOct) {
	d := m.ds()
	var x, y, w, h int
	ok := false
	d.settled(func() {
		x, y, w, h, ok = d.sectionRect(o, d.sect.cur)
		if !ok {
			x, y, w, h = d.rect(o)
		}
	})
	if d.room.on() {
		m.design3DFaceRect(o, x, y, x+w, y+h, sectionKeep)
		return
	}
	z := d.z()
	d.camX += (float64(x) + float64(w)/2 - float64(m.width)/2) / z
	if h > m.boardH()-2 {
		d.camY += float64(y-1) / z
	} else {
		d.camY += (float64(y) + float64(h)/2 - float64(m.boardH())/2) / z
	}
	d.dirty = true
}

// sectionKeep: in 3D, facing a small section the camera stays at least
// this share of the way it stands from the whole container, so what is
// round it still shows.
const sectionKeep = 0.5

// ---- what it says --------------------------------------------------------------------------

// sectionWhat names the section reached.
func (d *designState) sectionWhat(o *dOct) string {
	s := d.sect.cur
	switch s.kind {
	case secName:
		return "the name of " + octName(o)
	case secDrawer:
		return drawerName(o, s.slot) + " (" + plural(len(slotThings(o, s.slot)), "thing") + ")"
	case secShelf:
		return "the " + stackNames[s.stack] + " shelf of " + octName(o) + " (" + itoa(len(shelfThings(o, s.stack))) + ")"
	case secLayer:
		return "the " + boxLayerNames[s.slot] + " layer of " + octName(o) + " (" + plural(len(boxLayerThings(o, s.slot)), "thing") + ")"
	case secRow:
		if it := d.openIn(o); it != nil {
			return "“" + itemTitle(it) + "”, open in " + octName(o)
		}
		return "a row of " + octName(o)
	case secThing, secPin:
		if it := d.sectionThing(); it != nil {
			return "“" + itemTitle(it) + "” in " + octName(o)
		}
	}
	return octName(o)
}

// sectionHint is the status bar's line while Tab steps through a container.
func (d *designState) sectionHint() string {
	o := d.byID(d.sect.box)
	if o == nil {
		return ""
	}
	verb := "On"
	if d.editing != partNone {
		verb = "Writing"
	}
	where := " (" + itoa(d.sect.i+1) + " of " + itoa(len(d.sections(o))) + ")"
	return verb + " " + d.sectionWhat(o) + where + "  ·  Tab: next  ·  Shift+Tab: back  ·  Esc: done"
}
