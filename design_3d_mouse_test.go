package main

import (
	"math"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// Mouse look draws the room again: the camera turned, so the last drawing
// does not stand (it used to, as for a mouse move that changes nothing).
func TestDesign3DMouseLookRedraws(t *testing.T) {
	m := room3DModel(t, 120, 40)
	r := &m.ds().room
	r.yaw, r.pitch = 20, 10
	boardRows(m)
	x, y := roomSurface(t, m)
	m.Update(tea.MouseMotionMsg(tea.Mouse{X: x, Y: y}))
	boardRows(m)
	before := r.frame
	m.Update(tea.MouseMotionMsg(tea.Mouse{X: x + 4, Y: y}))
	if !r.free {
		t.Fatal("the move did not turn the camera")
	}
	boardRows(m)
	if r.frame == before {
		t.Fatal("mouse look turned the camera but the room was not drawn again")
	}
}

// In kitty the mouse reports pixels while the room is in 3D: the rest of
// sy still gets cells, and mouse look turns by the fraction of a cell.
func TestDesign3DPixelMouse(t *testing.T) {
	m := room3DModel(t, 120, 40)
	withPixels(t, m, 10, 20)
	r := &m.ds().room
	r.yaw, r.pitch = 20, 10
	boardRows(m)
	if c := m.pixelMouseCmd(); c == nil || !m.pxm.pending {
		t.Fatal("3D in kitty: pixel mode is asked for")
	}
	if c, ok := m.design3DMouseMsg(pixelMouseMsg{}); !ok || c == nil || !m.pxm.on {
		t.Fatal("pixel mode turned on")
	}
	x, y := roomSurface(t, m)
	px, py := x*10+2, y*20+5
	m.Update(tea.MouseMotionMsg(tea.Mouse{X: px, Y: py}))
	if m.pxm.cell != [2]int{x, y} {
		t.Fatalf("pixel %d,%d is cell %v, want %d,%d", px, py, m.pxm.cell, x, y)
	}
	m.Update(tea.MouseMotionMsg(tea.Mouse{X: px + 3, Y: py}))
	start := r.startLook
	if want := start[0] + 0.3*turnPerCol; math.Abs(r.look[0]-want) > 1e-9 {
		t.Fatalf("3 pixels right: look %v, want %.3f", r.look, want)
	}
	press(m, "esc") // (Update turns pixel mode off after it)
	if m.pxm.on || time.Since(m.pxm.offAt) > time.Second {
		t.Fatal("the flat board: pixel mode off")
	}
	if m.pixelMouseIn(tea.MouseMotionMsg(tea.Mouse{X: px, Y: py})) != nil {
		t.Fatal("a report still in pixels just after pixel mode went off is dropped")
	}
}

// With easing the look follows the mouse in a few steps and ends exactly
// where the mouse sent it.
func TestDesign3DEase(t *testing.T) {
	was := room3DEase
	room3DEase = true
	t.Cleanup(func() { room3DEase = was })
	m := room3DModel(t, 120, 40)
	r := &m.ds().room
	r.yaw, r.pitch = 20, 10
	boardRows(m)
	x, y := roomSurface(t, m)
	m.Update(tea.MouseMotionMsg(tea.Mouse{X: x, Y: y}))
	m.Update(tea.MouseMotionMsg(tea.Mouse{X: x + 4, Y: y + 1}))
	start := r.startLook
	want := [2]float64{wrap180(start[0] + 4*turnPerCol), start[1] + turnPerRow}
	if !r.easing || r.look == want {
		t.Fatal("the look eases, it does not jump")
	}
	for i := 0; i < 40 && r.easing; i++ {
		r.easeAt = r.easeAt.Add(-20 * time.Millisecond)
		m.design3DEase()
	}
	if r.easing || math.Abs(r.look[0]-want[0]) > 0.02 || math.Abs(r.look[1]-want[1]) > 0.02 {
		t.Fatalf("eased to %v (easing %v), want %v", r.look, r.easing, want)
	}
	if wrap180(190) != -170 || wrap180(-180) != -180 || wrap180(-190) != 170 {
		t.Fatal("wrap180")
	}
}

// Resting at the window's edge the camera turns on that way; leaving the
// edge stops it.
func TestDesign3DEdgeTurn(t *testing.T) {
	m := room3DModel(t, 120, 40)
	r := &m.ds().room
	r.yaw, r.pitch = 20, 10
	boardRows(m)
	_, y := roomSurface(t, m)
	m.Update(tea.MouseMotionMsg(tea.Mouse{X: 2, Y: y}))
	m.Update(tea.MouseMotionMsg(tea.Mouse{X: 0, Y: y}))
	if r.edge != [2]int{-1, 0} || !r.edgeOn {
		t.Fatalf("at the left edge: edge %v, ticking %v", r.edge, r.edgeOn)
	}
	look := r.look
	r.edgeStep = time.Now().Add(-50 * time.Millisecond)
	m.design3DEdgeStep()
	if d := wrap180(look[0] - r.look[0]); math.Abs(d-edgeAcross*0.05) > 0.5 {
		t.Fatalf("50 ms at the left edge turned %.2f°, want about %.1f° left", d, edgeAcross*0.05)
	}
	m.Update(tea.MouseMotionMsg(tea.Mouse{X: 3, Y: y}))
	if m.design3DEdgeStep() != nil || r.edgeOn {
		t.Fatal("off the edge: the turning stops")
	}
	// kitty is silent on its padding: heading for the edge fast, the last
	// report short of it is the edge too; slowly, it is not.
	m.Update(tea.MouseMotionMsg(tea.Mouse{X: 8, Y: y}))
	m.Update(tea.MouseMotionMsg(tea.Mouse{X: 6, Y: y}))
	if r.edge != [2]int{} {
		t.Fatalf("6 cells in, 2 a move: not the edge yet, edge %v", r.edge)
	}
	m.Update(tea.MouseMotionMsg(tea.Mouse{X: 3, Y: y}))
	if r.edge != [2]int{-1, 0} {
		t.Fatalf("3 cells in, 3 a move: the next would pass the edge, edge %v", r.edge)
	}
}
