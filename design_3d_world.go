package main

// ---- windows, the town outside them, and more lights ----------------------------------
//
// The room has windows in its side walls and in the wall behind the
// camera. Through the glass is a town, drawn as it is seen, pixel by
// pixel: a sky that follows the real time of day (dawn, day, dusk and
// night colours, the sun or the moon, stars at night, clouds drifting),
// a street with people walking along it, and rows of buildings behind,
// nearer ones moving past further ones as the camera moves.
//
// The town is made of text, as the room is. The people are little
// figures of letters ( o  /|\  / \ ), the buildings' windows are grids of
// letters (# ▒ █ = H ▓ +), lit at night, the shops have signs in words
// (CAFE, BOOKS, OPEN 24H …) and the stars are * + . ·, all painted in
// the terminal's own font as the words on the cards are
// (design_3d_glyphs.go).
//
// The room has more lights than the hanging lamp too: a cool panel on
// the ceiling, a warm lamp on a stand, a magenta strip of light low on
// the left wall, and the daylight through each wall's windows,
// which takes the sky's colour. Each lights what faces it in its colour,
// less with distance (no shadows: only the hanging lamp casts those).
//
// Everything here is worked out the same way on the graphics card
// (design_3d_world_glsl.go): change one, change the other.

import (
	"math"
	"time"

	tea "charm.land/bubbletea/v2"
)

// matWindow is a window's face: its frame, and the town through its glass.
const matWindow = matShade + 1

// xlightsOff: tests turn the extra lights off, to see what they add.
var xlightsOff bool

// room3DClock is the time the sky outside follows (tests set it).
var room3DClock = time.Now

// worldWalkers is how many people walk past each wall: two lanes (the near
// pavement and the far one) of walkLane each.
const walkLane = 7
const worldWalkers = 2 * walkLane

// maxXLights is how many extra lights there can be (the card has room for so many).
const maxXLights = 8

// r3world is the windows, the town and the extra lights of this frame.
type r3world struct {
	sky     r3sky
	lamps   []r3xlight
	walk    [3 * worldWalkers]r3walker // per wall (left, right, behind), per lane
	h       float64                    // a storey: the room's height
	g       float64                    // the street's height (a little under the floor)
	ou      float64                    // a letter's width outside (twice as tall)
	mid     r3vec                      // the room's middle: where each wall's walkers walk round
	cloud   float64                    // how far the clouds have drifted
	twink   uint32                     // the stars' twinkle, changing twice a second
	glowE   float64                    // the extra lights' own glow (the theme's lamp glow)
	tint    []uint32                   // per pixel: the extra lights' colour on it (see packTint)
	slots   [64]int32                  // the card's layer of each of outChars (design_3d_world_gpu.go)
	tick    bool                       // a room3DOutsideMsg is on its way
	letters bool                       // letters mode: big letters of the town are the terminal's own (townLetters)
}

// r3xlight is one extra light: where it is, its colour (times its
// strength) and how quickly it falls off (1/reach²).
type r3xlight struct {
	at  r3vec
	col [3]float64
	inv float64
}

// r3walker is one person outside: where along the street (from the
// wall's middle), the figure and step (code: figure·2 + step, skin·16)
// and the coat's colour.
type r3walker struct {
	x    float64
	code int
	coat uint32
}

// r3sky is the sky at this time of day.
type r3sky struct {
	top, horizon, cloud, haze, sunCol uint32
	sun, moon                         r3vec   // unit directions to them
	day                               float64 // 0 at night, 1 by day
	stars                             float64 // how much the stars show
	lit                               float64 // the share of the town's windows lit
	light                             [3]float64
}

// ---- the letters the town is drawn with ------------------------------------------------

// walkerFigs: each figure in two steps, three rows of three letters.
var walkerFigs = [][3]string{
	{" o ", "/|\\", "/ \\"}, {" o ", "/|\\", " | "},
	{" o ", "/|]", "/ \\"}, {" o ", "/|]", " | "}, // with a bag
	{"\\o/", " | ", "/ \\"}, {"\\o/", " | ", " | "}, // waving
	{" @ ", "/#\\", "/ \\"}, {" @ ", "/#\\", " | "}, // in a thick coat
	{" O ", "<|>", "/ \\"}, {" O ", "<|>", " | "},
}

// signWords are the shops' signs.
var signWords = []string{"CAFE", "HOTEL", "BAR", "BOOKS", "PIZZA", "OPEN 24H", "DINER", "JAZZ",
	"BANK", "TAXI", "RAMEN", "NEWS", "METRO", "SHOP", "CINEMA", "FLOWERS"}

// winChars are the buildings' windows; starChars the stars.
var winChars = []string{"#", "▒", "█", "=", "H", "▓", "+"}
var starChars = []string{"*", "+", ".", "·"}

// mirrors: a figure walking the other way.
var mirrorOf = map[string]string{"/": "\\", "\\": "/", "<": ">", ">": "<", "[": "]", "]": "["}

// outChars is every letter above, once ([0] is a space: nothing); the
// tables below are the same letters as places in it.
var outChars, figIdx, wordIdx, wordLen, winIdx, starIdx, mirrorIdx = outTables()

const signMax = 8 // the longest sign

