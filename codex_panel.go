package main

// ---- seeing what can be done: the inspector, the add menu, the marks ------------------------
//
// So every choice can be seen, not only remembered (the user asked, 16 Nov 2026):
//
//   - Editing, a band above the page (the inspector; at the top since 17 Nov 2026,
//     the user asked) shows the page's layout as a little map, and each option of
//     the chosen thing with its key and how it looks now: the font as a sample,
//     the size as bars, the colour as a swatch, the animation moving, the symbol,
//     the alignment, what activating does, a button's action and what it aims at.
//     The band is a fixed size (codexBandRows high, 3 or 4 columns of at most
//     codexColW), centred, so it never jumps when the chosen thing changes.
//   - Alt+N shows the five kinds to add as tiles, each with its key.
//   - Reading, a thing that does something when pressed wears a small ◈; the
//     one Tab reached has corners and "↵ press".
//   - Editing, a button's target is marked "◎ target of …", a thing hidden until
//     activated is dimmed with "hidden until activated".

import (
	"strings"
	"time"

	uv "github.com/charmbracelet/ultraviolet"
)

const (
	codexBandRows  = 7  // rows of options in the inspector band
	codexBandH     = 10 // the band: a rule, the rows, a rule, a gap below
	codexColMinW   = 28 // narrower columns than this: no band
	codexColMaxW   = 44 // wider than this only spreads things out
	codexBandMinW  = 3*codexColMinW + 4
	codexBandMinBH = 26 // board rows needed for the band and a usable page
)

// codexTop is how many rows the inspector band takes at the top (0: none).
func (m *Model) codexTop() int {
	c := m.cx()
	if c.cf == nil || !c.edit || m.width < codexBandMinW+2 || m.boardH() < codexBandMinBH {
		return 0
	}
	return codexBandH
}

// codexBandCols gives the band's columns, their width and the band's width,
// lined up with the page (pw wide) below it: 4 columns when they get room
// for their values, else 3 (the keys, also in the bar below, go first).
func (m *Model) codexBandCols(pw int) (n, cw, bw int) {
	bw = min(max(pw, codexBandMinW), m.width-2)
	n = 4
	if (bw-4)/n < 32 {
		n = 3
	}
	cw = min((bw-4)/n, codexColMaxW)
	if cw < codexColMinW {
		return 0, 0, 0
	}
	bw = n*cw + 4
	if pw > bw && pw-bw < n && pw <= m.width-2 { // what rounding left: the band's edges on the page's
		bw = pw
	}
	return n, cw, bw
}

// keyCap draws key k as a small cap at (x, y); it gives the cell after it.
func keyCap(cv *boardCanvas, x, y int, k, bg string) int {
	capBG := mixHex(bg, cAccent, 0.22)
	x = cvText(cv, x, y, " "+k+" ", cAccent, capBG, uv.AttrBold)
	return x + 1
}

// inspFlow writes inspector rows down a column and on into the next one.
type inspFlow struct {
	cv            *boardCanvas
	x0, y0, cw, n int // the first column's top left, column width, columns
	col, row      int
	bg, dim       string
}

func (f *inspFlow) x() int { return f.x0 + f.col*f.cw }

// next gives the cell to write a row at, false when the band is full.
func (f *inspFlow) next(rows int) (int, int, bool) {
	if f.row+rows > codexBandRows {
		f.col++
		f.row = 0
	}
	if f.col >= f.n {
		return 0, 0, false
	}
	x, y := f.x(), f.y0+f.row
	f.row += rows
	return x, y, true
}

// newCol starts the next column (if this one has anything).
func (f *inspFlow) newCol() {
	if f.row > 0 {
		f.col++
		f.row = 0
	}
}

// head writes a section title with a rule.
func (f *inspFlow) head(s string) {
	if f.row > codexBandRows-2 { // a title needs a row under it
		f.newCol()
	}
	x, y, ok := f.next(1)
	if !ok {
		return
	}
	cvText(f.cv, x, y, codexClip(s, f.cw-3), cEmber, f.bg, uv.AttrBold)
	for xx := x + codexWidth(s) + 1; xx < x+f.cw-2; xx++ {
		cvSet(f.cv, xx, y, "─", mixHex(f.bg, cEmber, 0.25), f.bg, 0)
	}
}

