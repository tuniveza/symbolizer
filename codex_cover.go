package main

// Editing the cover (the user asked, 19 Nov 2026: "the cover of the codex can
// be edited including size, etc").
//
// In edit mode on the cover, ↑ and ↓ choose a part: the whole cover, the
// symbol, the title, the subtitle or the page count. Then:
//
//	+ −          the whole cover: its size on the screen; a part: its letter size
//	drag ◢       the same with the mouse (the chosen part's corner)
//	C            the whole cover: the leather; a part: its colour (first: gilt)
//	F            the title's, the subtitle's or the page count's font
//	Alt+← →      line it up: left, middle, right (the whole cover: all of them)
//	Alt+↑ ↓      a part: move it before or after the others (the page count:
//	             top or bottom of the frame); the whole cover: the words go to
//	             the top, the middle or the bottom
//	X            hide the part (X again shows it)
//	R            the part (or the cover's size) back as it came
//
// A click on a part chooses it; a click elsewhere on the cover chooses the
// whole cover. The parts show the same on the Design board and in the library.

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// The cover's parts.
const (
	cpWhole = iota
	cpSymbol
	cpTitle
	cpSubtitle
	cpCount
)

var codexCoverPartNames = []string{"the whole cover", "the symbol", "the title", "the subtitle", "the page count"}

// codexCoverPart is how one part of the cover looks.
type codexCoverPart struct {
	Size  int    `json:"size,omitempty"`  // 0 as it comes, else 1 … codexSizes
	Color string `json:"color,omitempty"` // "" gilt
	Font  int    `json:"font,omitempty"`  // the subtitle's and page count's font (the title keeps TitleFont)
	Align int    `json:"align,omitempty"` // 0 the middle, 1 left, 2 right
	Hide  bool   `json:"hide,omitempty"`
	Top   bool   `json:"top,omitempty"` // the page count: on the top of the frame
}

// codexCoverInfo is where the cover's parts were drawn (on the cover) and
// at which size.
type codexCoverInfo struct {
	rects [5][4]int
	sizes [5]int
}

// partOf is part p as it is (nothing changes).
func (cf *codexFile) partOf(p int) codexCoverPart {
	if p >= cpSymbol && p <= len(cf.CoverParts) {
		return cf.CoverParts[p-1]
	}
	return codexCoverPart{}
}

// part is part p, to change.
func (cf *codexFile) part(p int) *codexCoverPart {
	for len(cf.CoverParts) < cpCount {
		cf.CoverParts = append(cf.CoverParts, codexCoverPart{})
	}
	p = min(max(p, cpSymbol), cpCount)
	return &cf.CoverParts[p-1]
}

// coverOrder is the order of the symbol, the title and the subtitle.
func (cf *codexFile) coverOrder() []int {
	if len(cf.CoverOrder) == 3 {
		seen := map[int]bool{}
		for _, p := range cf.CoverOrder {
			if p >= cpSymbol && p <= cpSubtitle {
				seen[p] = true
			}
		}
		if len(seen) == 3 {
			return append([]int(nil), cf.CoverOrder...)
		}
	}
	return []int{cpSymbol, cpTitle, cpSubtitle}
}

// tidyCover brings the cover's settings into range (codexFile.tidy).
func (cf *codexFile) tidyCover() {
	fix := func(v int) int {
		if v <= 0 || v >= 100 {
			return 0
		}
		return max(v, 30)
	}
	cf.CoverW, cf.CoverH = fix(cf.CoverW), fix(cf.CoverH)
	cf.CoverV = min(max(cf.CoverV, 0), 2)
	if len(cf.CoverOrder) > 0 {
		cf.CoverOrder = cf.coverOrder()
		if cf.CoverOrder[0] == cpSymbol && cf.CoverOrder[1] == cpTitle {
			cf.CoverOrder = nil // as it comes
		}
	}
	if len(cf.CoverParts) > cpCount {
		cf.CoverParts = cf.CoverParts[:cpCount]
	}
	empty := true
	for i := range cf.CoverParts {
		p := &cf.CoverParts[i]
		p.Size = min(max(p.Size, 0), codexSizes)
		p.Font = (p.Font%16 + 16) % 16
		p.Align = min(max(p.Align, 0), 2)
		if *p != (codexCoverPart{}) {
			empty = false
		}
	}
	if empty {
		cf.CoverParts = nil
	}
}