func outTables() (chars []string, fig, word, wlen, win, star, mirror []int) {
	at := map[string]int{" ": 0}
	chars = []string{" "}
	idx := func(s string) int {
		if i, ok := at[s]; ok {
			return i
		}
		at[s] = len(chars)
		chars = append(chars, s)
		return len(chars) - 1
	}
	for _, f := range walkerFigs {
		for _, row := range f {
			for _, c := range graphemes(row) {
				fig = append(fig, idx(c))
			}
		}
	}
	for _, w := range signWords {
		g := graphemes(w)
		wlen = append(wlen, len(g))
		for i := range signMax {
			c := 0
			if i < len(g) {
				c = idx(g[i])
			}
			word = append(word, c)
		}
	}
	for _, c := range winChars {
		win = append(win, idx(c))
	}
	for _, c := range starChars {
		star = append(star, idx(c))
	}
	for _, s := range []string{"/", "\\", "<", ">", "[", "]"} {
		idx(s)
	}
	mirror = make([]int, len(chars))
	for i, c := range chars {
		mirror[i] = i
		if m, ok := mirrorOf[c]; ok {
			mirror[i] = at[m]
		}
	}
	return
}

// outGlyphs is outChars drawn (nil where the font has no such letter).
func outGlyphs() []*r3glyph {
	gs := make([]*r3glyph, len(outChars))
	for i, c := range outChars {
		if i > 0 {
			gs[i] = glyphOf(c, c != "·" && c != ".", false)
		}
	}
	return gs
}

// whash is a fixed random value from 0 to 1 for three whole numbers.
func whash(a, b, c uint32) float64 {
	x := a*0x9E3779B1 ^ b*0x85EBCA77 ^ c*0xC2B2AE3D
	x ^= x >> 15
	x *= 0x2C1B3C6D
	x ^= x >> 12
	x *= 0x297A2D39
	x ^= x >> 15
	return float64(x>>8) / 16777216
}

// ihash is whash for whole numbers that may be below 0.
func ihash(a, b, c int) float64 { return whash(uint32(int32(a)), uint32(int32(b)), uint32(int32(c))) }

func smoothF(lo, hi, x float64) float64 {
	t := clamp01((x - lo) / (hi - lo))
	return t * t * (3 - 2*t)
}

// ---- the sky -----------------------------------------------------------------------------

// skyKey is the sky's colours with the sun this high (sin of its height).
type skyKey struct {
	s                         float64
	top, horizon, cloud, haze string
	light                     string
	strength                  float64
}

var skyKeys = []skyKey{
	{-1, "#070b1c", "#141d3a", "#1d2440", "#121a33", "#5a6fa8", 0.05},
	{-0.30, "#070b1c", "#141d3a", "#1d2440", "#121a33", "#5a6fa8", 0.05},
	{-0.12, "#141a45", "#3a3466", "#3b3a63", "#2c2c55", "#7a6fb8", 0.12},
	{-0.02, "#2c3a78", "#ee8a5f", "#c97a7a", "#a86a70", "#ff9a66", 0.3},
	{0.07, "#5684c2", "#ffc58a", "#f4d6c8", "#d9b9a0", "#ffd0a0", 0.45},
	{0.25, "#3a7fd5", "#b5d9f5", "#f4f7fb", "#a9c4dc", "#fff4e4", 0.55},
	{1, "#2f74cf", "#a8d2f3", "#ffffff", "#a3c1dc", "#ffffff", 0.6},
}

// skyAt is the sky at time now: where the sun is (from the day of the
// year and the hour: up longer in summer) and its colours.
func skyAt(now time.Time) r3sky {
	hr := float64(now.Hour()) + float64(now.Minute())/60 + float64(now.Second())/3600
	season := math.Cos(2 * math.Pi * float64(now.YearDay()-172) / 365.25) // 1 midsummer, −1 midwinter
	length := 12 + 3.2*season                                             // hours of daylight
	rise := 12.5 - length/2
	var phi, el float64
	if p := (hr - rise) / length; p >= 0 && p <= 1 {
		phi = math.Pi * p
		el = (42 + 22*season) * math.Pi / 180 * math.Sin(phi)
	} else {
		q := modF(hr-rise-length, 24) / (24 - length)
		phi = math.Pi + math.Pi*q
		el = 50 * math.Pi / 180 * math.Sin(phi)
	}
	// East is to the left (−x), south behind the camera (+z), west right.
	sun := r3vec{-math.Cos(phi) * math.Cos(el), math.Sin(el), math.Sin(phi) * math.Cos(el)}
	var k skyKey
	s := sun.y
	sk := r3sky{sun: sun}
	for i := 1; i < len(skyKeys); i++ {
		a, b := skyKeys[i-1], skyKeys[i]
		if s > b.s && i < len(skyKeys)-1 {
			continue
		}
		t := clamp01((s - a.s) / (b.s - a.s))
		m := func(x, y string) uint32 { return mix32(hex32(x), hex32(y), t) }
		k.strength = a.strength + (b.strength-a.strength)*t
		sk.top, sk.horizon, sk.cloud, sk.haze = m(a.top, b.top), m(a.horizon, b.horizon), m(a.cloud, b.cloud), m(a.haze, b.haze)
		lr, lg, lb := chans(m(a.light, b.light))
		sk.light = [3]float64{lr / 255 * k.strength, lg / 255 * k.strength, lb / 255 * k.strength}
		break
	}
	sk.day = smoothF(-0.12, 0.2, s)
	sk.stars = 1 - smoothF(-0.2, -0.04, s)
	sk.lit = 0.06 + 0.5*(1-sk.day)
	sk.sunCol = mix32(hex32("#ff7a3c"), hex32("#fff6dc"), smoothF(-0.02, 0.3, s))
	sk.moon = r3vec{-sun.x, -sun.y * 0.8, -sun.z}.norm()
	return sk
}

