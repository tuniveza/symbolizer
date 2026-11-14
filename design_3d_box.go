package main

// ---- a box in the 3D room: open, with its things inside at three depths -----------------
//
// In the 3D room (design_3d_draw.go) every structure is a solid block
// coming out of the back wall, its front showing its flat drawing. A box
// is not solid: it is a real open box. Its bottom lies against the back
// wall; its four walls (six for a hexagonal box, a ring of them for a
// round hat box) come out toward the viewer, as thick as its drawing's
// walls, their insides darker the deeper they go; its rim, at the front,
// shows the flat drawing's rim (its name, its buttons, the layers' counts).
// The things in it are thin slabs, each showing its miniature: the bottom
// layer's deepest, near the bottom, the top layer's nearest the opening.
// Shut, a lid closes the front, showing the flat drawing's lid.
//
// Every part of it stands for cells of the flat drawing (r3face's cx0 …
// py1): the rim and a slab's front show them, and a click on any part of
// it is a click on the cell straight behind the point (pickAt), so a
// click on a thing's slab chooses that thing, a click on the rim's [open]
// opens it, and a drag on a wall moves the box. Where a thing is covered
// on the flat drawing by one on a layer above, its slab is plain (its
// colour, no words) and a click on it still chooses it.
//
// The design shows: a crate's planks stand out round it, with posts at
// its corners; a tin has a rolled rim standing out at its front; a gift
// box has a ribbon round it and a bow on its lid; a hat box's round wall
// is striped; a cardboard box's flaps stand open; a hexagonal box is a
// hexagonal prism.
//
// Everything is in the room's units (a cell is one unit across, two
// down; depth in the same units), never in pixels.

import "math"

// r3pt is a point of a box's flat drawing, in cells: u across, v down
// (rows; a row is two units tall).
type r3pt struct{ u, v float64 }

// rectPts is the rectangle u0 ≤ u ≤ u1, v0 ≤ v ≤ v1, clockwise on the screen.
func rectPts(u0, v0, u1, v1 float64) []r3pt {
	return []r3pt{{u0, v0}, {u1, v0}, {u1, v1}, {u0, v1}}
}

// roundPts is that rectangle with its corners rounded (ru across, rv
// down), n steps a corner, clockwise. Its straight sides are exactly at
// u0, u1, v0 and v1.
func roundPts(u0, v0, u1, v1, ru, rv float64, n int) []r3pt {
	ru, rv = math.Min(ru, (u1-u0)/2), math.Min(rv, (v1-v0)/2)
	var out []r3pt
	corner := func(cu, cv float64, f func(c, s float64) r3pt, first, last func(p *r3pt)) {
		for k := 0; k <= n; k++ {
			t := float64(k) / float64(n) * math.Pi / 2
			p := f(math.Cos(t), math.Sin(t))
			switch k {
			case 0:
				first(&p)
			case n:
				last(&p)
			}
			out = append(out, p)
		}
	}
	corner(u0+ru, v0+rv, func(c, s float64) r3pt { return r3pt{u0 + ru - ru*c, v0 + rv - rv*s} },
		func(p *r3pt) { p.u = u0 }, func(p *r3pt) { p.v = v0 })
	corner(u1-ru, v0+rv, func(c, s float64) r3pt { return r3pt{u1 - ru + ru*s, v0 + rv - rv*c} },
		func(p *r3pt) { p.v = v0 }, func(p *r3pt) { p.u = u1 })
	corner(u1-ru, v1-rv, func(c, s float64) r3pt { return r3pt{u1 - ru + ru*c, v1 - rv + rv*s} },
		func(p *r3pt) { p.u = u1 }, func(p *r3pt) { p.v = v1 })
	corner(u0+ru, v1-rv, func(c, s float64) r3pt { return r3pt{u0 + ru - ru*s, v1 - rv + rv*c} },
		func(p *r3pt) { p.v = v1 }, func(p *r3pt) { p.u = u0 })
	return out
}