// coverRect is where the cover goes in a page of pw×ph at (px, py): its own
// size, in the middle.
func (cf *codexFile) coverRect(px, py, pw, ph int) [4]int {
	w, h := pw, ph
	if cf.CoverW > 0 {
		w = max(pw*cf.CoverW/100, 12)
	}
	if cf.CoverH > 0 {
		h = max(ph*cf.CoverH/100, 5)
	}
	w, h = min(w, pw), min(h, ph)
	return [4]int{px + (pw-w)/2, py + (ph-h)/2, w, h}
}

// cvOn writes s from x, on whatever is there (spaces let it show through).
func cvOn(cv *boardCanvas, x, y int, s, fg string, attrs uint8) {
	for _, r := range s {
		g := string(r)
		if g != " " {
			cvSet(cv, x, y, g, fg, cvBG(cv, x, y, cBG), attrs)
		}
		x += max(codexWidth(g), 1)
	}
}

// codexLinesW is how wide the widest of lines is.
func codexLinesW(lines []writeLine, font, size int) int {
	w := 0
	for _, ln := range lines {
		w = max(w, codexMeasure(trimSpaces(ln.s), font, size))
	}
	return w
}

// codexCoverWords draws the symbol, the title, the subtitle and the page
// count inside the cover's frame (x0, y0)–(x1, y1), and notes where they went.
func codexCoverWords(cf *codexFile, cv *boardCanvas, info *codexCoverInfo, x0, y0, x1, y1 int, col, gold string, t float64, what int, words []rune, cur int) {
	h := cv.Height()
	iw := x1 - x0 - 3 // room inside the frame
	ix := x0 + 2
	room := y1 - y0 - 1
	var parts [5]codexCoverPart
	for p := cpSymbol; p <= cpCount; p++ {
		parts[p] = cf.partOf(p)
	}

	// The symbol: bigger on a taller cover, or the size chosen.
	var sym []string
	symSize := 1
	if !parts[cpSymbol].Hide {
		switch {
		case parts[cpSymbol].Size > 0:
			symSize = parts[cpSymbol].Size
		case h >= 26:
			symSize = 3
		case h >= 15:
			symSize = 2
		}
		sym = codexSymbolBig(cf.Symbol, t, symSize)
		for symSize > 1 && len(sym) > room { // too big for this cover
			symSize--
			sym = codexSymbolBig(cf.Symbol, t, symSize)
		}
	}

	// The subtitle.
	subWords := []rune(strings.TrimSpace(cf.Subtitle))
	subCur := -1
	if what == codexWriteSubtitle {
		subWords, subCur = words, cur
	}
	sfont := parts[cpSubtitle].Font
	if what == codexWriteSubtitle {
		sfont = codexWriteFont(sfont)
	}
	sl := codexLay(subWords, sfont, max(parts[cpSubtitle].Size, 1), iw, iw, room)
	if sl.size == 1 {
		sl.lines = codexLinesAt(sl.lines, subCur, 2, iw)
	}
	subRows := sl.rows()
	if (len(subWords) == 0 || parts[cpSubtitle].Hide) && subCur < 0 {
		subRows = 0
	}

	// The title: in big letters when they fit (one line, or two on a tall
	// cover), else up to two lines of normal letters; or the size chosen.
	title := strings.TrimSpace(cf.Title)
	if title == "" {
		title = "Untitled"
	}
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
	switch {
	case parts[cpTitle].Size > 0: // as big as chosen, in the room the subtitle leaves
		tsize, tmax = parts[cpTitle].Size, room
		if subRows > 0 {
			tmax = max(room-subRows-1, 1)
		}
	case h >= 13: // big letters, in the title's font (codex_bigfont.go)
		tsize, tmax = 2, 3
		if h >= 22 {
			tmax = 6
		}
	}
	tl := codexLay(titleWords, tfont, tsize, iw, iw, tmax)
	if tl.size == 1 {
		n := 2
		if parts[cpTitle].Size == 1 { // chosen small: more lines may show
			n = max(2, min(room-2, 4))
		}
		tl.lines = codexLinesAt(tl.lines, titleCur, n, iw)
	}
	titleRows := tl.rows()
	if parts[cpTitle].Hide && titleCur < 0 {
		titleRows = 0
	}

	rows := map[int]int{cpSymbol: len(sym), cpTitle: titleRows, cpSubtitle: subRows}
	order := cf.coverOrder()
	block := func() int {
		n, shown := 0, 0
		for _, p := range order {
			if rows[p] > 0 {
				n += rows[p]
				shown++
			}
		}
		return n + max(shown-1, 0)
	}
	if block() > room { // no room for the symbol
		sym, rows[cpSymbol] = nil, 0
	}
	if block() > room { // nor for the subtitle
		rows[cpSubtitle] = 0
	}
	off := max((room-block())/2, 0)
	switch cf.CoverV {
	case 1:
		off = 0
	case 2:
		off = max(room-block(), 0)
	}
	y := y0 + 1 + off
	first := true
	for _, p := range order {
		if rows[p] == 0 {
			continue
		}
		if !first {
			y++
		}
		first = false
		pt := parts[p]
		switch p {
		case cpSymbol:
			sw := 0
			for _, r := range sym {
				sw = max(sw, codexWidth(r))
			}
			sx := ix + codexAlign(pt.Align, iw, sw)
			fg := codexOr(pt.Color, mixHex(codexSymColor(cf.Symbol), gold, 0.4))
			for i, r := range sym {
				cvOn(cv, sx+(sw-codexWidth(r))/2, y+i, r, fg, uv.AttrBold)
			}
			info.rects[p], info.sizes[p] = [4]int{sx, y, sw, len(sym)}, symSize
		case cpTitle:
			lw := codexLinesW(tl.lines, tfont, tl.size)
			codexDrawLines(cv, ix, y, iw, tl.lines, tl.size, tfont, pt.Align, codexOr(pt.Color, gold), uv.AttrBold, titleCur, gold)
			info.rects[p], info.sizes[p] = [4]int{ix + codexAlign(pt.Align, iw, lw), y, max(lw, 1), titleRows}, tl.size
		case cpSubtitle:
			lw := codexLinesW(sl.lines, sfont, sl.size)
			codexDrawLines(cv, ix, y, iw, sl.lines, sl.size, sfont, pt.Align, codexOr(pt.Color, mixHex(gold, col, 0.35)), uv.AttrItalic, subCur, gold)
			info.rects[p], info.sizes[p] = [4]int{ix + codexAlign(pt.Align, iw, lw), y, max(lw, 1), subRows}, sl.size
		}
		y += rows[p]
	}

	// The page count, on the frame.
	if pt := parts[cpCount]; !pt.Hide && h >= 9 {
		s := codexClip(codexFont(" "+plural(len(cf.Pages), "page")+" ", pt.Font), iw)
		sw := codexWidth(s)
		cy := y1
		if pt.Top {
			cy = y0
		}
		sx := ix + codexAlign(pt.Align, iw, sw)
		cvText(cv, sx, cy, s, codexOr(pt.Color, mixHex(gold, col, 0.3)), cvBG(cv, sx, cy, col), 0) // spaces too: the frame stops either side
		info.rects[cpCount], info.sizes[cpCount] = [4]int{sx, cy, sw, 1}, 1
	}
}

