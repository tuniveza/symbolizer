package main

import (
	"math"
	"strings"
	"testing"
)

// sectLabel names the section Tab has reached, for the tests.
func sectLabel(d *designState) string {
	o := d.byID(d.sect.box)
	if !d.sect.on || o == nil {
		return "off"
	}
	s := d.sect.cur
	switch s.kind {
	case secName:
		return "name"
	case secDrawer:
		return "drawer:" + itoa(s.slot+1)
	case secShelf:
		return "shelf:" + s.stack
	case secLayer:
		return "layer:" + boxLayerNames[s.slot]
	case secRow:
		if it := d.openIn(o); it != nil {
			return "row:" + it.Header
		}
		return "row?"
	}
	if it := d.sectionThing(); it != nil {
		return it.Header
	}
	return "?"
}

// fileIn puts a new thing of kind called name into container c (in drawer
// or layer slot).
func fileIn(m *Model, c *dOct, kind, name string, slot int) *dOct {
	d := m.ds()
	it := d.newKind(kind, c.X+2, c.Y+2)
	it.Header = name
	m.designFileInto(it, c)
	it.Slot = slot
	return it
}

// tabs presses Tab (or Shift+Tab) n times and says where each one went.
func tabs(m *Model, key string, n int) string {
	var seen []string
	for range n {
		press(m, key)
		seen = append(seen, sectLabel(m.ds()))
	}
	return strings.Join(seen, " ")
}

// sectionsScene: a cabinet, a drawer unit of two drawers, a rack, a board
// and a box, each holding things, apart on the board.
func sectionsScene(t *testing.T) (*Model, map[string]*dOct) {
	m := designTest(t)
	d := m.ds()
	d.camX, d.camY, d.zoom = 0, 0, designZoom100
	c := map[string]*dOct{}
	for i, k := range []string{kCabinet, kDrawer, kRack, kBoard, kBox} {
		o := d.newKind(k, 0, 0)
		o.X, o.Y, o.Header = 4+i*60, 4, kindNames[k]
		c[k] = o
	}
	fileIn(m, c[kCabinet], kPage, "Plan", 0)
	fileIn(m, c[kCabinet], kNote, "Call", 0)
	fileIn(m, c[kCabinet], kPage, "Ideas", 0)
	c[kCabinet].DrawerOpen = false
	c[kDrawer].Drawers = 2
	fileIn(m, c[kDrawer], kPage, "Saw", 0)
	fileIn(m, c[kDrawer], kNote, "Nails", 1)
	fileIn(m, c[kDrawer], kNote, "Glue", 1)
	c[kDrawer].Pulled, c[kDrawer].DrawerOpen = nil, false
	fileIn(m, c[kRack], kPage, "One", 0)
	fileIn(m, c[kRack], kNote, "Two", 0)
	fileIn(m, c[kBoard], kNote, "PinA", 0)
	fileIn(m, c[kBoard], kPage, "PinB", 0)
	fileIn(m, c[kBox], kNote, "Ball", 2)
	fileIn(m, c[kBox], kPage, "Idea", 0)
	c[kBox].DrawerOpen = false
	d.sel, d.editing, d.boxPick, d.pin = -1, partNone, boxSel{}, pinSel{}
	m.View()
	return m, c
}

// Tab steps through every section of each kind of container, in order,
// and round from the last to its name; Shift+Tab goes back.
func TestSectionsOrder(t *testing.T) {
	m, c := sectionsScene(t)
	d := m.ds()
	for _, k := range []struct{ kind, want string }{
		{kCabinet, "name shelf:page Plan Ideas shelf:note Call name"},
		{kDrawer, "name drawer:1 Saw drawer:2 Nails Glue name"},
		{kRack, "name row:One row:Two name"},
		{kBoard, "name PinA PinB name"},
		{kBox, "name layer:top Idea layer:bottom Ball name"},
	} {
		d.sel = c[k.kind].ID
		n := len(strings.Fields(k.want))
		if got := tabs(m, "tab", n); got != k.want {
			t.Fatalf("%s: Tab went %q, want %q", k.kind, got, k.want)
		}
		press(m, "esc")
		press(m, "esc")
		d.sel = c[k.kind].ID
		f := strings.Fields(k.want)
		var back []string
		for i := n - 2; i >= 0; i-- {
			back = append(back, f[i])
		}
		if got := tabs(m, "shift+tab", n-1); got != strings.Join(back, " ") {
			t.Fatalf("%s: Shift+Tab went %q, want %q", k.kind, got, strings.Join(back, " "))
		}
		press(m, "left") // leave it: put back as it was
		if d.sect.on {
			t.Fatalf("%s: an arrow should end the stepping", k.kind)
		}
		d.sel = -1
	}
}