// boxOutlines is a box's outer outline (round its walls) and inner one
// (where its inside starts), w × h cells, with as many corners each, in
// the same order: corner i of one is across the rim from corner i of the
// other. They follow its flat drawing's walls.
func boxOutlines(s boxShape, w, h int) (outer, inner []r3pt) {
	fw, fh := float64(w), float64(h)
	f0, top, bot, wall := float64(s.f0), float64(s.top), float64(s.bottom), float64(s.wall)
	switch s.design {
	case boxTin:
		return roundPts(0, 0, fw, fh, 1.1, 0.8, 3), roundPts(1, top, fw-1, fh-bot, 0.6, 0.4, 3)
	case boxHat:
		k := float64(s.hatK)
		return roundPts(0, 0, fw, fh, k+1, 2, 3), roundPts(1, 1, fw-1, fh-1, k, 1, 3) // 16 corners: a face has at most 19 (see draw)
	case boxHex:
		a, m := float64(boxHexSlant(w, h)), fh/2
		return []r3pt{{a, 0}, {fw - a, 0}, {fw, m}, {fw - a, fh}, {a, fh}, {0, m}},
			[]r3pt{{a + 1, 1}, {fw - a - 1, 1}, {fw - 1, m}, {fw - a - 1, fh - 1}, {a + 1, fh - 1}, {1, m}}
	}
	return rectPts(0, f0, fw, fh), rectPts(wall, top, fw-wall, fh-bot)
}

// c32 is c as a pixel colour.
func c32(c rgbf) uint32 {
	b := func(v float64) float64 { return math.Max(0, math.Min(255, math.Round(v))) }
	return rgb32(b(c.r), b(c.g), b(c.b))
}

// r3box is a box being put in the room: where it is, and what it adds.
type r3box struct {
	r      *room3D
	o      *dOct
	x, y   int // its corner on the screen (flat board cells)
	w, h   int
	hw, hh float64
	mid    r3pt // the middle of its outline
}

// at is point p of its drawing, z out of the wall, in the room.
func (b *r3box) at(p r3pt, z float64) r3vec {
	return r3vec{float64(b.x) + p.u - b.hw, b.hh - 2*(float64(b.y)+p.v), z}
}

// cells is the part of its flat drawing (whole cells, at least one) that
// pts cover, for a face of structure own.
func (b *r3box) cells(own int, pts ...r3pt) r3face {
	u0, v0, u1, v1 := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, p := range pts {
		u0, v0, u1, v1 = math.Min(u0, p.u), math.Min(v0, p.v), math.Max(u1, p.u), math.Max(v1, p.v)
	}
	cx0 := min(max(int(math.Floor(u0+1e-6)), 0), b.w-1)
	cx1 := min(max(int(math.Ceil(u1-1e-6)), cx0+1), b.w)
	cy0 := min(max(int(math.Floor(v0+1e-6)), 0), b.h-1)
	cy1 := min(max(int(math.Ceil(v1-1e-6)), cy0+1), b.h)
	return r3face{own: own, cx0: b.x + cx0, cx1: b.x + cx1, py0: 2 * (b.y + cy0), py1: 2 * (b.y + cy1)}
}

// cell is the face of structure own standing for the one cell (cx, cy) of
// its drawing.
func (b *r3box) cell(own, cx, cy int) r3face {
	return r3face{own: own, cx0: b.x + cx, cx1: b.x + cx + 1, py0: 2 * (b.y + cy), py1: 2 * (b.y + cy + 1)}
}

// wall adds the side from a to b of its drawing, from z0 out to z1,
// facing away from its middle (out) or toward it, coloured col.
func (b *r3box) wall(f r3face, a, c r3pt, z0, z1 float64, out bool, col uint32) {
	pa, pc := b.at(a, z0), b.at(c, z0)
	e := pc.sub(pa)
	if e.dot(e) < 1e-9 {
		return
	}
	n := r3vec{e.y, -e.x, 0}.norm()
	m := b.at(b.mid, z0)
	if (n.dot(pa.sub(m)) < 0) == out {
		n = n.mul(-1)
	}
	f.n, f.mat, f.col, f.edge, f.gloss = n, matSolid, col, out, 0.3
	b.r.quadFace(f, pa, e, r3vec{0, 0, z1 - z0})
}

