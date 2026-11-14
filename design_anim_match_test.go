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
	"testing"
	"time"

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
			p := d.openness(cab)
			flat := knobFrac(t, drawFlat(m), d, cab)
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

	for _, design := range []int{boxGift, boxCardboard} {
		m, boxes := boxes3DScene(t, 60, 24, designZoom100)
		b := boxes[design]
		b.X, b.Y = 4, 2
		m.ds().octs = []*dOct{b}
		b.DrawerOpen = false
		strip(fmt.Sprintf("box_%s", boxDesignNames[design]), m, 20, 15, func() { m.designBoxLid(b) })
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
