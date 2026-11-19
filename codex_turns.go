package main

// ---- the 16 page turns ----------------------------------------------------------------------
//
// The user asked (19 Nov 2026) for 16 ways a page can come in, the cover's
// opening among them. Each page has its own (V while editing a page; the
// cover's with V on the cover). A turn between two pages uses the page that
// comes in; one to or from the cover uses the cover's. Going back plays it
// the other way round.
//
// A turn is drawn by mixing two pictures of the whole view, cell by cell,
// inside the page's rectangle: the view before (from) and the view now.

import (
	"math"
	"time"

	uv "github.com/charmbracelet/ultraviolet"
)

var codexTurns16 = [16]string{
	"slide", "page turn", "push", "fade",
	"dissolve", "wipe down", "wipe up", "doors",
	"iris", "blinds", "tall blinds", "checkers",
	"diagonal", "zoom", "ember burn", "rain",
}

const (
	turnSlide = iota
	turnPage
	turnPush
	turnFade
	turnDissolve
	turnWipeDown
	turnWipeUp
	turnDoors
	turnIris
	turnBlinds
	turnTallBlinds
	turnCheckers
	turnDiagonal
	turnZoom
	turnBurn
	turnRain
)

const codexTurnFrame = 40 * time.Millisecond // frames while a page turns

// codexTurnDur is how long turn anim takes.
func codexTurnDur(anim int) time.Duration {
	switch anim {
	case turnSlide:
		return codexTurnFor
	case turnPage, turnBurn, turnRain, turnZoom:
		return 650 * time.Millisecond
	}
	return 500 * time.Millisecond
}

// coverTurn is how the cover opens and closes (the page turn unless chosen).
func (cf *codexFile) coverTurn() int {
	if cf.CoverTurn <= 0 {
		return turnPage
	}
	return (cf.CoverTurn - 1) % 16
}

// turnBetween is the turn used going from page a to page b (-1 the cover).
func (cf *codexFile) turnBetween(a, b int) int {
	if a < 0 || b < 0 {
		return cf.coverTurn()
	}
	if b >= 0 && b < len(cf.Pages) {
		return (cf.Pages[b].Turn%16 + 16) % 16
	}
	return turnSlide
}

// codexTurnShow plays turn anim onto the page now showing, from the page
// before it (the cover before the first page): V shows what it looks like.
func (m *Model) codexTurnShow(anim int) {
	c := m.cx()
	if c.cf == nil || !codexTurns {
		return
	}
	here := c.page
	was := c.page - 1
	c.page = was
	from := m.codexCanvas()
	c.page = here
	m.codexCanvas() // where things are again, for the keys
	c.turn = &codexTurn{from: from, at: time.Now(), ahead: true, anim: anim, rect: c.pageRect}
}

// codexCoverShow plays the cover's turn: the first page closing onto the cover.
func (m *Model) codexCoverShow(anim int) {
	c := m.cx()
	if c.cf == nil || !codexTurns || len(c.cf.Pages) == 0 {
		return
	}
	here := c.page
	c.page = 0
	from := m.codexCanvas()
	c.page = here
	m.codexCanvas()
	c.turn = &codexTurn{from: from, at: time.Now(), ahead: false, anim: anim, rect: c.pageRect}
}

// smooth starts and ends gently.
func smooth(p float64) float64 {
	p = fxClamp01(p)
	return p * p * (3 - 2*p)
}