// flat adds the flat face pts of its drawing at z, facing the viewer.
func (b *r3box) flat(f r3face, z float64, pts ...r3pt) {
	ps := make([]r3vec, len(pts))
	for i, p := range pts {
		ps[i] = b.at(p, z)
	}
	f.n = r3vec{0, 0, 1}
	b.r.addFace(f, ps...)
}

// inset is outline pts moved in toward its middle by k units (a corner
// moved along the line to the middle), for a wall set back from its edge.
func (b *r3box) inset(pts []r3pt, k float64) []r3pt {
	out := make([]r3pt, len(pts))
	for i, p := range pts {
		du, dv := b.mid.u-p.u, 2*(b.mid.v-p.v) // in units
		l := math.Hypot(du, dv)
		if l < 1e-9 {
			out[i] = p
			continue
		}
		out[i] = r3pt{p.u + du/l*k, p.v + dv/l*k/2}
	}
	return out
}

// boxBlock puts box b (dep out of the wall) in the room as an open box,
// and says whether it did; it does not when the box is drawn too small
// to show its walls (a block of its colour then) or is cut at the edge.
func (r *room3D) boxBlock(blk *r3block, dep, hw, hh float64) bool {
	d := r.ds
	if d == nil || !blk.whole {
		return false
	}
	o := blk.o
	x, y, w, h := d.rect(o)
	if w < 12 || h < 5 || x != blk.x0 || y != blk.y0 {
		return false
	}
	design := boxDesign(o)
	s := boxShapeOf(design, w, h)
	lk := boxLookOf(o)
	g := d.boxGridOf(o, w, h)
	open := d.slotOpenness(o, 0, o.DrawerOpen) // 0 shut … 1 open
	// The lid's pose, as the flat drawing has it (design_anim.go): how many
	// inside rows it still covers; whole, it closes the box.
	rows := s.insideRows(w, h)
	covered := boxLidCover(open, len(rows))
	whole := len(rows) > 0 && covered == len(rows) || len(rows) == 0 && open == 0
	outer, inner := boxOutlines(s, w, h)
	b := &r3box{r: r, o: o, x: x, y: y, w: w, h: h, hw: hw, hh: hh}
	for _, p := range outer {
		b.mid.u += p.u / float64(len(outer))
		b.mid.v += p.v / float64(len(outer))
	}
	own := o.ID
	n := len(outer)
	floorZ := clampF(0.03*dep, 0.05, 0.4)

	// The rim, at the front: ring pieces between the outlines, each
	// showing its part of the flat drawing.
	rimOf := func(i int) r3face {
		j := (i + 1) % n
		return b.cells(own, outer[i], outer[j], inner[j], inner[i])
	}
	if !whole {
		for i := range n {
			j := (i + 1) % n
			f := rimOf(i)
			f.mat, f.gloss = matFront, 0.3
			b.flat(f, dep, outer[i], outer[j], inner[j], inner[i])
		}
	}

	// The outside of its walls, by its design.
	r.boxOuterWalls(b, s, lk, outer, rimOf, dep)

	if !whole {
		// The inside of its walls, darker the deeper; its floor.
		bands := 3
		if design == boxCrate {
			bands = 4
		}
		for i := range n {
			j := (i + 1) % n
			for k := range bands {
				z0 := floorZ + (dep-floorZ)*float64(k)/float64(bands)
				z1 := floorZ + (dep-floorZ)*float64(k+1)/float64(bands)
				t := (float64(k) + 0.5) / float64(bands) // 0 at the floor, 1 at the rim
				col := lk.rim.shade(-0.72 + 0.4*t)
				if design == boxCrate && k%2 == 1 {
					col = col.shade(-0.06) // a plank of another shade
				}
				b.wall(rimOf(i), inner[i], inner[j], z0, z1, false, c32(col))
			}
		}
		fl := b.cells(own, inner...)
		fl.mat, fl.col = matSolid, c32(lk.floorC.shade(-0.1))
		if design == boxCrate { // planks across its floor, three rows each
			v0, v1 := inner[0].v, inner[2].v
			for k, v := 0, v0; v < v1; k, v = k+1, v+3 {
				ve := math.Min(v+3, v1)
				f := fl
				f.col = c32(lk.floorC.shade(-0.05 - 0.08*float64(k%2)))
				b.flat(f, floorZ, r3pt{inner[0].u, v}, r3pt{inner[1].u, v}, r3pt{inner[1].u, ve}, r3pt{inner[0].u, ve})
			}
		} else {
			b.flat(fl, floorZ, inner...)
		}
		r.boxThings3D(b, g, s, floorZ, dep)
		if design == boxCardboard && !boxShutAt(open) { // its flaps up, as the flat drawing has them
			r.boxFlaps(b, s, lk, outer, dep)
		}
	}

	// The lid: all on, it closes the front, showing the flat drawing's lid
	// (and a gift box's bow); coming off or going on, it slides over the
	// opening, its lower edge on the row the flat lid's is on, showing the
	// flat drawing's lid there.
	lt := clampF(0.06*dep, 0.3, 0.8)
	lidEdge := c32(lk.lidC.shade(-0.18))
	if whole {
		f := b.cells(own, r3pt{0, float64(s.f0)}, r3pt{float64(w), float64(h)})
		f.mat, f.gloss = matFront, 0.35
		b.flat(f, dep+lt, outer...)
		for i := range n {
			b.wall(rimOf(i), outer[i], outer[(i+1)%n], dep, dep+lt, true, lidEdge)
		}
		if design == boxGift {
			r.boxBow(b, lk, dep+lt)
		}
	} else if covered > 0 {
		part := clipBelow(inner, float64(rows[covered-1]+1))
		if len(part) >= 3 {
			f := b.cells(own, part...)
			f.mat, f.gloss = matFront, 0.35
			b.flat(f, dep+lt, part...)
			for i := range part {
				b.wall(f, part[i], part[(i+1)%len(part)], dep, dep+lt, true, lidEdge)
			}
		}
	}
	return true
}