// start writes a row's key and label; it gives where the value goes and its room.
func (f *inspFlow) start(k, label string) (int, int, int, bool) {
	x, y, ok := f.next(1)
	if !ok {
		return 0, 0, 0, false
	}
	if k != "" {
		keyCap(f.cv, x, y, k, f.bg)
	}
	lx := x + max(codexWidth(k)+3, 5)
	if k == "" {
		lx = x + 5
	}
	cx := cvText(f.cv, lx, y, codexClip(label, 9), f.dim, f.bg, 0)
	vx := max(cx+1, x+15)
	return vx, y, x + f.cw - 2 - vx, true
}

// line writes a row: key, label, value.
func (f *inspFlow) line(k, label, value, fg string) {
	if vx, y, room, ok := f.start(k, label); ok {
		cvText(f.cv, vx, y, codexClip(value, room), fg, f.bg, 0)
	}
}

// swatch writes a row with a colour swatch.
func (f *inspFlow) swatch(k, label, col, name string) {
	vx, y, room, ok := f.start(k, label)
	if !ok {
		return
	}
	frame := mixHex(f.bg, cText, 0.45) // a thin frame, so a colour like the band's still shows
	cvSet(f.cv, vx, y, "▕", frame, f.bg, 0)
	cx := cvText(f.cv, vx+1, y, "████", col, f.bg, 0)
	cvSet(f.cv, cx, y, "▏", frame, f.bg, 0)
	cvText(f.cv, cx+2, y, codexClip(name, room-8), cText, f.bg, 0)
}

// anim writes a row with the animation playing on a row of dots.
func (f *inspFlow) anim(anim, seed int, now time.Time) {
	vx, y, room, ok := f.start("A", "animation")
	if !ok {
		return
	}
	const n = 6
	t := float64(now.UnixMilli()%60000) / 1000
	if !codexAnimLoops(anim) {
		t = float64(now.UnixMilli()%3000) / 1000 // a settling one plays again every 3 s
	}
	for i := 0; i < n; i++ {
		p := codexPose(anim, i, 0, n, 1, t, seed)
		if p.Hide {
			continue
		}
		tx := i + p.DX
		if anim == codexMarquee {
			tx = ((tx % n) + n) % n
		}
		if tx < 0 || tx >= n {
			continue
		}
		g, fg := "●", cAccent
		if p.Glyph != "" {
			g = p.Glyph
		}
		if p.Tint != "" {
			fg = mixHex(fg, p.Tint, p.TintAmt)
		}
		fg = mixHex(fg, f.bg, p.Fade)
		cvSet(f.cv, vx+tx, y, g, fg, f.bg, 0)
	}
	cvText(f.cv, vx+n+1, y, codexClip(codexAnims[anim%16], room-n-1), cText, f.bg, 0)
}

