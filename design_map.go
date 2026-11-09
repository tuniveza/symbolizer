package main

// ---- the mini map ------------------------------------------------------------------
//
// AltGr+M shows a small map of the whole board in a corner of the screen,
// and moves it on round the corners (top right, bottom right, bottom left,
// top left), then hides it. The map is the board's own shape (wide board,
// wide map). Each structure is drawn in its own outline and colour (an
// octagon's cut corners, a note's round ones, a sheet's title line, a
// container's name line), with its symbol and title; the chosen one is
// picked out; links are dotted lines between them, and a frame shows what
// the screen shows now.

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

// paintMap draws the mini map onto the board's canvas. It is drawn in
// half cells (▀: a top and a bottom colour per cell), so each structure is
// a box of its own shape and colour; one big enough also shows its symbol
// and title. Links are lines, the screen a frame.
func (m *Model) paintMap(c *boardCanvas) {
	d := m.ds()
	if d.mapCorner == 0 {
		return
	}
	cw, ch := c.Width(), c.Height()
	maxIW, maxIH := min(60, cw*9/20)-2, min(22, ch*2/3)-2
	if maxIW < 10 || maxIH < 4 {
		return // the screen is too small for it
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
		ox, oy := o.X, o.Y
		if x, y, ok := d.openPlace(o); ok { // open inside a rack
			ox, oy = x, y
		}
		bx0, by0 = math.Min(bx0, float64(ox)), math.Min(by0, float64(oy))
		bx1, by1 = math.Max(bx1, float64(ox+w)), math.Max(by1, float64(oy+h))
	}
	pad := math.Max(bx1-bx0, by1-by0) * 0.03
	bx0, by0, bx1, by1 = bx0-pad, by0-pad, bx1+pad, by1+pad

	// Half cells: iw across, ih*2 down, each about square, so one scale
	// both ways keeps every shape (a board row is two half cells high).
	// The map is as big as fits, in the board's own shape: a wide board
	// makes a wide, low map, a tall one a narrow, high one.
	s := math.Min(float64(maxIW)/(bx1-bx0), float64(maxIH*2)/((by1-by0)*2))
	iw := min(maxIW, max(14, int(math.Ceil((bx1-bx0)*s))))
	ih := min(maxIH, max(4, int(math.Ceil((by1-by0)*s))))
	mw, mh := iw+2, ih+2
	x0, y0 := cw-mw-1, 1 // top right
	switch d.mapCorner {
	case 2:
		x0, y0 = cw-mw-1, ch-mh-1
	case 3:
		x0, y0 = 1, ch-mh-1
	case 4:
		x0, y0 = 1, 1
	}
	bgHex := mixHex(cBG, cBarBG, 0.6)
	bg := rgbaOf(bgHex)
	edge := mixHex(cDim, cBG, 0.2)
	put := func(x, y int, s, fg, back string, bold bool) {
		if x < 0 || y < 0 || x >= cw || y >= ch {
			return
		}
		st := uv.Style{Fg: rgbaOf(fg), Bg: bg}
		if back != "" {
			st.Bg = rgbaOf(back)
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
			put(x0+x, y0+y, s, edge, "", false)
		}
	}

	pw, ph := iw, ih*2
	offX := (float64(pw) - (bx1-bx0)*s) / 2
	offY := (float64(ph) - (by1-by0)*2*s) / 2
	px := func(wx float64) int { return int(math.Floor((wx-bx0)*s + offX)) }
	py := func(wy float64) int { return int(math.Floor((wy-by0)*2*s + offY)) }
	grid := make([][]string, ph)
	for i := range grid {
		grid[i] = make([]string, pw)
	}
	dot := func(x, y int, col string) {
		if x >= 0 && y >= 0 && x < pw && y < ph {
			grid[y][x] = col
		}
	}
	type box struct {
		o              *dOct
		x0, y0, x1, y1 int // in half cells, x1 and y1 included
		fill           string
	}
	var boxes []box
	for _, o := range d.octs {
		if o == nil {
			continue
		}
		w, h := d.size(o)
		ox, oy := o.X, o.Y
		if x, y, ok := d.openPlace(o); ok { // open inside a rack
			ox, oy = x, y
		}
		b := box{o: o, x0: px(float64(ox)), y0: py(float64(oy))}
		b.x1, b.y1 = max(b.x0, px(float64(ox+w))-1), max(b.y0, py(float64(oy+h))-1)
		b.fill = mixHex(o.Color, bgHex, 0.55)
		boxes = append(boxes, b)
	}

	// The structures, each in its own outline: filled with its colour,
	// a brighter rim (the chosen one's bright) and its inner lines. Each
	// half cell is looked at where it falls on the board, so the outline
	// is the structure's own at any scale.
	for _, b := range boxes {
		o := b.o
		w, h := d.size(o)
		rim := mixHex(o.Color, cText, 0.2)
		if o.ID == d.sel {
			rim = cAccent
		}
		line := mixHex(b.fill, rim, 0.6)
		bw, bh := b.x1-b.x0+1, b.y1-b.y0+1
		in := func(x, y int) int { // 0 out, 1 in, 2 on an inner line
			if x < b.x0 || x > b.x1 || y < b.y0 || y > b.y1 {
				return 0
			}
			return mapShape(o, w, h, s, x-b.x0, y-b.y0, bw, bh)
		}
		drawn := false
		for y := b.y0; y <= b.y1; y++ {
			for x := b.x0; x <= b.x1; x++ {
				k := in(x, y)
				if k == 0 {
					continue
				}
				drawn = true
				col := b.fill
				switch {
				case in(x-1, y) == 0 || in(x+1, y) == 0 || in(x, y-1) == 0 || in(x, y+1) == 0:
					col = rim
				case k == 2:
					col = line
				}
				dot(x, y, col)
			}
		}
		if !drawn { // a sliver between half cells: still a dot
			dot((b.x0+b.x1)/2, (b.y0+b.y1)/2, rim)
		}
	}

	// Half cells to cells: ▀ with the top colour in front, the bottom behind.
	// used marks the cells something is drawn in (structures, then labels).
	used := make([][]bool, ih)
	for cy := range used {
		used[cy] = make([]bool, pw)
	}
	for cy := 0; cy < ih; cy++ {
		for cx := 0; cx < pw; cx++ {
			top, bot := grid[cy*2][cx], grid[cy*2+1][cx]
			used[cy][cx] = top != "" || bot != ""
			switch {
			case top == "" && bot == "":
				put(x0+1+cx, y0+1+cy, " ", edge, "", false)
			case top == bot:
				put(x0+1+cx, y0+1+cy, " ", top, top, false)
			case top == "":
				put(x0+1+cx, y0+1+cy, "▄", bot, "", false)
			case bot == "":
				put(x0+1+cx, y0+1+cy, "▀", top, "", false)
			default:
				put(x0+1+cx, y0+1+cy, "▀", top, bot, false)
			}
		}
	}

	cell := func(x, y int, r string, fg, back string, bold bool) {
		if x >= 0 && x < pw && y >= 0 && y < ih {
			put(x0+1+x, y0+1+y, r, fg, back, bold)
		}
	}
	free := func(x, y int) bool { return x >= 0 && x < pw && y >= 0 && y < ih && !used[y][x] }

	// The screen: a thin frame, in the gaps between structures.
	view := mixHex(cAccent, cText, 0.3)
	fx0, fy0 := px(vx0), py(vy0)/2
	fx1, fy1 := max(fx0+1, px(vx1)-1), max(fy0+1, (py(vy1)-1)/2)
	for x := fx0; x <= fx1; x++ {
		for _, y := range []int{fy0, fy1} {
			if free(x, y) {
				g := "─"
				switch {
				case x == fx0 && y == fy0:
					g = "┌"
				case x == fx1 && y == fy0:
					g = "┐"
				case x == fx0:
					g = "└"
				case x == fx1:
					g = "┘"
				}
				cell(x, y, g, view, "", false)
			}
		}
	}
	for y := fy0 + 1; y < fy1; y++ {
		for _, x := range []int{fx0, fx1} {
			if free(x, y) {
				cell(x, y, "│", view, "", false)
			}
		}
	}

	// Links: thin dotted lines between the middles of what they join, in
	// the gaps between structures.
	link := mixHex(cDim, cText, 0.4)
	mid := func(b box) (int, int) { return (b.x0 + b.x1) / 2, (b.y0 + b.y1) / 4 }
	byID := map[int]box{}
	for _, b := range boxes {
		byID[b.o.ID] = b
	}
	for _, l := range d.links {
		a, okA := byID[l.A]
		b, okB := byID[l.B]
		if !okA || !okB {
			continue
		}
		ax, ay := mid(a)
		bx, by := mid(b)
		n := max(abs(bx-ax), abs(by-ay), 1)
		for i := 0; i <= n; i++ {
			if x, y := ax+(bx-ax)*i/n, ay+(by-ay)*i/n; free(x, y) {
				cell(x, y, "·", link, "", false)
			}
		}
	}

	// Labels: the symbol and title inside a box with room for them;
	// otherwise the symbol inside (or on it) and the title beside it,
	// right or left, where nothing else is drawn.
	for _, b := range boxes {
		w := b.x1 - b.x0 + 1
		cy := (b.y0 + b.y1) / 4 // the middle half cell, as a cell row
		icon := kindIcons[b.o.Kind]
		title := []rune(itemTitle(b.o))
		if len(title) > 16 {
			title = append(title[:15], '…')
		}
		fg := mixHex(cText, b.fill, 0.1)
		if b.o.ID == d.sel {
			fg = cAccent
		}
		bold := b.o.ID == d.sel
		inside := []rune(icon + " " + string(title))
		if len(title) > 0 && len(inside) <= w-2 {
			start := b.x0 + (w-len(inside))/2
			for i, r := range inside {
				cell(start+i, cy, string(r), fg, b.fill, bold)
			}
			continue
		}
		// The symbol on the box.
		back := b.fill
		if w < 3 {
			back = ""
		}
		cell(b.x0+(w-1)/2, cy, icon, fg, back, bold)
		if len(title) == 0 {
			continue
		}
		// The title beside it, where there is room.
		near := mixHex(b.o.Color, cText, 0.35)
		if bold {
			near = cAccent
		}
		fits := func(x int) bool {
			for i := -1; i <= len(title); i++ {
				if !free(x+i, cy) {
					return false
				}
			}
			return true
		}
		for _, x := range []int{b.x1 + 2, b.x0 - 1 - len(title)} {
			if fits(x) {
				for i, r := range title {
					cell(x+i, cy, string(r), near, "", bold)
					used[cy][x+i] = true
				}
				break
			}
		}
	}
	for i, r := range " map " {
		put(x0+2+i, y0, string(r), mixHex(cDim, cText, 0.4), "", false)
	}
}