// codexTurnApply mixes the view before (from) into cv, the view now, p of the
// way (0 … 1) through turn anim, inside rectangle r. ahead: to a later page.
func codexTurnApply(cv, from *boardCanvas, anim int, p float64, ahead bool, r [4]int) {
	rx, ry, rw, rh := r[0], r[1], r[2], r[3]
	if rw < 2 || rh < 2 {
		return
	}
	e := smooth(p)
	now := snapshot(cv, r) // the new view's cells, kept: some turns move them
	cell := func(src [][]uv.Cell, x, y int) uv.Cell {
		x, y = min(max(x, 0), rw-1), min(max(y, 0), rh-1)
		return src[y][x]
	}
	old := snapshot(from, r)
	set := func(x, y int, c uv.Cell) {
		if xx, yy := rx+x, ry+y; xx >= 0 && yy >= 0 && xx < cv.Width() && yy < cv.Height() {
			cv.Lines[yy][xx] = c
		}
	}
	shade := func(c uv.Cell, to string, amt float64) uv.Cell {
		c.Style.Bg = rgbaOf(mixHex(colorHex(c.Style.Bg, cBG), to, amt))
		if c.Style.Fg != nil {
			c.Style.Fg = rgbaOf(mixHex(colorHex(c.Style.Fg, cText), to, amt))
		}
		return c
	}
	switch anim {
	case turnPage: // the page swings round its binding (the left edge)
		for y := 0; y < rh; y++ {
			if ahead { // the old page folds to the left, the new is under it
				ow := int(math.Round(float64(rw) * math.Cos(e*math.Pi/2)))
				for x := 0; x < rw; x++ {
					if x < ow {
						set(x, y, shade(cell(old, x*rw/max(ow, 1), y), "#000000", 0.45*e))
					} else {
						c := cell(now, x, y)
						if d := x - ow; d < 4 {
							c = shade(c, "#000000", 0.4*(1-e)*(1-float64(d)/4))
						}
						set(x, y, c)
					}
				}
			} else { // the page before unfolds from the binding over this one
				nw := int(math.Round(float64(rw) * math.Sin(e*math.Pi/2)))
				for x := 0; x < rw; x++ {
					if x < nw {
						set(x, y, shade(cell(now, x*rw/max(nw, 1), y), "#000000", 0.45*(1-e)))
					} else {
						c := cell(old, x, y)
						if d := x - nw; d < 4 {
							c = shade(c, "#000000", 0.4*e*(1-float64(d)/4))
						}
						set(x, y, c)
					}
				}
			}
		}
		return
	case turnPush: // the new page pushes the old one out
		off := int(math.Round(e * float64(rw)))
		for y := 0; y < rh; y++ {
			for x := 0; x < rw; x++ {
				if ahead {
					if x+off < rw {
						set(x, y, cell(old, x+off, y))
					} else {
						set(x, y, cell(now, x+off-rw, y))
					}
				} else {
					if x-off >= 0 {
						set(x, y, cell(old, x-off, y))
					} else {
						set(x, y, cell(now, x-off+rw, y))
					}
				}
			}
		}
		return
	case turnDoors: // the old page opens like two doors from the middle
		half := rw / 2
		off := int(math.Round(e * float64(half+1)))
		for y := 0; y < rh; y++ {
			for x := 0; x < rw; x++ {
				var c uv.Cell
				if x < half && x+off < half {
					c = shade(cell(old, x+off, y), "#000000", 0.3*e)
				} else if x >= half && x-off >= half {
					c = shade(cell(old, x-off, y), "#000000", 0.3*e)
				} else {
					c = cell(now, x, y)
					if d := min(abs(x-(half-off)), abs(x-(half+off))); d < 2 && e < 0.98 {
						c = shade(c, "#000000", 0.25)
					}
				}
				set(x, y, c)
			}
		}
		return
	case turnZoom: // the new page grows out of the middle (back: the old shrinks)
		for y := 0; y < rh; y++ {
			for x := 0; x < rw; x++ {
				big, small := now, old
				k := e
				if !ahead {
					big, small, k = old, now, 1-e
				}
				zw, zh := max(int(float64(rw)*k), 1), max(int(float64(rh)*k), 1)
				zx, zy := (rw-zw)/2, (rh-zh)/2
				if k > 0.01 && x >= zx && x < zx+zw && y >= zy && y < zy+zh {
					set(x, y, cell(big, (x-zx)*rw/zw, (y-zy)*rh/zh))
				} else {
					set(x, y, shade(cell(small, x, y), "#000000", 0.4*k))
				}
			}
		}
		return
	case turnFade: // the colours blend; the letters change half way
		for y := 0; y < rh; y++ {
			for x := 0; x < rw; x++ {
				a, b := cell(old, x, y), cell(now, x, y)
				c := b
				if e < 0.5 {
					c = a
				}
				ab, bb := colorHex(a.Style.Bg, cBG), colorHex(b.Style.Bg, cBG)
				c.Style.Bg = rgbaOf(mixHex(ab, bb, e))
				if c.Style.Fg != nil {
					fade := math.Abs(e-0.5) * 2 // letters dim at the swap
					c.Style.Fg = rgbaOf(mixHex(colorHex(c.Style.Bg, cBG), colorHex(c.Style.Fg, cText), fade))
				}
				set(x, y, c)
			}
		}
		return
	}
	// The rest are masks: in(x, y, t) says the new page is there at t.
	fx := func(x int) float64 { return (float64(x) + 0.5) / float64(rw) }
	fy := func(y int) float64 { return (float64(y) + 0.5) / float64(rh) }
	in := func(x, y int, t float64) bool {
		switch anim {
		case turnSlide:
			return fx(x) >= 1-t
		case turnDissolve:
			return fxRand(x, y, 31) < t
		case turnWipeDown:
			return fy(y) <= t
		case turnWipeUp:
			return fy(y) >= 1-t
		case turnIris:
			dx, dy := (fx(x)-0.5)*float64(rw)/2, (fy(y)-0.5)*float64(rh)
			far := math.Hypot(float64(rw)/4, float64(rh)/2)
			return math.Hypot(dx, dy) <= t*far*1.02
		case turnBlinds:
			return float64(y%4)+0.5 <= t*4
		case turnTallBlinds:
			return float64(x%8)+0.5 <= t*8
		case turnCheckers:
			bx, by := x/8, y/4
			k := t*2 - float64((bx+by)%2)*0.6
			return (float64(x%8)+0.5)/8 <= k
		case turnDiagonal:
			return (fx(x)+fy(y))/2 <= t
		case turnBurn:
			n := fxNoise(float64(x)/7, float64(y)/3.5)
			return n < t*1.25-0.1
		case turnRain:
			d := fxRand(x, 97) * 0.45
			cp := fxClamp01((t - d) / 0.55)
			return fy(y) <= cp
		}
		return t >= 0.5
	}
	news := func(x, y int) bool {
		if ahead {
			return in(x, y, e)
		}
		return !in(x, y, 1-e)
	}
	for y := 0; y < rh; y++ {
		for x := 0; x < rw; x++ {
			isNew := news(x, y)
			c := cell(old, x, y)
			if isNew {
				c = cell(now, x, y)
			}
			// The edge between the two: a shadow, or for some an ember glow.
			edge := false
			for _, d := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}, {-2, 0}, {2, 0}} {
				xx, yy := x+d[0], y+d[1]
				if xx >= 0 && yy >= 0 && xx < rw && yy < rh && news(xx, yy) != isNew {
					edge = true
					break
				}
			}
			if edge && e > 0.02 && e < 0.98 {
				switch anim {
				case turnBurn:
					if !isNew {
						c = shade(c, cEmber, 0.7)
						c.Content, c.Width = []string{"░", "▒", "▓"}[int(fxRand(x, y, int(p*20))*3)%3], 1
						c.Style.Fg = rgbaOf(mixHex(cAccent, "#ffffff", 0.3))
					}
				case turnIris, turnRain:
					c = shade(c, cAccent, 0.35)
				case turnDissolve, turnCheckers:
				default:
					if isNew {
						c = shade(c, "#000000", 0.3)
					}
				}
			}
			set(x, y, c)
		}
	}
}

