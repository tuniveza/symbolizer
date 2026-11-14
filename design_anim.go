package main

// ---- opening and shutting, shown over a few frames ----------------------------------------
//
// A cabinet's doors and a drawer do not jump open or shut: for a moment
// (openAnimFor) they move, frame by frame. Each is drawn from openness(o),
// 0 shut and 1 open, eased; a tick (designAnimMsg) draws the board again
// while one moves. Only the drawing moves: DrawerOpen changes at once, so
// clicks, keys and undo see the new state straight away.
//
// The flat board and the 3D room are drawn from the same openness, read
// off the same clock (animNow) at one moment per frame (poseNow: every
// part of a frame, flat drawing and 3D model alike, sees the same pose),
// and turned into a pose by the same functions below (doorTurn, doorFace,
// boxLidCover, boxShutAt, trayRows): at any moment the 3D room shows the
// 3D form of the flat frame, and turning to 3D (or back) while something
// moves, it goes on from where it is.

import (
	"math"
	"time"

	tea "charm.land/bubbletea/v2"
)

// designAnims: opening and shutting move (on in the real program, see
// main.go; off in tests, which look straight away).
var designAnims = false

// openAnimFor is how long a door or drawer takes to open or shut; a frame
// is drawn every openAnimFrame while it moves.
const (
	openAnimFor   = 280 * time.Millisecond
	openAnimFrame = 7 * time.Millisecond // as often as the 3D room is drawn (room3DEvery)
)

// animNow is the clock every opening and shutting is read off (tests step it).
var animNow = time.Now

// openAnim is a door or drawer moving: since when, and which way.
type openAnim struct {
	start   time.Time
	opening bool
}

// designAnimMsg draws the board again while something opens or shuts.
type designAnimMsg struct{}

// animKey is what moves: a structure, and which of its drawers (0 for a
// cabinet's doors, or a drawer unit's first drawer).
type animKey struct{ id, slot int }

// startOpenAnim starts o moving to its (new) DrawerOpen state. Moving the
// other way already, it turns round from where it is.
func (d *designState) startOpenAnim(o *dOct) {
	if o != nil {
		d.startSlotAnim(o, 0, o.DrawerOpen)
	}
}

// startSlotAnim starts drawer slot of o moving open (or shut). Moving
// the other way already, it turns round from exactly where the last frame
// showed it; moving that way already, it goes on as it was.
func (d *designState) startSlotAnim(o *dOct, slot int, opening bool) {
	if !designAnims || o == nil {
		return
	}
	if d.anims == nil {
		d.anims = map[animKey]openAnim{}
	}
	k := animKey{o.ID, slot}
	now := animNow()
	start := now
	if a, ok := d.anims[k]; ok {
		if t := d.poseNow().Sub(a.start); t < openAnimFor {
			if a.opening == opening {
				return
			}
			// Where it is shown, and how far along the other way that is.
			at := a.at(d.poseNow()) // how open
			if !opening {
				at = 1 - at // how shut
			}
			start = now.Add(-time.Duration(easeBack(at) * float64(openAnimFor)))
		}
	}
	d.anims[k] = openAnim{start, opening}
	if !d.poseAt.IsZero() {
		d.poseAt = now // from now on (the next frame shows it from here)
	}
}

// at is how open a moves to have it at moment now: 0 shut, 1 open.
func (a openAnim) at(now time.Time) float64 {
	e := openEase(float64(now.Sub(a.start)) / float64(openAnimFor))
	if a.opening {
		return e
	}
	return 1 - e
}

// poseNow is the moment doors, lids and drawers are shown at: the moment
// the last frame was drawn at (flat or 3D, see designRows), so that every
// part of a frame, flat drawing and 3D model alike, and the clicks on it,
// see the same pose; before any frame, the clock.
func (d *designState) poseNow() time.Time {
	if d.poseAt.IsZero() {
		return animNow()
	}
	return d.poseAt
}

// openEase is how far along a move is at time t (0 … 1 of openAnimFor):
// quick to start, gentle to stop.
func openEase(t float64) float64 {
	t = clampF(t, 0, 1)
	return 1 - math.Pow(1-t, 3)
}

// easeBack is the time at which openEase reaches e.
func easeBack(e float64) float64 { return 1 - math.Cbrt(1-clampF(e, 0, 1)) }

