package main

import (
	"unicode"

	tea "charm.land/bubbletea/v2"
)

// Word wrap: a line too long for the zone carries on in the rows under it.
// It breaks after a space (or a hyphen between letters); a word longer than
// the whole row breaks where the row ends. The rows a line carries on in
// start at the line's own indent, so wrapped code still lines up. Spaces at
// a break hang past the edge instead of starting the next row. On by
// default; Alt+Z switches it, and the choice is remembered.

// lineRows is where each screen row of a line starts: the letter (start)
// and its screen column from the line's start (x). The first row starts at
// 0. indent is how far the rows after the first sit to the right.
type lineRows struct {
	start, x []int
	indent   int
}

var oneRow = lineRows{start: []int{0}, x: []int{0}}

type wrapEntry struct {
	line  []rune
	width int
	rows  lineRows
}

// rowsOf splits line r into screen rows. Without wrap, a line is one row.
// Edits always make new line slices, so a cached split is still right when
// it was made from the very same slice at the same width.
func (m *Model) rowsOf(r int) lineRows {
	l := m.buf.Lines[r]
	w := m.textWidth() - 1 // one column kept free for the cursor at the end
	if !m.wrap || len(l) == 0 || w < 2 {
		return oneRow
	}
	if e, ok := m.wrapCache[r]; ok && e.width == w && len(e.line) == len(l) && &e.line[0] == &l[0] {
		return e.rows
	}
	if m.wrapCache == nil || len(m.wrapCache) > 4000 {
		m.wrapCache = map[int]wrapEntry{}
	}
	rows := wrapLine(l, w)
	m.wrapCache[r] = wrapEntry{l, w, rows}
	return rows
}

func blank(c rune) bool { return c == ' ' || c == '\t' }

// wrapLine breaks l into rows of at most w columns.
func wrapLine(l []rune, w int) lineRows {
	rows := lineRows{start: []int{0}, x: []int{0}}
	ind := 0
	for _, c := range l {
		if !blank(c) {
			break
		}
		ind += runeWidth(c, ind)
	}
	if ind > w/2 || ind == colToX(l, len(l)) {
		ind = 0
	}
	rows.indent = ind
	x, rowX, avail := 0, 0, w
	start, brk, brkX := 0, -1, 0
	for c := 0; c < len(l); {
		ch := l[c]
		cw := runeWidth(ch, x)
		if c > start && !blank(ch) && (blank(l[c-1]) ||
			l[c-1] == '-' && c >= 2 && unicode.IsLetter(l[c-2]) && unicode.IsLetter(ch)) {
			brk, brkX = c, x // a new row may start here
		}
		if x+cw-rowX > avail && c > start && !blank(ch) {
			if brk <= start { // one word fills the row: break inside it
				brk, brkX = c, x
			}
			rows.start = append(rows.start, brk)
			rows.x = append(rows.x, brkX)
			start, rowX, avail = brk, brkX, w-ind
			continue // look at this letter again, in the new row
		}
		x += cw
		c++
	}
	return rows
}

// vrow is one screen row: row k of line r.
type vrow struct{ r, k int }

func (a vrow) before(b vrow) bool { return a.r < b.r || a.r == b.r && a.k < b.k }

// rowEnd is the letter after the last one on row k of line r.
func (m *Model) rowEnd(lr lineRows, r, k int) int {
	if k+1 < len(lr.start) {
		return lr.start[k+1]
	}
	return len(m.buf.Lines[r])
}

// rowAt is the row of line r that letter col is on. A letter where a row
// starts is on that row, so the end of a full row shows at the next one.
func (m *Model) rowAt(r, col int) vrow {
	lr := m.rowsOf(r)
	k := len(lr.start) - 1
	for k > 0 && lr.start[k] > col {
		k--
	}
	return vrow{r, k}
}

func (m *Model) topRow() vrow {
	if m.top >= len(m.buf.Lines) {
		return vrow{len(m.buf.Lines) - 1, 0}
	}
	return vrow{m.top, min(m.topSub, len(m.rowsOf(m.top).start)-1)}
}

func (m *Model) setTop(v vrow) { m.top, m.topSub = v.r, v.k }

func (m *Model) nextRow(v vrow) (vrow, bool) {
	if v.k+1 < len(m.rowsOf(v.r).start) {
		return vrow{v.r, v.k + 1}, true
	}
	if v.r+1 < len(m.buf.Lines) {
		return vrow{v.r + 1, 0}, true
	}
	return v, false
}

func (m *Model) prevRow(v vrow) (vrow, bool) {
	if v.k > 0 {
		return vrow{v.r, v.k - 1}, true
	}
	if v.r > 0 {
		return vrow{v.r - 1, len(m.rowsOf(v.r-1).start) - 1}, true
	}
	return v, false
}

// rowsUp is the row n rows above v (or the first row).
func (m *Model) rowsUp(v vrow, n int) vrow {
	for ; n > 0; n-- {
		p, ok := m.prevRow(v)
		if !ok {
			break
		}
		v = p
	}
	return v
}