// clipBelow is outline pts cut off below row line v: the part with v ≤ at
// (a box's lid, part way on).
func clipBelow(pts []r3pt, at float64) []r3pt {
	var out []r3pt
	for i, p := range pts {
		q := pts[(i+1)%len(pts)]
		if p.v <= at {
			out = append(out, p)
		}
		if (p.v-at)*(q.v-at) < 0 { // the edge crosses the line
			t := (at - p.v) / (q.v - p.v)
			out = append(out, r3pt{p.u + t*(q.u-p.u), at})
		}
	}
	return out
}

// boxOuterWalls adds the outside of box b's walls, out to dep: plain, or
// with its design's marks.
func (r *room3D) boxOuterWalls(b *r3box, s boxShape, lk boxLook, outer []r3pt, rimOf func(int) r3face, dep float64) {
	n := len(outer)
	side := c32(lk.rim.shade(-0.3))
	switch s.design {
	case boxCrate:
		// Planks round it, standing out of the wall behind them, with a
		// gap between each; a post at each corner.
		back := b.inset(outer, 0.3)
		planks := max(2, min(5, int(math.Round(dep/2.5))))
		step := dep / float64(planks)
		gap := math.Min(0.35, 0.18*step)
		for i := range n {
			j := (i + 1) % n
			f := rimOf(i)
			b.wall(f, back[i], back[j], 0, dep, true, c32(lk.rim.shade(-0.45)))
			for k := range planks {
				z0, z1 := float64(k)*step+gap/2, float64(k+1)*step-gap/2
				col := lk.rim.shade(-0.05 + 0.08*grain(i, k))
				b.wall(f, outer[i], outer[j], z0, z1, true, c32(col))
				// Its edge toward the viewer, between the plank and the wall behind.
				lip := f
				lip.mat, lip.col = matSolid, c32(col.shade(0.1))
				b.flat(lip, z1, outer[i], outer[j], back[j], back[i])
			}
		}
		post := c32(lk.rim.shade(-0.18))
		pu, pv := 1.2, 0.6 // a post's size across and down (cells, rows)
		for _, c := range []r3pt{outer[0], outer[1], outer[2], outer[3]} {
			su, sv := 1.0, 1.0 // which way is into the box
			if c.u > b.mid.u {
				su = -1
			}
			if c.v > b.mid.v {
				sv = -1
			}
			a, e := c, r3pt{c.u + su*pu, c.v}   // its side along the top or bottom
			a2, e2 := c, r3pt{c.u, c.v + sv*pv} // its side along the left or right
			f := b.cells(b.o.ID, a, r3pt{c.u + su*pu, c.v + sv*pv})
			b.wall(f, a, e, 0, dep-0.02, true, post)
			b.wall(f, a2, e2, 0, dep-0.02, true, post)
		}
	case boxTin:
		// A rolled rim stands out round its front; below it the wall is set back.
		back := b.inset(outer, 0.3)
		roll := math.Min(0.6, 0.15*dep)
		for i := range n {
			j := (i + 1) % n
			f := rimOf(i)
			b.wall(f, back[i], back[j], 0, dep-roll, true, c32(lk.rim.shade(-0.05)))
			b.wall(f, outer[i], outer[j], dep-roll, dep, true, c32(lk.rim.shade(0.25)))
		}
	case boxGift:
		// Its paper, and a ribbon down the middle of each side.
		back := b.inset(outer, 0.05)
		rib := c32(toRGB(lk.extra))
		for i := range n {
			j := (i + 1) % n
			f := rimOf(i)
			b.wall(f, back[i], back[j], 0, dep, true, side)
			a, c := outer[i], outer[j]
			m := r3pt{(a.u + c.u) / 2, (a.v + c.v) / 2}
			l := math.Hypot(c.u-a.u, 2*(c.v-a.v))
			if l < 3 {
				continue
			}
			du, dv := (c.u-a.u)/l*0.6, (c.v-a.v)/l*0.3 // half the ribbon's width along the side
			b.wall(f, r3pt{m.u - du, m.v - dv}, r3pt{m.u + du, m.v + dv}, 0, dep, true, rib)
		}
	case boxHat:
		// Stripes round its curved wall.
		stripe := c32(lk.rim.mix(toRGB(lk.extra), 0.45))
		k := 0
		for i := range n {
			j := (i + 1) % n
			a, c := outer[i], outer[j]
			l := math.Hypot(c.u-a.u, 2*(c.v-a.v))
			parts := max(1, int(math.Ceil(l/2)))
			for p := range parts {
				t0, t1 := float64(p)/float64(parts), float64(p+1)/float64(parts)
				pa := r3pt{a.u + (c.u-a.u)*t0, a.v + (c.v-a.v)*t0}
				pc := r3pt{a.u + (c.u-a.u)*t1, a.v + (c.v-a.v)*t1}
				col := side
				if k%2 == 0 {
					col = stripe
				}
				k++
				b.wall(rimOf(i), pa, pc, 0, dep, true, col)
			}
		}
	default:
		for i := range n {
			b.wall(rimOf(i), outer[i], outer[(i+1)%n], 0, dep, true, side)
		}
		if s.design == boxCardboard && n == 4 { // tape down its front
			w := float64(b.w)
			tape := rimOf(2)
			tape.mat, tape.col = matSolid, c32(toRGB(lk.extra))
			b.wall(tape, r3pt{w/2 - 3, outer[2].v}, r3pt{w/2 + 3, outer[2].v}, dep*0.45, dep, true, c32(toRGB(lk.extra)))
		}
	}
}

