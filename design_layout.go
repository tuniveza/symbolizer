package main

// ---- zoomed in, the gaps between structures stay their size --------------------------------
//
// Zooming scales every structure, and at 100% or less where it is too.
// Zoomed in, a structure's distance to its neighbour on its left (and above
// it) would grow with the zoom, leaving wide empty gaps; here every gap
// stays its real size, in screen cells, at any zoom: only the structures
// grow. Places are whole cells, so a gap is never a cell more or less.
// A structure with no neighbour in its own row keeps its gap to the nearest
// one before it in another row, or its place beside one it sits by; only
// one with nothing before it at all stays where the zoom puts it.
//
// relayout works it out (once a board change or zoom calls for it: before
// each event and each drawing); baseRect uses it.

import (
	"hash/fnv"
	"math"
	"sort"
)

// nearBy: a structure this many cells or fewer to one side of another is
// beside it.
const nearBy = 8

// layoutScan: how many structures on its left (or above) are looked at to
// find one's neighbours.
const layoutScan = 64

// zoomed is n board cells at zoom z, in whole screen cells (as baseRect
// draws a structure's size).
func zoomed(n, z float64) float64 { return math.Max(math.Round(n*z), 1) }

// screenGap is a gap of g board cells on the screen at zoom z: its own
// size zoomed in, smaller zoomed out but never closed (a cell at least).
func screenGap(g, z float64) float64 {
	if z >= 1 {
		return g
	}
	if g <= 0 {
		return math.Round(g * z) // they overlap on the board: a little less
	}
	return math.Max(math.Round(g*z), 1)
}

