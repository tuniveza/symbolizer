package main

// The mouse in the 3D view, made to feel like a game's mouse look.
//
//   - Pixel mode. A terminal says which cell the mouse is in, so the camera
//     turned in steps of a whole cell (1.5° across, 3° down). kitty can say
//     the pixel instead (SGR pixel mode, 1016): while the room is in 3D in
//     kitty it is turned on, every mouse report is turned back into cells
//     for the rest of sy (pixelMouseIn), and mouse look uses the fraction
//     too (mouseAt), so the camera turns pixel by pixel.
//   - No easing. The look turns the moment the mouse moves, by exactly
//     as much: smoothing it would make it lag behind the hand.
//   - The window's edge. A terminal cannot hold the mouse in the window as
//     a game does, so resting at the window's edge the camera turns on by
//     itself that way (design3DEdge), until the mouse leaves the edge.

import (
	"math"
	"time"

	tea "charm.land/bubbletea/v2"
)

const (
	pixelMouseOn  = "\x1b[?1016h"
	pixelMouseOff = "\x1b[?1016l"
	pixelMouseLag = 30 * time.Millisecond // after the room goes 3D: Bubble Tea's own mouse modes are written first
	pixelMouseOld = 300 * time.Millisecond

	edgeBand   = 0.6   // cells from the window's edge that count as resting on it
	edgeAcross = 100.0 // degrees a second the camera turns at the left or right edge
	edgeDown   = 50.0  // and at the top or bottom
	edgeEvery  = 8 * time.Millisecond
)

// pixelMouse is the terminal's pixel mode for the mouse.
type pixelMouse struct {
	on      bool       // asked for: mouse reports are in pixels
	pending bool       // a pixelMouseMsg is on its way
	offAt   time.Time  // when it was last turned off
	cw, ch  int        // a cell's pixels when it was turned on
	f       [2]float64 // the last mouse report, in cells to a fraction (window cells)
	cell    [2]int     // and the cell it is in
}

// pixelMouseMsg: time to turn pixel mode on.
type pixelMouseMsg struct{}

// room3DEdgeMsg is the next turn at the window's edge.
type room3DEdgeMsg struct{}

// pixelMouseWanted is whether the mouse should report pixels now: kitty,
// the board in 3D.
func (m *Model) pixelMouseWanted() bool {
	return room3DPixels && m.inDesign() && m.design != nil && m.design.room.on()
}

// pixelMouseCmd runs after every message: it turns pixel mode on (a moment
// later, see pixelMouseLag) or off as the room goes in and out of 3D.
func (m *Model) pixelMouseCmd() tea.Cmd {
	p := &m.pxm
	want := m.pixelMouseWanted()
	switch {
	case !want && p.on:
		p.on, p.offAt = false, time.Now()
		if m.design != nil {
			m.design.room.hoverOn = false
		}
		return tea.Raw(pixelMouseOff)
	case want && !p.on && !p.pending:
		p.pending = true
		return tea.Tick(pixelMouseLag, func(time.Time) tea.Msg { return pixelMouseMsg{} })
	}
	return nil
}

// pixelMouseIn turns a mouse report in pixels into cells, keeping the
// fraction for mouse look. It drops (nil) a report from just after pixel
// mode went off that is still in pixels: it is off the window in cells.
func (m *Model) pixelMouseIn(msg tea.Msg) tea.Msg {
	if _, ok := msg.(tea.ResumeMsg); ok {
		m.pxm.on = false // Bubble Tea sets its mouse modes again: cells (pixelMouseCmd asks again)
		return msg
	}
	mm, ok := msg.(tea.MouseMsg)
	if !ok {
		return msg
	}
	ms := mm.Mouse()
	p := &m.pxm
	if p.on {
		fx, fy := float64(ms.X)/float64(p.cw), float64(ms.Y)/float64(p.ch)
		ms.X, ms.Y = int(fx), int(fy)
		p.f = [2]float64{fx, fy}
	} else {
		if room3DPixels && time.Since(p.offAt) < pixelMouseOld && (ms.X >= m.width || ms.Y >= m.height) {
			return nil
		}
		p.f = [2]float64{float64(ms.X) + 0.5, float64(ms.Y) + 0.5}
	}
	p.cell = [2]int{ms.X, ms.Y}
	switch msg.(type) {
	case tea.MouseClickMsg:
		return tea.MouseClickMsg(ms)
	case tea.MouseReleaseMsg:
		return tea.MouseReleaseMsg(ms)
	case tea.MouseWheelMsg:
		return tea.MouseWheelMsg(ms)
	case tea.MouseMotionMsg:
		return tea.MouseMotionMsg(ms)
	}
	return msg
}

