package main

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// wordsScene is a w×h Design mode with an open box of things, a drawer
// unit, pages with lines of words, a cabinet, a shape and a note.
func wordsScene(t testing.TB, w, h int) *Model {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	m, _ := NewModel("", 0)
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	m.setMode(designMode)
	d := m.ds()
	d.camX, d.camY, d.zoom = 0, 0, designZoom100
	add := func(kind string, x, y, ow, oh int, title, body string) *dOct {
		o := d.newKind(kind, 0, 0)
		o.X, o.Y, o.Header, o.Body = x, y, title, body
		if ow > 0 {
			o.W, o.H = ow, oh
		}
		return o
	}
	box := add(kBox, 0, 1, 34, 14, "Card box", "")
	box.DrawerOpen = true
	box.Items = []*dOct{
		{ID: 801, Kind: kNote, Header: "Keys", Color: "#e5c07b", X: 2, Y: 1, W: 26, H: 9},
		{ID: 802, Kind: kPage, Header: "Plan", Color: "#c678dd", X: 8, Y: 3, W: 26, H: 9, Slot: 1},
	}
	dr := add(kDrawer, 38, 1, 31, 9, "Tools", "")
	dr.Drawers = 3
	dr.Items = []*dOct{{ID: 803, Kind: kNote, Header: "Saw", Color: "#98c379"}}
	add(kPage, 72, 1, 30, 13, "Notes", "A page of words to read in the room.\nThe second line of the page.\nA third line, and then\na fourth one.")
	add(kPage, 106, 1, 30, 13, "Shopping list", "Bread and milk\nEggs, a dozen\nCoffee beans\nApples\nSoap")
	add(kCabinet, 38, 16, 30, 10, "Shelf", "")
	add(kOct, 2, 18, 30, 9, "Idea", "Something long enough to wrap.")
	add(kNote, 72, 17, 30, 10, "Week plan", "Monday: write\nTuesday: read\nWednesday: build")
	d.sel = -1
	return m
}

// pagesScene is a w×h Design mode with four pages in a square, nothing
// in front of another, each with a title and lines of words.
func pagesScene(t testing.TB, w, h int) (*Model, []*dOct) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	m, _ := NewModel("", 0)
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	m.setMode(designMode)
	d := m.ds()
	d.camX, d.camY, d.zoom = 0, 0, designZoom100
	var out []*dOct
	for i, title := range []string{"Alpha", "Bravo", "Charlie", "Delta"} {
		o := d.newKind(kPage, 0, 0)
		o.X, o.Y, o.W, o.H = 2+(i%2)*30, 1+(i/2)*12, 24, 10
		o.Header = title
		o.Body = "First line of " + title + "\nSecond line\nThird line"
		out = append(out, o)
	}
	d.sel = -1
	return m, out
}

// room3DText is the 3D view's board area as plain text, a line a row.
func room3DText(m *Model) []string {
	rows := boardRows(m)
	for i := range rows {
		rows[i] = ansi.Strip(rows[i])
	}
	return rows
}

// titleShows is whether title, or its first three letters at least (cut
// short, with "…" or not), is on one of the rows as letters.
func titleShows(rows []string, title string) bool {
	all := strings.Join(rows, "\n")
	r := []rune(title)
	return strings.Contains(all, string(r[:min(3, len(r))]))
}

// frontSeen is how many pixels of the last drawing show structure o's front.
func frontSeen(r *room3D, o *dOct) int {
	n := 0
	for _, f := range r.face {
		if f >= 0 && r.faces[f].own == o.ID && r.faces[f].mat == matFront {
			n++
		}
	}
	return n
}

// checkNoWordThrough fails if a letter shows where its face is behind
// another structure: of a letter's four pixels, no more than two (the
// edge of a nearer block through the cell) may show a different
// structure nearer than the letter's face.
func checkNoWordThrough(t *testing.T, r *room3D, what string) {
	t.Helper()
	w := r.rm.w
	for at, ch := range r.chars {
		if ch.stamp != r.stamp {
			continue
		}
		fc := &r.faces[ch.face]
		num := fc.k - fc.n.dot(r.view.cam)
		tl := at/w*2*r.pw + 2*(at%w)
		bad := 0
		for _, i := range [4]int{tl, tl + 1, tl + r.pw, tl + r.pw + 1} {
			f := r.face[i]
			if f < 0 || f == ch.face || r.faces[f].own < 0 || r.faces[f].own == fc.own {
				continue
			}
			d := r.dirs[i]
			if nd := fc.n.dot(d); nd < 0 && float64(r.zb[i]) < num/nd-0.05 {
				bad++
			}
		}
		if bad > 2 {
			t.Fatalf("%s: the letter %q at row %d, column %d is drawn over a nearer structure", what, ch.s, at/w, at%w)
		}
	}
}

