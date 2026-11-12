package main

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// cabinetScene is a 3D room with a page and a cabinet holding a few
// things, at 100%.
func cabinetScene(t *testing.T) (*Model, *dOct) {
	t.Helper()
	m := room3DModel(t, 200, 60)
	d := m.ds()
	d.octs, d.zoom = nil, designZoom100
	pg := d.newKind(kPage, 0, 0)
	pg.X, pg.Y, pg.Header = 4, 4, "Notes"
	cab := d.newKind(kCabinet, 0, 0)
	cab.X, cab.Y, cab.W, cab.Header = 60, 3, 40, "Cupboard"
	cab.Items = []*dOct{
		{ID: 801, Kind: kPage, Header: "Plan", Color: "#c678dd"},
		{ID: 802, Kind: kPage, Header: "Recipes", Color: "#61afef"},
		{ID: 803, Kind: kNote, Header: "Keys", Color: "#e5c07b"},
		{ID: 804, Kind: kBook, Header: "Atlas", Color: "#e06c75"},
	}
	d.sel = -1
	return m, cab
}

// cabinetDoors are the faces of the cabinet's two door slabs (their knobs
// left out): the faces on the doors' cells that are its doors' wood.
func cabinetDoors(m *Model, cab *dOct) []int {
	d := m.ds()
	r := &d.room
	x, _, w, _ := d.rect(cab)
	fill := mixHex(boardBG(), cab.Color, 0.18)
	wood := r.hex(mixHex(cab.Color, fill, 0.55))
	var out []int
	for fi, f := range r.faces {
		if f.own == cab.ID && f.mat == matSolid && f.col == wood &&
			(f.cx0 == x+1 || f.cx0 == x+w-1-cabDoorW) && f.cx1 == f.cx0+cabDoorW {
			out = append(out, fi)
		}
	}
	return out
}

// cabinetDep is how far the cabinet comes out of the wall in the last drawing.
func cabinetDep(r *room3D, cab *dOct) float64 {
	for _, b := range r.blocks {
		if b.o == cab {
			return b.dep
		}
	}
	return 0
}

// Shut, the cabinet's front is its flat drawing, and behind it the two
// doors are slabs that fill its opening flush with the front.
func TestDesign3DCabinetShut(t *testing.T) {
	m, cab := cabinetScene(t)
	d := m.ds()
	r := &d.room
	r.yaw, r.pitch = 35, 0
	boardRows(m)
	dep := cabinetDep(r, cab)
	x, y, w, h := d.rect(cab)
	hw, hh := r.rm.hw, r.rm.hh
	fronts := 0
	for _, f := range r.faces {
		if f.own == cab.ID && f.mat == matFront {
			fronts++
			if z := r.pts[f.p0].z; math.Abs(z-dep) > 0.01 || f.cx0 != x || f.cx1 != x+w || f.py0 != 2*y || f.py1 != 2*(y+h) {
				t.Fatalf("shut, the front should be the whole flat drawing at the front: cells %d…%d, rows %d…%d at z %.2f", f.cx0, f.cx1, f.py0, f.py1, z)
			}
		}
	}
	if fronts != 1 {
		t.Fatalf("shut, the cabinet should show one front, not %d", fronts)
	}
	doors := cabinetDoors(m, cab)
	if len(doors) != 10 { // two slabs, each with its outside left to the front
		t.Fatalf("shut: %d door faces, want 10", len(doors))
	}
	// Together the doors fill the opening, from the frame's left post to
	// its right one, within the front's depth.
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, fi := range doors {
		f := &r.faces[fi]
		if math.Abs(f.n.x)+math.Abs(f.n.y)+math.Abs(f.n.z) > 1.0001 {
			t.Fatalf("shut, a door face is turned: %v", f.n)
		}
		for _, p := range r.pts[f.p0 : f.p0+f.pn] {
			if p.z > dep+1e-3 || p.z < dep-1.01 {
				t.Fatalf("shut, a door sticks out of the front or into the shelves: z %.2f, front at %.2f", p.z, dep)
			}
			lo, hi = math.Min(lo, p.x+hw), math.Max(hi, p.x+hw)
			if row := (hh - p.y) / 2; row < float64(y+3)-1e-9 || row > float64(y+h-1)+1e-9 {
				t.Fatalf("shut, a door goes past the opening: row %.2f", row)
			}
		}
	}
	if lo != float64(x+1) || hi != float64(x+w-1) {
		t.Fatalf("shut, the doors should fill the opening, columns %d to %d: %.2f to %.2f", x+1, x+w-1, lo, hi)
	}
}

