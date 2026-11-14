package main

import (
	"math"
	"strings"
	"testing"
	"time"

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
	if dz := front() - z0; math.Abs(dz-layerStep) > 0.05 {
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

// Every kind out on a layer keeps its drawing for its front (cabinets,
// drawer units, racks and boards are drawn in parts read only when put on
// the screen: they are read for the kept copy too).
func TestDesign3DLayerCopyEveryKind(t *testing.T) {
	m := room3DModel(t, 120, 40)
	withPixels(t, m, 8, 16)
	d := m.ds()
	press(m, "ctrl+f")
	r := &d.room
	for _, o := range d.octs {
		o.Layer = 1
		boardRows(m)
		for _, b := range r.blocks {
			if b.o != o {
				continue
			}
			x, y, w, h := d.rect(o)
			n := 0
			for cy := y; cy < y+h; cy++ {
				for cx := x; cx < x+w; cx++ {
					if r.tex[(cy+b.shift-r.rm.ty0)*r.rm.tw+cx-r.rm.tx0].glyph != "" {
						n++
					}
				}
			}
			if n == 0 {
				t.Errorf("%s %q on a layer: its kept drawing is empty", o.Kind, o.Header)
			}
		}
		o.Layer = 0
	}
}

// A structure far out on a layer does not push the camera back nor narrow
// its view, nor move the lamp out.
func TestDesign3DDeepLayerKeepsCamera(t *testing.T) {
	start := func(layer int) (r3vec, float64, r3vec) {
		m := room3DModel(t, 120, 40)
		d := m.ds()
		d.octs[2].Layer = layer
		press(m, "ctrl+f")
		boardRows(m)
		r := &d.room
		return r.pos, r.ff, lampAt(&r.rm)
	}
	p0, f0, l0 := start(0)
	p1, f1, l1 := start(28)
	if p0 != p1 || f0 != f1 || l0 != l1 {
		t.Fatalf("layer 28: camera %v lens %.0f lamp %v, without it %v %.0f %v", p1, f1, l1, p0, f0, l0)
	}
}

// Anything that changes the flat board's zoom while in 3D goes to the
// lens: the board stays at 100% and the cross stays on its spot.
func TestDesign3DZoomGuard(t *testing.T) {
	m := room3DModel(t, 120, 40)
	d := m.ds()
	press(m, "ctrl+f")
	d.room.lookBy(8, 3)
	bx, by := boardAtCross(t, m)
	d.zoomTo(designZoom100+2, 30, 10) // as the mouse wheel or a reload would
	x, y := boardAtCross(t, m)
	if d.zoom != designZoom100 || math.Abs(x-bx) > 0.5 || math.Abs(y-by) > 0.5 {
		t.Fatalf("zoom %d, cross at %.1f,%.1f, was %.1f,%.1f", d.zoom, x, y, bx, by)
	}
	if d.room.lens <= 1 {
		t.Fatalf("the lens takes the zoom: %v", d.room.lens)
	}
}

// Flat against the wall (going into 3D), a structure on a higher layer is
// in front of one on the wall where they overlap.
func TestDesign3DLayerOrderFlat(t *testing.T) {
	m := room3DModel(t, 120, 40)
	d := m.ds()
	a := d.octs[1]
	b := d.newKind(kNote, a.X+a.W/2, a.Y+a.H/2)
	b.Layer = 1
	press(m, "ctrl+f")
	boardRows(m)
	r := &d.room
	fz := map[int]float64{}
	for _, f := range r.faces {
		if f.mat == matFront && (f.own == a.ID || f.own == b.ID) {
			fz[f.own] = r.pts[f.p0].z
		}
	}
	if !(fz[b.ID] > fz[a.ID]) {
		t.Fatalf("the layer 1 note's front %v is not in front of the page's %v", fz[b.ID], fz[a.ID])
	}
}

// At every lens zoom, the cross picks the structure it is on, a click
// chooses it, a drag moves it the way the camera turns, and Alt + N
// places a new one centred on the cross.
func TestDesign3DEveryLens(t *testing.T) {
	for _, lens := range []float64{0.4, 1, 2.5, 6} {
		m := room3DModel(t, 120, 40)
		d := m.ds()
		r := &d.room
		press(m, "ctrl+f")
		r.lens = lens
		t.Logf("lens %v", lens)
		o := d.octs[1]
		x, y := cell3D(t, m, o.ID, func(sx, sy int) bool {
			px, py, pw, ph := d.rect(o)
			return sx > px+2 && sx < px+pw-2 && sy > py+3 && sy < py+ph-2
		})
		if h := r.pickAim(); h.own != o.ID {
			t.Fatalf("lens %v: the cross is on %d, want %d", lens, h.own, o.ID)
		}
		d.sel, d.lastClick = -1, time.Time{}
		click(m, x, y, tea.MouseLeft)
		if d.sel != o.ID {
			t.Fatalf("lens %v: a click chose %d, want %d", lens, d.sel, o.ID)
		}
		x0, y0 := o.X, o.Y
		d.lastClick = time.Time{}
		dragTo(m, x, y, x+2, y+1, tea.MouseLeft)
		if o.X <= x0 || o.Y < y0 {
			t.Fatalf("lens %v: dragged right (and down): at %d,%d, was %d,%d", lens, o.X, o.Y, x0, y0)
		}
		room3DCell(t, m, matBoard)
		bx, by := boardAtCross(t, m)
		press(m, "alt+n")
		press(m, "p")
		n := d.byID(d.sel)
		w, h := d.size(n)
		if n.Layer == 0 && (math.Abs(float64(n.X)+float64(w)/2-bx) > 1.5 || math.Abs(float64(n.Y)+float64(h)/2-by) > 1.5) {
			t.Fatalf("lens %v: placed at %d,%d (%dx%d), the cross at %.1f,%.1f", lens, n.X, n.Y, w, h, bx, by)
		}
		press(m, "esc")
	}
}

// The camera cannot walk out of the room, however far it goes.
func TestDesign3DWalkStaysInRoom(t *testing.T) {
	m := room3DModel(t, 120, 40)
	d := m.ds()
	press(m, "ctrl+f")
	boardRows(m)
	r := &d.room
	for _, k := range []string{"d", "a", "w", "s", "q", "e"} {
		for i := 0; i < 200; i++ {
			press(m, "shift+"+k)
		}
		boardRows(m)
		rm := &r.rm
		if p := r.pos; p.x <= rm.wx0 || p.x >= rm.wx1 || p.y <= rm.fy || p.y >= rm.cy || p.z <= 0 || p.z >= rm.fz {
			t.Fatalf("after %s: the camera at %v is out of the room (x %.0f..%.0f y %.0f..%.0f z 0..%.0f)", k, p, rm.wx0, rm.wx1, rm.fy, rm.cy, rm.fz)
		}
	}
}

// Saved while in 3D, the board keeps the flat board's own zoom and place.
func TestDesign3DSavesFlatZoom(t *testing.T) {
	m := room3DModel(t, 120, 40)
	d := m.ds()
	d.zoom, d.camX, d.camY = 2, -10, -5
	press(m, "ctrl+f")
	_, f := d.snapshot()
	if f.Zoom != 2 || f.CamX != -10 || f.CamY != -5 {
		t.Fatalf("saved in 3D: zoom %d cam %v,%v", f.Zoom, f.CamX, f.CamY)
	}
}