// ---- in Codex mode ----------------------------------------------------------------------------

// codexCoverSeq is the order ↑ ↓ go through the parts: the whole cover,
// then the parts top to bottom.
func (cf *codexFile) codexCoverSeq() []int {
	seq := []int{cpWhole}
	if cf.partOf(cpCount).Top {
		seq = append(seq, cpCount)
	}
	seq = append(seq, cf.coverOrder()...)
	if !cf.partOf(cpCount).Top {
		seq = append(seq, cpCount)
	}
	return seq
}

// codexCoverPartName says part p, and "(hidden)" when it is.
func (cf *codexFile) codexCoverPartName(p int) string {
	n := codexCoverPartNames[min(max(p, 0), cpCount)]
	if p > cpWhole && cf.partOf(p).Hide {
		n += " (hidden)"
	}
	return n
}

// codexCoverSays says the chosen part's size, place and look.
func (m *Model) codexCoverSays() string {
	c := m.cx()
	cf := c.cf
	p := c.coverPart
	if p == cpWhole {
		return "The whole cover: " + codexCoverSizeName(cf) + "."
	}
	pt := cf.partOf(p)
	s := strings.ToUpper(codexCoverPartNames[p][:1]) + codexCoverPartNames[p][1:] + ": "
	if pt.Hide {
		return s + "hidden. X shows it."
	}
	switch {
	case p == cpCount:
		s += "on the " + map[bool]string{true: "top", false: "bottom"}[pt.Top] + " of the frame"
	case pt.Size > 0:
		s += "size " + itoa(pt.Size) + " of " + itoa(codexSizes)
	default:
		s += "size as it comes (" + itoa(max(c.cinfo().sizes[p], 1)) + ")"
	}
	return s + ", " + []string{"in the middle", "on the left", "on the right"}[pt.Align] + "."
}