// boxThings3D adds the things in box b as thin slabs, a layer at a depth:
// the bottom layer near the floor, the top one nearest the opening. Each
// slab's front shows the thing's cells of the flat drawing g where it
// shows there; where a thing above covers it there, the slab is plain.
func (r *room3D) boxThings3D(b *r3box, g *boxGrid, s boxShape, floorZ, dep float64) {
	d := r.ds
	o := b.o
	view := d.boxView(o) - 1 // -1: all
	_, chosen := d.chosenBoxThing()
	thick := clampF(0.05*dep, 0.15, 0.5)
	depthOfLayer := [boxLayers]float64{0.72, 0.46, 0.2}
	k := 0 // how many slabs so far: each a hair nearer than the one before, as drawn
	var lifted *dOct
	if chosen != nil && d.boxLifted(o, chosen) {
		lifted = chosen // a container open in it: over every layer, as drawn (design_box_fold.go)
	}
	for l := boxLayers - 1; l >= 0; l-- {
		if view >= 0 && l < view {
			continue // see-through: looking at a layer below
		}
		dark := []float64{0, 0.28, 0.48}[l]
		order := boxLayerThings(o, l)
		for i, j := range order { // the chosen one last, as it is drawn
			if o.Items[j] == chosen {
				order = append(append(order[:i:i], order[i+1:]...), j)
				break
			}
		}
		for _, j := range order {
			it := o.Items[j]
			if it == lifted {
				continue
			}
			z := floorZ + (dep-floorZ)*depthOfLayer[l] + 1e-4*float64(k)
			k++
			r.boxSlab(b, g, s, j, z, thick, dark, it == chosen)
		}
	}
	if lifted != nil { // nearer than the top layer's things, bright
		z := floorZ + (dep-floorZ)*(depthOfLayer[0]+0.08) + 1e-4*float64(k)
		r.boxSlab(b, g, s, itemIndex(o, lifted), min(z, dep-1e-3), thick, 0, true)
	}
}

