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
// design_3d_kitty.go): the same drawing, with more pixels to a cell. Over
// either goes a layer of letters (design_3d_text.go): the edges, the
// planks, the walls' rails and the flat drawings' box lines as box line
// letters, so the room is made of text as the flat board is.
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
	"sync/atomic"
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
func clampF(v, lo, hi float64) float64 { return max(lo, min(hi, v)) } // the built-in min and max: no call, same answers
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
	off            float64 // how far out its back stands: its layer (design_3d_layers.go)
	shift          int     // rows its drawing is kept below its place in the picture (layerCopies)
}

// r3room is the room's size. The board on the back wall is the flat
// board's cells tx0 ≤ x < tx0+tw, ty0 ≤ y < ty0+th: the screen's board
// area, widened to take in every structure that shows on it (whole, up to
// a screen beyond it) and a margin. The side walls stand at its left and
// right edges, and the floor lies along its bottom edge, so no block goes
// through them; the camera is fitted to that box (x0 to x1, y0 to y1,
// out to depth).
//
// The room itself is closed: a ceiling over it and a wall behind the
// camera, and where the camera is beyond that box (turned far, tilted up
// under the floor's height), the walls, the floor and the ceiling stand
// further out, so the camera is always inside and sees room wherever it
// looks (see enclose). The back wall round the board is plain wall.
type r3room struct {
	w, h           int     // the screen's board area, cells
	hw, hh         float64 // half of it, in units
	tx0, ty0       int
	tw, th         int
	x0, x1, y0, y1 float64 // the box round the blocks: its sides, bottom and top
	depth          float64 // how far the box comes out
	base           float64 // how far it comes out but for the layers: the camera and the lamp keep to it
	big            float64 // the screen's bigger side, in units
	// The room's walls (see enclose): the side walls at wx0 and wx1, the
	// floor at fy, the ceiling at cy and the wall behind the camera at fz.
	wx0, wx1, fy, cy, fz float64
	walls, floor         int // the finishes (design_3d_finish.go)
}

// baseDepth is rm.base (rm.depth for a room made without it).
func (rm *r3room) baseDepth() float64 {
	if rm.base > 0 {
		return rm.base
	}
	return rm.depth
}

// enclose stands the room's walls round the box of the blocks, and further
// out where they must be to take in the camera at cam with room to spare:
// a camera inside a closed room sees room wherever it looks, never a void.
func (rm *r3room) enclose(cam r3vec) {
	// Well clear of the camera to the sides and behind it, so a wall right
	// next to it does not fill the edge of the screen; a little under it
	// and over it, so the floor and the ceiling are not further off than
	// they need be.
	wide, low := 0.35*rm.big, 2+0.04*rm.big
	rm.wx0, rm.wx1 = math.Min(rm.x0, cam.x-wide), math.Max(rm.x1, cam.x+wide)
	rm.fy = math.Min(rm.y0, cam.y-low)
	rm.cy = math.Max(rm.y1+math.Max(2, 0.125*(rm.y1-rm.y0)), cam.y+low)
	rm.fz = math.Max(rm.depth, cam.z+wide)
}

// The margin between the outermost blocks and the walls, in cells: two
// across, one down (one unit is a cell's width, a cell is two units tall).
const roomMarginX, roomMarginY = 3, 2

// roomReach is how many screens round the board area the room takes in,
// each way: the board beyond the screen is in the room too, so turned or
// walking about, it goes on past the screen's edges instead of being cut
// off at them. Far bigger boards are cut this far out, so the room's
// picture of the wall stays a size that draws quickly.
const roomReach = 4

