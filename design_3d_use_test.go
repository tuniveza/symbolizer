package main

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// cell3D is a screen cell (with the board's top added, as a click wants
// it) where the 3D view shows the front of structure own at a flat board
// cell for which want is true.
func cell3D(t *testing.T, m *Model, own int, want func(sx, sy int) bool) (int, int) {
	t.Helper()
	boardRows(m)
	r := &m.ds().room
	for row := 0; row < m.boardH(); row++ {
		for col := 0; col < m.width; col++ {
			h := r.pickAt(col, row)
			if h.flat && h.own == own && r.faces[h.face].mat == matFront && want(h.sx, h.sy) {
				return col, row + m.boardTop()
			}
		}
	}
	t.Fatalf("the 3D view shows no such cell of structure %d", own)
	return 0, 0
}

// room3DCell is a screen cell (board's top added) showing a face of the
// room that is (made of) mat, with nothing of a structure on it.
func room3DCell(t *testing.T, m *Model, mat int) (int, int) {
	t.Helper()
	boardRows(m)
	d := m.ds()
	r := &d.room
	for row := m.boardH() - 1; row >= 0; row-- {
		for col := m.width - 1; col >= 0; col-- {
			h := r.pickAt(col, row)
			if h.ok && h.own < 0 && r.faces[h.face].mat == mat && (mat != matBoard || h.flat && d.hit(h.sx, h.sy).o == nil) {
				return col, row + m.boardTop()
			}
		}
	}
	t.Fatalf("the 3D view shows no empty face of the room made of %d", mat)
	return 0, 0
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
					}
				}
				if rm.y0 > -rm.hh-1 || rm.x0 > -rm.hw-1 || rm.x1 < rm.hw+1 {
					t.Fatalf("%s: the floor and walls should stand beyond the screen's board area: %+v", what, rm)
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

// The mouse turns the room (left drag on the empty room), moves the camera
// (middle or right drag) and brings it closer (the wheel); a click on the
// empty wall is a click on the empty board.
func TestDesign3DMouseCamera(t *testing.T) {
	m := room3DModel(t, 120, 40)
	d := m.ds()
	r := &d.room
	r.yaw, r.pitch = 20, 10
	x, y := room3DCell(t, m, matFloor)
	dragTo(m, x, y, x+6, y, tea.MouseLeft)
	if r.yaw != 20+int(math.Round(6*turnPerCol)) || r.pitch != 10 {
		t.Fatalf("a drag of 6 cells right on the floor: yaw %d pitch %d", r.yaw, r.pitch)
	}
	x, y = room3DCell(t, m, matFloor)
	dragTo(m, x, y, x, y-2, tea.MouseLeft)
	if r.pitch != 10-int(2*turnPerRow) {
		t.Fatalf("a drag of 2 rows up: pitch %d", r.pitch)
	}
	if r.camDrag != camNone {
		t.Fatal("let go: no camera drag")
	}
	for _, b := range []tea.MouseButton{tea.MouseMiddle, tea.MouseRight} {
		r.pan = r3vec{}
		boardRows(m)
		dragTo(m, 50, 20, 56, 22, b)
		if r.pan.len() < 1 {
			t.Fatalf("a drag with button %v should move the camera: %v", b, r.pan)
		}
	}
	near := r.near
	m.Update(tea.MouseWheelMsg(tea.Mouse{X: 50, Y: 20, Button: tea.MouseWheelUp}))
	if r.near != near+1 {
		t.Fatal("the wheel up: the camera closer")
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
	press(m, "ctrl+right")
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
	camX := d.camX
	press(m, "shift+right")
	if d.camX == camX {
		t.Fatal("Shift + arrows in 3D: the board moves")
	}
	near := d.room.near
	typeKeys(m, "+")
	if d.room.near != near+1 || !d.room.on() {
		t.Fatal("+ in 3D: the camera closer")
	}
	press(m, "esc")
	if d.room.on() {
		t.Fatal("Esc in 3D: the flat board")
	}
}

// Free roam: Ctrl+F, W A S D Q E walk (Shift faster), Ctrl + arrows and a
// drag look round, through blocks too, and Esc goes back.
func TestDesign3DFreeRoam(t *testing.T) {
	m := room3DModel(t, 120, 40)
	d := m.ds()
	r := &d.room
	press(m, "ctrl+f")
	if !r.free || !r.on() {
		t.Fatal("Ctrl+F: free roam")
	}
	checkRows(t, m, "free roam")
	if !strings.Contains(m.designStatus(), "Free roam") {
		t.Fatalf("the status bar should say it is free roam: %q", m.designStatus())
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
	y0 := r.pos.y
	press(m, "e")
	if r.pos.y <= y0 {
		t.Fatal("E: up")
	}
	press(m, "ctrl+right")
	if r.look[0] != turnStep {
		t.Fatalf("Ctrl + Right in free roam: look right %v", r.look)
	}
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
		t.Fatal("Esc in free roam: back, and straight on that is the flat board")
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
	tx, ty := room3DCell(t, m, matBoard)
	d.lastClick = time.Time{}
	dragTo(m, x, y, tx, ty, tea.MouseLeft)
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

// A structure dragged in 3D moves on the board under the mouse.
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
	dragTo(m, x, y, x+10, y+4, tea.MouseLeft)
	if o.X <= x0 || o.Y <= y0 {
		t.Fatalf("dragged right and down in 3D: at %d,%d (was %d,%d)", o.X, o.Y, x0, y0)
	}
}