// boxSlab adds thing j of box b as a slab whose front is at z, thick deep.
func (r *room3D) boxSlab(b *r3box, g *boxGrid, s boxShape, j int, z, thick, dark float64, chosen bool) {
	o := b.o
	it := o.Items[j]
	own := it.ID
	if own <= 0 {
		own = o.ID
	}
	act := "boxthing:" + itoa(j)
	x0, y0, tw, th := r.ds.boxThingRect(o, it, b.w, b.h)
	// Each cell of its rectangle: 0 beyond the box's walls, 1 it shows
	// there, 2 something above covers it.
	kind := func(x, y int) int {
		if x < 0 || y < 0 || x >= b.w || y >= b.h || !s.inside(x, y, b.w, b.h) {
			return 0
		}
		if g.c[y][x].act == act {
			return 1
		}
		return 2
	}
	shows := [2]int{-1, -1} // a cell of it that shows, for clicks on its plain parts
	bx0, by0, bx1, by1 := b.w, b.h, -1, -1
	for y := y0; y < y0+th; y++ {
		for x := x0; x < x0+tw; x++ {
			if c := kind(x, y); c != 0 {
				bx0, by0, bx1, by1 = min(bx0, x), min(by0, y), max(bx1, x), max(by1, y)
				if c == 1 && shows[0] < 0 {
					shows = [2]int{x, y}
				}
			}
		}
	}
	if bx1 < 0 {
		return // all of it beyond the walls
	}
	if shows[0] < 0 {
		shows = [2]int{bx0, by0}
	}
	base := toRGB(it.Color)
	if it.Color == "" {
		base = toRGB(cText)
	}
	base = base.mix(toRGB(boxLookOf(o).floor), dark)
	plain := b.cell(own, shows[0], shows[1])
	plain.mat, plain.col, plain.gloss = matSolid, c32(base.shade(-0.15)), 0.25

	// Its front: runs of cells alike in a row, rows alike joined.
	type run struct{ x0, x1, k int }
	rowRuns := func(y int) []run {
		var rs []run
		for x := bx0; x <= bx1; x++ {
			c := kind(x, y)
			if len(rs) > 0 && rs[len(rs)-1].k == c && rs[len(rs)-1].x1 == x {
				rs[len(rs)-1].x1 = x + 1
				continue
			}
			rs = append(rs, run{x, x + 1, c})
		}
		return rs
	}
	same := func(a, c []run) bool {
		if len(a) != len(c) {
			return false
		}
		for i := range a {
			if a[i] != c[i] {
				return false
			}
		}
		return true
	}
	emit := func(rs []run, ya, yb int) {
		for _, ru := range rs {
			if ru.k == 0 {
				continue
			}
			pts := []r3pt{{float64(ru.x0), float64(ya)}, {float64(ru.x1), float64(ya)}, {float64(ru.x1), float64(yb)}, {float64(ru.x0), float64(yb)}}
			f := plain
			if ru.k == 1 {
				f = b.cells(own, pts...)
				f.mat, f.gloss = matFront, 0.3
			}
			b.flat(f, z, pts...)
		}
	}
	start, prev := by0, rowRuns(by0)
	for y := by0 + 1; y <= by1+1; y++ {
		var cur []run
		if y <= by1 {
			cur = rowRuns(y)
		}
		if y <= by1 && same(cur, prev) {
			continue
		}
		emit(prev, start, y)
		start, prev = y, cur
	}

	// Its sides.
	col := base.shade(-0.35)
	if chosen {
		col = toRGB(cAccent)
	}
	box := rectPts(float64(bx0), float64(by0), float64(bx1+1), float64(by1+1))
	saved := b.mid
	b.mid = r3pt{float64(bx0+bx1+1) / 2, float64(by0+by1+1) / 2}
	for i := range box {
		b.wall(plain, box[i], box[(i+1)%4], z-thick, z, true, c32(col))
	}
	b.mid = saved
}

