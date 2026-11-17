package main

// ---- drawing a codex: its cover, its pages, the library ----------------------------------

import (
	"path/filepath"
	"strings"
	"time"

	uv "github.com/charmbracelet/ultraviolet"
)

const codexTurnFor = 300 * time.Millisecond // a page turning

// ---- the theme's colours (Emberistal unless another sy theme is on) ----

func codexDesk() string        { return mixHex(cBG, "#000000", 0.35) }
func codexThemePaper() string  { return mixHex(cBG, cCurrentLine, 0.55) }
func codexThemeCover() string  { return mixHex(cBG, cEmber, 0.16) }
func codexThemeButton() string { return cEmber }
func codexGold() string        { return cAccent }

// codexSymColor is a symbol's own colour: the theme's ember gradient.
func codexSymColor(i int) string {
	return cGradient[(i%3+3)%3]
}

// codexMarquee is the animation whose cells wrap round inside the thing.
const codexMarquee = 6

// cvSet puts letter g at (x, y) of cv, fg on bg.
func cvSet(cv *boardCanvas, x, y int, g, fg, bg string, attrs uint8) {
	if x < 0 || y < 0 || x >= cv.Width() || y >= cv.Height() {
		return
	}
	w := codexWidth(g)
	if w < 1 {
		w = 1
	}
	cv.Lines[y][x] = uv.Cell{Content: g, Width: w, Style: uv.Style{Fg: rgbaOf(fg), Bg: rgbaOf(bg), Attrs: attrs}}
	if w == 2 && x+1 < cv.Width() {
		cv.Lines[y][x+1] = uv.Cell{} // the right half of a wide letter
	}
}

// cvBG is the background colour at (x, y) of cv (or, off it, or).
func cvBG(cv *boardCanvas, x, y int, or string) string {
	if x < 0 || y < 0 || x >= cv.Width() || y >= cv.Height() {
		return or
	}
	return colorHex(cv.Lines[y][x].Style.Bg, or)
}

func cvFill(cv *boardCanvas, x, y, w, h int, bg string) {
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			cvSet(cv, xx, yy, " ", bg, bg, 0)
		}
	}
}

func cvText(cv *boardCanvas, x, y int, s, fg, bg string, attrs uint8) int {
	for _, r := range s {
		g := string(r)
		cvSet(cv, x, y, g, fg, bg, attrs)
		x += max(codexWidth(g), 1)
	}
	return x
}

// cvMiddle writes s in the middle of the row from x to x+w.
func cvMiddle(cv *boardCanvas, x, y, w int, s, fg, bg string, attrs uint8) {
	s = codexClip(s, w)
	cvText(cv, x+(w-codexWidth(s))/2, y, s, fg, bg, attrs)
}

// codexClip cuts s to w cells.
func codexClip(s string, w int) string {
	if codexWidth(s) <= w {
		return s
	}
	var sb strings.Builder
	n := 0
	for _, r := range s {
		rw := max(codexWidth(string(r)), 1)
		if n+rw > w-1 {
			break
		}
		sb.WriteRune(r)
		n += rw
	}
	return sb.String() + "…"
}

// lum is how light a colour is, 0 … 1.
func lum(hex string) float64 {
	r, g, b := rgb(hex)
	return (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)) / 255
}

// codexInkOn is a colour that reads on bg.
func codexInkOn(bg string) string {
	if lum(bg) > 0.55 {
		return "#2b1608"
	}
	return cStatusText
}

func (c *codexState) paper(p *codexPage) string {
	if p != nil && p.Paper != "" {
		return p.Paper
	}
	if c.cf != nil && c.cf.Paper != "" {
		return c.cf.Paper
	}
	return codexThemePaper()
}

// codexPageSize is the page's size on a w×h screen: wide (about 16:10) or upright.
func codexPageSize(w, h int, tall bool) (int, int) {
	r := 3.2 // cells are twice as tall as wide
	if tall {
		r = 1.45
	}
	aw, ah := max(w-8, 20), max(h-2, 6)
	pw := min(aw, int(float64(ah)*r))
	ph := int(float64(pw) / r)
	return max(pw, 20), max(min(ph, ah), 6)
}

// ---- the whole view -----------------------------------------------------------------------

