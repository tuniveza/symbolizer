package main

// The flat board and the 3D room move together: one openness, one clock
// (design_anim.go). These tests stop the clock, step it through each move,
// and at every step draw the board flat and then in 3D (turning between
// them mid-move), checking the 3D model is the 3D form of the flat frame.

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// animClock stops the opening and shutting clock for a test (and turns
// moving on); the function it gives moves the clock on.
func animClock(t *testing.T) func(time.Duration) {
	now := time.Unix(1_000_000, 0)
	animNow = func() time.Time { return now }
	designAnims = true
	t.Cleanup(func() { animNow, designAnims = time.Now, false })
	return func(d time.Duration) { now = now.Add(d) }
}

// animSteps are the moments (parts of openAnimFor) the tests look at.
var animSteps = []float64{0, 0.05, 0.15, 0.3, 0.5, 0.7, 0.9, 1.05}

// drawFlat draws the board flat (the 3D view turned off for the moment)
// and is its lines; drawRoom draws it in 3D, turned yaw, pitch.
func drawFlat(m *Model) []string {
	r := &m.ds().room
	yaw, pitch, free := r.yaw, r.pitch, r.free
	r.yaw, r.pitch, r.free = 0, 0, false
	defer func() { r.yaw, r.pitch, r.free = yaw, pitch, free }()
	return boardRows(m)
}

func drawRoom(m *Model, yaw, pitch int) []string {
	r := &m.ds().room
	r.yaw, r.pitch, r.frame = yaw, pitch, ""
	return boardRows(m)
}

// A move turned round part way goes back from exactly where it is drawn,
// and one started again the same way goes on as it was.
func TestAnimTurnsRoundWhereItIs(t *testing.T) {
	step := animClock(t)
	d := &designState{}
	o := &dOct{ID: 7, Kind: kCabinet, DrawerOpen: true}
	d.startOpenAnim(o)
	step(openAnimFor / 4)
	was := d.openness(o)
	d.startOpenAnim(o) // the same way again: nothing changes
	if got := d.openness(o); got != was {
		t.Fatalf("started again the same way, it jumped from %.3f to %.3f", was, got)
	}
	o.DrawerOpen = false
	d.startOpenAnim(o)
	if got := d.openness(o); math.Abs(got-was) > 1e-6 {
		t.Fatalf("turned round at %.3f, it jumped to %.3f", was, got)
	}
	last := was
	for range 40 {
		step(openAnimFor / 32)
		p := d.openness(o)
		if p > last+1e-12 {
			t.Fatalf("shutting, it opened again: %.3f after %.3f", p, last)
		}
		last = p
	}
	if last != 0 {
		t.Fatalf("shut at the end, it is %.3f open", last)
	}
}

// knobFrac is how much of the cabinet's left door faces the front in the
// flat drawing (1 shut, 0 edge on), read off where its knob ● is drawn.
func knobFrac(t *testing.T, rows []string, d *designState, cab *dOct) float64 {
	t.Helper()
	x, y, w, h := d.rect(cab)
	line := rows[y+3+(h-4)/2]
	col := 0
	for _, r := range ansi.Strip(line) {
		if r == '●' && col > x && col < x+w/2 {
			dw := col - x + 1 // the door's width: its knob is its last cell but one
			return float64(dw-cabDoorW) / float64((w-2)/2-cabDoorW)
		}
		col += ansi.StringWidth(string(r))
	}
	t.Fatalf("no knob on the cabinet's left door: %q", ansi.Strip(line))
	return 0
}