// codexCoverSizeName says the cover's size on the screen.
func codexCoverSizeName(cf *codexFile) string {
	if cf.CoverW == 0 && cf.CoverH == 0 {
		return "as big as the page"
	}
	pc := func(v int) string {
		if v == 0 {
			return "100%"
		}
		return itoa(v) + "%"
	}
	return "width " + pc(cf.CoverW) + " · height " + pc(cf.CoverH) + " of the page"
}

// codexCoverEditKey handles the keys for the cover's parts; false: not one of them.
func (m *Model) codexCoverEditKey(name string) bool {
	c := m.cx()
	cf := c.cf
	p := c.coverPart
	switch name {
	case "up", "down":
		seq := cf.codexCoverSeq()
		at := 0
		for i, q := range seq {
			if q == p {
				at = i
			}
		}
		c.coverPart = seq[codexCycle(at, len(seq), name == "down")]
		m.say(strings.ToUpper(cf.codexCoverPartName(c.coverPart)[:1]) + cf.codexCoverPartName(c.coverPart)[1:] +
			" · + − size · C colour · Alt+arrows move · X hide · R as it was · ↑↓ another part")
	case "+", "=", "plus", "shift+=", "kp+", "-", "_", "minus", "kp-":
		m.codexCoverSize(name == "+" || name == "=" || name == "plus" || name == "shift+=" || name == "kp+")
	case "alt+left", "alt+right":
		m.codexStep()
		pos := codexAlignPos(cf.partOf(max(p, cpTitle)).Align)
		if name == "alt+left" {
			pos = max(pos-1, 0)
		} else {
			pos = min(pos+1, 2)
		}
		a := codexAlignOfPos(pos)
		if p == cpWhole {
			for q := cpSymbol; q <= cpCount; q++ {
				cf.part(q).Align = a
			}
		} else {
			cf.part(p).Align = a
		}
		cf.tidyCover()
		m.codexSave()
		m.say(strings.ToUpper(codexCoverPartNames[p][:1]) + codexCoverPartNames[p][1:] + ": " +
			[]string{"in the middle", "on the left", "on the right"}[a] + ". Alt+← → line it up.")
	case "alt+up", "alt+down":
		up := name == "alt+up"
		m.codexStep()
		switch p {
		case cpWhole:
			pos := []int{1, 0, 2}[cf.CoverV] // top 0, middle 1, bottom 2
			if up {
				pos = max(pos-1, 0)
			} else {
				pos = min(pos+1, 2)
			}
			cf.CoverV = []int{1, 0, 2}[pos]
			m.say("The words sit at the " + []string{"middle", "top", "bottom"}[cf.CoverV] + " of the cover. Alt+↑ ↓ move them.")
		case cpCount:
			cf.part(cpCount).Top = up
			m.say("The page count is on the " + map[bool]string{true: "top", false: "bottom"}[up] + " of the frame.")
		default:
			order := cf.coverOrder()
			at := 0
			for i, q := range order {
				if q == p {
					at = i
				}
			}
			to := at + 1
			if up {
				to = at - 1
			}
			if to < 0 || to > 2 {
				m.say(map[bool]string{true: "It is first already.", false: "It is last already."}[up])
				c.undo = c.undo[:len(c.undo)-1]
				return true
			}
			order[at], order[to] = order[to], order[at]
			cf.CoverOrder = order
			m.say(strings.ToUpper(codexCoverPartNames[p][:1]) + codexCoverPartNames[p][1:] + " moved " +
				map[bool]string{true: "up", false: "down"}[up] + ". Alt+↑ ↓ move it.")
		}
		cf.tidyCover()
		m.codexSave()
	case "x", "X", "shift+x", "delete", "backspace":
		if p == cpWhole {
			m.say("X hides a part of the cover: ↑↓ choose one first.")
			return true
		}
		m.codexStep()
		pt := cf.part(p)
		pt.Hide = !pt.Hide
		cf.tidyCover()
		m.codexSave()
		if pt.Hide {
			m.say(strings.ToUpper(codexCoverPartNames[p][:1]) + codexCoverPartNames[p][1:] + " is hidden. X shows it again.")
		} else {
			m.say(strings.ToUpper(codexCoverPartNames[p][:1]) + codexCoverPartNames[p][1:] + " shows again.")
		}
	case "r", "R", "shift+r":
		m.codexStep()
		if p == cpWhole {
			cf.CoverW, cf.CoverH, cf.CoverV = 0, 0, 0
			m.say("The cover is as big as the page again, the words in the middle.")
		} else {
			*cf.part(p) = codexCoverPart{}
			if p != cpCount {
				cf.CoverOrder = nil
			}
			m.say(strings.ToUpper(codexCoverPartNames[p][:1]) + codexCoverPartNames[p][1:] + " is as it came.")
		}
		cf.tidyCover()
		m.codexSave()
	case "c", "C", "shift+c":
		if p == cpWhole {
			return false // the leather (codexCoverKey)
		}
		m.codexStep()
		pt := cf.part(p)
		pt.Color = codexNext(append([]string{""}, codexPalette...), pt.Color, name == "c")
		cf.tidyCover()
		m.codexSave()
		m.say(strings.ToUpper(codexCoverPartNames[p][:1]) + codexCoverPartNames[p][1:] + ": " + codexOr(pt.Color, "gilt") + ". C next · Shift+C back.")
	case "f", "F", "shift+f":
		switch p {
		case cpSymbol:
			m.say("A symbol has no font. S chooses another symbol.")
		case cpSubtitle:
			m.codexPickFont(pickSub)
		case cpCount:
			m.codexPickFont(pickCount)
		default:
			return false // the title font
		}
	default:
		return false
	}
	return true
}

