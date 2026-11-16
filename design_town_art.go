package main

// Windows in a room wall: each window shows a little street scene outside
// (sky, skyline, buildings with lit windows) with small text NPCs that walk,
// wave, sit on ledges and fly by. Pure, deterministic for (seed, w, h, t).

import "math"

// winCell is one terminal cell. Bg "" means "leave the board colour".
type winCell struct {
	R      rune
	Fg, Bg string
}

// ---------------------------------------------------------------- hashing

func winMix(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}

func winH(seed uint64, a, b, c int) uint64 {
	k := uint64(int64(a))*0x9e3779b97f4a7c15 ^ uint64(int64(b))*0xc2b2ae3d27d4eb4f ^ uint64(int64(c))*0x165667b19e3779f9
	return winMix(winMix(seed+0x632be59bd9b4e019) ^ winMix(k))
}

// winF maps a hash to [0,1).
func winF(h uint64) float64 { return float64(h>>11) / (1 << 53) }

// winN maps a hash to [0,n); 0 when n <= 0.
func winN(h uint64, n int) int {
	if n <= 0 {
		return 0
	}
	return int(h % uint64(n))
}

func winFrac(x float64) float64 { return x - math.Floor(x) }

// winCount: how many windows a room has (1..4).
func winCount(roomSeed uint64) int {
	return 1 + int(winMix(roomSeed^0x5bd1e9955bd1e995)%4)
}

// winSeed: window i's own seed inside a room.
func winSeed(roomSeed uint64, i int) uint64 {
	return winMix(roomSeed*0x9e3779b97f4a7c15 + uint64(int64(i)+1)*0xd1b54a32d192ed03)
}

// ---------------------------------------------------------------- colours

func winParse(s string) uint32 {
	var v uint32
	for i := 1; i < len(s) && i < 7; i++ {
		c := s[i]
		var d byte
		switch {
		case c >= '0' && c <= '9':
			d = c - '0'
		case c >= 'a' && c <= 'f':
			d = c - 'a' + 10
		case c >= 'A' && c <= 'F':
			d = c - 'A' + 10
		}
		v = v<<4 | uint32(d)
	}
	return v
}

func winLerp(a, b uint32, f float64) uint32 {
	if f < 0 {
		f = 0
	} else if f > 1 {
		f = 1
	}
	var out uint32
	for sh := 16; sh >= 0; sh -= 8 {
		ca := float64((a >> uint(sh)) & 0xff)
		cb := float64((b >> uint(sh)) & 0xff)
		out |= uint32(ca+(cb-ca)*f+0.5) << uint(sh)
	}
	return out
}

func winHex(c uint32) string {
	const d = "0123456789abcdef"
	var b [7]byte
	b[0] = '#'
	for i := 0; i < 6; i++ {
		b[1+i] = d[(c>>uint(20-4*i))&0xf]
	}
	return string(b[:])
}

func winMixHex(a, b string, f float64) string {
	return winHex(winLerp(winParse(a), winParse(b), f))
}

// ---------------------------------------------------------------- palettes

type winPalette struct {
	skyTop, skyMid, skyBot  string
	far, farWin             string
	walls, roofs, lit, tv   []string
	darkWin, glass, glassHi string
	stars                   []string
	orb                     string
	cloud, cloudShade       string
	pave, road, dash        string
	lamp, lampGlow, post    string
	lampOn                  bool
	silhouette, smoke       string
	shirts, skins, legs     []string
	cat, dog, bird, note    string
	cars                    []string
	awningA, awningB        string
	antenna, blink          string
	door, shop              string
	litProb, occProb        float64
}

var winSkins = []string{"#f1c27d", "#d9a066", "#a8673a", "#8d5524", "#e8b98a"}

var winPalNight = winPalette{
	skyTop: "#07070f", skyMid: "#151027", skyBot: "#33202a",
	far: "#1d1620", farWin: "#a8703c",
	walls:   []string{"#2a1e1c", "#30221e", "#271e27", "#3a2a22", "#2d2420", "#36272a"},
	roofs:   []string{"#4a2a22", "#3e2a2a", "#55332a"},
	lit:     []string{"#ffb347", "#ffcf6b", "#ff9f43", "#ffd89a", "#f0a050"},
	tv:      []string{"#6f8fd0", "#4f6fb0", "#8aa8e0"},
	darkWin: "#140e0e", glass: "#140e0e", glassHi: "#1c1414",
	stars: []string{"#f2e6c8", "#c9b98f", "#8a7f6a"},
	orb:   "#f3e3b5",
	cloud: "#2e2636", cloudShade: "#241d2b",
	pave: "#2b2321", road: "#161214", dash: "#7a6a3a",
	lamp: "#ffd27a", lampGlow: "#4a3426", post: "#5a4a44", lampOn: true,
	silhouette: "#2a1810", smoke: "#5e504c",
	shirts: []string{"#e07b39", "#d9b26a", "#c8553d", "#7fa38a", "#c98bb9", "#6fa0c8", "#e8d8b0"},
	skins:  winSkins,
	legs:   []string{"#6a5a70", "#5a6a80", "#7a5a4a", "#8a7a6a"},
	cat:    "#e08a3a", dog: "#b07a45", bird: "#7a6a7a", note: "#ffe08a",
	cars:    []string{"#c0503c", "#d9b26a", "#6fa0c8", "#9a9aa8"},
	awningA: "#b5473a", awningB: "#e8d8b0",
	antenna: "#5a4840", blink: "#ff4a3a",
	door: "#3a2418", shop: "#ffc46b",
	litProb: 0.45, occProb: 0.18,
}