// A cabinet's doors: at every step the 3D doors are turned as far as the
// flat doors are drawn turned (the width the flat drawing gives each door
// is the 3D door's seen from the front), opening, turned round half way,
// and shutting; never all the way before the end.
func TestAnimCabinetFlatAnd3D(t *testing.T) {
	step := animClock(t)
	m, cab := cabinetScene(t)
	d := m.ds()
	r := &d.room
	tol := 1.5 / float64((cab.W-2)/2-cabDoorW)
	look := func(what string, until float64) {
		at := 0.0
		for _, s := range animSteps {
			if s > until {
				break
			}
			step(time.Duration((s - at) * float64(openAnimFor)))
			at = s
			flat := knobFrac(t, drawFlat(m), d, cab)
			p := d.openness(cab) // as that frame showed it
			drawRoom(m, 20, 10)
			c := r.cab[cab.ID]
			if c == nil {
				t.Fatalf("%s %.2f: the cabinet is not a cupboard in 3D", what, s)
			}
			if c.p != p {
				t.Fatalf("%s %.2f: 3D has the doors %.3f open, the flat board %.3f", what, s, c.p, p)
			}
			face := math.Max(0, math.Cos(c.ang[0]))
			if math.Abs(face-flat) > tol || math.Abs(c.ang[0]-c.ang[1]) > 1e-9 {
				t.Fatalf("%s %.2f: the 3D door faces %.2f of the front (turned %.0f°), the flat one is drawn %.2f", what, s, face, c.ang[0]*180/math.Pi, flat)
			}
			if p > 0 && p < 1 && (c.ang[0] <= 0 || c.ang[0] >= doorTurn(1)-1e-9) {
				t.Fatalf("%s %.2f: part open (%.2f), the 3D doors are turned %.0f°", what, s, p, c.ang[0]*180/math.Pi)
			}
		}
	}
	m.designDoors(cab) // open
	look("opening", 1.1)
	m.designDoors(cab) // shut, and half way open again
	step(openAnimFor / 2)
	was := d.openness(cab)
	m.designDoors(cab)
	if got := d.openness(cab); math.Abs(got-was) > 1e-6 {
		t.Fatalf("turned round half way, the doors jumped from %.3f to %.3f", was, got)
	}
	look("opening again", 1.1)

	// Seen straight on the cabinet is too flat to be a cupboard: a plain
	// block showing the flat drawing, whose doors swing as on the flat board.
	m.designDoors(cab)
	step(openAnimFor / 3)
	drawRoom(m, 1, 0)
	if r.cab[cab.ID] != nil {
		t.Fatal("seen straight on, the cabinet should be a plain block")
	}
	if p := d.openness(cab); d.cabinetDrawnOpen(cab, p) != p {
		t.Fatalf("a plain block's flat drawing should have the doors %.2f open, not %.2f", p, d.cabinetDrawnOpen(cab, p))
	}
}

// boxLid3D is how many inside rows the 3D box's lid covers (from the top),
// and whether any of its things are in the room.
func boxLid3D(r *room3D, d *designState, b *dOct, rows []int) (int, bool) {
	_, y, _, h := d.rect(b)
	dep := 0.0
	for _, bl := range r.blocks {
		if bl.o == b {
			dep = bl.dep
		}
	}
	items := map[int]bool{}
	for _, it := range b.Items {
		items[it.ID] = true
	}
	covered, things := 0, false
	for fi := range r.faces {
		f := &r.faces[fi]
		if items[f.own] {
			things = true
		}
		if f.own != b.ID || f.mat != matFront || r.pts[f.p0].z < dep+0.1 {
			continue
		}
		if f.py1 == 2*(y+h) {
			covered = len(rows) // the whole lid
			continue
		}
		n := 0
		for _, row := range rows {
			if 2*(y+row) < f.py1 {
				n++
			}
		}
		covered = max(covered, n)
	}
	return covered, things
}

// boxLidFlat is how many inside rows the flat drawing's lid covers: the
// rows from the top where none of the box's floor shows.
func boxLidFlat(g *boxGrid, rows []int) int {
	n := 0
	for _, row := range rows {
		for _, c := range g.c[row] {
			if c.act == "boxfloor" {
				return n
			}
		}
		n++
	}
	return n
}

