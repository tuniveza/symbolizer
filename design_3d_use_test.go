package main

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// In 3D a click acts where the cross is (the middle of the view), so the
// helpers below turn the camera until the cross is on what is wanted, and
// give the cross's screen cell (with the board's top added, as a click
// wants it): the mouse is there too.

// aim3D finds a board area cell for which ok is true, turns the camera so
// the cross is on the point it shows, and checks the cross is on such a
// thing in the room drawn again. It gives the cross's screen cell.
func aim3D(t *testing.T, m *Model, what string, ok func(h r3hit) bool) (int, int) {
	t.Helper()
	r := &m.ds().room
	lens := r.lens
	defer func() { r.lens = lens }()
	for try := 0; try < 8; try++ {
		boardRows(m)
		var found *r3hit
		if h := r.pickAim(); h.ok && ok(h) {
			found = &h
		}
		// A cell well inside what is wanted (its neighbours are too), so
		// the cross is not put on its very edge; else any.
		inside := func(col, row int) bool {
			for _, c := range [4][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
				if h := r.pickAt(col+c[0], row+c[1]); !h.ok || !ok(h) {
					return false
				}
			}
			return true
		}
		for pass := 0; pass < 2 && found == nil; pass++ {
			for row := m.boardH() - 1; found == nil && row >= 0; row-- {
				for col := m.width - 1; col >= 0; col-- {
					if h := r.pickAt(col, row); h.ok && ok(h) && (pass == 1 || inside(col, row)) {
						found = &h
						break
					}
				}
			}
		}
		if found == nil {
			if r.lens > 1 {
				r.lens = 1 // zoomed in: look for it with the normal lens, then aim zoomed in
				continue
			}
			if !r.free || r.pos == r.startPos && r.look == r.startLook {
				break
			}
			r.pos, r.look = r.startPos, r.startLook // out of sight: look from where 3D started
			continue
		}
		if !r.free {
			m.design3DFree()
		}
		c := found.p.sub(r.pos).norm()
		r.look = [2]float64{math.Atan2(c.x, -c.z) * 180 / math.Pi, math.Asin(clampF(-c.y, -1, 1)) * 180 / math.Pi}
		r.lens = lens
		boardRows(m)
		if h := r.pickAim(); h.ok && ok(h) {
			col, row := r.aim()
			return col, row + m.boardTop()
		}
	}
	t.Fatalf("the cross cannot be put on %s in the 3D view", what)
	return 0, 0
}

// cell3D aims the cross at the front of structure own, at a flat board
// cell for which want is true.
func cell3D(t *testing.T, m *Model, own int, want func(sx, sy int) bool) (int, int) {
	t.Helper()
	r := &m.ds().room
	return aim3D(t, m, fmt.Sprintf("a cell of structure %d", own), func(h r3hit) bool {
		return h.flat && h.own == own && r.faces[h.face].mat == matFront && want(h.sx, h.sy)
	})
}

// room3DCell aims the cross at a face of the room that is (made of) mat,
// with nothing of a structure on it.
func room3DCell(t *testing.T, m *Model, mat int) (int, int) {
	t.Helper()
	d := m.ds()
	r := &d.room
	return aim3D(t, m, fmt.Sprintf("an empty face of the room made of %d", mat), func(h r3hit) bool {
		return h.own < 0 && r.faces[h.face].mat == mat && (mat != matBoard || h.flat && d.hit(h.sx, h.sy).o == nil)
	})
}

// roomSurface aims the cross at some empty part of the room (a wall, the
// floor, the ceiling or the empty board).
func roomSurface(t *testing.T, m *Model) (int, int) {
	t.Helper()
	d := m.ds()
	r := &d.room
	return aim3D(t, m, "the empty room", func(h r3hit) bool {
		return h.own < 0 && (r.faces[h.face].mat != matBoard || h.flat && d.hit(h.sx, h.sy).o == nil)
	})
}

// actAt is whether a click on flat board cell (sx, sy) does an action
// starting with act (pull:1, boxlid, item:…).
func actAt(d *designState, act string) func(sx, sy int) bool {
	return func(sx, sy int) bool {
		h := d.hit(sx, sy)
		return h.where == hitRow && strings.HasPrefix(h.act, act)
	}
}

