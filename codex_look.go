package main

// ---- how pages and things look: page borders, header and footer, headings,
//      fills, and the font list with its preview (the user asked, 19 Nov 2026) --------------

import (
	"strings"

	uv "github.com/charmbracelet/ultraviolet"
)

// ---- page borders ---------------------------------------------------------------------------

// codexPageEdge is one page border: corners, and patterns repeated along each side.
type codexPageEdge struct {
	name                   string
	tl, tr, bl, br         string
	top, bottom, left, rgt string
}

var codexPageBorders = [16]codexPageEdge{
	{"thin line", "┌", "┐", "└", "┘", "─", "─", "│", "│"},
	{"rounded", "╭", "╮", "╰", "╯", "─", "─", "│", "│"},
	{"double", "╔", "╗", "╚", "╝", "═", "═", "║", "║"},
	{"heavy", "┏", "┓", "┗", "┛", "━", "━", "┃", "┃"},
	{"dashed", "┌", "┐", "└", "┘", "╌", "╌", "╎", "╎"},
	{"dotted", "·", "·", "·", "·", "·", "·", "·", "·"},
	{"gilt diamonds", "◆", "◆", "◆", "◆", "─◇─", "─◇─", "│", "│"},
	{"stars", "✦", "✦", "✦", "✦", " ✧ ·", " ✧ ·", "·", "·"},
	{"waves", "≈", "≈", "≈", "≈", "∿", "∿", "≀", "≀"},
	{"chain", "○", "○", "○", "○", "─○", "─○", "│o", "│o"},
	{"flowers", "❀", "❀", "❀", "❀", " ✿ ·", " ✿ ·", "✿·", "✿·"},
	{"hearts", "♥", "♥", "♥", "♥", " ♡ ", " ♡ ", "♡ ", "♡ "},
	{"art deco", "◢", "◣", "◥", "◤", "▚▞", "▚▞", "▌", "▐"},
	{"checkered", "▙", "▟", "▛", "▜", "▀▄", "▄▀", "▌", "▐"},
	{"shaded", "▓", "▓", "▓", "▓", "░▒", "▒░", "▒", "▒"},
	{"ember flames", "✹", "✹", "✹", "✹", "^ˆ", "˯v", "⟩", "⟨"},
}

// pageBorder is page p's border (ok false: none).
func (cf *codexFile) pageBorder(p *codexPage) (codexPageEdge, bool) {
	b := cf.PageBorder
	if p != nil && p.Border != 0 {
		b = p.Border
	}
	if b <= 0 || b > len(codexPageBorders) {
		return codexPageEdge{}, false
	}
	return codexPageBorders[b-1], true
}

// pageBorderName says what page p's border is, for the inspector.
func (cf *codexFile) pageBorderName(p *codexPage) string {
	switch {
	case p.Border < 0:
		return "none (this page)"
	case p.Border == 0 && cf.PageBorder == 0:
		return "none"
	case p.Border == 0:
		return codexPageBorders[cf.PageBorder-1].name + " (all pages)"
	}
	return itoa(p.Border) + "/16 " + codexPageBorders[p.Border-1].name
}

// patAt is the glyph of pattern pat at step i.
func patAt(pat string, i int) string {
	rs := []rune(pat)
	if len(rs) == 0 {
		return " "
	}
	return string(rs[(i%len(rs)+len(rs))%len(rs)])
}

// codexDrawPageBorder draws border f round x0…x1, y0…y1 on paper.
func codexDrawPageBorder(cv *boardCanvas, f codexPageEdge, x0, y0, x1, y1 int, paper string) {
	if x1-x0 < 2 || y1-y0 < 2 {
		return
	}
	col := codexAtLeast(mixHex(paper, cAccent, 0.75), paper, 2.5)
	gold := codexAtLeast(codexGold(), paper, 3)
	put := func(x, y int, g, fg string) {
		if g != " " {
			cvSet(cv, x, y, g, fg, cvBG(cv, x, y, paper), 0)
		}
	}
	for x := x0 + 1; x < x1; x++ {
		put(x, y0, patAt(f.top, x-x0-1), col)
		put(x, y1, patAt(f.bottom, x-x0-1), col)
	}
	for y := y0 + 1; y < y1; y++ {
		put(x0, y, patAt(f.left, y-y0-1), col)
		put(x1, y, patAt(f.rgt, y-y0-1), col)
	}
	put(x0, y0, f.tl, gold)
	put(x1, y0, f.tr, gold)
	put(x0, y1, f.bl, gold)
	put(x1, y1, f.br, gold)
}