// codexCoverSize: + (grow) or − on the chosen part, or on the whole cover.
func (m *Model) codexCoverSize(grow bool) {
	c := m.cx()
	cf := c.cf
	p := c.coverPart
	step := map[bool]int{true: 1, false: -1}[grow]
	switch p {
	case cpWhole:
		w, h := cf.CoverW, cf.CoverH
		nw, nh := codexCoverStep(w, step), codexCoverStep(h, step)
		if nw == w && nh == h {
			m.say(map[bool]string{true: "As big as the page.", false: "As small as it goes."}[grow] + " R: as it came.")
			return
		}
		m.codexStep()
		cf.CoverW, cf.CoverH = nw, nh
	case cpCount:
		m.say("The page count has one size. C colour · F font · Alt+arrows move it.")
		return
	default:
		if p == cpSubtitle && strings.TrimSpace(cf.Subtitle) == "" {
			m.say("No subtitle yet. Tab writes one.")
			return
		}
		pt := cf.partOf(p)
		now := pt.Size
		if now == 0 {
			now = max(c.cinfo().sizes[p], 1)
		}
		n := min(max(now+step, 1), codexSizes)
		if n == now && pt.Size != 0 {
			m.say(map[bool]string{true: "The biggest size.", false: "The smallest size."}[grow])
			return
		}
		m.codexStep()
		cf.part(p).Size = n
		cf.part(p).Hide = false
		cf.tidyCover()
		m.codexSave()
		if got := m.codexCoverLayNow().sizes[p]; got < n {
			m.say("Size " + itoa(n) + " does not fit, so it shows at " + itoa(max(got, 1)) + ". Make the cover bigger (choose the whole cover, then +) or the words shorter.")
			return
		}
		m.say(m.codexCoverSays() + " + bigger · − smaller · or drag its corner ◢.")
		return
	}
	cf.tidyCover()
	m.codexSave()
	m.say(m.codexCoverSays() + " + bigger · − smaller · or drag its corner ◢.")
}

