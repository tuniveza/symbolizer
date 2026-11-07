package main

// ---- the mini map ------------------------------------------------------------------
//
// AltGr+M shows a small map of the whole board in a corner of the screen,
// and moves it on round the corners (top right, bottom right, bottom left,
// top left), then hides it. Each structure is its kind's symbol (◆ shape,
// ≡ page, ✎ note, ≣ cabinet …) in its colour, the chosen one picked out;
// links are dotted lines between them, and a frame shows what the screen
// shows now.

import (
	"math"

	uv "github.com/charmbracelet/ultraviolet"
)

var mapCornerNames = []string{"", "top right", "bottom right", "bottom left", "top left"}

// designMapToggle shows the mini map, moves it to the next corner, or hides it.
func (m *Model) designMapToggle() {
	d := m.ds()
	d.mapCorner = (d.mapCorner + 1) % len(mapCornerNames)
	d.dirty = true
	if d.mapCorner == 0 {
		m.say("Map hidden.")
		return
	}
	m.say("Map: " + mapCornerNames[d.mapCorner] + " · AltGr+M again moves it on")
}

// paintMap draws the mini map onto the board's canvas.
func (m *Model) paintMap(c *boardCanvas) {
	d := m.ds()
	if d.mapCorner == 0 {
		return
	}
	cw, ch := c.Width(), c.Height()
	mw, mh := min(30, cw/3), min(11, ch/2)
	if mw < 10 || mh < 5 {
		return // the screen is too small for it
	}
	x0, y0 := cw-mw-1, 1 // top right
	switch d.mapCorner {
	case 2:
		x0, y0 = cw-mw-1, ch-mh-1
	case 3:
		x0, y0 = 1, ch-mh-1
	case 4:
		x0, y0 = 1, 1
	}
	iw, ih := mw-2, mh-2

	bg := rgbaOf(mixHex(cBG, cBarBG, 0.6))
	edge := rgbaOf(mixHex(cDim, cBG, 0.2))
	put := func(x, y int, s string, fg any, bold bool) {
		if x < 0 || y < 0 || x >= cw || y >= ch {
			return
		}
		st := uv.Style{Bg: bg}
		if f, ok := fg.(string); ok {
			st.Fg = rgbaOf(f)
		} else {
			st.Fg = edge
		}
		if bold {
			st.Attrs = uv.AttrBold
		}
		c.Lines[y][x] = uv.Cell{Content: s, Width: 1, Style: st}
	}

	// The frame and its title.
	for y := 0; y < mh; y++ {
		for x := 0; x < mw; x++ {
			s := " "
			switch {
			case y == 0 && x == 0:
				s = "╭"
			case y == 0 && x == mw-1:
				s = "╮"
			case y == mh-1 && x == 0:
				s = "╰"
			case y == mh-1 && x == mw-1:
				s = "╯"
			case y == 0 || y == mh-1:
				s = "─"
			case x == 0 || x == mw-1:
				s = "│"
			}
			put(x0+x, y0+y, s, nil, false)
		}
	}
	for i, r := range " map " {
		put(x0+2+i, y0, string(r), mixHex(cDim, cText, 0.4), false)
	}

	// What the map covers: every structure and the screen, with a margin.
	z := d.z()
	vx0, vy0 := d.camX, d.camY
	vx1, vy1 := d.camX+float64(cw)/z, d.camY+float64(ch)/z
	bx0, by0, bx1, by1 := vx0, vy0, vx1, vy1
	for _, o := range d.octs {
		if o == nil {
			continue
		}
		w, h := d.size(o)
		bx0, by0 = math.Min(bx0, float64(o.X)), math.Min(by0, float64(o.Y))
		bx1, by1 = math.Max(bx1, float64(o.X+w)), math.Max(by1, float64(o.Y+h))
	}
	pad := math.Max(bx1-bx0, by1-by0) * 0.04
	bx0, by0, bx1, by1 = bx0-pad, by0-pad, bx1+pad, by1+pad
	// One scale both ways: a map cell has the shape of a board cell.
	s := math.Min(float64(iw)/(bx1-bx0), float64(ih)/(by1-by0))
	offX := (float64(iw) - (bx1-bx0)*s) / 2
	offY := (float64(ih) - (by1-by0)*s) / 2
	at := func(wx, wy float64) (int, int) {
		x := int(math.Floor((wx-bx0)*s + offX))
		y := int(math.Floor((wy-by0)*s + offY))
		return x0 + 1 + max(0, min(iw-1, x)), y0 + 1 + max(0, min(ih-1, y))
	}

	// The screen: its corners.
	sx0, sy0 := at(vx0, vy0)
	sx1, sy1 := at(vx1, vy1)
	sx1, sy1 = max(sx1-1, sx0), max(sy1-1, sy0)
	view := mixHex(cAccent, cBG, 0.35)
	for x := sx0; x <= sx1; x++ {
		put(x, sy0, "┄", view, false)
		put(x, sy1, "┄", view, false)
	}
	for y := sy0; y <= sy1; y++ {
		put(sx0, y, "┆", view, false)
		put(sx1, y, "┆", view, false)
	}
	put(sx0, sy0, "┌", view, false)
	put(sx1, sy0, "┐", view, false)
	put(sx0, sy1, "└", view, false)
	put(sx1, sy1, "┘", view, false)

	// Links: dotted lines between the structures they join.
	centre := func(o *dOct) (int, int) {
		w, h := d.size(o)
		return at(float64(o.X)+float64(w)/2, float64(o.Y)+float64(h)/2)
	}
	link := mixHex(cDim, cText, 0.25)
	for _, l := range d.links {
		a, b := d.byID(l.A), d.byID(l.B)
		if a == nil || b == nil {
			continue
		}
		ax, ay := centre(a)
		bx, by := centre(b)
		n := max(abs(bx-ax), abs(by-ay))
		for i := 1; i < n; i++ {
			put(ax+(bx-ax)*i/n, ay+(by-ay)*i/n, "·", link, false)
		}
	}

	// The structures: their kind's symbol, in their colour; the chosen one bright.
	for _, o := range d.octs {
		if o == nil {
			continue
		}
		x, y := centre(o)
		col := mixHex(o.Color, cText, 0.15)
		if o.ID == d.sel {
			col = cAccent
		}
		put(x, y, kindIcons[o.Kind], col, o.ID == d.sel)
	}
}