// codexInspector draws the inspector band at the top of the w-wide canvas,
// as wide as the pw-wide page below it.
func (m *Model) codexInspector(cv *boardCanvas, w, pw int, now time.Time) {
	c := m.cx()
	n, cw, bw := m.codexBandCols(pw)
	if n == 0 {
		return
	}
	x, y := (w-bw)/2, 0
	h := codexBandRows + 2
	bg := mixHex(cBarBG, "#000000", 0.1)
	edge := mixHex(bg, cEmber, 0.45)
	dim := mixHex(bg, cText, 0.55)
	cvFill(cv, x, y, bw, h, bg)
	for xx := x; xx < x+bw; xx++ {
		cvSet(cv, xx, y, "━", edge, bg, 0)
		cvSet(cv, xx, y+h-1, "━", edge, bg, 0)
	}
	cvMiddle(cv, x, y, bw, " ✦ Inspector ", cAccent, bg, uv.AttrBold)
	for i := 1; i < n; i++ { // faint lines between the columns
		sx := x + 2 + i*cw - 2
		for yy := y + 1; yy < y+h-1; yy++ {
			cvSet(cv, sx, yy, "│", mixHex(bg, cText, 0.15), bg, 0)
		}
	}
	f := &inspFlow{cv: cv, x0: x + 2, y0: y + 1, cw: cw, n: n, bg: bg, dim: dim}
	keys := func(rows [][2]string) {
		if f.col >= n-1 && f.row > 0 { // only into a fresh last column
			return
		}
		f.newCol()
		f.head("Keys")
		for _, r := range rows {
			f.line(r[0], r[1], "", cText)
		}
	}
	if c.page < 0 { // the cover
		f.head("The cover")
		f.line("↵", "title", codexName(c.cf.Title), cText)
		f.line("Tab", "subtitle", codexOr(c.cf.Subtitle, "—"), cText)
		f.swatch("C", "colour", codexOr(c.cf.Color, codexThemeCover()), codexOr(c.cf.Color, "theme"))
		sym := codexSymbols[(c.cf.Symbol%len(codexSymbols)+len(codexSymbols))%len(codexSymbols)]
		f.line("S", "symbol", codexSymbolFrame(c.cf.Symbol, now.Sub(c.pageAt).Seconds())+"  "+sym.Name, cAccent)
		f.swatch("W", "paper", c.paper(nil), codexOr(c.cf.Paper, "theme"))
		f.newCol()
		f.head("Pages")
		f.line("PgDn", "pages", plural(len(c.cf.Pages), "page"), cText)
		f.line("O", "pages lie", map[bool]string{false: "wide ▭", true: "upright ▯"}[c.cf.Tall], cText)
		keys([][2]string{{"Alt+P", "add a page"}, {"E", "read it"}})
		return
	}
	p := c.pageOf()
	e := c.el(c.sel)
	f.head("Page " + itoa(c.page+1) + " of " + itoa(len(c.cf.Pages)))
	f.line("L", "layout", itoa(p.Layout+1)+"/16 "+codexLayouts[p.Layout].name, cText)
	// The layout as a little map: the chosen thing's slot lit.
	if mx, my, ok := f.next(4); ok {
		lit := -1
		if e != nil {
			lit = codexSlotOf(e, p.Layout)
		}
		mw, mh := cw-4, 4
		paper := c.paper(p)
		cvFill(cv, mx, my, mw, mh, paper)
		for si, s := range codexSlotRects(p.Layout, mw-2, mh) {
			sx, sy, sw, sh := mx+1+s[0], my+s[1], max(s[2]-1, 1), max(s[3], 1)
			fill := mixHex(paper, cEmber, 0.25)
			if si == lit {
				fill = mixHex(paper, cAccent, 0.7)
			}
			cvFill(cv, sx, sy, sw, sh, fill)
			k := len(elsIn(p, si))
			label := itoa(si + 1)
			if k > 0 {
				label += "·" + itoa(k)
			}
			cvMiddle(cv, sx, sy+sh/2, sw, label, codexInkOn(fill), fill, uv.AttrBold)
		}
	}
	f.swatch("W", "paper", c.paper(p), codexOr(p.Paper, "theme"))
	f.newCol()
	if e == nil {
		f.head("Nothing chosen")
		f.line("Alt+N", "add", "a thing here", cText)
		f.line("←→↑↓", "choose", "a thing", cText)
		keys([][2]string{{"Alt+N", "add"}, {"Alt+P", "page"}, {"E", "read"}})
		return
	}
	f.head(codexKindIcons[e.Kind] + " " + strings.ToUpper(codexKindNames[e.Kind][:1]) + codexKindNames[e.Kind][1:])
	switch e.Kind {
	case ceText, ceButton:
		f.line("↵", map[bool]string{true: "label", false: "words"}[e.Kind == ceButton], codexOr(strings.ReplaceAll(e.Text, "\n", " "), "—"), cText)
		f.line("F", "font", codexFont("Aa Bb 12", e.Font)+"  "+codexFonts[e.Font%16], cText)
	case ceImage:
		f.line("↵", "picture", codexOr(e.Image, "choose a file"), cText)
	case ceDrawing:
		f.line("↵", "draw", plural(len(e.Ink), "dot"), cText)
	case ceSymbol:
		sym := codexSymbols[(e.Symbol%len(codexSymbols)+len(codexSymbols))%len(codexSymbols)]
		f.line("S", "symbol", codexSymbolFrame(e.Symbol, now.Sub(c.pageAt).Seconds())+"  "+sym.Name, codexOr(e.Color, codexSymColor(e.Symbol)))
	}
	if e.Kind == ceText || e.Kind == ceSymbol {
		k := max(e.Size, 1)
		f.line("+ −", "size", strings.Repeat("▮", k)+strings.Repeat("▯", codexSizes-k)+" "+itoa(k)+"/"+itoa(codexSizes), cAccent)
	}
	def := codexInkOn(c.paper(p))
	switch e.Kind {
	case ceButton:
		def = codexThemeButton()
	case ceSymbol:
		def = codexSymColor(e.Symbol)
	}
	f.swatch("C", "colour", codexOr(e.Color, def), codexOr(e.Color, "theme"))
	f.anim(e.Anim, e.ID, now)
	f.line("J", "line up", []string{"◫ middle", "◧ left", "◨ right"}[e.Align%3], cText)
	f.line("Alt+←→", "slot", itoa(codexSlotOf(e, p.Layout)+1)+" of "+itoa(len(codexLayouts[p.Layout].slots)), cText)
	if e.Kind == ceButton {
		f.line("K", "action", codexActionNames[codexOr(e.Action, caNext)], cAccent)
		if e.Action == caGoto || e.Action == caActivate {
			f.line("G", "aims at", strings.TrimPrefix(strings.TrimPrefix(codexButtonSays(c, e), "go to "), "activate "), cAccent)
		}
	}
	_, do := codexDoOf(e.On.Do)
	f.line("T", "activated", do.name, cAccent)
	if do.value != "" {
		f.line("", "  to", codexTableValue(c, e), cText)
	}
	keys([][2]string{{"Space", "try it"}, {"X X", "delete"}, {"Alt+N", "add"}, {"Alt+P", "page"}, {"E", "read"}})
}