// codexCoverStep is a cover size (% of the page, 0 all of it) one step of 10 bigger or smaller.
func codexCoverStep(v, step int) int {
	if v == 0 {
		v = 100
	}
	v = min(max(v+10*step, 30), 100)
	if v == 100 {
		return 0
	}
	return v
}

// codexCoverLayNow lays the cover out as it is on the screen now.
func (m *Model) codexCoverLayNow() *codexCoverInfo {
	c := m.cx()
	r := c.box["cover"]
	if r[2] <= 0 {
		pr := c.pageRect
		r = c.cf.coverRect(pr[0], pr[1], pr[2], pr[3])
	}
	_, info := codexCoverLaid(c.cf, r[2], r[3], 0, false, 0, nil, 0)
	return info
}

// cinfo is where the cover's parts were drawn last (nothing yet: all empty).
func (c *codexState) cinfo() *codexCoverInfo {
	if c.coverInfo == nil {
		return &codexCoverInfo{}
	}
	return c.coverInfo
}

// codexCoverRect is where part p is on the screen (from the last drawing).
func (c *codexState) codexCoverRect(p int) [4]int {
	o := c.box["cover"]
	r := c.cinfo().rects[p]
	if p == cpWhole {
		return o
	}
	return [4]int{o[0] + r[0], o[1] + r[1], r[2], r[3]}
}

// codexCoverMarks marks the chosen part in edit mode: ▸ ◂ either side and
// the corner ◢ the mouse drags.
func (m *Model) codexCoverMarks(cv *boardCanvas, desk string) {
	c := m.cx()
	p := c.coverPart
	r := c.codexCoverRect(p)
	if r[2] <= 0 || r[3] <= 0 {
		return
	}
	if p == cpWhole {
		hx, hy := codexHandle(r)
		if hx < cv.Width() && hy < cv.Height() {
			cvSet(cv, hx, hy, "◢", cAccent, cvBG(cv, hx, hy, desk), uv.AttrBold)
		}
		return
	}
	mid := r[1] + r[3]/2
	if r[3] == 1 || p == cpCount {
		mid = r[1]
	}
	set := func(x, y int, g string) {
		if x >= 0 && y >= 0 && x < cv.Width() && y < cv.Height() {
			cvSet(cv, x, y, g, cAccent, cvBG(cv, x, y, desk), uv.AttrBold)
		}
	}
	set(r[0]-2, mid, "▸")
	set(r[0]+r[2]+1, mid, "◂")
	if p != cpCount {
		hx, hy := codexHandle(r)
		set(hx, hy, "◢")
	}
}

