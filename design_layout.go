package main

// ---- zoomed in, the gaps between structures stay their size --------------------------------
//
// Zooming scales every structure, and at 100% or less where it is too.
// Zoomed in, a structure's distance to its nearest neighbour on its left
// (and above it) would grow with the zoom, leaving wide empty gaps; here it
// stays its real size up to gapKept cells, and only what is beyond that
// grows. A structure with no neighbour on that side stays where the zoom
// puts it, so groups keep their places and their members stay close.
//
// relayout works it out (once a board change or zoom calls for it: before
// each event and each drawing); baseRect uses it.

import (
	"hash/fnv"
	"math"
	"sort"
)

// gapKept: zoomed in, gaps up to this many cells keep their size.
const gapKept = 8

// layoutScan: how many structures on its left (or above) are looked at to
// find one's neighbours.
const layoutScan = 64

// zoomGap is a gap of g board cells on the screen at zoom z (over 1).
func zoomGap(g, z float64) float64 {
	if g <= gapKept {
		return g
	}
	return gapKept + (g-gapKept)*z
}

// relayout works out where each structure is drawn when zoomed in.
func (d *designState) relayout() {
	z := d.z()
	if z <= 1 || len(d.octs) == 0 {
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
		dw, dh := d.size(o)
		mh := d.maxH
		d.maxH = 0
		w, h := d.size(o)
		d.maxH = mh
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
		// place is where items[k] goes, from its neighbours placed already;
		// firm: a neighbour before it holds it there (else it is only where
		// it keeps its place beside one, or where the zoom puts it).
		place := func(k int) (at float64, firm bool) {
			it := items[k]
			at = lo(it) * z // no neighbour: where the zoom puts it
			found := false
			// The structures ending at or before it, nearest first.
			j := sort.Search(len(hi), func(i int) bool { return hi[i] > lo(it) }) - 1
			for scanned := 0; j >= 0 && scanned < layoutScan; j, scanned = j-1, scanned+1 {
				n := items[byHi[j]]
				if n.o == it.o || !done[n.o.ID] || olo(n) >= olo(it)+osz(it) || olo(it) >= olo(n)+osz(n) {
					continue
				}
				c := get(n.o.ID) + drawn(n)*z + zoomGap(lo(it)-hi[j], z)
				if !found || c > at {
					at, found = c, true
				}
			}
			if found {
				return at, true
			}
			// None before it, but it starts beside (or on) one that starts
			// before it: it keeps its place against that one, from the edge
			// it is nearer to. One drawn shorter than its own size (taller
			// than the screen) leaves no hole beside it: what sits by its
			// end stays by its end.
			b := sort.Search(len(byLo), func(i int) bool { return lo(items[byLo[i]]) >= lo(it) }) - 1
			for scanned := 0; b >= 0 && scanned < layoutScan; b, scanned = b-1, scanned+1 {
				n := items[byLo[b]]
				nlo, nhi := lo(n), lo(n)+sz(n)
				if n.o == it.o || !done[n.o.ID] || nhi <= lo(it) || sz(n) <= 0 {
					continue
				}
				if og := math.Max(olo(it)-(olo(n)+osz(n)), olo(n)-(olo(it)+osz(it))); og > gapKept {
					continue // not near it on the other side
				}
				scale := drawn(n) * z / sz(n) // screen cells for each of n's own cells
				var c float64
				if ihi := lo(it) + sz(it); ihi <= nhi && nhi-ihi < lo(it)-nlo {
					c = get(n.o.ID) + drawn(n)*z - (nhi-ihi)*scale - drawn(it)*z // by n's end
				} else {
					c = get(n.o.ID) + (lo(it)-nlo)*scale // by n's start
				}
				if !found || c > at {
					at, found = c, true
				}
			}
			return at, false
		}
		// Structures lined up on the board (the same left edge, or the same
		// top) stay lined up: each goes where the furthest of them must, but
		// of those only kept beside another, the nearest decides.
		for i := 0; i < len(byLo); {
			j := i
			for j < len(byLo) && lo(items[byLo[j]]) == lo(items[byLo[i]]) {
				j++
			}
			held, free := math.Inf(-1), math.Inf(1)
			for _, k := range byLo[i:j] {
				if c, firm := place(k); firm {
					held = math.Max(held, c)
				} else {
					free = math.Min(free, c)
				}
			}
			at := free // none held: the nearest
			switch {
			case math.IsInf(free, 1):
				at = held // all held
			case !math.IsInf(held, -1):
				at = math.Max(held, free) // some held: never before where they must be
			}
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
	if p, ok := d.lay[o.ID]; ok && z > 1 {
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
