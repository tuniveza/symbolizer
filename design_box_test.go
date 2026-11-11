package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// boxTest is a board with a box (its corner at board cell 2, 1, the view
// at 0, 0, 100%) holding a thing on each layer.
func boxTest(t *testing.T) (*Model, *designState, *dOct) {
	t.Helper()
	m := designTest(t)
	d := m.ds()
	d.camX, d.camY, d.zoom = 0, 0, designZoom100
	d.sel = -1
	placeKey(m, "o")
	press(m, "esc")
	var b *dOct
	for _, o := range d.octs {
		if o.Kind == kBox {
			b = o
		}
	}
	if b == nil {
		t.Fatal("Alt+N, O placed no box")
	}
	b.X, b.Y, b.Header = 2, 1, "Toys"
	b.Items = []*dOct{
		{ID: 901, Kind: kNote, Header: "Ball", Color: "#98c379", X: 2, Y: 1, Slot: 2},
		{ID: 902, Kind: kPage, Header: "Plan", Color: "#61afef", X: 8, Y: 3, Slot: 1},
		{ID: 903, Kind: kPage, Header: "Idea", Color: "#c678dd", X: 12, Y: 4, Slot: 0},
	}
	d.sel = -1
	d.dirty = true
	d.save()
	m.View()
	return m, d, b
}

// boxCellOf is a screen cell (canvas coordinates) of box b where a click
// does act, as the box is drawn now.
func boxCellOf(t *testing.T, d *designState, b *dOct, act string) (int, int) {
	t.Helper()
	x, y, w, h := d.rect(b)
	g := d.boxGridOf(b, w, h)
	for cy := range h {
		for cx := range w {
			if g.c[cy][cx].act == act && y+cy >= 0 && y+cy < 25 && x+cx >= 0 && x+cx < 100 {
				return x + cx, y + cy
			}
		}
	}
	t.Fatalf("no cell of the box does %q", act)
	return 0, 0
}

// A box is placed with Alt+N, O, open. A page dragged onto it goes in, on
// the top layer, its miniature where it was let go, and it is chosen; a
// drag on a thing takes it out.
func TestDesignBoxDropAndTakeOut(t *testing.T) {
	m, d, b := boxTest(t)
	if !b.DrawerOpen || !strings.Contains(screenOf(m), "⊡ Toys · 3") {
		t.Fatalf("a new box should be open, with its name and count:\n%s", screenOf(m))
	}
	pg := d.newKind(kPage, 0, 0)
	pg.Header, pg.X, pg.Y = "Recipe", 60, 4
	d.sel = -1
	m.View()
	px, py, _, _ := d.rect(pg)
	bx, by, _, _ := d.rect(b)
	tx, ty := bx+34, by+12
	dragTo(m, px+2, py+1+m.boardTop(), tx, ty+m.boardTop(), tea.MouseLeft)
	j := itemIndex(b, pg)
	if j < 0 || d.byID(pg.ID) != nil || pg.Slot != 0 {
		t.Fatalf("the page did not go into the box on the top layer: index %d, slot %d", j, pg.Slot)
	}
	_, _, w, h := d.rect(b)
	if act := d.boxGridOf(b, w, h).c[ty-by][tx-bx].act; act != "boxthing:"+itoa(j) {
		t.Fatalf("its miniature is not where it was let go: %q there", act)
	}
	if bt, it := d.chosenBoxThing(); bt != b || it != pg || d.sel != b.ID {
		t.Fatal("the thing put in should be chosen, in its chosen box")
	}
	if s := screenOf(m); !strings.Contains(s, "Recipe") || !strings.Contains(s, "⊡ Toys · 4") {
		t.Fatalf("the box should show Recipe and hold 4:\n%s", s)
	}

	// A drag on it takes it out: it follows the mouse onto the board.
	d.lastClick = time.Time{}
	cx, cy := boxCellOf(t, d, b, "boxthing:"+itoa(j))
	dragTo(m, cx, cy+m.boardTop(), 75, 20+m.boardTop(), tea.MouseLeft)
	if itemIndex(b, pg) >= 0 || d.byID(pg.ID) == nil {
		t.Fatal("a drag on the thing did not take it out")
	}
	if x, y, w, h := d.rect(pg); 75 < x || 75 >= x+w || 20 < y || 20 >= y+h {
		t.Fatalf("taken out, it should be under the mouse: %d,%d %d×%d", x, y, w, h)
	}
}

