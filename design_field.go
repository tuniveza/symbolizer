package main

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// ---- writing in a structure, with Text mode's keys ----------------------------------
//
// The part being written in (a header, a text, a footer, a container's
// name) is held as a Buffer, the same kind Text mode edits, so its keys
// work as they do there: a real cursor; the arrows step by word, Ctrl +
// arrows a letter, Alt + Left / Right to the line's start / end, Alt + Up
// / Down a block; Shift highlights; Home, End, Ctrl + A; Ctrl + C / X / V;
// Backspace, Delete and Ctrl + Backspace / Delete; Ctrl + Z / Y undo the
// writing; Alt + Shift indent and move lines; Ctrl + D, Ctrl + K. Each key
// runs Text mode's own command, on a Model of its own that holds only this
// buffer, so the file open in Text mode and its view are never touched.
// Text mode's file, tab, find, Emmet and help keys are not here.

// fieldEd is the part being written as a buffer.
type fieldEd struct {
	id, part int     // the structure and its part the buffer is for
	buf      *Buffer // its words, cursor, highlight and undo
	text     string  // the part's words when the buffer last matched them
	ed       *Model  // the Model Text mode's commands run on (fieldModel)
}

// fieldKeys are Text mode's keys that work while writing in a structure:
// only what writes, moves the cursor or highlights (bindings, editor.go).
var fieldKeys = map[string]bool{
	"up": true, "down": true, "left": true, "right": true, "home": true, "end": true,
	"pgup": true, "pgdown": true, "alt+up": true, "alt+down": true, "ctrl+home": true, "ctrl+end": true,
	"alt+e": true, "alt+s": true, "alt+left": true, "alt+right": true,
	"shift+left": true, "shift+right": true, "shift+up": true, "shift+down": true,
	"ctrl+shift+left": true, "ctrl+shift+right": true, "ctrl+shift+up": true, "ctrl+shift+down": true,
	"shift+home": true, "shift+end": true, "shift+pgup": true, "shift+pgdown": true, "ctrl+a": true,
	"alt+shift+left": true, "alt+shift+right": true, "alt+shift+up": true, "alt+shift+down": true,
	"ctrl+c": true, "ctrl+x": true, "ctrl+v": true, "ctrl+p": true,
	"backspace": true, "delete": true, "alt+backspace": true, "ctrl+backspace": true, "ctrl+delete": true, "alt+delete": true,
	"ctrl+z": true, "ctrl+y": true, "ctrl+shift+z": true, "ctrl+d": true, "ctrl+k": true, "alt+d": true, "alt+b": true,
}

// fieldPtr is the words of part p of o.
func fieldPtr(o *dOct, p int) *string {
	switch p {
	case partHeader:
		return &o.Header
	case partBody:
		return &o.Body
	case partFooter:
		return &o.Footer
	case partThing: // a thing inside a container, reached by Tab: its title (design_sections.go)
		return &o.Header
	case partLabel: // written on the board (design_tags.go)
		return &o.LabelTitle
	case partPlate:
		return &o.Plate
	}
	return nil
}

// fieldBuffer is the buffer for the part of o being written, made afresh
// (the cursor at the end, as writing always started) when it is another
// part, or its words were changed some other way (an undo, a paste of a file).
func (d *designState) fieldBuffer(o *dOct) *Buffer {
	f := &d.field
	text := *fieldPtr(o, d.editing)
	if f.buf == nil || f.id != o.ID || f.part != d.editing || f.text != text {
		b := NewBuffer()
		b.Lines = b.Lines[:0]
		for l := range strings.SplitSeq(text, "\n") {
			b.Lines = append(b.Lines, []rune(l))
		}
		b.FinalNewline = false
		b.Cursor = Pos{len(b.Lines) - 1, len(b.Lines[len(b.Lines)-1])}
		f.buf, f.id, f.part, f.text = b, o.ID, d.editing, text
	}
	return f.buf
}