// mapShape is whether half cell (i, j) of a structure's box on the map,
// bw × bh half cells big, is inside its outline (1), on one of its inner
// lines (2), or out (0). w and h are its size on the board, s the map's
// half cells to a board cell. The outline is worked out on the box itself,
// so it comes out the same on each side.
func mapShape(o *dOct, w, h int, s float64, i, j, bw, bh int) int {
	if bw <= 2 || bh <= 2 {
		return 1 // too small for a shape: all of it
	}
	dx, dy := min(i, bw-1-i), min(j, bh-1-j) // from the nearest side
	// cut is whether a corner cuts this half cell off: a diagonal c board
	// cells across and c rows down from each corner, at least a half cell.
	cut := func(c float64) bool {
		cx, cy := math.Max(1, c*s), math.Max(1, c*2*s)
		return float64(dx)/cx+float64(dy)/cy < 1
	}
	// row is whether this half cell is on board row r of it (from the
	// bottom when r < 0).
	row := func(r int) bool {
		if r < 0 {
			return bh-1-j == int((float64(-r)-0.5)*2*s)
		}
		return j == int((float64(r)+0.5)*2*s)
	}
	lines := bh >= 10
	switch o.Kind {
	case kOct:
		c := 2.5 // the big octagon's corners: two cells and two rows
		if h < octMinH || w < octMinW {
			c = 1.2
		}
		if bw >= 5 && bh >= 4 && cut(c) {
			return 0
		}
	case kNote:
		if bw >= 4 && bh >= 4 && cut(0.6) { // round corners
			return 0
		}
	case kCabinet, kDrawer, kRack, kBoard:
		if lines && dx > 0 && row(2) { // the line under its name
			return 2
		}
	default: // a sheet: its folded corner, the line under its title, over its footer
		if bw >= 4 && bh >= 4 && float64(bw-1-i)/math.Max(1, 1.3*s)+float64(j)/math.Max(1, 2.6*s) < 1 {
			return 0
		}
		if lines && dx > 0 && (row(2) || (h >= 7 && row(-3))) {
			return 2
		}
	}
	return 1
}