// Each section is written in by itself: the name writes the container's
// name, a thing its own title; Enter goes on like Tab.
func TestSectionsWriteEach(t *testing.T) {
	m, c := sectionsScene(t)
	d := m.ds()
	u := c[kDrawer]
	d.sel = u.ID
	press(m, "tab")
	typeKeys(m, "s")
	if u.Header != kindNames[kDrawer]+"s" {
		t.Fatalf("typing on its name: %q", u.Header)
	}
	press(m, "tab") // drawer 1: no words of its own
	if d.editing != partNone || !slotOpen(u, 0) {
		t.Fatalf("drawer 1: writing %d, out %v", d.editing, u.Pulled)
	}
	typeKeys(m, "c") // (a drawer's colour: what C does with it chosen)
	press(m, "tab")
	typeKeys(m, "s!")
	press(m, "backspace")
	saw := d.sectionThing()
	if saw == nil || saw.Header != "Saws" || u.Header != kindNames[kDrawer]+"s" {
		t.Fatalf("typing on a thing in the drawer: %v, the unit %q", saw, u.Header)
	}
	if !strings.Contains(screenOf(m), "Saws") {
		t.Fatal("the thing's new title is not drawn")
	}
	press(m, "enter")
	if sectLabel(d) != "drawer:2" || slotOpen(u, 0) || !slotOpen(u, 1) {
		t.Fatalf("Enter on a thing: %s, drawers %v", sectLabel(d), u.Pulled)
	}
	// A pin's title, and a rack's row (open in the rack, written as itself).
	d.sel = c[kBoard].ID
	tabs(m, "tab", 2)
	typeKeys(m, "!")
	if c[kBoard].Items[0].Header != "PinA!" {
		t.Fatalf("a pin's title: %q", c[kBoard].Items[0].Header)
	}
	press(m, "esc")
	press(m, "esc")
	d.sel = c[kRack].ID
	tabs(m, "tab", 2)
	typeKeys(m, "!")
	if it := d.byID(d.sel); it == nil || it.Header != "One!" || !d.opened.active {
		t.Fatalf("a rack's row: %v open %v", it, d.opened.active)
	}
	press(m, "esc")
	if !d.opened.active {
		t.Fatal("Esc stops writing, the row still open")
	}
}

// Every step moves the view to the section: its middle in the middle of
// the screen.
func TestSectionsViewFollows(t *testing.T) {
	m, c := sectionsScene(t)
	d := m.ds()
	for _, kind := range []string{kCabinet, kDrawer, kRack, kBoard, kBox} {
		o := c[kind]
		d.sel = o.ID
		for i := range len(d.sections(o)) {
			press(m, "tab")
			x, y, w, h, ok := d.sectionRect(o, d.sect.cur)
			if !ok {
				t.Fatalf("%s, Tab %d (%s): the section does not show", kind, i+1, sectLabel(d))
			}
			cx, cy := float64(x)+float64(w)/2, float64(y)+float64(h)/2
			if math.Abs(cx-float64(m.width)/2) > 1 || (h <= m.boardH()-2 && math.Abs(cy-float64(m.boardH())/2) > 1) {
				t.Fatalf("%s, Tab %d (%s): its middle is at %.1f,%.1f, the screen's %d,%d", kind, i+1, sectLabel(d), cx, cy, m.width/2, m.boardH()/2)
			}
		}
		press(m, "left")
		d.sel = -1
	}
}