// Every design of box: at every step of shutting and opening, the 3D lid
// covers the rows the flat lid covers (sliding off from the bottom up),
// and the things show in 3D exactly while they show on the flat board.
func TestAnimBoxFlatAnd3D(t *testing.T) {
	step := animClock(t)
	m, boxes := boxes3DScene(t, 120, 40, designZoom100)
	d := m.ds()
	r := &d.room
	look := func(what string) {
		at := 0.0
		for _, s := range animSteps {
			step(time.Duration((s - at) * float64(openAnimFor)))
			at = s
			drawFlat(m)
			flat := map[int]int{}
			for _, b := range boxes {
				_, _, w, h := d.rect(b)
				rows := boxShapeOf(boxDesign(b), w, h).insideRows(w, h)
				flat[b.ID] = boxLidFlat(d.boxGridOf(b, w, h), rows)
			}
			drawRoom(m, 20, 15)
			for _, b := range boxes {
				_, _, w, h := d.rect(b)
				rows := boxShapeOf(boxDesign(b), w, h).insideRows(w, h)
				got, things := boxLid3D(r, d, b, rows)
				if got != flat[b.ID] {
					t.Fatalf("%s %.2f, %s: the 3D lid covers %d rows, the flat one %d of %d", what, s, boxDesignNames[boxDesign(b)], got, flat[b.ID], len(rows))
				}
				if things != (got < len(rows)) {
					t.Fatalf("%s %.2f, %s: lid on %d of %d rows, things in 3D %v", what, s, boxDesignNames[boxDesign(b)], got, len(rows), things)
				}
				if s == 0 && got == len(rows) && !b.DrawerOpen {
					t.Fatalf("%s, %s: just started shutting, the 3D lid is on already", what, boxDesignNames[boxDesign(b)])
				}
			}
		}
	}
	for _, b := range boxes {
		b.DrawerOpen = false
		d.startOpenAnim(b)
	}
	look("shutting")
	for _, b := range boxes {
		m.designBoxLid(b)
	}
	look("opening")
}

// A drawer unit with several drawers: one pulled out, then the next (the
// first going in as it comes out). At every step its 3D block is as tall
// as the flat unit, its front the flat drawing's rows, and both are part
// way while the drawers move.
func TestAnimDrawerFlatAnd3D(t *testing.T) {
	step := animClock(t)
	m := room3DModel(t, 200, 60)
	d := m.ds()
	d.octs = nil
	u := d.newKind(kDrawer, 0, 0)
	u.X, u.Y, u.Drawers, u.Header = 20, 2, 3, "Tools"
	u.Items = []*dOct{
		{ID: 701, Kind: kPage, Header: "Saw", Slot: 0}, {ID: 702, Kind: kNote, Header: "Nails", Slot: 0},
		{ID: 703, Kind: kPage, Header: "Glue", Slot: 1}, {ID: 704, Kind: kBook, Header: "Tape", Slot: 2},
	}
	d.sel = -1
	look := func(what string, from, to int) {
		at, last := 0.0, from
		for _, s := range animSteps {
			step(time.Duration((s - at) * float64(openAnimFor)))
			at = s
			drawFlat(m)
			_, fy, _, fh := d.rect(u)
			drawRoom(m, 20, 10)
			var blk *r3block
			for i := range m.ds().room.blocks {
				if b := &m.ds().room.blocks[i]; b.o == u {
					blk = b
				}
			}
			if blk == nil {
				t.Fatalf("%s %.2f: the drawer unit is not in the room", what, s)
			}
			if blk.y0 != fy || blk.y1-blk.y0 != fh {
				t.Fatalf("%s %.2f: the 3D unit is rows %d…%d, the flat one %d…%d", what, s, blk.y0, blk.y1, fy, fy+fh)
			}
			if (fh-last)*(to-from) < 0 {
				t.Fatalf("%s %.2f: the unit went back from %d to %d rows", what, s, last, fh)
			}
			if s > 0.1 && s < 0.5 && (fh == from || fh == to) && from != to {
				t.Fatalf("%s %.2f: part way, the unit is %d rows (from %d to %d)", what, s, fh, from, to)
			}
			last = fh
		}
		if last != to {
			t.Fatalf("%s: at the end the unit is %d rows, want %d", what, last, to)
		}
	}
	shut := drawerLines(u)
	m.designPull(u, false) // drawer 1 out
	look("drawer 1 out", shut, drawerLines(u))
	one := drawerLines(u)
	m.designPull(u, false) // drawer 2 out, drawer 1 in
	look("drawer 2 for 1", one, drawerLines(u))
}

// Undo puts back a board whose cabinet was shut: its doors swing shut,
// flat and in 3D, as a click would shut them. A page filed into a shut
// cabinet opens its doors the same way.
func TestAnimUndoAndFiling(t *testing.T) {
	step := animClock(t)
	m, cab := cabinetScene(t)
	d := m.ds()
	d.recordUndo() // shut
	cab.DrawerOpen = true
	cab.Header = "Larder"
	d.recordUndo()
	m.designUndo()
	c := d.byID(cab.ID)
	if c.DrawerOpen || d.openness(c) != 1 {
		t.Fatalf("just undone, the doors should start swinging shut from open: open %v, %.2f", c.DrawerOpen, d.openness(c))
	}
	step(openAnimFor / 3)
	drawRoom(m, 20, 10)
	if p := d.room.cab[c.ID].p; p <= 0 || p >= 1 {
		t.Fatalf("a third of the way, the 3D doors should be part shut: %.2f", p)
	}
	step(openAnimFor)
	drawFlat(m)
	if d.openness(c) != 0 {
		t.Fatal("at the end, undone, the doors should be shut")
	}

	pg := d.newKind(kPage, 0, 0)
	pg.Header = "Soup"
	m.designFileInto(pg, c)
	if !c.DrawerOpen || !d.moving(c.ID) || d.openness(c) != 0 {
		t.Fatalf("filed into a shut cabinet, its doors should start to open: open %v, moving %v, %.2f", c.DrawerOpen, d.moving(c.ID), d.openness(c))
	}
}

