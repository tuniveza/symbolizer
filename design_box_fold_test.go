package main

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// boxWithCabinet is boxTest's box with a cabinet, Archive, on its top
// layer (thing 3), holding a page and a note.
func boxWithCabinet(t *testing.T) (*Model, *designState, *dOct, *dOct) {
	t.Helper()
	m, d, b := boxTest(t)
	cab := &dOct{ID: 910, Kind: kCabinet, Header: "Archive", Color: "#d19a66", X: 24, Y: 8, Slot: 0, Items: []*dOct{
		{ID: 911, Kind: kPage, Header: "Recipe", Color: "#61afef"},
		{ID: 912, Kind: kNote, Header: "Lamp", Color: "#98c379"},
	}}
	b.Items = append(b.Items, cab)
	d.dirty = true
	d.save()
	m.View()
	return m, d, b, cab
}

// actRect is the smallest rectangle round every cell of g that does act
// (w = 0: none does), and how many cells do.
func actRect(g *boxGrid, act string) (x0, y0, w, h, n int) {
	x0, y0, x1, y1 := g.w, g.h, -1, -1
	for y := range g.h {
		for x := range g.w {
			if g.c[y][x].act == act {
				x0, y0, x1, y1 = min(x0, x), min(y0, y), max(x1, x), max(y1, y)
				n++
			}
		}
	}
	if x1 < 0 {
		return 0, 0, 0, 0, 0
	}
	return x0, y0, x1 - x0 + 1, y1 - y0 + 1, n
}

// A container in a box lies folded: a tile lower than a miniature, its
// name and the icons of what it holds. A click opens it out where it lies,
// over the other things, listing what it holds, and what is clicked is
// what is drawn; Esc, a click on another thing, or Tab on folds it again.
// Open or folded is not a change: no undo step, nothing saved.
func TestDesignBoxContainerFolds(t *testing.T) {
	m, d, b, cab := boxWithCabinet(t)
	act := "boxthing:3"
	_, _, w, h := d.rect(b)
	g := d.boxGridOf(b, w, h)
	rx, ry, rw, rh := d.boxThingRect(b, cab, w, h)
	x0, y0, tw, th, _ := actRect(g, act)
	if tw != miniW || th != boxTileH || x0 != rx || y0 != ry || tw != rw || th != rh {
		t.Fatalf("folded, it should be a %d×%d tile where it is drawn (%d,%d %d×%d): cells at %d,%d %d×%d",
			miniW, boxTileH, rx, ry, rw, rh, x0, y0, tw, th)
	}
	s := screenOf(m)
	if !strings.Contains(s, "▸"+kindIcons[kCabinet]+" Archive") || strings.Contains(s, "Recipe") {
		t.Fatalf("folded, it should show ▸, its icon and name, not what it holds:\n%s", s)
	}
	if !strings.Contains(s, kindIcons[kPage]+" "+kindIcons[kNote]) {
		t.Fatalf("folded, it should show the icons of what it holds:\n%s", s)
	}
	undo := len(d.undo)
	ox, oy, os := cab.X, cab.Y, cab.Slot

	// A click opens it.
	d.lastClick = time.Time{}
	cx, cy := boxCellOf(t, d, b, act)
	click(m, cx, cy+m.boardTop(), tea.MouseLeft)
	if _, it := d.chosenBoxThing(); it != cab || !d.boxThingOpen(b, cab) {
		t.Fatal("a click on the folded cabinet should choose it and open it")
	}
	s = screenOf(m)
	if !strings.Contains(s, "Recipe") || !strings.Contains(s, "Lamp") || !strings.Contains(s, "▾ "+kindIcons[kCabinet]+" Archive") {
		t.Fatalf("open, it should list what it holds under its name:\n%s", s)
	}
	g = d.boxGridOf(b, w, h)
	rx, ry, rw, rh = d.boxThingRect(b, cab, w, h)
	x0, y0, tw, th, n := actRect(g, act)
	if tw != boxOpenW || th < miniH || x0 != rx || y0 != ry || tw != rw || th != rh || n != rw*rh {
		t.Fatalf("open, every cell of it should be where it is drawn (%d,%d %d×%d), over the rest: %d,%d %d×%d, %d cells",
			rx, ry, rw, rh, x0, y0, tw, th, n)
	}
	// A click on a part of it that was not there folded keeps it chosen.
	d.lastClick = time.Time{}
	bx, by, _, _ := d.rect(b)
	click(m, bx+rx+rw-2, by+ry+rh-1+m.boardTop(), tea.MouseLeft)
	if _, it := d.chosenBoxThing(); it != cab {
		t.Fatal("a click on the open card should keep the cabinet chosen")
	}
	if cab.X != ox || cab.Y != oy || cab.Slot != os || len(d.undo) != undo {
		t.Fatalf("opening is only looking: %d,%d slot %d, %d undo steps (was %d,%d slot %d, %d)", cab.X, cab.Y, cab.Slot, len(d.undo), ox, oy, os, undo)
	}

	// Esc folds it again.
	press(m, "esc")
	if d.boxThingOpen(b, cab) || strings.Contains(screenOf(m), "Recipe") {
		t.Fatal("Esc should fold it again")
	}

	// Tab opens it when it comes to it, and folds it on the next thing.
	d.sel, d.boxPick = b.ID, boxSel{}
	found := false
	for range len(b.Items) {
		press(m, "tab")
		if _, it := d.chosenBoxThing(); it == cab {
			found = true
			break
		}
	}
	if !found || !d.boxThingOpen(b, cab) {
		t.Fatal("Tab should come to the cabinet and open it")
	}
	press(m, "tab")
	if _, it := d.chosenBoxThing(); it == cab || d.boxThingOpen(b, cab) {
		t.Fatal("Tab to the next thing should fold the cabinet")
	}
	if len(d.undo) != undo {
		t.Fatalf("folding and opening should be no undo step: %d, was %d", len(d.undo), undo)
	}
}