var winPalDusk = winPalette{
	skyTop: "#24163a", skyMid: "#8a3a4a", skyBot: "#f39a4a",
	far: "#5a2a3a", farWin: "#ffb060",
	walls:   []string{"#3a1e28", "#2e1a24", "#45242a", "#3a2230", "#4a2a2a"},
	roofs:   []string{"#2a141c", "#3a1a20", "#4a2026"},
	lit:     []string{"#ffb347", "#ffcf6b", "#ff9f43", "#ffd89a"},
	tv:      []string{"#6f8fd0", "#4f6fb0"},
	darkWin: "#22141a", glass: "#22141a", glassHi: "#2a1a20",
	stars: []string{"#f2e6c8", "#d9b8a0", "#a08080"},
	orb:   "#ffd27a",
	cloud: "#e0806a", cloudShade: "#b85a5a",
	pave: "#3a2428", road: "#24161a", dash: "#b08a4a",
	lamp: "#ffd27a", lampGlow: "#5a3a2e", post: "#2a1a1e", lampOn: true,
	silhouette: "#2a1218", smoke: "#a07a7a",
	shirts: []string{"#e07b39", "#d9b26a", "#c8553d", "#7fa38a", "#c98bb9", "#6fa0c8", "#e8d8b0"},
	skins:  winSkins,
	legs:   []string{"#6a4a5a", "#5a5a78", "#7a5a4a"},
	cat:    "#1a0e12", dog: "#2a1a14", bird: "#2a1a24", note: "#ffe08a",
	cars:    []string{"#c0503c", "#d9b26a", "#6fa0c8", "#9a7a6a"},
	awningA: "#b5473a", awningB: "#e8c8a0",
	antenna: "#2a1418", blink: "#ff4a3a",
	door: "#24121a", shop: "#ffc46b",
	litProb: 0.3, occProb: 0.18,
}

var winPalDay = winPalette{
	skyTop: "#7fb8e0", skyMid: "#b7d8ec", skyBot: "#f3e6cf",
	far: "#a9bccb", farWin: "",
	walls:   []string{"#c0673f", "#d9a35a", "#e6d2a8", "#b85a46", "#8a9aa6", "#d98a5a", "#cdb79a"},
	roofs:   []string{"#8a3a2a", "#6a4a3a", "#a0482e"},
	lit:     []string{"#ffe0a0"},
	tv:      []string{"#9fc3d8"},
	darkWin: "#6a8aa0", glass: "#9fc3d8", glassHi: "#d4e8f2",
	stars: []string{"#ffffff"},
	orb:   "#fff1b0",
	cloud: "#ffffff", cloudShade: "#dce6ee",
	pave: "#a39686", road: "#6a6660", dash: "#f0e0a0",
	lamp: "#9a9a9a", lampGlow: "#c8bfb2", post: "#5a5a5a", lampOn: false,
	silhouette: "#3a2a22", smoke: "#d8d8d8",
	shirts: []string{"#b5522a", "#2f6f8f", "#6d8b3a", "#8e3b5e", "#d9a03a", "#44506a"},
	skins:  winSkins,
	legs:   []string{"#3a3a4a", "#4a3a2a", "#2a3a5a"},
	cat:    "#3a2a22", dog: "#6b4423", bird: "#3a3a44", note: "#8a4a2a",
	cars:    []string{"#c0392b", "#2f6f8f", "#e0b030", "#f0f0f0"},
	awningA: "#c0503c", awningB: "#f4ead8",
	antenna: "#6a6a70", blink: "",
	door: "#5a3a24", shop: "#bcd6e2",
	litProb: 0, occProb: 0.07,
}

type winWood struct{ light, dark, sill string }

var winWoodsDark = []winWood{
	{"#b07a45", "#5a3520", "#8a5a34"},
	{"#c9925a", "#6b4026", "#9a6a3e"},
	{"#d8c3a0", "#7a6650", "#b8a280"},
}

var winWoodsLight = []winWood{
	{"#a8743f", "#5e3a20", "#7e5530"},
	{"#8a5a3a", "#4a2c1a", "#6e452a"},
	{"#c08a5a", "#7a4a3a", "#9a6040"},
}

type winCurtain struct{ light, dark string }

var winCurtsDark = []winCurtain{
	{"#b5473a", "#6e2620"}, {"#c9973f", "#7a5520"}, {"#8a4a6e", "#4e2440"}, {"#5e7f6a", "#2f4a3a"},
}