// codexRows writes the board rows of Codex mode.
func (m *Model) codexRows(sb *strings.Builder) {
	c := m.cx()
	cv := m.codexCanvas()
	if c.turn != nil {
		if p := float64(time.Since(c.turn.at)) / float64(codexTurnFor); p < 1 && c.turn.from != nil &&
			c.turn.from.Width() == cv.Width() && c.turn.from.Height() == cv.Height() {
			codexTurnBlend(cv, c.turn.from, p, c.turn.ahead)
		}
	}
	cv.renderTo(sb)
}

// codexTurnBlend lays the page before over the new one, uncovering it from
// the right (ahead) or the left, with a shadow at the edge.
func codexTurnBlend(cv, from *boardCanvas, p float64, ahead bool) {
	e := p * p * (3 - 2*p) // starts and ends gently
	w := cv.Width()
	cut := int(float64(w) * (1 - e))
	if !ahead {
		cut = int(float64(w) * e)
	}
	for y := 0; y < cv.Height(); y++ {
		for x := 0; x < w; x++ {
			old := ahead && x < cut || !ahead && x >= cut
			if old {
				cv.Lines[y][x] = from.Lines[y][x]
				continue
			}
			if d := abs(x - cut); d < 3 { // the edge's shadow
				cell := cv.Lines[y][x]
				cell.Style.Bg = rgbaOf(mixHex(colorHex(cell.Style.Bg, cBG), "#000000", 0.35-0.1*float64(d)))
				cv.Lines[y][x] = cell
			}
		}
	}
}

// codexCanvas draws Codex mode: the library, or the open codex.
func (m *Model) codexCanvas() *boardCanvas {
	c := m.cx()
	w, h := m.width, m.boardH()
	cv := newBoardCanvas(w, h)
	desk := codexDesk()
	cvFill(cv, 0, 0, w, h, desk)
	c.rects = map[int][4]int{}
	c.box = map[string][4]int{}
	now := time.Now()
	if c.cf == nil {
		m.codexLibrary(cv, desk, now)
		return cv
	}
	top := m.codexTop() // editing: the inspector band above the page (codex_panel.go)
	pw, ph := codexPageSize(w, h-top, c.cf.Tall)
	px, py := (w-pw)/2, top+(h-top-ph)/2
	// The page shadow.
	cvFill(cv, px+1, py+1, pw, ph, mixHex(desk, "#000000", 0.4))
	if c.page < 0 {
		cover := codexCover(c.cf, pw, ph, now.Sub(c.pageAt).Seconds(), false)
		cvPaste(cv, cover, px, py)
		c.box["cover"] = [4]int{px, py, pw, ph}
	} else {
		m.codexPage(cv, c.cf.Pages[c.page], px, py, pw, ph, now)
	}
	// Arrows either side: click to turn.
	dim := mixHex(desk, cText, 0.45)
	if c.page >= 0 {
		cvSet(cv, px-3, py+ph/2, "‹", dim, desk, uv.AttrBold)
		c.box["prev"] = [4]int{0, top, px, h - top}
	}
	if c.page < len(c.cf.Pages)-1 {
		cvSet(cv, px+pw+2, py+ph/2, "›", dim, desk, uv.AttrBold)
		c.box["next"] = [4]int{px + pw + 1, top, w - px - pw - 1, h - top}
	}
	if top > 0 {
		m.codexInspector(cv, w, pw, now)
	}
	if c.placing {
		m.codexAddMenu(cv, now)
	}
	if c.table {
		m.codexTableDraw(cv)
	}
	return cv
}

// cvPaste copies src onto cv with its top left at (x, y).
func cvPaste(cv, src *boardCanvas, x, y int) {
	for yy := 0; yy < src.Height(); yy++ {
		if y+yy < 0 || y+yy >= cv.Height() {
			continue
		}
		for xx := 0; xx < src.Width(); xx++ {
			if x+xx < 0 || x+xx >= cv.Width() {
				continue
			}
			cv.Lines[y+yy][x+xx] = src.Lines[yy][xx]
		}
	}
}

// ---- the cover ------------------------------------------------------------------------------