// The floor and the walls stand beyond every block, at every zoom and
// turn and with the board moved: no block goes through them, and a block
// that sticks out of the screen is in the room whole.
func TestDesign3DRoomFits(t *testing.T) {
	m := room3DModel(t, 120, 30)
	d := m.ds()
	r := &d.room
	w, h := m.width, m.boardH()
	cut := 0 // blocks cut by the screen's edge in the flat view
	for _, zoom := range []int{0, 2, designZoom100, 8} {
		for _, pan := range [][2]float64{{0, 0}, {-30, -12}, {25, 18}, {60, -20}, {-5, 30}} {
			d.zoom, d.camX, d.camY = zoom, pan[0], pan[1]
			for _, a := range [][2]int{{35, 25}, {-60, 40}, {80, 75}, {20, -45}} {
				r.yaw, r.pitch = a[0], a[1]
				checkRows(t, m, "fitted room")
				rm := r.rm
				what := fmt.Sprintf("zoom %d, board at %v, turned %v", zoom, pan, a)
				low, left, right := math.Inf(1), math.Inf(1), math.Inf(-1) // the blocks' lowest point and outermost sides
				for fi := range r.faces {
					f := &r.faces[fi]
					if f.own < 0 {
						continue
					}
					for _, p := range r.pts[f.p0 : f.p0+f.pn] {
						if p.x < rm.x0 || p.x > rm.x1 || p.y < rm.y0 || p.y > rm.y1 || p.z < 0 || p.z >= rm.depth {
							t.Fatalf("%s: a corner of structure %d at %v is outside the room x %.1f…%.1f, y %.1f…%.1f, z 0…%.1f",
								what, f.own, p, rm.x0, rm.x1, rm.y0, rm.y1, rm.depth)
						}
						low, left, right = math.Min(low, p.y), math.Min(left, p.x), math.Max(right, p.x)
					}
				}
				if len(r.blocks) > 0 && (low-rm.y0 < 2*roomMarginY || left-rm.x0 < roomMarginX || rm.x1-right < roomMarginX) {
					t.Fatalf("%s: the floor should be under the lowest block, the walls beyond the outermost, with room to spare: %+v, blocks %.1f to %.1f, lowest %.1f",
						what, rm, left, right, low)
				}
				for _, b := range r.blocks {
					x, y, ow, oh := d.rect(b.o)
					if x < 0 || y < 0 || x+ow > w || y+oh > h {
						cut++
					}
					if x >= -w && y >= -h && x+ow <= 2*w && y+oh <= 2*h && !b.whole {
						t.Fatalf("%s: structure %d sticks out of the screen but should be in the room whole", what, b.o.ID)
					}
				}
			}
		}
	}
	if cut == 0 {
		t.Fatal("no structure stuck out of the screen: the test checks nothing")
	}
}

// The turned room fills the whole board area: at every turn, tilt, zoom
// and camera distance, with the board moved and with nothing on it, every
// pixel shows a surface of the room (no pixel is left the colour of
// beyond it), and the far turns show the side walls, the floor and the
// ceiling.
func TestDesign3DRoomFillsScreen(t *testing.T) {
	m := room3DModel(t, 120, 30)
	d := m.ds()
	r := &d.room
	seen := map[int]bool{} // the materials seen
	check := func(what string) {
		t.Helper()
		boardRows(m)
		for i, f := range r.face {
			if f < 0 {
				t.Fatalf("%s: pixel %d, %d shows nothing (beyond the room)", what, i%r.pw, i/r.pw)
			}
			seen[r.faces[f].mat] = true
		}
	}
	for _, empty := range []bool{false, true} {
		if empty {
			d.octs = nil
		}
		for _, zoom := range []int{0, 2, designZoom100, 8} {
			for _, pan := range [][2]float64{{0, 0}, {-30, -12}, {60, -20}} {
				d.zoom, d.camX, d.camY = zoom, pan[0], pan[1]
				for _, a := range [][2]int{{5, 0}, {-25, 10}, {25, -10}, {80, 0}, {-80, 75}, {80, -45}, {0, 75}, {-40, -45}} {
					for _, near := range []int{-6, 0, 10} {
						r.yaw, r.pitch, r.near = a[0], a[1], near
						check(fmt.Sprintf("zoom %d, board at %v, turned %v, %d steps closer, empty %v", zoom, pan, a, near, empty))
					}
				}
			}
		}
	}
	r.near = 0
	for _, mat := range []int{matBoard, matBack, matFloor, matWall, matCeiling} {
		if !seen[mat] {
			t.Errorf("the room's surface %d never showed", mat)
		}
	}
	// Free roam starts inside the room: all room too.
	r.yaw, r.pitch = 30, 20
	press(m, "ctrl+f")
	check("free roam, at the start")
}

