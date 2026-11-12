package main

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// lineLetter is whether s is one of the letters the text layer draws lines with.
func lineLetter(s string) bool {
	return s != "" && strings.Contains("─│┌┐└┘├┤┬┴┼╱╲╳⎺⎻⎼⎽▔▁▏▕┄┊━┃┏┓┗┛┣┫┳┻╋═║╔╗╚╝╠╣╦╩╬╭╮╰╯", s)
}

// inkCells calls see with each cell of the last drawing that shows a letter
// of the text layer.
func inkCells(r *room3D, see func(i int, k *r3ink)) {
	for i := range r.cells {
		if k := r.inkAt(i); k != nil {
			see(i, k)
		}
	}
}

// Turned, a block's outline is drawn in box line letters: cells on its
// edges, in its own colour's family, written as those letters.
func TestDesign3DTextEdges(t *testing.T) {
	m := room3DModel(t, 120, 40)
	designFrames = false
	r := &m.ds().room
	r.yaw, r.pitch = 25, 15
	rows := room3DText(m)
	edges := 0
	inkCells(r, func(i int, k *r3ink) {
		if k.level == inkLine && k.face >= 0 && r.faces[k.face].own >= 0 {
			if !lineLetter(k.s) {
				t.Fatalf("an edge cell holds %q, want a line letter", k.s)
			}
			edges++
		}
	})
	if edges < 20 {
		t.Fatalf("turned 25°: %d cells of the blocks' edges are line letters, want many", edges)
	}
	// They are in what is written, too.
	n := 0
	for _, row := range rows {
		for _, c := range row {
			if lineLetter(string(c)) {
				n++
			}
		}
	}
	if n < edges {
		t.Fatalf("%d line letters written, fewer than the %d edge cells", n, edges)
	}
}

// The walls are papered in letters: the side walls' cells show the paper's
// stripes (┊) or the rails, and the ceiling its dots.
func TestDesign3DTextWalls(t *testing.T) {
	m := room3DModel(t, 120, 40)
	designFrames = false
	r := &m.ds().room
	r.yaw, r.pitch = -40, -20 // the left wall and the ceiling in sight
	boardRows(m)
	var walls, stripes, dots int
	inkCells(r, func(i int, k *r3ink) {
		switch {
		case k.face == faceLeft || k.face == faceRight:
			walls++
			if k.s == "┊" {
				stripes++
			}
		case k.face == faceCeiling && k.s == "·":
			dots++
		}
	})
	if walls == 0 || stripes == 0 {
		t.Fatalf("the side wall: %d cells of letters, %d of its stripes; want both", walls, stripes)
	}
	if dots == 0 {
		t.Fatal("the ceiling: no dots")
	}
}

// Words always win over the text layer: a cell with a word's letter is
// written as that letter, never as a line, though lines run under words.
func TestDesign3DTextUnderWords(t *testing.T) {
	m, _ := pagesScene(t, 200, 60)
	designFrames = false
	r := &m.ds().room
	for _, a := range [][2]int{{5, 0}, {25, 15}, {-35, 30}} {
		r.yaw, r.pitch = a[0], a[1]
		rows := room3DText(m)
		under := 0
		for i := range r.chars {
			if r.chars[i].stamp != r.stamp {
				continue
			}
			if r.inkAt(i) != nil {
				t.Fatalf("turned %v: cell %d holds a word's letter and a line both", a, i)
			}
			if r.cells[i].stamp == r.stamp {
				under++ // a mark of the layer the word went over
			}
		}
		if under == 0 {
			t.Fatalf("turned %v: no word went over a line: the test shows nothing", a)
		}
		if !titleShows(rows, "Charlie") || !strings.Contains(strings.Join(rows, "\n"), "First line") {
			t.Fatalf("turned %v: the words should still read:\n%s", a, strings.Join(rows, "\n"))
		}
		checkNoWordThrough(t, r, "with the text layer")
	}
}

