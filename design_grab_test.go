package main

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// Alt+Space is named "alt+space" (the grab key).
func TestGrabKeyName(t *testing.T) {
	if n := keyName(tea.Key{Code: tea.KeySpace, Mod: tea.ModAlt}); !isGrabKey(n) {
		t.Fatalf("Alt+Space is named %q", n)
	}
	if !isGrabKey("alt+ ") {
		t.Fatal(`"alt+ " should count as Alt+Space too`)
	}
}

// In 3D, Alt+Space takes up the structure under the cross; it follows the
// cross as the camera turns, Alt+Space puts it down, and the carry is one
// undo step.
func TestDesignGrab3D(t *testing.T) {
	m := room3DModel(t, 160, 50)
	d := m.ds()
	press(m, "ctrl+f")
	o := d.octs[1] // a page
	x0, y0 := o.X, o.Y
	cell3D(t, m, o.ID, func(sx, sy int) bool { return d.hit(sx, sy).o == o })
	steps := len(d.undo)
	press(m, "alt+space")
	if !d.grab.active || d.grab.id != o.ID || d.drag != dragMove {
		t.Fatalf("Alt+Space with the cross on the page should take it up: active %v id %d drag %d", d.grab.active, d.grab.id, d.drag)
	}
	if m.designStatus() == "" || m.grabStatus() != grabHint3D {
		t.Fatalf("the status bar should say it is carried: %q", m.grabStatus())
	}
	for range 4 {
		press(m, "right")
		press(m, "down")
	}
	if o.X <= x0 || o.Y <= y0 {
		t.Fatalf("the camera turned right and down: the page should follow, at %d,%d (was %d,%d)", o.X, o.Y, x0, y0)
	}
	if d.sel != o.ID {
		t.Fatalf("what is carried stays chosen: %d", d.sel)
	}
	press(m, "x") // other keys do nothing mid-carry
	press(m, "x")
	if d.byID(o.ID) == nil {
		t.Fatal("X X mid-carry should not delete it")
	}
	press(m, "alt+space")
	if d.grab.active || d.drag != dragNone {
		t.Fatal("Alt+Space again should put it down")
	}
	x1, y1 := o.X, o.Y
	if x1 == x0 && y1 == y0 {
		t.Fatal("put down where it was carried to")
	}
	if len(d.undo) != steps+1 {
		t.Fatalf("the carry should be one undo step: %d steps, was %d", len(d.undo), steps)
	}
	press(m, "ctrl+z")
	if o := d.byID(o.ID); o == nil || o.X != x0 || o.Y != y0 {
		t.Fatal("Ctrl+Z should put it back where it was taken up")
	}
}

// Esc (or Alt tapped twice) puts what is carried back, with no undo step.
func TestDesignGrab3DCancel(t *testing.T) {
	m := room3DModel(t, 160, 50)
	d := m.ds()
	press(m, "ctrl+f")
	o := d.octs[1]
	x0, y0, l0 := o.X, o.Y, o.Layer
	cell3D(t, m, o.ID, func(sx, sy int) bool { return d.hit(sx, sy).o == o })
	steps := len(d.undo)
	press(m, "alt+space")
	for range 4 {
		press(m, "right")
	}
	if o.X == x0 {
		t.Fatal("it should follow the cross")
	}
	press(m, "esc")
	if d.grab.active || o.X != x0 || o.Y != y0 || o.Layer != l0 {
		t.Fatalf("Esc should put it back: at %d,%d layer %d (was %d,%d layer %d)", o.X, o.Y, o.Layer, x0, y0, l0)
	}
	if !d.room.on() {
		t.Fatal("Esc mid-carry should only put it back, not leave 3D")
	}
	if len(d.undo) != steps {
		t.Fatalf("a carry put back is no step: %d, was %d", len(d.undo), steps)
	}
	// Alt tapped twice does the same.
	press(m, "alt+space")
	press(m, "right")
	m.designAltTap()
	m.designAltTap()
	if d.grab.active || o.X != x0 || !d.room.on() {
		t.Fatalf("Alt twice should put it back and stay in 3D: active %v at %d (was %d)", d.grab.active, o.X, x0)
	}
}