// SY_ANIM_PNG=dir go test -run TestAnimPictures writes strips of 3D frames
// (a cabinet, a gift box, a cardboard box, a drawer unit) as they open,
// left to right, to look at.
func TestAnimPictures(t *testing.T) {
	dir := os.Getenv("SY_ANIM_PNG")
	if dir == "" {
		t.Skip("set SY_ANIM_PNG to a folder")
	}
	moments := []float64{0, 0.1, 0.25, 0.45, 0.7, 1.05}
	strip := func(name string, m *Model, yaw, pitch int, start func()) {
		step := animClock(t)
		withPixels(t, m, 6, 12)
		var frames []*image.RGBA
		room3DSeeRGB = func(rgb []byte, w, h int) {
			img := image.NewRGBA(image.Rect(0, 0, w, h))
			for i := 0; i < w*h; i++ {
				img.Set(i%w, i/w, color.RGBA{rgb[3*i], rgb[3*i+1], rgb[3*i+2], 255})
			}
			frames = append(frames, img)
		}
		defer func() { room3DSeeRGB = nil }()
		start()
		at := 0.0
		for _, s := range moments {
			step(time.Duration((s - at) * float64(openAnimFor)))
			at = s
			drawRoom(m, yaw, pitch)
		}
		if len(frames) != len(moments) {
			t.Fatalf("%s: %d pictures for %d moments", name, len(frames), len(moments))
		}
		fw, fh := frames[0].Bounds().Dx(), frames[0].Bounds().Dy()
		out := image.NewRGBA(image.Rect(0, 0, fw*len(frames)+4*(len(frames)-1), fh))
		for i, f := range frames {
			for y := range fh {
				for x := range fw {
					out.Set(i*(fw+4)+x, y, f.At(x, y))
				}
			}
		}
		file, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if err := png.Encode(file, out); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s (%d frames, %dx%d each)", file.Name(), len(frames), fw, fh)
	}

	m, cab := cabinetScene(t)
	m.ds().octs = []*dOct{cab}
	cab.X, cab.Y = 10, 3
	strip("cabinet", m, 25, 10, func() { m.designDoors(cab) })

	for _, design := range []int{boxGift, boxCardboard, boxHex} {
		m, boxes := boxes3DScene(t, 60, 24, designZoom100)
		b := boxes[design]
		b.X, b.Y = 4, 2
		m.ds().octs = []*dOct{b}
		b.DrawerOpen = false
		strip(fmt.Sprintf("box_%s", strings.ReplaceAll(boxDesignNames[design], " ", "_")), m, 20, 15, func() { m.designBoxLid(b) })
	}

	m = room3DModel(t, 80, 40)
	d := m.ds()
	d.octs = nil
	u := d.newKind(kDrawer, 0, 0)
	u.X, u.Y, u.Drawers, u.Header = 10, 1, 3, "Tools"
	u.Items = []*dOct{{ID: 701, Kind: kPage, Header: "Saw"}, {ID: 702, Kind: kNote, Header: "Nails"}, {ID: 703, Kind: kPage, Header: "Glue", Slot: 1}}
	d.sel = -1
	strip("drawers", m, 20, 10, func() { m.designPull(u, false) })
}

// ---- every kind together, every way of starting a move ----------------------------------