// codexCoverPress: a press at (x, y) on the cover in edit mode: a corner
// starts a drag, a part is chosen, the rest of the cover chooses all of it.
func (m *Model) codexCoverPress(x, y int) bool {
	c := m.cx()
	if c.cf == nil || c.page >= 0 || !c.edit {
		return false
	}
	p := c.coverPart
	r := c.codexCoverRect(p)
	if r[2] > 0 && p != cpCount {
		hx, hy := codexHandle(r)
		s := &codexSizing{cover: p + 1, mx: x, my: y, r: r, size: max(c.cinfo().sizes[p], 1)}
		switch {
		case (x == hx || x == hx-1) && (y == hy || y == hy-1) && !(x == hx-1 && y == hy-1):
			s.w, s.h = true, true
		case p == cpWhole && x == hx && y >= r[1] && y < hy:
			s.w = true
		case p == cpWhole && y == hy && x >= r[0] && x < hx:
			s.h = true
		default:
			s = nil
		}
		if s != nil {
			m.codexStep() // one undo for the whole drag
			c.sizing = s
			m.say("Drag: right and down bigger, left and up smaller. Let go to keep it.")
			return true
		}
	}
	for _, q := range []int{cpCount, cpSymbol, cpTitle, cpSubtitle} {
		if qr := c.codexCoverRect(q); qr[2] > 0 && codexIn(qr, x, y) && !c.cf.partOf(q).Hide {
			c.coverPart = q
			m.say(m.codexCoverSays() + " + − size · C colour · Alt+arrows move · X hide")
			return true
		}
	}
	if codexIn(c.box["cover"], x, y) {
		c.coverPart = cpWhole
		m.say(m.codexCoverSays() + " + − or drag ◢ for its size · C its colour")
		return true
	}
	return false
}

// codexCoverDrag: the mouse moved with a cover corner held.
func (m *Model) codexCoverDrag(ms tea.Mouse) tea.Cmd {
	c := m.cx()
	s := c.sizing
	cf := c.cf
	x, y := ms.X, ms.Y-m.boardTop()
	dx, dy := x-s.mx, y-s.my
	if !s.w {
		dx = 0
	}
	if !s.h {
		dy = 0
	}
	p := s.cover - 1
	if p == cpWhole {
		pr := c.pageRect
		pct := func(n, of int) int {
			v := min(max(n*100/max(of, 1), 30), 100)
			if v >= 100 {
				return 0
			}
			return v
		}
		if s.w {
			cf.CoverW = pct(s.r[2]+2*dx, pr[2]) // in the middle: both sides move
		}
		if s.h {
			cf.CoverH = pct(s.r[3]+2*dy, pr[3])
		}
		m.say(m.codexCoverSays())
		return nil
	}
	// A part: the biggest size that fits where the mouse is.
	k := 1
	if cf.partOf(p).Align == 0 {
		k = 2
	}
	wantH, wantW := s.r[3]+dy, -1
	if abs(dx) > abs(dy) {
		wantH, wantW = -1, s.r[2]+k*dx
	}
	pt := cf.part(p)
	pt.Size = 1
	for n := codexSizes; n >= 1; n-- {
		pt.Size = n
		info := m.codexCoverLayNow()
		r := info.rects[p]
		if info.sizes[p] == n && ((wantH >= 0 && r[3] <= wantH) || (wantW >= 0 && r[2] <= wantW)) {
			break
		}
	}
	m.say(m.codexCoverSays())
	return nil
}

// codexOrInt is v, or or when v is 0.
func codexOrInt(v, or int) int {
	if v == 0 {
		return or
	}
	return v
}