// Moving the mouse with no button looks round on its own: the first move
// only notes where it is, then each cell turns the camera as a drag does,
// from where it is (free roam), and a jump (out of the window and back)
// does not turn it.
func TestDesign3DMouseLookNoButton(t *testing.T) {
	m := room3DModel(t, 120, 40)
	d := m.ds()
	r := &d.room
	r.yaw, r.pitch = 20, 10
	boardRows(m)
	cam := r.view.cam
	x, y := 60, 20 // anywhere: with no button the mouse only looks round
	move := func(x, y int) { m.Update(tea.MouseMotionMsg(tea.Mouse{X: x, Y: y})) }
	move(x, y)
	if r.free {
		t.Fatal("the first move only notes where the mouse is")
	}
	move(x+4, y)
	if !r.free {
		t.Fatal("moving the mouse in 3D: mouse look (free roam from where the camera is)")
	}
	start := r.startLook
	if math.Abs(r.look[0]-start[0]-4*turnPerCol) > 1e-9 || r.look[1] != start[1] {
		t.Fatalf("4 cells right: look %v, from %v", r.look, start)
	}
	move(x+4, y-2)
	if math.Abs(r.look[1]-(start[1]-2*turnPerRow)) > 1e-9 {
		t.Fatalf("and 2 rows up: look %v, from %v", r.look, start)
	}
	if r.pos.sub(cam).len() > 1e-9 {
		t.Fatalf("mouse look moved the camera: %v, was at %v", r.pos, cam)
	}
	look := r.look
	move(x+4+hoverJump+1, y-2)
	if r.look != look {
		t.Fatalf("a jump turned the camera: %v, was %v", r.look, look)
	}
	press(m, "esc")
	if r.on() {
		t.Fatal("Esc: the flat board")
	}
	move(x, y)
	move(x+3, y)
	if r.free || r.on() {
		t.Fatal("on the flat board the mouse does not look round")
	}
}