// animScene is a board with a cabinet, a box (a gift box, shut) and a
// drawer unit of n drawers, each big enough to be its 3D model.
func animScene(t *testing.T, n int) (m *Model, cab, box, unit *dOct) {
	t.Helper()
	m, cab = cabinetScene(t)
	d := m.ds()
	box = d.newKind(kBox, 0, 0)
	box.X, box.Y, box.W, box.H, box.Header, box.Layout, box.DrawerOpen = 110, 3, 34, 14, "Gifts", boxGift, false
	box.Items = []*dOct{{ID: 901, Kind: kNote, Header: "Keys", X: 2, Y: 1}, {ID: 902, Kind: kPage, Header: "Plan", X: 9, Y: 3, Slot: 1}}
	unit = d.newKind(kDrawer, 0, 0)
	unit.X, unit.Y, unit.Drawers, unit.Header = 4, 14, n, "Tools"
	if n == 1 {
		unit.Drawers = 0
	}
	for i := range max(n, 1) {
		unit.Items = append(unit.Items, &dOct{ID: 950 + i, Kind: kPage, Header: fmt.Sprint("Thing ", i), Slot: i})
	}
	d.octs = []*dOct{cab, box, unit}
	d.sel = -1
	return m, cab, box, unit
}

// agree draws the board flat and then in 3D (turned yaw, pitch), both at
// the clock's moment, and fails unless every cabinet, box and drawer unit
// on it is in the 3D room the 3D form of its flat frame: the doors turned
// as far as the flat doors are drawn turned, the lid over the rows the
// flat lid covers, the unit as tall as the flat one. It gives how open
// each is (cabinets and boxes) or how tall (drawer units), by ID.
func agree(t *testing.T, m *Model, what string, yaw, pitch int) map[int]float64 {
	t.Helper()
	d := m.ds()
	r := &d.room
	defer func(yaw, pitch int) { r.yaw, r.pitch = yaw, pitch }(r.yaw, r.pitch)
	rows := drawFlat(m)
	got := map[int]float64{}
	flatDoor, flatLid, flatH := map[int]float64{}, map[int]int{}, map[int][2]int{}
	for _, o := range d.octs {
		_, y, w, h := d.rect(o)
		switch o.Kind {
		case kCabinet:
			got[o.ID] = d.openness(o)
			flatDoor[o.ID] = knobFrac(t, rows, d, o)
		case kBox:
			got[o.ID] = d.openness(o)
			flatLid[o.ID] = boxLidFlat(d.boxGridOf(o, w, h), boxShapeOf(boxDesign(o), w, h).insideRows(w, h))
		case kDrawer:
			got[o.ID] = float64(h)
			flatH[o.ID] = [2]int{y, h}
		}
	}
	drawRoom(m, yaw, pitch)
	for _, o := range d.octs {
		_, _, w, h := d.rect(o)
		switch o.Kind {
		case kCabinet:
			c := r.cab[o.ID]
			if c == nil {
				t.Fatalf("%s: cabinet %d is not a cupboard in 3D", what, o.ID)
			}
			if c.p != got[o.ID] {
				t.Fatalf("%s: 3D has the doors %.3f open, the flat frame %.3f", what, c.p, got[o.ID])
			}
			if c.p == 0 && flatDoor[o.ID] < 0.9 {
				t.Fatalf("%s: shut in 3D, the flat doors are drawn turned (%.2f)", what, flatDoor[o.ID])
			}
			if c.p > 0 {
				tol := 1.5 / float64((w-2)/2-cabDoorW)
				if face := math.Max(0, math.Cos(c.ang[0])); math.Abs(face-flatDoor[o.ID]) > tol {
					t.Fatalf("%s: the 3D door faces %.2f of the front, the flat one %.2f", what, face, flatDoor[o.ID])
				}
			}
			if p := c.p; p > 0 && p < 1 && (c.ang[0] <= 0 || c.ang[0] >= doorTurn(1)-1e-9) {
				t.Fatalf("%s: part open (%.2f), the 3D doors are turned %.0f°", what, p, c.ang[0]*180/math.Pi)
			}
		case kBox:
			rows := boxShapeOf(boxDesign(o), w, h).insideRows(w, h)
			lid, things := boxLid3D(r, d, o, rows)
			if lid != flatLid[o.ID] {
				t.Fatalf("%s: the 3D lid covers %d rows, the flat one %d of %d", what, lid, flatLid[o.ID], len(rows))
			}
			if things != (lid < len(rows)) {
				t.Fatalf("%s: lid on %d of %d rows, things in 3D %v", what, lid, len(rows), things)
			}
		case kDrawer:
			var blk *r3block
			for i := range r.blocks {
				if r.blocks[i].o == o {
					blk = &r.blocks[i]
				}
			}
			if blk == nil {
				t.Fatalf("%s: the drawer unit is not in the room", what)
			}
			if f := flatH[o.ID]; blk.y0 != f[0] || blk.y1-blk.y0 != f[1] {
				t.Fatalf("%s: the 3D unit is rows %d…%d, the flat frame %d…%d", what, blk.y0, blk.y1, f[0], f[0]+f[1])
			}
		}
	}
	return got
}