// fieldModel is the Model Text mode's commands run on: it holds only the
// buffer, with no file, colours, suggestions or view of the real one.
func (m *Model) fieldModel(b *Buffer) *Model {
	d := m.ds()
	if d.field.ed == nil {
		// Big enough that its view never matters; no key releases, so two
		// Ctrl + arrows go diagonally by their timing (ctrlArrow).
		// Plain text: no brackets closing themselves, no Emmet, no suggestions.
		d.field.ed = &Model{width: 400, height: 200, chord: chordState{held: map[string]bool{}}, hl: NewHighlighter("", "")}
	}
	fe := d.field.ed
	fe.buf = b
	fe.clip, fe.clipLine = m.clip, m.clipLine
	fe.msg, fe.msgErr = "", false
	return fe
}

// fieldRunes is where p is in the buffer's words, counted in letters.
func fieldOffset(b *Buffer, p Pos) int {
	n := 0
	for r := 0; r < p.Row && r < len(b.Lines); r++ {
		n += len(b.Lines[r]) + 1
	}
	return n + p.Col
}

// fieldMarks is where the cursor and the highlight are in part p of o
// (letters into its words), when it is being written in.
func (d *designState) fieldMarks(o *dOct, p int) (cur, s, e int, ok bool) {
	if d.sel != o.ID || d.editing != p || p == partNone {
		return 0, 0, 0, false
	}
	text := *fieldPtr(o, p)
	f := &d.field
	if f.buf == nil || f.id != o.ID || f.part != p || f.text != text {
		n := len([]rune(text))
		return n, n, n, true // not keyed in yet: the end, as writing starts
	}
	cur = fieldOffset(f.buf, f.buf.Cursor)
	s, e = cur, cur
	if a, b, sel := f.buf.Selection(); sel {
		s, e = fieldOffset(f.buf, a), fieldOffset(f.buf, b)
	}
	return cur, s, e, true
}

// bufMark is a buffer as it was before a key, to put it back exactly
// (a note that would be over-full).
type bufMark struct {
	undo, edits int
	redo        []*step
	cur         Pos
	anchor      *Pos
}

func markBuf(b *Buffer) bufMark {
	mk := bufMark{undo: len(b.undo), redo: b.redo, cur: b.Cursor}
	if t := b.top(); t != nil {
		mk.edits = len(t.edits)
	}
	if b.Anchor != nil {
		a := *b.Anchor
		mk.anchor = &a
	}
	return mk
}

// back puts b back as it was at mk: the changes since then taken out
// and forgotten, as if they never happened.
func (mk bufMark) back(b *Buffer) {
	for len(b.undo) > mk.undo {
		b.TakeBack()
	}
	if t := b.top(); t != nil && len(t.edits) > mk.edits {
		for i := len(t.edits) - 1; i >= mk.edits; i-- {
			e := t.edits[i]
			b.replace(e.At, endOf(e.At, e.New), e.Old)
		}
		t.edits = t.edits[:mk.edits]
	}
	b.redo, b.Cursor, b.Anchor, b.WantCol = mk.redo, b.Clamp(mk.cur), mk.anchor, -1
}

