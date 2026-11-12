package main

// ---- how a box looks: an open box seen from a little above ------------------------------
//
// A box is drawn as a real box seen from above and a little in front of
// it: the top of its walls (its rim) all round, lit, with a light lip
// where the rim meets the inside; under the far rim the inside of the far
// wall, darker the deeper it goes; a thin strip of the side walls' inside;
// its floor, in the shadow of the walls; and along the bottom the near
// wall's rim and its outside face. Things on the lower layers are darker
// and smaller (boxThing), and each thing casts a shadow on what is under it.
//
// The walls, floor and lid are painted in pixels, four a cell (two across,
// two down: ▀ and the quarter blocks ▘ ▝ ▖ ▗ …, two colours a cell), so a
// rounded corner, a slant or a rim one pixel thick shows. Each design has
// its own material and marks: a cardboard box's flaps folded back and its
// tape, a wooden crate's planks and nails, a metal tin's rolled rim and
// shine, a gift box's ribbon, a round hat box's curve and stripes, a
// hexagonal box's slanted sides. Shut, the lid lies over it all: its top
// with the design's marks, its front edge, and the box under it.

import (
	"fmt"
	"math"
)

// boxLook is a design's colours. The hex ones are for words and things
// drawn on it; the others are the pixels' colours.
type boxLook struct {
	wall, edge, floor, lid, extra string
	base, rim, floorC, lidC, ink  rgbf
	outline                       rgbf // the outermost pixels (lighter chosen, the accent while something is dropped on it)
}

var (
	rgbBlack = rgbf{}
	rgbWhite = rgbf{255, 255, 255}
)

// hex is c as "#rrggbb".
func (a rgbf) hex() string {
	c := func(v float64) int { return max(0, min(255, int(math.Round(v)))) }
	return fmt.Sprintf("#%02x%02x%02x", c(a.r), c(a.g), c(a.b))
}

// shade lightens c by v (v > 0) or darkens it (v < 0).
func (a rgbf) shade(v float64) rgbf {
	if v > 0 {
		return a.mix(rgbWhite, v)
	}
	return a.mix(rgbBlack, -v)
}

// near is how far apart two colours are (the largest difference of a channel).
func (a rgbf) near(b rgbf) float64 {
	return math.Max(math.Abs(a.r-b.r), math.Max(math.Abs(a.g-b.g), math.Abs(a.b-b.b)))
}

func boxLookOf(o *dOct) boxLook {
	design := boxDesign(o)
	base := map[int]string{boxCardboard: "#b5895a", boxCrate: "#a0703f", boxTin: "#9aa5af", boxHat: "#d4a5c0"}[design]
	if base == "" {
		base = o.Color // a gift box's paper, a hexagonal box: its own colour
	}
	if base == "" {
		base = "#b08a55"
	}
	b := toRGB(base)
	l := boxLook{base: b, rim: b, floorC: b.mix(rgbBlack, 0.62), lidC: b.mix(rgbWhite, 0.05), extra: "#d9c48f"}
	switch design {
	case boxCardboard:
		l.extra = "#e3d3a8" // tape
	case boxCrate:
		l.extra = "#c9ced3" // nails
		l.floorC = b.mix(rgbBlack, 0.55)
	case boxTin:
		l.extra = "#f1f5f8" // its shine
		l.floorC = b.mix(rgbBlack, 0.5)
	case boxGift:
		l.extra = "#e5c07b"                                   // gold ribbon
		l.floorC = toRGB("#ece4d4").mix(b, 0.15).shade(-0.42) // tissue paper
	case boxHat:
		l.extra = "#f6e6ef" // stripes
	}
	l.outline = b.shade(-0.3)
	l.wall, l.edge, l.floor, l.lid = b.hex(), b.shade(-0.5).hex(), l.floorC.hex(), l.lidC.hex()
	l.ink = toRGB(boxInk(l.wall))
	return l
}

// ---- its pixels ---------------------------------------------------------------------------------

// boxPix is a box's pixels, two across a cell and two down; on says
// which are painted (the others show the board).
type boxPix struct {
	w, h int
	c    []rgbf
	on   []bool
}