// walk steps the clock through a move, checking at every step (agree)
// that flat and 3D are one, that nothing goes back the way it came, and
// that nothing is where it ends before the end. ends are where each
// structure ends (openness, or a drawer unit's rows).
func walk(t *testing.T, m *Model, step func(time.Duration), what string, ends map[int]float64) {
	t.Helper()
	start := agree(t, m, what+" at the start", 20, 10)
	last := start
	at := 0.0
	for _, s := range animSteps[1:] {
		step(time.Duration((s - at) * float64(openAnimFor)))
		at = s
		now := agree(t, m, fmt.Sprintf("%s %.2f", what, s), 20, 10)
		for id, end := range ends {
			if (now[id]-last[id])*(end-start[id]) < 0 {
				t.Fatalf("%s %.2f: %d went back, from %.3f to %.3f (towards %.3f)", what, s, id, last[id], now[id], end)
			}
			if s <= 0.3 && s > 0.05 && start[id] != end && now[id] == end && math.Abs(end-start[id]) > 1 {
				t.Fatalf("%s %.2f: %d is already where it ends (%.3f)", what, s, id, end)
			}
			if s >= 1 && now[id] != end {
				t.Fatalf("%s: at the end %d is %.3f, want %.3f", what, id, now[id], end)
			}
		}
		last = now
	}
}

// Undo and redo put back doors, lids and drawers that stood otherwise:
// they move there, flat and 3D as one, every kind at once.
func TestAnimUndoRedoEveryKind(t *testing.T) {
	step := animClock(t)
	m, cab, box, unit := animScene(t, 3)
	d := m.ds()
	shutH := float64(drawerLines(unit))
	d.undoBase, d.undo, d.redo = nil, nil, nil
	d.recordUndo() // the board as it starts: all shut
	m.designDoors(cab)
	m.designBoxLid(box)
	m.designPull(unit, false)
	outH := float64(drawerLines(unit))
	cab.Header = "Larder" // opening alone is no step to undo: a change with it is
	walk(t, m, step, "opening", map[int]float64{cab.ID: 1, box.ID: 1, unit.ID: outH})
	d.recordUndo()
	m.designUndo()
	cab, box, unit = d.byID(cab.ID), d.byID(box.ID), d.byID(unit.ID)
	walk(t, m, step, "undone", map[int]float64{cab.ID: 0, box.ID: 0, unit.ID: shutH})
	m.designRedo()
	cab, box, unit = d.byID(cab.ID), d.byID(box.ID), d.byID(unit.ID)
	walk(t, m, step, "redone", map[int]float64{cab.ID: 1, box.ID: 1, unit.ID: outH})

	// Undone half way through a move, it turns round where it is.
	m.designUndo()
	step(openAnimFor / 3)
	mid := agree(t, m, "undone, a third of the way", 20, 10)
	m.designRedo()
	if now := agree(t, m, "redone there", 20, 10); math.Abs(now[cab.ID]-mid[cab.ID]) > 1e-6 || math.Abs(now[box.ID]-mid[box.ID]) > 1e-6 || now[unit.ID] != mid[unit.ID] {
		t.Fatalf("redone part way, things jumped: %v, then %v", mid, now)
	}
	walk(t, m, step, "redone part way", map[int]float64{cab.ID: 1, box.ID: 1, unit.ID: outH})
}

// Drawer units of 1, 2, 4 and 6 drawers: each drawer pulled out by Tab in
// turn (the one before going in), then pushed in; flat and 3D as one.
func TestAnimDrawerCounts(t *testing.T) {
	for _, n := range []int{1, 2, 4, 6} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			step := animClock(t)
			m, _, _, unit := animScene(t, n)
			d := m.ds()
			d.sel = unit.ID
			press(m, "tab") // its name
			for i := range n {
				for k := 0; k < 50 && (d.sect.cur.kind != secDrawer || d.sect.cur.slot != i); k++ {
					press(m, "tab") // (past the things in the drawer before)
				}
				if !slotOpen(unit, i) {
					t.Fatalf("Tab %d: drawer %d should be out", i+1, i)
				}
				walk(t, m, step, fmt.Sprintf("Tab %d", i+1), map[int]float64{unit.ID: float64(drawerLines(unit))})
			}
			m.designPullSlot(unit, n-1)
			walk(t, m, step, "pushed in", map[int]float64{unit.ID: float64(drawerLines(unit))})
		})
	}
}

