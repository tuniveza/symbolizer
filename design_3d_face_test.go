package main

import (
	"strings"
	"testing"
	"time"
)

// F faces the structure under the mouse: the camera glides into free roam
// straight in front of it, and it then fills most of the board area, its
// middle and near its edges all it.
func TestDesign3DFace(t *testing.T) {
	m := wordsScene(t, 140, 40)
	withPixels(t, m, 10, 20)
	d := m.ds()
	r := &d.room
	r.yaw, r.pitch = 25, 10
	boardRows(m)
	var notes *dOct
	for _, o := range d.octs {
		if o.Header == "Notes" {
			notes = o
		}
	}
	d.hover = notes.ID + 1
	press(m, "f")
	if !r.free || !r.glide.on {
		t.Fatalf("F: free %v, gliding %v", r.free, r.glide.on)
	}
	if !strings.Contains(m.msg, "Facing") {
		t.Fatalf("status %q", m.msg)
	}
	r.glide.at = time.Now().Add(-2 * faceGlide) // the glide is over
	if m.design3DFaceStep() != nil || r.glide.on {
		t.Fatal("the glide should be over")
	}
	if r.look != [2]float64{0, 0} {
		t.Fatalf("looks %v, want straight at the wall", r.look)
	}
	boardRows(m)
	w, h := m.width, m.boardH()
	at := func(fx, fy float64) int { return r.pickAt(int(fx*float64(w)), int(fy*float64(h))).own }
	if got := at(0.5, 0.5); got != notes.ID {
		t.Fatalf("the middle shows %d, want the card %d", got, notes.ID)
	}
	// It fills faceFill of the width or the height: a little in from the
	// edges on that side is still the card, all the way in.
	wide := at(0.1, 0.5) == notes.ID && at(0.9, 0.5) == notes.ID
	tall := at(0.5, 0.1) == notes.ID && at(0.5, 0.9) == notes.ID
	if !wide && !tall {
		t.Fatal("the card does not fill the board area")
	}
	// Esc: back to the room as it was turned.
	press(m, "esc")
	if r.free || r.yaw != 25 {
		t.Fatalf("Esc: free %v, yaw %d", r.free, r.yaw)
	}
}

// F with nothing under the mouse and nothing chosen says what to do.
func TestDesign3DFaceNothing(t *testing.T) {
	m := wordsScene(t, 140, 40)
	d := m.ds()
	d.room.yaw = 20
	d.hover, d.sel = 0, -1
	press(m, "f")
	if d.room.free || !strings.Contains(m.msg, "point at one") {
		t.Fatalf("free %v, status %q", d.room.free, m.msg)
	}
}

// While writing, F is a letter, not a turn of the camera.
func TestDesign3DFaceWhileWriting(t *testing.T) {
	m := wordsScene(t, 140, 40)
	d := m.ds()
	d.room.yaw = 20
	o := d.octs[len(d.octs)-1]
	d.sel, d.editing, d.hover = o.ID, partHeader, o.ID+1
	press(m, "f")
	if d.room.free {
		t.Fatal("F while writing faced the card")
	}
}