// The mouse looks round (left drag on the empty room: the camera stays
// where it is and turns); the middle and right buttons and the wheel do
// nothing to the camera; a click on the empty wall is a click on the
// empty board.
func TestDesign3DMouseCamera(t *testing.T) {
	m := room3DModel(t, 120, 40)
	d := m.ds()
	r := &d.room
	r.yaw, r.pitch = 20, 10
	boardRows(m)
	cam := r.view.cam
	x, y := roomSurface(t, m)
	m.Update(tea.MouseClickMsg(tea.Mouse{X: x, Y: y, Button: tea.MouseLeft}))
	m.Update(tea.MouseMotionMsg(tea.Mouse{X: x + 1, Y: y, Button: tea.MouseLeft}))
	if !r.free {
		t.Fatal("a left drag on the empty room: mouse look (free roam from where the camera is)")
	}
	start := r.look
	m.Update(tea.MouseMotionMsg(tea.Mouse{X: x + 7, Y: y, Button: tea.MouseLeft}))
	look := r.look
	if math.Abs(look[0]-start[0]-6*turnPerCol) > 1e-9 || look[1] != start[1] {
		t.Fatalf("a drag of 6 cells right: look %v, from %v", look, start)
	}
	m.Update(tea.MouseMotionMsg(tea.Mouse{X: x + 7, Y: y - 2, Button: tea.MouseLeft}))
	m.Update(tea.MouseReleaseMsg(tea.Mouse{X: x + 7, Y: y - 2, Button: tea.MouseLeft}))
	if math.Abs(r.look[1]-(start[1]-2*turnPerRow)) > 1e-9 {
		t.Fatalf("and 2 rows up: look %v, from %v", r.look, start)
	}
	if r.pos.sub(cam).len() > 1e-9 {
		t.Fatalf("mouse look moved the camera: %v, was at %v", r.pos, cam)
	}
	if r.yaw != 20 || r.pitch != 10 {
		t.Fatalf("mouse look turned the room: yaw %d pitch %d", r.yaw, r.pitch)
	}
	if r.camDrag != camNone {
		t.Fatal("let go: no camera drag")
	}
	for _, b := range []tea.MouseButton{tea.MouseMiddle, tea.MouseRight} {
		x, y := roomSurface(t, m)
		pos, look := r.pos, r.look
		m.Update(tea.MouseClickMsg(tea.Mouse{X: x, Y: y, Button: b}))
		m.Update(tea.MouseMotionMsg(tea.Mouse{X: x + 6, Y: y + 2, Button: b}))
		m.Update(tea.MouseReleaseMsg(tea.Mouse{X: x + 6, Y: y + 2, Button: b}))
		if r.pos != pos || r.look != look {
			t.Fatalf("a drag with button %v moved the camera: %v %v", b, r.pos, r.look)
		}
	}
	pos, zoom := r.pos, d.zoom
	m.Update(tea.MouseWheelMsg(tea.Mouse{X: 50, Y: 20, Button: tea.MouseWheelUp}))
	if r.pos != pos || d.zoom != zoom {
		t.Fatal("the wheel: no zoom and no camera move in 3D")
	}
	// A click on the empty wall lets go of the chosen structure; a double
	// click there places a new one, as on the flat board.
	d.sel = d.octs[0].ID
	x, y = room3DCell(t, m, matBoard)
	click(m, x, y, tea.MouseLeft)
	if d.sel != -1 {
		t.Fatalf("a click on the empty wall should let go of the chosen structure: %d", d.sel)
	}
	n := len(d.octs)
	d.lastClick = time.Time{}
	click(m, x, y, tea.MouseLeft)
	click(m, x, y, tea.MouseLeft)
	if len(d.octs) != n+1 || d.editing != partHeader {
		t.Fatalf("a double click on the empty wall should place a structure: %d (was %d), writing %d", len(d.octs), n, d.editing)
	}
}

// Keys other than the 3D view's own do what they do on the flat board.
func TestDesign3DKeysPass(t *testing.T) {
	m := room3DModel(t, 120, 40)
	d := m.ds()
	o := d.octs[0]
	d.sel = o.ID
	press(m, "ctrl+f")
	d.sel = o.ID // as a click would
	press(m, "tab")
	if d.editing == partNone {
		t.Fatal("Tab in 3D: writing in the chosen structure, as on the flat board")
	}
	press(m, "esc")
	if d.editing != partNone || !d.room.on() {
		t.Fatal("Esc while writing in 3D: writing stops, the 3D view stays")
	}
	press(m, "x")
	press(m, "x")
	if d.byID(o.ID) != nil {
		t.Fatal("X X in 3D: the chosen structure deleted")
	}
	camX, camY, yaw := d.camX, d.camY, d.room.look[0]
	press(m, "shift+right")
	press(m, "right")
	press(m, "left")
	if d.camX != camX || d.camY != camY || d.room.look[0] != yaw+turnStep {
		t.Fatalf("arrows and Shift + arrows in 3D: the camera looks round (%v°, want %v), the board stays still (moved %v, %v)",
			d.room.look[0], yaw+turnStep, d.camX-camX, d.camY-camY)
	}
	// While writing, an arrow stops writing and looks round.
	d.sel = d.octs[0].ID
	press(m, "tab")
	pitch := d.room.look[1]
	press(m, "up")
	if d.editing != partNone || d.room.look[1] != pitch-turnStep {
		t.Fatalf("an arrow while writing in 3D: writing stops and the camera looks up: editing %v, look %v from %v", d.editing, d.room.look[1], pitch)
	}
	pos := d.room.pos
	typeKeys(m, "+")
	if d.room.pos != pos || !d.room.on() {
		t.Fatal("+ in 3D: the camera stays")
	}
	press(m, "esc")
	if d.room.on() {
		t.Fatal("Esc in 3D: the flat board")
	}
}