// ---- header and footer ------------------------------------------------------------------------

// hfWords is a header's or footer's words on page i: {page}, {pages} and
// {title} become the page's number, how many pages, the codex's title.
func (cf *codexFile) hfWords(s string, i int) string {
	r := strings.NewReplacer("{page}", itoa(i+1), "{pages}", itoa(len(cf.Pages)), "{title}", codexName(cf.Title))
	return r.Replace(s)
}

// hasHeader / hasFooter: page p shows a header / footer.
func (c *codexState) hasHeader(p *codexPage) bool {
	return p != nil && !p.Plain && (strings.TrimSpace(c.cf.Header) != "" || c.writing == codexWriteHeader)
}

func (c *codexState) hasFooter(p *codexPage) bool {
	return p != nil && !p.Plain && (strings.TrimSpace(c.cf.Footer) != "" || c.writing == codexWriteFooter)
}

// codexHF draws a header or footer row (what: codexWriteHeader or …Footer)
// in the middle of x … x+w at row y.
func (m *Model) codexHF(cv *boardCanvas, what, x, y, w int, paper, ink string) {
	c := m.cx()
	fg := mixHex(paper, ink, 0.62)
	src := c.cf.Header
	if what == codexWriteFooter {
		src = c.cf.Footer
	}
	if c.writing == what { // being written: the words as typed, with the cursor
		lines := []writeLine{{c.words, 0}}
		lay := codexLay(c.words, codexWriteFont(c.cf.HFFont), 1, w, w, 1)
		if len(lay.lines) > 0 {
			lines = codexLinesAt(lay.lines, c.cur, 1, w)
		}
		codexDrawLines(cv, x, y, w, lines, 1, codexWriteFont(c.cf.HFFont), 0, cText, 0, c.cur, cAccent)
		return
	}
	s := codexFont(c.cf.hfWords(strings.ReplaceAll(src, "\n", " "), c.page), c.cf.HFFont)
	cvMiddleOn(cv, x, y, w, s, fg, uv.AttrItalic)
}

// ---- headings ---------------------------------------------------------------------------------

var codexHeadNames = [4]string{"body text", "heading 1", "heading 2", "heading 3"}

// codexHeadSize is the size a heading level starts at.
func codexHeadSize(level int) int {
	return []int{1, 3, 2, 1}[min(max(level, 0), 3)]
}

// codexHeadRule is the line under a heading of this level.
func codexHeadRule(level int) string {
	return []string{"", "━", "─", "┄"}[min(max(level, 0), 3)]
}

// ---- fills ------------------------------------------------------------------------------------

// codexFills: the 16 looks of a thing's fill (0: none).
var codexFills = [16]string{
	"none", "solid", "soft", "fade →", "fade ↓", "glow", "stripes", "tall stripes",
	"slants", "dots", "checks", "light shade", "shade", "waves", "stars", "vignette",
}

// codexFillColor is e's fill colour.
func codexFillColor(e *codexEl, paper string) string {
	if e.Fill != "" {
		return e.Fill
	}
	return mixHex(paper, cEmber, 0.45)
}