// skyCol is the sky seen along unit direction d.
func (w *r3world) skyCol(d r3vec, f, kx, ky float64, lt *r3letter) uint32 {
	sk := &w.sky
	e := d.y
	col := mix32(sk.horizon, sk.top, smoothF(-0.02, 0.55, e))
	if e > 0.02 {
		// The clouds: soft, on a sky far above, drifting.
		cp := 0.35 / e
		cx, cz := d.x*cp+w.cloud, d.z*cp
		dn := 0.6*noise(cx, cz) + 0.4*noise(cx*2.3+5, cz*2.3+9)
		cov := smoothF(0.05, 0.45, dn) * smoothF(0.02, 0.12, e)
		col = mix32(col, sk.cloud, 0.85*cov)
		// The stars, letters on the sky.
		if sk.stars > 0 && e > 0.03 {
			const sw, sh = 0.014, 0.028
			el := math.Asin(math.Min(e, 1))
			gx, gy := math.Atan2(d.x, d.z)/sw, el/sh
			i, j := int(ffloor(gx)), int(ffloor(gy))
			if ihash(i, j, 77) < 0.08 {
				b := (0.55 + 0.45*ihash(i, j, 79)) * (0.75 + 0.25*ihash(i, j, int(w.twink)))
				du := 1 / (f * kx) / sw / math.Max(math.Cos(el), 0.2)
				dv := 1 / (f * ky) / sh
				k := sk.stars * b * (1 - cov)
				ch := starIdx[int(ihash(i, j, 78)*float64(len(starIdx)))]
				if g, ok := w.letterAt(ch, mix32(col, 0xfff6dc, k), gx-float64(i), 1-(gy-float64(j)), du, dv, lt); ok {
					col = mix32(col, 0xfff6dc, g*k)
				}
			}
		}
	}
	// The sun: a glow round it, and its disc.
	if cs := d.dot(sk.sun); cs > 0 {
		s2 := cs * cs
		s4 := s2 * s2
		s8 := s4 * s4
		s16 := s8 * s8
		col = addCol(col, sk.sunCol, 0.4*s16*s8)
		col = mix32(col, sk.sunCol, clamp01((cs-0.9988)/0.0005))
	}
	// The moon, a crescent.
	if cm := d.dot(sk.moon); cm > 0.999 {
		cut := r3vec{sk.moon.x + 0.012, sk.moon.y + 0.008, sk.moon.z}.norm()
		k := clamp01((cm-0.9993)/0.0003) * (1 - clamp01((d.dot(cut)-0.9993)/0.0003))
		col = mix32(col, 0xeef0f7, k*sk.stars)
	}
	return col
}

// addCol is a plus k of b, each kept within 255.
func addCol(a, b uint32, k float64) uint32 {
	ar, ag, ab := chans(a)
	br, bg, bb := chans(b)
	return rgb32(byteOf(ar+br*k+0.5), byteOf(ag+bg*k+0.5), byteOf(ab+bb*k+0.5))
}

// r3letter is the town's letter at a point (for letters mode, where they
// are the terminal's own): which of outChars, its colour, where in its
// cell the point is, and a pixel's size in its cell.
type r3letter struct {
	ch           int
	ink          uint32
	u, v, du, dv float64
}

// letterBig: a letter of the town this big on the screen (a pixel this
// share of its cell or less: two pixels a cell in letters mode) is the
// terminal's own letter in letters mode, not painted.
const letterBig = 0.6

// letterAt is how much of letter ch covers (u, v) of its cell, a pixel
// seen du by dv of it, and false when there is no such letter. lt (when
// given) notes it as the letter at the point.
func (w *r3world) letterAt(ch int, ink uint32, u, v, du, dv float64, lt *r3letter) (float64, bool) {
	g := w.glyph(ch)
	if g == nil {
		return 0, false
	}
	if lt != nil {
		*lt = r3letter{ch, ink, u, v, du, dv}
	}
	if w.letters && du <= letterBig && dv <= letterBig {
		return 0, true // a letter of the terminal's instead (townLetters)
	}
	return g.cover(u, v, du, dv), true
}

// glyph is outChars[i] drawn, or nil.
func (w *r3world) glyph(i int) *r3glyph {
	gs := outGlyphsOnce()
	if i <= 0 || i >= len(gs) {
		return nil
	}
	return gs[i]
}

var outGlyphsCache []*r3glyph

func outGlyphsOnce() []*r3glyph {
	if outGlyphsCache == nil {
		outGlyphsCache = outGlyphs()
	}
	return outGlyphsCache
}

// ---- the town ----------------------------------------------------------------------------

// outLayer is a row of buildings: how far out (storeys), how wide each
// building's plot is, how low and high they are, the gaps between them
// and how much of the haze is over them.
type outLayer struct{ s, slot, lo, hi, gap, haze float64 }

