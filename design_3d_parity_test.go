package main

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// middleScene is a 100×33 Design mode with page A in the middle of the
// board area and page B far to the right, nothing else.
func middleScene(t *testing.T) (*Model, *dOct, *dOct) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	m, _ := NewModel("", 0)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 33})
	m.setMode(designMode)
	d := m.ds()
	d.camX, d.camY, d.zoom = 0, 0, designZoom100
	a := d.newKind(kPage, 0, 0)
	a.X, a.Y, a.W, a.H, a.Header = 38, m.boardH()/2-5, 24, 10, "Alpha"
	b := d.newKind(kPage, 0, 0)
	b.X, b.Y, b.W, b.H, b.Header = 74, m.boardH()/2-5, 22, 10, "Bravo"
	d.sel = -1
	return m, a, b
}

// In 3D the chosen structure is what the middle of the camera is on: on
// going into 3D, when the camera moves over another, and none on the
// empty wall. A click still chooses, until the camera moves again.
func TestDesign3DMiddleChooses(t *testing.T) {
	m, a, b := middleScene(t)
	d := m.ds()
	r := &d.room
	press(m, "ctrl+f")
	boardRows(m)
	if d.sel != a.ID {
		t.Fatalf("into 3D: chosen %d, want the middle one %d", d.sel, a.ID)
	}
	r.pos = r.startPos.add(r3vec{-40, 0, 0}) // the camera moved left, over the empty wall
	m.design3DChooseMiddle()
	if d.sel != -1 {
		t.Fatalf("over the empty wall: chosen %d, want none", d.sel)
	}
	r.pos = r.startPos.add(r3vec{float64(b.X+b.W/2) - float64(m.width)/2, 0, 0}) // over B
	m.design3DChooseMiddle()
	if d.sel != b.ID {
		t.Fatalf("over B: chosen %d, want %d", d.sel, b.ID)
	}
	d.sel = a.ID // as a click would
	m.Update(tea.KeyPressMsg(tea.Key{Code: 'x', Text: "x"}))
	if d.sel != a.ID {
		t.Fatal("the camera did not move: a click's choice should stay")
	}
	press(m, "esc")
	if r.on() {
		t.Fatal("Esc: not flat")
	}
}

// Esc in 3D first leaves a mode, as on the flat board; then it goes flat.
func TestDesign3DEscLeavesModeFirst(t *testing.T) {
	m, _, _ := middleScene(t)
	d := m.ds()
	press(m, "ctrl+f")
	boardRows(m)
	d.linkMode = true
	press(m, "esc")
	if d.linkMode || !d.room.on() {
		t.Fatalf("Esc in link mode: link %v, 3D %v; want link mode left, still 3D", d.linkMode, d.room.on())
	}
	press(m, "esc")
	if d.room.on() {
		t.Fatal("second Esc: not flat")
	}
}

// Alt+N then a letter places in 3D as on the flat board (S is a shape,
// not walking), where the middle of the camera looks, and the flat board
// under the room does not scroll, for Alt+N, E and Tab alike.
func TestDesign3DPlaceAndWriteKeepWall(t *testing.T) {
	m, a, _ := middleScene(t)
	d := m.ds()
	r := &d.room
	press(m, "ctrl+f")
	boardRows(m)
	r.pos = r.startPos.add(r3vec{-30, 0, 0})
	boardRows(m)
	camX, camY := d.camX, d.camY
	n := len(d.octs)
	pos := r.pos
	press(m, "alt+n")
	press(m, "s")
	if r.pos != pos {
		t.Fatal("Alt+N, S: the camera moved")
	}
	if len(d.octs) != n+1 {
		t.Fatalf("Alt+N, S: %d structures, want %d", len(d.octs), n+1)
	}
	o := d.octs[len(d.octs)-1]
	if o.Kind != kOct || o.X+o.W/2 > 40 {
		t.Fatalf("placed %s at x %d (w %d): want a shape left of the middle, where the camera looks", o.Kind, o.X, o.W)
	}
	if d.camX != camX || d.camY != camY {
		t.Fatalf("Alt+N scrolled the wall: cam %v,%v → %v,%v", camX, camY, d.camX, d.camY)
	}
	press(m, "esc")
	d.sel = a.ID
	press(m, "e")
	press(m, "tab")
	if d.camX != camX || d.camY != camY {
		t.Fatalf("E, Tab scrolled the wall: cam %v,%v → %v,%v", camX, camY, d.camX, d.camY)
	}
}

// The toolbar's zoom in 3D does nothing: the camera is first person, and
// the flat board under the room keeps its zoom.
func TestDesign3DToolbarZoomNothing(t *testing.T) {
	m, _, _ := middleScene(t)
	d := m.ds()
	press(m, "ctrl+f")
	boardRows(m)
	z, pos := d.zoom, d.room.pos
	m.designZoomTo(d.zoom + 1)
	if d.zoom != z || d.room.pos != pos {
		t.Fatalf("zoom in: flat zoom %d (was %d), camera at %v (was %v); want both unchanged", d.zoom, z, d.room.pos, pos)
	}
}