var winCurtsLight = []winCurtain{
	{"#d0664a", "#a04530"}, {"#e3b062", "#b5843a"}, {"#9a6a8a", "#6e4a64"}, {"#88a890", "#5e7f6a"},
}

const winTie = "#e0b450"

var winHeads = [4]rune{'o', 'o', 'ô', '☻'}

var winCarShape = [6]rune{'▗', '▟', '█', '█', '▙', '▖'}

// ---------------------------------------------------------------- canvas

// winCanvas is a w×h view into the output rows (offset by the frame).
type winCanvas struct {
	c      []winCell
	stride int
	ox, oy int
	w, h   int
}

func (v *winCanvas) at(x, y int) *winCell {
	if x < 0 || y < 0 || x >= v.w || y >= v.h {
		return nil
	}
	return &v.c[(y+v.oy)*v.stride+x+v.ox]
}

// put sets rune+fg and keeps the background.
func (v *winCanvas) put(x, y int, r rune, fg string) {
	if p := v.at(x, y); p != nil {
		p.R, p.Fg = r, fg
	}
}

func (v *winCanvas) set(x, y int, r rune, fg, bg string) {
	if p := v.at(x, y); p != nil {
		p.R, p.Fg, p.Bg = r, fg, bg
	}
}

func (v *winCanvas) setBg(x, y int, bg string) {
	if p := v.at(x, y); p != nil {
		p.Bg = bg
	}
}

// puts writes s from x; spaces are transparent.
func (v *winCanvas) puts(x, y int, s string, fg string) {
	i := 0
	for _, r := range s {
		if r != ' ' {
			v.put(x+i, y, r, fg)
		}
		i++
	}
}

// ---------------------------------------------------------------- scene

type winPt struct{ x, y, w int }

type winScene struct {
	cv           winCanvas
	seed         uint64
	t            float64
	p            *winPalette
	tod          int // 0 day, 1 dusk, 2 night
	iw, ih       int
	ph, rr       int // person height (0 = none), road rows
	feet, bottom int // pavement row (-1 none), lowest building row
	skyLimit     int
	occ          [32]winPt
	nocc         int
	chim         [12]winPt
	nchim        int
	roofs        [16]winPt
	nroof        int
}

// winArt renders one window (frame included), h rows of w cells.
func winArt(seed uint64, w, h int, t float64, light bool) [][]winCell {
	if h <= 0 {
		return [][]winCell{}
	}
	if w < 0 {
		w = 0
	}
	flat := make([]winCell, w*h)
	rows := make([][]winCell, h)
	for y := range rows {
		rows[y] = flat[y*w : (y+1)*w : (y+1)*w]
	}
	if w == 0 {
		return rows
	}
	if math.IsNaN(t) || math.IsInf(t, 0) {
		t = 0
	}
	framed := w >= 4 && h >= 3
	s := winScene{seed: seed, t: t}
	if framed {
		s.cv = winCanvas{c: flat, stride: w, ox: 1, oy: 1, w: w - 2, h: h - 2}
	} else {
		s.cv = winCanvas{c: flat, stride: w, w: w, h: h}
	}
	s.iw, s.ih = s.cv.w, s.cv.h
	s.setup(light)
	s.render()
	s.decorate(rows, framed, light)
	return rows
}

func (s *winScene) setup(light bool) {
	f := winF(winH(s.seed, 1, 0, 0))
	switch {
	case light && f < 0.7:
		s.tod, s.p = 0, &winPalDay
	case !light && f < 0.6:
		s.tod, s.p = 2, &winPalNight
	default:
		s.tod, s.p = 1, &winPalDusk
	}
	switch {
	case s.ih >= 16:
		s.rr, s.ph = 2, 3
	case s.ih >= 8:
		s.rr, s.ph = 1, 2
	case s.ih >= 4:
		s.rr, s.ph = 0, 1
	}
	if s.ph > 0 {
		s.feet = s.ih - 1 - s.rr
		s.bottom = s.feet - 1
	} else {
		s.feet = -1
		s.bottom = s.ih - 1
	}
	s.skyLimit = max(1, (s.bottom+1)*2/5)
}

func (s *winScene) render() {
	p := s.p
	top, mid, bot := winParse(p.skyTop), winParse(p.skyMid), winParse(p.skyBot)
	// small per-window variation of the sky tint
	top = winLerp(top, mid, winF(winH(s.seed, 1, 1, 0))*0.25)
	den := float64(max(1, s.bottom))
	for y := 0; y < s.ih; y++ {
		f := math.Min(1, float64(y)/den)
		var c uint32
		if f < 0.55 {
			c = winLerp(top, mid, f/0.55)
		} else {
			c = winLerp(mid, bot, (f-0.55)/0.45)
		}
		bg := winHex(c)
		for x := 0; x < s.iw; x++ {
			s.cv.set(x, y, ' ', "", bg)
		}
	}
	s.drawStars()
	s.drawOrb()
	s.drawClouds()
	s.drawFar()
	s.drawBirds()
	s.drawBuildings()
	s.drawSmoke()
	s.drawCat()
	s.drawGround()
	s.drawLamps()
	s.drawCar()
	s.drawPeople()
	s.drawNotes()
}