// mouseAt is where the mouse is, in window cells to a fraction: the pixel
// it is on in pixel mode, else the middle of its cell.
func (m *Model) mouseAt(ms tea.Mouse) (float64, float64) {
	if p := &m.pxm; p.cell == [2]int{ms.X, ms.Y} && p.f != [2]float64{} {
		return p.f[0], p.f[1]
	}
	return float64(ms.X) + 0.5, float64(ms.Y) + 0.5
}

// design3DMouseMsg takes the ticks of this file; it says whether msg was one.
func (m *Model) design3DMouseMsg(msg tea.Msg) (tea.Cmd, bool) {
	switch msg.(type) {
	case pixelMouseMsg:
		p := &m.pxm
		p.pending = false
		if !m.pixelMouseWanted() || p.on {
			return nil, true
		}
		p.on = true
		p.cw, p.ch = termCell(m.width, m.height)
		m.design.room.hoverOn = false // the next report only notes where the mouse is
		return tea.Raw(pixelMouseOn), true
	case room3DEdgeMsg:
		return m.design3DEdgeStep(), true
	}
	return nil, false
}

// turnBy turns the look dx degrees right and dy down, at once.
func (r *room3D) turnBy(dx, dy float64) tea.Cmd {
	return r.turnTo(r.look[0]+dx, r.look[1]+dy)
}

// turnTo turns the look to yaw, pitch (degrees), at once.
func (r *room3D) turnTo(yaw, pitch float64) tea.Cmd {
	r.look = [2]float64{wrap180(yaw), clampF(pitch, -89, 89)}
	return nil
}

// wrap180 is a ∈ [-180, 180).
func wrap180(a float64) float64 {
	return math.Mod(math.Mod(a+180, 360)+360, 360) - 180
}

// design3DEdge notes whether the mouse (window cells x, y, the last move
// dx, dy) rests at the window's edge, and gives the first edge tick when it
// starts to. kitty says nothing while the mouse is on its padding round the
// cells, so the last report can be short of the edge: moving towards the
// edge, a move like the last one more would have passed it, and none came.
func (m *Model) design3DEdge(x, y, dx, dy float64) tea.Cmd {
	r := &m.ds().room
	r.edge = [2]int{edgeWay(x, dx, float64(m.width)), edgeWay(y, dy, float64(m.height))}
	if r.edge == [2]int{} || r.edgeOn {
		return nil
	}
	r.edgeOn, r.edgeStep = true, time.Now()
	return tea.Tick(edgeEvery, func(time.Time) tea.Msg { return room3DEdgeMsg{} })
}

// edgeWay is -1 at the start edge (a coordinate x of a window size big,
// the last move dx), 1 at the end edge, else 0.
func edgeWay(x, dx, size float64) int {
	const ahead = 1.5 // moves like the last one ahead
	switch {
	case x < edgeBand || dx < 0 && x+ahead*dx < edgeBand:
		return -1
	case x > size-edgeBand || dx > 0 && x+ahead*dx > size-edgeBand:
		return 1
	}
	return 0
}

// design3DEdgeStep turns the camera on while the mouse rests at the
// window's edge; it stops when the mouse leaves it, a button is down,
// another window has the keyboard, or the room is not in 3D.
func (m *Model) design3DEdgeStep() tea.Cmd {
	if m.design == nil {
		return nil
	}
	r := &m.design.room
	if r.edge == [2]int{} || !m.inDesign() || !r.free || m.blurred || r.camDrag != camNone {
		r.edgeOn, r.edge = false, [2]int{}
		return nil
	}
	now := time.Now()
	dt := math.Min(now.Sub(r.edgeStep).Seconds(), 0.05)
	r.edgeStep = now
	return tea.Batch(r.turnBy(float64(r.edge[0])*edgeAcross*dt, float64(r.edge[1])*edgeDown*dt),
		tea.Tick(edgeEvery, func(time.Time) tea.Msg { return room3DEdgeMsg{} }))
}