// rowsBetween counts the rows from a down to b, stopping past limit.
func (m *Model) rowsBetween(a, b vrow, limit int) int {
	if b.r-a.r > limit {
		return limit + 1
	}
	n := 0
	for a.before(b) && n <= limit {
		a, _ = m.nextRow(a)
		n++
	}
	return n
}

// screenRows lists the rows on screen, top to bottom. Rows past the end of
// the file have r = len(Lines).
func (m *Model) screenRows() []vrow {
	h := m.textHeight()
	out := make([]vrow, 0, h)
	v, more := m.topRow(), true
	for len(out) < h {
		if !more {
			out = append(out, vrow{len(m.buf.Lines), 0})
			continue
		}
		out = append(out, v)
		v, more = m.nextRow(v)
	}
	return out
}

// rowOffset is how far right row k of line r sits: centred lines are
// centred row by row; the rows a wrapped line carries on in, indented.
func (m *Model) rowOffset(r, k int) int {
	if !m.wrap {
		return m.lineOffset(r)
	}
	if r < 0 || r >= len(m.buf.Lines) {
		return 0
	}
	lr := m.rowsOf(r)
	if m.centred() {
		l := m.buf.Lines[r]
		s, e := lr.start[k], m.rowEnd(lr, r, k)
		for e > s && k+1 < len(lr.start) && blank(l[e-1]) {
			e-- // the space it broke at does not count
		}
		w := 0
		for c := s; c < e; c++ {
			w += runeWidth(l[c], lr.x[k]+w)
		}
		if tw := m.textWidth(); w < tw-1 {
			return (tw - 1 - w) / 2
		}
		return 0
	}
	if k > 0 {
		return lr.indent
	}
	return 0
}

// indentOf is the indent of a wrapped row, which Up / Down count as part
// of the column. Centring does not count: as without wrap, Up / Down keep
// the letter column on centred lines.
func (m *Model) indentOf(r, k int) int {
	if k == 0 || m.centred() {
		return 0
	}
	return m.rowsOf(r).indent
}

// scrollRows scrolls the text n rows (the mouse wheel).
func (m *Model) scrollRows(n int) {
	if !m.wrap {
		m.top = max(0, min(m.top+n, len(m.buf.Lines)-1))
		return
	}
	v := m.topRow()
	if n < 0 {
		v = m.rowsUp(v, -n)
	}
	for ; n > 0; n-- {
		v, _ = m.nextRow(v)
	}
	m.setTop(v)
}

// moveRows is Up / Down with wrap on: one screen row at a time, keeping the
// screen column.
func (m *Model) moveRows(d int) {
	b := m.buf
	cur := m.rowAt(b.Cursor.Row, b.Cursor.Col)
	lr := m.rowsOf(cur.r)
	if b.WantCol < 0 {
		b.WantCol = m.indentOf(cur.r, cur.k) + colToX(m.line(cur.r), b.Cursor.Col) - lr.x[cur.k]
	}
	want := b.WantCol
	v, ok := cur, true
	for i := 0; i < d && ok; i++ {
		v, ok = m.nextRow(v)
	}
	for i := 0; i > d && ok; i-- {
		v, ok = m.prevRow(v)
	}
	switch {
	case !ok && d < 0:
		b.Cursor, b.WantCol = Pos{0, 0}, -1
	case !ok:
		last := len(b.Lines) - 1
		b.Cursor, b.WantCol = Pos{last, len(m.line(last))}, -1
	default:
		l, lr := m.line(v.r), m.rowsOf(v.r)
		s := lr.start[v.k]
		col := xToColAt(l, s, lr.x[v.k], lr.x[v.k]+max(want-m.indentOf(v.r, v.k), 0))
		if v.k+1 < len(lr.start) {
			col = min(col, lr.start[v.k+1]-1) // the row's end shows at the next row's start
		}
		b.Cursor = Pos{v.r, col}
	}
}

// ensureRowVisible scrolls so the cursor's row is on screen, a few rows
// from the edges.
func (m *Model) ensureRowVisible() {
	h := m.textHeight()
	margin := min(3, (h-1)/2)
	cur := m.rowAt(m.buf.Cursor.Row, m.buf.Cursor.Col)
	top := m.topRow()
	if cur.before(top) {
		m.setTop(m.rowsUp(cur, margin))
		return
	}
	switch d := m.rowsBetween(top, cur, h); {
	case d < margin:
		m.setTop(m.rowsUp(cur, margin))
	case d > h-1-margin:
		m.setTop(m.rowsUp(cur, h-1-margin))
	default:
		m.setTop(top)
	}
}

func (m *Model) toggleWrap() tea.Cmd {
	m.wrap = !m.wrap
	m.left, m.topSub = 0, 0
	m.ensureVisible()
	if m.wrap {
		saveSetting("wrap", "on")
		m.say("Word wrap on: long lines carry on in the rows under them. Alt+Z turns it off.")
	} else {
		saveSetting("wrap", "off")
		m.say("Word wrap off: long lines run past the edge. Alt+Z turns it on.")
	}
	return nil
}