// First person: Ctrl+F, W A S D move (Shift faster), the arrows and a drag
// look round, through blocks too, and Esc goes back.
func TestDesign3DFreeRoam(t *testing.T) {
	m := room3DModel(t, 120, 40)
	d := m.ds()
	r := &d.room
	press(m, "ctrl+f")
	if !r.free || !r.on() {
		t.Fatal("Ctrl+F: free roam")
	}
	checkRows(t, m, "free roam")
	if !strings.Contains(m.designStatus(), "3D view") {
		t.Fatalf("the status bar should say it is 3D: %q", m.designStatus())
	}
	z0 := r.pos.z
	press(m, "w")
	step := z0 - r.pos.z
	if step <= 0 {
		t.Fatalf("W: the camera walks toward the wall: z %.1f → %.1f", z0, r.pos.z)
	}
	z0 = r.pos.z
	press(m, "shift+w")
	if fast := z0 - r.pos.z; fast < 2*step {
		t.Fatalf("Shift + W: faster, %.2f against %.2f", fast, step)
	}
	press(m, "right")
	if r.look[0] != turnStep {
		t.Fatalf("Right in 3D: look right %v", r.look)
	}
	r.look[1] = 30 // looking down: W goes down too
	y0 := r.pos.y
	press(m, "w")
	if r.pos.y >= y0 {
		t.Fatalf("W looking down: the camera should go down: y %.1f → %.1f", y0, r.pos.y)
	}
	r.look[1] = 0
	look := r.look
	x, y := room3DCell(t, m, matBoard)
	dragTo(m, x, y, x-6, y, tea.MouseLeft)
	if r.look[0] == look[0] {
		t.Fatalf("a drag on the empty room in free roam: look round %v", r.look)
	}
	// Held (kitty says when it is let go): it walks on, and stops when let go.
	m.releases = true
	z0 = r.pos.z
	press(m, "s")
	r.stepAt = time.Now().Add(-50 * time.Millisecond)
	m.Update(room3DWalkMsg{})
	if !r.walking || r.pos.z <= z0 {
		t.Fatalf("S held: walks back, z %.1f → %.1f", z0, r.pos.z)
	}
	m.Update(tea.KeyReleaseMsg(tea.Key{Code: 's', Text: "s"}))
	m.Update(room3DWalkMsg{})
	if r.walking {
		t.Fatal("S let go: it stops")
	}
	m.releases = false
	// Into a block and through it, looking every way: whole lines.
	o := d.octs[3] // the cabinet: deep
	var ow, oh int
	x, y, ow, oh = d.rect(o)
	rm := r.rm
	for _, z := range []float64{0.2, 2, 5} {
		for yaw := -180.0; yaw < 180; yaw += 45 {
			for _, pitch := range []float64{-80, 0, 60} {
				r.pos = r3vec{float64(x+ow/2) - rm.hw, rm.hh - float64(2*y+oh), z}
				r.look = [2]float64{yaw, pitch}
				checkRows(t, m, fmt.Sprintf("inside a block at depth %.1f, looking %.0f, %.0f", z, yaw, pitch))
			}
		}
	}
	// Writing, the keys type: W is a letter, Esc stops writing first.
	pg := d.octs[1]
	d.sel, d.editing = pg.ID, partHeader
	pos := r.pos
	typeKeys(m, "w")
	if r.pos != pos || !strings.HasSuffix(pg.Header, "w") {
		t.Fatalf("writing in free roam, W types: header %q", pg.Header)
	}
	press(m, "esc")
	if d.editing != partNone || !r.free {
		t.Fatal("Esc while writing: writing stops, free roam stays")
	}
	press(m, "esc")
	if r.free || r.on() {
		t.Fatal("Esc in 3D: the flat board")
	}
	checkRows(t, m, "flat again")
}

