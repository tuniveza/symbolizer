package main

// Codex frames: the 16 borders any thing on a page may have (B), the 16
// looks of a button (Y), and moving a thing with Alt + the arrows (the user
// asked, 17 Nov 2026).

import (
	"fmt"
	"image"
	"image/color"
	"math"

	uv "github.com/charmbracelet/ultraviolet"
)

// codexEdge is a frame's glyphs: corners and sides. A side of " " is left out.
type codexEdge struct {
	name                       string
	tl, t, tr, l, r, bl, b, br string
	shadow                     bool // a dark shadow below and to the right, one cell out
}

// codexBorders: the 16 borders. A thing's Border is 0 (none) or 1 … 16.
var codexBorders = [16]codexEdge{
	{"thin", "┌", "─", "┐", "│", "│", "└", "─", "┘", false},
	{"rounded", "╭", "─", "╮", "│", "│", "╰", "─", "╯", false},
	{"double", "╔", "═", "╗", "║", "║", "╚", "═", "╝", false},
	{"heavy", "┏", "━", "┓", "┃", "┃", "┗", "━", "┛", false},
	{"dashed", "┌", "╌", "┐", "╎", "╎", "└", "╌", "┘", false},
	{"heavy dashed", "┏", "╍", "┓", "╏", "╏", "┗", "╍", "┛", false},
	{"dotted", "╭", "┈", "╮", "┊", "┊", "╰", "┈", "╯", false},
	{"double top and bottom", "╒", "═", "╕", "│", "│", "╘", "═", "╛", false},
	{"double sides", "╓", "─", "╖", "║", "║", "╙", "─", "╜", false},
	{"solid line", "▛", "▀", "▜", "▌", "▐", "▙", "▄", "▟", false},
	{"thick block", "█", "█", "█", "█", "█", "█", "█", "█", false},
	{"shaded", "░", "░", "░", "░", "░", "░", "░", "░", false},
	{"stars", "✦", "·", "✦", "·", "·", "✦", "·", "✦", false},
	{"gilt corners", "◆", "─", "◆", "│", "│", "◆", "─", "◆", false},
	{"typewriter", "+", "-", "+", "|", "|", "+", "-", "+", false},
	{"drop shadow", "┌", "─", "┐", "│", "│", "└", "─", "┘", true},
}

// codexBorderOf is thing e's border (ok false: none).
func codexBorderOf(e *codexEl) (codexEdge, bool) {
	if e.Border < 1 || e.Border > len(codexBorders) {
		return codexEdge{}, false
	}
	return codexBorders[e.Border-1], true
}

// codexBorderName says a border number (0: none).
func codexBorderName(i int) string {
	if i < 1 || i > len(codexBorders) {
		return "none"
	}
	return codexBorders[i-1].name
}

// codexBorderPad is the room a border takes round a thing: on the left, top,
// right and bottom. Words keep a space between them and the border.
func codexBorderPad(e *codexEl) (l, t, r, b int) {
	f, ok := codexBorderOf(e)
	if !ok {
		return 0, 0, 0, 0
	}
	px := 1
	if e.Kind == ceImage || e.Kind == ceDrawing {
		px = 0
	}
	sh := 0
	if f.shadow {
		sh = 1
	}
	return 1 + px, 1, 1 + px + sh, 1 + sh
}

// codexDrawBorder draws frame f round the box r (x, y, w, h: the frame's own
// cells), in colour col, within the slot sx … sx+sw-1.
func (m *Model) codexDrawBorder(cv *boardCanvas, f codexEdge, r [4]int, col, paper string, l codexLook) {
	c := m.cx()
	x0, y0, x1, y1 := r[0], r[1], r[0]+r[2]-1, r[1]+r[3]-1
	if x1-x0 < 1 || y1-y0 < 1 {
		return
	}
	set := func(x, y int, g, fg string) {
		if g == " " || !c.insideInner(x, y) {
			return
		}
		under := cvBG(cv, x, y, paper)
		if l.flash > 0 {
			fg = mixHex(fg, "#ffffff", l.flash*0.8)
		}
		if l.ghost {
			fg = mixHex(fg, under, 0.6)
		}
		cvSet(cv, x, y, g, fg, under, 0)
	}
	if f.shadow { // first, so the frame sits over it
		dark := mixHex(paper, "#000000", 0.55)
		if lum(paper) < 0.25 {
			dark = mixHex(paper, "#000000", 0.75)
		}
		for y := y0 + 1; y <= y1+1; y++ {
			set(x1+1, y, "▒", dark)
		}
		for x := x0 + 1; x <= x1+1; x++ {
			set(x, y1+1, "▒", dark)
		}
	}
	gold := codexAtLeast(codexGold(), paper, 2.5)
	for x := x0 + 1; x < x1; x++ {
		set(x, y0, f.t, col)
		set(x, y1, f.b, col)
	}
	for y := y0 + 1; y < y1; y++ {
		set(x0, y, f.l, col)
		set(x1, y, f.r, col)
	}
	corner := col
	if f.tl == "◆" || f.tl == "✦" {
		corner = gold
	}
	set(x0, y0, f.tl, corner)
	set(x1, y0, f.tr, corner)
	set(x0, y1, f.bl, corner)
	set(x1, y1, f.br, corner)
}