func newBoxPix(w, h int) *boxPix {
	return &boxPix{w: w, h: h, c: make([]rgbf, w*h), on: make([]bool, w*h)}
}

func (p *boxPix) set(x, y int, c rgbf) {
	if x >= 0 && y >= 0 && x < p.w && y < p.h {
		p.c[y*p.w+x], p.on[y*p.w+x] = c, true
	}
}

// boxQuad is the letter and colours that show a cell's four pixels (top
// left, top right, bottom left, bottom right) best in two colours: " " for
// one colour, ▀ for a top and a bottom, else the quarter block that parts
// them best.
func boxQuad(q [4]rgbf) (r, fg, bg string) {
	if q[0].near(q[1]) < 3 && q[2].near(q[3]) < 3 {
		top, bot := q[0].mix(q[1], 0.5), q[2].mix(q[3], 0.5)
		if top.near(bot) < 3 {
			return " ", top.hex(), top.mix(bot, 0.5).hex()
		}
		return "▀", top.hex(), bot.hex()
	}
	// The two most different pixels, and each pixel with the nearer of them.
	a, b, far := 0, 1, -1.0
	for i := range 4 {
		for j := i + 1; j < 4; j++ {
			if d := q[i].near(q[j]); d > far {
				a, b, far = i, j, d
			}
		}
	}
	mask := 0
	var fa, fb rgbf
	na, nb := 0.0, 0.0
	for i := range 4 {
		if q[i].near(q[a]) <= q[i].near(q[b]) {
			mask |= 1 << i
			fa = fa.mix(q[i], 1/(na+1))
			na++
		} else {
			fb = fb.mix(q[i], 1/(nb+1))
			nb++
		}
	}
	return quadGlyph[mask], fa.hex(), fb.hex()
}

// fromPix writes cell (x, y) of g from pixels p: the board's colour where
// none is painted. It says whether any was.
func (g *boxGrid) fromPix(p *boxPix, x, y int, act string) bool {
	bg := toRGB(cBG)
	var q [4]rgbf
	any := false
	for i := range 4 {
		px, py := 2*x+i%2, 2*y+i/2
		q[i] = bg
		if k := py*p.w + px; px < p.w && py < p.h && p.on[k] {
			q[i], any = p.c[k], true
		}
	}
	if !any {
		return false
	}
	r, fg, bgc := boxQuad(q)
	g.c[y][x] = boxCell{r: r, fg: fg, bg: bgc, act: act}
	return true
}

// boxShade is the shading of a material at a pixel: a fixed pattern that
// moves with the box (grain, planks, a shine, stripes …), as a lightening
// (> 0) or darkening (< 0).
func boxShade(design, x, y, pw int, along bool) float64 {
	g := grain(x, y)
	switch design {
	case boxCardboard: // paper fibres, and the faint ribs of corrugated card
		v := 0.03 * g
		if x%3 == 0 {
			v -= 0.025
		}
		return v
	case boxCrate: // wood: wavy lines of grain along each plank, a knot now and then
		a, c := x, y
		if !along {
			a, c = y, x
		}
		v := 0.13*math.Sin(float64(c)*1.7+float64(a)*0.06+2.4*grain(a/7, c/3)) + 0.03*g
		if grain(a/9, c/2+11) > 0.88 {
			v -= 0.14
		}
		return v
	case boxTin: // brushed metal with a band of shine a third of the way along
		t := (float64(x) + 0.5) / float64(max(pw, 1))
		return 0.24*math.Exp(-math.Pow((t-0.3)/0.16, 2)) - 0.05 + 0.03*grain(x/4, y)
	case boxGift: // wrapping paper: fine diagonal stripes
		if ((x+2*y)/3)%2 == 0 {
			return 0.05
		}
		return -0.02
	case boxHat:
		return 0.02 * g
	}
	return 0.025 * g // paint
}

// ---- its outline ----------------------------------------------------------------------------

