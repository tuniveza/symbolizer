package main

// ---- a cabinet in the 3D room: a hollow cupboard with doors that swing --------------------
//
// In the 3D room a cabinet is not a solid block with its doors painted on
// the front: it is a cupboard. Its back stands against the wall, its sides,
// top and bottom have a thickness, and its front frame shows its name
// across the top, as on the flat board. Two doors, real slabs, fill the
// front, hinged at its left and right edges, each with a knob that stands
// out. Shut, the doors close it and show the flat drawing's doors. Opened
// (a click on a door, Tab, Space), they swing out towards the viewer on
// their hinges, about 105°, over the same moment the flat doors take and
// turned as far at every frame (doorTurn, design_anim.go), and inside are the shelves: real slabs, each its name
// and + on its front edge, and the things on it standing up, each its icon
// and title on its front. A click on a thing, a shelf's +, or a door does
// what it does on the flat board: every face of the cabinet stands for its
// flat board cells.
//
// A door never swings through a side wall or out past the room's front:
// where the wall is close, it opens a little less.

import (
	"math"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// cabSwing is how far a cabinet's doors swing open, in degrees, where the
// room has space for it.
const cabSwing = 105

// r3cab is what the 3D room draws of a cabinet this frame (see cabinets):
// how open its doors are, and the rows behind them.
type r3cab struct {
	p    float64    // 0 shut, 1 open, between while they swing
	ang  [2]float64 // how far (radians) the left and right doors are turned, as drawn
	rows []r3cabRow
}

// r3cabRow is a row behind a cabinet's doors, from the top: a shelf's board
// (a slab), or the things standing on the shelf under it.
type r3cabRow struct {
	shelf  bool
	things []r3thing
}

// r3thing is a thing on a shelf: its cells across (x0 ≤ x < x1, counted
// from the shelves' left edge) and its colour.
type r3thing struct {
	x0, x1 int
	col    string
}

// cabinets works out, once a frame and before the flat board is drawn,
// how open each cabinet in the room is and what is on its shelves, so the
// flat drawing (which the faces show) and the 3D doors agree. Only a
// cabinet the room draws as a cupboard (cupboardShows) is in r.cab: any
// other is a plain block showing its flat drawing, doors and all.
func (r *room3D) cabinets(d *designState, w, h int) {
	if r.cab == nil {
		r.cab = map[int]*r3cab{}
	}
	clear(r.cab)
	for _, o := range d.octs {
		if o.Kind != kCabinet {
			continue
		}
		x, y, ow, oh := d.rect(o)
		if !r.cupboardShows(o, x, y, ow, oh, w, h, d.z()) {
			continue
		}
		c := &r3cab{p: d.openness(o)}
		inside, sw := oh-4, cabShelfW(ow)
		rows := d.cabinetShelvesFit(o, sw, inside)
		more := ""
		if len(rows) > inside && inside >= 1 { // the rows that do not fit: "+N more" on the last
			more = ansi.Truncate("+"+itoa(len(rows)-(inside-1))+" more", sw, "…")
			rows = rows[:inside-1]
		}
		for _, row := range rows {
			var cr r3cabRow
			if len(row) > 0 && row[0].look == lookBoard {
				cr.shelf = true
			}
			at := 0
			for _, s := range row {
				n := lipgloss.Width(s.text)
				if !cr.shelf && strings.TrimSpace(s.text) != "" && at < sw {
					t := r3thing{x0: at, x1: min(at+n, sw), col: cAccent}
					switch {
					case strings.HasPrefix(s.act, "item:"):
						t.col = cText
						if j := atoi(strings.TrimPrefix(s.act, "item:")); j >= 0 && j < len(o.Items) && o.Items[j] != nil && o.Items[j].Color != "" {
							t.col = o.Items[j].Color
						}
					case s.look == lookDim:
						t.col = cDim
					}
					cr.things = append(cr.things, t)
				}
				at += n
			}
			c.rows = append(c.rows, cr)
		}
		if more != "" {
			n := lipgloss.Width(more)
			from := max(0, (sw-n)/2)
			c.rows = append(c.rows, r3cabRow{things: []r3thing{{from, min(from+n, sw), cDim}}})
		}
		r.cab[o.ID] = c
	}
}

// cupboardShows is whether the room draws cabinet o, at (x, y) and ow×oh
// on a w×h screen at zoom z, as a cupboard with doors (cabinetBlock): whole
// in the room, big enough, and out of the wall far enough this frame.
func (r *room3D) cupboardShows(o *dOct, x, y, ow, oh, w, h int, z float64) bool {
	if ow < 10 || oh < 5 {
		return false // drawn as a pill: a plain block
	}
	if x < -roomReach*w || y < -roomReach*h || x+ow > (roomReach+1)*w || y+oh > (roomReach+1)*h {
		return false // cut at the room's edge (see area): a plain block
	}
	b := r3block{o: o, x0: x, y0: y, x1: x + ow, y1: y + oh}
	return math.Max(0.02, blockDepth(&b, z, math.Max(float64(w), float64(2*h)))*r.outK()) >= 1
}

// cabinetDrawnOpen is how open a cabinet's doors are drawn on the flat
// board, p as they move. While the board shows as the 3D room and the
// cabinet is a cupboard there, the doors swing as slabs, so the flat
// drawing behind them (which the shelves and things show) is open all the
// while they are not quite shut; a cabinet drawn as a plain block shows
// the flat doors swinging, as the flat board does.
func (d *designState) cabinetDrawnOpen(o *dOct, p float64) float64 {
	r := &d.room
	if !r.on() {
		return p
	}
	c, ok := r.cab[o.ID]
	if !ok {
		return p
	}
	if c.p > 0 { // as the 3D room has it this frame
		return 1
	}
	return 0
}

// Which faces of a slab to put (see slab): the two spanned by its edges a
// and b (at its corner, and across from it along c), and so on.
const (
	slabC0 = 1 << iota
	slabC1
	slabA0
	slabA1
	slabB0
	slabB1
	slabAll = 1<<6 - 1
)

// slab adds the faces (of which) of the box with corner o and edges a, b
// and c (any way turned), each facing out.
func (r *room3D) slab(f r3face, o, a, b, c r3vec, which int) { r.boxFaces(f, o, a, b, c, which, false) }

// hollow adds the faces of such a box seen from inside it, each facing in
// (the inside of a cupboard).
func (r *room3D) hollow(f r3face, o, a, b, c r3vec, which int) {
	r.boxFaces(f, o, a, b, c, which, true)
}

func (r *room3D) boxFaces(f r3face, o, a, b, c r3vec, which int, in bool) {
	side := func(bit int, u, v, w r3vec) {
		n := u.cross(v)
		if n.dot(n) < 1e-18 {
			return
		}
		n = n.norm()
		if (n.dot(w) > 0) != in {
			n = n.mul(-1) // away from the box (or into it)
		}
		if which&bit != 0 {
			f.n = n
			r.quadFace(f, o, u, v)
		}
		if which&(bit<<1) != 0 {
			f.n = n.mul(-1)
			r.quadFace(f, o.add(w), u, v)
		}
	}
	side(slabC0, a, b, c)
	side(slabA0, b, c, a)
	side(slabB0, a, c, b)
}

// doorAngle is how far (radians) a door wide units and thick units, hinged
// at x = hinge on the front of a block dep deep, swings open at most: up to
// cabSwing, less where its free edge would go through the side wall at
// wall, or out past the front of the room.
func doorAngle(rm *r3room, hinge, wall, wide, thick, dep float64) float64 {
	a := cabSwing * math.Pi / 180
	if wide <= 0 {
		return a
	}
	// Past 90° the free edge goes out beyond the hinge, towards the wall.
	if lim := (math.Abs(hinge-wall) - 0.3) / wide; lim < 1 {
		a = math.Min(a, math.Pi/2+math.Asin(clampF(lim, -1, 1)))
	}
	// Near 90° it is furthest out from the wall.
	if room := rm.depth - 0.3 - dep - thick; room < wide {
		a = math.Min(a, math.Asin(clampF(room/wide, 0, 1)))
	}
	return math.Max(a, 0)
}

// cabinetBlock puts cabinet block b, dep deep, in faces as a cupboard,
// and says whether it did (else it is drawn as any block).
func (r *room3D) cabinetBlock(rm *r3room, b *r3block, dep float64) bool {
	o := b.o
	c := r.cab[o.ID]
	x, y, w, h := b.x0, b.y0, b.x1-b.x0, b.y1-b.y0
	if c == nil || !b.whole || w < 10 || h < 5 || dep < 1 {
		return false
	}
	hw, hh := rm.hw, rm.hh
	X := func(cx int) float64 { return float64(cx) - hw }
	Y := func(cy int) float64 { return hh - float64(2*cy) }
	fill := mixHex(boardBG(), o.Color, 0.18)
	wood := r.hex(mixHex(o.Color, fill, 0.55)) // the doors, as the flat drawing has them
	knob := r.hex(mixHex(o.Color, cText, 0.6))
	col := hex32(cText)
	if o.Color != "" {
		col = r.hex(o.Color)
	}
	inner := mix32(r.hex(fill), col, 0.3)
	back := mix32(inner, 0, 0.3)
	shelfTop := mix32(r.hex(fill), col, 0.45)

	cells := func(cx0, cy0, cx1, cy1 int) r3face {
		return r3face{own: o.ID, cx0: cx0, cx1: cx1, py0: 2 * cy0, py1: 2 * cy1, inside: true}
	}
	solid := func(f r3face, colour uint32, edge bool, gloss float64) r3face {
		f.mat, f.col, f.edge, f.gloss = matSolid, colour, edge, gloss
		return f
	}
	// front adds a face of the flat drawing, cells cx0 ≤ x < cx1 and rows
	// cy0 ≤ y < cy1, at z, from x = l to x = rt (within those cells).
	front := func(cx0, cy0, cx1, cy1 int, l, rt, z float64) {
		f := cells(cx0, cy0, cx1, cy1)
		f.n, f.mat, f.gloss = r3vec{0, 0, 1}, matFront, 0.3
		r.quadFace(f, r3vec{l, Y(cy1), z}, r3vec{rt - l, 0, 0}, r3vec{0, Y(cy0) - Y(cy1), 0})
	}
	X0, X1, Ytop, Ybot := X(x), X(x+w), Y(y), Y(y+h)
	// The opening behind the doors: inside the frame, under the name.
	top, bot := y+3, y+h-1
	Xa, Xb, Yt, Yb := X(x+1), X(x+w-1), Y(top), Y(bot)
	all := cells(x, y, x+w, y+h)

	// The outside: its sides, top and bottom, and its front: shut and
	// still, all of it the flat drawing (its name, the doors, their knobs'
	// ●), as any block's; else the frame round the doors.
	side := solid(all, col, true, 0.35)
	r.slab(side, r3vec{X0, Ybot, 0}, r3vec{X1 - X0, 0, 0}, r3vec{0, Ytop - Ybot, 0}, r3vec{0, 0, dep}, slabA0|slabA1|slabB0|slabB1)
	if c.p <= 0 {
		front(x, y, x+w, y+h, X0, X1, dep)
	} else {
		front(x, y, x+w, top, X0, X1, dep)
		front(x, top, x+1, bot, X0, Xa, dep)
		front(x+w-1, top, x+w, bot, Xb, X1, dep)
		front(x, bot, x+w, y+h, X0, X1, dep)
	}

	// The inside: its walls and back, a little in from the wall. A click
	// there (or on a shelf's top) is on the first cell of the shelves' row
	// it is level with, where nothing is: never on a thing it is not, seen
	// past the things in front.
	sx0, sx1 := x+1+cabDoorW, x+w-1-cabDoorW
	shelves := cells(sx0, top, sx0+1, bot)
	bt := math.Min(0.5, 0.1*dep)
	r.hollow(solid(shelves, inner, true, 0.2), r3vec{Xa, Yb, bt}, r3vec{Xb - Xa, 0, 0}, r3vec{0, Yt - Yb, 0}, r3vec{0, 0, dep - bt},
		slabA0|slabA1|slabB0|slabB1)
	r.quadFace(func() r3face { f := solid(shelves, back, false, 0.1); f.n = r3vec{0, 0, 1}; return f }(),
		r3vec{Xa, Yb, bt}, r3vec{Xb - Xa, 0, 0}, r3vec{0, Yt - Yb, 0})

	// The doors: the left one hinged at the opening's left edge, the right
	// at its right, meeting where the flat drawing's doors meet.
	thick := clampF(0.08*dep, 0.3, 1)
	lw := (w - 3) / 2
	rw := w - 3 - lw
	mid := X(x+1+lw) + 0.5
	const gap = 0.06
	inside := bot - top
	krow := Y(top+inside/2) - 1 // the knobs' height: the middle of the flat drawing's knob row
	for _, left := range []bool{true, false} {
		hinge, wide, wall, flat := Xa, mid-gap-Xa, rm.x0, lw
		dx0 := 1.0    // which way it reaches from the hinge, shut
		hit0 := x + 1 // its cells open (a click there shuts it; shut, any cell of the doors opens them)
		if !left {
			hinge, wide, wall, flat = Xb, Xb-mid-gap, rm.x1, rw
			dx0, hit0 = -1, x+w-1-cabDoorW
		}
		ang := math.Min(doorTurn(c.p), doorAngle(rm, hinge, wall, wide, thick, dep)) // as the flat doors turn (design_anim.go)
		if left {
			c.ang[0] = ang
		} else {
			c.ang[1] = ang
		}
		e := r3vec{dx0 * math.Cos(ang), 0, math.Sin(ang)}  // across the door, from the hinge
		t := r3vec{dx0 * math.Sin(ang), 0, -math.Cos(ang)} // through it, from its outside in
		o0 := r3vec{hinge, Yb, dep}
		f := solid(cells(hit0, top, hit0+cabDoorW, bot), wood, true, 0.35)
		which := slabAll
		if c.p <= 0 {
			which &^= slabC0 // shut and still: its outside is the front's flat drawing
		}
		r.slab(f, o0, e.mul(wide), r3vec{0, Yt - Yb, 0}, t.mul(thick), which)
		// The knob, where the flat drawing's ● is, standing out of the door.
		u := float64(flat) - 1.5
		if flat < 3 {
			u = wide / 2
		}
		ks, kh, kp := 1.0, 1.4, clampF(0.6*thick+0.5, 0.6, 1.2)
		ko := o0.add(e.mul(u - ks/2)).add(r3vec{0, krow - kh/2 - Yb, 0})
		r.slab(solid(f, knob, false, 0.8), ko, e.mul(ks), r3vec{0, kh, 0}, t.mul(-kp), slabAll&^slabC0)
	}

	// Behind the doors, while they are open: the shelves and their things.
	zs := dep - thick - 0.15 // the shelves' front edges, just in from the shut doors
	if c.p <= 0 || zs-bt < 0.6 {
		return true
	}
	depth := zs - bt
	cardFront := zs - math.Min(0.6, 0.08*depth)
	cardThick := clampF(0.5*depth, 0.25, cardFront-bt)
	used := func(i int) bool { return i < len(c.rows) && (c.rows[i].shelf || len(c.rows[i].things) > 0) }
	for i, row := range c.rows {
		ry := top + i
		if ry >= bot {
			break
		}
		if row.shelf {
			// A shelf: a slab from the back to its front edge, which shows its
			// board's name and + (the flat cells), and plain wood past them.
			sh := solid(shelves, shelfTop, true, 0.3)
			r.slab(sh, r3vec{Xa, Y(ry + 1), bt}, r3vec{Xb - Xa, 0, 0}, r3vec{0, 2, 0}, r3vec{0, 0, zs - bt}, slabB0|slabB1)
			front(sx0, ry, sx1, ry+1, X(sx0), X(sx1), zs)
			ends := solid(cells(sx0, ry, sx0+1, ry+1), r.hex(fill), false, 0.3)
			ends.n = r3vec{0, 0, 1}
			r.quadFace(ends, r3vec{Xa, Y(ry + 1), zs}, r3vec{X(sx0) - Xa, 0, 0}, r3vec{0, 2, 0})
			r.quadFace(ends, r3vec{X(sx1), Y(ry + 1), zs}, r3vec{Xb - X(sx1), 0, 0}, r3vec{0, 2, 0})
			continue
		}
		if len(row.things) == 0 {
			continue
		}
		// What its things stand on: the shelf under them, or the cabinet's
		// floor when nothing is below; else a thin ledge under the row.
		sup := ry + 1
		if i+1 >= len(c.rows) || !c.rows[i+1].shelf {
			later := false
			for k := i + 1; k < len(c.rows) && top+k < bot && !later; k++ {
				later = used(k)
			}
			if !later {
				sup = bot
			} else {
				r.slab(solid(shelves, shelfTop, true, 0.3), r3vec{Xa, Y(sup) - 0.25, bt}, r3vec{Xb - Xa, 0, 0}, r3vec{0, 0.25, 0},
					r3vec{0, 0, zs - bt}, slabB0|slabB1|slabC1)
			}
		}
		for _, th := range row.things {
			cx0, cx1 := sx0+th.x0, sx0+th.x1
			// Standing up: its front is its icon and title (the flat cells),
			// its top and sides its colour.
			front(cx0, ry, cx1, sup, X(cx0), X(cx1), cardFront)
			r.slab(solid(cells(cx0, ry, cx1, sup), r.hex(th.col), true, 0.4),
				r3vec{X(cx0), Y(sup), cardFront - cardThick}, r3vec{X(cx1) - X(cx0), 0, 0}, r3vec{0, Y(ry) - Y(sup), 0}, r3vec{0, 0, cardThick},
				slabA0|slabA1|slabB1)
		}
	}
	return true
}
