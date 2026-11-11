package main

// ---- the box: things in it on three layers, top, middle and bottom ----------------------
//
// A box (Alt+N, then O) is like a real box with things in it, seen from
// above: each thing lies on one of three layers, top, middle or bottom, at
// its own place inside the box, so things overlap. The bottom layer is
// drawn first, smaller and darker (deeper in the box), then the middle
// layer, then the top layer, brightest. Each thing is drawn small (a
// miniature: its outline, icon and title).
//
// A box has a design, and each design has its own outline: a cardboard box
// (open flaps at its top corners), a wooden crate (posts, planks and
// nails), a metal tin (round corners, a shine), a gift box (a ribbon and a
// bow), a round hat box (a curved outline) and a hexagonal box. D (Shift+D:
// back), with the box chosen, or the [◇] on its top edge, changes it. The
// design is kept in the box's Layout.
//
//	drop a thing on it   puts it in, on the top layer, where it is let go
//	click a thing        chooses it (the top one under the mouse)
//	drag a thing         takes it out: it follows the mouse
//	Tab / Shift+Tab      chooses the box's next (last) thing
//	PgUp / PgDn          a thing chosen: moves it to the layer above (below);
//	                     none chosen: looks at one layer (the layers above it
//	                     go see-through, so the things in it can be clicked)
//	Alt + arrows         moves the chosen thing inside the box
//	Enter                takes the chosen thing out, beside the box
//	X X                  takes the chosen thing away (Ctrl+Z brings it back)
//	Space                opens or shuts its lid (it moves, design_anim.go)
//	Esc                  lets go of the thing, then of the layer
//
// Which layer a thing is on is its Slot (0 top, 1 middle, 2 bottom); its
// place is its X and Y, in board cells from the box's inside corner (as a
// pin on a board). Those are changes (undo sees them). The lid being open
// (DrawerOpen), the layer looked at and the chosen thing are looking
// around. Drawing and clicking both use one grid of cells (boxGridOf), so
// a click finds exactly what is drawn under it, at every zoom.