// boxOutline is which pixels of a box w × h cells are in its outline (its
// walls and inside): a tin's round corners, a hat box's curve, a
// hexagon's slanted sides.
func boxOutline(s boxShape, w, h int) func(x, y int) bool {
	pw, ph := float64(2*w), float64(2*h)
	y0 := float64(2 * s.f0)
	round := func(x, y int, rx, ry float64) bool {
		fx, fy := float64(x)+0.5, float64(y)+0.5
		cx, cy := clampF(fx, rx, pw-rx), clampF(fy, y0+ry, ph-ry)
		dx, dy := (fx-cx)/rx, (fy-cy)/ry
		return dx*dx+dy*dy <= 1
	}
	switch s.design {
	case boxTin:
		return func(x, y int) bool { return round(x, y, 2.2, 1.6) }
	case boxHat:
		rx := float64(2*s.hatK + 2)
		return func(x, y int) bool { return round(x, y, rx, 4) }
	case boxHex:
		a := 2 * float64(boxHexSlant(w, h))
		return func(x, y int) bool {
			fx := float64(x) + 0.5
			cut := a * math.Abs(float64(y)+0.5-ph/2) / (ph / 2)
			return fx >= cut-0.25 && fx <= pw-cut+0.25
		}
	}
	return func(x, y int) bool { return float64(y) >= y0 }
}

