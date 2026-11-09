package main

// ---- snapping ----------------------------------------------------------------------
//
// A structure being moved snaps to the closest structure facing it (side by
// side, or one above the other) when it comes close: it sits a gap away
// from it (snapGapX columns beside it, snapGapY rows above or below).
// Dragged with the mouse, it snaps while within reach of that place and
// leaves it as the mouse goes on (dragged onto a structure, to file it,
// it does not snap). Moved by keyboard (Alt + arrows), it snaps only ahead,
// in the way it is going, and then stops there: a press on toward that
// structure does nothing (it can still slide along it, or go back); held,
// it stays a moment where it snaps (snapStay).

import "time"

const (
	snapReachX = 3 // columns (on screen) from the gap place at which a structure snaps to it
	snapReachY = 2 // rows
	snapGapX   = 2 // columns between two structures side by side
	snapGapY   = 1 // rows between two structures one above the other
	snapStay   = 250 * time.Millisecond
)

// snapGap is where o, at (x, y), would sit a gap away from the closest
// structure facing it: across (vert false: the X it would have) or up and
// down (vert true: the Y), and which way that structure is from it
// (toward: 1, more X or Y; -1, less). ok is false when nothing faces it.
func (d *designState) snapGap(o *dOct, x, y int) (place int, vert bool, toward int, ok bool) {
	w, h := d.size(o)
	best := -1
	for _, b := range d.octs {
		if b == nil || b.ID == o.ID {
			continue
		}
		bw, bh := d.size(b)
		across := max(b.X-(x+w), x-(b.X+bw)) // empty columns between them (< 0: they overlap across)
		upDown := max(b.Y-(y+h), y-(b.Y+bh)) // empty rows between them
		var sep, p, tw int
		var v bool
		switch {
		case upDown < 0 && across >= 0: // side by side
			sep, p, tw = across, b.X+bw+snapGapX, -1
			if x+w/2 < b.X+bw/2 {
				p, tw = b.X-w-snapGapX, 1
			}
		case across < 0 && upDown >= 0: // one above the other
			sep, p, tw, v = upDown*2, b.Y+bh+snapGapY, -1, true // a row is about two columns on screen
			if y+h/2 < b.Y+bh/2 {
				p, tw = b.Y-h-snapGapY, 1
			}
		default: // on top of it (being filed?), or off at a corner
			continue
		}
		if best < 0 || sep < best {
			best, place, vert, toward, ok = sep, p, v, tw, true
		}
	}
	return place, vert, toward, ok
}

// snapReach is how far (in board cells) a structure snaps, the same on
// screen at every zoom.
func (d *designState) snapReach() (int, int) {
	z := d.z()
	return max(1, int(float64(snapReachX)/z+0.5)), max(1, int(float64(snapReachY)/z+0.5))
}

// snapNearest moves (x, y), where o is dragged to, a gap away from the
// closest structure, when that is within reach.
func (d *designState) snapNearest(o *dOct, x, y int) (int, int) {
	p, vert, _, ok := d.snapGap(o, x, y)
	if !ok {
		return x, y
	}
	rx, ry := d.snapReach()
	if !vert && abs(p-x) <= rx {
		return p, y
	}
	if vert && abs(p-y) <= ry {
		return x, p
	}
	return x, y
}

// snapAhead is where o, moved by keyboard from (x0, y0) to (x, y), ends up:
// the gap place by the closest structure, when it passed it or is about to
// reach it, going that way. It says whether it snapped.
func (d *designState) snapAhead(o *dOct, x0, y0, x, y int) (int, int, bool) {
	p, vert, _, ok := d.snapGap(o, x0, y0) // what faces it before the step: a big step may land on it
	if !ok {
		return x, y, false
	}
	rx, ry := d.snapReach()
	ahead := func(from, to, reach int) bool {
		if to == from {
			return false
		}
		dir := 1
		if to < from {
			dir = -1
		}
		// Past where it started, and no further than reach beyond where it got to.
		return (p-from)*dir > 0 && (p-to)*dir <= reach
	}
	if !vert && ahead(x0, x, rx) {
		return p, y, true
	}
	if vert && ahead(y0, y, ry) {
		return x, p, true
	}
	return x, y, false
}

// moveByKeys moves o by (dx, dy) cells (Alt + arrows), snapping ahead.
// Sitting a gap away from the closest structure, it goes no closer to it.
func (d *designState) moveByKeys(o *dOct, dx, dy int) {
	d.detachOpened(o) // moved out of its rack: it is out for good
	if p, vert, toward, ok := d.snapGap(o, o.X, o.Y); ok {
		if !vert && o.X == p && dx*toward > 0 {
			dx = 0
		}
		if vert && o.Y == p && dy*toward > 0 {
			dy = 0
		}
		if dx == 0 && dy == 0 {
			return
		}
	}
	x, y, snapped := d.snapAhead(o, o.X, o.Y, o.X+dx, o.Y+dy)
	fromX, fromY := o.X, o.Y
	o.X, o.Y = x, y
	d.followMulti(o, fromX, fromY) // chosen with others: they move as much (design_select.go)
	if snapped {
		d.snapAt = time.Now()
	}
}