func (s *winScene) drawStars() {
	if s.tod == 0 {
		return
	}
	dens, rows := 0.035, s.skyLimit+s.skyLimit/2
	if s.tod == 1 {
		dens, rows = 0.012, max(1, s.skyLimit/2)
	}
	rows = min(rows, s.ih)
	for y := 0; y < rows; y++ {
		for x := 0; x < s.iw; x++ {
			hs := winH(s.seed, 600+y, x, 0)
			if winF(hs) >= dens {
				continue
			}
			ep := int(math.Floor(s.t/1.7 + winF(hs<<9)*7))
			r := '·'
			switch winH(hs, ep, 0, 0) % 12 {
			case 0:
				r = '✦'
			case 1, 2:
				r = '∙'
			case 3:
				continue // twinkled off
			}
			s.cv.put(x, y, r, s.p.stars[winN(hs>>40, len(s.p.stars))])
		}
	}
}

func (s *winScene) drawOrb() {
	ho := winH(s.seed, 700, 0, 0)
	if s.tod == 0 && ho%4 == 0 {
		return // hazy day, no sun
	}
	avail := s.bottom + 1
	cx := s.iw/5 + winN(ho>>8, max(1, s.iw*3/5))
	var cy int
	switch s.tod {
	case 2:
		cy = winN(ho>>20, max(1, avail/4))
		if s.ih > 6 {
			cy++
		}
	case 1:
		cy = s.bottom - avail*2/5 + winN(ho>>20, max(1, avail/6))
	default:
		cy = winN(ho>>20, max(1, avail/5))
	}
	fg := s.p.orb
	switch {
	case s.iw >= 30 && s.ih >= 12:
		s.cv.puts(cx-2, cy, "▗██▖", fg)
		s.cv.puts(cx-2, cy+1, "▝██▘", fg)
	case s.iw >= 14 && s.ih >= 6:
		s.cv.puts(cx-1, cy, "▐█▌", fg)
	default:
		s.cv.put(cx, cy, '●', fg)
	}
}

func (s *winScene) drawClouds() {
	var n int
	switch s.tod {
	case 0:
		n = 1 + s.iw/22
	case 1:
		n = s.iw / 30
	case 2:
		if winH(s.seed, 500, 99, 0)%3 == 0 {
			n = 1 + s.iw/50
		}
	}
	if s.ih < 3 {
		n = min(n, 1)
	}
	wind := 1
	if winH(s.seed, 500, 98, 0)&1 == 1 {
		wind = -1
	}
	for k := 0; k < n; k++ {
		hc := winH(s.seed, 500, k, 0)
		cw := min(s.iw, 3+winN(hc, 3+s.iw/10))
		if cw < 1 {
			continue
		}
		two := s.ih >= 12 && cw >= 6
		speed := 0.1 + winF(hc<<8)*0.25
		y := winN(hc>>24, s.skyLimit)
		span := s.iw + cw
		base := winN(hc>>32, span)
		off := int(math.Floor(s.t * speed))
		x := ((base+wind*off)%span+span)%span - cw
		if !two {
			for i := 0; i < cw; i++ {
				r := '█'
				if i == 0 || i == cw-1 {
					r = '▄'
				}
				s.cv.put(x+i, y, r, s.p.cloud)
			}
			continue
		}
		for i := 1; i < cw-1; i++ {
			r := '▄'
			if i > 1 && i <= cw/2+1 && i != cw-2 {
				r = '█'
			}
			s.cv.put(x+i, y, r, s.p.cloud)
		}
		for i := 0; i < cw; i++ {
			r := '█'
			if i == 0 || i == cw-1 {
				r = '▀'
			}
			s.cv.put(x+i, y+1, r, s.p.cloudShade)
		}
	}
}

func (s *winScene) drawFar() {
	p := s.p
	avail := s.bottom + 1
	lo := max(1, avail*2*45/100)
	hi := max(lo+1, avail*2*90/100)
	for x, i := 0, 0; x < s.iw; i++ {
		hb := winH(s.seed, 800, i, 0)
		bw := 2 + winN(hb, 2+s.iw/20)
		hh := lo + winN(hb>>16, hi-lo)
		top := s.bottom + 1 - hh/2
		for xx := x; xx < x+bw && xx < s.iw; xx++ {
			for y := max(0, top); y <= s.bottom; y++ {
				if p.farWin != "" && winF(winH(s.seed, 801, xx, y)) < 0.06 {
					s.cv.set(xx, y, '·', p.farWin, p.far)
				} else {
					s.cv.set(xx, y, ' ', "", p.far)
				}
			}
			if hh%2 == 1 {
				s.cv.put(xx, top-1, '▄', p.far)
			}
		}
		x += bw
	}
}