var outLayers = [3]outLayer{
	{3.6, 2.0, 1.0, 3.2, 0.06, 0.08},
	{8, 2.6, 1.6, 5.5, 0.18, 0.3},
	{18, 3.2, 3, 11, 0.3, 0.55},
}

// The street, in storeys out from the wall: the lanes the people walk
// in, the lamp posts, the kerb, the road and its middle line.
const (
	laneNear, laneFar = 0.32, 3.25
	postAt, postEvery = 0.62, 4.5
	kerbAt, roadAt    = 0.7, 0.8
	roadEnd, middleAt = 3.0, 1.9
)

var facadeCols = []string{"#9a5b4a", "#7d7f86", "#b8a58a", "#5f6f80", "#8c6f5a", "#a3a7a0", "#6b5a70"}
var neonCols = []string{"#ff4d6a", "#3ee6ff", "#ffe14d", "#6dff7a", "#ff6af0", "#ff9a3c"}
var coatCols = []string{"#e05a4f", "#4f8fe0", "#5fbf6a", "#e0c04f", "#a86fe0", "#e0904f", "#e8e8e8", "#3a3f4a"}
var skinCols = []string{"#f1c9a5", "#d9a37a", "#a8714a", "#6b4a32"}

// The same colours as numbers (they are used for every pixel of the town).
var facadeC, neonC, skinC = hexes(facadeCols), hexes(neonCols), hexes(skinCols)

func hexes(hs []string) []uint32 {
	cs := make([]uint32, len(hs))
	for i, h := range hs {
		cs[i] = hex32(h)
	}
	return cs
}

// wallOf is which wall a window faces in from: 0 left, 1 right, 2 behind.
func wallOf(n r3vec) int {
	if n.x > 0.5 {
		return 0
	}
	if n.x < -0.5 {
		return 1
	}
	return 2
}

// outside is the town seen through the glass at p along ray dir (as
// draw's rays: dir · fwd = 1, so t along it is how far in front of the
// camera p is), from a window facing in along n.
func (w *r3world) outside(n, p, dir r3vec, t, f, kx, ky float64, lt *r3letter) uint32 {
	sk := &w.sky
	out, along := r3vec{-n.x, 0, -n.z}, r3vec{n.z, 0, -n.x}
	wall := wallOf(n)
	dOut := math.Max(dir.dot(out), 1e-6)
	ka, kyy := dir.dot(along)/dOut, dir.y/dOut
	a0, y0 := p.dot(along), p.y
	dl := dir.len()
	H, G, ou := w.h, w.g, w.ou
	slant := math.Min(dl/dOut, 6)
	// A pixel's size on a plane facing out, s units beyond the glass,
	// in units across and down.
	foot := func(s float64) (float64, float64) {
		z := (t + s/dOut) / f * slant
		return z / kx, z / ky
	}
	sG := math.Inf(1) // where the ray meets the street
	if dir.y < 0 {
		sG = (G - y0) / kyy
	}
	sHit, col, hit := sG, uint32(0), false
	for k := range outLayers {
		L := &outLayers[k]
		s := L.s * H
		if s >= sG {
			break
		}
		hy := y0 + s*kyy - G
		sw := L.slot * H
		a := a0 + s*ka + float64(wall)*sw*7.31
		fi := ffloor(a / sw)
		i := int(fi)
		fx := a/sw - fi
		gap := L.gap * ihash(i, wall, k+10)
		hb := ihash(i, wall, k)
		h := (L.lo + (L.hi-L.lo)*hb*hb) * H
		if fx < gap || fx > 1-0.5*gap || hy >= h {
			continue
		}
		du, dv := foot(s)
		col = w.facade(k, i, wall, (fx-gap)*sw, (1-1.5*gap)*sw, hy, h, du, dv, lt)
		col = mix32(col, sk.haze, L.haze*(0.5+0.5*sk.day))
		sHit, hit = s, true
		break
	}
	if !hit {
		if sG < math.Inf(1) {
			col = w.street(sG, a0+sG*ka, wall)
		} else {
			col = w.skyCol(dir.mul(1/dl), f, kx, ky, lt)
		}
	}
	day := sk.day
	people := func(lane int) {
		s := laneNear * H
		if lane == 1 {
			s = laneFar * H
		}
		if s >= sHit {
			return
		}
		hy := y0 + s*kyy - G
		if hy < 0 || hy >= 6*ou {
			return
		}
		a := a0 + s*ka - w.mid.dot(along)
		du, dv := foot(s)
		for i := range walkLane {
			wk := &w.walk[wall*worldWalkers+lane*walkLane+i]
			dx := (a-wk.x)/ou + 1.5
			if dx < 0 || dx >= 3 {
				continue
			}
			c, rr := int(dx), 2-int(hy/(2*ou))
			fig := wk.code & 15
			var ch int
			if lane == 1 {
				ch = mirrorIdx[figIdx[fig*9+rr*3+2-c]]
			} else {
				ch = figIdx[fig*9+rr*3+c]
			}
			ink := wk.coat
			if rr == 0 {
				ink = skinC[(wk.code>>4)&3]
			} else if rr == 2 {
				ink = mix32(ink, 0x202020, 0.55)
			}
			ink = shade32k(ink, 0.4+0.6*day+0.25*(1-day))
			cv, ok := w.letterAt(ch, ink, dx-float64(c), 1-(hy/(2*ou)-float64(2-rr)), du/ou, dv/(2*ou), lt)
			if !ok {
				continue
			}
			col = mix32(col, ink, cv)
			return
		}
	}
	people(1)
	// The lamp posts on the near pavement, lit at night.
	if s := postAt * H; s < sHit {
		hy := y0 + s*kyy - G
		a := a0 + s*ka
		pe := postEvery * H
		dx := (modF(a, pe) - pe/2) / H
		du, _ := foot(s)
		top := 1.35
		if hy >= 0 && hy < top*H {
			col = mix32(col, mix32(0x2a2c30, sk.horizon, 0.15), cover(dx, -0.018, 0.018, du/H))
		}
		r2 := dx*dx + (hy/H-top)*(hy/H-top)
		if r2 < 0.0049 {
			col = mix32(col, mix32(0x50545a, 0xffe2a8, 1-day), clamp01((0.0049-r2)/0.002))
		} else if day < 1 {
			col = addCol(col, 0xffc070, 0.35*(1-day)/(1+r2*60))
		}
	}
	people(0)
	return col
}