// relayout works out where each structure is drawn when zoomed in or out.
func (d *designState) relayout() {
	z := d.z()
	d.layZoom = d.zoom + 1 // +1: the zero value means none yet
	if z == 1 || len(d.octs) == 0 {
		d.lay = nil
		return
	}
	type item struct {
		o          *dOct
		x, y, w, h float64 // its place and size on the board (its own size: gaps are measured from it)
		dw, dh     float64 // its size as drawn (a structure taller than the screen is drawn narrower)
	}
	items := make([]item, 0, len(d.octs))
	hs := fnv.New64a()
	var buf [8]byte
	put := func(v int) {
		for i := range buf {
			buf[i] = byte(v >> (8 * i))
		}
		hs.Write(buf[:])
	}
	put(d.zoom)
	put(d.maxH)
	for _, o := range d.octs {
		if o == nil {
			continue
		}
		if _, _, ok := d.openPlace(o); ok {
			continue // open in a rack: drawn inside it (openRect), its own place left alone
		}
		dw, dh := d.size(o)
		w, h := d.ownSize(o)
		items = append(items, item{o, float64(o.X), float64(o.Y), float64(w), float64(h), float64(dw), float64(dh)})
		put(o.ID)
		put(o.X)
		put(o.Y)
		put(w)
		put(h)
	}
	if key := hs.Sum64(); d.lay != nil && key == d.layKey {
		return
	}
	pos := make(map[int][2]float64, len(items))
	// along places each structure on one axis: lo, size, and the other
	// axis's lo, size (to tell whether two face each other).
	along := func(lo, sz, drawn, olo, osz func(it item) float64, set func(id int, v float64), get func(id int) float64) {
		byLo := make([]int, len(items))
		byHi := make([]int, len(items))
		for i := range items {
			byLo[i], byHi[i] = i, i
		}
		sort.Slice(byLo, func(a, b int) bool { return lo(items[byLo[a]]) < lo(items[byLo[b]]) })
		sort.Slice(byHi, func(a, b int) bool {
			return lo(items[byHi[a]])+sz(items[byHi[a]]) < lo(items[byHi[b]])+sz(items[byHi[b]])
		})
		hi := make([]float64, len(byHi))
		for i, k := range byHi {
			hi[i] = lo(items[k]) + sz(items[k])
		}
		done := make(map[int]bool, len(items))
		// ends is where the furthest reaching of the structures ending at or
		// before it (in its row when row is set) ends on the screen, the
		// smallest of their gaps to it, and the least place that covers
		// none of them (keeping a cell between where there was one).
		ends := func(it item, row bool) (end, gap, floor float64, found bool) {
			end, gap, floor = math.Inf(-1), math.Inf(1), math.Inf(-1)
			j := sort.Search(len(hi), func(i int) bool { return hi[i] > lo(it) }) - 1
			for scanned := 0; j >= 0 && scanned < layoutScan; j, scanned = j-1, scanned+1 {
				n := items[byHi[j]]
				if n.o == it.o || !done[n.o.ID] {
					continue
				}
				if row && (olo(n) >= olo(it)+osz(it) || olo(it) >= olo(n)+osz(n)) {
					continue
				}
				e, g := get(n.o.ID)+zoomed(drawn(n), z), screenGap(lo(it)-hi[j], z)
				end, gap, floor, found = math.Max(end, e), math.Min(gap, g), math.Max(floor, e+math.Min(g, 1)), true
			}
			return end, gap, floor, found
		}
		// place is where items[k] would go, from the structures placed
		// already, and the least place it can have (-Inf: any).
		place := func(k int) (want, floor float64) {
			it := items[k]
			// In its row: the smallest of its gaps, after the furthest of
			// them, and never over one of them.
			if end, gap, floor, ok := ends(it, true); ok {
				return end + gap, floor
			}
			// None before it, but it starts beside (or on) one that starts
			// before it: it keeps its place against that one, from the edge
			// it is nearer to. One drawn shorter than its own size (taller
			// than the screen) leaves no hole beside it: what sits by its
			// end stays by its end.
			found := false
			b := sort.Search(len(byLo), func(i int) bool { return lo(items[byLo[i]]) >= lo(it) }) - 1
			for scanned := 0; b >= 0 && scanned < layoutScan; b, scanned = b-1, scanned+1 {
				n := items[byLo[b]]
				nlo, nhi := lo(n), lo(n)+sz(n)
				if n.o == it.o || !done[n.o.ID] || nhi <= lo(it) || sz(n) <= 0 {
					continue
				}
				if og := math.Max(olo(it)-(olo(n)+osz(n)), olo(n)-(olo(it)+osz(it))); og > nearBy {
					continue // not near it on the other side
				}
				scale := zoomed(drawn(n), z) / sz(n) // screen cells for each of n's own cells
				var c float64
				if ihi := lo(it) + sz(it); drawn(n) < sz(n) && ihi <= nhi && nhi-ihi < lo(it)-nlo { // drawn shorter: by its end
					c = get(n.o.ID) + zoomed(drawn(n), z) - math.Round((nhi-ihi)*scale) - zoomed(drawn(it), z) // by n's end
				} else {
					c = get(n.o.ID) + math.Round((lo(it)-nlo)*scale) // by n's start
				}
				if !found || c > want {
					want, found = c, true
				}
			}
			if found {
				return want, math.Inf(-1)
			}
			// Nothing in its row before it, nor beside it: it keeps the
			// smallest of its gaps to those ending before it in other rows,
			// so a group set apart does not drift off as the zoom grows.
			if end, gap, _, ok := ends(it, false); ok {
				return end + gap, math.Inf(-1)
			}
			return math.Floor(lo(it) * z), math.Inf(-1) // nothing before it: where the zoom puts it
		}
		// Structures lined up on the board (the same left edge, or the same
		// top) stay lined up, where the nearest of them would go, but over
		// none of the structures before any of them. So a row under a board
		// drawn shorter (taller than the screen) closes up under it, even if
		// one of the row has its gap to something far above it.
		for i := 0; i < len(byLo); {
			j := i
			for j < len(byLo) && lo(items[byLo[j]]) == lo(items[byLo[i]]) {
				j++
			}
			want, floor := math.Inf(1), math.Inf(-1)
			for _, k := range byLo[i:j] {
				w, f := place(k)
				want, floor = math.Min(want, w), math.Max(floor, f)
			}
			at := math.Max(want, floor)
			for _, k := range byLo[i:j] {
				set(items[k].o.ID, at)
				done[items[k].o.ID] = true
			}
			i = j
		}
	}
	along(func(it item) float64 { return it.x }, func(it item) float64 { return it.w }, func(it item) float64 { return it.dw },
		func(it item) float64 { return it.y }, func(it item) float64 { return it.h },
		func(id int, v float64) { p := pos[id]; p[0] = v; pos[id] = p }, func(id int) float64 { return pos[id][0] })
	along(func(it item) float64 { return it.y }, func(it item) float64 { return it.h }, func(it item) float64 { return it.dh },
		func(it item) float64 { return it.x }, func(it item) float64 { return it.w },
		func(id int, v float64) { p := pos[id]; p[1] = v; pos[id] = p }, func(id int) float64 { return pos[id][1] })
	d.lay, d.layKey = pos, hs.Sum64()
}