// ---- Alt+N: the add menu ----------------------------------------------------------------------

// codexAddMenu draws the five kinds to add, as tiles, over the page.
func (m *Model) codexAddMenu(cv *boardCanvas, now time.Time) {
	kinds := []struct{ key, kind, look string }{
		{"T", ceText, "Aa"}, {"B", ceButton, "╭──╮"}, {"I", ceImage, "▨▨"}, {"S", ceSymbol, ""}, {"D", ceDrawing, "✎~"},
	}
	tw, th := 14, 6
	w := len(kinds)*(tw+1) + 3
	h := th + 4
	top := m.codexTop()
	x, y := (cv.Width()-w)/2, top+(cv.Height()-top-h)/2
	bg := mixHex(cBarBG, "#000000", 0.2)
	cvFill(cv, x, y, w, h, bg)
	for xx := x; xx < x+w; xx++ {
		cvSet(cv, xx, y, "━", cAccent, bg, 0)
		cvSet(cv, xx, y+h-1, "━", cAccent, bg, 0)
	}
	cvMiddle(cv, x, y, w, " Add to this page ", cAccent, bg, uv.AttrBold)
	for i, k := range kinds {
		tx, ty := x+2+i*(tw+1), y+2
		tile := mixHex(bg, cEmber, 0.15)
		cvFill(cv, tx, ty, tw, th, tile)
		look := k.look
		if k.kind == ceSymbol {
			look = codexSymbolFrame(3, float64(now.UnixMilli()%60000)/1000)
		}
		cvMiddle(cv, tx, ty+1, tw, look, cAccent, tile, uv.AttrBold)
		cvMiddle(cv, tx, ty+3, tw, codexKindNames[k.kind], cText, tile, 0)
		kx := tx + (tw-3)/2
		keyCap(cv, kx, ty+5, k.key, tile)
	}
	cvMiddle(cv, x, y+h-2, w, "press a key · any other key: nothing", mixHex(bg, cText, 0.5), bg, uv.AttrItalic)
}

// ---- marks on the page --------------------------------------------------------------------------