// shade32k is c times k, each kept within 255.
func shade32k(c uint32, k float64) uint32 {
	r, g, b := chans(c)
	return rgb32(byteOf(r*k+0.5), byteOf(g*k+0.5), byteOf(b*k+0.5))
}

// facade is building i of row k (of wall's town): lx units from its
// left, hy up from the street, of width wd and height h; a pixel is du
// units across and dv down.
func (w *r3world) facade(k, i, wall int, lx, wd, hy, h, du, dv float64, lt *r3letter) uint32 {
	sk := &w.sky
	day := sk.day
	base := facadeC[int(ihash(i, wall, k+20)*float64(len(facadeC)))]
	col := shade32k(base, 0.2+0.8*day)
	if hy < w.h && day < 1 { // the street lamps' light on the ground floor
		col = addCol(col, 0xffb060, 0.18*(1-day)*(1-hy/w.h))
	}
	cw := 1.15 * w.ou
	ch := 2 * cw
	if hy > h-0.35*ch {
		return mix32(col, 0, 0.3) // the cornice
	}
	ncol := int(wd / cw)
	j, q := int(ffloor(lx/cw)), int(ffloor(hy/ch))
	u, v := lx/cw-float64(j), 1-(hy/ch-float64(q))
	if j < 1 || j >= ncol-1 {
		return col
	}
	if k == 0 && q == 2 {
		// The shop's sign: a word in neon on a dark board.
		wi := int(ihash(i, wall, 50) * float64(len(signWords)))
		n := wordLen[wi]
		at := j - (ncol-n)/2
		col = mix32(col, 0x15161a, 0.8)
		if at >= 0 && at < n {
			neon := neonC[int(ihash(i, wall, 51)*float64(len(neonC)))]
			neon = shade32k(neon, 0.75+0.25*(1-day))
			if g, ok := w.letterAt(wordIdx[wi*signMax+at], neon, u, v, du/cw, dv/ch, lt); ok {
				col = mix32(col, neon, g)
			}
		}
		return col
	}
	if k == 0 && q < 2 {
		// The shop's window, lit inside at night.
		glass := mix32(mix32(0x1c2026, sk.horizon, 0.45*day), 0xffd590, 0.7*(1-day))
		return mix32(col, glass, cover(u, 0.1, 0.9, du/cw)*cover(hy, 0.15*ch, 1.9*ch, dv))
	}
	style := int(ihash(i, wall, k+30) * 3)
	first := 1
	if k == 0 {
		first = 3
	}
	if q < first || q >= int(h/ch)-1 || style == 1 && j%2 == 0 || style == 2 && j%3 == 0 {
		return col
	}
	lit := ihash(i*131+j, q, wall*7+k) < sk.lit
	ink := mix32(mix32(col, 0, 0.4), sk.horizon, 0.4*day)
	bg := mix32(col, 0, 0.12)
	if lit {
		ink = 0xffd98a
		if ihash(i, j, q+900) < 0.25 {
			ink = 0xbfe0ff
		}
		bg = mix32(col, ink, 0.22)
	}
	g, ok := w.letterAt(winIdx[int(ihash(i, wall, k+40)*float64(len(winIdx)))], ink, u, v, du/cw, dv/ch, lt)
	if !ok {
		return bg
	}
	return mix32(bg, ink, g)
}

// street is the ground s units out from the wall, a along it.
func (w *r3world) street(s, a float64, wall int) uint32 {
	sk := &w.sky
	H := w.h
	sh := s / H
	var col uint32
	switch {
	case sh < kerbAt:
		col = 0x8a8a86
	case sh < roadAt:
		col = 0xa8a8a2
	case sh < roadEnd:
		col = 0x3a3d42
		if absF(sh-middleAt) < 0.04 && modF(a/H, 1.2) < 0.6 {
			col = 0xe8e0b0
		}
	case sh < outLayers[0].s:
		col = 0x8a8a86
	default:
		col = 0x2a2a2c
	}
	col = shade32k(col, 0.35+0.65*sk.day)
	if sk.day < 1 {
		dx := (modF(a, postEvery*H) - postEvery*H/2) / H
		dz := sh - postAt
		col = addCol(col, 0xffb060, 0.5*(1-sk.day)/(1+(dx*dx+dz*dz)*4))
	}
	return mix32(col, sk.haze, clamp01(s/(30*H)))
}