// codexCover draws codex cf's cover, w×h, t seconds into its symbol's
// animation. chosen: a bright frame (the library).
func codexCover(cf *codexFile, w, h int, t float64, chosen bool) *boardCanvas {
	cv := newBoardCanvas(max(w, 1), max(h, 1))
	col := cf.Color
	if col == "" {
		col = codexThemeCover()
	}
	gold := codexGold()
	if lum(col) > 0.6 {
		gold = "#5a3a10" // a pale cover: dark gilding
	}
	title := strings.TrimSpace(cf.Title)
	if title == "" {
		title = "Untitled"
	}
	if w < 12 || h < 5 { // too small: a block of its colour with its title
		cvFill(cv, 0, 0, w, h, col)
		cvText(cv, 0, 0, codexClip("▥"+title, w), gold, col, uv.AttrBold)
		return cv
	}
	// Leather: the colour with a faint grain.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			g := float64(fxHash(x, y, 7)%100) / 100
			cvSet(cv, x, y, " ", col, mixHex(col, "#000000", 0.04+0.06*g), 0)
		}
	}
	// The spine, with gilt bands.
	spine := mixHex(col, "#000000", 0.35)
	for y := 0; y < h; y++ {
		for x := 0; x < 3; x++ {
			cvSet(cv, x, y, " ", spine, spine, 0)
		}
		cvSet(cv, 3, y, "▏", mixHex(spine, "#000000", 0.4), cvBG(cv, 3, y, col), 0)
	}
	for _, y := range []int{1, h - 2} {
		for x := 0; x < 3; x++ {
			cvSet(cv, x, y, "═", gold, spine, 0)
		}
	}
	// The gilt frame.
	frame := mixHex(gold, col, 0.15)
	if chosen {
		frame = cAccent
	}
	x0, y0, x1, y1 := 5, 1, w-3, h-2
	if x1-x0 >= 4 && y1-y0 >= 2 {
		for x := x0 + 1; x < x1; x++ {
			cvSet(cv, x, y0, "═", frame, cvBG(cv, x, y0, col), 0)
			cvSet(cv, x, y1, "═", frame, cvBG(cv, x, y1, col), 0)
		}
		for y := y0 + 1; y < y1; y++ {
			cvSet(cv, x0, y, "║", frame, cvBG(cv, x0, y, col), 0)
			cvSet(cv, x1, y, "║", frame, cvBG(cv, x1, y, col), 0)
		}
		for _, p := range [][2]int{{x0, y0}, {x1, y0}, {x0, y1}, {x1, y1}} {
			cvSet(cv, p[0], p[1], "◆", frame, cvBG(cv, p[0], p[1], col), 0)
		}
	}
	iw := x1 - x0 - 3 // room inside the frame
	ix := x0 + 2
	// The symbol, the title and the subtitle, in the middle.
	size := 1
	switch {
	case h >= 26:
		size = 3
	case h >= 15:
		size = 2
	}
	sym := codexSymbolBig(cf.Symbol, t, size)
	tsize := 1
	if big := codexBig(title, 2); h >= 13 && len(big) > 0 && codexWidth(big[0]) <= iw {
		tsize = 2
	}
	var titleRows []string
	if tsize == 2 {
		titleRows = codexBig(title, 2)
	} else {
		titleRows = codexWrap(title, iw)
		if len(titleRows) > 2 {
			titleRows = titleRows[:2]
		}
	}
	sub := codexWrap(strings.TrimSpace(cf.Subtitle), iw)
	if len(sub) > 2 {
		sub = sub[:2]
	}
	block := len(sym) + 1 + len(titleRows)
	if len(sub) > 0 {
		block += 1 + len(sub)
	}
	if block > y1-y0-1 { // no room for the symbol
		sym = nil
		block = len(titleRows) + map[bool]int{true: 1 + len(sub), false: 0}[len(sub) > 0]
	}
	y := y0 + 1 + max((y1-y0-1-block)/2, 0)
	symCol := mixHex(codexSymColor(cf.Symbol), gold, 0.4)
	for _, r := range sym {
		cvMiddleOn(cv, ix, y, iw, r, symCol, uv.AttrBold)
		y++
	}
	if len(sym) > 0 {
		y++
	}
	for _, r := range titleRows {
		cvMiddleOn(cv, ix, y, iw, r, gold, uv.AttrBold)
		y++
	}
	if len(sub) > 0 {
		y++
		for _, r := range sub {
			cvMiddleOn(cv, ix, y, iw, r, mixHex(gold, col, 0.35), uv.AttrItalic)
			y++
		}
	}
	if n := len(cf.Pages); h >= 9 {
		cvMiddleOn(cv, ix, y1, iw, " "+plural(n, "page")+" ", mixHex(gold, col, 0.3), 0)
	}
	return cv
}