// boxFlaps adds an open cardboard box's flaps: the two at its top folded
// back over the row above its rim, the others standing up from its sides
// and bottom, toward the viewer.
func (r *room3D) boxFlaps(b *r3box, s boxShape, lk boxLook, outer []r3pt, dep float64) {
	flap := c32(lk.rim.mix(toRGB("#f0dcb0"), 0.3))
	f := b.cells(b.o.ID, r3pt{0, 0}, r3pt{float64(b.w), float64(s.f0)})
	f.mat, f.col, f.both, f.gloss = matSolid, flap, true, 0.2
	w, f0 := float64(b.w), float64(s.f0)
	back := math.Min(0.45*dep, 4)
	for _, half := range [][2]float64{{0.3, w/2 - 0.3}, {w/2 + 0.3, w - 0.3}} {
		p0, p1 := b.at(r3pt{half[0], f0}, dep), b.at(r3pt{half[1], f0}, dep)
		tip := b.at(r3pt{half[0], 0}, dep-back)
		u, v := p1.sub(p0), tip.sub(p0)
		ff := f
		ff.n = u.cross(v).norm()
		if ff.n.z < 0 {
			ff.n = ff.n.mul(-1)
		}
		b.r.quadFace(ff, p0, u, v)
	}
	up := math.Min(0.25*dep, 3)
	side := f
	side.col = c32(lk.rim.shade(0.05))
	for _, e := range [][2]r3pt{{outer[3], outer[0]}, {outer[1], outer[2]}, {outer[2], outer[3]}} {
		a, c := e[0], e[1]
		pa := b.at(a, dep)
		ed := b.at(c, dep).sub(pa)
		l := ed.len()
		if l < 1 {
			continue
		}
		in := ed.mul(0.3 / l) // a little in from each corner
		ff := b.cells(b.o.ID, a, c)
		ff.mat, ff.col, ff.both, ff.gloss = side.mat, side.col, true, side.gloss
		ff.n = r3vec{ed.y, -ed.x, 0}.norm()
		b.r.quadFace(ff, pa.add(in), ed.sub(in.mul(2)), r3vec{0, 0, up})
	}
}

// boxBow adds a gift box's bow on its lid (its front at z): two loops of
// ribbon either side of a knot, leaning out.
func (r *room3D) boxBow(b *r3box, lk boxLook, z float64) {
	rib := toRGB(lk.extra)
	k := b.at(r3pt{float64(b.w) / 2, float64(b.h) / 2}, z)
	sz := math.Min(3, float64(b.w)/8)
	f := b.cell(b.o.ID, b.w/2, b.h/2)
	f.mat, f.col, f.both, f.gloss = matSolid, c32(rib), true, 0.5
	for _, sx := range []float64{-1, 1} {
		a := k.add(r3vec{sx * sz, sz * 0.7, sz * 0.5})
		c := k.add(r3vec{sx * sz, -sz * 0.7, sz * 0.5})
		ff := f
		ff.n = a.sub(k).cross(c.sub(k)).norm()
		if ff.n.z < 0 {
			ff.n = ff.n.mul(-1)
		}
		b.r.addFace(ff, k, a, c)
	}
	knot := f
	knot.col = c32(rib.shade(-0.15))
	knot.n = r3vec{0, 0, 1}
	q := sz * 0.3
	b.r.addFace(knot, k.add(r3vec{-q, q, 0.05}), k.add(r3vec{q, q, 0.05}), k.add(r3vec{q, -q, 0.05}), k.add(r3vec{-q, -q, 0.05}))
}
