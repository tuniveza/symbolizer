package main

// ---- drawing the room in 3D -----------------------------------------------------------
//
// A small renderer of its own: every pixel finds the nearest surface (a
// depth buffer), which is lit there. A cell of the screen is four pixels,
// two across and two down, drawn with one of the quarter-block letters
// (▘ ▝ ▖ ▗ ▀ ▄ ▌ ▐ ▚ ▞ ▙ ▛ ▜ ▟ █ or a space): of the ways to split the four
// into two colours, the one closest to them is taken. Where the wall and
// the fronts of the blocks are seen about one cell for one, the letters of
// the flat board show as they are, so the words stay readable.
//
// The world's units: one is a cell's width, and a cell is two units tall.
// x goes right, y up, and z out of the back wall toward the viewer. The
// back wall is the flat board, and the screen's board area is centred on
// (0, 0, 0): column sx of the flat board is x = sx − hw, row sy is
// y = hh − 2·sy (its top edge).

import (
	"image/color"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// ---- small sums ----------------------------------------------------------------------

type r3vec struct{ x, y, z float64 }

func (a r3vec) add(b r3vec) r3vec   { return r3vec{a.x + b.x, a.y + b.y, a.z + b.z} }
func (a r3vec) sub(b r3vec) r3vec   { return r3vec{a.x - b.x, a.y - b.y, a.z - b.z} }
func (a r3vec) mul(k float64) r3vec { return r3vec{a.x * k, a.y * k, a.z * k} }
func (a r3vec) dot(b r3vec) float64 { return a.x*b.x + a.y*b.y + a.z*b.z }
func (a r3vec) cross(b r3vec) r3vec {
	return r3vec{a.y*b.z - a.z*b.y, a.z*b.x - a.x*b.z, a.x*b.y - a.y*b.x}
}
func (a r3vec) norm() r3vec            { return a.mul(1 / math.Sqrt(a.dot(a))) }
func (a r3vec) len() float64           { return math.Sqrt(a.dot(a)) }
func clampF(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }
func hex32(h string) uint32            { r, g, b := rgb(h); return uint32(r)<<16 | uint32(g)<<8 | uint32(b) }
func rgb32(r, g, b float64) uint32     { return uint32(r)<<16 | uint32(g)<<8 | uint32(b) }
func chans(c uint32) (r, g, b float64) {
	return float64(c >> 16 & 255), float64(c >> 8 & 255), float64(c & 255)
}

// mix32 is a colour t of the way from a to b.
func mix32(a, b uint32, t float64) uint32 {
	ar, ag, ab := chans(a)
	br, bg, bb := chans(b)
	return rgb32(ar+(br-ar)*t+0.5, ag+(bg-ag)*t+0.5, ab+(bb-ab)*t+0.5)
}

// colour32 is a cell's colour as 0xRRGGBB (or, when it has none, or).
func colour32(c color.Color, or uint32) uint32 {
	switch v := c.(type) {
	case nil:
		return or
	case color.RGBA:
		return uint32(v.R)<<16 | uint32(v.G)<<8 | uint32(v.B)
	}
	r, g, b, _ := c.RGBA()
	return (r>>8)<<16 | (g>>8)<<8 | b>>8
}

// hex is hex32, remembered: thousands of blocks have only a few colours.
func (r *room3D) hex(h string) uint32 {
	c, ok := r.hexes[h]
	if !ok {
		if r.hexes == nil || len(r.hexes) > 4096 {
			r.hexes = map[string]uint32{}
		}
		c = hex32(h)
		r.hexes[h] = c
	}
	return c
}

// ---- the camera ------------------------------------------------------------------------

// r3view is how the camera sees: where it is, which way is right, up and
// forward, its focal length, and the screen point straight ahead (px, py),
// all in units (a cell is one unit across and two down).
type r3view struct {
	cam, right, up, fwd r3vec
	f, px, py           float64
}

// ray is the direction from the camera through screen point (ux, uy).
func (v *r3view) ray(ux, uy float64) r3vec {
	dx, dy := (ux-v.px)/v.f, -(uy-v.py)/v.f
	return v.fwd.add(v.right.mul(dx)).add(v.up.mul(dy))
}

// ---- the room's size ------------------------------------------------------------------

// r3block is a structure as a block: its place on the flat board (cells,
// x0 ≤ x < x1, y0 ≤ y < y1, cut at a screen beyond the screen's edges).
type r3block struct {
	o              *dOct
	x0, y0, x1, y1 int
	whole          bool    // not cut: all of it is in the room
	dep            float64 // how far it comes out of the wall
}

// r3room is the room's size. The back wall is the flat board's cells
// tx0 ≤ x < tx0+tw, ty0 ≤ y < ty0+th: the screen's board area, widened to
// take in every structure that shows on it (whole, up to a screen beyond
// it) and a margin. The side walls stand at its left and right edges, and
// the floor lies along its bottom edge, so no block goes through them.
type r3room struct {
	w, h           int     // the screen's board area, cells
	hw, hh         float64 // half of it, in units
	tx0, ty0       int
	tw, th         int
	x0, x1, y0, y1 float64 // the side walls, the floor and the top of the back wall
	depth          float64 // how far the floor and the side walls come out
	big            float64 // the screen's bigger side, in units
}

// The margin between the outermost blocks and the walls, in cells: two
// across, one down (one unit is a cell's width, a cell is two units tall).
const roomMarginX, roomMarginY = 2, 1

// area works out the room for a w×h board area at zoom z, and which
// structures are blocks in it. The room is fitted to what shows: the
// blocks (with a pulled out drawer) and the pen's ink on the screen, or,
// with nothing on the screen, the screen itself.
func (r *room3D) area(d *designState, w, h int, z float64) r3room {
	r.blocks = r.blocks[:0]
	big := math.Max(float64(w), float64(2*h))
	x0, y0, x1, y1 := math.MaxInt, math.MaxInt, math.MinInt, math.MinInt
	take := func(bx0, by0, bx1, by1 int) {
		bx0, by0, bx1, by1 = max(bx0, -w), max(by0, -h), min(bx1, 2*w), min(by1, 2*h)
		if bx0 < bx1 && by0 < by1 {
			x0, y0, x1, y1 = min(x0, bx0), min(y0, by0), max(x1, bx1), max(y1, by1)
		}
	}
	deepest := 0.0
	for _, o := range d.octs {
		x, y, ow, oh := d.rect(o)
		if x >= w || y >= h || x+ow <= 0 || y+oh <= 0 {
			continue // nothing of it on the screen: not in the room
		}
		b := r3block{o: o, x0: max(x, -w), y0: max(y, -h), x1: min(x+ow, 2*w), y1: min(y+oh, 2*h)}
		b.whole = b.x0 == x && b.y0 == y && b.x1 == x+ow && b.y1 == y+oh
		b.dep = blockDepth(&b, z, big)
		deepest = math.Max(deepest, b.dep)
		r.blocks = append(r.blocks, b)
		take(b.x0, b.y0, b.x1, b.y1)
		if o.DrawerOpen && o.Kind != kCabinet {
			dx, dy, dw, dh := d.drawerRect(o) // drawn beside it, on the wall
			take(dx, dy, dx+dw, dy+dh)
		}
	}
	if ix0, iy0, ix1, iy1, ok := r.ink(d, w, h); ok {
		take(ix0, iy0, ix1, iy1)
	}
	if x0 > x1 {
		x0, y0, x1, y1 = 0, 0, w, h // nothing shows: the screen
	}
	x0, y0, x1, y1 = x0-roomMarginX, y0-roomMarginY, x1+roomMarginX, y1+roomMarginY
	rm := r3room{w: w, h: h, hw: float64(w) / 2, hh: float64(h), tx0: x0, ty0: y0, tw: x1 - x0, th: y1 - y0, big: big}
	rm.x0, rm.x1 = float64(x0)-rm.hw, float64(x1)-rm.hw
	rm.y1, rm.y0 = rm.hh-float64(2*y0), rm.hh-float64(2*y1)
	// The floor comes out a little under half the room's size (no more
	// than for the screen), and over three times the deepest block.
	rm.depth = math.Max(8, math.Max(deepest/0.3, 0.45*math.Min(big, math.Max(rm.x1-rm.x0, rm.y1-rm.y0))))
	return rm
}

// ink is the part of the w×h screen the pen's ink is on (cells x0 ≤ x <
// x1, y0 ≤ y < y1), when there is any.
func (r *room3D) ink(d *designState, w, h int) (x0, y0, x1, y1 int, ok bool) {
	if len(d.marks) == 0 {
		return 0, 0, 0, 0, false
	}
	if r.inkVer != d.marksVer || r.inkLen != len(d.marks) { // where it is on the board, kept until it changes
		b := [4]int{math.MaxInt, math.MaxInt, math.MinInt, math.MinInt}
		for at := range d.marks {
			b = [4]int{min(b[0], at[0]), min(b[1], at[1]), max(b[2], at[0]+1), max(b[3], at[1]+1)}
		}
		r.inkBox, r.inkVer, r.inkLen = b, d.marksVer, len(d.marks)
	}
	z := d.z()
	b := r.inkBox
	x0, y0 = int(math.Floor((float64(b[0])-d.camX)*z)), int(math.Floor((float64(b[1])-d.camY)*z))
	x1, y1 = int(math.Ceil((float64(b[2])-d.camX)*z)), int(math.Ceil((float64(b[3])-d.camY)*z))
	x0, y0, x1, y1 = max(x0, 0), max(y0, 0), min(x1, w), min(y1, h)
	return x0, y0, x1, y1, x0 < x1 && y0 < y1
}

// blockDepth is how far block b comes out of the wall at zoom z, on a
// screen whose bigger side is big units.
func blockDepth(b *r3block, z, big float64) float64 {
	dep := math.Max(0.6, depthOf(b.o.Kind)*z)
	return math.Min(dep, math.Min(0.135*big, 0.8*float64(max(b.x1-b.x0, 2*(b.y1-b.y0)))))
}

// ---- the flat board as a picture ----------------------------------------------------------

// r3texel is one cell of the flat board: its four quarters' colours (top
// left, top right, bottom left, bottom right), its colours, and its letter
// when that can show as it is.
type r3texel struct {
	q      [4]uint32
	fg, bg uint32
	glyph  string
	blank  bool // a space
}

// quadMask is which quarters of a cell each block letter from ▀ (U+2580)
// to ▟ (U+259F) fills: 1 top left, 2 top right, 4 bottom left, 8 bottom
// right; 0 for the others (▁ ▂ … ░ ▒ ▓ and the rest).
var quadMask = [32]int{
	0x80 - 0x80: 3, 0x84 - 0x80: 12, 0x88 - 0x80: 15, 0x8c - 0x80: 5, 0x90 - 0x80: 10,
	0x96 - 0x80: 4, 0x97 - 0x80: 8, 0x98 - 0x80: 1, 0x99 - 0x80: 13, 0x9a - 0x80: 9,
	0x9b - 0x80: 7, 0x9c - 0x80: 11, 0x9d - 0x80: 2, 0x9e - 0x80: 6, 0x9f - 0x80: 14,
}

// quadGlyph is the letter for each mask (the quarters in the letter's colour).
var quadGlyph = [16]string{" ", "▘", "▝", "▀", "▖", "▌", "▞", "▛", "▗", "▚", "▐", "▜", "▄", "▙", "▟", "█"}

// texelOf reads one cell of the flat board.
func texelOf(cell *uv.Cell, defFG, defBG uint32) r3texel {
	t := r3texel{fg: colour32(cell.Style.Fg, defFG), bg: colour32(cell.Style.Bg, defBG)}
	if cell.Style.Attrs&uv.AttrReverse != 0 {
		t.fg, t.bg = t.bg, t.fg
	}
	all := func(c uint32) { t.q = [4]uint32{c, c, c, c} }
	s := cell.Content
	ru, size := utf8.DecodeRuneInString(s)
	if ru >= 0x2580 && ru <= 0x259f && quadMask[ru-0x2580] != 0 && size == len(s) {
		mask := quadMask[ru-0x2580]
		for i := range 4 {
			t.q[i] = t.bg
			if mask&(1<<i) != 0 {
				t.q[i] = t.fg
			}
		}
		return t
	}
	switch s {
	case "", " ":
		all(t.bg)
		t.blank = true
	case "▒":
		all(mix32(t.bg, t.fg, 0.5))
	case "░":
		all(mix32(t.bg, t.fg, 0.25))
	case "▓":
		all(mix32(t.bg, t.fg, 0.75))
	default:
		// A letter or a line: seen from afar, a tint of its colour.
		k := 0.35
		if ru >= 0x2500 && ru <= 0x257f {
			k = 0.6 // box lines: the edges of things
		}
		all(mix32(t.bg, t.fg, k))
		if cell.Width == 1 && ansi.StringWidth(s) == 1 {
			t.glyph = s
		}
	}
	return t
}

// takeRoom reads the back wall's cells: the flat board c where it is on
// the screen, and beyond the screen the drawings of the structures that
// stick out of it (on the board's own colour).
func (r *room3D) takeRoom(d *designState, c *boardCanvas, rm *r3room) {
	n := rm.tw * rm.th
	if len(r.tex) != n {
		r.tex = make([]r3texel, n)
		r.placed = make([]int32, n)
	}
	defFG, defBG := hex32(cText), hex32(cBG)
	plain := r3texel{fg: defFG, bg: defBG, q: [4]uint32{defBG, defBG, defBG, defBG}, blank: true}
	for i := range r.tex {
		r.tex[i] = plain
	}
	// put reads cell (x, y) of the flat board, when it is in the room.
	put := func(x, y int, cell *uv.Cell) {
		if x >= rm.tx0 && x < rm.tx0+rm.tw && y >= rm.ty0 && y < rm.ty0+rm.th {
			r.tex[(y-rm.ty0)*rm.tw+x-rm.tx0] = texelOf(cell, defFG, defBG)
		}
	}
	w, h := rm.w, rm.h
	for _, b := range r.blocks {
		x, y, ow, oh := d.rect(b.o)
		if x >= 0 && y >= 0 && x+ow <= w && y+oh <= h {
			continue // all of it on the screen: it is read from there
		}
		k, ok := d.drawKeyOf(b.o)
		kept := d.drawn[b.o.ID]
		if !ok || len(kept) == 0 || kept[0].key != k {
			continue // no fresh drawing of it (it is opening or shutting): only its colour
		}
		for _, p := range kept[0].parts {
			if p.cells == nil {
				continue
			}
			// Only the rows and columns of it that are in the room.
			px, py := p.l.GetX(), p.l.GetY()
			for cy := max(0, rm.ty0-py); cy < min(p.cells.Height(), rm.ty0+rm.th-py); cy++ {
				line := p.cells.Lines[cy]
				for cx := max(0, rm.tx0-px); cx < min(len(line), rm.tx0+rm.tw-px); cx++ {
					if sx, sy := px+cx, py+cy; sx < 0 || sy < 0 || sx >= w || sy >= h {
						put(sx, sy, &line[cx])
					}
				}
			}
		}
	}
	for y := 0; y < h && y < c.Height(); y++ {
		line := c.Lines[y]
		for x := 0; x < w && x < len(line); x++ {
			put(x, y, &line[x])
		}
	}
}

// ---- the scene ------------------------------------------------------------------------

// What a face is made of.
const (
	matBoard = iota // the back wall: the flat board
	matFront        // a block's front: its part of the flat board
	matSolid        // one colour (a block's sides)
	matFloor        // planks
	matWall         // a side wall, with a skirting board
	matLamp         // the bulb: it shines, it is not lit
	matShade        // the lamp's shade, lit from inside
)

// r3face is one flat, convex face.
type r3face struct {
	p0, pn int // its corners: pts[p0 : p0+pn]
	n      r3vec
	k      float64 // its plane: n · p = k
	mat    int
	col    uint32
	gloss  float64
	o      r3vec // for the edge lines of a block's side: a corner, two edge directions and lengths
	eu, ev r3vec
	lu, lv float64
	edge   bool
	both   bool // seen from either side (the shade)
	own    int  // the structure it is part of (its ID), or -1: the room
	// The flat board cells it shows or stands on: columns cx0 ≤ x < cx1,
	// rows of half cells py0 ≤ y < py1 (the back wall and the blocks).
	cx0, cx1, py0, py1 int
	seen               bool // drawn this frame (it faced the camera)
}

// depthOf is how far a kind of structure comes out of the wall, at 100%.
func depthOf(kind string) float64 {
	switch kind {
	case kPage, kNote, kSlide, kDrawing:
		return 1
	case kBook:
		return 2.5
	case kBoard:
		return 2
	case kOct:
		return 5
	case kCabinet, kDrawer, kRack:
		return 12
	case kBox: // a box is deep: things lie in it
		return 10
	}
	return 3
}

func (r *room3D) addFace(f r3face, pts ...r3vec) {
	f.p0, f.pn = len(r.pts), len(pts)
	r.pts = append(r.pts, pts...)
	f.k = f.n.dot(pts[0])
	r.faces = append(r.faces, f)
}

// quadFace adds the rectangle with corner o and edges u and v, facing n.
func (r *room3D) quadFace(f r3face, o, u, v r3vec) {
	f.o, f.lu, f.lv = o, math.Sqrt(u.dot(u)), math.Sqrt(v.dot(v))
	if f.lu > 0 && f.lv > 0 {
		f.eu, f.ev = u.mul(1/f.lu), v.mul(1/f.lv)
	}
	r.addFace(f, o, o.add(u), o.add(u).add(v), o.add(v))
}

// lampAt is where the lamp hangs: at the front left, near the top.
func lampAt(rm *r3room) r3vec {
	return r3vec{rm.x0 + 0.15*(rm.x1-rm.x0), rm.y1 - math.Max(1, 0.02*(rm.y1-rm.y0)), 0.6 * rm.depth}
}

// scene puts the room, the blocks and the lamp in faces.
func (r *room3D) scene(rm *r3room, z float64, lamp r3vec) {
	r.faces, r.pts = r.faces[:0], r.pts[:0]
	hw, hh := rm.hw, rm.hh
	bg, text, accent := hex32(cBG), hex32(cText), hex32(cAccent)
	x0, x1, y0, y1, depth := rm.x0, rm.x1, rm.y0, rm.y1, rm.depth
	wide, tall := x1-x0, y1-y0

	// The room (the back wall is face 0). The back wall and the floor have
	// a back too, seen from behind or below in free roam.
	r.quadFace(r3face{n: r3vec{0, 0, 1}, mat: matBoard, own: -1, gloss: 0.15,
		cx0: rm.tx0, cx1: rm.tx0 + rm.tw, py0: 2 * rm.ty0, py1: 2 * (rm.ty0 + rm.th)},
		r3vec{x0, y0, 0}, r3vec{wide, 0, 0}, r3vec{0, tall, 0})
	r.quadFace(r3face{n: r3vec{0, 1, 0}, mat: matFloor, own: -1, col: mix32(mix32(bg, text, 0.16), accent, 0.06), gloss: 0.6},
		r3vec{x0, y0, 0}, r3vec{wide, 0, 0}, r3vec{0, 0, depth})
	r.quadFace(r3face{n: r3vec{1, 0, 0}, mat: matWall, own: -1, col: mix32(mix32(bg, text, 0.09), accent, 0.05), gloss: 0.1},
		r3vec{x0, y0, 0}, r3vec{0, tall, 0}, r3vec{0, 0, depth})
	r.quadFace(r3face{n: r3vec{-1, 0, 0}, mat: matWall, own: -1, col: mix32(bg, text, 0.07), gloss: 0.1},
		r3vec{x1, y0, 0}, r3vec{0, tall, 0}, r3vec{0, 0, depth})
	back := mix32(mix32(bg, text, 0.12), 0, 0.2)
	r.quadFace(r3face{n: r3vec{0, 0, -1}, mat: matSolid, own: -1, col: back},
		r3vec{x0, y0, -0.05}, r3vec{wide, 0, 0}, r3vec{0, tall, 0})
	r.quadFace(r3face{n: r3vec{0, -1, 0}, mat: matSolid, own: -1, col: back},
		r3vec{x0, y0 - 0.05, 0}, r3vec{wide, 0, 0}, r3vec{0, 0, depth})

	// Every structure that shows, as a block.
	for i := range r.blocks {
		b := &r.blocks[i]
		o := b.o
		// A block on top of another on the flat board comes out a hair further.
		dep := b.dep + 1e-5*float64(i)
		X0, X1 := float64(b.x0)-hw, float64(b.x1)-hw
		Yt, Yb := hh-float64(2*b.y0), hh-float64(2*b.y1)
		// The front: an octagon for a shape drawn whole as one, else a rectangle.
		front := []r3vec{{X0, Yt, dep}, {X1, Yt, dep}, {X1, Yb, dep}, {X0, Yb, dep}}
		ow, oh := b.x1-b.x0, b.y1-b.y0
		if o.Kind == kOct && b.whole && ow >= 6 && oh >= 3 {
			cx, cy := 2.0, 4.0 // its corners, cut as the flat drawing cuts them
			if oh < octMinH || ow < octMinW {
				cx, cy = 1, 2
			}
			front = []r3vec{{X0 + cx, Yt, dep}, {X1 - cx, Yt, dep}, {X1, Yt - cy, dep}, {X1, Yb + cy, dep},
				{X1 - cx, Yb, dep}, {X0 + cx, Yb, dep}, {X0, Yb + cy, dep}, {X0, Yt - cy, dep}}
		}
		cells := r3face{own: o.ID, cx0: b.x0, cx1: b.x1, py0: 2 * b.y0, py1: 2 * b.y1}
		f := cells
		f.n, f.mat, f.gloss = r3vec{0, 0, 1}, matFront, 0.3
		r.addFace(f, front...)
		col := text
		if o.Color != "" {
			col = r.hex(o.Color)
		}
		mid := r3vec{(X0 + X1) / 2, (Yt + Yb) / 2, 0}
		for i := range front {
			a, b := front[i], front[(i+1)%len(front)]
			e := b.sub(a)
			n := r3vec{e.y, -e.x, 0}.norm()
			if n.dot(r3vec{a.x - mid.x, a.y - mid.y, 0}) < 0 {
				n = n.mul(-1)
			}
			a0 := r3vec{a.x, a.y, 0}
			f := cells
			f.n, f.mat, f.col, f.edge, f.gloss = n, matSolid, col, true, 0.35
			r.quadFace(f, a0, e, r3vec{0, 0, dep})
		}
	}

	// The lamp: a bulb under a shade, hung on a cord from above the walls.
	rb := math.Max(1, 0.012*rm.big)
	warm := hex32("#fff4d6")
	for _, sx := range []float64{-1, 1} {
		for _, sy := range []float64{-1, 1} {
			for _, sz := range []float64{-1, 1} {
				a, b, c := lamp.add(r3vec{sx * rb, 0, 0}), lamp.add(r3vec{0, sy * rb, 0}), lamp.add(r3vec{0, 0, sz * rb})
				r.addFace(r3face{n: r3vec{sx, sy, sz}.norm(), mat: matLamp, own: -1, col: warm}, a, b, c)
			}
		}
	}
	shade := mix32(accent, text, 0.15)
	b0, b1, tp := 2.1*rb, 0.8*rb, lamp.y+0.3*rb
	top := lamp.y + 2*rb
	corners := [4][2]float64{{-1, -1}, {1, -1}, {1, 1}, {-1, 1}}
	for i := range corners {
		c0, c1 := corners[i], corners[(i+1)%4]
		p := []r3vec{
			{lamp.x + c0[0]*b0, tp, lamp.z + c0[1]*b0}, {lamp.x + c1[0]*b0, tp, lamp.z + c1[1]*b0},
			{lamp.x + c1[0]*b1, top, lamp.z + c1[1]*b1}, {lamp.x + c0[0]*b1, top, lamp.z + c0[1]*b1},
		}
		n := p[1].sub(p[0]).cross(p[3].sub(p[0])).norm()
		if n.dot(p[0].sub(lamp)) < 0 {
			n = n.mul(-1) // outward
		}
		r.addFace(r3face{n: n, mat: matShade, own: -1, col: shade, both: true}, p...)
	}
	cw := math.Max(0.5, rb/4)
	cord := mix32(text, bg, 0.3)
	ceil := y1 + 0.125*tall
	for _, s := range []r3vec{{1, 0, 0}, {-1, 0, 0}, {0, 0, 1}, {0, 0, -1}} {
		side := r3vec{s.z, 0, -s.x} // along the face
		o := lamp.add(s.mul(cw)).sub(side.mul(cw))
		o.y = top
		r.quadFace(r3face{n: s, mat: matSolid, own: -1, col: cord}, o, side.mul(2*cw), r3vec{0, ceil - top, 0})
	}
}

// shadows works out where the blocks' shadows fall on the back wall, as
// seen from the lamp, softened at the edges: one value a unit of the wall
// (a cell is two of them, one above the other).
func (r *room3D) shadows(rm *r3room, lamp r3vec) {
	w, ph := rm.tw, 2*rm.th
	if len(r.shadow) != w*ph {
		r.shadow = make([]float32, w*ph)
	}
	clear(r.shadow)
	hw, hh := rm.hw, rm.hh
	var pts [8][2]float64
	for i := range r.faces {
		f := &r.faces[i]
		if f.mat != matFront {
			continue
		}
		// The shadow is the outline of the block's front, seen from the
		// lamp on the wall, joined to the block's own outline on it.
		dep := r.pts[f.p0].z
		if lamp.z-dep < 0.5 {
			continue
		}
		X0, X1 := float64(f.cx0)-hw, float64(f.cx1)-hw
		Yt, Yb := hh-float64(f.py0), hh-float64(f.py1)
		n := 0
		k := lamp.z / (lamp.z - dep)
		for _, c := range [4][2]float64{{X0, Yt}, {X1, Yt}, {X1, Yb}, {X0, Yb}} {
			pts[n] = [2]float64{c[0] - rm.x0, rm.y1 - c[1]}
			sx := lamp.x + (c[0]-lamp.x)*k
			sy := lamp.y + (c[1]-lamp.y)*k
			pts[n+4] = [2]float64{sx - rm.x0, rm.y1 - sy}
			n++
		}
		fillHull(pts[:], r.shadow, w, ph)
	}
	// Soften: a box blur three pixels wide, across and then down.
	tmp := make([]float32, max(w, ph))
	for y := 0; y < ph; y++ {
		row := r.shadow[y*w : y*w+w]
		for x := 0; x < w; x++ {
			a, c := row[max(x-1, 0)], row[min(x+1, w-1)]
			tmp[x] = (a + row[x] + c) / 3
		}
		copy(row, tmp[:w])
	}
	for x := 0; x < w; x++ {
		for y := 0; y < ph; y++ {
			a, c := r.shadow[max(y-1, 0)*w+x], r.shadow[min(y+1, ph-1)*w+x]
			tmp[y] = (a + r.shadow[y*w+x] + c) / 3
		}
		for y := 0; y < ph; y++ {
			r.shadow[y*w+x] = tmp[y]
		}
	}
}

// fillHull fills the smallest convex shape round pts with 1 in a w×h mask.
func fillHull(pts [][2]float64, mask []float32, w, h int) {
	// The hull, by the monotone chain.
	ps := make([][2]float64, len(pts))
	copy(ps, pts)
	for i := 1; i < len(ps); i++ { // insertion sort: eight points
		for j := i; j > 0 && (ps[j][0] < ps[j-1][0] || ps[j][0] == ps[j-1][0] && ps[j][1] < ps[j-1][1]); j-- {
			ps[j], ps[j-1] = ps[j-1], ps[j]
		}
	}
	cross := func(o, a, b [2]float64) float64 {
		return (a[0]-o[0])*(b[1]-o[1]) - (a[1]-o[1])*(b[0]-o[0])
	}
	hull := make([][2]float64, 0, 2*len(ps))
	for _, p := range ps {
		for len(hull) >= 2 && cross(hull[len(hull)-2], hull[len(hull)-1], p) <= 0 {
			hull = hull[:len(hull)-1]
		}
		hull = append(hull, p)
	}
	lo := len(hull) + 1
	for i := len(ps) - 2; i >= 0; i-- {
		p := ps[i]
		for len(hull) >= lo && cross(hull[len(hull)-2], hull[len(hull)-1], p) <= 0 {
			hull = hull[:len(hull)-1]
		}
		hull = append(hull, p)
	}
	hull = hull[:len(hull)-1]
	if len(hull) < 3 {
		return
	}
	spans(hull, w, h, func(y, x0, x1 int) {
		row := mask[y*w:]
		for x := x0; x <= x1; x++ {
			row[x] = 1
		}
	})
}

// spans calls fill with each row of pixels (whose middles are) inside the
// convex polygon poly, cut to a w×h screen: row y, columns x0 to x1.
func spans(poly [][2]float64, w, h int, fill func(y, x0, x1 int)) {
	ymin, ymax := math.Inf(1), math.Inf(-1)
	for _, p := range poly {
		ymin, ymax = math.Min(ymin, p[1]), math.Max(ymax, p[1])
	}
	ymin, ymax = clampF(ymin, -1, float64(h)+1), clampF(ymax, -1, float64(h)+1)
	j0, j1 := max(0, int(math.Ceil(ymin-0.5))), min(h-1, int(math.Ceil(ymax-0.5))-1)
	for j := j0; j <= j1; j++ {
		yc := float64(j) + 0.5
		xl, xr := math.Inf(1), math.Inf(-1)
		for i := range poly {
			a, b := poly[i], poly[(i+1)%len(poly)]
			if (a[1] <= yc && yc < b[1]) || (b[1] <= yc && yc < a[1]) {
				x := a[0] + (yc-a[1])*(b[0]-a[0])/(b[1]-a[1])
				xl, xr = math.Min(xl, x), math.Max(xr, x)
			}
		}
		if xl > xr {
			continue
		}
		xl, xr = clampF(xl, -1, float64(w)+1), clampF(xr, -1, float64(w)+1)
		i0, i1 := max(0, int(math.Ceil(xl-0.5))), min(w-1, int(math.Ceil(xr-0.5))-1)
		if i0 <= i1 {
			fill(j, i0, i1)
		}
	}
}

// ---- drawing ----------------------------------------------------------------------------

// r3light is the light: what every surface gets, what facing the viewer
// adds, the lamp's share, the lamp's own colour added where it shines
// (0–255), its shine on glossy surfaces, and how light the board is (0–1).
type r3light struct{ ambient, fill, lamp, glow, spec, lightness float64 }

// lightFor is the light for a theme: on a dark board the lamp can be
// strong and glow; on a light one (Daybreak) it is softer, or the room
// would be all white.
func lightFor(bg string) r3light {
	r, g, b := rgb(bg)
	t := clampF((0.3*float64(r)+0.59*float64(g)+0.11*float64(b))/255, 0, 1) // how light the board is
	mixF := func(dark, light float64) float64 { return dark + (light-dark)*t }
	return r3light{mixF(0.30, 0.42), mixF(0.22, 0.14), mixF(1.05, 0.55), mixF(38, 0), mixF(150, 70), t}
}

var lampWarm = [3]float64{1, 0.9, 0.72}

// orbitView is the camera of the turned room: it goes round the middle of
// the room (moved by the pan) at the room's turn and tilt.
func (r *room3D) orbitView(rm *r3room) r3view {
	fw, fh := float64(rm.w), float64(2*rm.h)
	depth := rm.depth
	dist0 := depth + 1.05*rm.big // the camera's distance from the back wall, straight on
	f := dist0                   // so straight on, the back wall is the board area cell for cell
	pivot := r3vec{0, 0, depth / 2}
	rad := dist0 - depth/2
	ya, pa := -float64(r.yaw)*math.Pi/180, float64(r.pitch)*math.Pi/180
	way := r3vec{math.Sin(ya) * math.Cos(pa), math.Sin(pa), math.Cos(ya) * math.Cos(pa)}
	cam := pivot.add(way.mul(rad))
	fwd := pivot.sub(cam).norm()
	right := fwd.cross(r3vec{0, 1, 0}).norm()
	up := right.cross(fwd)
	angle := math.Max(math.Abs(float64(r.yaw)), math.Abs(float64(r.pitch)))
	scale, ox, oy := fitRoom(cam, right, up, fwd, f, fw/2, fh/2, rm, angle)
	// The camera closer or further (+ and -) and moved sideways (a drag with the
	// middle or right button), after the fitting, which would undo the move.
	cam = pivot.add(r.pan).add(way.mul(rad * math.Pow(0.87, float64(r.near))))
	return r3view{cam: cam, right: right, up: up, fwd: fwd, f: f * scale, px: fw/2 + ox, py: fh/2 + oy}
}

// freeView is the camera of free roam: where it is, looking that way.
func (r *room3D) freeView(rm *r3room) r3view {
	ly, lp := r.look[0]*math.Pi/180, r.look[1]*math.Pi/180
	fwd := r3vec{math.Sin(ly) * math.Cos(lp), -math.Sin(lp), -math.Cos(ly) * math.Cos(lp)}
	right := fwd.cross(r3vec{0, 1, 0}).norm()
	up := right.cross(fwd)
	return r3view{cam: r.pos, right: right, up: up, fwd: fwd, f: r.ff, px: float64(rm.w) / 2, py: float64(rm.h)}
}

// fitRoom is how much smaller than the board the view is drawn, and how
// far its middle is moved (ox, oy units), so the room fits on the screen:
// close to straight on, the back wall is the board cell for cell (as flat);
// the further it is turned, the more the whole room is fitted and centred,
// fully from 30° on.
func fitRoom(cam, right, up, fwd r3vec, f, hw, hh float64, rm *r3room, angle float64) (s, ox, oy float64) {
	x0, x1, y0, y1 := math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
	for _, p := range []r3vec{{rm.x0, rm.y0, 0}, {rm.x1, rm.y0, 0}, {rm.x1, rm.y1, 0}, {rm.x0, rm.y1, 0},
		{rm.x0, rm.y0, rm.depth}, {rm.x1, rm.y0, rm.depth}, {rm.x1, rm.y1, rm.depth}, {rm.x0, rm.y1, rm.depth}} {
		q := p.sub(cam)
		x, y, z := q.dot(right), q.dot(up), q.dot(fwd)
		if z < 1 {
			continue
		}
		x0, x1 = math.Min(x0, f*x/z), math.Max(x1, f*x/z)
		y0, y1 = math.Min(y0, -f*y/z), math.Max(y1, -f*y/z)
	}
	if x1 <= x0 || y1 <= y0 {
		return 1, 0, 0
	}
	room := math.Min(2.5, 0.96*math.Min(2*hw/(x1-x0), 2*hh/(y1-y0))) // a small room is drawn bigger, up to 2.5 times
	a := math.Min(1, angle/30)
	a *= a
	return (1 - a) + room*a, -a * room * (x0 + x1) / 2, -a * room * (y0 + y1) / 2
}

// floorAt is the floor's colour at (x, z): planks running out from the
// wall, each a slightly different shade, with dark joins.
func floorAt(base uint32, x, z float64) uint32 {
	const plankW, plankL = 7.0, 34.0
	mod := func(a, b float64) float64 { return a - b*math.Floor(a/b) } // from 0 up to b, below 0 too
	p := math.Floor(x / plankW)
	along := z + mod(p*13, plankL)
	hash := mod(p*7.31+math.Floor(along/plankL)*3.17, 1)
	col := mix32(base, 0, 0.04+0.07*hash)
	if x-p*plankW < 0.45 || mod(along, plankL) < 0.6 {
		col = mix32(col, 0, 0.2)
	}
	return col
}

// draw draws the room into sb: rm.h lines of rm.w cells.
func (r *room3D) draw(rm *r3room, z float64, sb *strings.Builder) {
	w, h := rm.w, rm.h
	pw, ph := 2*w, 2*h
	if r.pw != pw || r.ph != ph || len(r.base) != pw*ph {
		r.pw, r.ph = pw, ph
		n := pw * ph
		r.base, r.mul, r.add = make([]uint32, n), make([]float32, n), make([]float32, n)
		r.zb, r.face, r.src, r.dirs = make([]float32, n), make([]int32, n), make([]int32, n), make([]r3vec, n)
		r.glyph = make([]int32, w*h)
	}
	r.stamp++
	var v r3view
	if r.free {
		v = r.freeView(rm)
	} else {
		v = r.orbitView(rm)
	}
	r.view, r.rm = v, *rm
	cam, right, up, fwd, f, px, py := v.cam, v.right, v.up, v.fwd, v.f, v.px, v.py
	lamp := lampAt(rm)
	invR2 := 1 / (1.15 * rm.big * 1.15 * rm.big)
	light := lightFor(cBG)

	r.scene(rm, z, lamp)
	r.shadows(rm, lamp)

	// A ray for every pixel (two across a cell, two down it), and a clean screen.
	outside := mix32(hex32(cBG), 0, 0.55-0.37*light.lightness) // round the room: darker than the board
	for j := 0; j < ph; j++ {
		dy := -(float64(j) + 0.5 - py) / f
		row := j * pw
		for i := 0; i < pw; i++ {
			dx := ((float64(i)+0.5)/2 - px) / f
			r.dirs[row+i] = r3vec{fwd.x + right.x*dx + up.x*dy, fwd.y + right.y*dx + up.y*dy, fwd.z + right.z*dx + up.z*dy}
		}
	}
	for i := range r.base {
		r.base[i], r.mul[i], r.add[i] = outside, 1, 0
		r.zb[i], r.face[i], r.src[i] = math.MaxFloat32, -1, -1
	}
	for i := range r.glyph {
		r.glyph[i] = -1
	}

	const near = 0.25 // faces closer to the camera than this are cut off
	hw, hh := rm.hw, rm.hh
	tw, tx0, ty0 := rm.tw, rm.tx0, rm.ty0
	var cs, clip [20]r3vec
	var poly [20][2]float64
	for fi := range r.faces {
		fc := &r.faces[fi]
		fc.seen = false
		n, k := fc.n, fc.k
		inside := false // seen from behind (the shade's inside)
		if n.dot(cam)-k <= 1e-9 {
			if !fc.both {
				continue // it faces away
			}
			n, k, inside = n.mul(-1), -k, true
		}
		// Its corners as the camera sees them, cut off in front of the camera.
		pts := r.pts[fc.p0 : fc.p0+fc.pn]
		for i, p := range pts {
			q := p.sub(cam)
			cs[i] = r3vec{q.dot(right), q.dot(up), q.dot(fwd)}
		}
		m := 0
		for i := range pts {
			a, b := cs[i], cs[(i+1)%len(pts)]
			if a.z >= near {
				clip[m] = a
				m++
			}
			if (a.z >= near) != (b.z >= near) {
				t := (near - a.z) / (b.z - a.z)
				clip[m] = a.add(b.sub(a).mul(t))
				m++
			}
		}
		if m < 3 {
			continue
		}
		for i := 0; i < m; i++ {
			poly[i] = [2]float64{2 * (px + f*clip[i].x/clip[i].z), py - f*clip[i].y/clip[i].z}
		}
		fc.seen = true
		num := k - n.dot(cam)
		fid := int32(fi)
		spans(poly[:m], pw, ph, func(j, i0, i1 int) {
			row := j * pw
			for i := i0; i <= i1; i++ {
				idx := row + i
				dir := r.dirs[idx]
				nd := n.x*dir.x + n.y*dir.y + n.z*dir.z
				if nd >= -1e-12 {
					continue
				}
				t := num / nd
				if t <= 0 || float32(t) >= r.zb[idx] {
					continue
				}
				p := r3vec{cam.x + dir.x*t, cam.y + dir.y*t, cam.z + dir.z*t}
				src := int32(-1)
				var col uint32
				shade := float32(1)
				switch fc.mat {
				case matBoard, matFront:
					fx := p.x + hw
					cx := max(fc.cx0, min(fc.cx1-1, int(math.Floor(fx))))
					hy := max(fc.py0, min(fc.py1-1, int(math.Floor(hh-p.y))))
					ti := (hy>>1-ty0)*tw + cx - tx0
					src = int32(ti)
					q := (hy & 1) << 1
					if fx-float64(cx) >= 0.5 {
						q++
					}
					col = r.tex[ti].q[q]
					if fc.mat == matBoard {
						shade = 1 - 0.7*r.shadow[(hy-2*ty0)*tw+cx-tx0]
					}
				case matSolid:
					col = fc.col
					if fc.edge {
						q := p.sub(fc.o)
						u, v := q.dot(fc.eu), q.dot(fc.ev)
						e := math.Min(0.45, 0.25*math.Min(fc.lu, fc.lv))
						if u < e || v < e || u > fc.lu-e || v > fc.lv-e {
							col = mix32(col, 0, 0.3) // its edges, a little darker
						}
					}
				case matFloor:
					col = floorAt(fc.col, p.x+hw, p.z)
				case matWall:
					col = fc.col
					if p.y < rm.y0+3 {
						col = mix32(col, hex32(cText), 0.12) // the skirting board
					}
				case matLamp, matShade:
					col = fc.col
				}
				r.zb[idx], r.face[idx], r.src[idx], r.base[idx] = float32(t), fid, src, col

				// The light here.
				dl := 1 / math.Sqrt(dir.x*dir.x+dir.y*dir.y+dir.z*dir.z)
				vx, vy, vz := -dir.x*dl, -dir.y*dl, -dir.z*dl
				fill := n.x*vx + n.y*vy + n.z*vz
				if fc.mat == matLamp {
					r.mul[idx], r.add[idx] = float32(0.85+0.15*fill), 0
					continue
				}
				lx, ly, lz := lamp.x-p.x, lamp.y-p.y, lamp.z-p.z
				d2 := lx*lx + ly*ly + lz*lz
				il := 1 / math.Sqrt(d2)
				lx, ly, lz = lx*il, ly*il, lz*il
				att := 1 / (1 + d2*invR2)
				ndl := n.x*lx + n.y*ly + n.z*lz
				if fc.mat == matShade && inside {
					ndl = 1 // the shade's inside is lit by the bulb right next to it
				}
				diff, spec := 0.0, 0.0
				if ndl > 0 {
					diff = ndl * att * float64(shade)
					hx, hy, hz := lx+vx, ly+vy, lz+vz
					hn := (n.x*hx + n.y*hy + n.z*hz) / math.Sqrt(hx*hx+hy*hy+hz*hz)
					if hn > 0 {
						s := hn * hn
						s *= s
						s *= s
						s *= s
						s *= s // to the 32nd
						spec = s * att * fc.gloss * float64(shade)
					}
				}
				r.mul[idx] = float32(light.ambient + light.fill*fill + light.lamp*diff)
				r.add[idx] = float32(light.glow*diff + light.spec*spec)
			}
		})
	}

	r.letters(&v, rm)
	r.write(w, h, sb)
}

// letters puts the flat board's letters on the screen where the wall and
// the blocks' fronts are seen about one cell for one: there the text
// reads as on the flat board. Elsewhere the cells are only their colours.
//
// Each board cell goes to the screen cell its middle is seen in. Drawn a
// little bigger or smaller than the board, a word would get a gap or lose
// a letter, so a word's letters go side by side on one screen row, centred
// on where it is seen, and a word shows whole or not at all (only what is
// off the screen is left out). Spaces take their screen cells too, so
// where two rows would fall on one screen row, the second does not show.
func (r *room3D) letters(v *r3view, rm *r3room) {
	pw := r.pw
	w, h := rm.w, rm.h
	hw, hh := rm.hw, rm.hh
	tw, tx0, ty0 := rm.tw, rm.tx0, rm.ty0
	if len(r.claim) != w*h {
		r.claim = make([]int32, w*h)
	}
	for i := range r.claim {
		r.claim[i] = -1
	}
	cam, right, up, fwd, f, px, py := v.cam, v.right, v.up, v.fwd, v.f, v.px, v.py
	if -fwd.z < math.Cos(60*math.Pi/180) {
		return // turned more than 60° from the wall: no cell is seen one for one
	}
	// A step of one cell right, and one down, as the camera sees it.
	dR := r3vec{right.x, up.x, fwd.x}
	dD := r3vec{-2 * right.y, -2 * up.y, -2 * fwd.y}
	// target is the screen cell where the middle of board cell (cx, cy) on
	// a face at depth z is seen, and how: readOK when it is seen there
	// about one for one, readOff when it would be but is off the screen.
	target := func(cx, cy int, z float64) (int, int) {
		q := r3vec{float64(cx) + 0.5 - hw, hh - float64(2*cy+1), z}.sub(cam)
		c := r3vec{q.dot(right), q.dot(up), q.dot(fwd)}
		cr, cd := c.add(dR), c.add(dD)
		if c.z < 1 || cr.z < 1 || cd.z < 1 {
			return 0, readBad
		}
		sx, sy := px+f*c.x/c.z, py-f*c.y/c.z
		rx, ry := px+f*cr.x/cr.z-sx, py-f*cr.y/cr.z-sy
		dx, dy := px+f*cd.x/cd.z-sx, py-f*cd.y/cd.z-sy
		if rx < 0.75 || rx > 1.6 || math.Abs(ry) > 0.4 || dy < 1.5 || dy > 3.2 || math.Abs(dx) > 0.4 {
			return 0, readBad // turned too far, or too small or big, to read
		}
		col, row := int(math.Floor(sx)), int(math.Floor(sy/2))
		if col < 0 || col >= w || row < 0 || row >= h {
			return 0, readOff
		}
		return row*w + col, readOK
	}
	// free: screen cell at shows face fid in all four of its pixels, and
	// nothing has taken it yet.
	free := func(at int, fid int32) bool {
		tl := at/w*2*pw + 2*(at%w)
		return r.claim[at] < 0 && r.face[tl] == fid && r.face[tl+1] == fid && r.face[tl+pw] == fid && r.face[tl+pw+1] == fid
	}
	tex := func(cx, cy int) *r3texel { return &r.tex[(cy-ty0)*tw+cx-tx0] }
	type letter struct {
		at int   // the screen cell
		s  int32 // the wall cell
	}
	var word []letter
	for fi := range r.faces {
		fc := &r.faces[fi]
		if !fc.seen || (fc.mat != matBoard && fc.mat != matFront) {
			continue
		}
		z := r.pts[fc.p0].z
		fid := int32(fi)
		for cy := fc.py0 >> 1; cy < fc.py1>>1; cy++ {
			if fc.mat == matFront && !r.rowReads(target, tex, fc, cy, z) {
				continue // a line of a structure shows whole or not at all
			}
			lineRow := -1 // the screen row the line's first word went on
			for cx := fc.cx0; cx < fc.cx1; {
				s := int32((cy-ty0)*tw + cx - tx0)
				if tex(cx, cy).glyph == "" { // a space: it takes its cell
					at, how := target(cx, cy, z)
					ok := how == readOK
					if ok && cx > fc.cx0 && r.claim[at] == s-1 && (at+1)/w == at/w {
						at++ // drawn a little smaller: the cell before it took its cell
					}
					if ok && free(at, fid) {
						r.claim[at] = s
						if tex(cx, cy).blank {
							r.glyph[at] = s // a clean space between the words
						}
					}
					cx++
					continue
				}
				end := cx
				for end < fc.cx1 && tex(end, cy).glyph != "" {
					end++
				}
				// The word goes on one screen row, its letters side by side,
				// centred on where its first and last letters are seen.
				word = word[:0]
				ok := true
				for x := cx; x < end && ok; x++ {
					at, how := target(x, cy, z)
					if how != readOff { // off the screen: the rest of the word still shows
						ok = how == readOK
						word = append(word, letter{at, s + int32(x-cx)})
					}
				}
				if n := len(word); ok && n > 0 {
					row := word[n/2].at / w
					if lineRow >= 0 && abs(row-lineRow) <= 1 {
						row = lineRow // the words of a line stay on one screen row
					}
					start := int(math.Round(float64(word[0].at%w+word[n-1].at%w)/2 - float64(n-1)/2))
					if at := row*w + start; start >= 0 && start < w && r.claim[at] >= 0 && r.claim[at] == word[0].s-1 {
						start++ // drawn a little smaller: the cell before it took its cell
					}
					if at := row*w + start - 1; start >= 1 && start <= w && r.glyph[at] >= 0 && r.tex[r.glyph[at]].glyph != "" {
						start++ // straight after another word: a cell further, for the space between them
					}
					for i := range word {
						col, l := start+i, &word[i]
						if col < 0 || col >= w || abs(l.at%w-col) > 2 || abs(l.at/w-row) > 1 || !free(row*w+col, fid) {
							ok = false // not about one cell for one here, or its cells are taken
							break
						}
						l.at = row*w + col
					}
				}
				if ok && len(word) > 0 {
					for _, l := range word {
						r.claim[l.at], r.glyph[l.at], r.placed[l.s] = l.s, l.s, r.stamp
					}
					lineRow = word[0].at / w
				}
				cx = end
			}
		}
	}
}

// How a letter of the board is seen (letters' target).
const (
	readOK  = iota // about one cell for one, on the screen
	readOff        // so, but off the screen
	readBad        // turned too far, or too small or big, to read
)

// rowReads is whether every letter of row cy of face fc is seen about one
// cell for one (on the screen or just off it).
func (r *room3D) rowReads(target func(cx, cy int, z float64) (int, int), tex func(cx, cy int) *r3texel, fc *r3face, cy int, z float64) bool {
	for cx := fc.cx0; cx < fc.cx1; cx++ {
		if tex(cx, cy).glyph != "" {
			if _, how := target(cx, cy, z); how == readBad {
				return false
			}
		}
	}
	return true
}

// pixel is a pixel's colour, lit.
func (r *room3D) pixel(i int) uint32 {
	col := r.base[i]
	if s := r.src[i]; s >= 0 && r.placed[s] == r.stamp {
		col = r.tex[s].bg // its letter shows in a cell of its own: here only its background
	}
	return r.lit(col, i)
}

// lit is colour col in the light of pixel i.
func (r *room3D) lit(col uint32, i int) uint32 {
	m, a := float64(r.mul[i]), float64(r.add[i])
	cr, cg, cb := chans(col)
	return rgb32(byteOf(cr*m+a*lampWarm[0]), byteOf(cg*m+a*lampWarm[1]), byteOf(cb*m+a*lampWarm[2]))
}

// byteOf is v kept within 0 to 255 (quicker than clampF: no checks for NaN).
func byteOf(v float64) float64 {
	if v > 255 {
		return 255
	}
	if v < 0 {
		return 0
	}
	return v
}

// quads splits a cell's four pixels (top left, top right, bottom left,
// bottom right) into two colours, the way that is closest to them: mask
// is the pixels in colour fg (as quadGlyph draws them), the rest are bg.
// All four alike, the mask is 0.
func quads(c *[4]uint32) (mask int, fg, bg uint32) {
	if c[0] == c[1] && c[1] == c[2] && c[2] == c[3] {
		return 0, c[0], c[0]
	}
	var cr, cg, cb [4]int
	tr, tg, tb := 0, 0, 0
	for i, v := range c {
		cr[i], cg[i], cb[i] = int(v>>16&255), int(v>>8&255), int(v&255)
		tr, tg, tb = tr+cr[i], tg+cg[i], tb+cb[i]
	}
	// The best split leaves the least error, which is the one where the
	// sums of the two groups, squared and shared out, are the largest.
	best, bestScore := 1, -1
	var bs [3]int
	var bn int
	for m := 1; m < 8; m++ { // each split once: the side without the bottom right pixel
		sr, sg, sb, n := 0, 0, 0, 0
		for i := range 4 {
			if m&(1<<i) != 0 {
				sr, sg, sb, n = sr+cr[i], sg+cg[i], sb+cb[i], n+1
			}
		}
		rr, rg, rb := tr-sr, tg-sg, tb-sb
		score := (sr*sr+sg*sg+sb*sb)*12/n + (rr*rr+rg*rg+rb*rb)*12/(4-n) // 12: whole numbers for 1, 2 and 3
		if score > bestScore {
			best, bestScore, bs, bn = m, score, [3]int{sr, sg, sb}, n
		}
	}
	rn := 4 - bn
	fg = uint32((bs[0]+bn/2)/bn)<<16 | uint32((bs[1]+bn/2)/bn)<<8 | uint32((bs[2]+bn/2)/bn)
	bg = uint32((tr-bs[0]+rn/2)/rn)<<16 | uint32((tg-bs[1]+rn/2)/rn)<<8 | uint32((tb-bs[2]+rn/2)/rn)
	return best, fg, bg
}

// write turns the pixels into h lines of w cells, four pixels a cell.
func (r *room3D) write(w, h int, sb *strings.Builder) {
	pw := r.pw
	buf := make([]byte, 0, w*40)
	colour := func(buf []byte, lead string, c uint32) []byte {
		buf = append(buf, lead...)
		buf = strconv.AppendUint(buf, uint64(c>>16&255), 10)
		buf = append(buf, ';')
		buf = strconv.AppendUint(buf, uint64(c>>8&255), 10)
		buf = append(buf, ';')
		buf = strconv.AppendUint(buf, uint64(c&255), 10)
		return append(buf, 'm')
	}
	var px [4]uint32
	for row := 0; row < h; row++ {
		buf = buf[:0]
		fg, bg := int64(-1), int64(-1)
		for col := 0; col < w; col++ {
			tl := 2*row*pw + 2*col
			var top, bot uint32
			var glyph string
			if s := r.glyph[row*w+col]; s >= 0 {
				t := &r.tex[s]
				top, bot, glyph = r.lit(t.fg, tl), r.lit(t.bg, tl), t.glyph
				if glyph == "" {
					glyph = " " // a space of the board
				}
			} else {
				px[0], px[1], px[2], px[3] = r.pixel(tl), r.pixel(tl+1), r.pixel(tl+pw), r.pixel(tl+pw+1)
				mask, a, b := quads(&px)
				changes := func(f, g uint32) int { // how many colours would have to be set
					n := 0
					if int64(f) != fg {
						n++
					}
					if int64(g) != bg {
						n++
					}
					return n
				}
				switch {
				case mask == 0:
					glyph, bot = " ", a
				case changes(b, a) < changes(a, b):
					glyph, top, bot = quadGlyph[mask^15], b, a // the other way round keeps the colours already set
				default:
					glyph, top, bot = quadGlyph[mask], a, b
				}
			}
			if glyph != " " && int64(top) != fg {
				buf = colour(buf, "\x1b[38;2;", top)
				fg = int64(top)
			}
			if int64(bot) != bg {
				buf = colour(buf, "\x1b[48;2;", bot)
				bg = int64(bot)
			}
			buf = append(buf, glyph...)
		}
		buf = append(buf, "\x1b[m\n"...)
		sb.Write(buf)
	}
}
