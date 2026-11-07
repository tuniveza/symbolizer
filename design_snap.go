package main

// ---- snapping ----------------------------------------------------------------------
//
// A structure being moved snaps to the ones near it when it comes close:
// its edges line up with theirs (left with left, top with top, …) or it
// sits beside them with a small gap. Dragged with the mouse, it snaps to
// the nearest place on each side within reach, and leaves it again as the
// mouse goes on. Moved by keyboard (Alt + arrows), it snaps only ahead, in
// the way it is going, so the next step always gets away; held, it stays a
// moment at each place it snaps to (snapStay).

import "time"

const (
	snapReachX = 3  // columns (on screen) from a place at which a structure snaps to it
	snapReachY = 2  // rows
	snapGapX   = 2  // columns between two structures side by side
	snapGapY   = 1  // rows between two structures one above the other
	snapNearX  = 12 // columns: structures further apart across do not line up their tops and bottoms
	snapNearY  = 6  // rows: further apart up and down, they do not line up their sides
	snapStay   = 250 * time.Millisecond
)

// snapPlaces are the X places (or Y places, when vert) o could snap to at
// (x, y): edges lined up with each structure near it, or beside it.
func (d *designState) snapPlaces(o *dOct, x, y int, vert bool) []int {
	w, h := d.size(o)
	var out []int
	for _, b := range d.octs {
		if b == nil || b.ID == o.ID {
			continue
		}
		bw, bh := d.size(b)
		if vert {
			// Close enough across: overlapping, or a little apart.
			if b.X > x+w+snapNearX || x > b.X+bw+snapNearX {
				continue
			}
			out = append(out, b.Y, b.Y+bh-h, b.Y+bh+snapGapY, b.Y-h-snapGapY)
		} else {
			if b.Y > y+h+snapNearY || y > b.Y+bh+snapNearY {
				continue
			}
			out = append(out, b.X, b.X+bw-w, b.X+bw+snapGapX, b.X-w-snapGapX)
		}
	}
	return out
}

// snapReach is how far (in board cells) a structure snaps, the same on
// screen at every zoom.
func (d *designState) snapReach() (int, int) {
	z := d.z()
	return max(1, int(float64(snapReachX)/z+0.5)), max(1, int(float64(snapReachY)/z+0.5))
}

// snapNearest moves (x, y), where o is dragged to, onto the nearest snap
// place on each side within reach.
func (d *designState) snapNearest(o *dOct, x, y int) (int, int) {
	rx, ry := d.snapReach()
	nearest := func(v, reach int, places []int) int {
		best, bestD := v, reach+1
		for _, p := range places {
			if dist := abs(p - v); dist < bestD {
				best, bestD = p, dist
			}
		}
		return best
	}
	nx := nearest(x, rx, d.snapPlaces(o, x, y, false))
	ny := nearest(y, ry, d.snapPlaces(o, nx, y, true))
	return nx, ny
}

// snapAhead is where o, moved from (x0, y0) to (x, y) by keyboard, ends up:
// on the first snap place it passed or is about to reach, going that way.
// It says whether it snapped.
func (d *designState) snapAhead(o *dOct, x0, y0, x, y int) (int, int, bool) {
	rx, ry := d.snapReach()
	ahead := func(from, to, reach int, places []int) (int, bool) {
		if to == from {
			return to, false
		}
		dir := 1
		if to < from {
			dir = -1
		}
		best, ok := to, false
		for _, p := range places {
			// Past where it started, and no further than reach beyond where it got to.
			if (p-from)*dir > 0 && (p-to)*dir <= reach && (!ok || (p-best)*dir < 0) {
				best, ok = p, true
			}
		}
		return best, ok
	}
	nx, sx := ahead(x0, x, rx, d.snapPlaces(o, x, y, false))
	ny, sy := ahead(y0, y, ry, d.snapPlaces(o, nx, y, true))
	return nx, ny, sx || sy
}

// moveByKeys moves o by (dx, dy) cells (Alt + arrows), snapping ahead.
func (d *designState) moveByKeys(o *dOct, dx, dy int) {
	x, y, snapped := d.snapAhead(o, o.X, o.Y, o.X+dx, o.Y+dy)
	o.X, o.Y = x, y
	if snapped {
		d.snapAt = time.Now()
	}
}