// boxPaint paints a box's walls and floor (open) in pixels, and what
// stands beyond its outline (a cardboard box's flaps, a gift box's lid).
func boxPaint(s boxShape, lk boxLook, w, h int, shut bool) *boxPix {
	p := newBoxPix(2*w, 2*h)
	in := boxOutline(s, w, h)
	inside := make([]bool, w*h)
	for y := range h {
		for x := range w {
			inside[y*w+x] = s.inside(x, y, w, h)
		}
	}
	isIn := func(x, y int) bool { // pixel (x, y) is in an inside cell
		return x >= 0 && y >= 0 && x < 2*w && y < 2*h && inside[(y/2)*w+x/2]
	}
	outl := func(x, y int) bool { return x >= 0 && y >= 0 && x < 2*w && y < 2*h && in(x, y) }
	rows := s.insideRows(w, h)
	if len(rows) == 0 {
		rows = []int{s.top}
	}
	top, bot := 2*rows[0], 2*(rows[len(rows)-1]+1) // the inside's first and last pixel rows (+1)
	pw := 2 * w
	design := s.design

	// The walls: the rim, lit, a lip of light at its inner edge, darker at
	// its outer edge; the near wall's outside face below its rim.
	for y := 2 * s.f0; y < 2*h; y++ {
		for x := range pw {
			if !in(x, y) || isIn(x, y) {
				continue
			}
			along := y < top || y >= bot // the far and near walls run across; the sides down
			mat := lk.rim.shade(boxShade(design, x, y, pw, along))
			switch {
			case design == boxGift && (x == w-1 || x == w): // the ribbon, round it
				mat = toRGB(lk.extra)
			case design == boxGift && y >= top && y < bot && y/2 == (1+h)/2:
				mat = toRGB(lk.extra)
			case design == boxHat && y >= bot && (x/4)%2 == 0: // its striped band
				mat = mat.mix(toRGB(lk.extra), 0.45)
			case design == boxCardboard && y >= bot+1 && x >= w-3 && x < w+3: // tape down its front
				mat = toRGB(lk.extra).shade(0.02 * grain(x, y))
			}
			lip := isIn(x, y-1) || isIn(x-1, y) || isIn(x+1, y) || (y < top && isIn(x, y+1))
			edge := !outl(x, y-1) || !outl(x-1, y) || !outl(x+1, y) || !outl(x, y+1)
			c := mat
			switch {
			case y >= bot: // the near wall's outside face, darker lower down (its rim is the floor's last pixels)
				c = mat.shade(-0.3 - 0.15*float64(y-bot)/float64(max(2*h-bot, 1)))
				if !outl(x, y+1) || !outl(x-1, y) || !outl(x+1, y) {
					c = c.shade(-0.2)
				}
			case design == boxTin && y == top-2 && top >= 4 || design == boxTin && lip && y >= top: // its rolled rim, shining
				c = mat.shade(0.4)
			case design == boxTin && y == top-1:
				c = mat.shade(-0.18) // under the roll
			case lip:
				c = mat.shade(0.22)
			case edge:
				c = lk.outline.mix(mat, 0.35)
			case x >= w: // the right of the rim, a touch darker: the light is from the left
				c = mat.shade(-0.04)
			}
			p.set(x, y, c)
		}
	}

	// The inside: under the far rim the far wall's inside, darker toward the
	// floor; the side walls' insides, a pixel or two; the floor, in their shadow.
	colTop := make([]int, w) // each column's first inside row
	for x := range w {
		colTop[x] = h
		for y := range h {
			if inside[y*w+x] {
				colTop[x] = y
				break
			}
		}
	}
	ih := float64(bot - top)
	band := min(max(int(math.Round(0.2*ih)), 2), 8)
	sideW := 1
	if w >= 40 {
		sideW = 2
	}
	inner := lk.rim.shade(-0.3)
	for y := range h {
		lo, hi := -1, -1
		for x := range w {
			if inside[y*w+x] {
				if lo < 0 {
					lo = x
				}
				hi = x
			}
		}
		if lo < 0 {
			continue
		}
		for py := 2 * y; py < 2*y+2; py++ {
			for px := 2 * lo; px < 2*hi+2; px++ {
				if !isIn(px, py) {
					continue
				}
				dy := py - 2*colTop[px/2]
				dl, dr := px-2*lo, 2*hi+1-px
				var c rgbf
				switch {
				case py == bot-1: // the near wall's rim, seen over its top
					c = lk.rim.shade(0.2 + boxShade(design, px, py, pw, true))
				case dy < band: // the far wall's inside
					t := (float64(dy) + 0.5) / float64(band)
					c = inner.shade(boxShade(design, px, py, pw, true)).shade(-0.45 * t)
					if design == boxCrate && dy%3 == 2 {
						c = c.shade(-0.3) // where its planks meet
					}
				case dl < sideW: // the left wall's inside, away from the light
					c = inner.shade(-0.3 - 0.2*float64(py-top)/math.Max(ih, 1))
				case dr < sideW: // the right wall's inside, toward it
					c = inner.shade(-0.12 - 0.2*float64(py-top)/math.Max(ih, 1))
				default: // the floor, darker near the far wall and the left one
					k := clampF(float64(dy-band)/math.Max(ih*0.6, 1), 0, 1)
					c = lk.floorC.shade(-0.3 * (1 - k))
					if dl < sideW+2 {
						c = c.shade(-0.1)
					}
					switch design {
					case boxCrate: // planks, three pixel rows apart … six
						c = c.shade(boxShade(design, px, py, pw, true) * 0.6)
						if (py-top)%6 == 5 {
							c = c.shade(-0.35)
						}
					case boxTin:
						kk := float64(px-pw/3) + 0.8*float64(py)
						c = c.shade(0.12*math.Exp(-math.Pow(kk/6, 2)) + 0.02*grain(px/4, py))
					case boxHat:
						if (px/6)%2 == 0 {
							c = c.mix(toRGB(lk.extra), 0.05)
						}
					default:
						c = c.shade(0.03 * grain(px, py))
					}
				}
				p.set(px, py, c)
			}
		}
	}

	// Beyond the outline, above it.
	switch {
	case design == boxCardboard && !shut && s.f0 > 0: // two flaps, folded back
		flap := lk.rim.mix(toRGB("#f0dcb0"), 0.35) // their inside, paler
		for y := 0; y < 2*s.f0; y++ {
			for x := range pw {
				gap := x == w-1 || x == w // where the two meet
				inset := y - 2*s.f0 + 2   // they widen away from the box
				if gap || x < inset || x > pw-1-inset {
					continue
				}
				c := flap.shade(boxShade(design, x, y, pw, true))
				if y == 0 {
					c = c.shade(0.1) // their far edge, catching the light
				}
				p.set(x, y, c)
			}
		}
	case design == boxGift && !shut && s.f0 > 0: // its lid, off, behind it
		lid := lk.lidC.shade(0.08)
		for x := 4; x < pw-4; x++ {
			c := lid.shade(boxShade(design, x, 1, pw, true))
			if x == w-1 || x == w {
				c = toRGB(lk.extra)
			}
			p.set(x, 2*s.f0-1, c)
		}
	}
	return p
}