// Open, the doors are slabs turned out on their hinges, towards the
// viewer, about 105°; never through a side wall, the floor or out past the
// room's front, even when the cabinet is the outermost block. Halfway,
// they are halfway.
func TestDesign3DCabinetOpen(t *testing.T) {
	m, cab := cabinetScene(t)
	d := m.ds()
	r := &d.room
	cab.DrawerOpen = true
	for _, place := range [][2]int{{60, 3}, {2, 3}, {150, 30}} {
		cab.X, cab.Y = place[0], place[1]
		for _, a := range [][2]int{{35, 0}, {-35, 0}, {0, 30}} {
			r.yaw, r.pitch = a[0], a[1]
			what := fmt.Sprintf("at %v, turned %v", place, a)
			boardRows(m)
			dep := cabinetDep(r, cab)
			doors := cabinetDoors(m, cab)
			if len(doors) != 12 {
				t.Fatalf("%s: %d door faces, want two whole slabs", what, len(doors))
			}
			turned, out := 0, 0.0
			for _, fi := range doors {
				f := &r.faces[fi]
				if math.Abs(f.n.x) > 0.1 && math.Abs(f.n.z) > 0.1 {
					turned++
				}
				for _, p := range r.pts[f.p0 : f.p0+f.pn] {
					out = math.Max(out, p.z-dep)
					rm := &r.rm
					if p.x < rm.x0 || p.x > rm.x1 || p.y < rm.y0 || p.z >= rm.depth {
						t.Fatalf("%s: a door at %v goes through the room: x %.1f…%.1f, floor %.1f, front %.1f", what, p, rm.x0, rm.x1, rm.y0, rm.depth)
					}
				}
			}
			if turned < 8 {
				t.Fatalf("%s: open, the doors should be turned slabs: %d of their faces are turned", what, turned)
			}
			if out < 5 {
				t.Fatalf("%s: open, the doors should swing out towards the viewer: %.1f out of the front", what, out)
			}
		}
	}
	// With room each side, each door's outside faces 105° round: away from
	// the viewer and outwards.
	cab.X, cab.Y = 60, 3
	right := d.newKind(kNote, 0, 0)
	right.X, right.Y = 130, 4
	r.yaw, r.pitch = 20, 10
	boardRows(m)
	want := math.Cos(cabSwing * math.Pi / 180)
	found := 0
	for _, fi := range cabinetDoors(m, cab) {
		if n := r.faces[fi].n; math.Abs(n.z-want) < 1e-6 && math.Abs(math.Abs(n.x)-math.Sin(cabSwing*math.Pi/180)) < 1e-6 {
			found++
		}
	}
	if found != 2 {
		t.Fatalf("open, each door's outside should face %d° round: %d do", cabSwing, found)
	}
	// Halfway through opening, the doors are halfway open.
	designAnims = true
	defer func() { designAnims = false }()
	r.yaw, r.pitch = 0, 15
	d.anims = map[animKey]openAnim{{cab.ID, 0}: {time.Now().Add(-openAnimFor / 4), true}}
	boardRows(m)
	p := r.cab[cab.ID].p
	if p <= 0.2 || p >= 0.9 {
		t.Fatalf("a quarter of the way through, the doors should be part open: %.2f", p)
	}
	found = 0
	for _, fi := range cabinetDoors(m, cab) {
		if n := r.faces[fi].n; math.Abs(n.z-math.Cos(p*cabSwing*math.Pi/180)) < 1e-6 {
			found++
		}
	}
	if found != 2 {
		t.Fatalf("part open (%.2f), each door's outside should face %.0f° round: %d do", p, p*cabSwing, found)
	}
	// Meanwhile the flat drawing the faces show is open: what is on its
	// shelves shows between the doors.
	rows := room3DText(m)
	if !titleShows(rows, "Recipes") {
		t.Fatalf("while the doors swing, the shelves should show:\n%s", strings.Join(rows, "\n"))
	}
}

