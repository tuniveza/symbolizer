package main

import (
	"math"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// Each wedge letter covers its share of the cell (1/8 to 7/8), on the
// side its angle faces, and its points say the same.
func TestGeoWedgeShapes(t *testing.T) {
	ms := geoWedgeMasks()
	for k := range geoAngles {
		for j := 1; j <= geoDepths; j++ {
			s := wedgeShape(k, j)
			if a := polyArea(s); math.Abs(a-2*float64(j)/8) > 1e-6 {
				t.Fatalf("wedge %d,%d covers %.4f of 2", k, j, a)
			}
			n := ms[k*geoDepths+j-1].count()
			if want := j * geoSX * geoSY / 8; n < want-10 || n > want+10 {
				t.Fatalf("wedge %d,%d has %d points, want about %d", k, j, n, want)
			}
		}
	}
	// Angle 0 faces right: depth 4 is the right half.
	m := ms[0*geoDepths+3]
	if !m.has(7) || m.has(0) {
		t.Fatal("wedge 0,4 should be the right half of the cell")
	}
}

// A wedge's own points fit back to it (or to the letter for its other
// side), and a cell of no edge (a checkerboard) fits none.
func TestGeoFit(t *testing.T) {
	ms := geoWedgeMasks()
	for i := range ms {
		got := geoFit(ms[i])
		if got < 0 || min(ms[got].apart(ms[i]), ms[got].apart(ms[i].not())) > 2 {
			t.Fatalf("wedge %d fits %d", i, got)
		}
	}
	var chk geoMask
	for i := range geoSX * geoSY {
		if (i/geoSX+i%geoSX)%2 == 0 {
			chk.set(i)
		}
	}
	if geoFit(chk) >= 0 {
		t.Fatal("a checkerboard is not split by one edge")
	}
}

// A line through a cell gets the line letter at its angle and place: a
// level line through the middle is the middle place of angle 0, one a
// quarter down is higher, and a line that misses the cell has none. A
// slope of 1 in 2 rows runs at 45° in a cell twice as tall as wide.
func TestGeoLineFor(t *testing.T) {
	if g := geoLineFor(0, 5.5, 1, 0, 3, 5); g != geoLine(0, 4) {
		t.Fatalf("a level line through the middle: %U", []rune(g))
	}
	if g := geoLineFor(0, 5.1, 1, 0, 3, 5); g != geoLine(0, 0) {
		t.Fatalf("a level line near the top: %U, want %U", []rune(g), []rune(geoLine(0, 0)))
	}
	if g := geoLineFor(0, 9, 1, 0, 3, 5); g != "" {
		t.Fatalf("a line rows away: %U", []rune(g))
	}
	if g := geoLineFor(3.5, 5.5, 1, 0.5, 3, 5); g != geoLine(geoLineAng/4, 4) {
		t.Fatalf("a line at 45°: %U, want %U", []rune(g), []rune(geoLine(geoLineAng/4, 4)))
	}
	if !isGeo(geoLine(0, 0)) || !isGeo(geoWedge(geoAngles-1, geoDepths)) || isGeo("╱") {
		t.Fatal("isGeo")
	}
}

// With the font set up, the turned room is drawn in shape letters: its
// sloping edges are wedge letters, its sloping lines line letters, with no
// ╱ ╲ ╳ for them, and the words still read. P goes to the text over the picture,
// the plain picture, and back to shape letters.
func TestDesign3DShapes(t *testing.T) {
	was := shapesWanted
	shapesWanted = func() bool { return true }
	t.Cleanup(func() { shapesWanted = was })
	m := room3DModel(t, 120, 40)
	designFrames = false
	r := &m.ds().room
	r.yaw, r.pitch = -45, 10
	rows := strings.Join(boardRows(m), "\n")
	plain := ansi.Strip(rows)
	wedges, lines := 0, 0
	for _, c := range plain {
		switch {
		case c >= geoBase && c < geoBase+geoLines:
			wedges++
		case c >= geoBase+geoLines && c < geoBase+geoCount:
			lines++
		}
	}
	for i := range r.cells { // (a flat drawing's own ╱ ╲, an octagon's corners, stays)
		if k := r.inkAt(i); k != nil && k.glyph == "" && (k.s == "╱" || k.s == "╲" || k.s == "╳") {
			t.Fatalf("in shape letters a sloping line is no %s (cell %d, %d)", k.s, i%120, i/120)
		}
	}
	if wedges < 50 || lines < 10 {
		t.Fatalf("turned 45°, the room should have edges in wedge letters (%d) and lines in line letters (%d)", wedges, lines)
	}
	if !strings.Contains(plain, "Thing") {
		t.Fatal("the titles should still read in shape letters:\n" + plain)
	}
	if !r.shapes() || r.pw != 120*geoSX && r.pw != 120*geoSX/2 {
		t.Fatalf("shape letters are fitted to 8×16 points a cell: picture %d wide", r.pw)
	}
	press(m, "p")
	if !r.shapes() {
		t.Fatal("P in 3D: no key of its own now, shape letters stay")
	}
}