import (
	"fmt"
	"math"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// A box's designs (its Layout).
const (
	boxCardboard = iota
	boxCrate
	boxTin
	boxGift
	boxHat
	boxHex
	boxDesigns // how many there are
)

var boxDesignNames = [boxDesigns]string{"cardboard box", "wooden crate", "metal tin", "gift box", "round hat box", "hexagonal box"}

// boxLayers is how many layers a box has; boxLayerNames are their names.
const boxLayers = 3

var boxLayerNames = [boxLayers]string{"top", "middle", "bottom"}

// A new box's size, and the smallest a box can be, on the board.
const (
	boxNewW, boxNewH = 46, 17
	boxMinW, boxMinH = miniW + 12, miniH + 6
)

// boxDesign is o's design.
func boxDesign(o *dOct) int { return (o.Layout%boxDesigns + boxDesigns) % boxDesigns }

// boxLayerOf is which layer thing it is on (0 top, 1 middle, 2 bottom).
func boxLayerOf(it *dOct) int { return min(max(it.Slot, 0), boxLayers-1) }

// boxBoardSize is a box's own size on the board.
func boxBoardSize(o *dOct) (int, int) { return max(o.W, boxMinW), max(o.H, boxMinH) }

// ---- its shape ------------------------------------------------------------------------------

// boxShape is a design's outline, w × h cells big.
type boxShape struct {
	design      int
	f0          int   // its first row of frame (a cardboard box's flaps and a gift box's bow are above it)
	top, bottom int   // rows of frame above and below where things go
	wall        int   // cells of wall at each side (a crate's posts are two)
	left, right int   // cells beside where things go (a hexagon's slanted sides take more)
	label       int   // the row its name is on
	cut         []int // each row: cells cut off at each side (a hexagon's slant, a hat box's curve)
	hatK        int   // a hat box: how far in its curve goes
}

// boxHexSlant is how far a hexagonal box w × h comes in at its top and
// bottom: a slant of a cell a row, unless that is too much for its width.
func boxHexSlant(w, h int) int { return max(1, min((h-1)/2, w/5)) }

func boxShapeOf(design, w, h int) boxShape {
	s := boxShape{design: design, top: 1, bottom: 1, wall: 1, left: 1, right: 1, cut: make([]int, max(h, 0))}
	switch design {
	case boxCardboard, boxGift:
		s.f0, s.top, s.label = 1, 2, 1
	case boxCrate:
		s.wall, s.left, s.right = 2, 2, 2
	case boxTin:
		s.top = 2
	case boxHat:
		s.top, s.bottom = 2, 2
		s.hatK = min(max(w/8, 2), 5)
		if h > 0 {
			s.cut[0], s.cut[h-1] = s.hatK, s.hatK
		}
	case boxHex:
		a := boxHexSlant(w, h)
		mid := float64(h-1) / 2
		for r := range s.cut {
			if mid > 0 {
				s.cut[r] = int(math.Round(math.Abs(float64(r)-mid) / mid * float64(a)))
			}
		}
		s.left, s.right = 1+a/2, 1+a/2
	}
	return s
}

// inside is whether cell (x, y) is inside the box's walls: where its floor
// and the things in it show.
func (s boxShape) inside(x, y, w, h int) bool {
	if s.design == boxHat && (y == 1 || y == h-2) {
		return x > s.hatK && x < w-1-s.hatK
	}
	if y < s.top || y > h-1-s.bottom || y >= len(s.cut) {
		return false
	}
	return x >= s.cut[y]+s.wall && x <= w-1-s.cut[y]-s.wall
}

// insideRows are the rows of a box w × h that have inside cells, top to bottom.
func (s boxShape) insideRows(w, h int) []int {
	var rows []int
	for y := range h {
		for x := range w {
			if s.inside(x, y, w, h) {
				rows = append(rows, y)
				break
			}
		}
	}
	return rows
}

// boxPlaces is where things go inside a box drawn w × h: the inside's corner
// and size on the screen, and screen cells to a board cell across and down.
func boxPlaces(o *dOct, w, h int) (ix, iy, iw, ih int, kx, ky float64) {
	design := boxDesign(o)
	bw, bh := boxBoardSize(o)
	sb := boxShapeOf(design, bw, bh)
	inW, inH := max(1, bw-sb.left-sb.right), max(1, bh-sb.top-sb.bottom)
	s := boxShapeOf(design, w, h)
	ix, iy = s.left, s.top
	iw, ih = max(1, w-s.left-s.right), max(1, h-s.top-s.bottom)
	return ix, iy, iw, ih, float64(iw) / float64(inW), float64(ih) / float64(inH)
}

// boxRoom is how far a thing's place can go in box o (board cells from its
// inside corner): what keeps its miniature inside.
func boxRoom(o *dOct) (int, int) {
	bw, bh := boxBoardSize(o)
	sb := boxShapeOf(boxDesign(o), bw, bh)
	return max(0, bw-sb.left-sb.right-miniW), max(0, bh-sb.top-sb.bottom-miniH)
}

// boxClamp keeps thing it inside box o.
func boxClamp(o, it *dOct) {
	mx, my := boxRoom(o)
	it.X, it.Y = min(max(it.X, 0), mx), min(max(it.Y, 0), my)
}

// boxDepth is how much smaller a layer is drawn (deeper in the box).
var boxDepth = [boxLayers]float64{1, 0.88, 0.76}

// boxThingRect is where thing it of box o is drawn, w × h on the screen,
// from the box's corner: further down, it is smaller and nearer the
// middle, as things deeper in a box look.
func boxThingRect(o, it *dOct, w, h int) (x, y, tw, th int) {
	ix, iy, iw, ih, kx, ky := boxPlaces(o, w, h)
	s := boxDepth[boxLayerOf(it)]
	pw, ph := float64(miniW)*kx, float64(miniH)*ky
	cx := float64(ix) + float64(it.X)*kx + pw/2
	cy := float64(iy) + float64(it.Y)*ky + ph/2
	mx, my := float64(ix)+float64(iw)/2, float64(iy)+float64(ih)/2
	cx, cy = mx+(cx-mx)*s, my+(cy-my)*s
	tw, th = max(3, int(math.Round(pw*s))), max(1, int(math.Round(ph*s)))
	return int(math.Round(cx - float64(tw)/2)), int(math.Round(cy - float64(th)/2)), tw, th
}

// ---- its colours ----------------------------------------------------------------------------

// boxLook is a design's colours: its walls, the lines on them, its floor
// (inside, in shadow), its lid, and a second colour (tape, ribbon, nails …).
type boxLook struct {
	wall, edge, floor, lid, extra string
}

func boxLookOf(o *dOct) boxLook {
	base := map[int]string{boxCardboard: "#b5895a", boxCrate: "#9c6b3c", boxTin: "#9aa5af", boxHat: "#d4a5c0"}[boxDesign(o)]
	if base == "" {
		base = o.Color // a gift box's paper, a hexagonal box: its own colour
	}
	l := boxLook{
		wall:  mixHex(base, cBG, 0.25),
		edge:  mixHex(base, "#1a1410", 0.45),
		floor: mixHex(cBG, base, 0.2),
		lid:   mixHex(base, cBG, 0.1),
		extra: "#d9c48f",
	}
	switch boxDesign(o) {
	case boxCardboard:
		l.extra = "#d8c9a3" // tape
	case boxCrate:
		l.edge, l.extra = mixHex(base, "#1a1410", 0.55), "#c9ced3" // nails
	case boxTin:
		l.edge, l.extra = mixHex(base, "#20262c", 0.5), "#f1f5f8" // its shine
	case boxGift:
		l.extra = "#e5c07b" // gold ribbon
	case boxHat:
		l.edge, l.extra = mixHex(base, "#2a1a24", 0.5), "#f6e6ef" // stripes
	case boxHex:
		l.edge = mixHex(base, cText, 0.35)
	}
	return l
}

// ---- its cells --------------------------------------------------------------------------------

// boxCell is one cell of a box's drawing, and what a click on it does.
type boxCell struct {
	r      string
	fg, bg string
	bold   bool
	act    string
}

// boxGrid is a box drawn, cell by cell: drawing and clicking both read it.
type boxGrid struct {
	w, h int
	c    [][]boxCell
}

func (g *boxGrid) set(x, y int, r, fg, bg string) {
	if x >= 0 && y >= 0 && x < g.w && y < g.h {
		g.c[y][x] = boxCell{r: r, fg: fg, bg: bg}
	}
}

// text writes s from (x, y), at most up to column end (inclusive).
func (g *boxGrid) text(x, y, end int, s, fg, bg string, bold bool, act string) int {
	for _, r := range s {
		if x > end || x >= g.w {
			break
		}
		if x >= 0 && y >= 0 && y < g.h {
			g.c[y][x] = boxCell{r: string(r), fg: fg, bg: bg, bold: bold, act: act}
		}
		x++
	}
	return x
}

// boxView is the layer looked at in box o: 0 all of them, 1 top, 2
// middle, 3 bottom.
func (d *designState) boxView(o *dOct) int { return d.boxLayer[o.ID] }

// boxGridOf draws box o, w × h on the screen.
func (d *designState) boxGridOf(o *dOct, w, h int) *boxGrid {
	g := &boxGrid{w: w, h: h, c: make([][]boxCell, h)}
	for y := range g.c {
		g.c[y] = make([]boxCell, w)
		for x := range g.c[y] {
			g.c[y][x] = boxCell{r: " ", fg: cText, bg: cBG}
		}
	}
	design := boxDesign(o)
	s := boxShapeOf(design, w, h)
	lk := boxLookOf(o)
	selected := d.sel == o.ID
	if selected {
		lk.edge = mixHex(lk.edge, cText, 0.4)
	}
	if d.dropOn == o.ID {
		lk.edge, lk.floor = cAccent, mixHex(lk.floor, cAccent, 0.18)
	}
	p := d.slotOpenness(o, 0, o.DrawerOpen)
	shut := p < 0.5
	d.boxFrame(g, o, s, lk, shut)

	// Its floor, and on it the things, bottom layer first.
	for y := range h {
		for x := range w {
			if s.inside(x, y, w, h) {
				g.c[y][x] = boxCell{r: " ", fg: cText, bg: boxFloorAt(design, lk, x, y, w, s), act: "boxfloor"}
				if design == boxCrate && (y-s.top)%3 == 2 {
					g.c[y][x].r, g.c[y][x].fg = "─", mixHex(lk.floor, lk.edge, 0.35) // the planks of its back
				}
			}
		}
	}
	rows := s.insideRows(w, h)
	covered := int(math.Round((1 - p) * float64(len(rows))))
	if covered < len(rows) {
		d.boxThings(g, o, s, lk)
	}
	if covered > 0 {
		boxLid(g, o, s, lk, rows[:covered], covered == len(rows))
	}
	d.boxEdges(g, o, s, lk, shut)
	return g
}

// boxFloorAt is the colour of a box's floor at (x, y): a tin's has a shine
// across it, a hat box's a soft stripe.
func boxFloorAt(design int, lk boxLook, x, y, w int, s boxShape) string {
	switch design {
	case boxTin: // a soft band of light, slanting down from its top
		k := float64(x-w/3) + 1.5*float64(y)
		return mixHex(lk.floor, lk.extra, 0.09*math.Exp(-math.Pow(k/5, 2)))
	case boxHat:
		if (x/3)%2 == 0 {
			return mixHex(lk.floor, lk.extra, 0.05)
		}
	}
	return lk.floor
}

// boxFrame draws a box's walls and outline, by its design.
func (d *designState) boxFrame(g *boxGrid, o *dOct, s boxShape, lk boxLook, shut bool) {
	w, h := g.w, g.h
	wall := func(x, y int, r string) { g.set(x, y, r, lk.edge, lk.wall) }
	rect := func(y0 int, tl, tr, bl, br, hz, vt string) {
		for x := 1; x < w-1; x++ {
			wall(x, y0, hz)
			wall(x, h-1, hz)
		}
		for y := y0 + 1; y < h-1; y++ {
			for k := range s.wall {
				wall(k, y, vt)
				wall(w-1-k, y, vt)
			}
		}
		wall(0, y0, tl)
		wall(w-1, y0, tr)
		wall(0, h-1, bl)
		wall(w-1, h-1, br)
	}
	switch s.design {
	case boxCardboard:
		rect(1, "┌", "┐", "└", "┘", "─", "│")
		if !shut { // its flaps, open, folded out at its top corners
			fw := max(3, w/3)
			flap := mixHex(lk.wall, "#ffffff", 0.1)
			for x := 1; x < fw-1; x++ {
				g.set(x, 0, " ", lk.edge, flap)
				g.set(w-1-x, 0, " ", lk.edge, flap)
			}
			g.set(0, 0, "◤", flap, cBG)
			g.set(fw-1, 0, "◣", flap, cBG)
			g.set(w-1, 0, "◥", flap, cBG)
			g.set(w-fw, 0, "◢", flap, cBG)
		}
		for x := w/2 - 2; x <= w/2+1; x++ { // tape across its bottom
			g.set(x, h-1, "═", mixHex(lk.extra, lk.edge, 0.3), lk.extra)
		}
	case boxCrate:
		plank := mixHex(lk.wall, "#ffffff", 0.06)
		for x := range w { // planks along its top and bottom
			g.set(x, 0, "▁", mixHex(plank, lk.edge, 0.4), plank)
			g.set(x, h-1, "▔", mixHex(plank, lk.edge, 0.4), plank)
		}
		for y := 1; y < h-1; y++ { // a post each side
			g.set(0, y, "▌", lk.edge, lk.wall)
			g.set(1, y, " ", lk.edge, lk.wall)
			g.set(w-2, y, " ", lk.edge, lk.wall)
			g.set(w-1, y, "▐", lk.edge, lk.wall)
			if (y-s.top)%3 == 2 || y == 1 || y == h-2 { // a nail where each plank meets it
				g.set(1, y, "•", lk.extra, lk.wall)
				g.set(w-2, y, "•", lk.extra, lk.wall)
			}
		}
		for _, x := range []int{1, w - 2} {
			g.set(x, 0, "•", lk.extra, plank)
			g.set(x, h-1, "•", lk.extra, plank)
		}
	case boxTin:
		rect(0, "╭", "╮", "╰", "╯", "─", "│")
		for x := 1; x < w-1; x++ { // its rolled rim
			wall(x, 1, "═")
		}
		wall(0, 1, "╞")
		wall(w-1, 1, "╡")
		for x := range w { // a shine along its rim, a third of the way along
			t := float64(x) / float64(max(w-1, 1))
			shine := math.Exp(-math.Pow((t-0.3)/0.2, 2))
			for _, y := range []int{0, 1} {
				c := &g.c[y][x]
				c.fg = mixHex(c.fg, lk.extra, 0.5*shine)
				c.bg = mixHex(c.bg, lk.extra, 0.22*shine)
			}
		}
	case boxGift:
		rect(1, "┌", "┐", "└", "┘", "─", "│")
		cx := w / 2
		for _, y := range []int{1, h - 1} { // its ribbon, round it
			g.set(cx, y, "║", mixHex(lk.extra, lk.edge, 0.3), lk.extra)
		}
		mid := (1 + h) / 2
		g.set(0, mid, "╪", mixHex(lk.extra, lk.edge, 0.3), lk.extra)
		g.set(w-1, mid, "╪", mixHex(lk.extra, lk.edge, 0.3), lk.extra)
		if shut { // its bow, on the lid
			g.text(cx-1, 0, w-1, "⊰⋈⊱", lk.extra, cBG, true, "")
		} else { // the lid, off, behind it
			lid := mixHex(lk.wall, "#ffffff", 0.08)
			for x := 2; x < w-2; x++ {
				g.set(x, 0, "▄", lid, cBG)
			}
			g.set(cx, 0, "▄", lk.extra, cBG)
		}
	case boxHat:
		k := s.hatK
		for x := k + 1; x < w-1-k; x++ {
			wall(x, 0, "─")
			stripe := lk.wall
			if (x/2)%2 == 0 {
				stripe = mixHex(lk.wall, lk.extra, 0.45) // its striped band
			}
			g.set(x, h-1, "─", lk.edge, stripe)
		}
		wall(k, 0, "╭")
		wall(w-1-k, 0, "╮")
		wall(k, h-1, "╰")
		wall(w-1-k, h-1, "╯")
		for x := 1; x < k; x++ {
			wall(x, 1, "─")
			wall(w-1-x, 1, "─")
			wall(x, h-2, "─")
			wall(w-1-x, h-2, "─")
		}
		wall(0, 1, "╭")
		wall(k, 1, "╯")
		wall(w-1-k, 1, "╰")
		wall(w-1, 1, "╮")
		wall(0, h-2, "╰")
		wall(k, h-2, "╮")
		wall(w-1-k, h-2, "╭")
		wall(w-1, h-2, "╯")
		for y := 2; y < h-2; y++ {
			wall(0, y, "│")
			wall(w-1, y, "│")
		}
	case boxHex:
		// Its slanted sides: each cell half wall, cut on the slant, so
		// the side is one smooth line.
		mid := float64(h-1) / 2
		side := lk.wall
		if d.sel == o.ID || d.dropOn == o.ID {
			side = mixHex(lk.wall, lk.edge, 0.5)
		}
		for y := range h {
			c := s.cut[y]
			l, r := "◢", "◣"
			switch {
			case float64(y) > mid:
				l, r = "◥", "◤"
			case float64(y) == mid:
				l, r = "▐", "▌"
			}
			if y == 0 || y == h-1 {
				for x := c + 1; x < w-1-c; x++ {
					wall(x, y, "─")
				}
			}
			g.set(c, y, l, side, cBG)
			g.set(w-1-c, y, r, side, cBG)
		}
	}
}

// boxThings draws what is in a box, bottom layer first: each thing its
// miniature, clipped by the walls. The layer looked at (if any) is drawn
// bright and the layers above it only as faint outlines, which a click
// goes through; a click on a cell finds the top thing drawn there.
func (d *designState) boxThings(g *boxGrid, o *dOct, s boxShape, lk boxLook) {
	w, h := g.w, g.h
	view := d.boxView(o) - 1 // -1: all
	_, chosen := d.chosenBoxThing()
	var sel *dOct
	for l := boxLayers - 1; l >= 0; l-- {
		ghost := view >= 0 && l < view
		dark := []float64{0, 0.28, 0.48}[l]
		switch {
		case view >= 0 && l == view:
			dark = 0
		case view >= 0 && l > view:
			dark = 0.5
		}
		order := boxLayerThings(o, l)
		for _, j := range order {
			it := o.Items[j]
			if it == chosen {
				sel = it
				continue // drawn last on its layer, to be seen
			}
			d.boxDrawThing(g, o, s, lk, j, false, ghost, dark)
		}
		if sel != nil && boxLayerOf(sel) == l {
			for j, it := range o.Items {
				if it == sel {
					d.boxDrawThing(g, o, s, lk, j, true, ghost, dark)
				}
			}
		}
	}
	if sel == nil {
		return
	}
	// The chosen thing's outline, over everything, so it shows even under others.
	x0, y0, tw, th := boxThingRect(o, sel, w, h)
	for y := y0; y < y0+th; y++ {
		for x := x0; x < x0+tw; x++ {
			edge := y == y0 || y == y0+th-1 || x == x0 || x == x0+tw-1
			if edge && th >= 3 && s.inside(x, y, w, h) && g.c[y][x].act != "boxthing:"+itoa(itemIndex(o, sel)) {
				c := &g.c[y][x]
				c.fg = cAccent
				if c.r == " " {
					c.r = "┈"
					if x == x0 || x == x0+tw-1 {
						c.r = "┊"
					}
				}
			}
		}
	}
}

// itemIndex is where it is in o's things (-1: not there).
func itemIndex(o, it *dOct) int {
	for j, t := range o.Items {
		if t == it {
			return j
		}
	}
	return -1
}

// boxLayerThings are the indexes (in o.Items) of what is on layer l, in
// the order they are drawn (the last on top).
func boxLayerThings(o *dOct, l int) []int {
	var idx []int
	for j, it := range o.Items {
		if it != nil && boxLayerOf(it) == l {
			idx = append(idx, j)
		}
	}
	return idx
}

// boxDrawThing draws thing j of box o: its miniature (darker by dark), or
// as a ghost only its outline, faint, owning no cell.
func (d *designState) boxDrawThing(g *boxGrid, o *dOct, s boxShape, lk boxLook, j int, sel, ghost bool, dark float64) {
	it := o.Items[j]
	x0, y0, tw, th := boxThingRect(o, it, g.w, g.h)
	mini := miniOfSize(it, tw, th, sel, true)
	act := "boxthing:" + itoa(j)
	for yy, row := range mini {
		for xx, mc := range row {
			x, y := x0+xx, y0+yy
			if !s.inside(x, y, g.w, g.h) {
				continue
			}
			c := &g.c[y][x]
			if ghost {
				if mc.r != " " && th >= 3 && (yy == 0 || yy == th-1 || xx == 0 || xx == tw-1) {
					c.r, c.fg = mc.r, mixHex(mc.fg, c.bg, 0.6)
				}
				continue
			}
			if mc.bg == "" { // a clear corner: what is under it shows
				c.r, c.fg = mc.r, mixHex(mc.fg, c.bg, dark)
				continue
			}
			*c = boxCell{r: mc.r, fg: mixHex(mc.fg, lk.floor, dark), bg: mixHex(mc.bg, lk.floor, dark), act: act}
		}
	}
}

// boxLid draws a box's lid over the given inside rows; shut right down,
// with its name, how many things it holds, and a button that opens it.
func boxLid(g *boxGrid, o *dOct, s boxShape, lk boxLook, rows []int, whole bool) {
	w, h := g.w, g.h
	cx := w / 2
	for i, y := range rows {
		for x := range w {
			if !s.inside(x, y, w, h) {
				continue
			}
			r, fg, bg := " ", lk.edge, lk.lid
			switch s.design {
			case boxCardboard: // two flaps folded down, taped where they meet
				if x == cx-1 || x == cx {
					r, bg = " ", lk.extra
				}
			case boxCrate: // planks, nailed at each end
				if i%2 == 1 {
					r, fg = "─", mixHex(lk.lid, lk.edge, 0.5)
				}
				if x == s.cut[y]+s.wall && i%2 == 0 || x == w-1-s.cut[y]-s.wall && i%2 == 0 {
					r, fg = "•", lk.extra
				}
			case boxTin: // metal, with a shine across it
				if k := float64(x-w/3) + 1.5*float64(y); math.Abs(k) < 12 {
					bg = mixHex(lk.lid, lk.extra, 0.22*math.Exp(-math.Pow(k/5, 2)))
				}
			case boxGift: // the ribbon, across and down
				if x == cx || y == rows[len(rows)/2] {
					r, fg, bg = " ", lk.edge, lk.extra
				}
			case boxHat: // stripes
				if (x/2)%2 == 0 {
					bg = mixHex(lk.lid, lk.extra, 0.4)
				}
			case boxHex: // a line round inside its edge
				if x == s.cut[y]+s.wall || x == w-1-s.cut[y]-s.wall {
					r, fg = "·", mixHex(lk.lid, lk.edge, 0.5)
				}
			}
			g.c[y][x] = boxCell{r: r, fg: fg, bg: bg}
		}
	}
	if !whole || len(rows) == 0 {
		return
	}
	// Its name and what it holds, in the middle; under them, a button.
	name := strings.TrimSpace(o.Header)
	if name == "" {
		name = boxDesignNames[s.design]
	}
	lines := []string{kindIcons[kBox] + " " + name, plural(len(drawerThings(o)), "thing")}
	mid := rows[max(0, len(rows)/2-1)]
	if len(rows) < 4 {
		lines = []string{kindIcons[kBox] + " " + name + " · " + itoa(len(drawerThings(o)))}
		mid = rows[len(rows)/2]
	}
	if len(rows) >= 5 {
		lines = append(lines, "[open]")
	}
	for i, t := range lines {
		y := mid + i
		if y > rows[len(rows)-1] {
			break
		}
		lo, hi := 0, w-1
		for lo < w && !s.inside(lo, y, w, h) {
			lo++
		}
		for hi >= 0 && !s.inside(hi, y, w, h) {
			hi--
		}
		t = ansi.Truncate(t, max(hi-lo+1, 0), "…")
		x := lo + max(0, (hi-lo+1-lipgloss.Width(t))/2)
		bg, act := mixHex(lk.lid, cBG, 0.35), ""
		fg := boxInk(bg)
		if t == "[open]" {
			bg, fg, act = mixHex(cBG, lk.wall, 0.2), cAccent, "boxlid"
		}
		g.text(x, y, hi, t, fg, bg, true, act)
	}
}

// boxEdges writes on a box's edges: its name and how many things it holds
// on its top edge (and, chosen, its design and lid buttons), and how many
// are on each layer on its bottom edge (a click there looks at that layer).
func (d *designState) boxEdges(g *boxGrid, o *dOct, s boxShape, lk boxLook, shut bool) {
	w, h := g.w, g.h
	y := s.label
	lo := s.cut[y] + 2
	hi := w - 3 - s.cut[y]
	if y == 0 {
		hi = min(hi, w-6) // the ✕ that deletes it is at its top right
	}
	bg := lk.wall
	head := boxInk(bg)
	btnBG := mixHex(cBG, lk.wall, 0.2) // a button: dark, so it reads on any wall
	if d.sel == o.ID {
		lid := "[shut]"
		if shut {
			lid = "[open]"
		}
		btn := "[◇] " + lid
		if bx := hi - lipgloss.Width(btn) + 1; bx > lo+6 {
			g.text(bx, y, hi, "[◇]", cAccent, btnBG, true, "boxdesign")
			g.text(bx+4, y, hi, lid, cAccent, btnBG, true, "boxlid")
			hi = bx - 2
		}
	}
	name := o.Header
	if d.sel == o.ID && d.editing == partHeader {
		name += "▏"
	} else if strings.TrimSpace(name) == "" {
		name = boxDesignNames[s.design]
	}
	if n := len(drawerThings(o)); n > 0 && !(d.sel == o.ID && d.editing == partHeader) {
		name += " · " + itoa(n)
	}
	if hi-lo >= 3 {
		g.text(lo, y, hi, ansi.Truncate(" "+kindIcons[kBox]+" "+name+" ", hi-lo+1, "…"), head, bg, true, "")
	}
	if shut || h < 6 {
		return
	}
	// The layers, with how many things each has.
	y = h - 1
	lo, hi = s.cut[y]+2, w-3-s.cut[y]
	view := d.boxView(o) - 1
	parts := make([]string, boxLayers)
	short := make([]string, boxLayers)
	for l := range boxLayers {
		n := len(boxLayerThings(o, l))
		parts[l] = " " + boxLayerNames[l] + " " + itoa(n) + " "
		short[l] = " " + strings.ToUpper(boxLayerNames[l][:1]) + itoa(n) + " "
	}
	total := func(ps []string) int {
		n := 0
		for _, p := range ps {
			n += lipgloss.Width(p) + 1
		}
		return n - 1
	}
	if total(parts) > hi-lo+1 {
		parts = short
	}
	if total(parts) > hi-lo+1 {
		return
	}
	x := lo + (hi-lo+1-total(parts))/2
	for l, p := range parts {
		pbg := lk.wall
		fg := boxInk(pbg)
		if l == view {
			fg, pbg = cBG, cAccent
		}
		x = g.text(x, y, hi, p, fg, pbg, l == view, "boxlayer:"+itoa(l))
		x++
	}
}

// boxInk is a colour for words on bg that reads well: dark on a light
// wall, light on a dark one.
func boxInk(bg string) string {
	r, g, b := rgb(bg)
	if 0.299*float64(r)+0.587*float64(g)+0.114*float64(b) > 128 {
		return mixHex(bg, "#000000", 0.78)
	}
	return mixHex(bg, "#ffffff", 0.8)
}

// ---- drawing it ----------------------------------------------------------------------------

// boxLayers draws a box: its walls by its design, and the things in it, or
// its lid.
func (d *designState) boxLayers(o *dOct) []*lipgloss.Layer {
	x, y, w, h := d.rect(o)
	selected := d.sel == o.ID
	lk := boxLookOf(o)
	if w < 12 || h < 5 { // too small (zoomed far out): a block of its colour with its name
		return []*lipgloss.Layer{pillLayer(x, y, w, h, ansi.Truncate(kindIcons[kBox]+o.Header, w, ""), mixHex(cBG, lk.wall, 0.8))}
	}
	g := d.boxGridOf(o, w, h)
	c := newBoardCanvas(w, h)
	for cy, row := range g.c {
		for cx, bc := range row {
			stl := uv.Style{Fg: rgbaOf(bc.fg), Bg: rgbaOf(bc.bg)}
			if bc.bold {
				stl.Attrs = uv.AttrBold
			}
			c.Lines[cy][cx] = uv.Cell{Content: bc.r, Width: 1, Style: stl}
		}
	}
	l := lipgloss.NewLayer("").X(x).Y(y) // its cells are made already
	d.preCells(l, c)
	edge := lk.edge
	if selected {
		edge = mixHex(lk.edge, cText, 0.4)
	}
	return d.octExtras([]*lipgloss.Layer{l}, o, x, y, w, h, edge, mixHex(cBG, lk.wall, 0.2), selected, d.linkMode && d.linkFrom == o.ID)
}

// boxHit is what a click does at (cx, cy) from the corner of box o, drawn
// w × h: what boxGridOf drew there ("" for its walls: they move it).
func (d *designState) boxHit(o *dOct, cx, cy, w, h int) string {
	if w < 12 || h < 5 || cx < 0 || cy < 0 || cx >= w || cy >= h {
		return ""
	}
	return d.boxGridOf(o, w, h).c[cy][cx].act
}

// boxKey is what a box's drawing depends on beyond what every structure's
// does: where each thing is, the layer looked at and the chosen thing.
func (d *designState) boxKey(o *dOct) string {
	var sb strings.Builder
	for _, it := range o.Items {
		if it != nil {
			fmt.Fprintf(&sb, "%d,%d,%d;", it.X, it.Y, it.Slot)
		}
	}
	_, it := d.chosenBoxThing()
	fmt.Fprintf(&sb, "v%d c%d", d.boxView(o), itemIndex(o, it))
	return sb.String()
}

// boxMapShape is a box's outline on the mini map (as mapShape): a tin's
// round corners, a hat box's curve, a hexagon's slanted sides, the line
// under its rim.
func boxMapShape(o *dOct, s float64, dx, dy, bw, bh int, lines bool, row func(int) bool) int {
	if bw >= 4 && bh >= 4 {
		var cx, cy float64 // a corner cut cx half cells across and cy down
		switch boxDesign(o) {
		case boxTin:
			cx, cy = math.Max(1, 0.6*s), math.Max(1, 1.2*s)
		case boxHat:
			cx, cy = math.Max(1, 2*s), math.Max(1, 2*s)
		case boxHex:
			cx, cy = float64(bw)/5, float64(bh)/2
		}
		if cx > 0 && float64(dx)/cx+float64(dy)/cy < 1 {
			return 0
		}
	}
	if lines && dx > 0 && row(1) {
		return 2
	}
	return 1
}

// ---- choosing and changing ------------------------------------------------------------------

// boxSel is the chosen thing in a box: box's thing at.
type boxSel struct {
	box, at int
	active  bool
}

// chosenBoxThing is the chosen thing's box and the thing, if one is chosen
// (and its box is still the chosen structure).
func (d *designState) chosenBoxThing() (*dOct, *dOct) {
	if !d.boxPick.active {
		return nil, nil
	}
	b := d.byID(d.boxPick.box)
	if b == nil || b.Kind != kBox || d.sel != b.ID || d.boxPick.at < 0 || d.boxPick.at >= len(b.Items) || b.Items[d.boxPick.at] == nil {
		d.boxPick = boxSel{}
		return nil, nil
	}
	return b, b.Items[d.boxPick.at]
}

// setBoxView looks at layer v of box o (0: all of them).
func (d *designState) setBoxView(o *dOct, v int) {
	if d.boxLayer == nil {
		d.boxLayer = map[int]int{}
	}
	if v <= 0 {
		delete(d.boxLayer, o.ID)
	} else {
		d.boxLayer[o.ID] = min(v, boxLayers)
	}
	d.dirty = true
}

// sayBoxThing says the chosen thing, its layer, and what moves it.
func (m *Model) sayBoxThing() {
	b, it := m.ds().chosenBoxThing()
	if it == nil {
		return
	}
	m.say(kindIcons[it.Kind] + " " + itemTitle(it) + " in " + octName(b) + ", " + boxLayerNames[boxLayerOf(it)] +
		" layer · PgUp / PgDn: layer above / below · Alt+arrows: move it · drag it out")
}

// designBoxPick chooses thing at of box b (a click on it).
func (m *Model) designBoxPick(b *dOct, at int) {
	d := m.ds()
	if at < 0 || at >= len(b.Items) || b.Items[at] == nil {
		return
	}
	d.boxPick = boxSel{box: b.ID, at: at, active: true}
	d.sel, d.editing = b.ID, partNone
	d.dirty = true
	m.sayBoxThing()
}

// boxOrder is box b's things in the order Tab goes through them: the top
// layer first, then the middle, then the bottom.
func boxOrder(b *dOct) []int {
	var idx []int
	for l := range boxLayers {
		idx = append(idx, boxLayerThings(b, l)...)
	}
	return idx
}

// designBoxStep chooses box b's next thing (step 1) or the one before (-1).
func (m *Model) designBoxStep(b *dOct, step int) {
	d := m.ds()
	idx := boxOrder(b)
	if len(idx) == 0 {
		m.say(octName(b) + " is empty: drop something on it.")
		return
	}
	at := -1
	if _, it := d.chosenBoxThing(); it != nil {
		for i, j := range idx {
			if b.Items[j] == it {
				at = i
			}
		}
	}
	next := 0
	switch {
	case at < 0 && step < 0:
		next = len(idx) - 1
	case at >= 0:
		next = ((at+step)%len(idx) + len(idx)) % len(idx)
	}
	if !b.DrawerOpen {
		m.designBoxLid(b) // shut: it opens, to show what is chosen
	}
	m.designBoxPick(b, idx[next])
	if d.boxView(b) > 0 { // looking at one layer: the thing's
		d.setBoxView(b, boxLayerOf(b.Items[idx[next]])+1)
	}
}

// designBoxDesign gives box b its next design (step 1) or the one before.
func (m *Model) designBoxDesign(b *dOct, step int) {
	b.Layout = ((boxDesign(b)+step)%boxDesigns + boxDesigns) % boxDesigns
	for _, it := range b.Items { // a new outline: everything still inside it
		if it != nil {
			boxClamp(b, it)
		}
	}
	m.ds().dirty = true
	m.say(octName(b) + " is a " + boxDesignNames[boxDesign(b)] + " now (" + itoa(boxDesign(b)+1) + " of " + itoa(boxDesigns) +
		") · D: the next design · Shift+D: the one before")
}

// designBoxLid opens box b's lid, or shuts it.
func (m *Model) designBoxLid(b *dOct) {
	d := m.ds()
	b.DrawerOpen = !b.DrawerOpen
	d.startOpenAnim(b)
	d.dirty = true
	if b.DrawerOpen {
		d.sel = b.ID
		d.toFront(b)
		m.say(octName(b) + " open: click a thing to choose it · drag it out · PgUp / PgDn: look at one layer.")
		return
	}
	d.boxPick = boxSel{}
	m.say(octName(b) + " shut.")
}

// designBoxRow acts on a click on a box (an act from boxHit).
func (m *Model) designBoxRow(b *dOct, act string, sx, sy int) {
	d := m.ds()
	switch {
	case strings.HasPrefix(act, "boxthing:"):
		// A click chooses it; a drag takes it out (as a rack's row: designMotion, designRackOpen).
		var j int
		fmt.Sscanf(act, "boxthing:%d", &j)
		if j < 0 || j >= len(b.Items) || b.Items[j] == nil {
			return
		}
		d.boxPick = boxSel{box: b.ID, at: j, active: true}
		d.rackPress = &rackPress{rack: b.ID, at: j, sx: sx, sy: sy}
	case strings.HasPrefix(act, "boxlayer:"):
		var l int
		fmt.Sscanf(act, "boxlayer:%d", &l)
		if d.boxView(b) == l+1 {
			d.setBoxView(b, 0)
			m.say("All the layers of " + octName(b) + ".")
			return
		}
		d.setBoxView(b, l+1)
		m.sayBoxView(b)
	case act == "boxlid":
		m.designBoxLid(b)
	case act == "boxdesign":
		m.designBoxDesign(b, 1)
	case act == "boxfloor": // its empty floor: it lets go of the thing, and a drag moves the box
		d.boxPick = boxSel{}
		d.drag = dragMove
	}
}

// sayBoxView says which layer of b is looked at.
func (m *Model) sayBoxView(b *dOct) {
	v := m.ds().boxView(b)
	if v == 0 {
		m.say("All the layers of " + octName(b) + " · PgDn: look at the top layer.")
		return
	}
	msg := "The " + boxLayerNames[v-1] + " layer of " + octName(b)
	if v > 1 {
		msg += ": the layers above it are see-through"
	}
	m.say(msg + " · PgUp / PgDn: another layer · Esc: all of them.")
}

// designBoxKey handles a key with a box chosen. It says whether it did.
func (m *Model) designBoxKey(name string) (bool, tea.Cmd) {
	d := m.ds()
	b := d.byID(d.sel)
	if b == nil || b.Kind != kBox {
		return false, nil
	}
	_, it := d.chosenBoxThing()
	switch name {
	case "d", "D", "shift+d":
		m.designBoxDesign(b, map[bool]int{true: 1, false: -1}[name == "d"])
	case "tab", "shift+tab":
		m.designBoxStep(b, map[bool]int{true: -1, false: 1}[name == "shift+tab"])
	case "pgup", "pgdown":
		up := name == "pgup"
		if it == nil { // the layer looked at
			v := d.boxView(b)
			if up {
				v--
			} else {
				v++
			}
			d.setBoxView(b, min(max(v, 0), boxLayers))
			m.sayBoxView(b)
			break
		}
		l := boxLayerOf(it)
		switch {
		case up && l == 0:
			m.say(itemTitle(it) + " is on the top layer already.")
			return true, nil
		case !up && l == boxLayers-1:
			m.say(itemTitle(it) + " is on the bottom layer already.")
			return true, nil
		case up:
			it.Slot = l - 1
		default:
			it.Slot = l + 1
		}
		if d.boxView(b) > 0 { // looking at one layer: the thing's, so it stays in sight
			d.setBoxView(b, it.Slot+1)
		}
		d.dirty = true
		m.sayBoxThing()
	case "alt+left", "alt+right", "alt+up", "alt+down", "alt+shift+left", "alt+shift+right", "alt+shift+up", "alt+shift+down":
		if it == nil {
			return false, nil // the box itself moves
		}
		dx, dy, step := arrowStep(strings.TrimPrefix(name, "alt+"))
		it.X, it.Y = it.X+dx*step, it.Y+dy*step
		boxClamp(b, it)
		d.dirty = true
	case "enter":
		if it == nil {
			return false, nil
		}
		m.designBoxTakeOut(b)
	case "x", "X", "shift+x":
		if it == nil {
			return false, nil
		}
		if d.xAt.IsZero() || time.Since(d.xAt) > xTwice {
			d.xAt = time.Now()
			m.say("Press X again to take " + itemTitle(it) + " away (Ctrl+Z brings it back).")
			return true, nil
		}
		d.xAt = time.Time{}
		b.Items = append(b.Items[:d.boxPick.at:d.boxPick.at], b.Items[d.boxPick.at+1:]...)
		d.boxPick = boxSel{}
		d.dirty = true
		m.say("Took " + itemTitle(it) + " out of " + octName(b) + " and away. Ctrl+Z brings it back.")
	case "esc":
		switch {
		case it != nil:
			d.boxPick = boxSel{}
			m.say("Let go of " + itemTitle(it) + ".")
		case d.boxView(b) > 0:
			d.setBoxView(b, 0)
			m.say("All the layers of " + octName(b) + ".")
		default:
			return false, nil
		}
		d.dirty = true
	default:
		return false, nil
	}
	return true, nil
}

// designBoxTakeOut takes the chosen thing out of its box, beside it, chosen.
func (m *Model) designBoxTakeOut(b *dOct) {
	d := m.ds()
	_, it := d.chosenBoxThing()
	if it == nil {
		return
	}
	j := d.boxPick.at
	b.Items = append(b.Items[:j:j], b.Items[j+1:]...)
	d.boxPick = boxSel{}
	bw, _ := d.size(b)
	it.Slot, it.X, it.Y = 0, b.X+bw+3, b.Y
	d.octs = append(d.octs, it)
	d.clearSpot(it)
	d.sel, d.editing = it.ID, partNone
	d.dirty = true
	m.say(kindIcons[it.Kind] + " " + itemTitle(it) + " taken out of " + octName(b) + ".")
}

// boxDropAt is where in box to a thing being dragged would go if let go
// now: its miniature's place (board cells from the inside corner), with
// its middle under the mouse. The mouse is worked out from the drag (it
// moved the thing by as much, at this zoom, as the mouse moved); not in a
// drag, the thing's own place is used.
func (d *designState) boxDropAt(to, it *dOct) (int, int) {
	if d.drag != dragMove {
		return it.X - to.X - 1, it.Y - to.Y - 1
	}
	z := d.z()
	mx := d.pressX + int(math.Round(float64(it.X-d.origX)*z))
	my := d.pressY + int(math.Round(float64(it.Y-d.origY)*z))
	x, y, w, h := d.rect(to)
	ix, iy, _, _, kx, ky := boxPlaces(to, w, h)
	bx := (float64(mx-x-ix)+0.5)/kx - float64(miniW)/2
	by := (float64(my-y-iy)+0.5)/ky - float64(miniH)/2
	return int(math.Round(bx)), int(math.Round(by))
}

// boxPut puts thing it (just added to box to) at (bx, by), on the top
// layer, chosen; a shut box opens to show it.
func (m *Model) boxPut(to, it *dOct, bx, by int) {
	d := m.ds()
	it.Slot, it.X, it.Y = 0, bx, by
	boxClamp(to, it)
	if !to.DrawerOpen {
		to.DrawerOpen = true
		d.startOpenAnim(to)
	}
	d.sel = to.ID
	d.boxPick = boxSel{box: to.ID, at: itemIndex(to, it), active: true}
	m.say(kindIcons[it.Kind] + " " + itemTitle(it) + " put in " + octName(to) + ", on the top layer · PgDn: one layer down.")
}