// At every zoom the screen shows exactly the box's grid, and a click on any
// cell of a thing chooses that thing: the top one under the mouse, or,
// looking at a lower layer, the one on it (the layers above are see-through).
func TestDesignBoxClicksEveryZoom(t *testing.T) {
	m, d, b := boxTest(t)
	// Two things in the same place: one on top, one at the bottom.
	b.Items = append(b.Items,
		&dOct{ID: 904, Kind: kNote, Header: "Over", Color: "#e5c07b", X: 20, Y: 5, Slot: 0},
		&dOct{ID: 905, Kind: kNote, Header: "Under", Color: "#e06c75", X: 20, Y: 5, Slot: 2})
	for _, zi := range []int{3, 4, 5, 6, 7, 8} {
		d.zoomTo(zi, 0, 0)
		d.camX, d.camY = 0, 0
		d.sel, d.boxPick = -1, boxSel{}
		d.setBoxView(b, 0)
		rows := strings.Split(screenOf(m), "\n")
		x, y, w, h := d.rect(b)
		g := d.boxGridOf(b, w, h)
		for cy := range h {
			sy := y + cy + m.boardTop()
			if sy >= m.boardTop()+m.boardH() || sy >= len(rows) {
				break
			}
			r := []rune(rows[sy])
			for cx := range w {
				if x+cx < len(r) && x+cx >= 0 && string(r[x+cx]) != g.c[cy][cx].r {
					t.Fatalf("zoom %d: cell %d,%d of the box shows %q, its grid says %q", zi, cx, cy, string(r[x+cx]), g.c[cy][cx].r)
				}
			}
		}
		for j := range b.Items {
			if j == 4 { // under "Over": it is found looking at its layer, below
				continue
			}
			cx, cy := boxCellOf(t, d, b, "boxthing:"+itoa(j))
			d.lastClick = time.Time{}
			click(m, cx, cy+m.boardTop(), tea.MouseLeft)
			if bt, it := d.chosenBoxThing(); bt != b || it != b.Items[j] {
				t.Fatalf("zoom %d: a click on thing %d (%s) chose %v", zi, j, b.Items[j].Header, it)
			}
		}
		// Where Under shows looking at the bottom layer, Over is on top
		// when all of them show.
		press(m, "esc")
		d.sel, d.boxPick = b.ID, boxSel{}
		press(m, "pgdown")
		press(m, "pgdown")
		press(m, "pgdown")
		if d.boxView(b) != 3 {
			t.Fatalf("zoom %d: PgDn three times should look at the bottom layer, not %d", zi, d.boxView(b))
		}
		cx, cy := boxCellOf(t, d, b, "boxthing:4")
		d.setBoxView(b, 0)
		_, _, w, h = d.rect(b)
		if act := d.boxGridOf(b, w, h).c[cy-y][cx-x].act; act != "boxthing:3" {
			t.Fatalf("zoom %d: with all the layers showing, Over should cover Under there, not %q", zi, act)
		}
		d.setBoxView(b, 3)
		d.lastClick = time.Time{}
		click(m, cx, cy+m.boardTop(), tea.MouseLeft)
		if _, it := d.chosenBoxThing(); it == nil || it.Header != "Under" {
			t.Fatalf("zoom %d: looking at the bottom layer, a click should choose Under, not %v", zi, it)
		}
	}
}