// ---- the 16 looks of a button ---------------------------------------------------------------

// codexButtonStyle is one look of a button: its frame, how it is filled, and
// the colour of its frame.
type codexButtonStyle struct {
	codexEdge
	fill int // 0 none (the paper), 1 its colour, 2 its colour darkened
	edge int // 0 its colour darkened, 1 its colour, 2 gold, 3 lit from the top left
}

var codexButtonStyles = [16]codexButtonStyle{
	{codexEdge{name: "rounded", tl: "╭", t: "─", tr: "╮", l: "│", r: "│", bl: "╰", b: "─", br: "╯"}, 1, 0},
	{codexEdge{name: "square", tl: "┌", t: "─", tr: "┐", l: "│", r: "│", bl: "└", b: "─", br: "┘"}, 1, 0},
	{codexEdge{name: "double", tl: "╔", t: "═", tr: "╗", l: "║", r: "║", bl: "╚", b: "═", br: "╝"}, 1, 0},
	{codexEdge{name: "heavy", tl: "┏", t: "━", tr: "┓", l: "┃", r: "┃", bl: "┗", b: "━", br: "┛"}, 1, 0},
	{codexEdge{name: "soft block", tl: "▗", t: "▄", tr: "▖", l: "▐", r: "▌", bl: "▝", b: "▀", br: "▘"}, 1, 1},
	{codexEdge{name: "outline", tl: "╭", t: "─", tr: "╮", l: "│", r: "│", bl: "╰", b: "─", br: "╯"}, 0, 1},
	{codexEdge{name: "square outline", tl: "┌", t: "─", tr: "┐", l: "│", r: "│", bl: "└", b: "─", br: "┘"}, 0, 1},
	{codexEdge{name: "double outline", tl: "╔", t: "═", tr: "╗", l: "║", r: "║", bl: "╚", b: "═", br: "╝"}, 0, 1},
	{codexEdge{name: "raised", tl: "▗", t: "▄", tr: "▖", l: "▐", r: "▌", bl: "▝", b: "▀", br: "▘"}, 1, 3},
	{codexEdge{name: "corners", tl: "┌", t: " ", tr: "┐", l: " ", r: " ", bl: "└", b: " ", br: "┘"}, 0, 1},
	{codexEdge{name: "underlined", tl: " ", t: " ", tr: " ", l: " ", r: " ", bl: "━", b: "━", br: "━"}, 0, 1},
	{codexEdge{name: "glow", tl: "░", t: "░", tr: "░", l: "░", r: "░", bl: "░", b: "░", br: "░"}, 1, 1},
	{codexEdge{name: "ember", tl: "✦", t: "─", tr: "✦", l: "│", r: "│", bl: "✦", b: "─", br: "✦"}, 1, 2},
	{codexEdge{name: "ticket", tl: "┌", t: "┄", tr: "┐", l: "┆", r: "┆", bl: "└", b: "┄", br: "┘"}, 1, 0},
	{codexEdge{name: "dashed outline", tl: "┏", t: "╍", tr: "┓", l: "╏", r: "╏", bl: "┗", b: "╍", br: "┛"}, 0, 1},
	{codexEdge{name: "dark", tl: "┏", t: "━", tr: "┓", l: "┃", r: "┃", bl: "┗", b: "━", br: "┛"}, 2, 1},
}

func codexButtonStyleOf(e *codexEl) codexButtonStyle {
	return codexButtonStyles[(e.Style%16+16)%16]
}

// codexButtonCell is one cell of a button bw wide, in row ry of rows (3, or 1
// with no room for its frame): its glyph, colour, background ("" the paper),
// and whether it is left out.
func codexButtonCell(st codexButtonStyle, rows, ry, rx, bw int, col, paper string, focus bool) (g, fg, bg string, skip bool) {
	if rows == 1 {
		ry = 1
	}
	last := bw - 1
	switch {
	case ry == 0 && rx == 0:
		g = st.tl
	case ry == 0 && rx == last:
		g = st.tr
	case ry == 0:
		g = st.t
	case ry == 2 && rx == 0:
		g = st.bl
	case ry == 2 && rx == last:
		g = st.br
	case ry == 2:
		g = st.b
	case rx == 0:
		g = st.l
	case rx == last:
		g = st.r
	default: // inside
		switch st.fill {
		case 0:
			return " ", "", "", true
		case 2:
			return " ", "", codexButtonFill(st, col), false
		}
		return " ", "", col, false
	}
	if g == " " {
		return g, "", "", true
	}
	switch st.edge {
	case 0:
		fg = mixHex(col, "#000000", 0.3)
	case 1:
		fg = codexAtLeast(col, paper, 2)
	case 2:
		fg = codexAtLeast(codexGold(), paper, 2.5)
	case 3: // light on the top and left, shade on the bottom and right
		fg = mixHex(col, "#ffffff", 0.35)
		if ry == 2 || (rx == last && ry != 0) {
			fg = mixHex(col, "#000000", 0.45)
		}
	}
	if focus {
		fg = cAccent
	}
	return g, fg, "", false
}