// layAt is where structure o is drawn, in board cells times the zoom (its
// own place times the zoom when the zoom is 100% or less, or it is new).
func (d *designState) layAt(o *dOct) (float64, float64) {
	z := d.z()
	if d.layZoom != d.zoom+1 { // the zoom changed since: work it out again (a view centred straight after a zoom)
		d.relayout()
	}
	if p, ok := d.lay[o.ID]; ok && z != 1 {
		return p[0], p[1]
	}
	return float64(o.X) * z, float64(o.Y) * z
}

// shownXY is where o is drawn, in board cells (for moving the view to it).
func (d *designState) shownXY(o *dOct) (float64, float64) {
	x, y := d.layAt(o)
	z := d.z()
	return x / z, y / z
}

// ---- dragging, zoomed in: the structure stays under the mouse --------------------------
//
// Zoomed in, a gap is drawn its real size, so near a neighbour a board cell
// is one screen cell, while away from any it is the zoom's many. Moving a
// dragged structure by the mouse's cells over the zoom left it behind the
// mouse near its neighbours (ten cells and more at 200%); zoomed out, a gap
// rounded to whole cells made it jump a cell to and fro. followMouse finds
// the board place at which it is drawn where the mouse has taken it.

// dragFrom is where a dragged structure was drawn (layAt) when its drag
// began from board place (ox, oy) at a zoom.
type dragFrom struct {
	id, ox, oy, zoom int
	lx, ly           float64
	ok               bool
}

// followMouse is the board place for o, dragged (dsx, dsy) screen cells
// from where its drag began (d.origX, d.origY), at which it is drawn that
// far from where it was.
func (d *designState) followMouse(o *dOct, dsx, dsy int) (int, int) {
	z := d.z()
	gx, gy := d.origX+int(math.Round(float64(dsx)/z)), d.origY+int(math.Round(float64(dsy)/z))
	if z == 1 {
		return gx, gy // at 100% a board cell is a screen cell everywhere
	}
	drawn := func(x, y int) (float64, float64) {
		o.X, o.Y = x, y
		d.relayout()
		if p, ok := d.lay[o.ID]; ok {
			return p[0], p[1]
		}
		return float64(x) * z, float64(y) * z
	}
	f := d.dragFrom
	if !f.ok || f.id != o.ID || f.ox != d.origX || f.oy != d.origY || f.zoom != d.zoom {
		lx, ly := drawn(d.origX, d.origY)
		f = dragFrom{o.ID, d.origX, d.origY, d.zoom, lx, ly, true}
		d.dragFrom = f
	}
	tx, ty := f.lx+float64(dsx), f.ly+float64(dsy) // where it should be drawn
	// Each axis: from the guess, step by what is still off, at the slope
	// seen between the last two tries (1 by a neighbour, the zoom away from
	// any); a few tries at most, keeping the closest.
	type try struct {
		x, y   int
		lx, ly float64
	}
	cur := try{gx, gy, 0, 0}
	cur.lx, cur.ly = drawn(gx, gy)
	best := cur
	off := func(t try) float64 { return math.Abs(t.lx-tx) + math.Abs(t.ly-ty) }
	prev := try{d.origX, d.origY, f.lx, f.ly}
	for range 4 {
		if off(best) < 0.5 {
			break
		}
		step := func(at, was int, lat, lwas, target float64) int {
			slope := math.Min(1, z) // zoomed out, gaps shrink with the zoom too
			if at != was && lat != lwas {
				slope = math.Min(math.Max((lat-lwas)/float64(at-was), math.Min(1, z)), math.Max(1, z))
			}
			return at + int(math.Round((target-lat)/slope))
		}
		nx, ny := step(cur.x, prev.x, cur.lx, prev.lx, tx), step(cur.y, prev.y, cur.ly, prev.ly, ty)
		if nx == cur.x && ny == cur.y {
			break
		}
		prev, cur = cur, try{nx, ny, 0, 0}
		cur.lx, cur.ly = drawn(nx, ny)
		if off(cur) < off(best) {
			best = cur
		}
	}
	o.X, o.Y = best.x, best.y
	d.relayout()
	return best.x, best.y
}