// cvMiddleOn writes s in the middle of a row, on whatever is there.
func cvMiddleOn(cv *boardCanvas, x, y, w int, s, fg string, attrs uint8) {
	s = codexClip(s, w)
	x += (w - codexWidth(s)) / 2
	for _, r := range s {
		g := string(r)
		if g != " " {
			cvSet(cv, x, y, g, fg, cvBG(cv, x, y, cBG), attrs)
		}
		x += max(codexWidth(g), 1)
	}
}

// codexWrap breaks s into lines at most w cells wide, at spaces (a word
// longer than a line is cut). Line breaks in s are kept.
func codexWrap(s string, w int) []string {
	w = max(w, 1)
	var out []string
	for _, para := range strings.Split(s, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			if s != "" {
				out = append(out, "")
			}
			continue
		}
		line, lw := "", 0
		for _, wd := range words {
			ww := codexWidth(wd)
			for ww > w { // a word too long for a line: cut it
				if line != "" {
					out = append(out, line)
					line, lw = "", 0
				}
				head, rest := codexCut(wd, w)
				out = append(out, head)
				wd, ww = rest, codexWidth(rest)
			}
			switch {
			case line == "":
				line, lw = wd, ww
			case lw+1+ww <= w:
				line += " " + wd
				lw += 1 + ww
			default:
				out = append(out, line)
				line, lw = wd, ww
			}
		}
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// codexCut splits s after at most w cells.
func codexCut(s string, w int) (string, string) {
	n := 0
	for i, r := range s {
		rw := max(codexWidth(string(r)), 1)
		if n+rw > w {
			if i == 0 {
				_, size := firstRune(s)
				return s[:size], s[size:]
			}
			return s[:i], s[i:]
		}
		n += rw
	}
	return s, ""
}

func firstRune(s string) (rune, int) {
	for i, r := range s {
		_ = i
		return r, len(string(r))
	}
	return 0, 0
}

// ---- a page ---------------------------------------------------------------------------------

// codexInner is the writing area of a pw×ph page at (px, py).
func codexInner(px, py, pw, ph int) (int, int, int, int) {
	mx, top, bottom := 3, 1, 2
	if pw < 40 {
		mx = 2
	}
	return px + mx, py + top, max(pw-2*mx, 4), max(ph-top-bottom, 2)
}

// codexPage draws page p, pw×ph at (px, py).
func (m *Model) codexPage(cv *boardCanvas, p *codexPage, px, py, pw, ph int, now time.Time) {
	c := m.cx()
	paper := c.paper(p)
	ink := codexInkOn(paper)
	cvFill(cv, px, py, pw, ph, paper)
	// A soft line down the binding side, and the page number.
	for y := py; y < py+ph; y++ {
		cvSet(cv, px, y, "▏", mixHex(paper, ink, 0.18), paper, 0)
	}
	// An ember rule along the top and bottom edges, with gilt corners.
	rule := mixHex(paper, cEmber, 0.3)
	for x := px + 1; x < px+pw-1; x++ {
		cvSet(cv, x, py, "─", rule, paper, 0)
		cvSet(cv, x, py+ph-1, "─", rule, paper, 0)
	}
	for _, q := range [][2]int{{px, py}, {px + pw - 1, py}, {px, py + ph - 1}, {px + pw - 1, py + ph - 1}} {
		cvSet(cv, q[0], q[1], "◆", mixHex(paper, cAccent, 0.6), paper, 0)
	}
	num := " " + itoa(c.page+1) + " "
	cvMiddle(cv, px, py+ph-1, pw, num, mixHex(paper, ink, 0.6), paper, 0)
	ix, iy, iw, ih := codexInner(px, py, pw, ph)
	c.inner = [4]int{ix, iy, iw, ih}
	slots := codexSlotRects(p.Layout, iw, ih)
	for si, s := range slots {
		sx, sy, sw, sh := ix+s[0], iy+s[1], s[2], s[3]
		els := elsIn(p, si)
		if c.edit && c.writing == 0 { // the slots show while editing
			edge := mixHex(paper, ink, 0.22)
			for x := sx; x < sx+sw; x++ {
				if (x-sx)%2 == 0 {
					cvSet(cv, x, sy, "┈", edge, paper, 0)
					cvSet(cv, x, sy+sh-1, "┈", edge, paper, 0)
				}
			}
			if len(els) == 0 {
				cvMiddle(cv, sx, sy+sh/2, sw, "slot "+itoa(si+1)+" · Alt+N adds", mixHex(paper, ink, 0.4), paper, uv.AttrItalic)
			}
		}
		m.codexSlot(cv, els, sx, sy, sw, sh, paper, ink, now)
	}
	// The chosen thing (editing) or the one Tab reached (reading): corners round it.
	mark := c.focus
	if c.edit {
		mark = c.sel
	}
	if r, ok := c.rects[mark]; ok && mark != 0 && c.writing == 0 && !c.drawing {
		col := cAccent
		if lum(paper) > 0.55 {
			col = mixHex(cAccent, "#000000", 0.25)
		}
		x0, y0, x1, y1 := r[0]-1, r[1]-1, r[0]+r[2], r[1]+r[3]
		y0, y1 = max(y0, py), min(y1, py+ph-1)
		for _, p := range []struct {
			x, y int
			g    string
		}{{x0, y0, "┏"}, {x1, y0, "┓"}, {x0, y1, "┗"}, {x1, y1, "┛"}} {
			cvSet(cv, p.x, p.y, p.g, col, cvBG(cv, p.x, p.y, paper), uv.AttrBold)
		}
	}
	m.codexMarks(cv, p, paper, now) // pressable things, a button's target (codex_panel.go)
}

// codexSlot lays the things of one slot out one under another, and draws them.
func (m *Model) codexSlot(cv *boardCanvas, els []*codexEl, x, y, w, h int, paper, ink string, now time.Time) {
	c := m.cx()
	if len(els) == 0 {
		return
	}
	looks := make([]codexLook, len(els))
	heights := make([]int, len(els))
	fixed, flex := 0, 0
	for i, e := range els {
		looks[i] = c.look(e, now)
		if c.writing == codexWriteEl && c.sel == e.ID {
			looks[i].hidden = false
		}
		heights[i] = c.elHeight(e, looks[i], w, now)
		if heights[i] < 0 {
			flex++
		} else {
			fixed += heights[i]
		}
	}
	gap := 0
	if len(els) > 1 && fixed+flex*3+len(els)-1 <= h {
		gap = 1
	}
	left := h - fixed - gap*(len(els)-1)
	for i := range heights {
		if heights[i] < 0 {
			heights[i] = max(left/max(flex, 1), 3)
		}
	}
	total := gap * (len(els) - 1)
	for _, hh := range heights {
		total += hh
	}
	yy := y + max((h-total)/2, 0)
	for i, e := range els {
		eh := min(heights[i], y+h-yy)
		if eh <= 0 {
			break
		}
		m.codexEl(cv, e, looks[i], x, yy, w, eh, paper, ink, now)
		yy += heights[i] + gap
	}
}

// elHeight is how many rows thing e needs at width w (-1: it takes what is left).
func (c *codexState) elHeight(e *codexEl, l codexLook, w int, now time.Time) int {
	switch e.Kind {
	case ceButton:
		return 3
	case ceSymbol:
		return len(codexSymbolBig(l.symbol, 0, l.size))
	case ceImage, ceDrawing:
		return -1
	}
	if c.writing == codexWriteEl && c.sel == e.ID {
		return max(len(writeLines(c.words, w)), 1)
	}
	return max(len(codexTextRows(l, w)), 1)
}

// codexTextRows are a text's rows as they show: in its font, wrapped, or
// in big letters.
func codexTextRows(l codexLook, w int) []string {
	text := l.text
	if l.size <= 1 {
		return codexWrap(codexFont(text, l.font), w)
	}
	var rows []string
	for _, line := range strings.Split(text, "\n") {
		big := codexBig(line, l.size)
		if len(big) > 0 && codexWidth(big[0]) > w && l.size > 2 { // too wide: a size smaller
			big = codexBig(line, 2)
		}
		rows = append(rows, big...)
	}
	return rows
}

// codexEl draws thing e (as it looks: l) in the w×h box at (x, y).
func (m *Model) codexEl(cv *boardCanvas, e *codexEl, l codexLook, x, y, w, h int, paper, ink string, now time.Time) {
	c := m.cx()
	if l.hidden {
		return // no click box either: a click on empty paper must not find it
	}
	c.rects[e.ID] = [4]int{x, y, w, h}
	fg := l.color
	if fg == "" {
		fg = ink
	}
	t := now.Sub(l.animAt).Seconds()
	// Each cell goes through the animation's pose on its way to the page.
	put := func(lx, ly int, g, cfg, cbg string, attrs uint8, ew, eh int) {
		p := codexPose(l.anim, lx, ly, ew, eh, t, e.ID)
		if p.Hide {
			return
		}
		tx, ty := lx+p.DX, ly+p.DY
		if l.anim == codexMarquee {
			tx = ((lx+p.DX)%ew + ew) % ew
		}
		if p.Glyph != "" && g != " " {
			g = p.Glyph
		}
		if p.Tint != "" {
			cfg = mixHex(cfg, p.Tint, p.TintAmt)
		}
		under := cvBG(cv, x+tx, y+ty, paper)
		if cbg == "" {
			cbg = under
		}
		if l.flash > 0 {
			cfg = mixHex(cfg, "#ffffff", l.flash*0.8)
			if cbg != under {
				cbg = mixHex(cbg, "#ffffff", l.flash*0.6)
			}
		}
		if l.invert {
			cfg, cbg = cbg, cfg
		}
		if p.Fade > 0 {
			cfg = mixHex(cfg, under, p.Fade)
			cbg = mixHex(cbg, under, p.Fade)
		}
		if l.ghost {
			cfg = mixHex(cfg, under, 0.6)
			cbg = mixHex(cbg, under, 0.6)
		}
		if tx < 0 || ty < 0 || tx >= ew+2 || ty >= eh+1 { // not far outside its box
			return
		}
		if !c.insideInner(x+tx, y+ty) {
			return
		}
		cvSet(cv, x+tx, y+ty, g, cfg, cbg, attrs)
	}
	switch e.Kind {
	case ceText:
		if c.writing == codexWriteEl && c.sel == e.ID {
			m.codexWriting(cv, x, y, w, h, fg, paper)
			return
		}
		rows := codexTextRows(l, w)
		if len(rows) == 0 && c.edit {
			cvMiddle(cv, x, y, w, "text · Enter: write", mixHex(paper, ink, 0.4), paper, uv.AttrItalic)
			return
		}
		attrs := uint8(0)
		if l.size <= 1 && l.font == 0 {
			attrs = 0
		}
		for ry, r := range rows {
			if ry >= h {
				break
			}
			rw := codexWidth(r)
			ox := codexAlign(e.Align, w, rw)
			cx := ox
			for _, ch := range r {
				g := string(ch)
				put(cx, ry, g, fg, "", attrs, w, h)
				cx += max(codexWidth(g), 1)
			}
		}
	case ceButton:
		label := codexFont(l.text, l.font)
		if c.writing == codexWriteEl && c.sel == e.ID {
			label = string(c.words)
		}
		bw := min(max(codexWidth(label)+4, 8), w)
		bx := codexAlign(e.Align, w, bw)
		col := l.color
		if col == "" {
			col = codexThemeButton()
		}
		txt := codexInkOn(col)
		edge := mixHex(col, "#000000", 0.3)
		if !c.edit && c.focus == e.ID {
			edge = cAccent
		}
		c.rects[e.ID] = [4]int{x + bx, y, bw, min(3, h)}
		rows := []string{"╭" + strings.Repeat("─", bw-2) + "╮", "│" + strings.Repeat(" ", bw-2) + "│", "╰" + strings.Repeat("─", bw-2) + "╯"}
		for ry, r := range rows {
			if ry >= h {
				break
			}
			cx := bx
			for _, ch := range r {
				g := string(ch)
				bg := col
				if ch != ' ' {
					bg = ""
				}
				gc := edge
				if ch == ' ' {
					gc = txt
				}
				put(cx, ry, g, gc, bg, 0, w, h)
				cx++
			}
		}
		if h >= 2 {
			label = codexClip(label, bw-4)
			cx := bx + (bw-codexWidth(label))/2
			for _, ch := range label {
				g := string(ch)
				put(cx, 1, g, txt, col, uv.AttrBold, w, h)
				cx += max(codexWidth(g), 1)
			}
			if c.writing == codexWriteEl && c.sel == e.ID {
				cvSet(cv, x+cx, y+1, " ", col, txt, 0) // the cursor
			}
		}
	case ceSymbol:
		rows := codexSymbolBig(l.symbol, now.Sub(c.pageAt).Seconds(), l.size)
		col := l.color
		if col == "" {
			col = codexSymColor(l.symbol)
		}
		for ry, r := range rows {
			if ry >= h {
				break
			}
			cx := codexAlign(e.Align, w, codexWidth(r))
			for _, ch := range r {
				g := string(ch)
				if g != " " {
					put(cx, ry, g, col, "", uv.AttrBold, w, h)
				}
				cx += max(codexWidth(g), 1)
			}
		}
	case ceImage:
		if l.image == "" {
			if c.edit {
				m.codexPlaceholder(cv, x, y, w, h, "▨ picture · Enter: choose a file", paper, ink)
			}
			return
		}
		pc := pictureCanvas(filepath.Join(codexDir(c.id), "images", l.image), w, h, paper)
		for ry := 0; ry < min(h, pc.Height()); ry++ {
			for rx := 0; rx < min(w, pc.Width()); rx++ {
				cell := pc.Lines[ry][rx]
				put(rx, ry, cell.Content, colorHex(cell.Style.Fg, paper), colorHex(cell.Style.Bg, paper), 0, w, h)
			}
		}
	case ceDrawing:
		m.codexDrawing(cv, e, l, x, y, w, h, paper, ink, put)
	}
}

// codexAlign is where a row rw wide starts in a box w wide.
func codexAlign(align, w, rw int) int {
	switch align {
	case 1:
		return 0
	case 2:
		return max(w-rw, 0)
	}
	return max((w-rw)/2, 0)
}

func (c *codexState) insideInner(x, y int) bool {
	in := c.inner
	return x >= in[0]-1 && y >= in[1] && x < in[0]+in[2]+1 && y < in[1]+in[3]
}

func (m *Model) codexPlaceholder(cv *boardCanvas, x, y, w, h int, s, paper, ink string) {
	edge := mixHex(paper, ink, 0.3)
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			if (xx+yy)%4 == 0 {
				cvSet(cv, xx, yy, "╱", mixHex(paper, ink, 0.1), paper, 0)
			}
		}
	}
	cvMiddle(cv, x, y+h/2, w, " "+s+" ", edge, paper, uv.AttrItalic)
}