func (s *winScene) drawBirds() {
	if s.bottom < 2 {
		return
	}
	period := 18 + float64(winN(s.seed>>5, 17))
	ep := math.Floor(s.t / period)
	hb := winH(s.seed, 900, int(ep), 0)
	chance := 0.6
	if s.tod == 2 {
		chance = 0.3
	}
	if winF(hb) >= chance {
		return
	}
	local := s.t - ep*period
	speed := 2.5 + winF(hb<<10)*2.5
	step := int(math.Floor(local * speed))
	dir := 1
	if (hb>>9)&1 == 1 {
		dir = -1
	}
	x := step - 3
	if dir < 0 {
		x = s.iw + 2 - step
	}
	y := winN(hb>>20, s.skyLimit)
	if s.ih >= 10 && (step/3)%2 == 1 {
		y++
	}
	count := 1
	if s.iw >= 30 {
		count = 1 + winN(hb>>30, 3)
	}
	for b := 0; b < count; b++ {
		bx := x - dir*3*b
		by := y + b%2
		flap := (int(math.Floor(local*5)) + b) & 1
		if s.ph == 3 {
			if flap == 0 {
				s.cv.puts(bx, by, "\\/", s.p.bird)
			} else {
				s.cv.puts(bx, by, "/\\", s.p.bird)
			}
		} else {
			r := 'v'
			if flap == 1 {
				r = '^'
			}
			s.cv.put(bx, by, r, s.p.bird)
		}
	}
}

func (s *winScene) drawBuildings() {
	p := s.p
	avail := s.bottom + 1
	hmin := max(1, avail*35/100)
	hmax := max(hmin, avail*80/100)
	minW := max(3, s.iw/12)
	maxW := max(minW+1, s.iw/5)
	ground := 0
	if s.ph >= 2 {
		ground = 1
	}
	shadeF := 0.25
	if s.tod == 0 {
		shadeF = 0.18
	}
	x := -winN(winH(s.seed, 9, 0, 0), 3)
	for i := 0; x < s.iw && i < 64; i++ {
		hb := winH(s.seed, 10, i, 0)
		bw := minW + winN(hb, maxW-minW+1)
		kind := 0
		switch k := winN(hb>>16, 10); {
		case k >= 4 && k <= 6:
			kind = 1
		case k >= 7 && k <= 8 && s.ph >= 2 && bw >= 5:
			kind = 2
		case k == 9:
			kind = 3
		}
		bh := hmin + winN(hb>>24, hmax-hmin+1)
		if kind == 3 {
			bh = min(avail, bh+avail/6)
		}
		if kind == 1 {
			bh = max(1, bh*3/4)
		}
		wall := p.walls[winN(hb>>32, len(p.walls))]
		shade := winMixHex(wall, "#000000", shadeF)
		s.building(i, x, s.bottom-bh+1, bw, kind, wall, shade, ground, hb)
		gap := 0
		if winN(hb>>40, 4) == 0 {
			gap = 1 + winN(hb>>44, 1+s.iw/40)
		}
		x += bw + gap
	}
}

func (s *winScene) building(i, bx, top, bw, kind int, wall, shade string, ground int, hb uint64) {
	p := s.p
	for y := max(0, top); y <= s.bottom; y++ {
		for x := bx; x < bx+bw; x++ {
			bg := wall
			if x == bx+bw-1 && bw > 3 {
				bg = shade
			}
			s.cv.set(x, y, ' ', "", bg)
		}
	}
	// roof
	if kind == 1 {
		roof := p.roofs[winN(hb>>48, len(p.roofs))]
		rh := max(1, min((bw-1)/2, max(1, s.ph)))
		for r := 0; r < rh; r++ {
			y, x0, x1 := top-1-r, bx+r, bx+bw-1-r
			if x1-x0 < 1 {
				break
			}
			s.cv.put(x0, y, '▟', roof)
			s.cv.put(x1, y, '▙', roof)
			for x := x0 + 1; x < x1; x++ {
				s.cv.put(x, y, '█', roof)
			}
		}
		if bw >= 6 && s.ph >= 1 {
			cx := bx + bw - 3
			cy := top - 1 - min(2, rh-1) - 1
			s.cv.put(cx, cy, '█', shade)
			if s.nchim < len(s.chim) && cx >= 0 && cx < s.iw {
				s.chim[s.nchim] = winPt{cx, cy - 1, 1}
				s.nchim++
			}
		}
	}
	if kind == 3 || (kind == 0 && (hb>>52)%4 == 0) {
		ax := bx + 1 + winN(hb>>56, max(1, bw-2))
		ah := 1
		if s.ph >= 2 {
			ah = 2
		}
		if kind == 3 && s.ph >= 3 {
			ah = 3
		}
		for r := 1; r <= ah; r++ {
			ch := '│'
			if r == ah {
				ch = '╻'
			}
			s.cv.put(ax, top-r, ch, p.antenna)
		}
		if p.blink != "" && (int(math.Floor(s.t/0.8))+i)&1 == 0 {
			s.cv.put(ax, top-ah-1, '·', p.blink)
		}
	}
	if (kind == 0 || kind == 2) && bw >= 5 && bx >= 0 && bx+bw <= s.iw && top >= 1 && s.nroof < len(s.roofs) {
		s.roofs[s.nroof] = winPt{bx, top, bw}
		s.nroof++
	}
	// windows
	ww := 1
	if s.ph == 3 && bw >= 7 {
		ww = 2
	}
	start := s.bottom - ground
	if kind == 2 {
		start = s.bottom - 3
	}
	minY := top + 1
	if s.ph == 0 {
		minY = top
	}
	for y := start; y >= minY && y >= 0; y -= 2 {
		for wx := bx + 1; wx+ww-1 <= bx+bw-2; wx += ww + 1 {
			if wx+ww-1 < 0 || wx >= s.iw {
				continue
			}
			s.window(i, wx, y, ww)
		}
	}
	if ground == 0 {
		return
	}
	if kind == 2 { // shop: big window, door, striped awning
		for x := bx + 1; x <= bx+bw-2; x++ {
			s.cv.set(x, s.bottom, ' ', "", p.shop)
		}
		s.cv.set(bx+bw-2, s.bottom, ' ', "", p.door)
		for x := bx; x < bx+bw; x++ {
			fg := p.awningA
			if (x-bx)%2 == 1 {
				fg = p.awningB
			}
			s.cv.put(x, s.bottom-1, '▀', fg)
		}
		return
	}
	s.cv.set(bx+bw/2, s.bottom, ' ', "", p.door)
}