// designWrite is a key while writing in a structure: Esc stops, Tab and
// Shift + Tab go to the next part (and the one before), Enter starts a new
// line in the text (in a header or footer: the next part), and the rest
// are Text mode's keys, on the part's words.
func (m *Model) designWrite(d *designState, o *dOct, k tea.Key, name string) tea.Cmd {
	if fieldPtr(o, d.editing) == nil || d.editing != partThing && !isTagPart(d.editing) && !hasPart(o, d.editing) {
		d.editing = partNone
		return nil
	}
	if d.editing == partThing { // a thing in a container: Tab, Shift + Tab and Enter go on to the next section
		switch name {
		case "tab", "enter", "ctrl+j":
			m.designSectionTab(false)
			return nil
		case "shift+tab":
			m.designSectionTab(true)
			return nil
		}
	}
	switch name {
	case "esc":
		d.editing = partNone
		return nil
	case "tab":
		d.editing = nextPart(o, d.editing, 1)
		m.viewToPart(o, d.editing)
		return nil
	case "shift+tab":
		d.editing = nextPart(o, d.editing, -1)
		m.viewToPart(o, d.editing)
		return nil
	case "enter", "ctrl+j":
		if d.editing != partBody {
			d.editing = nextPart(o, d.editing, 1)
			m.viewToPart(o, d.editing)
			return nil
		}
		return m.fieldDo(d, o, func(fe *Model) tea.Cmd {
			fe.buf.BreakUndo()
			fe.buf.Insert("\n"+fieldIndent(fe.buf), "")
			return nil
		})
	case "ctrl+u":
		return m.fieldDo(d, o, func(fe *Model) tea.Cmd { // the whole part
			fe.buf.BreakUndo()
			fe.selectAll()
			fe.buf.DeleteSelection()
			return nil
		})
	}
	if dir := ctrlArrow(name); dir != "" {
		return m.fieldDo(d, o, func(fe *Model) tea.Cmd { fe.ctrlArrow(dir, k); return nil })
	}
	if fieldKeys[name] {
		fn := bindings[name]
		return m.fieldDo(d, o, func(fe *Model) tea.Cmd {
			switch name {
			case "ctrl+z":
				fe.undo()
				return nil
			case "ctrl+y", "ctrl+shift+z":
				fe.redo()
				return nil
			}
			fe.buf.BreakUndo()
			if name == "ctrl+v" || name == "ctrl+p" {
				return readClipboard(m.clip) // its words come back as a pasteMsg (designPaste)
			}
			return fn(fe)
		})
	}
	// Letters: typed when no Ctrl or Alt is held, or both (AltGr), as in Text mode.
	ctrl, alt := k.Mod.Contains(tea.ModCtrl), k.Mod.Contains(tea.ModAlt)
	s := k.Text
	if s == "" && k.Code == tea.KeySpace && !ctrl && !alt {
		s = " "
	}
	if s == "" || ctrl != alt {
		return nil
	}
	s = textLike(s)
	return m.fieldDo(d, o, func(fe *Model) tea.Cmd {
		if s != " " && fe.afterSpace {
			fe.buf.BreakUndo() // undo takes back a word at a time
		}
		fe.afterSpace = s == " "
		fe.buf.Insert(s, "type")
		return nil
	})
}

// fieldIndent is the indent a new line keeps: the line's own, up to the cursor.
func fieldIndent(b *Buffer) string {
	in := leadingSpace(b.Lines[b.Cursor.Row])
	if s, _, ok := b.Selection(); ok {
		in = leadingSpace(b.Lines[s.Row])
	}
	if b.Cursor.Col < len(in) {
		in = in[:b.Cursor.Col]
	}
	return in
}

// fieldWrite puts words in at the cursor while writing (a paste): a header
// or footer keeps to one line.
func (m *Model) fieldWrite(text string) tea.Cmd {
	d := m.ds()
	o := d.byID(d.sel)
	if d.editing == partThing {
		o = d.sectionThing() // a thing in a container (design_sections.go)
	}
	if o == nil || d.editing == partNone || fieldPtr(o, d.editing) == nil {
		return nil
	}
	add := textLike(text)
	if d.editing != partBody {
		add = strings.ReplaceAll(add, "\n", " ")
	}
	return m.fieldDo(d, o, func(fe *Model) tea.Cmd { return fe.pasteText(add) })
}

// fieldDo runs fn (a Text mode command) on the part of o being written,
// then puts the words back in the part. A header or footer stays one line,
// and a note never holds more than a page: a key that would break either
// is taken back.
func (m *Model) fieldDo(d *designState, o *dOct, fn func(fe *Model) tea.Cmd) tea.Cmd {
	field := fieldPtr(o, d.editing)
	b := d.fieldBuffer(o)
	fe := m.fieldModel(b)
	mk := markBuf(b)
	before := *field
	cmd := fn(fe)
	m.clip, m.clipLine = fe.clip, fe.clipLine
	if fe.msg != "" {
		m.msg, m.msgErr = fe.msg, fe.msgErr
	}
	text := b.Text()
	if text == before {
		return cmd // the cursor moved, or something was highlighted or copied
	}
	if d.editing != partBody && len(b.Lines) > 1 {
		mk.back(b)
		m.say("A " + map[int]string{partHeader: "title", partFooter: "footer", partThing: "title", partLabel: "label", partPlate: "name plate"}[d.editing] + " is one line.")
		return nil
	}
	if o.Kind == kNote && d.editing == partBody && !noteFits(text) {
		mk.back(b)
		m.say("This note is full: a note holds one page (" + fmt.Sprint(noteRows) + " lines). Use a page for more.")
		return nil
	}
	*field, d.field.text = text, text
	d.dirty = true
	if d.editing == partThing {
		return cmd // a thing in a container: the container draws it
	}
	if isTagPart(d.editing) { // drawn on the board over the structure (design_tags.go)
		if d.editing == partPlate {
			m.tagSpread(o)
		}
		return cmd
	}
	if m.growToFit(o) { // all of it shows, without resizing by hand
		m.keepInView(o, d.editing == partBody)
	}
	return cmd
}