// codexDrawing draws a drawing's dots, stretched to w×h cells (two dots a cell, up and down).
func (m *Model) codexDrawing(cv *boardCanvas, e *codexEl, l codexLook, x, y, w, h int, paper, ink string,
	put func(lx, ly int, g, fg, bg string, attrs uint8, ew, eh int)) {
	c := m.cx()
	grid := make(map[[2]int]int8, len(e.Ink))
	for _, d := range e.Ink {
		grid[[2]int{d.X, d.Y}] = d.C
	}
	if len(grid) == 0 && c.edit && !(c.drawing && c.sel == e.ID) {
		m.codexPlaceholder(cv, x, y, w, h, "✎ drawing · Enter: draw", paper, ink)
		return
	}
	// dot is the colour of target dot (tx, ty) of w × 2h, or "".
	dot := func(tx, ty int) string {
		x0, x1 := tx*codexInkW/w, max((tx+1)*codexInkW/w, tx*codexInkW/w+1)
		y0, y1 := ty*codexInkH/(2*h), max((ty+1)*codexInkH/(2*h), ty*codexInkH/(2*h)+1)
		for sy := y0; sy < y1; sy++ {
			for sx := x0; sx < x1; sx++ {
				if ci, ok := grid[[2]int{sx, sy}]; ok && int(ci) < len(codexInkColors) {
					col := codexInkColors[ci]
					if l.color != "" && e.On.Do == "colour" && l.color != e.Color {
						col = mixHex(col, l.color, 0.7)
					}
					return col
				}
			}
		}
		return ""
	}
	for ry := 0; ry < h; ry++ {
		for rx := 0; rx < w; rx++ {
			top, bot := dot(rx, 2*ry), dot(rx, 2*ry+1)
			switch {
			case top == "" && bot == "":
				continue
			case top == "":
				put(rx, ry, "▄", bot, paper, 0, w, h)
			case bot == "":
				put(rx, ry, "▀", top, paper, 0, w, h)
			default:
				put(rx, ry, "▀", top, bot, 0, w, h)
			}
		}
	}
	if c.drawing && c.sel == e.ID { // the brush
		bx := c.brushX * w / codexInkW
		by := c.brushY * 2 * h / codexInkH / 2
		col := "#ffffff"
		if c.brushC >= 0 && int(c.brushC) < len(codexInkColors) {
			col = codexInkColors[c.brushC]
		}
		g := "◎"
		if c.penDown {
			g = "●"
		}
		cvSet(cv, x+bx, y+by, g, col, mixHex(paper, ink, 0.25), uv.AttrBold)
		for _, p := range [][2]int{{-1, -1}, {w, -1}, {-1, h}, {w, h}} { // the drawing's corners
			cvSet(cv, x+p[0], y+p[1], "+", cAccent, cvBG(cv, x+p[0], y+p[1], paper), 0)
		}
	}
}