// codexFillApply fills thing e's box r behind what is drawn there: empty
// cells (the paper's colour) take the fill; words on it stay readable.
func codexFillApply(cv *boardCanvas, e *codexEl, r [4]int, paper string, ghost bool) {
	if e.FillBy <= 0 || r[2] <= 0 || r[3] <= 0 {
		return
	}
	col := codexFillColor(e, paper)
	if ghost {
		col = mixHex(col, paper, 0.6)
	}
	soft := mixHex(paper, col, 0.45)
	x0, y0 := r[0], r[1]
	if e.Kind == ceText || e.Kind == ceSymbol { // a little room round words
		x0--
		r[2] += 2
	}
	w, h := r[2], r[3]
	for y := y0; y < y0+h; y++ {
		for x := x0; x < x0+w; x++ {
			if x < 0 || y < 0 || x >= cv.Width() || y >= cv.Height() {
				continue
			}
			cell := cv.Lines[y][x]
			if colorHex(cell.Style.Bg, paper) != paper {
				continue // its own colour (a button, a picture, a border's shadow)
			}
			fx := (float64(x-x0) + 0.5) / float64(w)
			fy := (float64(y-y0) + 0.5) / float64(h)
			bg, g, gfg := col, "", ""
			switch e.FillBy {
			case 2:
				bg = soft
			case 3:
				bg = mixHex(col, paper, fx*0.85)
			case 4:
				bg = mixHex(col, paper, fy*0.85)
			case 5:
				d := (fx-0.5)*(fx-0.5)*4 + (fy-0.5)*(fy-0.5)*4
				bg = mixHex(col, paper, fxClamp01(d)*0.9)
			case 6:
				if (y-y0)%2 == 1 {
					bg = soft
				}
			case 7:
				if (x-x0)/2%2 == 1 {
					bg = soft
				}
			case 8:
				if (x-x0+2*(y-y0))/2%3 == 0 {
					bg = col
				} else {
					bg = soft
				}
			case 9:
				bg, g, gfg = soft, "·", col
				if (x-x0)%2 == 1 {
					g = ""
				}
			case 10:
				if ((x-x0)/2+(y-y0))%2 == 1 {
					bg = soft
				}
			case 11:
				bg, g, gfg = paper, "░", col
			case 12:
				bg, g, gfg = paper, "▒", col
			case 13:
				bg, g, gfg = soft, "∿", mixHex(col, "#ffffff", 0.15)
			case 14:
				bg = soft
				if fxRand(x-x0, y-y0, e.ID) < 0.12 {
					g, gfg = "✧", mixHex(col, "#ffffff", 0.35)
				}
			case 15:
				d := min(min(fx, 1-fx)*2, min(fy, 1-fy)*2)
				bg = mixHex(col, paper, fxClamp01(d*1.4))
			}
			blank := cell.Content == " " || cell.Content == ""
			if blank && cell.Width == 0 {
				continue // the right half of a wide letter
			}
			if blank && g != "" {
				cvSet(cv, x, y, g, gfg, bg, 0)
				continue
			}
			cell.Style.Bg = rgbaOf(bg)
			if !blank && cell.Style.Fg != nil {
				cell.Style.Fg = rgbaOf(codexAtLeast(colorHex(cell.Style.Fg, cText), bg, 3))
			}
			cv.Lines[y][x] = cell
		}
	}
}

// ---- the font list (F) ------------------------------------------------------------------------

const (
	pickThing = 1 + iota // the chosen text or button
	pickTitle            // the cover's title
	pickHF               // the header and footer
)

// codexPick is the open font list: what it sets, the font lit, the font before.
type codexPick struct {
	what, sel, was int
}

func (m *Model) codexPickFont(what int) {
	c := m.cx()
	was := 0
	switch what {
	case pickThing:
		e := c.el(c.sel)
		if e == nil {
			return
		}
		was = e.Font
	case pickTitle:
		was = c.cf.TitleFont
	case pickHF:
		was = c.cf.HFFont
	}
	m.codexStep()
	c.pick = &codexPick{what: what, sel: was, was: was}
	if e := c.el(c.sel); what == pickThing && e != nil && e.Kind == ceText && e.Size > 1 {
		m.say("Fonts · ↑↓ choose · Enter keep · Esc back. Big letters have one shape: − makes the text normal size to see its font.")
		return
	}
	m.say("Fonts · ↑↓ choose (it shows at once) · Enter keep · Esc back.")
}

// pickSet puts font f on what the list is for (it shows at once).
func (c *codexState) pickSet(f int) {
	switch c.pick.what {
	case pickThing:
		if e := c.el(c.sel); e != nil {
			e.Font = f
		}
	case pickTitle:
		c.cf.TitleFont = f
	case pickHF:
		c.cf.HFFont = f
	}
}

