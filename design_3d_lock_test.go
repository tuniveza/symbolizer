package main

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// click3D clicks the left button where the cross is (the cross is the
// pointer in 3D).
func click3D(m *Model, col, row int) {
	m.Update(tea.MouseClickMsg(tea.Mouse{X: col, Y: row, Button: tea.MouseLeft}))
	m.Update(tea.MouseReleaseMsg(tea.Mouse{X: col, Y: row, Button: tea.MouseLeft}))
}

// A structure clicked in 3D stays the chosen one as the camera looks at
// others: only it moves (Alt+wheel), till a click on the empty room lets
// it go and the middle of the view chooses again.
func TestDesign3DClickLocksChosen(t *testing.T) {
	m := room3DModel(t, 160, 50)
	d := m.ds()
	r := &d.room
	press(m, "ctrl+f")
	o, p := d.octs[0], d.octs[1]
	col, row := cell3D(t, m, o.ID, func(sx, sy int) bool { return true })
	click3D(m, col, row)
	boardRows(m)
	if d.sel != o.ID || !r.locked {
		t.Fatalf("clicked: chosen %d (locked %v), want %d", d.sel, r.locked, o.ID)
	}
	cell3D(t, m, p.ID, func(sx, sy int) bool { return true }) // the cross on another structure
	m.design3DAfter()
	if d.sel != o.ID {
		t.Fatalf("the cross on %d chose it; the clicked %d must stay chosen", p.ID, o.ID)
	}
	m.Update(tea.MouseWheelMsg(tea.Mouse{X: col, Y: row, Button: tea.MouseWheelUp, Mod: tea.ModAlt}))
	if o.Layer != 1 || p.Layer != 0 {
		t.Fatalf("Alt+wheel up: layers %d and %d, want the chosen one out (1) and the other on the wall", o.Layer, p.Layer)
	}
	col, row = roomSurface(t, m)
	click3D(m, col, row)
	m.design3DAfter()
	if r.locked || d.sel == o.ID {
		t.Fatalf("a click on the empty room lets go: chosen %d, locked %v", d.sel, r.locked)
	}
	cell3D(t, m, p.ID, func(sx, sy int) bool { return true })
	m.design3DAfter()
	if d.sel != p.ID {
		t.Fatalf("let go, the middle of the view chooses again: chosen %d, want %d", d.sel, p.ID)
	}
}

// Alt + the wheel moves the chosen structure a layer nearer the camera
// (up) or back to the wall (down), and never past the camera.
func TestDesign3DAltWheelLayers(t *testing.T) {
	m := room3DModel(t, 160, 50)
	d := m.ds()
	r := &d.room
	press(m, "ctrl+f")
	o := d.octs[1]
	cell3D(t, m, o.ID, func(sx, sy int) bool { return true })
	m.design3DAfter()
	if d.sel != o.ID {
		t.Fatalf("the cross on %d: chosen %d", o.ID, d.sel)
	}
	wheel := func(b tea.MouseButton) {
		m.Update(tea.MouseWheelMsg(tea.Mouse{X: 5, Y: 5, Button: b, Mod: tea.ModAlt}))
	}
	lens := r.lens
	wheel(tea.MouseWheelUp)
	wheel(tea.MouseWheelUp)
	if o.Layer != 2 || r.lens != lens {
		t.Fatalf("Alt+wheel up twice: layer %d (want 2), lens %v (want %v)", o.Layer, r.lens, lens)
	}
	wheel(tea.MouseWheelDown)
	if o.Layer != 1 {
		t.Fatalf("Alt+wheel down: layer %d, want 1", o.Layer)
	}
	for range 200 {
		wheel(tea.MouseWheelUp)
		boardRows(m)
	}
	if !r.pastCamera(o, o.Layer+1) || r.pastCamera(o, o.Layer) {
		t.Fatalf("up and up: layer %d, it must stop just in front of the camera (z %.1f)", o.Layer, r.pos.z)
	}
	for range 300 {
		wheel(tea.MouseWheelDown)
	}
	if o.Layer != 0 {
		t.Fatalf("down and down: layer %d, want back on the wall", o.Layer)
	}
}

// In 3D the room keeps its size: a structure moved far past its edge does
// not make it grow, and a drag stops it at the walls.
func TestDesign3DRoomKeepsSize(t *testing.T) {
	m := room3DModel(t, 160, 50)
	d := m.ds()
	r := &d.room
	press(m, "ctrl+f")
	r.lookBy(15, 5)
	boardRows(m)
	was := r.rm
	o := d.octs[3]
	o.X += 400 // far past the right wall
	o.Y += 300 // and under the floor
	boardRows(m)
	if r.rm.x0 != was.x0 || r.rm.x1 != was.x1 || r.rm.y0 != was.y0 || r.rm.y1 != was.y1 || r.rm.base != was.base {
		t.Fatalf("the room changed size: x %.0f..%.0f y %.0f..%.0f base %.1f, was x %.0f..%.0f y %.0f..%.0f base %.1f",
			r.rm.x0, r.rm.x1, r.rm.y0, r.rm.y1, r.rm.base, was.x0, was.x1, was.y0, was.y1, was.base)
	}
	d.keepInRoom(o)
	x, y, w, h := d.rect(o)
	if x < r.fix[0] || y < r.fix[1] || x+w > r.fix[2] || y+h > r.fix[3] {
		t.Fatalf("kept in: at %d,%d %dx%d, the room takes cells %v", x, y, w, h, r.fix)
	}
	press(m, "esc")
	press(m, "ctrl+f")
	if r.fixOn {
		t.Fatal("going into 3D again, the room is fitted afresh")
	}
}

// Alt held for the wheel is not Alt tapped: Alt + wheel twice in a row
// does not count as Alt tapped twice (Esc), so 3D stays.
func TestDesign3DAltWheelNotTap(t *testing.T) {
	m := room3DModel(t, 160, 50)
	d := m.ds()
	press(m, "ctrl+f")
	o := d.octs[1]
	cell3D(t, m, o.ID, func(sx, sy int) bool { return true })
	m.design3DAfter()
	alt := tea.Key{Code: tea.KeyLeftAlt}
	for range 2 {
		m.Update(tea.KeyPressMsg(alt))
		m.Update(tea.MouseWheelMsg(tea.Mouse{X: 5, Y: 5, Button: tea.MouseWheelUp, Mod: tea.ModAlt}))
		m.Update(tea.KeyReleaseMsg(alt))
	}
	if !d.room.on() || o.Layer != 2 {
		t.Fatalf("Alt + wheel twice: 3D %v, layer %d; want still 3D and layer 2", d.room.on(), o.Layer)
	}
}