// writeLines are the words being written, broken at w (by letters, so the
// cursor can be found), with where each line starts in the words.
type writeLine struct {
	s     []rune
	start int
}

func writeLines(words []rune, w int) []writeLine {
	w = max(w, 1)
	var out []writeLine
	start := 0
	line := []rune{}
	for i, r := range words {
		if r == '\n' {
			out = append(out, writeLine{line, start})
			line, start = []rune{}, i+1
			continue
		}
		if len(line) >= w {
			out = append(out, writeLine{line, start})
			line, start = []rune{}, i
		}
		line = append(line, r)
	}
	out = append(out, writeLine{line, start})
	return out
}

// codexWriting draws the words being written plainly, with the cursor.
func (m *Model) codexWriting(cv *boardCanvas, x, y, w, h int, fg, paper string) {
	c := m.cx()
	lines := writeLines(c.words, w)
	first := 0
	curLine := 0
	for i, l := range lines {
		if c.cur >= l.start && c.cur <= l.start+len(l.s) {
			curLine = i
		}
	}
	if curLine >= h {
		first = curLine - h + 1
	}
	hl := mixHex(paper, cAccent, 0.15)
	for ry := 0; ry < h && first+ry < len(lines); ry++ {
		l := lines[first+ry]
		cvFill(cv, x, y+ry, w, 1, hl)
		for i, r := range l.s {
			cvSet(cv, x+i, y+ry, string(r), fg, hl, 0)
		}
		if first+ry == curLine {
			cx := c.cur - l.start
			g := " "
			if cx < len(l.s) {
				g = string(l.s[cx])
			}
			cvSet(cv, x+cx, y+ry, g, hl, cAccent, 0)
		}
	}
}