// snapshot copies the cells of cv inside r (rows of r's width).
func snapshot(cv *boardCanvas, r [4]int) [][]uv.Cell {
	out := make([][]uv.Cell, r[3])
	for y := range out {
		out[y] = make([]uv.Cell, r[2])
		for x := range out[y] {
			xx, yy := r[0]+x, r[1]+y
			if xx >= 0 && yy >= 0 && xx < cv.Width() && yy < cv.Height() {
				out[y][x] = cv.Lines[yy][xx]
			}
		}
	}
	return out
}

// fxNoise is smooth noise in 0 … 1 (value noise on a grid).
func fxNoise(x, y float64) float64 {
	x0, y0 := math.Floor(x), math.Floor(y)
	tx, ty := smooth(x-x0), smooth(y-y0)
	at := func(i, j float64) float64 { return fxRand(int(i), int(j), 53) }
	a := at(x0, y0) + (at(x0+1, y0)-at(x0, y0))*tx
	b := at(x0, y0+1) + (at(x0+1, y0+1)-at(x0, y0+1))*tx
	return a + (b-a)*ty
}

// codexTurnFix mends wide letters a turn cut in half, so a row never shifts.
func codexTurnFix(cv *boardCanvas, r [4]int) {
	for y := max(r[1], 0); y < min(r[1]+r[3], cv.Height()); y++ {
		row := cv.Lines[y]
		for x := 0; x < len(row); x++ {
			c := row[x]
			switch {
			case c.Width >= 2:
				if x+1 >= len(row) || row[x+1].Width != 0 || row[x+1].Content != "" {
					row[x].Content, row[x].Width = " ", 1
				} else {
					x++
				}
			case c.Width == 0:
				row[x].Content, row[x].Width = " ", 1
			}
		}
	}
}