// area works out the room for a w×h board area at zoom z, and which
// structures are blocks in it. The room is fitted to the board round the
// screen (roomReach screens each way): the blocks (with a pulled out
// drawer) and the pen's ink, and always the screen itself.
func (r *room3D) area(d *designState, w, h int, z float64) r3room {
	r.blocks = r.blocks[:0]
	big := math.Max(float64(w), float64(2*h))
	lx0, ly0, lx1, ly1 := -roomReach*w, -roomReach*h, (roomReach+1)*w, (roomReach+1)*h
	x0, y0, x1, y1 := math.MaxInt, math.MaxInt, math.MinInt, math.MinInt
	take := func(bx0, by0, bx1, by1 int) {
		bx0, by0, bx1, by1 = max(bx0, lx0), max(by0, ly0), min(bx1, lx1), min(by1, ly1)
		if bx0 < bx1 && by0 < by1 {
			x0, y0, x1, y1 = min(x0, bx0), min(y0, by0), max(x1, bx1), max(y1, by1)
		}
	}
	deepest, outmost := 0.0, 0.0 // the deepest block against the wall; the front furthest out, layers and all
	for _, o := range d.octs {
		x, y, ow, oh := d.rect(o)
		if x >= lx1 || y >= ly1 || x+ow <= lx0 || y+oh <= ly0 {
			continue // too far from the screen: not in the room
		}
		b := r3block{o: o, x0: max(x, lx0), y0: max(y, ly0), x1: min(x+ow, lx1), y1: min(y+oh, ly1)}
		b.whole = b.x0 == x && b.y0 == y && b.x1 == x+ow && b.y1 == y+oh
		b.dep = blockDepth(&b, z, big)
		b.off = float64(o.Layer) * layerStep
		deepest = math.Max(deepest, b.dep)
		outmost = math.Max(outmost, b.off+b.dep)
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
	// The wall reaches past the screen's sides and top, so every block
	// stays well inside the room, under the ceiling and between the side
	// walls, at every turn; the floor stays just under the lowest block.
	x0, y0, x1 = min(x0, 0), min(y0, 0), max(x1, w)
	x0, y0, x1, y1 = x0-roomMarginX, y0-roomMarginY, x1+roomMarginX, y1+roomMarginY
	rm := r3room{w: w, h: h, hw: float64(w) / 2, hh: float64(h), tx0: x0, ty0: y0, tw: x1 - x0, th: y1 - y0, big: big}
	rm.x0, rm.x1 = float64(x0)-rm.hw, float64(x1)-rm.hw
	rm.y1, rm.y0 = rm.hh-float64(2*y0), rm.hh-float64(2*y1)
	// The floor comes out a little under half the room's size (no more
	// than for the screen), and over three times the deepest block.
	rm.base = math.Max(8, math.Max(deepest/0.3, 0.45*math.Min(big, math.Max(rm.x1-rm.x0, rm.y1-rm.y0))))
	// Structures out on layers make the room deeper, only as far as they
	// need: the camera, the lamp and the lens keep to rm.base, so a
	// structure far out does not push the camera back and narrow its view.
	rm.depth = math.Max(rm.base, outmost+0.15*big)
	// The blocks come out of the wall as the room turns (see outK): seen
	// nearly straight on they are nearly flat, so each one is the size it
	// is on the flat board, not bigger for standing nearer the camera.
	k := r.outK()
	for i := range r.blocks {
		r.blocks[i].dep = math.Max(0.02, r.blocks[i].dep*k)
		// Layers come out with the blocks; a hair apart even flat against
		// the wall, so a higher layer is always in front.
		r.blocks[i].off = r.blocks[i].off*k + 0.02*float64(r.blocks[i].o.Layer)
	}
	r.layerCopies(d, &rm)
	r.pinCopies(d, &rm) // turned pins' cards (design_3d_pins.go)
	return rm
}

// outTurn is the turn (degrees away from straight at the wall) by which
// the blocks are all the way out of it.
const outTurn = 40.0

// outK is how far out of the wall the blocks come, 0 to 1: nothing
// straight on, all the way at outTurn and more, smoothly between. A block
// stands out toward the camera, so the further out, the bigger it looks
// than on the flat board; coming out slowly, going into 3D from the flat
// board has no jump in size. For the first-person camera it is the most
// it has been since 3D started, and grows as the camera looks away from
// straight at the wall or moves (all the way out by outWalk of the
// screen's bigger side): it never goes back in, so the blocks do not
// swell and shrink as it looks round.
func (r *room3D) outK() float64 {
	away := func(a, b float64) float64 { // degrees between the way of turn a, tilt b and straight at the wall
		c := math.Cos(a*math.Pi/180) * math.Cos(b*math.Pi/180)
		return math.Acos(clampF(c, -1, 1)) * 180 / math.Pi
	}
	smooth := func(deg float64) float64 {
		t := clampF(deg/outTurn, 0, 1)
		return t * t * (3 - 2*t)
	}
	if !r.free {
		return smooth(away(float64(r.yaw), float64(r.pitch)))
	}
	k := smooth(away(r.look[0], r.look[1]))
	if big := r.rm.big; big > 0 {
		k = math.Max(k, smooth(outTurn*r.pos.sub(r.startPos).len()/(outWalk*big)))
	}
	r.freeK = math.Max(r.freeK, k)
	return r.freeK
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
	wide   bool     // the letter is two cells wide
	blank  bool     // a space
	arms   int8     // a box line: its arms (see boxArms)
	style  uint8    // a box line: heavy, double, dashed or round (see boxStyle)
	mark   string   // a sign the text layer shows as it is, seen big enough: a shade, braille, a slanting line (design_3d_text.go)
	bold   bool     // its letter is bold
	gl     *r3glyph // its letter painted on the face (design_3d_glyphs.go), or nil
	gx     int8     // 1: the right half of a wide letter (gl is the letter)
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
	t := r3texel{fg: colour32(cell.Style.Fg, defFG), bg: colour32(cell.Style.Bg, defBG), bold: cell.Style.Attrs&uv.AttrBold != 0}
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
		t.mark = s
	case s == "░":
		all(mix32(t.bg, t.fg, 0.25))
		t.mark = s
	case s == "▓":
		all(mix32(t.bg, t.fg, 0.75))
		t.mark = s
	case ru >= 0x2500 && ru <= 0x257f && size == len(s):
		// A box line, the edge of a thing: each quarter is tinted by the
		// arms of the line that touch it, so a corner stays a corner.
		arms := boxArms(ru)
		t.arms, t.style = int8(arms), boxStyle(ru)
		if arms == 0 {
			all(mix32(t.bg, t.fg, 0.55))
			t.mark = s // ╱ ╲ ╳
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
		t.mark = s
	case ru >= 0x2800 && ru <= 0x28ff && size == len(s):
		all(mix32(t.bg, t.fg, 0.4)) // braille dots: a picture
		t.mark = s
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
	defFG, defBG := hex32(cText), hex32(boardBG())
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
	// The structures as the 3D view draws them (clean3D: see design3DRows),
	// so their drawings are the ones made for it.
	clean3D = r.paint
	defer func() { clean3D = false }()
	for _, b := range r.blocks {
		x, y, ow, oh := d.rect(b.o)
		if b.shift == 0 && x >= 0 && y >= 0 && x+ow <= w && y+oh <= h {
			continue // all of it on the screen: it is read from there
		}
		k, ok := d.drawKeyOf(b.o)
		if !ok {
			continue // opening or shutting (a new drawing each frame): only its colour
		}
		kept := d.drawn[b.o.ID]
		if len(kept) == 0 || kept[0].key != k {
			// Off the screen, the flat board did not draw it: draw it now
			// (and keep the drawing, as the flat board keeps its own).
			seen := map[int][]drawnStruct{}
			d.cachedLayers(b.o, seen)
			if kept = seen[b.o.ID]; len(kept) == 0 || kept[0].key != k {
				continue
			}
			if d.drawn == nil {
				d.drawn = map[int][]drawnStruct{}
			}
			d.drawn[b.o.ID] = kept
		}
		for _, p := range kept[0].parts {
			p.read() // (drawn but never put on the screen: its cells not read yet)
			// Only the rows and columns of it that are in the room.
			px, py := p.l.GetX(), p.l.GetY()
			for cy := max(0, rm.ty0-py); cy < min(p.cells.Height(), rm.ty0+rm.th-py); cy++ {
				line := p.cells.Lines[cy]
				for cx := max(0, rm.tx0-px); cx < min(len(line), rm.tx0+rm.tw-px); cx++ {
					if sx, sy := px+cx, py+cy; b.shift != 0 {
						put(sx, sy+b.shift, &line[cx]) // on a layer: below the board (layerCopies)
					} else if sx < 0 || sy < 0 || sx >= w || sy >= h {
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
	if r.paint {
		r.pinTexels(d, rm, defFG, defBG)
		r.cleanCards(rm)
		// Labels and name plates over them, as the flat board has them
		// over its structures: on the clean cards, and on the structures
		// off the screen or out on layers too (design_labels.go).
		for _, b := range r.blocks {
			if o := b.o; o.LabelStyle != labelNone || o.Plate != "" {
				x, y, w, h := d.rect(o)
				d.labelCells(o, x, y+b.shift, w, h, func(x, y int, cell uv.Cell) { put(x, y, &cell) })
			}
		}
		r.paintGlyphs(rm)
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
	if r.paint {
		// For the picture between its pixels (smooth): a painted letter's
		// cell is its background there, the letter is painted over it.
		if len(r.flat0) != len(l0) {
			r.flat0 = make([]uint32, len(l0))
		}
		copy(r.flat0, l0)
		for cy := 0; cy < rm.th; cy++ {
			for cx := 0; cx < rm.tw; cx++ {
				if t := &r.tex[cy*rm.tw+cx]; t.gl != nil {
					i := 2*cy*w + 2*cx
					r.flat0[i], r.flat0[i+1], r.flat0[i+w], r.flat0[i+w+1] = t.bg, t.bg, t.bg, t.bg
				}
			}
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
	matBoard   = iota // the back wall: the flat board
	matFront          // a block's front: its part of the flat board
	matSolid          // one colour (a block's sides)
	matFloor          // planks
	matWall           // a side wall (or the wall behind the camera): wallpaper, rails and a skirting board
	matBack           // the back wall round the board: papered, with a frame round the board
	matCeiling        // the ceiling: plaster
	matLamp           // the bulb: it shines, it is not lit
	matShade          // the lamp's shade, lit from inside
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
	seen               bool    // drawn this frame (it faced the camera)
	inside             bool    // its words hide behind its own block's other faces too (a cabinet's, design_3d_cabinet.go)
	back               float64 // a block's: how far out its back stands (its layer)
	tdy                int     // half rows its drawing is below its cells in the picture (a block on a layer, layerCopies)
	// A card turned on its face (a board's turned pin, design_3d_pins.go):
	// the picture's column at point p is au · p + a0 and its half row
	// av · p + b0, within columns tcx0 ≤ x < tcx1 and half rows tpy0 ≤ y < tpy1.
	aff                    bool
	au, av                 r3vec
	a0, b0                 float64
	tcx0, tcx1, tpy0, tpy1 int
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
	return r3vec{rm.x0 + 0.15*(rm.x1-rm.x0), rm.y1 - math.Max(1, 0.02*(rm.y1-rm.y0)), 0.6 * rm.baseDepth()}
}

// scene puts the room, the blocks and the lamp in faces.
func (r *room3D) scene(rm *r3room, z float64, lamp r3vec) {
	r.faces, r.pts = r.faces[:0], r.pts[:0]
	hw, hh := rm.hw, rm.hh
	bg, text, accent := hex32(cBG), hex32(cText), hex32(cAccent)
	x0, x1, y0, y1 := rm.x0, rm.x1, rm.y0, rm.y1
	wide, tall := x1-x0, y1-y0

	// The room (the board on the back wall is face 0), closed all round
	// (see enclose). Each wall runs a little past the ones it meets, so no
	// pixel slips through the corners. The back wall and the floor have a
	// back too, seen from behind or below by the first-person camera.
	wg := wallGloss(rm.walls) // how shiny the walls' finish is
	r.quadFace(r3face{n: r3vec{0, 0, 1}, mat: matBoard, own: -1, gloss: wg + 0.05,
		cx0: rm.tx0, cx1: rm.tx0 + rm.tw, py0: 2 * rm.ty0, py1: 2 * (rm.ty0 + rm.th)},
		r3vec{x0, y0, 0}, r3vec{wide, 0, 0}, r3vec{0, tall, 0})
	const lap = 0.5
	wx0, wx1, fy, cy, fz := rm.wx0-lap, rm.wx1+lap, rm.fy-lap, rm.cy+lap, rm.fz+lap
	across, up, out := r3vec{wx1 - wx0, 0, 0}, r3vec{0, cy - fy, 0}, r3vec{0, 0, fz + lap}
	wall := mix32(bg, text, 0.08)
	// The back wall round the board, a hair behind it (so the board is
	// always seen in front of it): the board hangs on it like a panel.
	r.quadFace(r3face{n: r3vec{0, 0, 1}, mat: matBack, own: -1, col: wall, gloss: wg},
		r3vec{wx0, fy, -0.02}, across, up)
	r.quadFace(r3face{n: r3vec{0, 1, 0}, mat: matFloor, own: -1, col: mix32(mix32(bg, text, 0.16), accent, 0.06), gloss: floorGloss(rm.floor)},
		r3vec{wx0, rm.fy, -lap}, across, out)
	r.quadFace(r3face{n: r3vec{1, 0, 0}, mat: matWall, own: -1, col: mix32(mix32(bg, text, 0.09), accent, 0.05), gloss: wg},
		r3vec{rm.wx0, fy, -lap}, up, out)
	r.quadFace(r3face{n: r3vec{-1, 0, 0}, mat: matWall, own: -1, col: mix32(bg, text, 0.07), gloss: wg},
		r3vec{rm.wx1, fy, -lap}, up, out)
	r.quadFace(r3face{n: r3vec{0, 0, -1}, mat: matWall, own: -1, col: wall, gloss: wg},
		r3vec{wx0, fy, rm.fz}, across, up)
	r.quadFace(r3face{n: r3vec{0, -1, 0}, mat: matCeiling, own: -1, col: mix32(bg, text, 0.05), gloss: 0.05},
		r3vec{wx0, rm.cy, -lap}, across, out)
	back := mix32(mix32(bg, text, 0.12), 0, 0.2)
	r.quadFace(r3face{n: r3vec{0, 0, -1}, mat: matSolid, own: -1, col: back},
		r3vec{wx0, fy, -0.05}, across, up)
	r.quadFace(r3face{n: r3vec{0, -1, 0}, mat: matSolid, own: -1, col: back},
		r3vec{wx0, rm.fy - 0.05, -lap}, across, out)

	// Every structure that shows, as a block, built against the wall and
	// then moved out to its layer (layerOut).
	for i := range r.blocks {
		f0, p0 := len(r.faces), len(r.pts)
		r.block(rm, i, hw, hh, text)
		r.layerOut(&r.blocks[i], f0, p0, hw, hh)
	}
	r.lamp(rm, lamp, text, bg, accent)
	r.worldScene(rm) // the windows and the other lights (design_3d_world.go)
}

// block adds block i's faces, against the wall.
func (r *room3D) block(rm *r3room, i int, hw, hh float64, text uint32) {
	{
		b := &r.blocks[i]
		o := b.o
		// A block on top of another on the flat board comes out a hair further.
		dep := b.dep + 1e-5*float64(i)
		if o.Kind == kCabinet && r.cabinetBlock(rm, b, dep) {
			return // a cupboard with doors that open (design_3d_cabinet.go)
		}
		if o.Kind == kBox && r.boxBlock(b, dep, hw, hh) {
			return // an open box, its walls and what is in it (design_3d_box.go)
		}
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
		if o.Kind == kBoard {
			r.pinFaces(i, dep, hw, hh) // its turned pins, cards in front of it (design_3d_pins.go)
		}
	}
}

// lamp adds the lamp: a bulb under a shade, hung on a cord from above the walls.
func (r *room3D) lamp(rm *r3room, lamp r3vec, text, bg, accent uint32) {
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
	ceil := rm.cy
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
		// lamp on the wall, joined to the outline of its back (on the
		// wall, or out on its layer) seen the same way.
		dep := r.pts[f.p0].z
		if lamp.z-dep < 0.5 {
			continue
		}
		X0, X1 := float64(f.cx0)-hw, float64(f.cx1)-hw
		Yt, Yb := hh-float64(f.py0), hh-float64(f.py1)
		n := 0
		k := lamp.z / (lamp.z - dep)
		kb := lamp.z / (lamp.z - f.back)
		for _, c := range [4][2]float64{{X0, Yt}, {X1, Yt}, {X1, Yb}, {X0, Yb}} {
			pts[n] = [2]float64{lamp.x + (c[0]-lamp.x)*kb - rm.x0, rm.y1 - (lamp.y + (c[1]-lamp.y)*kb)}
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

// orbitView is the tests' fixed camera (and where 3D starts): it goes round the middle of
// the room (moved by the pan) at the room's turn and tilt.
func (r *room3D) orbitView(rm *r3room) r3view {
	fw, fh := float64(rm.w), float64(2*rm.h)
	dist0 := rm.baseDepth() + 1.05*rm.big // the camera's distance from the back wall, straight on
	f := dist0                            // so straight on, the back wall is the board area cell for cell
	// The camera turns round the middle of the screen on the back wall, at
	// the same distance and with the same lens at every turn: the wall and
	// the blocks on it stay where they are on the screen, only seen from
	// another side. Nothing is fitted or zoomed as it turns.
	pivot := r3vec{0, 0, 0}
	rad := dist0
	ya, pa := -float64(r.yaw)*math.Pi/180, float64(r.pitch)*math.Pi/180
	way := r3vec{math.Sin(ya) * math.Cos(pa), math.Sin(pa), math.Cos(ya) * math.Cos(pa)}
	cam := pivot.add(way.mul(rad))
	fwd := pivot.sub(cam).norm()
	right := fwd.cross(r3vec{0, 1, 0}).norm()
	up := right.cross(fwd)
	// The camera closer or further (+ and -) and moved sideways (a drag with
	// the middle or right button).
	cam = pivot.add(r.pan).add(way.mul(rad * math.Pow(0.87, float64(r.near))))
	return r3view{cam: cam, right: right, up: up, fwd: fwd, f: f, px: fw / 2, py: fh / 2}
}

// freeView is the first-person camera: where it is, looking that way.
func (r *room3D) freeView(rm *r3room) r3view {
	ly, lp := r.look[0]*math.Pi/180, r.look[1]*math.Pi/180
	fwd := r3vec{math.Sin(ly) * math.Cos(lp), -math.Sin(lp), -math.Cos(ly) * math.Cos(lp)}
	right := fwd.cross(r3vec{0, 1, 0}).norm()
	up := right.cross(fwd)
	return r3view{cam: r.pos, right: right, up: up, fwd: fwd, f: r.ff * r.lensK(), px: float64(rm.w) / 2, py: float64(rm.h)}
}

// lensK is the lens's zoom (1 before it is set).
func (r *room3D) lensK() float64 {
	if r.lens <= 0 {
		return 1
	}
	return r.lens
}

// floorAt is the floor's colour at (x, z), seen with px units to a pixel:
// planks running out from the wall, each a shade apart, with soft joins.
// Far away, where a join would be thinner than a pixel and only flicker,
// the joins fade out.
func floorAt(base uint32, x, z, px float64) uint32 {
	const plankW, plankL = 7.0, 34.0
	mod := func(a, b float64) float64 { return a - b*ffloor(a/b) } // from 0 up to b, below 0 too
	p := ffloor(x / plankW)
	along := z + mod(p*13, plankL)
	hash := mod(p*7.31+ffloor(along/plankL)*3.17, 1) // each length of plank its own shade
	dark := 0.04 + 0.03*hash
	// The join between two planks (the lines running out from the wall),
	// blurred to a pixel's width: 1 on it, 0 away from it.
	d := min(x-p*plankW, plankW-(x-p*plankW))
	j := clampF(1-(d-0.25)/max(px, 0.3), 0, 1)
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
	bgPlain        uint32  // the board's own background: an empty cell of it
	wallCol        uint32  // the walls' colour, which the empty board takes too
	gk             float64 // how strong the walls' grain is in this theme (see grainFor)
}

// draw draws the room into sb: rm.h lines of rm.w cells. The screen is
// r.pw × r.ph pixels, kx a unit across and ky a unit down: r.want of them
// as a picture (kitty, see design_3d_kitty.go), or else four a cell, two
// across and two down, drawn as quarter blocks.
func (r *room3D) draw(rm *r3room, z float64, sb *strings.Builder) {
	w, h := rm.w, rm.h
	pw, ph := 2*w, 2*h
	r.pix = r.want[0] > 0 && r.want[1] > 0
	r.paint = r.paint && r.pix && r.plain
	if r.pix {
		pw, ph = r.want[0], r.want[1]
	}
	r.pw, r.ph = pw, ph
	r.kx, r.ky = float64(pw)/float64(w), float64(ph)/float64(2*h)
	r.stamp++
	var v r3view
	if r.free {
		v = r.freeView(rm)
	} else {
		v = r.orbitView(rm)
	}
	if r.free {
		rm.enclose(r.startPos) // the room as it was round the camera when 3D began (walking stays inside it: inRoom)
	} else {
		rm.enclose(v.cam)
	}
	r.view, r.rm = v, *rm
	cam, right, up, fwd, f, px, py := v.cam, v.right, v.up, v.fwd, v.f, v.px, v.py
	lamp := lampAt(rm)
	light := lightFor(cBG)
	gk := grainFor(light.lightness)

	r.scene(rm, z, lamp)
	r.shadows(rm, lamp)

	// Beyond the room (only seen from outside it, by the first-person camera): the walls'
	// colour in the light they mostly have, so it reads as more room.
	bgPlain := hex32(boardBG()) // an empty cell of the board, which is the walls' colour
	wr, wg, wb := chans(bgPlain)
	wl := light.ambient + 0.85*light.fill + 0.12*light.lamp
	outside := rgb32(byteOf(wr*wl), byteOf(wg*wl), byteOf(wb*wl))

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
		outer: outside, bgPlain: bgPlain, wallCol: bgPlain, gk: gk}
	if r.pix {
		c.minV = 0.03 // a cell is twice as tall as wide: a line across as thin as one down
	}
	// Each pixel's ray is fwd + right·colDX[i] + up·rowDY[j] (see dirAt):
	// worked out where it is needed, not kept for every pixel.
	if cap(r.colDX) < pw {
		r.colDX = make([]float64, pw)
	}
	if cap(r.rowDY) < ph {
		r.rowDY = make([]float64, ph)
	}
	r.colDX, r.rowDY = r.colDX[:pw], r.rowDY[:ph]
	for i := range pw {
		r.colDX[i] = ((float64(i)+0.5)/kx - px) / f
	}
	for j := range ph {
		r.rowDY[j] = -((float64(j)+0.5)/ky - py) / f
	}
	// The kitty picture on the graphics card, when there is one
	// (design_3d_gpu.go): its pixels straight into rgbOut, and no pixel
	// buffers here (pickAt then casts its rays, see castAt).
	r.gpuDrawn = r.paint && len(r.rgbOut) == 3*pw*ph && r.gpuDraw(&c, rm)
	if r.gpuDrawn {
		r.base, r.mul, r.add, r.zb, r.face, r.plas = nil, nil, nil, nil, nil, nil
		r.world.tint = nil
	} else if n := pw * ph; len(r.base) != n {
		r.base, r.mul, r.add = make([]uint32, n), make([]float32, n), make([]float32, n)
		r.zb, r.face, r.plas = make([]float32, n), make([]int32, n), make([]float32, n)
		r.world.tint = make([]uint32, n)
	}
	r.bands(ph, func(j0, j1 int) {
		if r.gpuDrawn {
			return
		}
		for i := j0 * pw; i < j1*pw; i++ {
			r.base[i], r.mul[i], r.add[i], r.world.tint[i] = c.outer, 1, 0, 0
			r.zb[i], r.face[i] = math.MaxFloat32, -1
		}
		for fi := range r.faces {
			if r.faces[fi].seen {
				r.depthRows(int32(fi), j0, j1)
			}
		}
		r.shadeRows(&c, j0, j1)
		if out := r.rgbOut; len(out) == 3*pw*ph && r.paint {
			// The picture's bytes for kitty, while these rows are at hand:
			// in the painted picture nothing changes a pixel after this.
			for i := j0 * pw; i < j1*pw; i++ {
				col := r.lit(r.base[i], i)
				out[3*i], out[3*i+1], out[3*i+2] = byte(col>>16), byte(col>>8), byte(col)
			}
		}
	})
	r.rgbDone = len(r.rgbOut) == 3*pw*ph && r.paint

	// The back wall's cells a block stands on: the block's front shows
	// their words and lines, not the wall.
	for _, b := range r.blocks {
		for y := max(b.y0, rm.ty0); y < min(b.y1, rm.ty0+rm.th); y++ {
			row := r.cover[(y-rm.ty0)*rm.tw:]
			for x := max(b.x0, rm.tx0); x < min(b.x1, rm.tx0+rm.tw); x++ {
				row[x-rm.tx0] = r.stamp
			}
		}
	}
	if !r.plain {
		r.textLayer(&c) // the room in letters (design_3d_text.go)
	}
	if !r.paint {
		r.words(&v, rm)
	} else if len(r.chars) != w*h {
		r.chars = make([]r3char, w*h) // none: in the picture the words are painted on (design_3d_glyphs.go)
	}
	if !r.pix {
		r.townLetters(&c) // the town outside in the terminal's letters (design_3d_world.go)
	}
	if !r.plain {
		r.finishInk()
	}
	if r.pix && r.shapes() {
		r.writeShapes(w, h, sb) // design_3d_shapes.go
	} else if r.pix {
		r.writeCells(w, h, sb)
	} else {
		r.write(w, h, sb)
	}
}

// bands calls do for bands of the rows 0 to ph (j0 ≤ row < j1), side by
// side, and waits for them all. The bands are small (bandRows rows) and
// each processor takes the next one when it is done with its last, so a
// slow processor (this laptop has quick and slow ones) or a band with more
// in it does not hold up the rest.
func (r *room3D) bands(ph int, do func(j0, j1 int)) {
	const bandRows = 12
	nb := (ph + bandRows - 1) / bandRows
	n := min(runtime.GOMAXPROCS(0), nb, 64)
	if n <= 1 {
		do(0, ph)
		return
	}
	var next atomic.Int32
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				b := int(next.Add(1)) - 1
				if b >= nb {
					return
				}
				do(b*bandRows, min(ph, (b+1)*bandRows))
			}
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
	v := &r.view
	// n · ray = n·fwd + n·right × colDX[i] + n·up × rowDY[j]
	nf, nr, nu := n.dot(v.fwd), n.dot(v.right), n.dot(v.up)
	spansIn(r.polys[fd.p0:fd.p0+fd.pn], pw, j0, j1, func(j, i0, i1 int) {
		row := j * pw
		nrow := nf + nu*r.rowDY[j]
		for i := i0; i <= i1; i++ {
			idx := row + i
			nd := nrow + nr*r.colDX[i]
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
	fwd, right, up := r.view.fwd, r.view.right, r.view.up
	pw := r.pw
	for j := j0; j < j1; j++ {
		dy := r.rowDY[j]
		rx, ry, rz := fwd.x+up.x*dy, fwd.y+up.y*dy, fwd.z+up.z*dy // the row's rays, less right·colDX
		row0 := (j &^ 1) * pw                                     // the top row of its 2×2 squares (bands start on an even row)
		if j&^1 < j0 {
			row0 = j * pw
		}
		for i, idx := 0, j*pw; i < pw; i, idx = i+1, idx+1 {
			r.shadePixel(c, idx, row0+i&^1, r3vec{rx + right.x*r.colDX[i], ry + right.y*r.colDX[i], rz + right.z*r.colDX[i]})
		}
	}
}

// shadePixel colours and lights pixel idx, whose ray is dir, for the face
// it shows.
func (r *room3D) shadePixel(c *r3shade, idx, anchor int, dir r3vec) {
	rm := c.rm
	cam, lamp, f, light := c.cam, c.lamp, c.f, c.light
	hw, hh, tx0, ty0 := c.hw, c.hh, c.tx0, c.ty0
	{
		fi := r.face[idx]
		if fi < 0 {
			return
		}
		fc, fd := &r.faces[fi], &r.fdraw[fi]
		n, inside := fd.n, fd.inside
		nd := n.x*dir.x + n.y*dir.y + n.z*dir.z
		t := fd.num / nd
		p := r3vec{cam.x + dir.x*t, cam.y + dir.y*t, cam.z + dir.z*t}
		var col uint32
		shade := float32(1)
		shadowOn := false
		dl := 1 / math.Sqrt(dir.x*dir.x+dir.y*dir.y+dir.z*dir.z)
		// foot is how many units of the face one pixel spans here (more
		// where it is seen at a slant): a texture's details finer than
		// that fade out instead of flickering.
		foot := func() float64 { return t / f * c.kpx / max(-nd*dl, 0.12) }
		// The walls' plaster: soft bumps, the smallest over a dozen pixels
		// across, so a pixel takes it from the top left of its 2×2 square
		// when that shows the same face (as the light, below).
		plas := func(u, v, ft float64) float64 {
			if anchor != idx && r.face[anchor] == fi {
				return float64(r.plas[anchor])
			}
			pl := plaster(u, v, ft)
			r.plas[idx] = float32(pl)
			return pl
		}
		switch fc.mat {
		case matBoard, matFront:
			fx := p.x + hw
			hp := hh - p.y + float64(fc.tdy) // half rows down the picture
			cx := max(fc.cx0, min(fc.cx1-1, int(ffloor(fx))))
			hy := max(fc.py0, min(fc.py1-1, int(ffloor(hp))-fc.tdy))
			hy += fc.tdy // a block out on a layer: its drawing is kept below the board (layerCopies)
			sc := 1.0
			if fc.aff { // a turned card (design_3d_pins.go)
				fx, hp = fc.au.dot(p)+fc.a0, fc.av.dot(p)+fc.b0
				cx = max(fc.tcx0, min(fc.tcx1-1, int(ffloor(fx))))
				hy = max(fc.tpy0, min(fc.tpy1-1, int(ffloor(hp))))
				sc = fc.affScale()
			}
			// The wall's pixel here (two a cell across, two down), from
			// the picture about as fine as the screen's pixels are.
			tx := 2*(cx-tx0) + int(2*(fx-float64(cx)))
			ty := hy - 2*ty0
			// How many of the wall's pixels one of the screen's takes
			// (more where the face is seen at a slant), and so which
			// picture. s0 is how many units of the face a unit of the
			// screen is.
			k, s0 := 0, t/f*sc
			s := s0 * c.kpx
			if s > 1.25 || nd > -0.5 {
				q := math.Sqrt(math.Max(-nd/math.Sqrt(dir.x*dir.x+dir.y*dir.y+dir.z*dir.z), 0.03))
				s /= q
				s0 /= q
			}
			if r.paint {
				// The picture between its pixels, smoothly: a texture, a
				// picture in half blocks, a coloured strip have soft edges,
				// not squares; far off, the smaller pictures blended
				// (design_3d_glyphs.go).
				col = r.wallAt(2*(fx-float64(tx0)), hp-float64(2*ty0), s)
				if s < glyphGone {
					// A letter, painted (design_3d_glyphs.go), fading into
					// its tint as it gets too small to read.
					cy := hy >> 1
					if t := &r.tex[(cy-ty0)*rm.tw+cx-tx0]; t.gl != nil {
						u := fx - float64(cx) + float64(t.gx)
						v := hp/2 - float64(cy)
						g := mix32(t.bg, t.fg, t.gl.cover(u, v, s0/c.kx, s0/(2*c.ky)))
						col = mix32(col, g, clamp01((glyphGone-s)/(glyphGone-glyphFull)))
					}
				}
			} else {
				for ss := s; k < len(r.mips)-1 && ss > 1.6; ss /= 2 {
					k++
				}
				col = r.mips[k][(ty>>k)*r.mipW[k]+tx>>k]
			}
			if s < lineGone {
				// Seen bigger than on the flat board: a box line is
				// drawn as a thin line, not a whole cell's tint (and
				// between the two smoothly, so it does not pop).
				cy := hy >> 1
				if t := &r.tex[(cy-ty0)*rm.tw+cx-tx0]; t.arms != 0 {
					cov := lineCoverUV(int(t.arms), fx-float64(cx), hp/2-float64(cy), s0/c.kx, s0/(2*c.ky), c.minU, c.minV)
					col = mix32(col, mix32(t.bg, t.fg, 0.85*cov), clamp01((lineGone-s)/(lineGone-lineFull)))
				}
			}
			if fc.mat == matBoard {
				if col == c.bgPlain {
					// The empty board is the room's own wall, papered as the
					// side walls are: the structures hang on it, with no
					// dark panel behind them.
					ft := foot()
					col = wallFinish(rm.walls, c.wallCol, c.textCol, p.x, p.y, ft, c.gk, plas(p.x, p.y, ft), rm)
				}
				shadowOn = true // the blocks' shadows on it (see the light, below)
			}
		case matSolid:
			col = fc.col
			if fc.edge && r.plain { // in text, the edges are lines of letters (design_3d_text.go)
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
			col = floorFinish(rm.floor, fc.col, p.x+hw, p.z, t/f*c.kpx, foot())
		case matWall:
			u := p.z // along the wall
			if n.z != 0 {
				u = p.x
			}
			ft := foot()
			col = wallFinish(rm.walls, fc.col, c.textCol, u, p.y, ft, c.gk, plas(u, p.y, ft), rm)
		case matBack:
			ft := foot()
			col = wallFinish(rm.walls, fc.col, c.textCol, p.x, p.y, ft, c.gk, plas(p.x, p.y, ft), rm)
		case matCeiling:
			col = plasterLit(fc.col, plas(p.x, p.z, foot()), ceilingGrain*c.gk)
		case matLamp, matShade:
			col = fc.col
		case matWindow:
			glass := false
			if col, glass = r.windowAt(c, fc, n, p, dir, t, nil); glass {
				// The town through the glass is as bright as it is outside.
				r.base[idx], r.mul[idx], r.add[idx], r.world.tint[idx] = col, 1, 0, 0
				return
			}
		}
		r.base[idx] = col

		// The light changes smoothly over a face: a pixel takes it from
		// the top left pixel of its 2×2 square when that shows the same
		// face (worked out before it), so it is worked out for a quarter
		// of the pixels. The colours above, its letters and lines, are
		// every pixel's own, so nothing is blurred.
		if anchor != idx && r.face[anchor] == fi && fc.mat != matWindow {
			r.mul[idx], r.add[idx], r.world.tint[idx] = r.mul[anchor], r.add[anchor], r.world.tint[anchor]
			return
		}
		if shadowOn {
			shade = 1 - 0.7*r.shadowAt(p.x+hw-float64(tx0), hh-p.y-float64(2*ty0))
		}

		// The light here.
		vx, vy, vz := -dir.x*dl, -dir.y*dl, -dir.z*dl
		fill := n.x*vx + n.y*vy + n.z*vz
		if fc.mat == matLamp {
			r.mul[idx], r.add[idx], r.world.tint[idx] = float32(0.85+0.15*fill), 0, 0
			return
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
		r.world.tint[idx] = r.world.xlightAt(n, p) // the other lights (design_3d_world.go)
	}
}

// dirAt is the ray of pixel idx of the last drawing (not of unit length).
func (r *room3D) dirAt(idx int) r3vec {
	v := &r.view
	j := idx / r.pw
	dx, dy := r.colDX[idx-j*r.pw], r.rowDY[j]
	return v.fwd.add(v.right.mul(dx)).add(v.up.mul(dy))
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
	runGap = 3 // this many spaces in a row part two lines of words
)

// words puts the words of every face that shows the flat board on the
// screen, the nearest face first.
func (r *room3D) words(v *r3view, rm *r3room) {
	w, h := rm.w, rm.h
	if len(r.chars) != w*h {
		r.chars = make([]r3char, w*h)
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
	for i := range r.order {
		r.surfaceWords(&r.order[i], v, rm) // each line on its face, the nearest face first
	}
}

// r3near is a face and how far in front of the camera it is.
type r3near struct {
	fi int
	d  float64
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
		if f < 0 || f == fi || r.faces[f].own < 0 || r.faces[f].own == own && !r.faces[fi].inside {
			continue // nothing, the face itself, the room, or its own block's sides (behind its front)
		}
		d := r.dirAt(i)
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
	if e := r.world.tint; e != nil && e[i] != 0 {
		// The other lights' colours too (design_3d_world.go).
		er, eg, eb := unpackTint(e[i])
		g := r.world.glowE
		return rgb32(byteOf(cr*(m+er)+a*lampWarm[0]+g*er), byteOf(cg*(m+eg)+a*lampWarm[1]+g*eg), byteOf(cb*(m+eb)+a*lampWarm[2]+g*eb))
	}
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
			} else if k := r.inkAt(row*w + col); k != nil {
				top, bot, glyph = k.fg, k.bg, k.s // the text layer's letter (design_3d_text.go)
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

// ---- words on their surfaces ----------------------------------------------------------------
//
// In 3D a face's words are lettering on it, not labels facing the camera:
// each line of words stays where it is on its face and goes with it. Its
// letters go where the face's cells are seen, so on a block turned or
// tilted the line slopes with the face. A face seen from behind, or so
// edge-on that its letters would crowd into each other, shows none; as it
// turns away its words fade into it; and seen too small to read (far off,
// or zoomed out), they are not drawn: letters cannot shrink in a terminal,
// so that is what getting smaller is. Letters are never mirrored. A nearer
// block hides the letters behind it.

const (
	wordsFadeFrom = 0.5  // a face turned this far from the camera (the cosine) starts to fade its words…
	wordsGone     = 0.25 // …and at this, they are gone
	wordsTight    = 0.5  // letters closer than this many cells apart on the screen are too small to read
	wordsLow      = 0.5  // a row seen this many rows high or less is too squashed to read
)

// surfaceWords puts face o's words on the screen, each line where it is on
// the face (see above).
func (r *room3D) surfaceWords(o *r3near, v *r3view, rm *r3room) {
	fi := o.fi
	fc := &r.faces[fi]
	wall := fc.mat == matBoard
	z := r.pts[fc.p0].z
	cam, right, up, fwd, f, px, py := v.cam, v.right, v.up, v.fwd, v.f, v.px, v.py
	hw, hh := rm.hw, rm.hh
	tw, tx0, ty0 := rm.tw, rm.tx0, rm.ty0
	proj := func(bx, by float64) (sx, sy float64, ok bool) {
		q := r3vec{bx - hw - cam.x, hh - 2*by - cam.y, z - cam.z}
		cz := q.dot(fwd)
		if cz < 0.25 {
			return 0, 0, false
		}
		return px + f*q.dot(right)/cz, (py - f*q.dot(up)/cz) / 2, true
	}
	// How squarely the face is seen, from its middle.
	mid := r3vec{float64(fc.cx0+fc.cx1)/2 - hw, hh - float64(fc.py0+fc.py1)/2, z}
	to := cam.sub(mid)
	facing := math.Abs(fc.n.dot(to)) / math.Max(math.Sqrt(to.dot(to)), 1e-9)
	fade := clampF((facing-wordsGone)/(wordsFadeFrom-wordsGone), 0, 1)
	if fade <= 0 {
		return // seen edge-on (or from behind): no words
	}
	sh := fc.tdy / 2 // a block on a layer: its drawing is kept below the board (layerCopies)
	tex := func(x, y int) *r3texel { return &r.tex[(y+sh-ty0)*tw+x-tx0] }
	covered := func(x, y int) bool { return wall && r.cover[(y-ty0)*tw+x-tx0] == r.stamp }
	num := fc.k - fc.n.dot(cam)
	w := rm.w
	var runs []r3run
	for cy := fc.py0 >> 1; cy < fc.py1>>1; cy++ {
		runs = r.rowRuns(runs[:0], fc, cy, tex, covered)
		for _, ru := range runs {
			by := float64(cy) + 0.5
			// The row's height on the screen: too squashed, it cannot be read.
			_, ya, ok1 := proj(float64(ru.x0)+0.5, float64(cy))
			_, yb, ok2 := proj(float64(ru.x0)+0.5, float64(cy+1))
			if !ok1 || !ok2 || math.Abs(yb-ya) < wordsLow {
				continue
			}
			// How far apart its letters are on the screen.
			x0, x1 := float64(ru.x0)+0.5, float64(ru.x1)-0.5
			if ru.x1-ru.x0 < 2 {
				x1 = x0 + 1
			}
			ax, _, okA := proj(x0, by)
			bx, _, okB := proj(x1, by)
			if !okA || !okB {
				continue
			}
			gap := (bx - ax) / (x1 - x0)
			if gap < wordsTight {
				continue // too small (far, or seen at a slant) to read
			}
			alpha := fade * clampF((gap-wordsTight)/0.2, 0, 1)
			if alpha < 0.15 {
				continue
			}
			// The letters side by side, the line's middle where the face's
			// middle of it is seen; each on the row where its own cell is.
			width := ru.x1 - ru.x0
			col := int(math.Floor((ax+bx)/2 - float64(width)/2 + 0.5))
			if ru.x1-ru.x0 < 2 {
				col = int(math.Floor(ax))
			}
			// Each word on one row: the line's row, or, where the face
			// slopes a whole row away by then, the row its middle is seen
			// on (a line steps between words, never inside one).
			_, msy, _ := proj(float64(ru.x0+ru.x1)/2, by)
			base := int(math.Floor(msy))
			wordRow := make(map[int]int, 4)
			for x := ru.x0; x < ru.x1; {
				if tex(x, cy).glyph == "" || tex(x, cy).glyph == " " {
					x++
					continue
				}
				e := x
				for e < ru.x1 && tex(e, cy).glyph != "" && tex(e, cy).glyph != " " {
					e++
				}
				_, sy, ok := proj(float64(x+e)/2, by)
				row := math.MinInt
				if ok {
					row = base
					if d := sy - (float64(base) + 0.5); math.Abs(d) >= 1 {
						row = base + int(math.Round(d))
					}
				}
				for k := x; k < e; k++ {
					wordRow[k] = row
				}
				x = e
			}
			// A word that would run into another face's or line's words, or
			// touch them with no space between, is left out whole (never
			// "footerlines").
			skip := make(map[int]bool, 4)
			for x := ru.x0; x < ru.x1; {
				if g := tex(x, cy).glyph; g == "" || g == " " {
					x++
					continue
				}
				e := x
				for e < ru.x1 && tex(e, cy).glyph != "" && tex(e, cy).glyph != " " {
					e++
				}
				row := wordRow[x]
				taken := false
				if row != math.MinInt && row >= 0 && row < rm.h {
					for c := col + x - ru.x0 - 1; c <= col+e-ru.x0 && !taken; c++ {
						taken = c >= 0 && c < w && r.chars[row*w+c].stamp == r.stamp
					}
				}
				for k := x; k < e; k++ {
					skip[k] = taken
				}
				x = e
			}
			for x := ru.x0; x < ru.x1; x++ {
				ti := int32((cy+sh-ty0)*tw + x - tx0)
				t := &r.tex[ti]
				c := col + x - ru.x0
				if skip[x] {
					if t.wide {
						x++
					}
					continue
				}
				row, ok := wordRow[x]
				g := t.glyph
				if g == "" || g == " " { // a space inside the line: the row of the word before it
					g = " "
					row, ok = wordRow[x-1]
				}
				ok = ok && row != math.MinInt
				if !ok || c < 0 || c >= w || row < 0 || row >= rm.h {
					if t.wide {
						x++
					}
					continue
				}
				at := row*w + c
				if r.chars[at].stamp == r.stamp || !r.inSight(at, int32(fi), fc.own, fc.n, num) {
					if t.wide {
						x++
					}
					continue // a nearer face's words, or a nearer block, there
				}
				r.setChar(at, int32(fi), g, t)
				if alpha < 1 { // turning away: into the face
					ch := &r.chars[at]
					ch.fg = mix32(ch.bg, ch.fg, alpha)
				}
				if t.wide {
					if c+1 < w {
						r.chars[at+1] = r3char{stamp: r.stamp, cont: true, face: int32(fi)}
					}
					x++
				}
			}
		}
	}
}