// codexButtonFill is the colour inside a button of style st ("" the paper).
func codexButtonFill(st codexButtonStyle, col string) string {
	switch st.fill {
	case 0:
		return ""
	case 2:
		return mixHex(col, "#000000", 0.62)
	}
	return col
}

// codexButtonInk is the colour of a button's label.
func codexButtonInk(st codexButtonStyle, col, paper string) string {
	switch st.fill {
	case 0:
		return codexAtLeast(col, paper, 3)
	case 2:
		return codexAtLeast(col, codexButtonFill(st, col), 3)
	}
	return codexInkOn(col)
}

// ---- pictures: whole, or filling their box --------------------------------------------------

// pictureCells is how many cells picture path takes, shown whole in w×h.
func pictureCells(path string, w, h int) (int, int, bool) {
	picMu.Lock()
	img, ok := loadPictureLocked(path)
	picMu.Unlock()
	if !ok {
		return 0, 0, false
	}
	b := img.Bounds()
	iw, ih := b.Dx(), b.Dy()
	if iw <= 0 || ih <= 0 || w <= 0 || h <= 0 {
		return 0, 0, false
	}
	scale := min(float64(w)/float64(iw), float64(h*2)/float64(ih))
	tw := max(1, int(float64(iw)*scale+0.5))
	th := max(1, int(math.Ceil(float64(ih)*scale/2)))
	return min(tw, w), min(th, h), true
}

// pictureCanvasFilled is picture path cut to fill all of w×h (its middle kept).
func pictureCanvasFilled(path string, w, h int, fill string) *boardCanvas {
	picMu.Lock()
	img, ok := loadPictureLocked(path)
	picMu.Unlock()
	if !ok || w <= 0 || h <= 0 {
		return pictureCanvas(path, w, h, fill)
	}
	sub, can := img.(interface {
		SubImage(image.Rectangle) image.Image
	})
	b := img.Bounds()
	iw, ih := b.Dx(), b.Dy()
	if !can || iw <= 0 || ih <= 0 {
		return pictureCanvas(path, w, h, fill)
	}
	want := float64(w) / float64(h*2) // the box's shape, in square pixels
	r := b
	if float64(iw)/float64(ih) > want { // too wide: cut the sides
		cw := max(1, int(float64(ih)*want+0.5))
		r.Min.X = b.Min.X + (iw-cw)/2
		r.Max.X = r.Min.X + cw
	} else { // too tall: cut the top and bottom
		ch := max(1, int(float64(iw)/want+0.5))
		r.Min.Y = b.Min.Y + (ih-ch)/2
		r.Max.Y = r.Min.Y + ch
	}
	key := fmt.Sprintf("%s|%d|%d|%s|filled", path, w, h, fill)
	picMu.Lock()
	defer picMu.Unlock()
	if cv, ok := picCells[key]; ok {
		return cv
	}
	cv := newBoardCanvas(w, h)
	cell := uv.Cell{Content: "▀", Width: 1}
	forPictureCells(sub.SubImage(r), w, h, fill, func(col, row int, t, bo [3]int) {
		cell.Style.Fg = color.RGBA{uint8(t[0]), uint8(t[1]), uint8(t[2]), 255}
		cell.Style.Bg = color.RGBA{uint8(bo[0]), uint8(bo[1]), uint8(bo[2]), 255}
		cv.SetCell(col, row, &cell)
	})
	if picCellsKept += w * h; picCellsKept > 1_000_000 {
		clear(picCells)
		picCellsKept = w * h
	}
	picCells[key] = cv
	return cv
}

// ---- Alt + the arrows: moving a thing ---------------------------------------------------------

// codexAlignPos is where a thing sits across its slot: 0 left, 1 middle, 2 right.
func codexAlignPos(align int) int { return []int{1, 0, 2}[(align%3+3)%3] }

// codexAlignOfPos is the Align for a place across the slot (codexAlignPos).
func codexAlignOfPos(pos int) int { return []int{1, 0, 2}[min(max(pos, 0), 2)] }