// ---- drawing the cursor and the highlight ---------------------------------------------

// fcell is one cell of words being written: a letter, and whether it is
// highlighted, has the cursor on it, or is the caret after a line's end.
type fcell struct {
	g    string
	mark byte // 0, markHL, markCur or markCaret
}

const (
	markHL    = 'h'
	markCur   = 'c'
	markCaret = '|'
)

// fieldCells marks letters rs, which are at offs in the words: the
// highlight from s to e, the cursor on letter at (or the caret after the
// end when at == len(rs)); at < 0: no cursor here.
func fieldCells(rs []rune, offs []int, at, s, e int) []fcell {
	out := make([]fcell, 0, len(rs)+1)
	for i, r := range rs {
		c := fcell{g: string(r)}
		if i < len(offs) && offs[i] >= s && offs[i] < e {
			c.mark = markHL
		}
		if i == at {
			c.mark = markCur
		}
		out = append(out, c)
	}
	if at >= len(rs) {
		out = append(out, fcell{g: "▏", mark: markCaret})
	}
	return out
}

func cellsWidth(cs []fcell) int {
	n := 0
	for _, c := range cs {
		n += ansi.StringWidth(c.g)
	}
	return n
}

// fieldLine is a one-line part being written (a title, a footer, a
// container's name), after prefix (its icon): as much as fits in room
// cells round the cursor, "…" where it is cut.
func (d *designState) fieldLine(o *dOct, part int, prefix string, room int) []fcell {
	text := *fieldPtr(o, part)
	cur, s, e, ok := d.fieldMarks(o, part)
	if !ok {
		cur, s, e = -1, 0, 0
	}
	rs := []rune(text)
	offs := make([]int, len(rs))
	for i := range offs {
		offs[i] = i
	}
	pre := fieldCells([]rune(prefix), nil, -1, 0, 0)
	cells := fieldCells(rs, offs, cur, s, e)
	room -= cellsWidth(pre)
	if room < 2 || cellsWidth(cells) <= room {
		return append(pre, cells...)
	}
	// Too long: the end shows (as typing at the end); with the cursor
	// further back, from the cursor on.
	w := func(i int) int { return ansi.StringWidth(cells[i].g) }
	a, used := len(cells), 1 // 1: the "…" on the left
	for a > 0 && used+w(a-1) <= room {
		a--
		used += w(a)
	}
	b := len(cells)
	if cur >= 0 && cur < a {
		a, used = max(0, cur-2), 2 // "…" either side
		if a == 0 {
			used = 1
		}
		b = a
		for b < len(cells) && used+w(b) <= room {
			used += w(b)
			b++
		}
	}
	out := pre
	if a > 0 {
		out = append(out, fcell{g: "…"})
	}
	out = append(out, cells[a:b]...)
	if b < len(cells) {
		out = append(out, fcell{g: "…"})
	}
	return out
}

// fieldRow is one row of wrapped words: its letters and where each is.
type fieldRow struct {
	rs   []rune
	offs []int
}

