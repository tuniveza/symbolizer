package main

// ---- drawing the room in 3D -----------------------------------------------------------
//
// A small renderer of its own: every pixel finds the nearest surface (a
// depth buffer), which is lit there. A cell of the screen is four pixels,
// two across and two down, drawn with one of the quarter-block letters
// (▘ ▝ ▖ ▗ ▀ ▄ ▌ ▐ ▚ ▞ ▙ ▛ ▜ ▟ █ or a space): of the ways to split the four
// into two colours, the one closest to them is taken; four colours close
// together are one plain cell, so lit surfaces stay smooth. The faces show
// the flat board's cells by their background (and box lines as lines),
// and every word is drawn over them in the terminal's own letters, upright
// and crisp, wherever it is seen (see words). In kitty the pixels are a
// real picture instead, as many as the terminal's own (see
// design_3d_kitty.go): the same drawing, with more pixels to a cell.
//
// The world's units: one is a cell's width, and a cell is two units tall.
// x goes right, y up, and z out of the back wall toward the viewer. The
// back wall is the flat board, and the screen's board area is centred on
// (0, 0, 0): column sx of the flat board is x = sx − hw, row sy is
// y = hh − 2·sy (its top edge).

import (
	"image/color"
	"math"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"
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
// left, top right, bottom left, bottom right) as the faces show it, its
// colours, and its letter when it is a word's letter (drawn as a letter
// of the terminal, see words), else "".
type r3texel struct {
	q      [4]uint32
	fg, bg uint32
	glyph  string
	wide   bool // the letter is two cells wide
	blank  bool // a space
	arms   int8 // a box line: its arms (see boxArms)
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
	switch {
	case s == "" || s == " ":
		all(t.bg)
		t.blank = true
	case s == "▒":
		all(mix32(t.bg, t.fg, 0.5))
	case s == "░":
		all(mix32(t.bg, t.fg, 0.25))
	case s == "▓":
		all(mix32(t.bg, t.fg, 0.75))
	case ru >= 0x2500 && ru <= 0x257f && size == len(s):
		// A box line, the edge of a thing: each quarter is tinted by the
		// arms of the line that touch it, so a corner stays a corner.
		arms := boxArms(ru)
		t.arms = int8(arms)
		if arms == 0 {
			all(mix32(t.bg, t.fg, 0.55))
			break
		}
		touch := [4]int{1 | 4, 1 | 8, 2 | 4, 2 | 8} // up and left touch the top left quarter, and so on
		for i, m := range touch {
			n := 0
			for b := arms & m; b != 0; b &= b - 1 {
				n++
			}
			t.q[i] = mix32(t.bg, t.fg, 0.45*float64(n))
		}
	case ru >= 0x2580 && ru <= 0x259f && ru != '▏' && size == len(s):
		all(mix32(t.bg, t.fg, 0.45)) // the other blocks (▁ ▂ … ▉ ▊ …): about half the cell
	case ru >= 0x2800 && ru <= 0x28ff && size == len(s):
		all(mix32(t.bg, t.fg, 0.4)) // braille dots: a picture
	default:
		// A letter (or the caret ▏): the face shows only its background,
		// and the letter itself is drawn over it as a letter.
		all(t.bg)
		switch sw := ansi.StringWidth(s); {
		case sw == 1:
			t.glyph = s
		case sw == 2 && cell.Width == 2:
			t.glyph, t.wide = s, true
		default:
			all(mix32(t.bg, t.fg, 0.35)) // nothing a cell can hold: its tint
		}
	}
	return t
}

// lineCover is how much of a pixel at (u, v) in a cell (0 to 1 across
// and down), px of a cell wide and tall, a box line with those arms
// covers: the line about a pixel thick (never thinner than a tenth of the
// cell), its arms running to the cell's edges.
func lineCover(arms int, u, v, px float64) float64 {
	return lineCoverUV(arms, u, v, px, px, 0.06, 0.06)
}

// lineCoverUV is lineCover for a pixel pu of a cell wide and pv of it
// tall, the line's half thickness at least minU of the cell across (a
// line up and down) and minV down (a line across).
func lineCoverUV(arms int, u, v, pu, pv, minU, minV float64) float64 {
	hu, hv := math.Max(minU, 0.5*pu), math.Max(minV, 0.5*pv) // half the lines' thickness
	over := func(c, lo, hi, px float64) float64 {            // how much of [c - px/2, c + px/2] is in [lo, hi]
		return clampF((math.Min(c+px/2, hi)-math.Max(c-px/2, lo))/px, 0, 1)
	}
	span := func(a, b int, hw float64) (lo, hi float64) { // the arms a (to 0) and b (to 1) along one way
		lo, hi = 0.5-hw, 0.5+hw
		if arms&a != 0 {
			lo = -1
		}
		if arms&b != 0 {
			hi = 2
		}
		return lo, hi
	}
	cov := 0.0
	if arms&(4|8) != 0 { // across
		lo, hi := span(4, 8, hu)
		cov = over(u, lo, hi, pu) * over(v, 0.5-hv, 0.5+hv, pv)
	}
	if arms&(1|2) != 0 { // up and down
		lo, hi := span(1, 2, hv)
		cov = math.Max(cov, over(v, lo, hi, pv)*over(u, 0.5-hu, 0.5+hu, pu))
	}
	return cov
}

// boxArms is which arms a box line letter (U+2500 to U+257F) has: 1 up,
// 2 down, 4 left, 8 right (0: not known, a diagonal).
func boxArms(r rune) int { return int(boxArmTable[r-0x2500]) }

var boxArmTable = func() (t [128]int8) {
	for _, a := range boxArmSets {
		for _, r := range a.s {
			t[r-0x2500] = int8(a.arms)
		}
	}
	return t
}()

var boxArmSets = []struct {
	s    string
	arms int
}{
	{"─━═╌╍┄┅┈┉╼╾", 4 | 8}, {"│┃║╎╏┆┇┊┋╽╿", 1 | 2},
	{"┌┍┎┏╒╓╔╭", 2 | 8}, {"┐┑┒┓╕╖╗╮", 2 | 4}, {"└┕┖┗╘╙╚╰", 1 | 8}, {"┘┙┚┛╛╜╝╯", 1 | 4},
	{"├┝┞┟┠┡┢┣╞╟╠", 1 | 2 | 8}, {"┤┥┦┧┨┩┪┫╡╢╣", 1 | 2 | 4},
	{"┬┭┮┯┰┱┲┳╤╥╦", 2 | 4 | 8}, {"┴┵┶┷┸┹┺┻╧╨╩", 1 | 4 | 8},
	{"┼┽┾┿╀╁╂╃╄╅╆╇╈╉╊╋╪╫╬", 15},
	{"╴╸", 4}, {"╵╹", 1}, {"╶╺", 8}, {"╷╻", 2},
}

// takeRoom reads the back wall's cells: the flat board c where it is on
// the screen, and beyond the screen the drawings of the structures that
// stick out of it (on the board's own colour).
func (r *room3D) takeRoom(d *designState, c *boardCanvas, rm *r3room) {
	n := rm.tw * rm.th
	if len(r.tex) != n {
		r.tex = make([]r3texel, n)
		r.cover = make([]int32, n)
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

// mipmap makes the back wall's pictures: the first is its pixels (four a
// cell, as the faces show them), and each after it is half as big each
// way, a pixel the mean of four. A face seen small takes its colours from
// the picture whose pixels are about as big as the screen's, so its
// lines and letters' backgrounds blend smoothly instead of flickering
// into noise.
func (r *room3D) mipmap(rm *r3room) {
	w, h := 2*rm.tw, 2*rm.th
	if len(r.mips) == 0 || r.mipW[0] != w || r.mipH[0] != h {
		r.mips, r.mipW, r.mipH = r.mips[:0], r.mipW[:0], r.mipH[:0]
		for lw, lh := w, h; len(r.mips) < 6; lw, lh = (lw+1)/2, (lh+1)/2 {
			r.mips, r.mipW, r.mipH = append(r.mips, make([]uint32, lw*lh)), append(r.mipW, lw), append(r.mipH, lh)
			if lw == 1 && lh == 1 {
				break
			}
		}
	}
	l0 := r.mips[0]
	for cy := 0; cy < rm.th; cy++ {
		for cx := 0; cx < rm.tw; cx++ {
			q := &r.tex[cy*rm.tw+cx].q
			i := 2*cy*w + 2*cx
			l0[i], l0[i+1], l0[i+w], l0[i+w+1] = q[0], q[1], q[2], q[3]
		}
	}
	for k := 1; k < len(r.mips); k++ {
		src, sw, sh := r.mips[k-1], r.mipW[k-1], r.mipH[k-1]
		dst, dw := r.mips[k], r.mipW[k]
		for y := 0; y < r.mipH[k]; y++ {
			y0, y1 := 2*y*sw, min(2*y+1, sh-1)*sw
			for x := 0; x < dw; x++ {
				x0, x1 := 2*x, min(2*x+1, sw-1)
				a, b, c, d := src[y0+x0], src[y0+x1], src[y1+x0], src[y1+x1]
				red := (a>>16&255 + b>>16&255 + c>>16&255 + d>>16&255 + 2) / 4
				green := (a>>8&255 + b>>8&255 + c>>8&255 + d>>8&255 + 2) / 4
				blue := (a&255 + b&255 + c&255 + d&255 + 2) / 4
				dst[y*dw+x] = red<<16 | green<<8 | blue
			}
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

// shadowAt is the shadow at (u, v) on the back wall (u units from its
// left edge, v from its top), blended between the four nearest values so
// its edges are smooth, not steps.
func (r *room3D) shadowAt(u, v float64) float32 {
	w, ph := r.rm.tw, 2*r.rm.th
	u, v = clampF(u-0.5, 0, float64(w-1)), clampF(v-0.5, 0, float64(ph-1))
	i, j := int(u), int(v)
	i1, j1 := min(i+1, w-1), min(j+1, ph-1)
	fu, fv := float32(u-float64(i)), float32(v-float64(j))
	s := r.shadow
	top := s[j*w+i] + (s[j*w+i1]-s[j*w+i])*fu
	bot := s[j1*w+i] + (s[j1*w+i1]-s[j1*w+i])*fu
	return top + (bot-top)*fv
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
	spansIn(poly, w, 0, h, fill)
}

// spansIn is spans for rows y0 ≤ y < y1 of the screen only.
func spansIn(poly [][2]float64, w, y0, y1 int, fill func(y, x0, x1 int)) {
	ymin, ymax := math.Inf(1), math.Inf(-1)
	for _, p := range poly {
		ymin, ymax = math.Min(ymin, p[1]), math.Max(ymax, p[1])
	}
	ymin, ymax = clampF(ymin, float64(y0)-1, float64(y1)+1), clampF(ymax, float64(y0)-1, float64(y1)+1)
	j0, j1 := max(y0, int(math.Ceil(ymin-0.5))), min(y1-1, int(math.Ceil(ymax-0.5))-1)
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

// floorAt is the floor's colour at (x, z), seen with px units to a pixel:
// planks running out from the wall, each a shade apart, with soft joins.
// Far away, where a join would be thinner than a pixel and only flicker,
// the joins fade out.
func floorAt(base uint32, x, z, px float64) uint32 {
	const plankW, plankL = 7.0, 34.0
	mod := func(a, b float64) float64 { return a - b*math.Floor(a/b) } // from 0 up to b, below 0 too
	p := math.Floor(x / plankW)
	along := z + mod(p*13, plankL)
	hash := mod(p*7.31+math.Floor(along/plankL)*3.17, 1) // each length of plank its own shade
	dark := 0.04 + 0.03*hash
	// The join between two planks (the lines running out from the wall),
	// blurred to a pixel's width: 1 on it, 0 away from it.
	d := math.Min(x-p*plankW, plankW-(x-p*plankW))
	j := clampF(1-(d-0.25)/math.Max(px, 0.3), 0, 1)
	dark += 0.1 * j * clampF(1.6-px, 0, 1)
	return mix32(base, 0, dark)
}

// r3draw is how a face is drawn in this frame: the side of it the camera
// sees (its plane n · p = k, num = k − n · camera; inside: seen from
// behind), and its outline on the screen in pixels, polys[p0 : p0+pn].
type r3draw struct {
	n       r3vec
	num     float64
	inside  bool
	p0, pn  int
	top, bo float64 // the outline's top and bottom, pixels
}

// r3shade is what lighting a pixel needs, the same for all of a frame.
type r3shade struct {
	cam, lamp      r3vec
	f, invR2       float64
	light          r3light
	hw, hh         float64
	tx0, ty0       int
	kx, ky, kpx    float64 // the pixels a unit across and down, and how big a wall pixel is to a screen pixel at f (see draw)
	minU, minV     float64 // a box line's least half thickness, across and down (see lineCoverUV)
	rm             *r3room
	textCol, outer uint32
}

// draw draws the room into sb: rm.h lines of rm.w cells. The screen is
// r.pw × r.ph pixels, kx a unit across and ky a unit down: r.want of them
// as a picture (kitty, see design_3d_kitty.go), or else four a cell, two
// across and two down, drawn as quarter blocks.
func (r *room3D) draw(rm *r3room, z float64, sb *strings.Builder) {
	w, h := rm.w, rm.h
	pw, ph := 2*w, 2*h
	r.pix = r.want[0] > 0 && r.want[1] > 0
	if r.pix {
		pw, ph = r.want[0], r.want[1]
	}
	if r.pw != pw || r.ph != ph || len(r.base) != pw*ph {
		r.pw, r.ph = pw, ph
		n := pw * ph
		r.base, r.mul, r.add = make([]uint32, n), make([]float32, n), make([]float32, n)
		r.zb, r.face, r.dirs = make([]float32, n), make([]int32, n), make([]r3vec, n)
	}
	r.kx, r.ky = float64(pw)/float64(w), float64(ph)/float64(2*h)
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
	light := lightFor(cBG)

	r.scene(rm, z, lamp)
	r.shadows(rm, lamp)

	// Each face the camera sees, as an outline on the screen.
	const near = 0.25 // faces closer to the camera than this are cut off
	kx, ky := r.kx, r.ky
	if cap(r.fdraw) < len(r.faces) {
		r.fdraw = make([]r3draw, len(r.faces))
	}
	r.fdraw = r.fdraw[:len(r.faces)]
	r.polys = r.polys[:0]
	var cs, clip [20]r3vec
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
		fd := r3draw{n: n, num: k - n.dot(cam), inside: inside, p0: len(r.polys), pn: m, top: math.Inf(1), bo: math.Inf(-1)}
		for i := 0; i < m; i++ {
			p := [2]float64{kx * (px + f*clip[i].x/clip[i].z), ky * (py - f*clip[i].y/clip[i].z)}
			r.polys = append(r.polys, p)
			fd.top, fd.bo = math.Min(fd.top, p[1]), math.Max(fd.bo, p[1])
		}
		r.fdraw[fi] = fd
		fc.seen = true
	}

	// The pixels, in bands of rows side by side (one a processor): a ray
	// for each, the nearest face each sees, and then that face lit there.
	c := r3shade{cam: cam, lamp: lamp, f: f, invR2: 1 / (1.15 * rm.big * 1.15 * rm.big), light: light,
		hw: rm.hw, hh: rm.hh, tx0: rm.tx0, ty0: rm.ty0, kx: kx, ky: ky, kpx: math.Max(2/kx, 1/ky),
		minU: 0.06, minV: 0.06, rm: rm, textCol: hex32(cText),
		outer: mix32(hex32(cBG), 0, 0.55-0.37*light.lightness)} // round the room: darker than the board
	if r.pix {
		c.minV = 0.03 // a cell is twice as tall as wide: a line across as thin as one down
	}
	r.bands(ph, func(j0, j1 int) {
		for j := j0; j < j1; j++ {
			dy := -((float64(j)+0.5)/ky - py) / f
			row := j * pw
			for i := 0; i < pw; i++ {
				dx := ((float64(i)+0.5)/kx - px) / f
				r.dirs[row+i] = r3vec{fwd.x + right.x*dx + up.x*dy, fwd.y + right.y*dx + up.y*dy, fwd.z + right.z*dx + up.z*dy}
			}
		}
		for i := j0 * pw; i < j1*pw; i++ {
			r.base[i], r.mul[i], r.add[i] = c.outer, 1, 0
			r.zb[i], r.face[i] = math.MaxFloat32, -1
		}
		for fi := range r.faces {
			if r.faces[fi].seen {
				r.depthRows(int32(fi), j0, j1)
			}
		}
		r.shadeRows(&c, j0, j1)
	})

	r.words(&v, rm)
	if r.pix {
		r.writeCells(w, h, sb)
	} else {
		r.write(w, h, sb)
	}
}

// bands calls do for bands of the rows 0 to ph (j0 ≤ row < j1), side by
// side, one a processor, and waits for them all.
func (r *room3D) bands(ph int, do func(j0, j1 int)) {
	n := min(runtime.GOMAXPROCS(0), max(1, ph/16), 32)
	if n <= 1 {
		do(0, ph)
		return
	}
	var wg sync.WaitGroup
	for b := range n {
		j0, j1 := ph*b/n, ph*(b+1)/n
		wg.Add(1)
		go func() {
			defer wg.Done()
			do(j0, j1)
		}()
	}
	wg.Wait()
}

// depthRows finds where face fi is the nearest yet, in rows j0 to j1 of
// the screen: those pixels show it.
func (r *room3D) depthRows(fi int32, j0, j1 int) {
	fd := &r.fdraw[fi]
	if fd.bo < float64(j0) || fd.top > float64(j1) {
		return
	}
	n, num, pw := fd.n, fd.num, r.pw
	spansIn(r.polys[fd.p0:fd.p0+fd.pn], pw, j0, j1, func(j, i0, i1 int) {
		row := j * pw
		for i := i0; i <= i1; i++ {
			idx := row + i
			dir := &r.dirs[idx]
			nd := n.x*dir.x + n.y*dir.y + n.z*dir.z
			if nd >= -1e-12 {
				continue
			}
			t := num / nd
			if t <= 0 || float32(t) >= r.zb[idx] {
				continue
			}
			r.zb[idx], r.face[idx] = float32(t), fi
		}
	})
}

// shadeRows colours and lights each pixel of rows j0 to j1 for the face
// it shows.
func (r *room3D) shadeRows(c *r3shade, j0, j1 int) {
	rm := c.rm
	cam, lamp, f, light := c.cam, c.lamp, c.f, c.light
	hw, hh, tx0, ty0 := c.hw, c.hh, c.tx0, c.ty0
	for idx := j0 * r.pw; idx < j1*r.pw; idx++ {
		fi := r.face[idx]
		if fi < 0 {
			continue
		}
		fc, fd := &r.faces[fi], &r.fdraw[fi]
		n, inside := fd.n, fd.inside
		dir := r.dirs[idx]
		nd := n.x*dir.x + n.y*dir.y + n.z*dir.z
		t := fd.num / nd
		p := r3vec{cam.x + dir.x*t, cam.y + dir.y*t, cam.z + dir.z*t}
		var col uint32
		shade := float32(1)
		switch fc.mat {
		case matBoard, matFront:
			fx := p.x + hw
			cx := max(fc.cx0, min(fc.cx1-1, int(math.Floor(fx))))
			hy := max(fc.py0, min(fc.py1-1, int(math.Floor(hh-p.y))))
			// The wall's pixel here (two a cell across, two down), from
			// the picture about as fine as the screen's pixels are.
			tx := 2*(cx-tx0) + int(2*(fx-float64(cx)))
			ty := hy - 2*ty0
			// How many of the wall's pixels one of the screen's takes
			// (more where the face is seen at a slant), and so which
			// picture. s0 is how many units of the face a unit of the
			// screen is.
			k, s0 := 0, t/f
			s := s0 * c.kpx
			if s > 1.25 || nd > -0.5 {
				q := math.Sqrt(math.Max(-nd/math.Sqrt(dir.x*dir.x+dir.y*dir.y+dir.z*dir.z), 0.03))
				s /= q
				s0 /= q
			}
			for ss := s; k < len(r.mips)-1 && ss > 1.6; ss /= 2 {
				k++
			}
			col = r.mips[k][(ty>>k)*r.mipW[k]+tx>>k]
			if s < 0.75 {
				// Seen bigger than on the flat board: a box line is
				// drawn as a thin line, not a whole cell's tint.
				cy := hy >> 1
				if t := &r.tex[(cy-ty0)*rm.tw+cx-tx0]; t.arms != 0 {
					cov := lineCoverUV(int(t.arms), fx-float64(cx), (hh-p.y)/2-float64(cy), s0/c.kx, s0/(2*c.ky), c.minU, c.minV)
					col = mix32(t.bg, t.fg, 0.85*cov)
				}
			}
			if fc.mat == matBoard {
				shade = 1 - 0.7*r.shadowAt(fx-float64(tx0), hh-p.y-float64(2*ty0))
			}
		case matSolid:
			col = fc.col
			if fc.edge {
				q := p.sub(fc.o)
				u, v := q.dot(fc.eu), q.dot(fc.ev)
				// Its edges, a little darker: a band e wide, blurred to
				// a pixel's width (px) so far away it does not flicker.
				e := math.Min(0.45, 0.25*math.Min(fc.lu, fc.lv))
				px := math.Max(t/f*c.kpx, 0.1*c.kpx)
				d := math.Min(math.Min(u, v), math.Min(fc.lu-u, fc.lv-v))
				if cov := clampF((e-d)/px+0.5, 0, 1); cov > 0 {
					col = mix32(col, 0, 0.3*cov*math.Min(1, 2*e/px))
				}
			}
		case matFloor:
			col = floorAt(fc.col, p.x+hw, p.z, t/f*c.kpx)
		case matWall:
			col = fc.col
			if p.y < rm.y0+3 {
				col = mix32(col, c.textCol, 0.12) // the skirting board
			}
		case matLamp, matShade:
			col = fc.col
		}
		r.base[idx] = col

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
		att := 1 / (1 + d2*c.invR2)
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
}

// cellBox is the pixels of screen cell (col, row) of the last drawing:
// columns x0 ≤ x < x1, rows y0 ≤ y < y1.
func (r *room3D) cellBox(col, row int) (x0, y0, x1, y1 int) {
	w, h := r.rm.w, r.rm.h
	return col * r.pw / w, row * r.ph / h, (col + 1) * r.pw / w, (row + 1) * r.ph / h
}

// cellFour is four pixels of screen cell (col, row), one in the middle of
// each quarter of it: top left, top right, bottom left, bottom right. In
// quarter blocks, where a cell is four pixels, those four.
func (r *room3D) cellFour(col, row int) [4]int {
	x0, y0, x1, y1 := r.cellBox(col, row)
	xa, xb := x0+(x1-x0)/4, x0+3*(x1-x0)/4
	ya, yb := y0+(y1-y0)/4, y0+3*(y1-y0)/4
	return [4]int{ya*r.pw + xa, ya*r.pw + xb, yb*r.pw + xa, yb*r.pw + xb}
}

// cellSamples calls see with pixels spread evenly over screen cell (col,
// row): all four in quarter blocks, at most 4×4 of a picture's.
func (r *room3D) cellSamples(col, row int, see func(i int)) {
	x0, y0, x1, y1 := r.cellBox(col, row)
	nx, ny := min(x1-x0, 4), min(y1-y0, 4)
	for b := 0; b < ny; b++ {
		y := y0 + (2*b+1)*(y1-y0)/(2*ny)
		for a := 0; a < nx; a++ {
			see(y*r.pw + x0 + (2*a+1)*(x1-x0)/(2*nx))
		}
	}
}

// ---- the words ----------------------------------------------------------------------------
//
// Words are never pixels: every line of words on the wall and on the
// blocks' fronts is drawn in the terminal's own letters, one a cell and
// upright, at any turn, tilt, zoom or distance. A line starts where the
// start of its cells is seen (a centred line seen bigger: where its middle
// is), and runs to the right along the screen row. A letter shows only
// where its face is not behind another block, so a word half hidden
// behind a nearer block is cut there (with "…").
//
// Where a front is seen so small or slanted that its lines would fall
// closer than a row apart, its title still shows, and then as many of the
// next lines as fit, a row apart. A line longer than the face is wide
// there is cut short with "…" (a title may stand out a little past a very
// narrow face). Every face's title is placed before any other line, the
// nearer face first: where words would meet, a title wins over other
// lines, and a nearer face's words over a further one's. A title that
// would be hidden, taken or off the screen moves along its face to where
// it can show.

// r3char is a letter drawn in a screen cell.
type r3char struct {
	s      string
	fg, bg uint32
	stamp  int32 // the frame it was put there (r.stamp): older ones are not there
	cont   bool  // the second cell of a two cells wide letter
	face   int32 // the face whose words it is part of
}

// r3run is a line of words on a face: cells x0 ≤ x < x1 of row y of the
// flat board. Without words (a letter or a digit) it is only a mark.
type r3run struct {
	x0, x1, y int
	words     bool
}

const (
	titleRoom = 16 // a title may stand out past a narrow face until it is this wide (and 2 past any face)
	runGap    = 3  // this many spaces in a row part two lines of words
)

// words puts the words of every face that shows the flat board on the
// screen, the nearest face first.
func (r *room3D) words(v *r3view, rm *r3room) {
	w, h := rm.w, rm.h
	if len(r.chars) != w*h {
		r.chars = make([]r3char, w*h)
	}
	// The back wall's cells a block stands on: the block's front shows
	// their words, not the wall.
	for _, b := range r.blocks {
		for y := max(b.y0, rm.ty0); y < min(b.y1, rm.ty0+rm.th); y++ {
			row := r.cover[(y-rm.ty0)*rm.tw:]
			for x := max(b.x0, rm.tx0); x < min(b.x1, rm.tx0+rm.tw); x++ {
				row[x-rm.tx0] = r.stamp
			}
		}
	}
	r.order = r.order[:0]
	for fi := range r.faces {
		fc := &r.faces[fi]
		if !fc.seen || (fc.mat != matBoard && fc.mat != matFront) {
			continue
		}
		d := math.Inf(1) // the wall: last
		if fc.mat == matFront {
			mid := r3vec{(float64(fc.cx0+fc.cx1))/2 - rm.hw, rm.hh - float64(fc.py0+fc.py1)/2, r.pts[fc.p0].z}
			d = mid.sub(v.cam).dot(v.fwd)
		}
		r.order = append(r.order, r3near{fi: fi, d: d})
	}
	sort.Slice(r.order, func(i, j int) bool { return r.order[i].d < r.order[j].d })
	// The titles first, so another face's lines never take a title's
	// place; then the other lines and marks.
	for i := range r.order {
		r.faceWords(&r.order[i], v, rm, true)
	}
	for i := range r.order {
		r.faceWords(&r.order[i], v, rm, false)
	}
}

// r3near is a face and how far in front of the camera it is.
type r3near struct {
	fi int
	d  float64
	// After its title is placed: the flat board row after the title's,
	// and the first screen row its next line may go on.
	cy, next int
}

// faceWords puts the words of face o on the screen: with titles, its
// title (a front's first line of words); else its lines one under another
// from under its title down, as many as fit, and then its marks (a line
// of only signs, such as ◹ or ●) where they are seen, if there is room.
// The back wall has no title: all its words go with the other lines.
func (r *room3D) faceWords(o *r3near, v *r3view, rm *r3room, titles bool) {
	fi := o.fi
	fc := &r.faces[fi]
	wall := fc.mat == matBoard
	z := r.pts[fc.p0].z
	cam, right, up, fwd, f, px, py := v.cam, v.right, v.up, v.fwd, v.f, v.px, v.py
	hw, hh := rm.hw, rm.hh
	tw, tx0, ty0 := rm.tw, rm.tx0, rm.ty0
	// proj is where point (bx, by) of the face (in flat board cells: bx
	// across, by down) is seen on the screen, in cells.
	proj := func(bx, by float64) (sx, sy float64, ok bool) {
		q := r3vec{bx - hw - cam.x, hh - 2*by - cam.y, z - cam.z}
		cz := q.dot(fwd)
		if cz < 0.25 {
			return 0, 0, false
		}
		return px + f*q.dot(right)/cz, (py - f*q.dot(up)/cz) / 2, true
	}
	tex := func(x, y int) *r3texel { return &r.tex[(y-ty0)*tw+x-tx0] }
	covered := func(x, y int) bool { return wall && r.cover[(y-ty0)*tw+x-tx0] == r.stamp }
	// place is where line i of runs starts on the screen, and how many
	// cells it may take: to the next line's start, or the face's edge.
	place := func(runs []r3run, i int) (col, row, limit int, ok bool) {
		ru := runs[i]
		sx, sy, vis := proj(float64(ru.x0), float64(ru.y)+0.5)
		end := float64(fc.cx1)
		if i+1 < len(runs) {
			end = float64(runs[i+1].x0) - 1
		} else if wall {
			end = float64(ru.x1) + 2
		}
		ex, _, vis2 := proj(end, float64(ru.y)+0.5)
		col = int(math.Floor(sx + 0.5))
		if n := ru.x1 - ru.x0; !wall && len(runs) == 1 && abs((ru.x0-fc.cx0)-(fc.cx1-ru.x1)) <= 1 {
			// A line in the middle of its face (a title, centred words),
			// seen bigger than its letters: in the middle of where it is seen.
			if rx, _, ok := proj(float64(ru.x1), float64(ru.y)+0.5); ok && rx-sx > float64(n)+0.5 {
				col = int(math.Floor((sx+rx)/2 - float64(n)/2 + 0.5))
			}
		}
		return col, int(math.Floor(sy)), int(math.Floor(ex+0.5)) - col, vis && vis2
	}
	if titles && wall {
		return
	}
	next := math.MinInt // the first screen row a further line may go on
	title := titles
	from := fc.py0 >> 1 // the first row whose lines go on the screen now
	if !titles && !wall {
		from, next = o.cy, o.next
	}
	o.cy = fc.py1 >> 1 // no title: no further lines
	var runs, words []r3run
	marks := r.marks[:0]
	for cy := fc.py0 >> 1; cy < fc.py1>>1; cy++ {
		runs = r.rowRuns(runs[:0], fc, cy, tex, covered)
		words = words[:0]
		for _, ru := range runs {
			if ru.words {
				words = append(words, ru)
			} else if !titles {
				marks = append(marks, ru)
			}
		}
		if len(words) == 0 || cy < from {
			continue
		}
		var at [16][3]int // each line's column, row and room
		n := min(len(words), len(at))
		top := math.MaxInt
		ok := true
		for i := 0; i < n && ok; i++ {
			col, row, limit, vis := place(words, i)
			if i > 0 && abs(row-at[0][1]) <= 1 {
				row = at[0][1] // the lines of a row stay on one screen row
			}
			at[i] = [3]int{col, row, limit}
			top, ok = min(top, row), vis
		}
		if !ok {
			continue
		}
		// onFace is whether screen row row is still on the face, under the
		// row's first line.
		_, bot, vis := proj(float64(words[0].x0), float64(fc.py1)/2)
		onFace := func(row int) bool { return vis && float64(row)+0.5 <= bot+0.25 }
		shift := 0
		if !wall && !title {
			// A line a row under the one before at least, and only while it
			// is still on the face.
			shift = max(0, next-top)
			if !onFace(top + shift) {
				break
			}
		}
		if !wall && title {
			// A title above or left of the screen, on a face that is on it:
			// on the screen's first row, or from its first column.
			if top < 0 && onFace(0) {
				shift = -top
			}
			if c := at[0][0]; c < 0 && c+at[0][2] > 1 {
				at[0][0], at[0][2] = 0, c+at[0][2]
			}
			if c, need := at[0][0], min(words[0].x1-words[0].x0, 5); c < rm.w && c+need > rm.w {
				at[0][0] = rm.w - need // running off the right: its first letters on the screen
			}
			if row := at[0][1] + shift; row < 0 || row >= rm.h || at[0][0] < 0 || at[0][0] >= rm.w {
				// Still off the screen, where some of the face is on it:
				// where the face is first seen.
				if col, row, n, ok := r.firstSeen(int32(fi)); ok {
					at[0] = [3]int{col, row - shift, n}
				}
			}
		}
		if !wall {
			// A line goes a row lower while the words of a nearer face are
			// where it would go, as long as it is on the face. A title also
			// moves while its face is hidden there (behind a nearer block),
			// up or down along the face's side, to the nearest row where it
			// is seen.
			good := func(shift int) bool {
				for i := 0; i < n; i++ {
					need := min(words[i].x1-words[i].x0, 3)
					if title && i == 0 {
						need = min(words[i].x1-words[i].x0, 4) // at least "Abc…" of it in sight
					}
					if r.room(int32(fi), fc, at[i][0], at[i][1]+shift, need, title && i == 0) < need {
						return false
					}
				}
				return true
			}
			if !good(shift) {
				if title {
					// The rows the face's left side is seen on.
					_, ra, ok1 := proj(float64(fc.cx0), float64(fc.py0)/2)
					_, rb, ok2 := proj(float64(fc.cx0), float64(fc.py1)/2)
					lo, hi := int(math.Floor(math.Min(ra, rb))), int(math.Floor(math.Max(ra, rb)))
					for k := 1; ok1 && ok2 && k <= 16; k++ {
						if row := top + shift + k; row <= hi && good(shift+k) {
							shift += k
							break
						}
						if row := top + shift - k; row >= lo && good(shift-k) {
							shift -= k
							break
						}
					}
				} else {
					for k := 1; k <= 12 && onFace(top+shift+k); k++ {
						if good(shift + k) {
							shift += k
							break
						}
					}
				}
			}
		}
		low := math.MinInt
		prevRow, prevEnd := math.MinInt, math.MinInt // where the last line put on a screen row ends
		for i := 0; i < n; i++ {
			// A title stands out past a narrow face if it must (what it
			// covers of the row's other words then does not show); on the
			// wall, the last words of a row may.
			keep := title && i == 0 || wall && i+1 == len(words)
			col, row, limit := at[i][0], at[i][1]+shift, at[i][2]
			if row == prevRow && col <= prevEnd {
				// Squeezed onto the line before it: a space after that one,
				// in what is left of its own room, or not at all (letters
				// of two lines never mix: "Keys lan" not "Keysn").
				limit -= prevEnd + 1 - col
				col = prevEnd + 1
				if limit < min(words[i].x1-words[i].x0, 3) {
					continue
				}
			}
			r.putRun(int32(fi), fc, words[i], col, row, limit, keep)
			w := words[i].x1 - words[i].x0
			if !keep {
				w = min(w, limit)
			}
			prevRow, prevEnd = row, col+w
			low = max(low, row)
		}
		next = low + 1
		if title {
			o.cy, o.next = cy+1, next
			return
		}
	}
	for i, ru := range marks {
		if col, row, _, ok := place(marks[i:i+1], 0); ok {
			r.putRun(int32(fi), fc, ru, col, row, ru.x1-ru.x0, false)
		}
	}
	r.marks = marks
}

// firstSeen is the first screen cell, row by row, that shows face fi
// (three of its four quarters at least, see cellFour), and how many cells
// from it along the row do.
func (r *room3D) firstSeen(fi int32) (col, row, n int, ok bool) {
	w := r.rm.w
	shows := func(c, row int, fi int32) bool {
		k := 0
		for _, i := range r.cellFour(c, row) {
			if r.face[i] == fi {
				k++
			}
		}
		return k >= 3
	}
	if r.firstAt == nil || r.firstStamp != r.stamp {
		// Once a drawing: every face's first such cell, in one look over
		// the screen.
		r.firstStamp = r.stamp
		if cap(r.firstAt) < len(r.faces) {
			r.firstAt = make([]int32, len(r.faces))
		}
		r.firstAt = r.firstAt[:len(r.faces)]
		for i := range r.firstAt {
			r.firstAt[i] = -1
		}
		for row := 0; row < r.rm.h; row++ {
			for c := 0; c < w; c++ {
				if f := r.face[r.cellFour(c, row)[0]]; f >= 0 && r.firstAt[f] < 0 && shows(c, row, f) {
					r.firstAt[f] = int32(row*w + c)
				}
			}
		}
	}
	at := int(r.firstAt[fi])
	if at < 0 {
		return 0, 0, 0, false
	}
	col, row = at%w, at/w
	n = 1
	for col+n < w && shows(col+n, row, fi) {
		n++
	}
	return col, row, n, true
}

// room is how many cells from (col, row) to the right, up to n, a line
// of face fi may take: cells no other words have taken and, with sight,
// where the face is not hidden behind a nearer block. Cells left of the
// screen count (the line runs off there); the screen's right edge ends it.
func (r *room3D) room(fi int32, fc *r3face, col, row, n int, sight bool) int {
	w := r.rm.w
	if row < 0 || row >= r.rm.h {
		return 0
	}
	num := fc.k - fc.n.dot(r.view.cam)
	c := col
	for ; c < col+n && c < w; c++ {
		if c < 0 {
			continue
		}
		at := row*w + c
		if r.chars[at].stamp == r.stamp || sight && !r.inSight(at, fi, fc.own, fc.n, num) {
			break
		}
	}
	return c - col
}

// rowRuns adds the lines of words in row cy of face fc to runs: letters,
// with no more than runGap-1 spaces between them, between the box lines.
func (r *room3D) rowRuns(runs []r3run, fc *r3face, cy int, tex func(x, y int) *r3texel, covered func(x, y int) bool) []r3run {
	for cx := fc.cx0; cx < fc.cx1; {
		if g := tex(cx, cy).glyph; g == "" || frameGlyph(g) || covered(cx, cy) {
			cx++
			continue
		}
		start, end, gap := cx, cx, 0
		for x := cx; x < fc.cx1 && !covered(x, cy); {
			t := tex(x, cy)
			if frameGlyph(t.glyph) {
				break // a box's line (drawn on the face) parts two lines of words: "Keys│lan" is two
			}
			if t.glyph != "" {
				x++
				if t.wide {
					x++
				}
				end, gap = min(x, fc.cx1), 0
				continue
			}
			if t.blank && gap < runGap-1 {
				gap++
				x++
				continue
			}
			break
		}
		// A lone dot or cross of the board's grid on the wall is not a word.
		if g := tex(start, cy).glyph; fc.mat == matBoard && end-start == 1 && (g == "·" || g == "+") {
			cx = end
			continue
		}
		ru := r3run{x0: start, x1: end, y: cy}
		for x := start; x < end && !ru.words; x++ {
			ru.words = strings.ContainsFunc(tex(x, cy).glyph, isWordRune)
		}
		runs = append(runs, ru)
		cx = end
	}
	return runs
}

// putRun draws line ru of face fc from screen cell (col, row) to the
// right, in at most limit cells (keep: a title, which may stand out past
// a narrow face). It takes only cells no nearer face's words have taken,
// where the face is not behind something nearer: where those end, the
// line is cut short with "…" (a title keeps at least its first letters).
func (r *room3D) putRun(fi int32, fc *r3face, ru r3run, col, row, limit int, keep bool) {
	if row < 0 || row >= r.rm.h {
		return
	}
	tw, tx0, ty0 := r.rm.tw, r.rm.tx0, r.rm.ty0
	type letter struct {
		s    string
		t    int32 // its cell of the flat board
		wide bool
	}
	var buf [96]letter
	ls := buf[:0]
	width := 0
	for x := ru.x0; x < ru.x1; {
		ti := int32((ru.y-ty0)*tw + x - tx0)
		t := &r.tex[ti]
		l := letter{s: t.glyph, t: ti, wide: t.wide}
		if l.s == "" {
			l.s = " "
		}
		ls = append(ls, l)
		x++
		width++
		if t.wide {
			x++
			width++
		}
	}
	if keep {
		limit = max(limit+2, min(width, titleRoom))
	}
	limit = min(limit, r.room(fi, fc, col, row, limit, true))
	if width > limit && len(ls) > 3 && ls[1].s == " " && !ls[0].wide && !strings.ContainsFunc(ls[0].s, isWordRune) {
		ls, width = ls[2:], width-2 // squeezed: the sign before a title gives way to its words
	}
	if limit < 1 || width > limit && (limit < 2 || !keep && limit < 3) {
		return // no room for a letter and "…"
	}
	if width > limit {
		// Cut short: as many letters as fit with "…" after them.
		n, used := 0, 0
		for n < len(ls) {
			lw := 1
			if ls[n].wide {
				lw = 2
			}
			if used+lw > limit-1 {
				break
			}
			used += lw
			n++
		}
		last := ls[max(n-1, 0)].t
		ls = append(ls[:n], letter{s: "…", t: last})
	}
	w := r.rm.w
	c := col
	for _, l := range ls {
		if c >= 0 && c < w {
			at := row*w + c
			r.setChar(at, fi, l.s, &r.tex[l.t])
			if l.wide && c+1 < w {
				r.chars[at+1] = r3char{stamp: r.stamp, cont: true, face: fi}
			}
		}
		c++
		if l.wide {
			c++
		}
	}
}

// frameGlyph is whether g is a line or block of a drawing (box lines,
// corners, half blocks), which the face shows as lines, not as words.
func frameGlyph(g string) bool {
	for _, c := range g {
		return c >= 0x2500 && c <= 0x259f && c != '▏' // not the caret, which is part of the words
	}
	return false
}

// isWordRune is whether c is a letter or a digit.
func isWordRune(c rune) bool { return unicode.IsLetter(c) || unicode.IsDigit(c) }

// inSight is whether a letter of face fi (of structure own) can show in
// screen cell at: fewer than three of its four quarters show another
// structure nearer than the face's plane (n · p = k, num = k − n · camera).
//
// The room itself never hides words: seen from inside, its walls and
// floor are behind everything in it, so they cover only a line that
// stands out past its face; and the lamp is too small to hide a word.
func (r *room3D) inSight(at int, fi int32, own int, n r3vec, num float64) bool {
	w := r.rm.w
	bad := 0
	for _, i := range r.cellFour(at%w, at/w) {
		f := r.face[i]
		if f < 0 || f == fi || r.faces[f].own < 0 || r.faces[f].own == own {
			continue // nothing, the face itself, the room, or its own block's sides (behind its front)
		}
		d := r.dirs[i]
		nd := n.x*d.x + n.y*d.y + n.z*d.z
		if nd >= -1e-12 {
			bad++
			continue
		}
		t := num / nd
		eps := 2e-4*t + 0.01
		switch z := float64(r.zb[i]); {
		case z < t-eps:
			bad++ // nearer
		case z <= t+eps && r.over(fi, r.view.cam.add(d.mul(z))):
			bad++ // the front of a block as far out, over this one on the flat board
		}
	}
	return bad <= 2
}

// over is whether point p is on face fi's part of the flat board (its
// cells), as a block over it on the flat board would be.
func (r *room3D) over(fi int32, p r3vec) bool {
	fc := &r.faces[fi]
	x, y := p.x+r.rm.hw, r.rm.hh-p.y
	return x >= float64(fc.cx0) && x < float64(fc.cx1) && y >= float64(fc.py0) && y < float64(fc.py1)
}

// setChar puts letter s of the flat board's cell t, on face fi, in screen
// cell at: its background is the mean of the pixels under it (the face's
// colour there, lit), and its letter's colour is lit a little by the
// face's light, kept clear of the background.
func (r *room3D) setChar(at int, fi int32, s string, t *r3texel) {
	w := r.rm.w
	var sr, sg, sb, m, a, k float64
	r.cellSamples(at%w, at/w, func(i int) {
		col := r.lit(r.base[i], i)
		if r.face[i] == fi {
			col = r.lit(t.bg, i)
		}
		cr, cg, cb := chans(col)
		sr, sg, sb = sr+cr, sg+cg, sb+cb
		m, a, k = m+float64(r.mul[i]), a+float64(r.add[i]), k+1
	})
	bg := rgb32(sr/k+0.5, sg/k+0.5, sb/k+0.5)
	m, a = 0.55+0.45*m/k, 0.4*a/k
	fr, fg, fb := chans(t.fg)
	ink := rgb32(byteOf(fr*m+a*lampWarm[0]), byteOf(fg*m+a*lampWarm[1]), byteOf(fb*m+a*lampWarm[2]))
	r.chars[at] = r3char{s: s, fg: clearInk(ink, bg), bg: bg, stamp: r.stamp, face: fi}
}

// clearInk is ink c made light or dark enough to read on background bg: it
// keeps its own way (lighter or darker than bg) when that can be clear
// enough, else it goes the other way.
func clearInk(c, bg uint32) uint32 {
	const need = 105.0 // the least difference in lightness, out of 255
	lum := func(c uint32) float64 { r, g, b := chans(c); return 0.299*r + 0.587*g + 0.114*b }
	lc, lb := lum(c), lum(bg)
	if math.Abs(lc-lb) >= need {
		return c
	}
	lighter := func() uint32 { // toward white, until need lighter than bg (or white)
		t := clampF((lb+need-lc)/math.Max(255-lc, 1), 0, 1)
		return mix32(c, 0xffffff, t)
	}
	darker := func() uint32 {
		t := clampF((lc-(lb-need))/math.Max(lc, 1), 0, 1)
		return mix32(c, 0, t)
	}
	up, down := lb+need <= 255, lb-need >= 0
	switch {
	case lc >= lb && up, !down && lb < 128:
		return lighter()
	case lc < lb && down, !up:
		return darker()
	case lb < 128:
		return lighter()
	}
	return darker()
}

// pixel is a pixel's colour, lit.
func (r *room3D) pixel(i int) uint32 { return r.lit(r.base[i], i) }

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

// smoothSpread is how far apart (in any of red, green and blue, out of
// 255) a cell's four pixels may be and still be drawn as one colour.
const smoothSpread = 14

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
	// Four colours close together (a lit surface's gradient, a soft
	// pattern) are one: their mean, as a plain cell, so a smooth surface
	// stays smooth and is not turned into a grain of little blocks.
	spread := 0
	for _, ch := range [3]*[4]int{&cr, &cg, &cb} {
		lo, hi := min(ch[0], ch[1], ch[2], ch[3]), max(ch[0], ch[1], ch[2], ch[3])
		spread = max(spread, hi-lo)
	}
	if spread <= smoothSpread {
		c := uint32((tr+2)/4)<<16 | uint32((tg+2)/4)<<8 | uint32((tb+2)/4)
		return 0, c, c
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
			if ch := &r.chars[row*w+col]; ch.stamp == r.stamp {
				if ch.cont {
					continue // the second half of a wide letter: drawn with it
				}
				top, bot, glyph = ch.fg, ch.bg, ch.s
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