// Clicks in 3D do what they do on the flat board: a drawer pulls out, a
// box's lid opens, a thing is dragged out of a drawer, a page's title is
// written in.
func TestDesign3DClicks(t *testing.T) {
	m, d, b := boxTest(t)
	b.X, b.Y, b.DrawerOpen = 1, 1, false
	dr := d.newKind(kDrawer, 0, 0)
	dr.X, dr.Y, dr.Header, dr.Drawers = 44, 1, "Tools", 3
	dr.Items = []*dOct{{ID: 401, Kind: kNote, Header: "Saw", Color: "#98c379"}}
	pg := d.newKind(kPage, 0, 0)
	pg.X, pg.Y, pg.Header = 76, 2, "Plan"
	d.sel, d.editing = -1, partNone
	r := &d.room
	r.yaw, r.pitch = 15, 10

	// The lid.
	x, y := cell3D(t, m, b.ID, actAt(d, "boxlid"))
	d.lastClick = time.Time{}
	click(m, x, y, tea.MouseLeft)
	if !b.DrawerOpen {
		t.Fatal("a click in 3D on the box's [open] should open it")
	}
	// The second drawer.
	x, y = cell3D(t, m, dr.ID, actAt(d, "pull:1"))
	d.lastClick = time.Time{}
	click(m, x, y, tea.MouseLeft)
	if !slotOpen(dr, 1) || slotOpen(dr, 0) {
		t.Fatalf("a click in 3D on the second drawer's front should pull it out: %v", dr.Pulled)
	}
	// The first drawer, and Saw dragged out of it onto the wall.
	x, y = cell3D(t, m, dr.ID, actAt(d, "pull:0"))
	d.lastClick = time.Time{}
	click(m, x, y, tea.MouseLeft)
	if !slotOpen(dr, 0) {
		t.Fatalf("a click in 3D on the first drawer's front should pull it out: %v", dr.Pulled)
	}
	x, y = cell3D(t, m, dr.ID, actAt(d, "item:"))
	d.lastClick = time.Time{}
	// Pressed with the cross on Saw, the mouse turns the camera down until
	// the cross is on the wall below the unit, and is let go there.
	m.Update(tea.MouseClickMsg(tea.Mouse{X: x, Y: y, Button: tea.MouseLeft}))
	_, dy, _, dh := d.rect(dr)
	for i := 1; i <= 40; i++ {
		y++
		m.Update(tea.MouseMotionMsg(tea.Mouse{X: x, Y: y, Button: tea.MouseLeft}))
		if _, sy, ok := m.design3DMiddle(); ok && sy > dy+dh+2 {
			break
		}
	}
	m.Update(tea.MouseReleaseMsg(tea.Mouse{X: x, Y: y, Button: tea.MouseLeft}))
	if d.byID(401) == nil || len(dr.Items) != 0 {
		t.Fatalf("Saw dragged in 3D out of the drawer onto the wall: on the board %v, the unit holds %d", d.byID(401) != nil, len(dr.Items))
	}
	// The page's title: a double click on it, then typing.
	x, y = cell3D(t, m, pg.ID, func(sx, sy int) bool {
		px, py, _, ph := d.rect(pg)
		return d.hit(sx, sy).o == pg && sx > px+1 && partAt(pg, sy-py, ph) == partHeader
	})
	d.lastClick = time.Time{}
	click(m, x, y, tea.MouseLeft)
	click(m, x, y, tea.MouseLeft)
	if d.sel != pg.ID || d.editing != partHeader {
		t.Fatalf("a double click in 3D on the page's title should write it: chosen %d, writing %d", d.sel, d.editing)
	}
	typeKeys(m, " two")
	if !strings.Contains(pg.Header, " two") {
		t.Fatalf("typing in 3D should write the title: %q", pg.Header)
	}
	checkRows(t, m, "after the clicks")
}

// A structure dragged in 3D moves on the board under the cross, as the
// mouse turns the camera.
func TestDesign3DDrag(t *testing.T) {
	m := room3DModel(t, 120, 40)
	d := m.ds()
	r := &d.room
	r.yaw, r.pitch = 20, 15
	o := d.octs[1] // a page
	x0, y0 := o.X, o.Y
	x, y := cell3D(t, m, o.ID, func(sx, sy int) bool {
		_, py, _, _ := d.rect(o)
		return d.hit(sx, sy).o == o && sy > py+2
	})
	dragTo(m, x, y, x+3, y+1, tea.MouseLeft) // the camera turns right and down: the cross, and the page, go that way
	if o.X <= x0 || o.Y <= y0 {
		t.Fatalf("dragged right and down in 3D: at %d,%d (was %d,%d)", o.X, o.Y, x0, y0)
	}
}