// Space, a click on the flat board and a click in 3D all move the doors
// the same way: the same openness and the same 3D turn at every step.
func TestAnimSpaceAndClickAlike(t *testing.T) {
	record := func(how string, open func(m *Model, cab *dOct)) []float64 {
		step := animClock(t)
		m, cab := cabinetScene(t)
		d := m.ds()
		d.room.yaw, d.room.pitch = 25, 15
		boardRows(m)
		open(m, cab)
		if !cab.DrawerOpen || !d.moving(cab.ID) {
			t.Fatalf("%s: the doors should start to open", how)
		}
		var got []float64
		at := 0.0
		for _, s := range animSteps {
			step(time.Duration((s - at) * float64(openAnimFor)))
			at = s
			drawRoom(m, 25, 15)
			c := d.room.cab[cab.ID]
			got = append(got, c.p, c.ang[0], c.ang[1])
		}
		return got
	}
	space := record("Space", func(m *Model, cab *dOct) {
		m.ds().sel = cab.ID
		press(m, "space")
	})
	flat := record("a flat click", func(m *Model, cab *dOct) {
		d := m.ds()
		r := &d.room
		r.yaw, r.pitch = 0, 0
		boardRows(m)
		x, y, w, h := d.rect(cab)
		for sy := y; sy < y+h; sy++ {
			for sx := x + 3; sx < x+w-3; sx++ {
				if d.hit(sx, sy).act == "doors" {
					d.lastClick = time.Time{}
					click(m, sx, sy+m.boardTop(), tea.MouseLeft)
					r.yaw, r.pitch = 25, 15
					return
				}
			}
		}
		t.Fatal("no door to click on the flat board")
	})
	in3D := record("a 3D click", func(m *Model, cab *dOct) {
		d := m.ds()
		x, _, w, _ := d.rect(cab)
		col, row := cell3D(t, m, cab.ID, func(sx, sy int) bool {
			return sx > x+3 && sx < x+w-3 && d.hit(sx, sy).act == "doors"
		})
		d.lastClick = time.Time{}
		click(m, col, row, tea.MouseLeft)
	})
	for i := range space {
		if space[i] != flat[i] || space[i] != in3D[i] {
			t.Fatalf("step %d: Space %.4f, flat click %.4f, 3D click %.4f", i/3, space[i], flat[i], in3D[i])
		}
	}
}

// Ctrl+F part way through a move: the room goes on from where the flat
// board was, and back flat (Esc) the flat board goes on from where the
// room was; looking round, the cabinet comes out of the wall as a
// cupboard with its doors turned as far as the flat ones.
func TestAnimCtrlFMidMove(t *testing.T) {
	step := animClock(t)
	m, cab := cabinetScene(t)
	d := m.ds()
	r := &d.room
	boardRows(m)
	m.designDoors(cab)
	step(openAnimFor / 5)
	flat := knobFrac(t, boardRows(m), d, cab)
	p0 := d.openness(cab)
	press(m, "ctrl+f")
	if !r.free {
		t.Fatal("Ctrl+F should turn the room 3D")
	}
	boardRows(m)
	if p := d.openness(cab); p != p0 {
		t.Fatalf("turned 3D, the doors jumped from %.3f to %.3f", p0, p)
	}
	if c := r.cab[cab.ID]; c != nil {
		if face := math.Max(0, math.Cos(c.ang[0])); math.Abs(face-flat) > 0.1 {
			t.Fatalf("turned 3D, the 3D doors face %.2f of the front, the flat ones were %.2f", face, flat)
		}
	} else if d.cabinetDrawnOpen(cab, p0) != p0 {
		t.Fatal("still flat on the wall, the cabinet should show the flat doors as they were")
	}
	r.look[0] += 35 // look round: the blocks come out
	step(openAnimFor / 5)
	boardRows(m)
	c := r.cab[cab.ID]
	if c == nil {
		t.Fatal("looking round, the cabinet should be a cupboard")
	}
	p1 := d.openness(cab)
	if c.p != p1 || p1 <= p0 || p1 >= 1 {
		t.Fatalf("in 3D the doors are %.3f open (before %.3f), the clock has them %.3f", c.p, p0, p1)
	}
	if c.ang[0] <= 0 || c.ang[0] > doorTurn(p1)+1e-9 {
		t.Fatalf("the 3D doors are turned %.1f°, %.1f° at %.3f", c.ang[0]*180/math.Pi, doorTurn(p1)*180/math.Pi, p1)
	}
	press(m, "esc")
	if r.on() {
		t.Fatal("Esc should make the board flat")
	}
	flat = knobFrac(t, boardRows(m), d, cab)
	if p := d.openness(cab); p != p1 || math.Abs(flat-doorFace(p1)) > 0.1 {
		t.Fatalf("flat again, the doors are %.3f open (were %.3f), drawn facing %.2f, want %.2f", p, p1, flat, doorFace(p1))
	}
	step(openAnimFor)
	boardRows(m)
	if d.openness(cab) != 1 {
		t.Fatal("at the end the doors should be open")
	}
}

