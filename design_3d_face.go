package main

// ---- F: face a card (the 3D room) ------------------------------------------------------
//
// In 3D, F turns the camera square to the structure under the mouse (else
// the chosen one) and brings it close: the camera glides, in free roam, to
// straight in front of its front, near enough that it fills most of the
// board area. Its words are then big and flat on, to read, and a double-click
// on its title or text writes in it as anywhere. Esc goes back to the room as
// it was turned.

import (
	"math"
	"time"

	tea "charm.land/bubbletea/v2"
)

// faceGlide is how long the camera takes to glide to a card.
const faceGlide = 380 * time.Millisecond

// faceFill is how much of the board area's width or height (whichever it
// meets first) the faced card fills.
const faceFill = 0.86

// room3DFaceMsg is the next step of the camera gliding to a card.
type room3DFaceMsg struct{}

// r3glide is the camera gliding to a card: from where to where, which way
// it looked and will look, and when it set off.
type r3glide struct {
	on               bool
	from, to         r3vec
	fromLook, toLook [2]float64
	at               time.Time
}

// design3DFace takes F in the 3D room. It says what it did.
func (m *Model) design3DFace() tea.Cmd {
	d := m.ds()
	r := &d.room
	o := d.byID(d.hover - 1)
	if d.hover == 0 || o == nil {
		o = d.byID(d.sel)
	}
	rm := r.area(d, m.width, m.boardH(), d.z())
	var blk *r3block
	for i := range r.blocks {
		if o != nil && r.blocks[i].o.ID == o.ID {
			blk = &r.blocks[i]
		}
	}
	if blk == nil {
		m.say("F faces a structure: point at one with the mouse (or click it), then press F.")
		return nil
	}
	if !r.free {
		m.design3DFree() // the camera as it is now, free to go anywhere
	}
	to, look := faceSpot(blk, &rm, r.ff)
	r.glide = r3glide{on: true, from: r.pos, to: to, fromLook: r.look, toLook: look, at: time.Now()}
	m.say("Facing " + octName(blk.o) + ".  Double-click it to write  ·  W A S D: walk  ·  Esc: back to the room.")
	return faceTick()
}

// faceSpot is where the camera stands to face block b square on, and
// which way it looks: straight at the wall, in front of the middle of
// the block's front, as far off as fills faceFill of the board area
// (focal length f).
func faceSpot(b *r3block, rm *r3room, f float64) (r3vec, [2]float64) {
	ow, oh := float64(b.x1-b.x0), float64(2*(b.y1-b.y0)) // units: a row is two
	mid := r3vec{float64(b.x0+b.x1)/2 - rm.hw, rm.hh - float64(b.y0+b.y1), b.dep}
	dist := f * math.Max(ow/(faceFill*float64(rm.w)), oh/(faceFill*float64(2*rm.h)))
	return mid.add(r3vec{0, 0, math.Max(dist, 1)}), [2]float64{0, 0}
}

func faceTick() tea.Cmd {
	return tea.Tick(16*time.Millisecond, func(time.Time) tea.Msg { return room3DFaceMsg{} })
}

// design3DFaceStep moves the gliding camera on, and asks for the next step
// until it is there.
func (m *Model) design3DFaceStep() tea.Cmd {
	if m.design == nil {
		return nil
	}
	r := &m.design.room
	g := &r.glide
	if !g.on || !r.free {
		g.on = false
		return nil
	}
	t := min(1, float64(time.Since(g.at))/float64(faceGlide))
	e := t * t * (3 - 2*t) // slow at both ends
	r.pos = g.from.add(g.to.sub(g.from).mul(e))
	dl := math.Mod(g.toLook[0]-g.fromLook[0]+540, 360) - 180 // the short way round
	r.look = [2]float64{g.fromLook[0] + dl*e, g.fromLook[1] + (g.toLook[1]-g.fromLook[1])*e}
	if t >= 1 {
		r.pos, r.look, g.on = g.to, g.toLook, false // there, exactly
		return nil
	}
	return faceTick()
}