// With the cross on nothing, Alt+Space takes up the structure nearest the
// middle of the screen, and brings it under the cross.
func TestDesignGrab3DNearest(t *testing.T) {
	m := room3DModel(t, 160, 50)
	d := m.ds()
	press(m, "ctrl+f")
	roomSurface(t, m)
	press(m, "alt+space")
	if !d.grab.active {
		t.Fatalf("with the cross on the empty room, the nearest structure should be taken up: %q", m.msg)
	}
	boardRows(m)
	if own, _, ok := d.room.middleHit(); !ok || own != d.grab.id {
		t.Fatalf("the structure taken up should be under the cross: the cross is on %d, carried %d", own, d.grab.id)
	}
	press(m, "esc")
}

// In 3D, Alt+Space with the cross on a thing in a container shows it big,
// facing the camera; Alt+Space again puts it back, the camera too.
func TestDesignMaximize3D(t *testing.T) {
	m := room3DModel(t, 160, 50)
	d := m.ds()
	rk := d.octs[5]
	rk.Items = []*dOct{{ID: 950, Kind: kPage, Header: "Blues", W: 26, H: 9}, {ID: 951, Kind: kPage, Header: "Jazz", W: 26, H: 9}}
	rk.H = rackLines(len(rk.Items))
	press(m, "ctrl+f")
	cell3D(t, m, rk.ID, actAt(d, "open:"))
	r := &d.room
	pos, look := r.pos, r.look
	h := r.pickAim()
	want := actItem(d.hit(h.sx, h.sy).act)
	it := rk.Items[want]
	press(m, "alt+space")
	if d.grab.active {
		t.Fatal("a thing in a container is shown big, not carried")
	}
	if !d.opened.active || !d.opened.max || d.opened.id != it.ID || d.byID(it.ID) == nil {
		t.Fatalf("Alt+Space on a rack's row should show its thing big: %+v", d.opened)
	}
	if it.W <= 26 || it.Layer <= rk.Layer || len(rk.Items) != 1 {
		t.Fatalf("shown big: %d wide, layer %d (rack %d), rack holds %d", it.W, it.Layer, rk.Layer, len(rk.Items))
	}
	if r.look != [2]float64{0, 0} || r.pos == pos {
		t.Fatalf("the camera should stand square in front of it: look %v", r.look)
	}
	if m.grabStatus() != maxHint {
		t.Fatalf("status: %q", m.grabStatus())
	}
	boardRows(m)
	if own, _, ok := r.middleHit(); !ok || own != it.ID {
		t.Fatalf("it should be in the middle of the view: %d", own)
	}
	// Saved while big, it is saved in its rack.
	_, f := d.snapshot()
	for _, o := range f.Octs {
		if o.ID == it.ID {
			t.Fatal("saved while shown big: it should be saved in its rack")
		}
	}
	press(m, "alt+space")
	if d.opened.active || d.byID(it.ID) != nil || len(rk.Items) != 2 || rk.Items[want] != it {
		t.Fatalf("Alt+Space again should put it back in its place: open %v, rack holds %d", d.opened.active, len(rk.Items))
	}
	if it.W != 26 || it.H != 9 || it.Layer != 0 {
		t.Fatalf("back with its own size: %dx%d layer %d", it.W, it.H, it.Layer)
	}
	if r.pos != pos || r.look != look {
		t.Fatal("the camera should go back to where it was")
	}
	// Esc puts it back too.
	cell3D(t, m, rk.ID, actAt(d, "open:"))
	press(m, "alt+space")
	if !d.opened.max {
		t.Fatal("shown big again")
	}
	press(m, "esc")
	if d.opened.active || len(rk.Items) != 2 || !d.room.on() {
		t.Fatalf("Esc should put it back and stay in 3D: open %v, %d in the rack", d.opened.active, len(rk.Items))
	}
}