// PgUp and PgDn move the chosen thing between layers, Alt + arrows inside
// the box; each is an undo step.
func TestDesignBoxLayersAndUndo(t *testing.T) {
	m, d, b := boxTest(t)
	d.sel = b.ID
	press(m, "tab") // the first thing on the top layer: Idea
	_, it := d.chosenBoxThing()
	if it == nil || it.Header != "Idea" {
		t.Fatalf("Tab should choose the top layer's thing, not %v", it)
	}
	press(m, "pgdown")
	if it.Slot != 1 {
		t.Fatalf("PgDn should move it to the middle layer, not %d", it.Slot)
	}
	press(m, "pgdown")
	press(m, "pgdown")
	if it.Slot != 2 {
		t.Fatalf("PgDn should stop at the bottom layer, not %d", it.Slot)
	}
	press(m, "pgup")
	if it.Slot != 1 {
		t.Fatalf("PgUp should move it up a layer, not %d", it.Slot)
	}
	x0 := it.X
	press(m, "alt+right")
	if it.X != x0+1 {
		t.Fatalf("Alt+Right should move it a cell: %d, want %d", it.X, x0+1)
	}
	for range 200 {
		press(m, "alt+shift+right")
	}
	if mx, _ := boxRoom(b); it.X != mx {
		t.Fatalf("it should stop at the box's side: %d, want %d", it.X, mx)
	}
	press(m, "ctrl+z") // the moves, one by one
	for range 200 {
		if f := findItem(d, 903); f.X == x0 {
			break
		}
		press(m, "ctrl+z")
	}
	if f := findItem(d, 903); f.X != x0 || f.Slot != 1 {
		t.Fatalf("undoing the moves: %d (slot %d), want %d (slot 1)", f.X, f.Slot, x0)
	}
	press(m, "ctrl+z")
	if f := findItem(d, 903); f.Slot != 2 {
		t.Fatalf("Ctrl+Z should undo PgUp: slot %d, want 2", f.Slot)
	}
	press(m, "ctrl+z")
	if f := findItem(d, 903); f.Slot != 1 {
		t.Fatalf("Ctrl+Z should undo PgDn: slot %d, want 1", f.Slot)
	}
	press(m, "ctrl+z")
	if f := findItem(d, 903); f.Slot != 0 {
		t.Fatalf("Ctrl+Z should undo the first PgDn: slot %d, want 0", f.Slot)
	}
	// Tab and Shift+Tab go round, top layer first.
	b = d.byID(b.ID)
	d.sel, d.boxPick = b.ID, boxSel{}
	var seen []string
	for range 4 {
		press(m, "tab")
		_, it := d.chosenBoxThing()
		seen = append(seen, it.Header)
	}
	if got := strings.Join(seen, " "); got != "Idea Plan Ball Idea" {
		t.Fatalf("Tab went %s, want Idea Plan Ball Idea", got)
	}
	press(m, "shift+tab")
	if _, it := d.chosenBoxThing(); it.Header != "Ball" {
		t.Fatalf("Shift+Tab should go back to Ball, not %s", it.Header)
	}
	// Esc lets go of the thing; X X with one chosen takes it away.
	press(m, "esc")
	if _, it := d.chosenBoxThing(); it != nil || d.sel != b.ID {
		t.Fatal("Esc should let go of the thing and keep the box chosen")
	}
	press(m, "tab")
	pressXX(m)
	if len(b.Items) != 2 || d.byID(b.ID) == nil {
		t.Fatalf("X X should take the chosen thing away, not the box: %d left", len(b.Items))
	}
}

// Each design draws, at several sizes, with its own outline; D (and the
// [◇] on its top edge) goes to the next.
func TestDesignBoxDesigns(t *testing.T) {
	m, d, b := boxTest(t)
	b.Items = nil
	seen := map[string]int{}
	for des := range boxDesigns {
		b.Layout = des
		for _, wh := range [][2]int{{12, 5}, {20, 8}, {46, 17}, {90, 30}} {
			for _, open := range []bool{true, false} {
				b.DrawerOpen = open
				g := d.boxGridOf(b, wh[0], wh[1])
				if len(g.c) != wh[1] || len(g.c[0]) != wh[0] {
					t.Fatalf("design %d at %v: grid %d×%d", des, wh, len(g.c[0]), len(g.c))
				}
			}
		}
		b.DrawerOpen = true
		g := d.boxGridOf(b, 46, 17)
		var sb strings.Builder
		for _, row := range g.c {
			for _, c := range row {
				sb.WriteString(c.r)
			}
			sb.WriteByte('\n')
		}
		if prev, ok := seen[sb.String()]; ok {
			t.Fatalf("designs %d and %d draw the same outline", prev, des)
		}
		seen[sb.String()] = des
	}
	b.Layout = 0
	d.sel = b.ID
	for i := 1; i <= boxDesigns; i++ {
		press(m, "d")
		if boxDesign(b) != i%boxDesigns || !strings.Contains(m.msg, boxDesignNames[i%boxDesigns]) {
			t.Fatalf("D press %d: design %d (%q)", i, boxDesign(b), m.msg)
		}
	}
	press(m, "shift+d")
	if boxDesign(b) != boxDesigns-1 {
		t.Fatalf("Shift+D should go back to the last design, not %d", boxDesign(b))
	}
	cx, cy := boxCellOf(t, d, b, "boxdesign")
	d.lastClick = time.Time{}
	click(m, cx, cy+m.boardTop(), tea.MouseLeft)
	if boxDesign(b) != 0 {
		t.Fatalf("a click on [◇] should go to the next design, not %d", boxDesign(b))
	}
	// A change of design is an undo step.
	press(m, "ctrl+z")
	if boxDesign(d.byID(b.ID)) != boxDesigns-1 {
		t.Fatalf("Ctrl+Z should undo the design change, not %d", boxDesign(d.byID(b.ID)))
	}
}

