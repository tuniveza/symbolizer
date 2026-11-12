package main

import (
	"math"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// boxes3DScene is a w×h Design mode with a box of each design, each with
// a thing on each layer, at zoom zi.
func boxes3DScene(t testing.TB, w, h, zi int) (*Model, []*dOct) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	m, _ := NewModel("", 0)
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	m.setMode(designMode)
	d := m.ds()
	d.camX, d.camY, d.zoom = -1, -1, zi
	var out []*dOct
	for n := range boxDesigns {
		b := d.newKind(kBox, 0, 0)
		b.X, b.Y, b.W, b.H, b.Header, b.Layout, b.DrawerOpen = (n%3)*36, 1+(n/3)*16, 34, 14, boxDesignNames[n], n, true
		b.Items = []*dOct{
			{ID: 1000 + 10*n, Kind: kNote, Header: "Keys", Color: "#e5c07b", X: 2, Y: 1},
			{ID: 1001 + 10*n, Kind: kPage, Header: "Plan", Color: "#c678dd", X: 8, Y: 3, Slot: 1},
			{ID: 1002 + 10*n, Kind: kPage, Header: "Glue", Color: "#61afef", X: 14, Y: 2, Slot: 2},
		}
		out = append(out, b)
	}
	d.sel = -1
	return m, out
}

// Every design, open and shut, at every zoom, turned and tilted, draws.
func TestDesign3DBoxEveryDesign(t *testing.T) {
	for _, zi := range []int{0, 2, designZoom100, 8, 9} {
		m, boxes := boxes3DScene(t, 120, 40, zi)
		r := &m.ds().room
		r.yaw, r.pitch = 35, 30
		for _, open := range []bool{true, false} {
			for _, b := range boxes {
				b.DrawerOpen = open
			}
			checkRows(t, m, "boxes")
		}
	}
}

// In 3D a box is hollow: a floor at the back wall, its rim (its flat
// drawing's frame) at the front, and its things as slabs between them, the
// bottom layer's deepest. A click on a thing's slab chooses that thing.
// Shut, a lid closes the front and no thing shows.
func TestDesign3DBoxHollow(t *testing.T) {
	m, d, b := boxTest(t)
	r := &d.room
	r.yaw, r.pitch = 20, 15
	boardRows(m)
	dep := 0.0
	for _, bl := range r.blocks {
		if bl.o == b {
			dep = bl.dep
		}
	}
	if dep <= 0 {
		t.Fatal("the box is not in the room")
	}
	ids := map[int]*dOct{}
	for _, it := range b.Items {
		ids[it.ID] = it
	}
	rim, floor := false, false
	slab := map[int]float64{} // each thing's slab: how far out its front is
	for fi := range r.faces {
		f := &r.faces[fi]
		z := r.pts[f.p0].z
		switch {
		case f.own == b.ID && f.mat == matFront && math.Abs(z-dep) < 1e-3:
			rim = true
		case f.own == b.ID && f.mat == matSolid && f.n.z > 0.99 && z < 0.1*dep:
			floor = true
		case ids[f.own] != nil && f.mat == matFront:
			slab[f.own] = z
		}
	}
	if !rim || !floor {
		t.Fatalf("the open box should have a rim at its front (%v) and a floor at the back (%v)", rim, floor)
	}
	ball, plan, idea := b.Items[0], b.Items[1], b.Items[2] // bottom, middle, top
	if len(slab) != 3 || !(slab[ball.ID] < slab[plan.ID] && slab[plan.ID] < slab[idea.ID] && slab[idea.ID] < dep) {
		t.Fatalf("its things should be slabs, the bottom layer deepest, all inside: %v (rim at %.2f)", slab, dep)
	}
	for _, it := range []*dOct{idea, plan, ball} {
		x, y := cell3D(t, m, it.ID, func(sx, sy int) bool { return true })
		d.lastClick = time.Time{}
		click(m, x, y, tea.MouseLeft)
		if bt, got := d.chosenBoxThing(); bt != b || got != it {
			t.Fatalf("a click in 3D on %s's slab chose %v", it.Header, got)
		}
		d.boxPick = boxSel{} // (Esc would leave the 3D view)
	}

	// Shut: a lid in front of the rim, showing the flat drawing's lid; no things.
	d.sel, d.boxPick = -1, boxSel{}
	b.DrawerOpen = false
	d.anims = nil
	d.dirty = true
	boardRows(m)
	lid := false
	for fi := range r.faces {
		f := &r.faces[fi]
		if ids[f.own] != nil {
			t.Fatalf("shut, %s should not be in the room", ids[f.own].Header)
		}
		if f.own == b.ID && f.mat == matFront && r.pts[f.p0].z > dep {
			lid = true
		}
	}
	if !lid {
		t.Fatal("shut, a lid should close the box's front")
	}
	x, y := cell3D(t, m, b.ID, actAt(d, "boxlid"))
	d.lastClick = time.Time{}
	click(m, x, y, tea.MouseLeft)
	if !b.DrawerOpen {
		t.Fatal("a click in 3D on the lid's [open] should open it")
	}
}