// On the flat board a box's chosen thing or a board's chosen pin is shown
// big in the middle of the screen; Esc puts it back where it was.
func TestDesignMaximize2D(t *testing.T) {
	m := room3DModel(t, 160, 50)
	d := m.ds()
	bd := d.octs[6]
	bd.Items = []*dOct{{ID: 960, Kind: kPage, Header: "Milk", X: 3, Y: 2, W: 26, H: 9}}
	d.sel = bd.ID
	d.pin = pinSel{board: bd.ID, at: 0, active: true}
	press(m, "alt+space")
	it := d.byID(960)
	if it == nil || !d.opened.max || len(bd.Items) != 0 {
		t.Fatal("Alt+Space on a chosen pin should show it big")
	}
	x, y, w, h := d.rect(it)
	if w <= 26 || abs((x+w/2)-m.width/2) > 2 || abs((y+h/2)-m.boardH()/2) > 2 {
		t.Fatalf("shown big in the middle of the screen: %d,%d %dx%d", x, y, w, h)
	}
	boardRows(m)
	press(m, "esc")
	if d.byID(960) != nil || len(bd.Items) != 1 || it.X != 3 || it.Y != 2 || it.W != 26 {
		t.Fatalf("Esc should pin it back where it was: at %d,%d %d wide", it.X, it.Y, it.W)
	}
	// Tab shows the next thing big.
	bd.Items = append(bd.Items, &dOct{ID: 961, Kind: kNote, Header: "Bob", X: 30, Y: 2})
	d.sel = bd.ID
	d.pin = pinSel{board: bd.ID, at: 0, active: true}
	press(m, "alt+space")
	press(m, "tab")
	if d.opened.id != 961 || !d.opened.max || len(bd.Items) != 1 {
		t.Fatalf("Tab should show the next pin big: %+v", d.opened)
	}
	press(m, "alt+space")
	if d.opened.active || len(bd.Items) != 2 || bd.Items[0].ID != 960 {
		t.Fatal("Alt+Space should put it back")
	}
}

// On the flat board, Alt+Space takes up the chosen structure: the arrows
// move it, Alt+Space puts it down (one undo step), Esc puts it back.
func TestDesignGrab2D(t *testing.T) {
	m := room3DModel(t, 160, 50)
	d := m.ds()
	o := d.octs[1]
	d.sel = o.ID
	x0, y0 := o.X, o.Y
	d.save()
	steps := len(d.undo)
	press(m, "alt+space")
	if !d.grab.active || d.grab.in3D || m.grabStatus() != grabHint2D {
		t.Fatal("Alt+Space on the flat board should take up the chosen structure")
	}
	press(m, "down")
	press(m, "down")
	if o.Y <= y0 || d.sel != o.ID {
		t.Fatalf("the arrows should move it: at %d (was %d)", o.Y, y0)
	}
	press(m, "esc")
	if d.grab.active || o.X != x0 || o.Y != y0 || len(d.undo) != steps {
		t.Fatalf("Esc should put it back with no step: at %d,%d", o.X, o.Y)
	}
	press(m, "alt+space")
	press(m, "down")
	press(m, "down")
	press(m, "alt+space")
	if d.grab.active || o.Y <= y0 {
		t.Fatal("Alt+Space should put it down where it was moved to")
	}
	if len(d.undo) != steps+1 {
		t.Fatalf("one undo step: %d, was %d", len(d.undo), steps)
	}
	press(m, "ctrl+z")
	if o := d.byID(o.ID); o == nil || o.Y != y0 {
		t.Fatal("Ctrl+Z should put it back")
	}
}

// Mid-carry the mouse looks round (no button) and what is carried follows;
// a click puts it down.
func TestDesignGrab3DMouseLook(t *testing.T) {
	m := room3DModel(t, 160, 50)
	d := m.ds()
	press(m, "ctrl+f")
	o := d.octs[1]
	x0 := o.X
	cell3D(t, m, o.ID, func(sx, sy int) bool { return d.hit(sx, sy).o == o })
	press(m, "alt+space")
	y := m.boardTop() + 20
	for x := 80; x <= 92; x += 2 {
		m.Update(tea.MouseMotionMsg(tea.Mouse{X: x, Y: y}))
	}
	if o.X <= x0 {
		t.Fatalf("the mouse looked right: the page should follow, at %d (was %d)", o.X, x0)
	}
	m.Update(tea.MouseClickMsg(tea.Mouse{X: 92, Y: y, Button: tea.MouseLeft}))
	m.Update(tea.MouseReleaseMsg(tea.Mouse{X: 92, Y: y, Button: tea.MouseLeft}))
	if d.grab.active || d.drag != dragNone || d.byID(o.ID) == nil || o.X <= x0 {
		t.Fatalf("a click should put it down where it is: active %v at %d", d.grab.active, o.X)
	}
}