// Space shuts the lid (its name and count show on it) and opens it; that
// is looking around, not an undo step. A click on [open] opens it too.
func TestDesignBoxLid(t *testing.T) {
	m, d, b := boxTest(t)
	space := func() {
		d.hover, d.spaceAt = 0, time.Time{}
		press(m, "space")
	}
	d.sel = b.ID
	undos := len(d.undo)
	space()
	if b.DrawerOpen {
		t.Fatal("Space should shut the box")
	}
	s := screenOf(m)
	if !strings.Contains(s, "3 things") || !strings.Contains(s, "[open]") || strings.Contains(s, "Idea") {
		t.Fatalf("shut, the lid should show its name and count and hide the things:\n%s", s)
	}
	if len(d.undo) != undos {
		t.Fatal("shutting the lid made an undo step")
	}
	d.sel = -1
	m.View()
	if s := screenOf(m); strings.Contains(s, "middle 0") {
		t.Fatal("shut, the layers' counts should not show")
	}
	cx, cy := boxCellOf(t, d, b, "boxlid")
	d.lastClick = time.Time{}
	click(m, cx, cy+m.boardTop(), tea.MouseLeft)
	if !b.DrawerOpen || !strings.Contains(screenOf(m), "Idea") {
		t.Fatal("a click on [open] should open the box")
	}
	// A click on a layer's name on its bottom edge looks at that layer; again, at all.
	if s := screenOf(m); !strings.Contains(s, "top 1") || !strings.Contains(s, "middle 1") || !strings.Contains(s, "bottom 1") {
		t.Fatalf("open, its bottom edge should count each layer:\n%s", s)
	}
	cx, cy = boxCellOf(t, d, b, "boxlayer:1")
	d.lastClick = time.Time{}
	click(m, cx, cy+m.boardTop(), tea.MouseLeft)
	if d.boxView(b) != 2 {
		t.Fatalf("a click on “middle” should look at the middle layer, not %d", d.boxView(b))
	}
	d.lastClick = time.Time{}
	click(m, cx, cy+m.boardTop(), tea.MouseLeft)
	if d.boxView(b) != 0 {
		t.Fatalf("a second click should show all the layers, not %d", d.boxView(b))
	}
	space()
	space()
	if !b.DrawerOpen {
		t.Fatal("Space twice should leave it open")
	}
}

// A box, its design, its lid and each thing's layer and place are saved
// and read back.
func TestDesignBoxSaveLoad(t *testing.T) {
	_, d, b := boxTest(t)
	b.Layout, b.DrawerOpen = boxHat, false
	b.Items[1].X, b.Items[1].Y, b.Items[1].Slot = 7, 2, 2
	d.dirty = true
	d.save()
	again := loadDesign(d.loadedFrom)
	var b2 *dOct
	for _, o := range again.octs {
		if o.Kind == kBox {
			b2 = o
		}
	}
	if b2 == nil || b2.Layout != boxHat || b2.DrawerOpen || b2.Header != "Toys" || len(b2.Items) != 3 {
		t.Fatalf("box read back wrong: %+v", b2)
	}
	for i, it := range b.Items {
		got := b2.Items[i]
		if fmt.Sprint(got.ID, got.X, got.Y, got.Slot, got.Kind) != fmt.Sprint(it.ID, it.X, it.Y, it.Slot, it.Kind) {
			t.Fatalf("thing %d read back as %+v, want %+v", i, got, it)
		}
	}
}

// A thing lying over half of another's name hides the rest of that name:
// the letters after it never run on from its own ("Keysn").
func TestBoxNoHalfWords(t *testing.T) {
	m := designTest(t)
	d := m.ds()
	d.camX, d.camY, d.zoom = 0, 0, 2 // 50%: things are one row, their names side by side
	b := d.newKind(kBox, 0, 0)
	b.X, b.Y, b.W, b.H, b.Header, b.DrawerOpen = 2, 1, 34, 14, "Card", true
	b.Items = []*dOct{
		{ID: 501, Kind: kNote, Header: "Keys", Color: "#e5c07b", X: 2, Y: 1},
		{ID: 502, Kind: kPage, Header: "Plan", Color: "#c678dd", X: 8, Y: 3, Slot: 1},
		{ID: 503, Kind: kPage, Header: "Glue", Color: "#61afef", X: 14, Y: 2, Slot: 2},
	}
	s := screenOf(m)
	if !strings.Contains(s, "Keys") {
		t.Fatalf("the top thing's name should show:\n%s", s)
	}
	for _, bad := range []string{"Keysn", "Keysa", "n ue", "Keysl"} {
		if strings.Contains(s, bad) {
			t.Fatalf("half a covered name runs on from Keys (%q):\n%s", bad, s)
		}
	}
}