// Words are lettering on their faces: a title shows where its face is
// seen squarely and big enough to read; on a face seen edge-on, or too
// small (zoomed far out), it does not show at all; and no letter is ever
// drawn over a nearer block.
func TestDesign3DTitlesEverywhere(t *testing.T) {
	m, pages := pagesScene(t, 200, 60)
	d := m.ds()
	r := &d.room
	shown, hidden := 0, 0
	for _, zoom := range []int{0, 2, 4, designZoom100, 8} { // 25%, 50%, 80%, 100%, 200%
		d.zoom = zoom
		for _, a := range [][2]int{{5, 0}, {20, 10}, {35, 30}, {-35, -30}, {80, 0}, {-80, 0}, {0, 70}, {0, -45}, {60, 45}, {80, 75}} {
			for _, near := range []int{roomNearMin, 0, 3} {
				r.yaw, r.pitch, r.near = a[0], a[1], near
				what := fmt.Sprintf("zoom %.0f%%, turned %d°, tilted %d°, camera %d", 100*d.z(), a[0], a[1], near)
				checkRows(t, m, what)
				rows := room3DText(m)
				for _, o := range pages {
					if frontSeen(r, o) < 12 {
						continue // its front hardly shows (off the screen)
					}
					square := abs(a[0]) <= 20 && abs(a[1]) <= 10
					edgeOn := abs(a[0]) >= 80 || abs(a[1]) >= 70
					switch {
					case zoom >= designZoom100 && square && near == 0:
						shown++
						if !titleShows(rows, o.Header) {
							t.Fatalf("%s: the title %q, seen squarely and big enough, should show:\n%s", what, o.Header, strings.Join(rows, "\n"))
						}
					case edgeOn && near == 0:
						hidden++
						if titleShows(rows, o.Header) {
							t.Fatalf("%s: the title %q is seen edge-on, and should not show:\n%s", what, o.Header, strings.Join(rows, "\n"))
						}
					}
				}
				checkNoWordThrough(t, r, what)
			}
		}
	}
	if shown < 4 || hidden < 6 {
		t.Fatalf("only %d titles checked shown and %d hidden: the test checks too little", shown, hidden)
	}
}

// In free roam too: close to a page, far away, and looking round.
func TestDesign3DTitlesFreeRoam(t *testing.T) {
	m, pages := pagesScene(t, 160, 50)
	d := m.ds()
	r := &d.room
	r.yaw, r.pitch = 35, 30
	press(m, "ctrl+f")
	start := r.pos
	o := pages[0]
	x, y, ow, _ := d.rect(o)
	rm := r.rm
	page := r3vec{float64(x+ow/2) - rm.hw, rm.hh - float64(2*y+4), 1} // near the top of Alpha's front
	for _, c := range []struct {
		what string
		pos  r3vec
		look [2]float64
		show bool // its titles are big and square enough to read
	}{
		{"far away", start.add(r3vec{0, 0, 3 * rm.big}), [2]float64{0, 0}, false},
		{"far away, turned", start.add(r3vec{-rm.big, rm.big / 2, 2 * rm.big}), [2]float64{20, 15}, false},
		{"close to a page", page.add(r3vec{0, 0, 30}), [2]float64{0, 0}, true},
		{"very close to a page", page.add(r3vec{0, -2, 12}), [2]float64{0, 0}, true},
	} {
		r.pos, r.look = c.pos, c.look
		checkRows(t, m, "free roam "+c.what)
		rows := room3DText(m)
		seen := 0
		for _, p := range pages {
			if frontSeen(r, p) < 12 {
				continue
			}
			seen++
			if c.show && p == o && !titleShows(rows, p.Header) {
				t.Fatalf("free roam %s: the title %q should show as letters:\n%s", c.what, p.Header, strings.Join(rows, "\n"))
			}
			if !c.show && titleShows(rows, p.Header) {
				t.Fatalf("free roam %s: the title %q is too far to read, and should not show:\n%s", c.what, p.Header, strings.Join(rows, "\n"))
			}
		}
		if seen == 0 {
			t.Fatalf("free roam %s: no page in sight: the test checks nothing", c.what)
		}
		checkNoWordThrough(t, r, "free roam "+c.what)
	}
}