// Leaving by an arrow puts the container back as Tab found it: doors
// shut, drawers in, the rack's row back, the box's lid shut; Esc leaves
// it as it is.
func TestSectionsPutBack(t *testing.T) {
	m, c := sectionsScene(t)
	d := m.ds()
	cab, u, r, b := c[kCabinet], c[kDrawer], c[kRack], c[kBox]
	d.sel = cab.ID
	tabs(m, "tab", 3)
	if !cab.DrawerOpen {
		t.Fatal("inside, the doors are open")
	}
	press(m, "down")
	if cab.DrawerOpen || d.sect.on || d.editing != partNone {
		t.Fatal("an arrow: the doors shut, the stepping and the writing end")
	}
	d.sel = u.ID
	tabs(m, "tab", 4)
	press(m, "right")
	if u.DrawerOpen {
		t.Fatalf("an arrow: the drawers go back in: %v", u.Pulled)
	}
	d.sel = r.ID
	tabs(m, "tab", 2)
	press(m, "right")
	if d.opened.active || len(r.Items) != 2 {
		t.Fatal("an arrow: the rack's row goes back in")
	}
	d.sel = b.ID
	tabs(m, "tab", 3)
	press(m, "esc")
	if !b.DrawerOpen || d.sect.on {
		t.Fatal("Esc: the lid stays open, the stepping ends")
	}
	// Another structure chosen (a click, or the camera in 3D) ends it too, putting it back.
	b.DrawerOpen = false
	d.sel = u.ID
	tabs(m, "tab", 2)
	d.sel = cab.ID
	press(m, "f1")
	if u.DrawerOpen || d.sel != cab.ID {
		t.Fatalf("another chosen: the drawer goes in, the other stays chosen (%v)", u.Pulled)
	}
}

// In 3D Tab steps through the same sections, the drawer pulled out, and
// the camera stands square in front of each section, its middle in the
// middle of the view; the container stays chosen.
func TestSections3D(t *testing.T) {
	m := room3DModel(t, 140, 46)
	d := m.ds()
	r := &d.room
	press(m, "ctrl+f")
	var u *dOct
	for _, o := range d.octs {
		if o.Kind == kDrawer {
			u = o
		}
	}
	u.Drawers = 2
	fileIn(m, u, kPage, "Saw", 0)
	fileIn(m, u, kNote, "Nails", 1)
	u.Pulled, u.DrawerOpen = nil, false
	cell3D(t, m, u.ID, func(sx, sy int) bool { return true })
	r.lookBy(17, -9)
	d.sel = u.ID
	var seen []string
	poses := map[r3vec]bool{}
	for range 5 {
		press(m, "tab")
		seen = append(seen, sectLabel(d))
		boardRows(m)
		if d.sel != u.ID || r.look != [2]float64{0, 0} {
			t.Fatalf("%s: chosen %d, looks %v", sectLabel(d), d.sel, r.look)
		}
		poses[r.pos] = true
		var x, y, w, h int
		d.settled(func() { x, y, w, h, _ = d.sectionRect(u, d.sect.cur) })
		v, _ := r.nowView()
		var b r3block
		for _, bb := range r.blocks {
			if bb.o == u {
				b = bb
			}
		}
		rm := &r.rm
		front := b.off + b.dep
		q := r3vec{float64(x) + float64(w)/2 - rm.hw, rm.hh - float64(2*y+h), front}.sub(v.cam)
		cz := q.dot(v.fwd)
		px, py := v.px+v.f*q.dot(v.right)/cz, v.py-v.f*q.dot(v.up)/cz
		if math.Abs(px-v.px) > 0.5 || math.Abs(py-v.py) > 0.5 {
			t.Fatalf("%s: its middle at %.2f,%.2f on the screen, the middle is %.2f,%.2f", sectLabel(d), px, py, v.px, v.py)
		}
	}
	if got, want := strings.Join(seen, " "), "name drawer:1 Saw drawer:2 Nails"; got != want {
		t.Fatalf("in 3D Tab went %q, want %q", got, want)
	}
	if !slotOpen(u, 1) || slotOpen(u, 0) {
		t.Fatalf("in 3D the drawer reached is out: %v", u.Pulled)
	}
	if len(poses) < 4 {
		t.Fatalf("the camera should move to each section: %d places for 5", len(poses))
	}
	typeKeys(m, "s")
	if it := d.sectionThing(); it == nil || it.Header != "Nailss" || !r.on() {
		t.Fatal("in 3D, typing writes the thing's title")
	}
	press(m, "esc")
	if !r.on() || d.sect.on {
		t.Fatal("Esc in 3D stops writing, the 3D view stays")
	}
}