// Near straight on, a front is its flat drawing: the page's frame shows as
// the same box line letters as on the flat board.
func TestDesign3DTextFrontAsFlat(t *testing.T) {
	m, _ := pagesScene(t, 200, 60)
	designFrames = false
	flat := strings.Join(room3DTextFlat(m), "\n")
	r := &m.ds().room
	r.yaw = 5
	turned := strings.Join(room3DText(m), "\n")
	for _, g := range []string{"──────────", "│"} {
		if !strings.Contains(flat, g) {
			t.Fatalf("the flat board should have %q in it", g)
		}
		if !strings.Contains(turned, g) {
			t.Fatalf("turned 5°, the fronts' frames should show %q as on the flat board:\n%s", g, turned)
		}
	}
}

// room3DTextFlat is the flat board area as plain text.
func room3DTextFlat(m *Model) []string {
	rows := boardRows(m)
	for i := range rows {
		rows[i] = ansi.Strip(rows[i])
	}
	return rows
}

// P switches the 3D view between the room in letters and the plain
// picture (in the status bar's hint too); on the flat board it is not the
// 3D view's, and after Alt+N it places a page as ever.
func TestDesign3DTextToggle(t *testing.T) {
	m := room3DModel(t, 120, 40)
	designFrames = false
	d := m.ds()
	r := &d.room
	press(m, "p")
	if r.plain {
		t.Fatal("P on the flat board: not the 3D view's key")
	}
	press(m, "ctrl+right")
	press(m, "ctrl+right")
	text := strings.Join(boardRows(m), "\n")
	if !strings.Contains(m.designStatus(), "P: picture") {
		t.Fatalf("the hint should offer P: %q", m.designStatus())
	}
	press(m, "p")
	if !r.plain {
		t.Fatal("P in 3D: want the plain picture")
	}
	pic := strings.Join(boardRows(m), "\n")
	if pic == text {
		t.Fatal("the plain picture should differ from the room in letters")
	}
	inkCells(r, func(i int, k *r3ink) { t.Fatalf("the plain picture: cell %d holds the layer's %q", i, k.s) })
	if !strings.Contains(m.designStatus(), "P: text") {
		t.Fatalf("the hint should offer P: text: %q", m.designStatus())
	}
	press(m, "p")
	if r.plain || strings.Join(boardRows(m), "\n") != text {
		t.Fatal("P again: the room in letters, as before")
	}
	pages := len(d.octs)
	press(m, "alt+n")
	press(m, "p")
	if r.plain || len(d.octs) != pages+1 {
		t.Fatalf("Alt+N, P in 3D: want a page placed (structures %d → %d) and the look kept", pages, len(d.octs))
	}
}

// The 3D view's hints fit a 100 column status bar.
func TestDesign3DHintsFit(t *testing.T) {
	for _, h := range []string{roomHint, roomHintPic} {
		if !strings.HasPrefix(h, "3D view") || utf8.RuneCountInString(h) > 100 {
			t.Fatalf("hint %q: want it to start with \"3D view\" and fit 100 columns (%d)", h, utf8.RuneCountInString(h))
		}
	}
}

// In kitty the layer's letters are written in their cells in place of the
// picture's placeholders, each one cell, so the rows keep their width.
func TestKittyTextLayer(t *testing.T) {
	m := wordsScene(t, 120, 40)
	withPixels(t, m, 10, 20)
	r := &m.ds().room
	r.yaw, r.pitch = 25, 15
	rows := boardRows(m)
	if !r.pix {
		t.Fatal("want a picture")
	}
	n := 0
	inkCells(r, func(i int, k *r3ink) { n++ })
	if n == 0 {
		t.Fatal("no letters of the layer")
	}
	letters := 0
	for _, row := range rows {
		if w := ansi.StringWidth(row); w != m.width {
			t.Fatalf("a row is %d cells wide, want %d", w, m.width)
		}
		for _, c := range ansi.Strip(row) {
			if lineLetter(string(c)) {
				letters++
			}
		}
	}
	if letters == 0 {
		t.Fatal("the layer's line letters are not in what is written")
	}
	// Mouse picking still sees the faces under the letters.
	if hit := r.pickAt(m.width/2, m.boardH()/2); !hit.ok {
		t.Fatal("picking in the middle of the room found nothing")
	}
}