// ---- the library ------------------------------------------------------------------------------

const codexTileW, codexTileH = 26, 12

// codexLibrary draws every codex as its cover, side by side.
func (m *Model) codexLibrary(cv *boardCanvas, desk string, now time.Time) {
	c := m.cx()
	w, h := cv.Width(), cv.Height()
	dim := mixHex(desk, cText, 0.5)
	if len(c.lib) == 0 {
		cvMiddle(cv, 0, h/2-1, w, "No codexes yet.", cText, desk, uv.AttrBold)
		cvMiddle(cv, 0, h/2+1, w, "Alt+N makes one: a book of pages with text, pictures, drawings, moving symbols and buttons.", dim, desk, 0)
		return
	}
	cols := max(1, (w-2)/(codexTileW+3))
	rows := max(1, (h-1)/(codexTileH+2))
	first := 0
	if r := c.libSel / cols; r >= rows {
		first = (r - rows + 1) * cols
	}
	gridW := cols*(codexTileW+3) - 3
	x0 := (w - gridW) / 2
	y0 := max((h-min(rows, (len(c.lib)+cols-1)/cols)*(codexTileH+2))/2, 0)
	t := now.Sub(c.pageAt).Seconds()
	for i := first; i < len(c.lib) && i < first+rows*cols; i++ {
		e := c.lib[i]
		cf, err := codexPeek(e.id)
		if err != nil {
			continue
		}
		col, row := (i-first)%cols, (i-first)/cols
		tx, ty := x0+col*(codexTileW+3), y0+row*(codexTileH+2)
		chosen := i == c.libSel
		ct := 0.0
		if chosen {
			ct = t
		}
		cvFill(cv, tx+1, ty+1, codexTileW, codexTileH, mixHex(desk, "#000000", 0.4))
		cvPaste(cv, codexCover(cf, codexTileW, codexTileH, ct, chosen), tx, ty)
		c.box["lib:"+itoa(i)] = [4]int{tx, ty, codexTileW, codexTileH}
		label := "changed " + e.at.Format("2 Jan 15:04")
		fg := dim
		if chosen {
			fg = cAccent
		}
		cvMiddle(cv, tx, ty+codexTileH, codexTileW, label, fg, desk, 0)
	}
}