func (s *winScene) window(i, wx, y, ww int) {
	p := s.p
	hw := winH(s.seed, 1000+i, wx, y)
	period := 7 + float64(hw%17)
	ep := int(math.Floor((s.t + winF(hw<<5)*period) / period))
	he := winH(hw, ep, 1, 0)
	occ := false
	var bg string
	switch {
	case s.tod == 0:
		bg = p.glass
		if (hw>>40)%3 == 0 {
			bg = p.glassHi
		}
		occ = winF(he<<3) < p.occProb
	case winF(he) < p.litProb:
		bg = p.lit[winN(hw>>20, len(p.lit))]
		if s.tod == 2 && (hw>>50)%18 == 0 { // a TV flickering
			k := int(math.Floor(s.t*2.5)) + int(hw%7)
			bg = p.tv[((k%len(p.tv))+len(p.tv))%len(p.tv)]
		}
		occ = winF(he<<3) < p.occProb
	default:
		bg = p.darkWin
	}
	for k := 0; k < ww; k++ {
		s.cv.set(wx+k, y, ' ', "", bg)
	}
	if !occ {
		return
	}
	head := 'o'
	if ww == 1 && (he>>33)%4 == 0 {
		head = '☻'
	}
	s.cv.put(wx, y, head, p.silhouette)
	if ww == 2 && (he>>36)%3 == 0 { // waving out of the window
		arm := '/'
		if int(math.Floor(s.t/0.4))&1 == 1 {
			arm = '\\'
		}
		s.cv.put(wx+1, y, arm, p.silhouette)
	}
	if s.nocc < len(s.occ) && wx >= 0 {
		s.occ[s.nocc] = winPt{wx, y, ww}
		s.nocc++
	}
}

func (s *winScene) drawSmoke() {
	for k := 0; k < s.nchim; k++ {
		c := s.chim[k]
		for j := 0; j < 3; j++ {
			age := winFrac(s.t*0.3 + float64(j)/3 + float64(k)*0.37)
			y := c.y - int(age*4)
			x := c.x + int(math.Floor(age*2.5+math.Sin(age*5+float64(k))*0.6+0.5))
			r := '°'
			if age > 0.66 {
				r = '·'
			} else if age > 0.33 {
				r = '∙'
			}
			s.cv.put(x, y, r, s.p.smoke)
		}
	}
}

func (s *winScene) drawCat() {
	if s.ph < 2 || s.nroof == 0 {
		return
	}
	hc := winH(s.seed, 1100, 0, 0)
	if hc%5 == 0 {
		return // no cat in this view
	}
	rf := s.roofs[winN(hc>>8, s.nroof)]
	span := rf.w - 3
	if span < 0 {
		return
	}
	a := rf.x + winN(hc>>16, span+1)
	b := rf.x + winN(hc>>24, span+1)
	const cycle = 26.0
	m := winFrac(s.t/cycle+winF(hc<<7)) * cycle
	pos, right, walking, steps := a, b >= a, false, 0
	walk := func(from, to int, since float64) {
		steps = int(since / 0.5)
		d := to - from
		st := min(steps, max(d, -d))
		if d < 0 {
			pos, right = from-st, false
		} else {
			pos, right = from+st, true
		}
		walking = st < max(d, -d)
	}
	switch {
	case m < 9:
	case m < 13:
		walk(a, b, m-9)
	case m < 22:
		pos = b
	default:
		walk(b, a, m-22)
	}
	y := rf.y - 1
	tail := '~'
	if walking {
		if steps&1 == 1 {
			tail = '-'
		}
	} else if int(math.Floor(s.t/0.9))&1 == 1 {
		tail = '-'
	}
	body := '▄'
	if walking && steps&1 == 1 {
		body = '▃'
	}
	if right {
		s.cv.put(pos, y, tail, s.p.cat)
		s.cv.put(pos+1, y, body, s.p.cat)
		s.cv.put(pos+2, y, '▟', s.p.cat)
	} else {
		s.cv.put(pos, y, '▙', s.p.cat)
		s.cv.put(pos+1, y, body, s.p.cat)
		s.cv.put(pos+2, y, tail, s.p.cat)
	}
}