func (m *Model) codexPickKey(name string) {
	c := m.cx()
	pk := c.pick
	switch name {
	case "up", "k", "shift+tab":
		pk.sel = codexCycle(pk.sel, 16, false)
	case "down", "j", "tab":
		pk.sel = codexCycle(pk.sel, 16, true)
	case "home", "pgup":
		pk.sel = 0
	case "end", "pgdown":
		pk.sel = 15
	case "enter", "f", "F", "shift+f":
		c.pickSet(pk.sel)
		c.pick = nil
		if pk.sel == pk.was && len(c.undo) > 0 {
			c.undo = c.undo[:len(c.undo)-1] // nothing changed: no undo step
		}
		m.codexSave()
		m.say("Font: " + codexFonts[pk.sel] + ".")
		return
	case "esc":
		c.pickSet(pk.was)
		c.pick = nil
		if len(c.undo) > 0 {
			c.undo = c.undo[:len(c.undo)-1]
		}
		m.say("Font kept as it was: " + codexFonts[pk.was] + ".")
		return
	default:
		if len(name) == 1 && name[0] >= '1' && name[0] <= '9' {
			pk.sel = int(name[0] - '1')
		}
	}
	c.pickSet(pk.sel)
	m.say("Font " + itoa(pk.sel+1) + " of 16: " + codexFonts[pk.sel] + " · Enter keep · Esc back.")
}

// codexPickSample is what the font list shows each font with.
func (c *codexState) codexPickSample() string {
	s := ""
	switch c.pick.what {
	case pickThing:
		if e := c.el(c.sel); e != nil {
			s = e.Text
		}
	case pickTitle:
		s = c.cf.Title
	case pickHF:
		s = c.cf.hfWords(codexOr(c.cf.Header, c.cf.Footer), max(c.page, 0))
	}
	s = strings.TrimSpace(strings.SplitN(s, "\n", 2)[0])
	if s == "" {
		s = "The quick brown fox 123"
	}
	return s
}

// codexPickDraw draws the font list beside what it changes, so that stays in sight.
func (m *Model) codexPickDraw(cv *boardCanvas) {
	c := m.cx()
	pk := c.pick
	sample := c.codexPickSample()
	w := min(max(cv.Width()/3, 36), 52, cv.Width()-2)
	rows := 16
	top := m.codexTop()
	room := cv.Height() - top - 4
	if room < 4 {
		return
	}
	rows = min(rows, room)
	h := rows + 4
	// Beside the page if there is room; else over the page, on the side
	// away from the thing it changes, so that stays in sight.
	pr := c.pageRect
	x := pr[0] + pr[2] + 3
	switch {
	case x+w <= cv.Width():
	case pr[0]-w-3 >= 0:
		x = pr[0] - w - 3
	default:
		x = pr[0] + pr[2] - w - 2
		if r, ok := c.rects[c.sel]; ok && pk.what == pickThing && r[0]+r[2]/2 > pr[0]+pr[2]/2 {
			x = pr[0] + 2
		}
	}
	x = min(max(x, 0), max(cv.Width()-w, 0))
	y := top + max((cv.Height()-top-h)/2, 0)
	bg := mixHex(cBarBG, "#000000", 0.2)
	cvFill(cv, x, y, w, h, bg)
	for xx := x; xx < x+w; xx++ {
		cvSet(cv, xx, y, "━", cAccent, bg, 0)
		cvSet(cv, xx, y+h-1, "━", cAccent, bg, 0)
	}
	title := map[int]string{pickThing: " Font ", pickTitle: " Title font ", pickHF: " Header & footer font "}[pk.what]
	cvMiddle(cv, x, y, w, title, cAccent, bg, uv.AttrBold)
	first := min(max(pk.sel-rows/2, 0), 16-rows)
	for i := 0; i < rows; i++ {
		f := first + i
		ry := y + 2 + i
		lit := f == pk.sel
		rbg := bg
		if lit {
			rbg = mixHex(bg, cAccent, 0.25)
			cvFill(cv, x+1, ry, w-2, 1, rbg)
		}
		mark := "  "
		if lit {
			mark = "▸ "
		}
		name := itoa(f+1) + " " + codexFonts[f]
		if f == 0 && pk.what == pickTitle {
			name = "1 big letters"
		}
		nx := cvText(cv, x+1, ry, mark+name, map[bool]string{true: cAccent, false: mixHex(bg, cText, 0.6)}[lit], rbg, map[bool]uint8{true: uv.AttrBold, false: 0}[lit])
		sx := max(nx+1, x+19)
		cvText(cv, sx, ry, codexClip(codexFont(sample, f), x+w-2-sx), cText, rbg, 0)
	}
	cvMiddle(cv, x, y+h-2, w, "↑↓ choose · Enter keep · Esc back", mixHex(bg, cText, 0.5), bg, uv.AttrItalic)
}