// codexMarks draws what helps see a page: pressable things while reading,
// a button's target and things hidden until activated while editing. Its
// tags go on empty paper next to a thing (below, above or beside it), kept
// inside the page's writing area.
func (m *Model) codexMarks(cv *boardCanvas, p *codexPage, paper string, now time.Time) {
	c := m.cx()
	if c.drawing {
		return
	}
	in := c.inner
	left, right := in[0]-1, in[0]+in[2]+1 // the columns a tag may use
	top, bottom := in[1], in[1]+in[3]     // the rows (the empty row under the area too)
	tag := func(x, y int, s, fg string) {
		fg = codexAtLeast(fg, paper, 3)
		bg := mixHex(paper, fg, 0.18)
		cvText(cv, x, y, " "+s+" ", fg, bg, uv.AttrBold)
	}
	// spot is a place to try for a tag: clamp moves it sideways into the area.
	type spot struct {
		x, y  int
		clamp bool
	}
	// place puts tag s at the first spot that is inside the area and on
	// empty paper; with none, at the last spot, kept inside the area.
	place := func(s, fg string, spots ...spot) {
		tw := codexWidth(s) + 2
		fit := func(q spot) (int, bool) {
			x := q.x
			if q.clamp {
				x = min(max(x, left), right-tw)
			}
			return x, x >= left && x+tw <= right && q.y >= top && q.y <= bottom
		}
		for _, q := range spots {
			if x, ok := fit(q); ok && codexBlank(cv, x, q.y, tw) {
				tag(x, q.y, s, fg)
				return
			}
		}
		q := spots[len(spots)-1]
		x := min(max(q.x, left), right-tw)
		tag(x, min(max(q.y, top), bottom), s, fg)
	}
	// Below the thing, then above it, then (forced) on its last row.
	near := func(r [4]int, s, fg string) {
		place(s, fg, spot{r[0], r[1] + r[3], true}, spot{r[0], r[1] - 1, true}, spot{r[0], r[1] + r[3] - 1, true})
	}
	if c.writing != 0 {
		if r, ok := c.rects[c.sel]; ok && c.writing == codexWriteAlt {
			near(r, "the other words · Esc: done", cAccent)
		}
		return
	}
	if !c.edit {
		for _, e := range c.interactive() {
			r, ok := c.rects[e.ID]
			if !ok || c.look(e, now).hidden {
				continue
			}
			if e.ID == c.focus {
				s := "↵ press"
				tw := codexWidth(s) + 2
				x, dy := r[0]+r[2]-tw, 0
				if tw > r[2] { // wider than the thing: under its corner marks
					x, dy = r[0]+r[2]-tw+1, 1
				}
				place(s, cAccent, spot{x, r[1] + r[3] + dy, true}, spot{x, r[1] + r[3], true}, spot{x, r[1] - 1, true},
					spot{x, r[1] + r[3], true})
				continue
			}
			if e.Kind != ceButton { // a button looks pressable already
				col := codexAtLeast(mixHex(paper, cAccent, 0.75), paper, 3)
				x := r[0] + r[2] + 1
				if x >= right || !codexBlank(cv, x, r[1], 1) {
					x = r[0] - 2
				}
				if x < left || !codexBlank(cv, x, r[1], 1) {
					x = min(r[0]+r[2], right-1)
				}
				cvSet(cv, x, r[1], "◈", col, cvBG(cv, x, r[1], paper), uv.AttrBold)
			}
		}
		return
	}
	for _, e := range p.Els {
		r, ok := c.rects[e.ID]
		if !ok {
			continue
		}
		if e.On.Do == "show" {
			near(r, "hidden until activated", mixHex(paper, codexInkOn(paper), 0.6))
		}
	}
	e := c.el(c.sel)
	if e == nil || e.Kind != ceButton {
		return
	}
	r := c.rects[e.ID]
	// A button's tag: beside it (right, then left), else below or above.
	side := func(s string) {
		tw := codexWidth(s) + 2
		mid := r[1] + r[3]/2 // the label's row
		place(s, cEmber, spot{r[0] + r[2] + 1, mid, false}, spot{r[0] - tw - 1, mid, false},
			spot{r[0], r[1] + r[3], true}, spot{r[0], r[1] - 1, true}, spot{r[0], r[1] + r[3], true})
	}
	switch e.Action {
	case caActivate:
		t, pi := c.find(e.Target)
		if t == nil {
			side("→ G: aim it")
			return
		}
		if pi != c.page {
			side("→ page " + itoa(pi+1))
			return
		}
		if tr, ok := c.rects[t.ID]; ok {
			col := codexAtLeast(cEmber, paper, 3)
			for _, q := range []struct {
				x, y int
				g    string
			}{{tr[0] - 1, tr[1] - 1, "╭"}, {tr[0] + tr[2], tr[1] - 1, "╮"}, {tr[0] - 1, tr[1] + tr[3], "╰"}, {tr[0] + tr[2], tr[1] + tr[3], "╯"}} {
				if codexBlank(cv, q.x, q.y, 1) && q.y >= top && q.y <= bottom {
					cvSet(cv, q.x, q.y, q.g, col, cvBG(cv, q.x, q.y, paper), uv.AttrBold)
				}
			}
			place("◎ target", cEmber, spot{tr[0], tr[1] - 1, true}, spot{tr[0], tr[1] + tr[3], true}, spot{tr[0], tr[1] - 1, true})
			side("→ ◎")
		}
	case caGoto:
		to := "→ the cover"
		if e.Target >= 1 {
			to = "→ page " + itoa(e.Target)
		}
		side(to)
	case caPrev:
		side("← page before")
	default:
		side("→ next page")
	}
}