func (s *winScene) drawGround() {
	if s.feet < 0 {
		return
	}
	p := s.p
	for x := 0; x < s.iw; x++ {
		s.cv.set(x, s.feet, ' ', "", p.pave)
	}
	for y := s.feet + 1; y < s.ih; y++ {
		for x := 0; x < s.iw; x++ {
			s.cv.set(x, y, ' ', "", p.road)
		}
	}
	if s.rr >= 1 {
		for x := 0; x < s.iw; x++ {
			if (x/3)%2 == 0 {
				s.cv.put(x, s.ih-1, '─', p.dash)
			}
		}
	}
}

func (s *winScene) drawLamps() {
	if s.ph < 2 {
		return
	}
	p := s.p
	n := s.iw / 22
	if n == 0 && s.iw >= 14 {
		n = 1
	}
	top := s.feet - s.ph - 1
	for k := 0; k < n; k++ {
		lx := (2*k+1)*s.iw/(2*n) + winN(winH(s.seed, 1200, k, 0), 5) - 2
		if p.lampOn {
			for dx := -2; dx <= 2; dx++ {
				s.cv.setBg(lx+dx, s.feet, p.lampGlow)
			}
		}
		for y := top + 1; y < s.feet; y++ {
			s.cv.put(lx, y, '│', p.post)
		}
		s.cv.put(lx, top, '●', p.lamp)
	}
}

func (s *winScene) drawCar() {
	if s.rr < 1 || s.iw < 20 {
		return
	}
	p := s.p
	period := 11 + float64(winN(s.seed>>9, 13))
	ep := math.Floor(s.t / period)
	hc := winH(s.seed, 1300, int(ep), 0)
	if winF(hc) > 0.7 {
		return
	}
	step := int((s.t - ep*period) * (5 + winF(hc<<8)*5))
	col := p.cars[winN(hc>>24, len(p.cars))]
	y := s.feet + 1
	x, front := step-6, 0
	if (hc>>20)&1 == 1 {
		x = s.iw - step
		front = -1
	} else {
		front = x + 6
	}
	for i, r := range winCarShape {
		s.cv.put(x+i, y, r, col)
	}
	if p.lampOn {
		if front < 0 {
			s.cv.setBg(x-1, y, p.lampGlow)
			s.cv.setBg(x-2, y, p.lampGlow)
		} else {
			s.cv.setBg(front, y, p.lampGlow)
			s.cv.setBg(front+1, y, p.lampGlow)
		}
	}
}

