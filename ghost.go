package main

// Unit hints in CSS: a dim "shadow" in the line shows the recommended unit
// before or while a value is typed (font-size: 1|rem), or a better unit for
// a value already there (16px → 1rem). Tab takes it. The message line says
// why in one sentence. The rules are in emmet/units.go.

import (
	"symbolizer/emmet"
)

type hintCache struct {
	buf     *Buffer
	version int
	cursor  Pos
	hint    emmet.UnitHint
	ok      bool
}

// unitHint is the hint for the cursor's line, or ok false.
func (m *Model) unitHint() (emmet.UnitHint, bool) {
	b := m.buf
	c := &m.hintC
	if c.buf == b && c.version == b.Version() && c.cursor == b.Cursor {
		return c.hint, c.ok
	}
	c.buf, c.version, c.cursor = b, b.Version(), b.Cursor
	c.hint, c.ok = m.findUnitHint()
	return c.hint, c.ok
}

func (m *Model) findUnitHint() (emmet.UnitHint, bool) {
	syn := m.webSyntax()
	if !emmet.IsStylesheet(syn) || len(m.buf.Lines) > 50000 {
		return emmet.UnitHint{}, false
	}
	if _, _, sel := m.buf.Selection(); sel {
		return emmet.UnitHint{}, false
	}
	if syn != "sass" && syn != "stylus" && !m.insideBraces() {
		return emmet.UnitHint{}, false
	}
	return emmet.UnitHintAt(m.line(m.buf.Cursor.Row), m.buf.Cursor.Col)
}

// showHint: the hint is drawn only in the plain editing view.
func (m *Model) showHint() (emmet.UnitHint, bool) {
	if m.prompt != pNone || m.suggestOn() != nil || m.learn != nil || m.picker != nil || m.help {
		return emmet.UnitHint{}, false
	}
	return m.unitHint()
}

// takeUnitHint is Tab on a hint at the cursor: it writes the recommended value.
func (m *Model) takeUnitHint() bool {
	h, ok := m.showHint()
	if !ok || !h.AtCursor || h.New == "" {
		return false
	}
	b := m.buf
	row := b.Cursor.Row
	b.BreakUndo()
	end := b.Change(Pos{row, h.Start}, Pos{row, h.End}, h.New, "")
	b.BreakUndo()
	b.Cursor = end
	m.say("Done: " + h.New + ". Ctrl+Z takes it back.")
	m.ensureVisible()
	return true
}

// noteHint puts the reason for a hint in the message line, after a key.
func (m *Model) noteHint() {
	if m.msg != "" {
		return
	}
	h, ok := m.showHint()
	if !ok {
		return
	}
	if h.AtCursor && h.New != "" {
		m.say(h.Why + " Tab: " + trimSpace(h.New) + ".")
	} else {
		m.say(h.Why)
	}
}

func trimSpace(s string) string {
	for len(s) > 0 && s[0] == ' ' {
		s = s[1:]
	}
	return s
}