// boxLidPaint paints a box's lid in pixels over the inside rows given;
// whole, over all of it: its top, with the design's marks (a cardboard
// box's tape, a crate's planks, a tin's pressed ring, a gift's ribbon …),
// lit at its top and left edges, and its front edge, with the box's front
// under it.
func boxLidPaint(p *boxPix, s boxShape, lk boxLook, w, h int, rows []int, whole bool) {
	in := boxOutline(s, w, h)
	pw := 2 * w
	y0, y1 := 2*rows[0], 2*(rows[len(rows)-1]+1) // the lid's pixel rows
	if whole {
		y0, y1 = 2*s.f0, 2*(h-s.bottom)
	}
	isLid := func(x, y int) bool {
		if x < 0 || y < y0 || x >= pw || y >= y1 || !in(x, y) {
			return false
		}
		return whole || s.inside(x/2, y/2, w, h)
	}
	midY := (y0 + y1) / 2
	for y := y0; y < y1; y++ {
		for x := range pw {
			if !isLid(x, y) {
				continue
			}
			c := lk.lidC.shade(boxShade(s.design, x, y, pw, true))
			switch s.design {
			case boxCardboard: // two flaps meeting in the middle, taped
				switch {
				case x >= w-2 && x < w+2:
					c = toRGB(lk.extra).shade(0.03 * grain(x, y))
				case x == w-3 || x == w+2:
					c = c.shade(-0.12)
				}
			case boxCrate: // planks, nailed at each end
				if (y-y0)%4 == 3 {
					c = c.shade(-0.35)
				} else if (y-y0)%4 == 1 && (x == 3 || x == pw-4) {
					c = toRGB(lk.extra)
				}
			case boxTin: // a ring pressed into it
				ring := func(k int) bool {
					return (x == k || x == pw-1-k) && y >= y0+k && y < y1-k || (y == y0+k || y == y1-1-k) && x >= k && x < pw-k
				}
				if whole && ring(4) {
					c = c.shade(0.18)
				} else if whole && ring(5) {
					c = c.shade(-0.15)
				}
			case boxGift: // the ribbon, across and down
				if x == w-1 || x == w || y == midY-1 || y == midY {
					c = toRGB(lk.extra).shade(0.04 * grain(x, y))
				}
			case boxHat: // stripes
				if (x/4)%2 == 0 {
					c = c.mix(toRGB(lk.extra), 0.35)
				}
			case boxHex: // a line round inside its edge
				if whole && in(x, y) && (!in(x-3, y) || !in(x+3, y)) && !(!in(x-1, y) || !in(x+1, y)) {
					c = c.shade(0.12)
				}
			}
			switch {
			case y == y1-1: // its front edge
				c = lk.lidC.shade(-0.38)
			case !isLid(x, y-1): // lit at its top edge …
				c = c.shade(0.25)
			case !isLid(x-1, y): // … and its left
				c = c.shade(0.12)
			case !isLid(x+1, y):
				c = c.shade(-0.2)
			}
			p.set(x, y, c)
		}
	}
	if !whole {
		return
	}
	// The box under the lid: its front, in the lid's shadow at the top.
	for y := y1; y < 2*h; y++ {
		for x := range pw {
			if in(x, y) {
				c := lk.rim.shade(boxShade(s.design, x, y, pw, true) - 0.3)
				if y == y1 {
					c = c.shade(-0.35)
				}
				if s.design == boxCardboard && x >= w-3 && x < w+3 {
					c = toRGB(lk.extra).shade(-0.15)
				}
				p.set(x, y, c)
			}
		}
	}
}