// moving is whether any part of structure id is opening or shutting.
func (d *designState) moving(id int) bool {
	for k := range d.anims {
		if k.id == id {
			return true
		}
	}
	return false
}

// openness is how open o is drawn now: 0 shut, 1 open, between while it
// moves (eased: quick to start, gentle to stop).
func (d *designState) openness(o *dOct) float64 {
	return d.slotOpenness(o, 0, o.DrawerOpen)
}

// slotOpenness is how open drawer slot of o is drawn now; open is where
// it is going.
func (d *designState) slotOpenness(o *dOct, slot int, open bool) float64 {
	done := 0.0
	if open {
		done = 1
	}
	k := animKey{o.ID, slot}
	a, ok := d.anims[k]
	if !ok {
		return done
	}
	now := d.poseNow()
	if now.Sub(a.start) >= openAnimFor {
		delete(d.anims, k)
		return done
	}
	return a.at(now)
}

// animating is whether anything is opening or shutting.
func (d *designState) animating() bool {
	for id, a := range d.anims {
		if animNow().Sub(a.start) >= openAnimFor+openAnimFrame {
			delete(d.anims, id)
		}
	}
	return len(d.anims) > 0
}

// designAnimCmd asks for the next frame while something moves.
func (m *Model) designAnimCmd() tea.Cmd {
	d := m.design
	if d == nil || d.animTick || !d.animating() {
		return nil
	}
	d.animTick = true
	return tea.Tick(openAnimFrame, func(time.Time) tea.Msg { return designAnimMsg{} })
}

// ---- the pose: what a structure looks like at openness p, flat and in 3D alike -------------

// doorTurn is how far (radians) a cabinet's door has swung on its hinge at
// openness p: cabSwing at 1. The 3D doors swing this far (no further than
// the room has space for, doorAngle).
func doorTurn(p float64) float64 { return clampF(p, 0, 1) * cabSwing * math.Pi / 180 }

// doorFace is how much of a door's width faces the front at openness p:
// 1 shut, 0 once it has swung edge on (90°) and past. The flat board draws
// each door this wide, so it is the 3D door seen straight on.
func doorFace(p float64) float64 { return math.Max(0, math.Cos(doorTurn(p))) }

// boxLidCover is how many of a box's n inside rows its lid still covers at
// openness p: all shut, none open, sliding off from the bottom up (the
// lid's lower edge rises, flat and in 3D alike).
func boxLidCover(p float64, n int) int {
	return min(max(int(math.Round((1-clampF(p, 0, 1))*float64(n))), 0), n)
}

// boxShutAt is whether a box looks shut at openness p (its flaps folded
// in, its nails hidden): until it is half open.
func boxShutAt(p float64) bool { return p < 0.5 }

// trayRows is how many of a drawer tray's n rows show at openness p: it
// slides out of its front row by row.
func trayRows(p float64, n int) int {
	return min(max(int(math.Round(clampF(p, 0, 1)*float64(n))), 0), n)
}

// slotsOf is how many parts of o open and shut on their own: each drawer
// of a drawer unit; a cabinet's doors or a box's lid; nothing else.
func slotsOf(o *dOct) int {
	switch o.Kind {
	case kDrawer:
		return drawerCount(o)
	case kCabinet, kBox:
		return 1
	}
	return 0
}

// slotIsOpen is whether part slot of o is open (see slotsOf).
func slotIsOpen(o *dOct, slot int) bool {
	if o.Kind == kDrawer {
		return slotOpen(o, slot)
	}
	return o.DrawerOpen
}

// animOpenChanges starts moving every part that is open now and was shut
// in was (or the other way), matched by ID: undo and redo put back a
// board whose doors, lids and drawers may stand otherwise, and they move
// there as a click would move them, flat and in 3D alike.
func (d *designState) animOpenChanges(was []*dOct) {
	if !designAnims {
		return
	}
	old := map[int]*dOct{}
	for _, o := range was {
		if o != nil {
			old[o.ID] = o
		}
	}
	for _, o := range d.octs {
		p, ok := old[o.ID]
		if !ok || p.Kind != o.Kind {
			continue
		}
		for i := range min(slotsOf(o), slotsOf(p)) {
			if now := slotIsOpen(o, i); now != slotIsOpen(p, i) {
				d.startSlotAnim(o, i, now)
			}
		}
	}
}