// fieldWrap wraps text to w cells as ansi.Wrap does (each paragraph on
// its own; the spaces where a line breaks are left out), keeping where
// each letter came from, and says which row and letter the cursor cur is
// on. A caret at the end of a full row goes to the start of the next.
func fieldWrap(text string, w, cur int) (rows []fieldRow, crow, ccol int) {
	crow, ccol = -1, -1
	start := 0
	for p := range strings.SplitSeq(text, "\n") {
		pr := []rune(p)
		first := len(rows)
		i := 0
		for l := range strings.SplitSeq(ansi.Wrap(p, w, ""), "\n") {
			lr := []rune(l)
			offs := make([]int, len(lr))
			for j, r := range lr {
				for i < len(pr) && pr[i] != r {
					i++
				}
				offs[j] = start + i
				if i < len(pr) {
					i++
				}
			}
			rows = append(rows, fieldRow{lr, offs})
		}
		end := start + len(pr)
		if crow < 0 && cur >= start && cur <= end {
			crow, ccol = caretIn(rows, first, cur)
			if ccol >= len(rows[crow].rs) && ansi.StringWidth(string(rows[crow].rs)) >= w {
				// No room for the caret after a full row: it goes to the
				// start of the next row, or under the paragraph's last.
				if crow == len(rows)-1 {
					rows = append(rows, fieldRow{})
				}
				crow, ccol = crow+1, 0
			}
		}
		start = end + 1
	}
	return rows, crow, ccol
}

// caretIn is the row and letter of cursor cur in the paragraph whose rows
// run from first to the last of rows: on the letter at cur, or (cur being
// a space left out where a line breaks, or the end) after the row before.
func caretIn(rows []fieldRow, first, cur int) (int, int) {
	for r := first; r < len(rows); r++ {
		for j, o := range rows[r].offs {
			switch {
			case o == cur, o > cur && (j > 0 || r == first):
				return r, j
			case o > cur:
				return r - 1, len(rows[r-1].rs)
			}
		}
	}
	last := len(rows) - 1
	return last, len(rows[last].rs)
}

// fieldBody is the text of o being written, wrapped to w cells, as rows
// of cells: rows of them round the cursor (in the middle, but the end
// shows while it fits, as when typing at the end). total is how many
// rows the whole text has, and top the first one shown.
func (d *designState) fieldBody(o *dOct, w, rows int) (out [][]fcell, total, top int) {
	if rows <= 0 || w <= 0 {
		return nil, 0, 0
	}
	cur, s, e, ok := d.fieldMarks(o, partBody)
	if !ok {
		cur, s, e = len([]rune(o.Body)), 0, 0
	}
	wr, crow, ccol := fieldWrap(o.Body, w, cur)
	total = len(wr)
	top = max(0, min(total-rows, crow-rows/2))
	for r := top; r < min(total, top+rows); r++ {
		at := -1
		if r == crow {
			at = ccol
		}
		out = append(out, trimCells(fieldCells(wr[r].rs, wr[r].offs, at, s, e)))
	}
	return out, total, top
}

// cutCells keeps the cells that fit in w cells.
func cutCells(cs []fcell, w int) []fcell {
	n := 0
	for i, c := range cs {
		if n += ansi.StringWidth(c.g); n > w {
			return cs[:i]
		}
	}
	return cs
}

// trimCells drops the plain spaces at a row's end (as the board's text is drawn).
func trimCells(cs []fcell) []fcell {
	for len(cs) > 0 && cs[len(cs)-1].g == " " && cs[len(cs)-1].mark == 0 {
		cs = cs[:len(cs)-1]
	}
	return cs
}

// cellsStyled draws cells in style base on fill: the highlight in the
// selection's colours, the cursor's letter the other way round, the caret
// in the accent.
func cellsStyled(cs []fcell, base lipgloss.Style, fill string) string {
	var sb strings.Builder
	run, mark := "", byte(0)
	flush := func() {
		if run == "" {
			return
		}
		switch mark {
		case markHL:
			sb.WriteString(base.Foreground(hex(cSelectedText)).Background(hex(cSelection)).Render(run))
		case markCur:
			sb.WriteString(base.Foreground(hex(fill)).Background(hex(cAccent)).Render(run))
		case markCaret:
			sb.WriteString(st(cAccent, fill).Render(run))
		default:
			sb.WriteString(base.Render(run))
		}
		run = ""
	}
	for _, c := range cs {
		if c.mark != mark {
			flush()
			mark = c.mark
		}
		run += c.g
	}
	flush()
	return sb.String()
}

// fieldHeader is a container's name being written, drawn: its icon and
// the name round the cursor, in room cells, in style head on fill.
func (d *designState) fieldHeader(o *dOct, prefix string, room int, head lipgloss.Style, fill string) string {
	return cellsStyled(d.fieldLine(o, partHeader, prefix, room), head, fill)
}