// ---- the windows in the walls ------------------------------------------------------------

// windowAt is the window's colour at p: its frame (glass false: it is lit
// as the walls are) or the town through its glass (glass true: not lit).
func (r *room3D) windowAt(c *r3shade, fc *r3face, n, p, dir r3vec, t float64, lt *r3letter) (uint32, bool) {
	q := p.sub(fc.o)
	u, v := q.dot(fc.eu), q.dot(fc.ev)
	lu, lv := fc.lu, fc.lv
	dl := dir.len()
	foot := t / c.f * c.kpx / math.Max(-n.dot(dir)/dl, 0.12)
	fw := 0.07 * math.Min(lu, lv)
	mw := 0.3 * fw
	d := math.Min(math.Min(u, lu-u), math.Min(v, lv-v))
	k := cover(d, -1, fw, foot)
	k = math.Max(k, cover(u-lu/2, -mw, mw, foot))
	k = math.Max(k, cover(v-0.66*lv, -mw, mw, foot))
	if k >= 0.5 {
		col := fc.col
		if d < fw && d > 0.7*fw {
			col = mix32(col, 0, 0.18) // the frame's inner edge, in its own shadow
		}
		return col, false
	}
	col := r.world.outside(n, p, dir, t, c.f, c.kx, c.ky, lt)
	return mix32(col, mix32(fc.col, 0, 0.5), 2*k), true
}

// worldScene works out the sky, the people and the lights for this frame,
// and puts the windows and the lights' fittings in the room's faces.
func (r *room3D) worldScene(rm *r3room) {
	w := &r.world
	now := room3DClock()
	w.sky = skyAt(now)
	H := rm.cy - rm.fy // the room's height
	w.h = 0.5 * H      // a storey outside: the room is a tall hall
	w.g = rm.fy - 0.03*H
	w.ou = 0.1 * w.h
	w.mid = r3vec{(rm.wx0 + rm.wx1) / 2, 0, rm.fz / 2}
	secs := float64(now.UnixNano()%(1<<50)) / 1e9
	w.cloud = modF(secs*0.004, 64)
	w.twink = uint32(int64(secs * 2))
	w.walkers(secs)
	w.letters = !r.pix
	outGlyphsOnce() // drawn here, before the pixels are shared out to be drawn side by side
	light := lightFor(cBG)
	w.glowE = light.glow
	bg := hex32(cBG)

	// The windows: on each side wall past the blocks, and on the wall behind.
	frame := mix32(hex32("#d6cec0"), bg, 0.6)
	text := hex32(cText)
	f := r3face{mat: matWindow, own: -1, col: frame, gloss: 0.25}
	yb, yt := rm.fy+0.1*H, rm.fy+0.8*H
	var sums [3]r3vec
	var counts [3]int
	put := func(wall int, lo, hi float64) {
		span := hi - lo
		if span < 0.4*H {
			return
		}
		nw := max(1, min(3, int(span/(1.3*H))))
		ww := math.Min(0.6*H, 0.7*span/float64(nw))
		for i := range nw {
			mid := lo + span*(float64(i)+0.5)/float64(nw)
			var o, eu r3vec
			switch wall {
			case 0:
				f.n, o, eu = r3vec{1, 0, 0}, r3vec{rm.wx0 + 0.05, yb, mid + ww/2}, r3vec{0, 0, -ww}
			case 1:
				f.n, o, eu = r3vec{-1, 0, 0}, r3vec{rm.wx1 - 0.05, yb, mid - ww/2}, r3vec{0, 0, ww}
			default:
				f.n, o, eu = r3vec{0, 0, -1}, r3vec{mid + ww/2, yb, rm.fz - 0.05}, r3vec{-ww, 0, 0}
			}
			r.quadFace(f, o, eu, r3vec{0, yt - yb, 0})
			sums[wall] = sums[wall].add(o.add(eu.mul(0.5)).add(r3vec{0, (yt - yb) / 2, 0}))
			counts[wall]++
		}
	}
	zlo, zhi := rm.baseDepth()+6, rm.fz-6
	put(0, zlo, zhi)
	put(1, zlo, zhi)
	put(2, rm.wx0+6, rm.wx1-6)

	// The lights: the daylight through each wall's windows, and three lamps.
	w.lamps = w.lamps[:0]
	k := light.lamp
	for wall, n := range [3]r3vec{{1, 0, 0}, {-1, 0, 0}, {0, 0, -1}} {
		if counts[wall] == 0 {
			continue
		}
		at := sums[wall].mul(1 / float64(counts[wall])).sub(n.mul(0.3 * H)) // outside: the light comes in through the glass
		l := w.sky.light
		w.lamps = append(w.lamps, r3xlight{at, [3]float64{l[0] * k, l[1] * k, l[2] * k}, 1 / (1.1 * H * 1.1 * H)})
	}
	x0, x1 := rm.x0, rm.x1
	dep := rm.baseDepth()
	inv := 1 / (0.45 * H * 0.45 * H)
	lampCol := func(h string, s float64) [3]float64 {
		cr, cg, cb := chans(hex32(h))
		return [3]float64{cr / 255 * s * k, cg / 255 * s * k, cb / 255 * s * k}
	}
	// A cool panel on the ceiling.
	pc := r3vec{(x0 + x1) / 2, rm.cy, dep + 0.4*(rm.fz-dep)}
	pw, pd := 0.09*(rm.wx1-rm.wx0), 0.06*(rm.fz)
	r.xbox(r3vec{pc.x - pw, rm.cy - 0.6, pc.z - pd}, r3vec{pc.x + pw, rm.cy + 0.2, pc.z + pd},
		r3face{mat: matLamp, own: -1, col: hex32("#dcebff")})
	w.lamps = append(w.lamps, r3xlight{pc.sub(r3vec{0, 0.06 * H, 0}), lampCol("#bcd4ff", 1.2), inv * 0.4})
	// A warm lamp on a stand, to the right.
	st := r3vec{x0 + 0.85*(x1-x0), rm.fy, dep + 8}
	bulb := st.add(r3vec{0, 0.45 * H, 0})
	pole := math.Max(0.4, 0.004*rm.big)
	stand := mix32(hex32("#3a3b40"), text, 0.1)
	r.xbox(r3vec{st.x - 4*pole, rm.fy, st.z - 4*pole}, r3vec{st.x + 4*pole, rm.fy + 0.8, st.z + 4*pole}, r3face{mat: matSolid, own: -1, col: stand, gloss: 0.4})
	r.xbox(r3vec{st.x - pole, rm.fy, st.z - pole}, r3vec{st.x + pole, bulb.y, st.z + pole}, r3face{mat: matSolid, own: -1, col: stand, gloss: 0.4})
	r.globe(bulb, math.Max(2, 0.045*H), hex32("#ffd29a"))
	w.lamps = append(w.lamps, r3xlight{bulb, lampCol("#ff9e4a", 1.9), inv})
	// A magenta strip low on the left wall.
	sp := r3vec{rm.wx0, rm.fy + 0.1*H, dep + 0.2*H}
	r.xbox(r3vec{sp.x, sp.y - 0.4, sp.z - 0.12*H}, r3vec{sp.x + 0.6, sp.y + 0.4, sp.z + 0.12*H},
		r3face{mat: matLamp, own: -1, col: hex32("#ff7ae6")})
	w.lamps = append(w.lamps, r3xlight{sp.add(r3vec{2, 0, 0}), lampCol("#ff40d0", 1.9), inv})
	if xlightsOff {
		w.lamps = w.lamps[:0]
	}
	if len(w.lamps) > maxXLights {
		w.lamps = w.lamps[:maxXLights]
	}
}

