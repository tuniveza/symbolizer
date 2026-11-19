package main

// ---- drawing a codex: its cover, its pages, the library ----------------------------------

import (
	"math"
	"path/filepath"
	"strings"
	"time"

	uv "github.com/charmbracelet/ultraviolet"
)

const codexTurnFor = 300 * time.Millisecond // a page turning

// ---- the theme's colours (Emberistal unless another sy theme is on) ----

// The desk is darker than the theme's background and the paper a little
// warmer, so a page stands out from the desk.
func codexDesk() string        { return mixHex(cBG, "#000000", 0.5) }
func codexThemePaper() string  { return mixHex(cBG, cEmber, 0.08) }
func codexThemeCover() string  { return mixHex(cBG, cEmber, 0.16) }
func codexThemeButton() string { return cEmber }
func codexGold() string        { return cAccent }

// codexSymColor is a symbol's own colour: the theme's ember gradient.
func codexSymColor(i int) string {
	return cGradient[(i%3+3)%3]
}

// codexReadW is how wide a block of normal text gets at most: wider lines are hard to read.
const codexReadW = 72

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

// relLum is a colour's relative luminance (WCAG).
func relLum(hex string) float64 {
	r, g, b := rgb(hex)
	lin := func(v int) float64 {
		c := float64(v) / 255
		if c <= 0.03928 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}

// contrastOf is the WCAG contrast ratio of two colours, 1 … 21.
func contrastOf(a, b string) float64 {
	la, lb := relLum(a), relLum(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// codexInkOn is a colour that reads on bg: a dark ink or the theme's light
// one, whichever stands out more.
func codexInkOn(bg string) string {
	dark := "#2b1608"
	if contrastOf(dark, bg) > contrastOf(cStatusText, bg) {
		return dark
	}
	return cStatusText
}

// codexAtLeast is fg, moved towards the ink that reads on bg until the two
// have at least contrast ratio least.
func codexAtLeast(fg, bg string, least float64) string {
	ink := codexInkOn(bg)
	for i := 1; i <= 10 && contrastOf(fg, bg) < least; i++ {
		fg = mixHex(fg, ink, 0.1*float64(i))
	}
	return fg
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
	// Writing the title or subtitle: the cover shows the words as they are typed.
	if r, ok := c.box["cover"]; ok && c.cf != nil && c.page < 0 &&
		(c.writing == codexWriteTitle || c.writing == codexWriteSubtitle) {
		t := time.Since(c.pageAt).Seconds()
		cvPaste(cv, codexCoverW(c.cf, r[2], r[3], t, false, c.writing, c.words, c.cur), r[0], r[1])
	}
	if t := c.turn; t != nil {
		if p := float64(time.Since(t.at)) / float64(codexTurnDur(t.anim)); p < 1 && t.from != nil &&
			t.from.Width() == cv.Width() && t.from.Height() == cv.Height() {
			r := t.rect
			if r[2] <= 0 || r != c.pageRect { // the page moved (the screen changed size): the whole view
				r = [4]int{0, 0, cv.Width(), cv.Height()}
			}
			codexTurnApply(cv, t.from, t.anim, p, t.ahead, r)
			codexTurnFix(cv, [4]int{0, r[1], cv.Width(), r[3]})
		}
	}
	cv.renderTo(sb)
}

// codexCanvas draws Codex mode: the library, or the open codex.
func (m *Model) codexCanvas() *boardCanvas {
	c := m.cx()
	w, h := m.width, m.boardH()
	cv := newBoardCanvas(w, h)
	desk := codexDesk()
	cvFill(cv, 0, 0, w, h, desk)
	c.rects = map[int][4]int{}
	c.place = map[int][4]int{}
	c.box = map[string][4]int{}
	now := time.Now()
	if c.cf == nil {
		m.codexLibrary(cv, desk, now)
		return cv
	}
	top := m.codexTop()   // editing: the inspector band above the page (codex_panel.go)
	room := m.codexRoom() // kept reading too, so the page keeps its size
	pw, ph := codexPageSize(w, h-room, c.cf.Tall)
	px, py := (w-pw)/2, top+(h-top-ph)/2
	c.pageRect = [4]int{px, py, pw, ph}
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
	dim := mixHex(desk, cText, 0.65)
	if c.page >= 0 {
		cvSet(cv, px-3, py+ph/2, "‹", dim, desk, uv.AttrBold)
		c.box["prev"] = [4]int{0, top, px, h - top}
	}
	if c.page < len(c.cf.Pages)-1 {
		cvSet(cv, px+pw+2, py+ph/2, "›", dim, desk, uv.AttrBold)
		c.box["next"] = [4]int{px + pw + 1, top, w - px - pw - 1, h - top}
	}
	if top > 0 {
		m.codexInspector(cv, w, now)
	}
	if c.placing {
		m.codexAddMenu(cv, now)
	}
	if c.table {
		m.codexTableDraw(cv)
	}
	if c.pick != nil {
		m.codexPickDraw(cv)
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
	return codexCoverW(cf, w, h, t, chosen, 0, nil, 0)
}

// codexCoverW is codexCover while the title or subtitle is being written
// (what: codexWriteTitle or codexWriteSubtitle): it shows words, with the
// cursor at cur.
func codexCoverW(cf *codexFile, w, h int, t float64, chosen bool, what int, words []rune, cur int) *boardCanvas {
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
	// The front of the book (the user asked, 19 Nov 2026: a front, not a
	// side): leather with a faint grain, the board's bevel catching the light
	// at the top and left and in shadow at the bottom and right, gilt corner
	// pieces, and a double gilt frame with ornaments.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			g := float64(fxHash(x, y, 7)%100) / 100
			bg := mixHex(col, "#000000", 0.04+0.06*g)
			switch {
			case y == 0 || x == 0:
				bg = mixHex(col, "#ffffff", 0.10)
			case y == h-1 || x == w-1:
				bg = mixHex(col, "#000000", 0.30)
			}
			cvSet(cv, x, y, " ", col, bg, 0)
		}
	}
	for _, q := range []struct {
		x, y int
		g    string
	}{{0, 0, "◤"}, {w - 1, 0, "◥"}, {0, h - 1, "◣"}, {w - 1, h - 1, "◢"}} {
		cvSet(cv, q.x, q.y, q.g, mixHex(gold, col, 0.1), cvBG(cv, q.x, q.y, col), 0)
	}
	// The gilt frame.
	frame := mixHex(gold, col, 0.15)
	if chosen {
		frame = cAccent
	}
	x0, y0, x1, y1 := 3, 1, w-4, h-2
	if w < 20 {
		x0, x1 = 2, w-3
	}
	if cb := cf.CoverBorder; cb > 0 && x1-x0 >= 4 && y1-y0 >= 2 { // a chosen frame (B on the cover)
		if cb < codexCoverNone {
			codexDrawEdge(cv, codexPageBorders[cb-1], x0, y0, x1, y1, frame, mixHex(gold, col, 0.05), col)
		}
	} else if x1-x0 >= 4 && y1-y0 >= 2 {
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
		if x1-x0 >= 12 {
			mid := (x0 + x1) / 2
			cvSet(cv, mid, y0, "❖", frame, cvBG(cv, mid, y0, col), 0)
		}
		// A thin inner frame, on a cover big enough for it.
		if x1-x0 >= 26 && y1-y0 >= 12 {
			thin := mixHex(frame, col, 0.45)
			for x := x0 + 3; x < x1-2; x++ {
				cvSet(cv, x, y0+1, "─", thin, cvBG(cv, x, y0+1, col), 0)
				cvSet(cv, x, y1-1, "─", thin, cvBG(cv, x, y1-1, col), 0)
			}
			for y := y0 + 2; y < y1-1; y++ {
				cvSet(cv, x0+2, y, "│", thin, cvBG(cv, x0+2, y, col), 0)
				cvSet(cv, x1-2, y, "│", thin, cvBG(cv, x1-2, y, col), 0)
			}
			for _, p := range []struct {
				x, y int
				g    string
			}{{x0 + 2, y0 + 1, "╭"}, {x1 - 2, y0 + 1, "╮"}, {x0 + 2, y1 - 1, "╰"}, {x1 - 2, y1 - 1, "╯"}} {
				cvSet(cv, p.x, p.y, p.g, thin, cvBG(cv, p.x, p.y, col), 0)
			}
			y0, y1 = y0+1, y1-1 // the words stay inside it
			x0, x1 = x0+2, x1-2
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
	// The title: in big letters when they fit (one line, or two on a tall
	// cover), else up to two lines of normal letters.
	titleWords := []rune(title)
	titleCur := -1
	if what == codexWriteTitle {
		titleWords, titleCur = words, cur
	}
	tsize, tmax := 1, 0
	tfont := (cf.TitleFont%16 + 16) % 16
	if what == codexWriteTitle {
		tfont = codexWriteFont(tfont)
	}
	if h >= 13 && tfont == 0 { // big letters: only plain (another font shows in normal letters)
		tsize, tmax = 2, 3
		if h >= 22 {
			tmax = 6
		}
	}
	tl := codexLay(titleWords, tfont, tsize, iw, iw, tmax)
	if tl.size == 1 {
		tl.lines = codexLinesAt(tl.lines, titleCur, 2, iw)
	}
	subWords := []rune(strings.TrimSpace(cf.Subtitle))
	subCur := -1
	if what == codexWriteSubtitle {
		subWords, subCur = words, cur
	}
	sl := codexLay(subWords, 0, 1, iw, iw, 0)
	sl.lines = codexLinesAt(sl.lines, subCur, 2, iw)
	subRows := len(sl.lines)
	if len(subWords) == 0 && subCur < 0 {
		subRows = 0
	}
	titleRows := tl.rows()
	block := len(sym) + 1 + titleRows
	if subRows > 0 {
		block += 1 + subRows
	}
	if block > y1-y0-1 { // no room for the symbol
		sym = nil
		block = titleRows + map[bool]int{true: 1 + subRows, false: 0}[subRows > 0]
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
	y += codexDrawLines(cv, ix, y, iw, tl.lines, tl.size, tfont, 0, gold, uv.AttrBold, titleCur, gold)
	if subRows > 0 {
		y++
		subCol := mixHex(gold, col, 0.35)
		y += codexDrawLines(cv, ix, y, iw, sl.lines, 1, 0, 0, subCol, uv.AttrItalic, subCur, gold)
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
	// An ember rule along the top and bottom edges, with gilt corners, or
	// the page's border (Alt+B; Alt+B on the cover for every page).
	rule := mixHex(paper, cEmber, 0.3)
	// A thin edge down the outer side too, so the page's shape reads on the desk.
	for y := py + 1; y < py+ph-1; y++ {
		cvSet(cv, px+pw-1, y, "▕", rule, paper, 0)
	}
	if f, ok := c.cf.pageBorder(p); ok {
		codexDrawPageBorder(cv, f, px+1, py, px+pw-2, py+ph-1, paper)
	} else {
		for x := px + 1; x < px+pw-1; x++ {
			cvSet(cv, x, py, "─", rule, paper, 0)
			cvSet(cv, x, py+ph-1, "─", rule, paper, 0)
		}
		for _, q := range [][2]int{{px, py}, {px + pw - 1, py}, {px, py + ph - 1}, {px + pw - 1, py + ph - 1}} {
			cvSet(cv, q[0], q[1], "◆", mixHex(paper, cAccent, 0.6), paper, 0)
		}
	}
	ix, iy, iw, ih := codexInner(px, py, pw, ph)
	// The header and footer (H, U): a row each, inside the border.
	if c.hasHeader(p) && ih > 4 {
		m.codexHF(cv, codexWriteHeader, ix, py+1, iw, paper, ink)
		iy, ih = iy+1, ih-1
	}
	foot := c.hasFooter(p) && ih > 4
	if foot {
		m.codexHF(cv, codexWriteFooter, ix, py+ph-2, iw, paper, ink)
		ih--
	}
	if !foot || !strings.Contains(c.cf.Footer, "{page}") { // the footer shows the number: once is enough
		num := " " + itoa(c.page+1) + " "
		cvMiddle(cv, px, py+ph-1, pw, num, mixHex(paper, ink, 0.6), paper, 0)
	}
	c.inner = [4]int{ix, iy, iw, ih}
	slots := codexSlotRects(p.Layout, iw, ih)
	card := (p.Layout%16+16)%16 == 15 // "a card in the middle"
	showSlots := c.edit && c.writing == 0
	if showSlots && !card { // the slots show while editing: marks at their corners, outside them
		edge := mixHex(paper, ink, 0.35)
		arms := map[[2]int]uint8{}
		for _, s := range slots {
			x0, y0, x1, y1 := ix+s[0]-1, iy+s[1]-1, ix+s[0]+s[2], iy+s[1]+s[3]
			arms[[2]int{x0, y0}] |= armR | armD
			arms[[2]int{x1, y0}] |= armL | armD
			arms[[2]int{x0, y1}] |= armR | armU
			arms[[2]int{x1, y1}] |= armL | armU
		}
		for q, a := range arms {
			codexBoxMark(cv, q[0], q[1], a, edge)
		}
	}
	for si, s := range slots {
		sx, sy, sw, sh := ix+s[0], iy+s[1], s[2], s[3]
		els := elsIn(p, si)
		slotPaper := paper
		if card {
			slotPaper = codexCard(cv, sx, sy, sw, sh, paper, py, ph)
		}
		if showSlots && len(els) == 0 {
			cvMiddle(cv, sx, sy+sh/2, sw, "slot "+itoa(si+1)+" · Alt+N adds", mixHex(slotPaper, ink, 0.55), slotPaper, uv.AttrItalic)
		}
		m.codexSlot(cv, els, sx, sy, sw, sh, slotPaper, ink, now)
	}
	// The chosen thing (editing) or the one Tab reached (reading): its cells
	// tinted, and corners round it where the paper is empty.
	mark := c.focus
	if c.edit {
		mark = c.sel
	}
	if r, ok := c.rects[mark]; ok && mark != 0 && c.writing == 0 && !c.drawing {
		col := cAccent
		if lum(paper) > 0.55 {
			col = mixHex(cAccent, "#000000", 0.25)
		}
		for y := r[1]; y < r[1]+r[3]; y++ {
			for x := r[0]; x < r[0]+r[2]; x++ {
				if x >= 0 && y >= 0 && x < cv.Width() && y < cv.Height() {
					cell := &cv.Lines[y][x]
					cell.Style.Bg = rgbaOf(mixHex(colorHex(cell.Style.Bg, paper), cAccent, 0.16))
				}
			}
		}
		x0, y0, x1, y1 := r[0]-1, r[1]-1, r[0]+r[2], r[1]+r[3]
		for _, p := range []struct {
			x, y int
			g    string
		}{{x0, y0, "┏"}, {x1, y0, "┓"}, {x0, y1, "┗"}, {x1, y1, "┛"}} {
			if p.y > py && p.y < py+ph-1 && codexBlank(cv, p.x, p.y, 1) && !c.inOther(mark, x0, x1, p.y) {
				cvSet(cv, p.x, p.y, p.g, col, cvBG(cv, p.x, p.y, paper), uv.AttrBold)
			}
		}
		if c.edit && !c.table {
			m.codexDrawHandle(cv, r, col, paper) // the corner the mouse drags to resize it (codex_size.go)
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
	xs, ws := make([]int, len(els)), make([]int, len(els)) // each thing's own width (W) in the slot
	fixed, flex := 0, 0
	for i, e := range els {
		looks[i] = c.look(e, now)
		if c.writingIn(e) {
			looks[i].hidden = false
		}
		xs[i], ws[i] = codexSized(e, x, w)
		pl, pt, pr, pb := codexBorderFits(e, ws[i], h)
		c.place[e.ID] = [4]int{x + pl, y + pt, w - pl - pr, h - pt - pb}
		heights[i] = c.elHeight(e, looks[i], ws[i]-pl-pr, h-pt-pb, now)
		if heights[i] < 0 {
			flex++
		} else {
			heights[i] += pt + pb
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
	want := append([]int(nil), heights...) // what each would like
	total := gap * (len(els) - 1)
	for _, hh := range heights {
		total += hh
	}
	// Too tall for the slot: things give up rows, one at a time, so every
	// thing keeps a place: pictures and drawings first, then texts (a text
	// then picks a smaller size or is cut), symbols, buttons last; the
	// tallest of a kind first.
	give := map[string]int{ceImage: 0, ceDrawing: 0, ceText: 1, ceSymbol: 2, ceButton: 3}
	for total > h {
		ti := -1
		for i, hh := range heights {
			if hh <= 1 {
				continue
			}
			if ti < 0 || give[els[i].Kind] < give[els[ti].Kind] ||
				give[els[i].Kind] == give[els[ti].Kind] && hh > heights[ti] {
				ti = i
			}
		}
		if ti < 0 {
			break
		}
		heights[ti]--
		total--
	}
	yy := y + max((h-total)/2, 0)
	missing, cut := 0, false
	for i, e := range els {
		eh := min(heights[i], y+h-yy)
		if eh <= 0 {
			missing++
			continue
		}
		if eh < want[i] && want[i] > 0 {
			cut = true
		}
		ex, ew := xs[i], ws[i]
		pl, pt, pr, pb := codexBorderFits(e, ew, eh)
		if eh-pt-pb < 1 {
			pl, pt, pr, pb = 0, 0, 0, 0
		}
		m.codexEl(cv, e, looks[i], ex+pl, yy+pt, ew-pl-pr, eh-pt-pb, paper, ink, now)
		if f, ok := codexBorderOf(e); ok && pt > 0 && !looks[i].hidden {
			// The border goes round where the thing was drawn, and is its box.
			r := c.rects[e.ID]
			sh := pb - pt
			x0 := max(r[0]-pl, x)
			x1 := min(r[0]+r[2]-1+pr-sh, x+w-1-sh)
			y0, y1 := r[1]-pt, r[1]+r[3]-1+pb-sh
			box := [4]int{x0, y0, x1 - x0 + 1, y1 - y0 + 1}
			col := looks[i].color
			if col == "" {
				col = codexAtLeast(cAccent, paper, 2.5)
				if e.Kind == ceButton {
					col = codexThemeButton()
				}
			}
			m.codexDrawBorder(cv, f, box, col, paper, looks[i])
			c.rects[e.ID] = box
		}
		if r, ok := c.rects[e.ID]; ok && !looks[i].hidden {
			if _, bordered := codexBorderOf(e); bordered && pt > 0 { // inside the border
				r = [4]int{r[0] + 1, r[1] + 1, r[2] - 2, r[3] - 2}
				if e.Kind == ceText || e.Kind == ceSymbol {
					r[0]++
					r[2] -= 2
				}
			}
			if e.Kind == ceImage || e.Kind == ceDrawing { // a picture: its whole place
				r = [4]int{ex + pl, yy + pt, ew - pl - pr, eh - pt - pb}
			}
			codexFillApply(cv, e, r, paper, looks[i].ghost)
		}
		yy += heights[i] + gap
	}
	// Editing: say so when not everything fits.
	if c.edit && c.writing == 0 && (missing > 0 || cut) {
		s := " ▾ cut off "
		if missing > 0 {
			s = " ▾ " + itoa(missing) + " more "
		}
		if codexWidth(s) > w { // a narrow slot: the short way
			s = "▾"
			if missing > 0 {
				s += itoa(missing)
			}
		}
		s = codexClip(s, w)
		fg := codexAtLeast(cAccent, paper, 3)
		cvText(cv, x+w-codexWidth(s), y+h-1, s, fg, mixHex(paper, cAccent, 0.18), uv.AttrBold)
	}
}

// writingIn is whether the words of thing e are being written (its own, or
// the other words it swaps to).
func (c *codexState) writingIn(e *codexEl) bool {
	return (c.writing == codexWriteEl || c.writing == codexWriteAlt) && c.sel == e.ID &&
		(e.Kind == ceText || e.Kind == ceButton)
}

// codexWriteFont is the font words are written in: upside down turns lines
// back to front, so it is written plainly.
func codexWriteFont(font int) int {
	if font == fxFontUpsideDown {
		return fxFontPlain
	}
	return font
}

// elHeight is how many rows thing e needs at width w, in a slot maxH tall
// (-1: it takes what is left).
func (c *codexState) elHeight(e *codexEl, l codexLook, w, maxH int, now time.Time) int {
	switch e.Kind {
	case ceButton:
		return max(e.H, 3)
	case ceSymbol:
		return len(codexSymbolBig(l.symbol, 0, l.size))
	case ceImage, ceDrawing:
		if e.H > 0 { // its own height: a share of its place
			return max(maxH*e.H/100, 1)
		}
		return -1
	}
	if c.writingIn(e) {
		return max(codexLay(c.words, codexWriteFont(l.font), l.size, w, min(w, codexReadW), maxH).rows(), 1)
	}
	head := 0
	if e.Head > 0 && strings.TrimSpace(l.text) != "" {
		head = 1 // the line under a heading
	}
	rows, _, _ := codexTextRows(l, w, maxH-head)
	return max(len(rows), 1) + head
}

// codexTextRows are a text's rows as they show at width w (and at most maxH
// rows, if it can: 0 any): in its font, wrapped by words, in big letters at
// the biggest size up to its own that fits. It also gives back the size it
// took and how wide the block of rows may be (normal text: codexReadW).
func codexTextRows(l codexLook, w, maxH int) ([]string, int, int) {
	lay := codexLay([]rune(l.text), l.font, l.size, w, min(w, codexReadW), maxH)
	if lay.size <= 1 {
		return codexWrap(codexFont(l.text, l.font), lay.w), 1, lay.w
	}
	var rows []string
	for _, ln := range lay.lines {
		rows = append(rows, codexBig(string(trimSpaces(ln.s)), lay.size)...)
	}
	return rows, lay.size, lay.w
}

// ---- laying words out -----------------------------------------------------------------------

// writeLine is one line of laid-out words, with where it starts in them.
type writeLine struct {
	s     []rune
	start int
}

// codexLaid is words laid out: the size they took, their lines, how wide
// the block of lines is.
type codexLaid struct {
	size  int
	lines []writeLine
	w     int
}

// rows is how many screen rows the lines take.
func (l codexLaid) rows() int { return len(l.lines) * codexRowsPer(l.size) }

// codexRowsPer is how many screen rows one line of size size takes.
func codexRowsPer(size int) int {
	if size <= 1 {
		return 1
	}
	return 3 * (size - 1)
}

// trimSpaces is rs without the spaces at its end.
func trimSpaces(rs []rune) []rune {
	for len(rs) > 0 && rs[len(rs)-1] == ' ' {
		rs = rs[:len(rs)-1]
	}
	return rs
}

// codexMeasure is how many cells rs takes in font at size.
func codexMeasure(rs []rune, font, size int) int {
	if size <= 1 {
		return codexWidth(codexFont(string(rs), font))
	}
	k := size - 1
	n := 0
	for i, r := range rs {
		if i > 0 {
			n += k // the gap between letters
		}
		n += len([]rune(fxPixGlyph(r)[0])) * k
	}
	return n
}

// codexLay lays words out at the biggest size, from size down to 1, at
// which every word fits on a line w wide (w1 for normal letters) and the
// lines fit in maxH rows (0: any). Size 1 always fits: a word too long for
// its line is cut.
func codexLay(words []rune, font, size, w, w1, maxH int) codexLaid {
	for s := min(max(size, 1), codexSizes); s >= 1; s-- {
		ww := w
		if s == 1 {
			ww = w1
		}
		lines, ok := codexWrapRunes(words, ww, func(rs []rune) int { return codexMeasure(rs, font, s) })
		l := codexLaid{s, lines, max(ww, 1)}
		if s == 1 || ok && (maxH <= 0 || l.rows() <= maxH) {
			return l
		}
	}
	return codexLaid{1, []writeLine{{words, 0}}, max(w1, 1)}
}

// codexWrapRunes breaks words into lines at most w wide (as width measures
// them), at spaces; a line break in words always breaks. A word too long
// for a line is cut (ok: false). The spaces a line breaks at stay at its
// end, so a cursor there has a place.
func codexWrapRunes(words []rune, w int, width func([]rune) int) (out []writeLine, ok bool) {
	ok = true
	w = max(w, 1)
	for start := 0; start <= len(words); {
		end := start
		for end < len(words) && words[end] != '\n' {
			end++
		}
		para := words[start:end]
		ls, pos := 0, 0
		for pos < len(para) {
			j := pos
			for j < len(para) && para[j] != ' ' {
				j++
			}
			wordEnd := j
			for j < len(para) && para[j] == ' ' {
				j++
			}
			if pos > ls && width(para[ls:wordEnd]) > w { // the word goes on the next line
				out = append(out, writeLine{para[ls:pos], start + ls})
				ls = pos
			}
			for pos == ls && wordEnd > pos && width(para[pos:wordEnd]) > w { // too long for any line
				ok = false
				n := 1
				for n < wordEnd-pos && width(para[pos:pos+n+1]) <= w {
					n++
				}
				out = append(out, writeLine{para[pos : pos+n], start + pos})
				pos += n
				ls = pos
			}
			pos = j
		}
		out = append(out, writeLine{para[ls:], start + ls})
		start = end + 1
	}
	return out, ok
}

// codexCurLine is the line the cursor cur is on (-1: none).
func codexCurLine(lines []writeLine, cur int) int {
	at := -1
	for i, l := range lines {
		if cur >= l.start && cur <= l.start+len(l.s) {
			at = i
		}
	}
	return at
}

// codexLinesAt keeps at most n of lines: those round the cursor cur, or,
// with no cursor (cur < 0), the first n, the last ending in "…" when some
// are left out (w: the width it must fit).
func codexLinesAt(lines []writeLine, cur, n, w int) []writeLine {
	if len(lines) <= n {
		return lines
	}
	if cur >= 0 {
		first := max(codexCurLine(lines, cur)-n+1, 0)
		return lines[first : first+n]
	}
	out := append([]writeLine(nil), lines[:n]...)
	last := &out[n-1]
	last.s = []rune(codexClip(string(trimSpaces(last.s))+"…", w))
	return out
}

// codexDrawLines draws laid-out lines from row y, each placed in the row
// from x to x+w as align says, on whatever is there. cur >= 0: the cursor
// is drawn there too, on curBG. It gives back how many rows it took.
func codexDrawLines(cv *boardCanvas, x, y, w int, lines []writeLine, size, font, align int, fg string, attrs uint8, cur int, curBG string) int {
	per := codexRowsPer(size)
	cl := -1
	if cur >= 0 {
		cl = codexCurLine(lines, cur)
	}
	for i, ln := range lines {
		ty := y + i*per
		tr := trimSpaces(ln.s)
		var rows []string
		if size <= 1 {
			rows = []string{codexFont(string(tr), font)}
		} else {
			rows = codexBig(string(tr), size)
		}
		ox := x + codexAlign(align, w, codexMeasure(tr, font, size))
		for ry, r := range rows {
			cx := ox
			for _, ch := range r {
				g := string(ch)
				if g != " " {
					cvSet(cv, cx, ty+ry, g, fg, cvBG(cv, cx, ty+ry, cBG), attrs)
				}
				cx += max(codexWidth(g), 1)
			}
		}
		if i != cl {
			continue
		}
		at := min(max(cur-ln.start, 0), len(ln.s))
		cx := ox + codexMeasure(ln.s[:at], font, size)
		if size > 1 && at == 0 {
			cx-- // before the first letter
		}
		cx = min(max(cx, x-1), x+w)
		for ry := 0; ry < per; ry++ {
			g := " "
			if size <= 1 && cx >= 0 && cx < cv.Width() && ty >= 0 && ty < cv.Height() {
				if cell := cv.Lines[ty][cx]; cell.Content != "" {
					g = cell.Content
				}
			}
			cvSet(cv, cx, ty+ry, g, cvBG(cv, cx, ty+ry, cBG), curBG, 0)
		}
	}
	return len(lines) * per
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
		moves := p.DX != 0 || p.DY != 0 || l.anim == codexMarquee
		if tx < 0 || ty < 0 || tx >= ew || ty >= eh { // outside its box: only a moving thing, a little
			if !moves || tx < 0 || ty < 0 || tx >= ew+2 || ty >= eh+1 {
				return
			}
		}
		if !c.insideInner(x+tx, y+ty) {
			return
		}
		cvSet(cv, x+tx, y+ty, g, cfg, cbg, attrs)
	}
	switch e.Kind {
	case ceText:
		if c.writingIn(e) {
			m.codexWriting(cv, l, e.Align, x, y, w, h, fg, paper)
			return
		}
		rows, _, bw := codexTextRows(l, w, h)
		if len(rows) == 0 && c.edit {
			cvMiddle(cv, x, y, w, "text · Enter: write", mixHex(paper, ink, 0.55), paper, uv.AttrItalic)
			return
		}
		// Normal text keeps to a block at most codexReadW wide, in the middle
		// of a wide slot; its rows are placed in the block as it is aligned.
		bx := (w - bw) / 2
		minX, maxX := w, 0
		for ry, r := range rows {
			if ry >= h {
				break
			}
			rw := codexWidth(r)
			ox := bx + codexAlign(e.Align, bw, rw)
			if strings.TrimSpace(r) != "" {
				minX, maxX = min(minX, ox), max(maxX, ox+rw)
			}
			cx := ox
			attrs := uint8(0)
			if e.Head > 0 {
				attrs = uv.AttrBold
			}
			for _, ch := range r {
				g := string(ch)
				put(cx, ry, g, fg, "", attrs, w, h)
				cx += max(codexWidth(g), 1)
			}
		}
		n := min(len(rows), h)
		if e.Head > 0 && maxX > minX && len(rows) < h { // a heading: a line under it
			rule := codexHeadRule(e.Head)
			for rx := minX; rx < maxX; rx++ {
				put(rx, len(rows), rule, mixHex(fg, paper, 0.35), "", 0, w, h)
			}
			n++
		}
		if maxX > minX { // its box: where its words are
			c.rects[e.ID] = [4]int{x + minX, y, maxX - minX, n}
		}
	case ceButton:
		label := codexFont(l.text, l.font)
		if c.writingIn(e) {
			label = string(c.words)
		}
		bw := min(max(codexWidth(label)+4, 8), w)
		if e.W > 0 { // its own width: the room it was given
			bw = w
		}
		bx := codexAlign(e.Align, w, bw)
		col := l.color
		if col == "" {
			col = codexThemeButton()
		}
		st := codexButtonStyleOf(e)
		fillBG := codexButtonFill(st, col)
		txt := codexButtonInk(st, col, paper)
		focus := !c.edit && c.focus == e.ID
		nrows := min(max(e.H, 3), max(h, 3))
		labelRow := nrows / 2
		if h < 3 { // no room for its frame: one row, the label between its sides
			nrows, labelRow = 1, 0
		}
		c.rects[e.ID] = [4]int{x + bx, y, bw, nrows}
		for ry := 0; ry < nrows && ry < h; ry++ {
			for rx := 0; rx < bw; rx++ {
				g, gc, bg, skip := codexButtonCell(st, nrows, ry, rx, bw, col, paper, focus)
				if skip {
					continue
				}
				put(bx+rx, ry, g, gc, bg, 0, w, h)
			}
		}
		label = codexClip(label, bw-4)
		cx := bx + (bw-codexWidth(label))/2
		for _, ch := range label {
			g := string(ch)
			put(cx, labelRow, g, txt, fillBG, uv.AttrBold, w, h)
			cx += max(codexWidth(g), 1)
		}
		if focus && st.fill == 0 { // an unfilled button: Tab's mark is its label lit
			for xx := bx + 1; xx < bx+bw-1; xx++ {
				put(xx, labelRow, " ", txt, mixHex(paper, cAccent, 0.22), 0, w, h)
			}
			cx = bx + (bw-codexWidth(label))/2
			for _, ch := range label {
				g := string(ch)
				put(cx, labelRow, g, codexAtLeast(txt, mixHex(paper, cAccent, 0.22), 3), mixHex(paper, cAccent, 0.22), uv.AttrBold, w, h)
				cx += max(codexWidth(g), 1)
			}
		}
		if c.writingIn(e) {
			cvSet(cv, x+cx, y+labelRow, " ", codexOr(fillBG, paper), txt, 0) // the cursor
		}
	case ceSymbol:
		rows := codexSymbolBig(l.symbol, now.Sub(c.pageAt).Seconds(), l.size)
		col := l.color
		if col == "" { // the theme's colour, made to read on light paper too
			col = codexAtLeast(codexSymColor(l.symbol), paper, 3)
		}
		sw := 0
		for _, r := range rows {
			sw = max(sw, codexWidth(r))
		}
		if sw > 0 { // its box: where it is drawn
			sx := codexAlign(e.Align, w, sw)
			c.rects[e.ID] = [4]int{x + sx, y, min(sw, w-sx), min(len(rows), h)}
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
		path := filepath.Join(codexDir(c.id), "images", l.image)
		// It fills its box (cut at the edges), or shows whole (-), lined up in it.
		ox, oy, pw, ph := 0, 0, w, h
		var pc *boardCanvas
		if e.Whole {
			if tw, th, ok := pictureCells(path, w, h); ok {
				pw, ph = tw, th
				ox, oy = codexAlign(e.Align, w, tw), (h-th)/2
			}
			pc = pictureCanvas(path, pw, ph, paper)
		} else {
			pc = pictureCanvasFilled(path, w, h, paper)
		}
		c.rects[e.ID] = [4]int{x + ox, y + oy, pw, ph}
		for ry := 0; ry < min(ph, pc.Height()); ry++ {
			for rx := 0; rx < min(pw, pc.Width()); rx++ {
				cell := pc.Lines[ry][rx]
				put(ox+rx, oy+ry, cell.Content, colorHex(cell.Style.Fg, paper), colorHex(cell.Style.Bg, paper), 0, w, h)
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

// The arms of a thin box-drawing letter: which sides its lines go out to.
const (
	armU uint8 = 1 << iota
	armD
	armL
	armR
)

var codexArmGlyph = map[uint8]string{
	armL | armR: "─", armU | armD: "│", armR | armD: "┌", armL | armD: "┐", armR | armU: "└", armL | armU: "┘",
	armU | armD | armR: "├", armU | armD | armL: "┤", armL | armR | armD: "┬", armL | armR | armU: "┴",
	armU | armD | armL | armR: "┼",
}

// codexBoxMark draws a thin corner with arms a at (x, y): joined with a
// thin line already there, else only on empty paper.
func codexBoxMark(cv *boardCanvas, x, y int, a uint8, fg string) {
	if x < 0 || y < 0 || x >= cv.Width() || y >= cv.Height() {
		return
	}
	have := cv.Lines[y][x].Content
	if have != " " && have != "" {
		found := false
		for k, g := range codexArmGlyph {
			if g == have {
				a |= k
				found = true
				break
			}
		}
		if !found {
			return
		}
		fg = colorHex(cv.Lines[y][x].Style.Fg, fg)
	}
	if g, ok := codexArmGlyph[a]; ok {
		cvSet(cv, x, y, g, fg, cvBG(cv, x, y, cBG), 0)
	}
}

// inOther is whether row y, from column x0 to x1, has a thing other than id
// on it (a letter's width beside it counts too).
func (c *codexState) inOther(id, x0, x1, y int) bool {
	for oid, r := range c.rects {
		if oid != id && y >= r[1] && y < r[1]+r[3] && x1 >= r[0]-1 && x0 <= r[0]+r[2] {
			return true
		}
	}
	return false
}

// codexBlank is whether the n cells from (x, y) on cv are empty.
func codexBlank(cv *boardCanvas, x, y, n int) bool {
	if y < 0 || y >= cv.Height() || x < 0 || x+n > cv.Width() {
		return false
	}
	for i := 0; i < n; i++ {
		if cv.Lines[y][x+i].Content != " " {
			return false
		}
	}
	return true
}

// codexCard draws the card of layout "a card in the middle" round the slot
// at (x, y), w×h, on a page at row py, ph tall, and gives back its colour.
func codexCard(cv *boardCanvas, x, y, w, h int, paper string, py, ph int) string {
	card := mixHex(paper, cEmber, 0.08)
	if lum(paper) > 0.55 {
		card = mixHex(paper, "#ffffff", 0.5)
	}
	edge := mixHex(paper, cEmber, 0.35)
	x0, y0, x1, y1 := x-2, max(y-1, py+1), x+w+1, min(y+h, py+ph-2)
	for yy := y0 + 1; yy <= y1+1 && yy < py+ph-1; yy++ { // its shadow
		cvSet(cv, x1+1, yy, " ", paper, mixHex(paper, "#000000", 0.3), 0)
	}
	for xx := x0 + 1; xx <= x1+1 && y1+1 < py+ph-1; xx++ {
		cvSet(cv, xx, y1+1, " ", paper, mixHex(paper, "#000000", 0.3), 0)
	}
	cvFill(cv, x0, y0, x1-x0+1, y1-y0+1, card)
	for xx := x0 + 1; xx < x1; xx++ {
		cvSet(cv, xx, y0, "─", edge, card, 0)
		cvSet(cv, xx, y1, "─", edge, card, 0)
	}
	for yy := y0 + 1; yy < y1; yy++ {
		cvSet(cv, x0, yy, "│", edge, card, 0)
		cvSet(cv, x1, yy, "│", edge, card, 0)
	}
	for _, q := range [][3]any{{x0, y0, "╭"}, {x1, y0, "╮"}, {x0, y1, "╰"}, {x1, y1, "╯"}} {
		cvSet(cv, q[0].(int), q[1].(int), q[2].(string), edge, card, 0)
	}
	return card
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

// codexWriting draws the words being written as they will show: wrapped by
// words, in the thing's font, size and alignment, on a light band, with the
// cursor.
func (m *Model) codexWriting(cv *boardCanvas, l codexLook, align, x, y, w, h int, fg, paper string) {
	c := m.cx()
	font := codexWriteFont(l.font)
	lay := codexLay(c.words, font, l.size, w, min(w, codexReadW), h)
	per := codexRowsPer(lay.size)
	lines := lay.lines
	if n := max(h/per, 1); len(lines) > n { // the lines round the cursor
		first := max(codexCurLine(lines, c.cur)-n+1, 0)
		lines = lines[first:min(first+n, len(lines))]
	}
	bx := x + (w-lay.w)/2
	hl := mixHex(paper, cAccent, 0.15)
	cvFill(cv, bx, y, lay.w, min(max(len(lines)*per, 1), h), hl)
	codexDrawLines(cv, bx, y, lay.w, lines, lay.size, font, align, fg, 0, c.cur, cAccent)
}

// ---- the library ------------------------------------------------------------------------------

// A library tile is codexTileW … codexTileMaxW wide, and at least codexTileH tall.
const codexTileW, codexTileH, codexTileMaxW = 30, 12, 40

// codexLibrary draws every codex as its cover, side by side.
func (m *Model) codexLibrary(cv *boardCanvas, desk string, now time.Time) {
	c := m.cx()
	w, h := cv.Width(), cv.Height()
	dim := codexAtLeast(mixHex(desk, cText, 0.75), desk, 3.5)
	if len(c.lib) == 0 {
		cvMiddle(cv, 0, h/2-1, w, "No codexes yet.", cText, desk, uv.AttrBold)
		cvMiddle(cv, 0, h/2+1, w, "Alt+N makes one: a book of pages with text, pictures, drawings, moving symbols and buttons.", dim, desk, 0)
		return
	}
	// As many columns as tiles of the smallest width fit (as the keys count
	// them: codexLibCols), each tile as wide as the screen then allows.
	cols := max(1, (w-2)/(codexTileW+3))
	tw := min(max((w-2)/cols-3, codexTileW), codexTileMaxW)
	th := max(int(math.Round(float64(tw)*0.46)), codexTileH)
	allRows := (len(c.lib) + cols - 1) / cols
	rows := max(1, (h-1)/(th+2))
	if rows < allRows && rows < 2 && (h-2)/2-2 >= 9 { // a low screen: smaller tiles, two rows
		th, rows = (h-2)/2-2, 2
	}
	more := rows < allRows
	if more { // a row says what is not shown
		rows = max(1, (h-2)/(th+2))
	}
	first := 0
	if r := c.libSel / cols; r >= rows {
		first = (r - rows + 1) * cols
	}
	gridW := min(cols, len(c.lib))*(tw+3) - 3 // a short library sits in the middle too
	x0 := (w - gridW) / 2
	shown := min(rows, allRows)
	y0 := max((h-shown*(th+2))/2, 0)
	if more {
		y0 = max((h-1-shown*(th+2))/2, 0)
	}
	t := now.Sub(c.pageAt).Seconds()
	for i := first; i < len(c.lib) && i < first+rows*cols; i++ {
		e := c.lib[i]
		cf, err := codexPeek(e.id)
		if err != nil {
			continue
		}
		col, row := (i-first)%cols, (i-first)/cols
		tx, ty := x0+col*(tw+3), y0+row*(th+2)
		chosen := i == c.libSel
		ct := 0.0
		if chosen {
			ct = t
		}
		cvFill(cv, tx+1, ty+1, tw, th, mixHex(desk, "#000000", 0.4))
		cvPaste(cv, codexCover(cf, tw, th, ct, chosen), tx, ty)
		c.box["lib:"+itoa(i)] = [4]int{tx, ty, tw, th}
		label := "changed " + e.at.Format("2 Jan 15:04")
		fg := dim
		if chosen {
			fg = cAccent
		}
		cvMiddle(cv, tx, ty+th, tw, label, fg, desk, 0)
	}
	if more {
		var parts []string
		if first > 0 {
			parts = append(parts, "↑ "+itoa(first)+" above")
		}
		if n := len(c.lib) - first - rows*cols; n > 0 {
			parts = append(parts, "↓ "+itoa(n)+" more")
		}
		cvMiddle(cv, 0, min(y0+shown*(th+2), h-1), w, strings.Join(parts, " · "), dim, desk, uv.AttrBold)
	}
}