// A title behind a nearer block is not drawn through it: the cabinet
// stands over the top of the page, and the page's title shows only
// where the page itself is seen (it moves down to there), never on the
// cabinet.
func TestDesign3DTitleBehindBlock(t *testing.T) {
	m, pages := pagesScene(t, 160, 50)
	d := m.ds()
	r := &d.room
	pg := pages[0]
	pg.Header = "Hiddenword"
	cab := d.newKind(kCabinet, 0, 0)
	cab.X, cab.Y, cab.W, cab.H, cab.Header = pg.X-2, pg.Y-3, pg.W+4, 6, "Cupboard"
	for _, a := range [][2]int{{5, 0}, {10, 5}, {20, 15}, {-20, 25}, {35, 30}} {
		r.yaw, r.pitch = a[0], a[1]
		what := fmt.Sprintf("turned %d°, tilted %d°", a[0], a[1])
		rows := room3DText(m)
		checkNoWordThrough(t, r, what)
		for row, line := range rows {
			col := strings.Index(line, "Hid")
			if col < 0 {
				continue
			}
			col = ansi.StringWidth(line[:col])
			for c := col; c < col+3; c++ {
				h := r.pickAt(c, row)
				if h.own == cab.ID {
					t.Fatalf("%s: the page's title is drawn on the cabinet in front of it, at row %d:\n%s", what, row, strings.Join(rows, "\n"))
				}
			}
		}
	}
}

// Where there is room, a page's lines of words show whole, not only its
// title; squeezed, the first lines show and the others give way.
func TestDesign3DBodyLines(t *testing.T) {
	m := wordsScene(t, 200, 60)
	r := &m.ds().room
	for _, a := range [][2]int{{10, 5}, {20, 10}, {-15, 10}} {
		r.yaw, r.pitch = a[0], a[1]
		all := strings.Join(room3DText(m), "\n")
		for _, want := range []string{"Bread and milk", "Eggs, a dozen", "Coffee beans", "Monday: write", "Tuesday: read", "The second line of the"} {
			if !strings.Contains(all, want) {
				t.Fatalf("turned %v: the line %q should show whole:\n%s", a, want, all)
			}
		}
	}
	// Tilted far down, the pages are seen nearly edge-on: their words are
	// too squashed to read, so they do not show.
	r.yaw, r.pitch = 0, 75
	all := strings.Join(room3DText(m), "\n")
	for _, want := range []string{"Shopping", "Week plan", "Monday"} {
		if strings.Contains(all, want) {
			t.Fatalf("tilted 75°: %q is seen edge-on and should not show:\n%s", want, all)
		}
	}
}

// What is written shows in 3D as it is typed, with its caret; letters two
// cells wide take two cells, and no line is wider than the board area.
func TestDesign3DWordsLive(t *testing.T) {
	m, pages := pagesScene(t, 160, 50)
	d := m.ds()
	r := &d.room
	r.yaw, r.pitch = 20, 10
	pg := pages[1]
	d.sel, d.editing = pg.ID, partHeader
	typeKeys(m, " news")
	rows := room3DText(m)
	all := strings.Join(rows, "\n")
	if !strings.Contains(all, "Bravo news▏") {
		t.Fatalf("typing in 3D: the title with its caret should show as letters:\n%s", all)
	}
	press(m, "esc")
	pages[2].Header = "日本語 title"
	for _, a := range [][2]int{{20, 10}, {35, 30}, {80, 0}, {0, 70}} {
		r.yaw, r.pitch = a[0], a[1]
		checkRows(t, m, fmt.Sprintf("wide letters, turned %v", a))
	}
	r.yaw, r.pitch = 20, 10
	if all := strings.Join(room3DText(m), "\n"); !strings.Contains(all, "日本語") {
		t.Fatalf("a title of letters two cells wide should show:\n%s", all)
	}
}

// The faces show the words' cells as their background, not as a smear of
// the letters' colour; box lines stay lines.
func TestDesign3DFacesClean(t *testing.T) {
	m, _ := pagesScene(t, 160, 50)
	r := &m.ds().room
	r.yaw, r.pitch = 35, 30
	boardRows(m)
	for i, t3 := range r.tex {
		if t3.glyph == "" {
			continue
		}
		for _, q := range t3.q {
			if q != t3.bg {
				t.Fatalf("wall cell %d (%q): its quarters should be its background %06x, not %06x", i, t3.glyph, t3.bg, q)
			}
		}
	}
	lines := 0
	for _, t3 := range r.tex {
		if t3.arms != 0 && t3.q[0] != t3.bg || t3.arms != 0 && t3.q[3] != t3.bg {
			lines++
		}
	}
	if lines == 0 {
		t.Fatal("the pages' box lines should tint the faces")
	}
	if c := lineCover(4|8, 0.5, 0.5, 0.2); c < 0.9 {
		t.Fatalf("a line across, at its middle: covered %.2f", c)
	}
	if c := lineCover(4|8, 0.5, 0.1, 0.2); c > 0.01 {
		t.Fatalf("a line across, far above its middle: covered %.2f", c)
	}
	if c := lineCover(2|8, 0.1, 0.5, 0.2); c > 0.01 {
		t.Fatalf("a corner ┌, left of it: covered %.2f", c)
	}
}