// A click in 3D on a door opens the cabinet, and on an open door shuts it;
// a click on a thing standing on a shelf is a click on its flat cell: it
// takes the thing out.
func TestDesign3DCabinetClicks(t *testing.T) {
	m, cab := cabinetScene(t)
	d := m.ds()
	r := &d.room
	r.yaw, r.pitch = 25, 15
	x, _, w, _ := d.rect(cab)

	// Shut: a click on the doors' front opens them.
	col, row := cell3D(t, m, cab.ID, func(sx, sy int) bool {
		return sx > x+3 && sx < x+w-3 && d.hit(sx, sy).act == "doors"
	})
	d.lastClick = time.Time{}
	click(m, col, row, tea.MouseLeft)
	if !cab.DrawerOpen {
		t.Fatal("a click in 3D on the shut doors should open them")
	}

	// Open: a click on a door, turned out on its hinge, shuts them.
	boardRows(m)
	doors := map[int32]bool{}
	for _, fi := range cabinetDoors(m, cab) {
		if n := r.faces[fi].n; math.Abs(n.x) > 0.1 && math.Abs(n.z) > 0.1 {
			doors[int32(fi)] = true
		}
	}
	col, row = -1, -1
	for y := 0; y < m.boardH() && col < 0; y++ {
		for c := 0; c < m.width; c++ {
			if h := r.pickAt(c, y); h.ok && doors[h.face] {
				col, row = c, y+m.boardTop()
				break
			}
		}
	}
	if col < 0 {
		t.Fatal("the 3D view shows no open door")
	}
	d.lastClick = time.Time{}
	click(m, col, row, tea.MouseLeft)
	if cab.DrawerOpen {
		t.Fatal("a click in 3D on an open door should shut the cabinet")
	}

	// Open again, a click on a thing on a shelf: the thing at that flat cell.
	cab.DrawerOpen = true
	boardRows(m)
	dep := cabinetDep(r, cab)
	want := ""
	col, row = cell3D(t, m, cab.ID, func(sx, sy int) bool {
		h := d.hit(sx, sy)
		if !strings.HasPrefix(h.act, "item:") {
			return false
		}
		want = h.act
		return true
	})
	// Only a thing's own faces are a click on it: seen past the things,
	// the cabinet's back, walls and shelves' tops are never a thing's cell
	// (nor a shelf's +).
	for _, a := range [][2]int{{25, 15}, {-30, 20}, {0, 35}} {
		r.yaw, r.pitch = a[0], a[1]
		boardRows(m)
		for y := 0; y < m.boardH(); y++ {
			for c := 0; c < m.width; c++ {
				h := r.pickAt(c, y)
				if !h.ok || h.own != cab.ID {
					continue
				}
				act := d.hit(h.sx, h.sy).act
				if !strings.HasPrefix(act, "item:") && !strings.HasPrefix(act, "new:") {
					continue
				}
				f := &r.faces[h.face]
				if f.mat == matFront {
					continue // the flat drawing itself: the cell right under the mouse
				}
				for x := f.cx0; x < f.cx1; x++ {
					if d.hit(x, f.py0>>1).act != act {
						t.Fatalf("turned %v: screen cell %d,%d shows a face of cells %d…%d (not only %s), but is a click on %s",
							a, c, y, f.cx0, f.cx1, act, act)
					}
				}
			}
		}
	}
	r.yaw, r.pitch = 25, 15
	boardRows(m)
	hit := r.pickAt(col, row-m.boardTop())
	if z := r.pts[r.faces[hit.face].p0].z; z >= dep-0.5 {
		t.Fatalf("a thing on a shelf should stand inside the cabinet: its front at %.2f, the cabinet's at %.2f", z, dep)
	}
	var i int
	fmt.Sscanf(want, "item:%d", &i)
	thing := cab.Items[i]
	d.lastClick = time.Time{}
	click(m, col, row, tea.MouseLeft)
	for _, it := range cab.Items {
		if it == thing {
			t.Fatalf("a click in 3D on %q on its shelf should take it out", thing.Header)
		}
	}
	if d.byID(thing.ID) == nil {
		t.Fatalf("%q taken out in 3D should be on the board", thing.Header)
	}
}
