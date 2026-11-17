package main

// ---- Draw mode on the screen: the canvas, the tray, the start screen (draw.go) ---------------

import (
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

const drawTrayH = 3 // the tray of pens along the bottom of the canvas

// drawCanvasSize is the canvas's size in cells: the board's rows less the tray.
func (m *Model) drawCanvasSize() (int, int) { return m.width, max(m.boardH()-drawTrayH, 1) }

// drawPage is a note's page: its first and last-but-one column on the screen.
func (m *Model) drawPage() (int, int) {
	w := max(min(m.width-4, 104), 10)
	x0 := max((m.width-w)/2, 0)
	return x0, x0 + w
}

// drawOrigin is where the ink's cell (0, 0) is on the board's canvas (row
// 0 is the board's first row): a drawing's corner, or a note's first letter.
func (m *Model) drawOrigin() (int, int) {
	dw := m.dw()
	if dw.f == nil || dw.f.Note == "" {
		return 0, 0
	}
	px0, _ := m.drawPage()
	return padTextX(dw.style, px0), 1 - dw.scroll
}

// drawSheet is a blank drawing's paper.
func drawSheet() string { return mixHex(cBG, "#ffffff", 0.035) }

// drawRows fills the board's rows: the start screen, or the canvas and its tray.
func (m *Model) drawRows(sb *strings.Builder) {
	cv := newBoardCanvas(m.width, m.boardH())
	dw := m.dw()
	if dw.f == nil {
		m.drawStart(cv)
	} else {
		m.drawCanvas(cv)
		m.drawTray(cv)
	}
	cv.renderTo(sb)
}

// drawCanvas paints the paper (a note's text faintly on it), then the ink.
func (m *Model) drawCanvas(cv *boardCanvas) {
	dw := m.dw()
	w, h := m.drawCanvasSize()
	ox, oy := m.drawOrigin()
	if dw.f.Note == "" {
		cvFill(cv, 0, 0, w, h, drawSheet())
		if len(dw.f.Strokes) == 0 && !dw.drawing {
			dim := mixHex(drawSheet(), cDim, 0.7)
			cvMiddle(cv, 0, h/2-1, w, "a blank drawing", dim, drawSheet(), uv.AttrItalic)
			cvMiddle(cv, 0, h/2, w, "drag with the mouse to draw · 1–7 pick a pen · arrows and Space draw with the keys", dim, drawSheet(), 0)
		}
	} else {
		m.drawNotePaper(cv, w, h)
	}
	dw.glitterOn = drawInkOver(cv, dw.f, ox, oy, 0, 0, w, h, dw.twinkle)
	if dw.brushShown { // the keyboard's brush
		bx, by := ox+floorDiv(dw.bx, 2), oy+floorDiv(dw.by, 4)
		g, col := "◎", dw.pen.ink(dw.pen.Dev)
		if dw.penDown {
			g = "●"
		}
		if dw.pen.Rubber {
			g, col = "▫", "#d0d0d0"
			if dw.penDown {
				g = "▪"
			}
		}
		if by >= 0 && by < h {
			cvSet(cv, bx, by, g, col, cvBG(cv, bx, by, cBG), uv.AttrBold)
		}
	}
}

// drawNotePaper paints a note's page: its notepad's paper and ruling, and
// its text faintly, so the ink stands out over it.
func (m *Model) drawNotePaper(cv *boardCanvas, w, h int) {
	dw := m.dw()
	s := dw.style
	desk := codexDesk()
	cvFill(cv, 0, 0, w, h, desk)
	px0, px1 := m.drawPage()
	margin := 3
	for y := 0; y < h; y++ {
		for x := px0; x < px1 && x < w; x++ {
			fg := mixHex(s.Paper, s.Rule, 0.55)
			if s.Ruling == ruleMargin && x-px0 == margin {
				fg = mixHex(s.Paper, "#c0392b", 0.6)
			}
			cvSet(cv, x, y, ruleAt(s.Ruling, x-px0, y+dw.scroll, margin), fg, s.Paper, 0)
		}
		if px1 < w { // the page's shadow
			cvSet(cv, px1, y, " ", mixHex(desk, "#000000", 0.4), mixHex(desk, "#000000", 0.4), 0)
		}
	}
	ox, oy := m.drawOrigin()
	faint := mixHex(s.Paper, s.Ink, 0.5)
	for row, line := range dw.text {
		y := oy + row
		if y < 0 {
			continue
		}
		if y >= h {
			break
		}
		x := ox
		for _, r := range line {
			g := string(r)
			cw := max(codexWidth(g), 1)
			if x+cw > px1-1 {
				break
			}
			if r != ' ' {
				cvSet(cv, x, y, g, faint, s.Paper, 0)
			}
			x += cw
		}
	}
	if len(dw.text) == 0 || (len(dw.text) == 1 && dw.text[0] == "") {
		cvMiddle(cv, px0, h/2, px1-px0, "this note is empty: draw on its page", mixHex(s.Paper, s.Ink, 0.4), s.Paper, uv.AttrItalic)
	}
}

// ---- ink on cells --------------------------------------------------------------------------

// brailleBits is the braille dot for dot (dx, dy) of a cell.
var brailleBits = [2][4]uint8{{0x01, 0x02, 0x04, 0x40}, {0x08, 0x10, 0x20, 0x80}}

// rulingGlyphs are a page's printed lines: ink treats them as empty paper.
var rulingGlyphs = map[string]bool{"▁": true, "│": true, "┼": true, "─": true, "·": true}

// drawInkOver lays drawing f's ink over the cells of cv, its cell (0, 0)
// at (ox, oy), only inside the rectangle x0, y0, w, h. It says whether
// glitter showed. Draw mode and Design's notepad viewer both use it.
func drawInkOver(cv *boardCanvas, f *drawFile, ox, oy, x0, y0, w, h, twinkle int) (glitter bool) {
	if f == nil {
		return false
	}
	for at, ci := range f.inked() {
		x, y := ox+at[0], oy+at[1]
		if x < max(x0, 0) || y < max(y0, 0) || x >= min(x0+w, cv.Width()) || y >= min(y0+h, cv.Height()) {
			continue
		}
		cell := &cv.Lines[y][x]
		if cell.Width == 0 {
			continue
		}
		if drawLay(cell, ci, f, at[0], at[1], twinkle) {
			glitter = true
		}
	}
	return glitter
}

// drawLay lays one cell's ink over canvas cell c: each device in it, the
// oldest first, so a pen's line drawn over a marker shows on the marker.
// It says whether there is glitter in it.
func drawLay(c *uv.Cell, ci *drawCellInk, f *drawFile, cx, cy, twinkle int) bool {
	type layer struct {
		dev int8
		top int32 // its newest stroke in this cell
	}
	var layers []layer
	for _, id := range ci {
		if id == 0 || int(id) > len(f.Strokes) {
			continue
		}
		dev, found := f.Strokes[id-1].Dev, false
		for k := range layers {
			if layers[k].dev == dev {
				layers[k].top, found = max(layers[k].top, id), true
			}
		}
		if !found {
			layers = append(layers, layer{dev, id})
		}
	}
	sort.Slice(layers, func(i, j int) bool { return layers[i].top < layers[j].top })
	glitter := false
	for _, l := range layers {
		drawLayDev(c, ci, f, l.dev, &f.Strokes[l.top-1], cx, cy, twinkle)
		glitter = glitter || int(l.dev) == devGlitter
	}
	return glitter
}

// drawLayDev lays the dots of device dev in one cell over canvas cell c,
// in that device's look (Design's colours, design_desk.go); s is its
// newest stroke there. Lines (pen, fineliner, pencil) are braille dots on
// empty paper; a marker or highlighter that only covers half a cell is a
// half block. Over letters the letters stay readable.
func drawLayDev(c *uv.Cell, ci *drawCellInk, f *drawFile, dev int8, s *drawStroke, cx, cy, twinkle int) {
	var bits uint8
	var half [2]bool // the top half, the bottom half
	for i, id := range ci {
		if id == 0 || int(id) > len(f.Strokes) || f.Strokes[id-1].Dev != dev {
			continue
		}
		dx, dy := i%2, i/2
		half[dy/2] = true
		if dev == devPencil && inkHash(cx*2+dx, cy*4+dy)%4 == 0 {
			continue // a pencil's grain
		}
		bits |= brailleBits[dx][dy]
	}
	if bits == 0 {
		bits = 0x02 // a pencil's grain took all its dots: one is left
	}
	void := strings.TrimSpace(c.Content) == "" || rulingGlyphs[c.Content] || isBrailleCell(c.Content)
	mk := dMark{Dev: dev, Ink: s.Ink, Over: int(dev) != devHighlighter}
	bg, fg := colorHex(c.Style.Bg, cBG), colorHex(c.Style.Fg, cText)
	put := func(g, nfg, nbg string) {
		if g != "" {
			c.Content, c.Width = g, 1
		}
		c.Style.Fg, c.Style.Bg = rgbaOf(nfg), rgbaOf(nbg)
	}
	halfBlock := func(col string) {
		if half[0] {
			put("▀", col, bg)
		} else {
			put("▄", col, bg)
		}
	}
	switch {
	case int(dev) == devHighlighter:
		nbg, nfg, _ := highlightColours(mk, bg, fg, void)
		if void && half[0] != half[1] {
			halfBlock(nbg)
		} else {
			put("", nfg, nbg)
		}
	case lineDev(dev):
		nbg, nfg := inkColours(mk, bg, fg, void)
		if void {
			put(string(rune(0x2800+int(bits))), nfg, nbg)
		} else {
			put("", nfg, nbg)
		}
	case int(dev) == devMarker:
		nbg, nfg := inkColours(mk, bg, fg, void)
		switch {
		case !void:
			put("", nfg, nbg)
		case half[0] && half[1]:
			put(" ", nfg, nbg)
		default:
			halfBlock(nbg)
		}
	default: // brush, glitter: Design's own letters
		nbg, nfg := inkColours(mk, bg, fg, void)
		g := ""
		if void {
			g = inkGlyphFor(mk, cx, cy, twinkle, nil)
			if g == "" {
				g = " "
			}
			if int(dev) == devGlitter && lum(nbg) > 0.55 {
				nfg = mixHex(s.Ink, "#000000", 0.35) // on light paper white sparkles would not show
			}
		}
		put(g, nfg, nbg)
	}
}

// isBrailleCell: is s a braille letter (a line's dots, drawn over again)?
func isBrailleCell(s string) bool {
	r := []rune(s)
	return len(r) == 1 && r[0] >= 0x2800 && r[0] <= 0x28ff
}

// drawPreview draws drawing f shrunk into pw × ph cells at (x, y): two
// dots a cell, up and down, as half blocks.
func drawPreview(cv *boardCanvas, f *drawFile, x, y, pw, ph int, paper string) {
	cvFill(cv, x, y, pw, ph, paper)
	W, H := max(f.W, 1)*2, max(f.H, 1)*4
	ids := make([]int32, pw*ph*2)
	for at, ci := range f.inked() {
		for i, id := range ci {
			if id == 0 {
				continue
			}
			dx, dy := at[0]*2+i%2, at[1]*4+i/2
			px, py := dx*pw/W, dy*2*ph/H
			if px < 0 || py < 0 || px >= pw || py >= 2*ph {
				continue
			}
			if k := py*pw + px; id > ids[k] {
				ids[k] = id
			}
		}
	}
	col := func(id int32) string {
		if id == 0 || int(id) > len(f.Strokes) {
			return ""
		}
		s := f.Strokes[id-1]
		if int(s.Dev) == devHighlighter {
			return mixHex(paper, s.Ink, 0.6)
		}
		return s.Ink
	}
	for ry := 0; ry < ph; ry++ {
		for rx := 0; rx < pw; rx++ {
			t, b := col(ids[2*ry*pw+rx]), col(ids[(2*ry+1)*pw+rx])
			switch {
			case t == "" && b == "":
			case t == "":
				cvSet(cv, x+rx, y+ry, "▄", b, paper, 0)
			case b == "":
				cvSet(cv, x+rx, y+ry, "▀", t, paper, 0)
			default:
				cvSet(cv, x+rx, y+ry, "▀", t, b, 0)
			}
		}
	}
}

// ---- the tray ------------------------------------------------------------------------------

// drawTraySlot is how wide each device's place in the tray is, and where the first starts.
func (m *Model) drawTraySlot() (slot, x0 int) {
	slot = min(max((m.width-46)/len(trayDevs), 9), 18)
	return slot, 2
}

// drawTrayAt is the tray's device under screen column x (-1: none; -2: the rubber).
func (m *Model) drawTrayAt(x int) int {
	slot, x0 := m.drawTraySlot()
	if x >= x0 && x < x0+slot*len(trayDevs) {
		return trayDevs[(x-x0)/slot]
	}
	if rx := x0 + slot*len(trayDevs); x >= rx && x < rx+slot {
		return -2
	}
	return -1
}

// drawTray paints the tray along the bottom: the seven devices in their
// colours (the one in hand lifted), the rubber, and the pen's colour and size.
func (m *Model) drawTray(cv *boardCanvas) {
	dw := m.dw()
	_, ch := m.drawCanvasSize()
	y := ch
	tray := mixHex(cBG, "#000000", 0.4)
	lip := mixHex(tray, cEmber, 0.25)
	for x := 0; x < cv.Width(); x++ {
		cvSet(cv, x, y, "▄", tray, cvBG(cv, x, y-1, cBG), 0)
		cvSet(cv, x, y+1, " ", tray, tray, 0)
		cvSet(cv, x, y+2, "▔", lip, tray, 0)
	}
	slot, x0 := m.drawTraySlot()
	penLen := max(slot-5, 4)
	for i, dev := range trayDevs {
		x := x0 + i*slot
		chosen := dev == dw.pen.Dev && !dw.pen.Rubber
		bg := tray
		if chosen {
			bg = mixHex(tray, cAccent, 0.3)
			for xx := x; xx < x+slot-1; xx++ {
				cvSet(cv, xx, y+1, " ", bg, bg, 0)
				cvSet(cv, xx, y+2, " ", bg, bg, 0)
			}
		}
		numFG := cDim
		if chosen {
			numFG = cGlow
		}
		cvText(cv, x+1, y+1, itoa(i+1), numFG, bg, uv.AttrBold)
		for k, c := range penAcross(dev, dw.pen.ink(dev), penLen) {
			cvSet(cv, x+3+k, y+1, c.r, c.fg, bg, 0)
		}
		name, nameFG, attrs := devNames[dev], mixHex(cDim, cText, 0.3), uint8(0)
		if chosen {
			nameFG, attrs = cGlow, uv.AttrBold
		}
		cvMiddle(cv, x, y+2, slot-1, name, nameFG, bg, attrs)
	}
	// the rubber
	x := x0 + len(trayDevs)*slot
	bg := tray
	if dw.pen.Rubber {
		bg = mixHex(tray, cAccent, 0.3)
		cvFill(cv, x, y+1, slot-1, 2, bg)
	}
	cvText(cv, x+1, y+1, "E", map[bool]string{true: cGlow, false: cDim}[dw.pen.Rubber], bg, uv.AttrBold)
	cvText(cv, x+3, y+1, "▕▒▒▒▏", "#d8c8c0", bg, 0)
	cvMiddle(cv, x, y+2, slot-1, "rubber", map[bool]string{true: cGlow, false: mixHex(cDim, cText, 0.3)}[dw.pen.Rubber], bg, 0)
	// the pen's colour and size
	x += slot + 1
	ink := dw.pen.ink(dw.pen.Dev)
	cvText(cv, cvText(cv, x, y+1, "C ", cDim, tray, uv.AttrBold), y+1, "● "+inkName(ink), ink, tray, uv.AttrBold)
	size := strings.Repeat("●", dw.pen.Size) + strings.Repeat("○", drawMaxSize-dw.pen.Size)
	cvText(cv, cvText(cv, x, y+2, "[ ] ", cDim, tray, uv.AttrBold), y+2, size, mixHex(cDim, cEmber, 0.6), tray, 0)
}

// ---- the start screen ------------------------------------------------------------------------

const (
	drawPrevW, drawPrevH = 16, 4 // a drawing's preview, in cells
	drawCardH            = 5     // a drawing's row on the start screen
)

// drawStartCols are the two columns: the drawings' left and width, the notes' left and width.
func (m *Model) drawStartCols() (lx, lw, rx, rw int) {
	lw = max(min(m.width*2/5, 64), 30)
	return 2, lw, 2 + lw + 3, max(m.width-lw-7, 10)
}

// drawStartHit is a row on the start screen, for the mouse.
type drawStartHit struct {
	y0, y1, x0, x1, col, row int
}

var drawHits []drawStartHit // kept by the last drawing of the start screen

// drawStart paints the start screen: "New drawing" and the drawings,
// newest first, each with a preview; beside them the notes by notepad,
// oldest to newest as in the notepads.
func (m *Model) drawStart(cv *boardCanvas) {
	dw := m.dw()
	w, h := cv.Width(), cv.Height()
	desk := cBG
	cvFill(cv, 0, 0, w, h, desk)
	drawHits = drawHits[:0]
	lx, lw, rx, rw := m.drawStartCols()
	head := func(x, wd int, s string, on bool) {
		fg := mixHex(cDim, cText, 0.3)
		if on {
			fg = cEmber
		}
		cvText(cv, x, 0, s, fg, desk, uv.AttrBold)
		for xx := x + codexWidth(s) + 1; xx < x+wd; xx++ {
			cvSet(cv, xx, 0, "─", mixHex(desk, fg, 0.4), desk, 0)
		}
	}
	head(lx, lw, "DRAWINGS", dw.col == 0)
	head(rx, rw, "NOTES  ·  the ink is a layer over a note: its file is never changed", dw.col == 1)

	// The drawings column: row 0 is "New drawing" (3 rows), then a card each.
	rowY := func(i int) int { // where row i starts, from the column's top
		if i == 0 {
			return 0
		}
		return 3 + (i-1)*drawCardH
	}
	rowH := func(i int) int {
		if i == 0 {
			return 3
		}
		return drawCardH
	}
	room := h - 2
	n := len(dw.drawings) + 1
	if s := dw.sel[0]; rowY(s)-rowY(dw.top[0]) < 0 {
		dw.top[0] = s
	} else {
		for dw.top[0] < s && rowY(s)+rowH(s)-rowY(dw.top[0]) > room {
			dw.top[0]++
		}
	}
	for i := dw.top[0]; i < n; i++ {
		y := 2 + rowY(i) - rowY(dw.top[0])
		if y+rowH(i)-1 > h {
			break
		}
		chosen := dw.col == 0 && dw.sel[0] == i
		bg := desk
		if chosen {
			bg = mixHex(desk, cAccent, 0.22)
		}
		cvFill(cv, lx, y, lw, rowH(i)-1, bg)
		drawHits = append(drawHits, drawStartHit{y, y + rowH(i) - 1, lx, lx + lw, 0, i})
		if chosen {
			cvSet(cv, lx, y, "▸", cGlow, bg, uv.AttrBold)
		}
		if i == 0 {
			x := cvText(cv, lx+2, y, "＋  New drawing", map[bool]string{true: cGlow, false: cEmber}[chosen], bg, uv.AttrBold)
			cvText(cv, x+2, y, "N", cDim, bg, 0)
			cvText(cv, lx+6, y+1, "a blank sheet, with the seven pens", mixHex(cDim, cText, 0.2), bg, uv.AttrItalic)
			continue
		}
		e := dw.drawings[i-1]
		drawPreview(cv, e.f, lx+2, y, drawPrevW, drawPrevH, drawSheet())
		tx := lx + 2 + drawPrevW + 2
		cvText(cv, tx, y, codexClip(e.f.Name, lx+lw-tx-1), map[bool]string{true: cGlow, false: cText}[chosen], bg, uv.AttrBold)
		cvText(cv, tx, y+1, "changed "+e.f.Changed.Local().Format("Mon 2 Jan 15:04"), cDim, bg, 0)
		cvText(cv, tx, y+2, plural(len(e.f.Strokes), "stroke"), cDim, bg, 0)
	}
	if len(dw.drawings) == 0 && h > 8 {
		cvText(cv, lx+2, 6, "No drawings yet. N makes one.", cDim, desk, uv.AttrItalic)
	}

	// The notes column.
	if len(dw.notes) == 0 {
		cvText(cv, rx, 2, "No notes yet. Every .txt saved with a name in Text mode is a note.", cDim, desk, uv.AttrItalic)
		return
	}
	nroom := h - 2
	if dw.sel[1] < dw.top[1] {
		dw.top[1] = dw.sel[1]
		if dw.top[1] > 0 && dw.notes[dw.top[1]-1].note == nil {
			dw.top[1]-- // its notepad's heading shows too
		}
	}
	if dw.sel[1] >= dw.top[1]+nroom {
		dw.top[1] = dw.sel[1] - nroom + 1
	}
	for row := 0; row < nroom && dw.top[1]+row < len(dw.notes); row++ {
		i := dw.top[1] + row
		r := dw.notes[i]
		y := 2 + row
		s := notepadStyle(r.pad)
		if r.note == nil {
			cvText(cv, rx, y, "▌", s.Cover, desk, 0)
			x := cvText(cv, rx+2, y, "Notepad "+itoa(r.pad), mixHex(s.Cover, "#ffffff", 0.35), desk, uv.AttrBold)
			for xx := x + 1; xx < rx+rw; xx++ {
				cvSet(cv, xx, y, "─", mixHex(desk, s.Cover, 0.5), desk, 0)
			}
			continue
		}
		chosen := dw.col == 1 && dw.sel[1] == i
		bg := desk
		if chosen {
			bg = mixHex(desk, cAccent, 0.22)
			cvFill(cv, rx, y, rw, 1, bg)
			cvSet(cv, rx, y, "▸", cGlow, bg, uv.AttrBold)
		}
		drawHits = append(drawHits, drawStartHit{y, y + 1, rx, rx + rw, 1, i})
		cvText(cv, rx+1, y, "▌", s.Cover, bg, 0)
		x := rx + 3
		name := filepath.Base(r.note.Path)
		x = cvText(cv, x, y, codexClip(name, 28), map[bool]string{true: cGlow, false: cText}[chosen], bg, uv.AttrBold) + 2
		end := rx + rw
		if noteHasInk(r.note.Path) {
			end -= 7
			cvText(cv, end+1, y, "✎ ink", cEmber, bg, uv.AttrBold)
		}
		words := firstWords(noteHead(r.note.Path, 6))
		if !fileExists(r.note.Path) {
			words = "missing"
		}
		if end-x > 3 {
			wfg := cDim
			if chosen {
				wfg = mixHex(cText, bg, 0.25) // readable on the chosen row's colour
			}
			cvText(cv, x, y, codexClip(words, end-x-1), wfg, bg, 0)
		}
	}
}

// ---- the bars ------------------------------------------------------------------------------

func (m *Model) drawTopBar() string {
	dw := m.dw()
	bar := st(cStatusText, cBarBG)
	left := " " + modes[drawMode].Icon() + " Draw"
	right := plural(len(dw.drawings), "drawing") + " · " + plural(countNotes(dw.notes), "note") + " to draw on"
	if f := dw.f; f != nil {
		right = "“" + f.Name + "” · a drawing"
		if f.Note != "" {
			right = "“" + filepath.Base(f.Note) + "” · ink over the note (its file is never changed)"
		}
	}
	return centreBar(bar.Bold(true).Render(left)+bar.Render("  ·  "+right), m.width, bar)
}

func countNotes(rows []drawNoteRow) int {
	n := 0
	for _, r := range rows {
		if r.note != nil {
			n++
		}
	}
	return n
}

// drawStrip is the row under the board: the pen in hand and what is kept.
func (m *Model) drawStrip() string {
	dw := m.dw()
	bar := st(cStatusText, cBarBG)
	if dw.f == nil {
		return centreBar(bar.Render("drawings: newest first  ·  notes: by notepad, oldest first  ·  ← → the other column"), m.width, bar)
	}
	what := inkName(dw.pen.ink(dw.pen.Dev)) + " " + devNames[dw.pen.Dev]
	if dw.pen.Rubber {
		what = "the rubber"
	}
	kept := "saved"
	switch {
	case dw.err != "":
		kept = "✗ not saved: " + dw.err
	case len(dw.f.Strokes) == 0:
		kept = "nothing drawn yet"
	}
	return centreBar(bar.Render("in hand: "+what+" · size "+itoa(dw.pen.Size)+"  ·  "+plural(len(dw.f.Strokes), "stroke")+"  ·  "+kept), m.width, bar)
}

// drawStatus is the hint bar: the keys.
func (m *Model) drawStatus() string {
	dw := m.dw()
	var hint string
	switch {
	case dw.f == nil:
		hint = "↑↓ choose · ←→ drawings / notes · Enter open · N new drawing · X X delete a drawing · F1 all keys"
	case dw.penDown:
		hint = "Pen down · arrows draw (Shift: a dot) · Space lifts the pen · Esc lifts it"
	default:
		hint = "drag draws · 1–7 pen · C ⇧C colour · [ ] size · E rubber · arrows + Space draw by keys · Ctrl+Z / Ctrl+Y · Esc done · F1"
	}
	return centreBar(stStatus.Render(hint), m.width, stStatus)
}

// ---- glitter twinkles ------------------------------------------------------------------------

type drawTickMsg struct{}

// drawGlitterCmd asks for the next twinkle while glitter ink shows.
func (m *Model) drawGlitterCmd() tea.Cmd {
	dw := m.draw
	if dw == nil || !m.inDraw() || m.help || dw.f == nil || dw.tick || !dw.glitterOn {
		return nil
	}
	dw.tick = true
	return tea.Tick(glitterEvery, func(time.Time) tea.Msg { return drawTickMsg{} })
}

func (m *Model) drawTicked() {
	if dw := m.draw; dw != nil {
		dw.tick = false
		dw.twinkle++
	}
}