// codexSlides is whether thing e moves across its slot when lined up
// differently (a drawing, or a picture filling its box, already fills it).
func codexSlides(e *codexEl) bool {
	switch e.Kind {
	case ceDrawing:
		return false
	case ceImage:
		return e.Whole
	}
	return true
}

// codexSlotToward is the nearest slot of layout li from slot s, that way
// ("left", "right", "up", "down"); -1: none.
func codexSlotToward(li, s int, dir string) int {
	slots := codexLayouts[(li%16+16)%16].slots
	if s < 0 || s >= len(slots) {
		return -1
	}
	cur := slots[s]
	cx, cy := cur[0]+cur[2]/2, cur[1]+cur[3]/2
	best, bestD := -1, math.Inf(1)
	for i, o := range slots {
		if i == s {
			continue
		}
		ox, oy := o[0]+o[2]/2, o[1]+o[3]/2
		var along, across float64
		var overlap bool
		switch dir {
		case "left", "right":
			along, across = cx-ox, math.Abs(oy-cy)
			if dir == "right" {
				along = ox - cx
			}
			overlap = o[1] < cur[1]+cur[3] && cur[1] < o[1]+o[3]
		default:
			along, across = cy-oy, math.Abs(ox-cx)
			if dir == "down" {
				along = oy - cy
			}
			overlap = o[0] < cur[0]+cur[2] && cur[0] < o[0]+o[2]
		}
		if along <= 0.001 {
			continue
		}
		d := along + 2*across
		if !overlap {
			d += 10
		}
		if d < bestD {
			best, bestD = i, d
		}
	}
	return best
}

// codexMove moves thing e one step that way: across its slot (left, middle,
// right) or up and down among the things in it, and from its edge into the
// next slot that way. It says where it went.
func (m *Model) codexMove(e *codexEl, dir string) {
	c := m.cx()
	p := c.pageOf()
	if p == nil {
		return
	}
	n := len(codexLayouts[(p.Layout%16+16)%16].slots)
	s := codexSlotOf(e, p.Layout)
	where := func() string {
		w := "slot " + itoa(codexSlotOf(e, p.Layout)+1) + " of " + itoa(n)
		if codexSlides(e) {
			w += ", " + []string{"on the left", "in the middle", "on the right"}[codexAlignPos(e.Align)]
		}
		return w
	}
	switch dir {
	case "left", "right":
		pos, step := codexAlignPos(e.Align), 1
		if dir == "left" {
			step = -1
		}
		if codexSlides(e) && pos+step >= 0 && pos+step <= 2 {
			m.codexStep()
			e.Align = codexAlignOfPos(pos + step)
			m.codexSave()
			m.say(codexElName(e) + ": " + where() + ".")
			return
		}
		to := codexSlotToward(p.Layout, s, dir)
		if to < 0 {
			m.say(codexElName(e) + " is as far " + dir + " as it goes (" + where() + "). L changes the layout.")
			return
		}
		m.codexStep()
		e.Slot = to
		if codexSlides(e) { // it comes in at the near side
			e.Align = codexAlignOfPos(map[string]int{"left": 2, "right": 0}[dir])
		}
		m.codexSave()
		m.say(codexElName(e) + ": " + where() + ".")
	case "up", "down":
		same := elsIn(p, s)
		at := -1
		for i, x := range same {
			if x == e {
				at = i
			}
		}
		if (dir == "up" && at > 0) || (dir == "down" && at >= 0 && at < len(same)-1) {
			m.codexStep()
			m.codexReorder(e, dir == "up")
			m.codexSave()
			m.say(codexElName(e) + " moved " + dir + " in its slot.")
			return
		}
		to := codexSlotToward(p.Layout, s, dir)
		if to < 0 {
			m.say(codexElName(e) + " is as far " + dir + " as it goes (" + where() + ").")
			return
		}
		m.codexStep()
		// Into the slot that way: at its bottom (going up) or its top (going down).
		var rest []*codexEl
		for _, x := range p.Els {
			if x != e {
				rest = append(rest, x)
			}
		}
		e.Slot = to
		ins := len(rest)
		if dir == "down" {
			for i, x := range rest {
				if codexSlotOf(x, p.Layout) == to {
					ins = i
					break
				}
			}
		} else {
			for i, x := range rest {
				if codexSlotOf(x, p.Layout) == to {
					ins = i + 1
				}
			}
		}
		p.Els = append(rest[:ins], append([]*codexEl{e}, rest[ins:]...)...)
		m.codexSave()
		m.say(codexElName(e) + ": " + where() + ".")
	}
}

// codexBorderFits is the room thing e's border takes in a box w×h: none when
// the box is too small for it.
func codexBorderFits(e *codexEl, w, h int) (l, t, r, b int) {
	l, t, r, b = codexBorderPad(e)
	if w-l-r < 3 || h-t-b < 1 {
		return 0, 0, 0, 0
	}
	return
}