func (s *winScene) drawPeople() {
	if s.ph == 0 || s.feet < 0 {
		return
	}
	p := s.p
	n := min(7, max(1, (s.iw+6)/12))
	margin := 2
	if s.ph == 3 {
		margin = 4
	}
	L := s.iw + 2*margin
	wave := int(math.Floor(s.t/0.35)) & 1
	for j := 0; j < n; j++ {
		hp := winH(s.seed, 1400, j, 0)
		period := 0.3 + winF(hp)*0.3
		dir := 1
		if (hp>>8)&1 == 1 {
			dir = -1
		}
		kind := winN(hp>>12, 6) // 0 waver, 1 dog walker, 2 child, 3 idler, else walker
		if j == 0 && (kind == 3 || (kind == 0 && s.ph < 2)) {
			kind = 4 // the first walker never stands still, so the street always moves
		}
		C := 16 + float64(winN(hp>>20, 12))
		D := 0.0
		switch kind {
		case 0:
			D = 5
		case 3:
			D = 2.5
		}
		tt := s.t + winF(hp<<28)*C
		cyc := math.Floor(tt / C)
		m := tt - cyc*C
		wt := cyc*(C-D) + math.Max(0, m-D)
		paused := m < D
		steps := int(math.Floor(wt / period))
		start := winN(hp>>36, L)
		pos := ((start+dir*steps)%L+L)%L - margin
		frame := steps & 1
		shirt := p.shirts[winN(hp>>44, len(p.shirts))]
		skin := p.skins[winN(hp>>50, len(p.skins))]
		legs := p.legs[winN(hp>>56, len(p.legs))]
		f := s.feet
		switch s.ph {
		case 3:
			if kind == 2 { // child: head + legs
				s.cv.put(pos, f-1, 'o', skin)
				leg := 'Λ'
				if frame == 1 && !paused {
					leg = '|'
				}
				s.cv.put(pos, f, leg, shirt)
				continue
			}
			s.cv.put(pos, f-2, winHeads[winN(hp>>40, 4)], skin)
			body, leg := "/|\\", "/ \\"
			switch {
			case paused:
				leg = "| |"
				if kind == 0 {
					body = "/|/"
					if wave == 1 {
						body = "/|-"
					}
				}
			case frame == 1:
				leg = " | "
				if dir > 0 {
					body = " |\\"
				} else {
					body = "/| "
				}
			}
			s.cv.puts(pos-1, f-1, body, shirt)
			s.cv.puts(pos-1, f, leg, legs)
			if kind == 1 {
				dx := pos + dir*3
				bd := '▄'
				if frame == 1 && !paused {
					bd = '▃'
				}
				s.cv.put(dx, f, bd, p.dog)
				s.cv.put(dx+dir, f, '▀', p.dog)
			}
		case 2:
			head := 'o'
			if (hp>>40)&3 == 3 {
				head = '☻'
			}
			s.cv.put(pos, f-1, head, skin)
			leg := 'Λ'
			if frame == 1 && !paused {
				leg = '|'
			}
			s.cv.put(pos, f, leg, shirt)
			if paused && kind == 0 {
				arm := '/'
				if wave == 1 {
					arm = '-'
				}
				s.cv.put(pos+1, f-1, arm, shirt)
			}
			if kind == 1 {
				s.cv.put(pos+dir*2, f, '▄', p.dog)
				s.cv.put(pos+dir*3, f, '▀', p.dog)
			}
		default:
			g := '☻'
			if (hp>>40)&1 == 1 {
				g = '♙'
			}
			s.cv.put(pos, f, g, shirt)
		}
	}
}

func (s *winScene) drawNotes() {
	if s.nocc == 0 {
		return
	}
	const period = 6.0
	ep := math.Floor(s.t / period)
	hn := winH(s.seed, 1500, int(ep), 0)
	if winF(hn) > 0.5 {
		return
	}
	age := (s.t - ep*period) / period
	if age > 0.8 {
		return
	}
	o := s.occ[winN(hn>>8, s.nocc)]
	s.cv.put(o.x+o.w-1+int(age*2), o.y-1-int(age*4), '♪', s.p.note)
}

// decorate draws the frame, sill, glazing bars and curtains.
func (s *winScene) decorate(rows [][]winCell, framed, light bool) {
	if !framed {
		return
	}
	hd := winH(s.seed, 2, 0, 0)
	woods := winWoodsDark
	if light {
		woods = winWoodsLight
	}
	wd := woods[winN(hd, len(woods))]
	iw, ih := s.iw, s.ih
	mx, hy := -1, -1
	if iw >= 12 && ih >= 4 {
		switch winN(hd>>8, 3) {
		case 1:
			mx = iw / 2
		case 2:
			mx = iw / 2
			if ih >= 6 {
				hy = ih * 2 / 5
			}
		}
	}
	if mx >= 0 {
		for y := 0; y < ih; y++ {
			s.cv.put(mx, y, '┃', wd.light)
		}
	}
	if hy >= 0 {
		for x := 0; x < iw; x++ {
			r := '━'
			if x == mx {
				r = '╋'
			}
			s.cv.put(x, hy, r, wd.light)
		}
	}
	hc := winH(s.seed, 3, 0, 0)
	if iw >= 10 && ih >= 5 && hc%4 != 0 {
		cs := winCurtsDark
		if light {
			cs = winCurtsLight
		}
		c := cs[winN(hc>>8, len(cs))]
		cw := min(3, 1+iw/26)
		ty := ih * 3 / 5
		for y := 0; y < ih; y++ {
			for k := 0; k < cw; k++ {
				for side := 0; side < 2; side++ {
					x, edge := k, '▌'
					if side == 1 {
						x, edge = iw-1-k, '▐'
					}
					switch {
					case y == ty:
						s.cv.set(x, y, '━', winTie, c.dark)
					case k == cw-1:
						s.cv.put(x, y, edge, c.light)
					default:
						s.cv.set(x, y, '▓', c.light, c.dark)
					}
				}
			}
		}
	}
	w, h := len(rows[0]), len(rows)
	for x := 0; x < w; x++ {
		r := '━'
		switch {
		case x == 0:
			r = '┏'
		case x == w-1:
			r = '┓'
		case x-1 == mx:
			r = '┳'
		}
		rows[0][x] = winCell{r, wd.light, wd.dark}
	}
	for y := 1; y < h-1; y++ {
		l, r := '┃', '┃'
		if y-1 == hy {
			l, r = '┣', '┫'
		}
		rows[y][0] = winCell{l, wd.light, wd.dark}
		rows[y][w-1] = winCell{r, wd.light, wd.dark}
	}
	for x := 0; x < w; x++ {
		rows[h-1][x] = winCell{'▀', wd.dark, wd.sill}
	}
}