// While a frame is drawn the clock goes on (here every reading of it a
// little later): still the whole frame, the flat drawing on the wall and
// the 3D model alike, shows one moment.
func TestAnimOneMomentPerFrame(t *testing.T) {
	animClock(t)
	now := time.Unix(2_000_000, 0)
	animNow = func() time.Time { now = now.Add(3 * time.Millisecond); return now }
	m, _, box, unit := animScene(t, 4)
	d := m.ds()
	boardRows(m)
	m.designBoxLid(box)
	d.sel = unit.ID
	press(m, "tab") // its name
	press(m, "tab") // its first drawer, pulled out
	for i := range 12 {
		drawRoom(m, 20, 10)
		r := &d.room
		_, y, w, h := d.rect(box)
		rows := boxShapeOf(boxDesign(box), w, h).insideRows(w, h)
		lid, _ := boxLid3D(r, d, box, rows)
		if flat := boxLidFlat(d.boxGridOf(box, w, h), rows); lid != flat {
			t.Fatalf("frame %d: the 3D lid covers %d rows, the frame's flat lid %d", i, lid, flat)
		}
		_, y, _, h = d.rect(unit)
		for _, b := range r.blocks {
			if b.o == unit && (b.y0 != y || b.y1 != y+h) {
				t.Fatalf("frame %d: the 3D unit is rows %d…%d, the frame's flat one %d…%d", i, b.y0, b.y1, y, y+h)
			}
		}
	}
}

// While drawers slide or a lid opens, the room in 3D and its camera stay
// still, as the flat board does: the room changes once (as a drawer
// starts out, or once it is all the way in), never row by row under the
// moving unit.
func TestAnimRoomStaysStill(t *testing.T) {
	step := animClock(t)
	m, cab, box, unit := animScene(t, 3)
	unit.Y = 30 // the lowest thing: the room's floor is under it
	d := m.ds()
	r := &d.room
	type seen struct {
		room [4]int
		cam  r3vec
	}
	look := func() seen {
		drawRoom(m, 20, 10)
		return seen{[4]int{r.rm.tx0, r.rm.ty0, r.rm.tw, r.rm.th}, r.view.cam}
	}
	for _, way := range []string{"out", "in"} {
		look()
		m.designPullSlot(unit, 1)
		var first seen
		at := 0.0
		for i, s := range animSteps[1 : len(animSteps)-1] {
			step(time.Duration((s - at) * float64(openAnimFor)))
			at = s
			now := look()
			if i == 0 {
				first = now
				continue
			}
			if now != first {
				t.Fatalf("drawer going %s, %.2f: the room or camera moved: %+v, then %+v", way, s, first, now)
			}
		}
		step(openAnimFor)
		look()
	}

	// A lid or doors do not change the room at all.
	for _, o := range []*dOct{box, cab, box, cab} {
		before := look()
		if o == box {
			m.designBoxLid(o)
		} else {
			m.designDoors(o)
		}
		for _, s := range animSteps {
			step(openAnimFor / time.Duration(len(animSteps)))
			if now := look(); now != before {
				t.Fatalf("%s opening or shutting (%.2f): the room or camera moved: %+v, then %+v", o.Kind, s, before, now)
			}
		}
		step(openAnimFor)
	}
}