// Open, it moves by its open size: PgDn takes it a layer down (still over
// the rest while chosen, still hit where drawn), Alt+arrows stop at the
// box's walls with all of it inside.
func TestDesignBoxContainerOpenMoves(t *testing.T) {
	m, d, b, cab := boxWithCabinet(t)
	m.designBoxPick(b, 3)
	press(m, "pgdown")
	if cab.Slot != 1 || !d.boxThingOpen(b, cab) {
		t.Fatalf("PgDn should take it to the middle layer, still open: slot %d", cab.Slot)
	}
	_, _, w, h := d.rect(b)
	rx, ry, rw, rh := d.boxThingRect(b, cab, w, h)
	if x0, y0, tw, th, n := actRect(d.boxGridOf(b, w, h), "boxthing:3"); x0 != rx || y0 != ry || tw != rw || th != rh || n != rw*rh {
		t.Fatalf("on the middle layer, open, it should be hit where drawn (%d,%d %d×%d): %d,%d %d×%d, %d cells", rx, ry, rw, rh, x0, y0, tw, th, n)
	}
	for range 100 {
		press(m, "alt+shift+right")
		press(m, "alt+shift+down")
	}
	if mx, my := boxRoomOf(b, cab, true); cab.X != mx || cab.Y != my {
		t.Fatalf("Alt+arrows should stop it open at the walls: %d,%d, want %d,%d", cab.X, cab.Y, mx, my)
	}
	ix, iy, iw, ih, _, _ := boxPlaces(b, w, h)
	rx, ry, rw, rh = d.boxThingRect(b, cab, w, h)
	if rx < ix || ry < iy || rx+rw > ix+iw || ry+rh > iy+ih {
		t.Fatalf("open in the corner, it should still be inside: %d,%d %d×%d in %d,%d %d×%d", rx, ry, rw, rh, ix, iy, iw, ih)
	}
}

// A box takes containers (a drawer or rack still does not): a box dragged
// onto it goes in, chosen and open; Esc folds it; a drag on it takes it
// out again, with what it holds.
func TestDesignBoxTakesContainer(t *testing.T) {
	m, d, b := boxTest(t)
	if !accepts(b, &dOct{Kind: kCabinet}) || accepts(&dOct{Kind: kRack}, &dOct{Kind: kCabinet}) || accepts(b, b) {
		t.Fatal("a box should take a container; a rack should not; a box not itself")
	}
	in := d.newKind(kBox, 0, 0)
	in.Header, in.X, in.Y = "Tins", 52, 1
	in.Items = []*dOct{{ID: 920, Kind: kNote, Header: "Bolt", Color: "#e5c07b"}}
	d.sel = -1
	m.View()
	px, py, _, ph := d.rect(in)
	bx, by, _, _ := d.rect(b)
	if act := d.boxHit(in, 0, ph/2, 46, 17); act != "" {
		t.Fatalf("the inner box's left wall should move it, not %q", act)
	}
	dragTo(m, px, py+ph/2+m.boardTop(), bx+20, by+9+m.boardTop(), tea.MouseLeft)
	j := itemIndex(b, in)
	if j < 0 || d.byID(in.ID) != nil {
		t.Fatal("the box Tins dragged onto Toys should go in")
	}
	if _, it := d.chosenBoxThing(); it != in || !d.boxThingOpen(b, in) || !strings.Contains(screenOf(m), "Bolt") {
		t.Fatalf("put in, it should be chosen and open, showing what it holds:\n%s", screenOf(m))
	}
	press(m, "esc")
	if d.boxThingOpen(b, in) || strings.Contains(screenOf(m), "Bolt") {
		t.Fatal("Esc should fold it")
	}
	d.lastClick = time.Time{}
	cx, cy := boxCellOf(t, d, b, "boxthing:"+itoa(j))
	dragTo(m, cx, cy+m.boardTop(), 80, 22+m.boardTop(), tea.MouseLeft)
	if itemIndex(b, in) >= 0 || d.byID(in.ID) == nil || len(in.Items) != 1 {
		t.Fatal("a drag on it should take it out, with what it holds")
	}
}
