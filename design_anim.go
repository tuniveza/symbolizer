package main

// ---- opening and shutting, shown over a few frames ----------------------------------------
//
// A cabinet's doors and a drawer do not jump open or shut: for a moment
// (openAnimFor) they move, frame by frame. Each is drawn from openness(o),
// 0 shut and 1 open, eased; a tick (designAnimMsg) draws the board again
// while one moves. Only the drawing moves: DrawerOpen changes at once, so
// clicks, keys and undo see the new state straight away.

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
	openAnimFrame = 16 * time.Millisecond
)

// openAnim is a door or drawer moving: since when, and which way.
type openAnim struct {
	start   time.Time
	opening bool
}

// designAnimMsg draws the board again while something opens or shuts.
type designAnimMsg struct{}

// startOpenAnim starts o moving to its (new) DrawerOpen state. Moving the
// other way already, it turns round from where it is.
func (d *designState) startOpenAnim(o *dOct) {
	if !designAnims || o == nil {
		return
	}
	if d.anims == nil {
		d.anims = map[int]openAnim{}
	}
	start := time.Now()
	if a, ok := d.anims[o.ID]; ok {
		if t := time.Since(a.start); t < openAnimFor {
			start = start.Add(-(openAnimFor - t)) // from where it is now
		}
	}
	d.anims[o.ID] = openAnim{start, o.DrawerOpen}
}

// openness is how open o is drawn now: 0 shut, 1 open, between while it
// moves (eased: quick to start, gentle to stop).
func (d *designState) openness(o *dOct) float64 {
	done := 0.0
	if o.DrawerOpen {
		done = 1
	}
	a, ok := d.anims[o.ID]
	if !ok {
		return done
	}
	t := float64(time.Since(a.start)) / float64(openAnimFor)
	if t >= 1 {
		delete(d.anims, o.ID)
		return done
	}
	e := 1 - math.Pow(1-math.Max(t, 0), 3) // ease out
	if a.opening {
		return e
	}
	return 1 - e
}

// animating is whether anything is opening or shutting.
func (d *designState) animating() bool {
	for id, a := range d.anims {
		if time.Since(a.start) >= openAnimFor+openAnimFrame {
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