// walkers puts each person where they are secs into the day.
func (w *r3world) walkers(secs float64) {
	H := w.h
	P := 14 * H
	for wall := range 3 {
		for lane := range 2 {
			way := 1.0
			if lane == 1 {
				way = -1
			}
			for i := range walkLane {
				seed := wall*2 + lane
				speed := 0.32 + 0.3*ihash(i, seed, 2)
				x := modF(ihash(i, seed, 1)*P+way*speed*H*secs, P) - P/2
				step := int(ffloor(secs*speed*4+2*ihash(i, seed, 3))) & 1
				fig := int(ihash(i, seed, 4) * float64(len(walkerFigs)/2))
				skin := int(ihash(i, seed, 5) * float64(len(skinCols)))
				coat := hex32(coatCols[int(ihash(i, seed, 6)*float64(len(coatCols)))])
				w.walk[wall*worldWalkers+lane*walkLane+i] = r3walker{x, fig*2 + step | skin<<4, coat}
			}
		}
	}
}

// xbox adds a box from lo to hi, its faces like f.
func (r *room3D) xbox(lo, hi r3vec, f r3face) {
	d := hi.sub(lo)
	X, Y, Z := r3vec{d.x, 0, 0}, r3vec{0, d.y, 0}, r3vec{0, 0, d.z}
	for _, s := range []struct {
		n, o, u, v r3vec
	}{
		{r3vec{-1, 0, 0}, lo, Z, Y}, {r3vec{1, 0, 0}, lo.add(X), Y, Z},
		{r3vec{0, -1, 0}, lo, X, Z}, {r3vec{0, 1, 0}, lo.add(Y), Z, X},
		{r3vec{0, 0, -1}, lo, Y, X}, {r3vec{0, 0, 1}, lo.add(Z), X, Y},
	} {
		f.n = s.n
		r.quadFace(f, s.o, s.u, s.v)
	}
}

// globe adds a glowing bulb (eight faces) at c, rb across.
func (r *room3D) globe(c r3vec, rb float64, col uint32) {
	for _, sx := range []float64{-1, 1} {
		for _, sy := range []float64{-1, 1} {
			for _, sz := range []float64{-1, 1} {
				a, b, d := c.add(r3vec{sx * rb, 0, 0}), c.add(r3vec{0, sy * rb, 0}), c.add(r3vec{0, 0, sz * rb})
				r.addFace(r3face{n: r3vec{sx, sy, sz}.norm(), mat: matLamp, own: -1, col: col}, a, b, d)
			}
		}
	}
}

