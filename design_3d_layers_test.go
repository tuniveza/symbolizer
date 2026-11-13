package main

import (
	"math"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// boardAtCross is the board point (board cells) the cross is on, on the
// back wall.
func boardAtCross(t *testing.T, m *Model) (float64, float64) {
	t.Helper()
	boardRows(m)
	d := m.ds()
	r := &d.room
	v, ok := r.nowView()
	if !ok {
		t.Fatal("no drawing")
	}
	dir := v.ray(v.px, v.py)
	p := v.cam.add(dir.mul(-v.cam.z / dir.z))
	return p.x + r.rm.hw + d.camX, (r.rm.hh-p.y)/2 + d.camY
}

// 3D shows the board at 100% whatever the flat board's zoom, from as far
// back as that zoom showed it, and the flat board's zoom comes back after.
func TestDesign3DAt100(t *testing.T) {
	m := room3DModel(t, 120, 40)
	d := m.ds()
	d.zoom, d.camX, d.camY = 1, -40, -20 // 33%
	bx, by := d.world(60, m.boardH()/2)
	press(m, "ctrl+f")
	if d.zoom != designZoom100 {
		t.Fatalf("in 3D the board is at 100%%: zoom %d", d.zoom)
	}
	cx, cy := boardAtCross(t, m)
	if math.Abs(cx-bx) > 2 || math.Abs(cy-by) > 2 {
		t.Fatalf("the cross stays on the board point in the middle: %.1f,%.1f, want %.1f,%.1f", cx, cy, bx, by)
	}
	if r := &d.room; r.pos.z < 2.5*r.rm.big/2 {
		t.Fatalf("the camera stands back as far as 33%% showed: %.1f", r.pos.z)
	}
	press(m, "esc")
	if d.zoom != 1 || d.camX != -40 || d.camY != -20 {
		t.Fatalf("the flat board's zoom and place come back: %d %v %v", d.zoom, d.camX, d.camY)
	}
}

// A window of another size (or the font zoomed) keeps the camera on the
// same spot of the board.
func TestDesign3DKeepsCameraOnResize(t *testing.T) {
	m := room3DModel(t, 120, 40)
	press(m, "ctrl+f")
	m.ds().room.lookBy(12, 6)
	press(m, "w")
	bx, by := boardAtCross(t, m)
	for _, s := range [][2]int{{80, 27}, {160, 54}, {120, 40}} {
		m.Update(tea.WindowSizeMsg{Width: s[0], Height: s[1]})
		x, y := boardAtCross(t, m)
		if math.Abs(x-bx) > 0.5 || math.Abs(y-by) > 0.5 {
			t.Fatalf("%v: the cross moved on the board: %.2f,%.2f, was %.2f,%.2f", s, x, y, bx, by)
		}
	}
}

// = and − zoom the lens in 3D (the camera stays), 0 sets it back; the
// flat board's zoom is left alone.
func TestDesign3DLens(t *testing.T) {
	m := room3DModel(t, 120, 40)
	d := m.ds()
	press(m, "ctrl+f")
	r := &d.room
	pos, look, zoom := r.pos, r.look, d.zoom
	press(m, "=")
	press(m, "=")
	if math.Abs(r.lens-lensStep*lensStep) > 1e-9 || r.pos != pos || r.look != look || d.zoom != zoom {
		t.Fatalf("= twice: lens %v, camera %v %v, zoom %d", r.lens, r.pos, r.look, d.zoom)
	}
	boardRows(m)
	if v, _ := r.nowView(); math.Abs(v.f-r.ff*r.lens) > 1e-9 {
		t.Fatal("the lens narrows the view")
	}
	press(m, "-")
	press(m, "0")
	if r.lens != 1 {
		t.Fatalf("0: the lens back to normal: %v", r.lens)
	}
}

// Shift + = brings the chosen structure a layer out, Shift + − takes it
// back; its block stands out that far, with a back.
func TestDesign3DLayers(t *testing.T) {
	m := room3DModel(t, 120, 40)
	d := m.ds()
	r := &d.room
	r.yaw, r.pitch = 20, 15
	o := d.octs[1]
	cell3D(t, m, o.ID, func(sx, sy int) bool { return true })
	r.freeK = 1 // all the way out
	d.sel = o.ID
	front := func() float64 {
		boardRows(m)
		for _, f := range r.faces {
			if f.own == o.ID && f.mat == matFront {
				return r.pts[f.p0].z
			}
		}
		t.Fatal("no front")
		return 0
	}
	z0 := front()
	key := func(code rune, text string, mod tea.KeyMod) {
		m.Update(tea.KeyPressMsg(tea.Key{Code: code, Text: text, Mod: mod}))
	}
	key('+', "+", 0) // a terminal that sends the letter
	if o.Layer != 1 {
		t.Fatalf("+: layer %d", o.Layer)
	}
	if dz := front() - z0; math.Abs(dz-layerStep) > 1e-3 {
		t.Fatalf("a layer out: the front came %.2f nearer, want %.0f", dz, layerStep)
	}
	backs := 0
	for _, f := range r.faces {
		if f.own == o.ID && f.n.z < -0.99 {
			backs++
		}
	}
	if backs == 0 {
		t.Fatal("off the wall it has a back")
	}
	key('=', "+", tea.ModShift) // kitty: the key, with Shift
	if o.Layer != 2 {
		t.Fatalf("Shift + =: layer %d", o.Layer)
	}
	key('_', "_", 0)
	key('-', "_", tea.ModShift)
	key('-', "_", tea.ModShift)
	if o.Layer != 0 {
		t.Fatalf("back to the wall, and no further: layer %d", o.Layer)
	}
}

// Alt + N places the new structure centred on the cross: on the empty
// wall on it, on a structure a layer out in front of it, and never moved
// aside: where its spot is taken, on the first free layer in front.
func TestDesign3DPlaceAtCross(t *testing.T) {
	m := room3DModel(t, 120, 40)
	d := m.ds()
	r := &d.room
	r.yaw, r.pitch = 10, 5
	room3DCell(t, m, matBoard)
	r.lookBy(3, 2)
	boardRows(m)
	bx, by := boardAtCross(t, m)
	press(m, "alt+n")
	press(m, "p")
	o := d.byID(d.sel)
	if o == nil || o.Kind != kPage || d.coversOnLayer(o) {
		t.Fatalf("a page on the wall, or on the first free layer in front: %+v", o)
	}
	w, h := d.size(o)
	if math.Abs(float64(o.X)+float64(w)/2-bx) > 1.5 || math.Abs(float64(o.Y)+float64(h)/2-by) > 1.5 {
		t.Fatalf("centred on the cross (%.1f,%.1f): at %d,%d size %dx%d", bx, by, o.X, o.Y, w, h)
	}
	press(m, "esc") // stop writing
	target := d.octs[1]
	cell3D(t, m, target.ID, func(sx, sy int) bool { return true })
	press(m, "alt+n")
	press(m, "n")
	n := d.byID(d.sel)
	if n == nil || n.Kind != kNote || n.Layer < target.Layer+1 || d.coversOnLayer(n) {
		t.Fatalf("a note in front of %s: %+v", target.Header, n)
	}
}

// A new structure is moved off the others on its own layer only.
func TestClearSpotOwnLayer(t *testing.T) {
	m := room3DModel(t, 120, 40)
	d := m.ds()
	a := d.octs[1]
	b := d.newKind(kNote, a.X+a.W/2, a.Y+a.H/2)
	b.Layer = 1
	x, y := b.X, b.Y
	if d.clearSpot(b) || b.X != x || b.Y != y {
		t.Fatal("on another layer it stays where it is")
	}
	b.Layer = 0
	if !d.clearSpot(b) {
		t.Fatal("on the same layer it is moved off")
	}
}

// The wall behind a structure out on a layer is drawn without it (its
// words do not show twice); its front shows its own drawing, kept below
// the board.
func TestDesign3DLayerNoGhost(t *testing.T) {
	m := room3DModel(t, 120, 40)
	withPixels(t, m, 8, 16)
	d := m.ds()
	press(m, "ctrl+f")
	o := d.octs[1]
	o.Header = "Ghostly"
	o.Layer = 1
	boardRows(m)
	r := &d.room
	x, y, w, h := d.rect(o)
	text := func(dy int) string {
		s := ""
		for cy := y; cy < y+h; cy++ {
			for cx := x; cx < x+w; cx++ {
				s += r.tex[(cy+dy-r.rm.ty0)*r.rm.tw+cx-r.rm.tx0].glyph
			}
		}
		return s
	}
	var shift int
	for _, b := range r.blocks {
		if b.o == o {
			shift = b.shift
		}
	}
	if shift == 0 {
		t.Fatal("its drawing is kept below the board")
	}
	if strings.Contains(text(0), "Ghostly") {
		t.Fatal("the wall behind it shows its title")
	}
	if !strings.Contains(text(shift), "Ghostly") {
		t.Fatal("its kept drawing has its title")
	}
	for _, f := range r.faces {
		if f.own == o.ID && f.mat == matFront && f.tdy != 2*shift {
			t.Fatalf("its front shows the kept drawing: tdy %d, want %d", f.tdy, 2*shift)
		}
	}
}

// Looking far down the room, a new structure goes no further than
// placeReach in front of the camera.
func TestDesign3DPlaceNotFar(t *testing.T) {
	m := room3DModel(t, 120, 40)
	d := m.ds()
	d.zoom = 0 // 25%: the camera stands far back
	press(m, "ctrl+f")
	r := &d.room
	r.freeK = 1
	r.look = [2]float64{-70, 0} // at the side wall, far off
	boardRows(m)
	press(m, "alt+n")
	press(m, "n")
	o := d.byID(d.sel)
	if o == nil {
		t.Fatal("nothing placed")
	}
	if z := float64(o.Layer) * layerStep; z < r.pos.z-placeReach*r.rm.big-layerStep {
		t.Fatalf("placed too far off: layer %d (z %.0f), camera at z %.0f", o.Layer, z, r.pos.z)
	}
}