// ---- the extra lights on a surface --------------------------------------------------------

// xlightAt is the extra lights' colour on a surface at p facing n,
// packed (see packTint).
func (w *r3world) xlightAt(n, p r3vec) uint32 {
	var er, eg, eb float64
	for i := range w.lamps {
		l := &w.lamps[i]
		lx, ly, lz := l.at.x-p.x, l.at.y-p.y, l.at.z-p.z
		d2 := lx*lx + ly*ly + lz*lz
		ndl := (n.x*lx + n.y*ly + n.z*lz) / math.Sqrt(d2)
		if ndl <= 0 {
			continue
		}
		k := ndl / (1 + d2*l.inv)
		er, eg, eb = er+l.col[0]*k, eg+l.col[1]*k, eb+l.col[2]*k
	}
	return packTint(er, eg, eb)
}

// packTint keeps three amounts of light (0 to 4) in one number, ten bits each.
func packTint(r, g, b float64) uint32 {
	q := func(v float64) uint32 { return uint32(math.Min(v*256+0.5, 1023)) }
	return q(r)<<20 | q(g)<<10 | q(b)
}

// unpackTint is packTint undone.
func unpackTint(t uint32) (r, g, b float64) {
	return float64(t>>20&1023) / 256, float64(t>>10&1023) / 256, float64(t&1023) / 256
}

// ---- letters mode ------------------------------------------------------------------------

// townLetters puts the town's letters (the people, the signs, the
// windows, the stars) in the cells where they are big enough to be the
// terminal's own letters, as words are (letters mode: two pixels a cell,
// too few to paint a letter in). Each goes in the cell its middle is in.
func (r *room3D) townLetters(c *r3shade) {
	w, h := r.rm.w, r.rm.h
	v := &r.view
	for row := range h {
		for col := range w {
			at := row*w + col
			if r.chars[at].stamp == r.stamp {
				continue // a word is there
			}
			four := r.cellFour(col, row)
			fi := r.face[four[0]]
			if fi < 0 || r.faces[fi].mat != matWindow || r.face[four[1]] != fi || r.face[four[2]] != fi || r.face[four[3]] != fi {
				continue
			}
			fc, fd := &r.faces[fi], &r.fdraw[fi]
			dir := v.ray(float64(col)+0.5, float64(2*row+1))
			nd := fd.n.dot(dir)
			if nd >= 0 {
				continue
			}
			t := fd.num / nd
			p := v.cam.add(dir.mul(t))
			var lt r3letter
			if _, glass := r.windowAt(c, fc, fd.n, p, dir, t, &lt); !glass || lt.ch <= 0 {
				continue
			}
			// Big enough, and its middle in this cell (a cell is two pixels each way).
			if lt.du > letterBig || lt.dv > letterBig || absF(lt.u-0.5) > lt.du || absF(lt.v-0.5) > lt.dv {
				continue
			}
			var sr, sg, sb float64
			for _, i := range four {
				cr, cg, cb := chans(r.pixel(i))
				sr, sg, sb = sr+cr, sg+cg, sb+cb
			}
			bg := rgb32(sr/4+0.5, sg/4+0.5, sb/4+0.5)
			r.chars[at] = r3char{s: outChars[lt.ch], fg: clearInk(lt.ink, bg), bg: bg, stamp: r.stamp, face: fi}
		}
	}
}

// ---- the people walk on --------------------------------------------------------------------

// room3DOutsideMsg: time to draw the room again, the people outside having walked on.
type room3DOutsideMsg struct{}

// outsideEvery is how often the room is drawn again while a window shows.
const outsideEvery = 33 * time.Millisecond

// windowShown is whether a window was on the screen in the last drawing.
func (r *room3D) windowShown() bool {
	if len(r.fdraw) != len(r.faces) {
		return false
	}
	for i := range r.faces {
		fc := &r.faces[i]
		if fc.mat != matWindow || !fc.seen {
			continue
		}
		fd := &r.fdraw[i]
		if fd.bo < 0 || fd.top > float64(r.ph) {
			continue
		}
		for _, p := range r.polys[fd.p0 : fd.p0+fd.pn] {
			if p[0] > -1 && p[0] < float64(r.pw)+1 {
				return true
			}
		}
		// All corners off the sides: on the screen still if they are either side of it.
		lo, hi := math.Inf(1), math.Inf(-1)
		for _, p := range r.polys[fd.p0 : fd.p0+fd.pn] {
			lo, hi = math.Min(lo, p[0]), math.Max(hi, p[0])
		}
		if lo < 0 && hi > float64(r.pw) {
			return true
		}
	}
	return false
}

// outsideCmd asks for the next drawing while a window shows, so the people
// outside walk on (and the clouds drift). A slow drawing (no graphics
// card) is asked for less often.
func (m *Model) outsideCmd() tea.Cmd {
	d := m.design
	if d == nil || !m.inDesign() || !d.room.on() || d.room.world.tick || d.room.frame == "" || !d.room.windowShown() {
		return nil
	}
	d.room.world.tick = true
	wait := max(outsideEvery, 3*d.room.cost)
	return tea.Tick(wait, func(time.Time) tea.Msg { return room3DOutsideMsg{} })
}
