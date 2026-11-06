package main

// Brackets and quotes that close themselves, in code files (not plain text
// or Markdown, where ' is an apostrophe):
//
//	( [ { " ' `   the closer appears after the cursor
//	) ] } " ' `   typed in front of the same closer steps over it
//	Backspace     between an empty pair deletes both halves
//	highlighted   an opener wraps the highlighted text in the pair
//	CSS ":"       after a property name, the ";" appears after the cursor
//	CSS ";"       typed in front of a ";" steps over it

import (
	"strings"

	"symbolizer/emmet"
)

var closerOf = map[rune]rune{'(': ')', '[': ']', '{': '}', '"': '"', '\'': '\'', '`': '`'}
var openerOf = map[rune]rune{')': '(', ']': '[', '}': '{'}

func isQuote(r rune) bool { return r == '"' || r == '\'' || r == '`' }

// pairsOn tells whether brackets close themselves in this file.
func (m *Model) pairsOn() bool {
	if m.inDiary() {
		return false
	}
	switch m.hl.Name {
	case "Text", "Markdown", "Org", "ReStructuredText", "TeX":
		return false
	}
	return true
}

// typePair handles one typed letter that is part of a pair. It says false
// when the letter should be typed the normal way.
func (m *Model) typePair(s string) bool {
	rs := []rune(s)
	if len(rs) != 1 || !m.pairsOn() {
		return false
	}
	r := rs[0]
	b := m.buf
	if _, _, sel := b.Selection(); sel {
		if _, ok := closerOf[r]; ok {
			m.wrapPair(r)
			return true
		}
		return false
	}
	l, col := m.line(b.Cursor.Row), b.Cursor.Col
	var prev, next rune
	if col > 0 {
		prev = l[col-1]
	}
	if col < len(l) {
		next = l[col]
	}
	syn := m.webSyntax()

	// step over a closer that is already there
	if r == next && (openerOf[r] != 0 && balanced(l, openerOf[r], r) ||
		isQuote(r) && strings.Count(string(l[:col]), s)%2 == 1 ||
		r == ';' && emmet.IsStylesheet(syn)) {
		b.BreakUndo()
		b.Cursor, b.WantCol = Pos{b.Cursor.Row, col + 1}, -1
		m.afterTyped(s) // a finished path or CSS line is still checked
		return true
	}

	closer, ok := closerOf[r]
	if !ok {
		return false
	}
	if isQuote(r) {
		if prev == r && col >= 2 && l[col-2] == r && strings.HasPrefix(m.hl.Name, "Python") && next != r {
			m.insertPair(s, s+s+s) // """ docstring """
			return true
		}
		if prev == r || isWord(prev) && !strPrefix(l[:col]) {
			return false
		}
		if next != 0 && (isWord(next) || isQuote(next)) {
			return false
		}
	} else if next != 0 && !strings.ContainsRune(" \t)]};,:>", next) {
		return false // in front of a word: the closer goes somewhere else
	}
	m.insertPair(s, string(closer))
	return true
}

// insertPair types open and close with the cursor between them, as one undo step with the typing.
func (m *Model) insertPair(open, close string) {
	b := m.buf
	b.Insert(open+close, "type")
	p := Pos{b.Cursor.Row, b.Cursor.Col - len([]rune(close))}
	b.Cursor, b.WantCol = p, -1
	if top := b.top(); top != nil {
		top.cursorAfter = p
	}
}

// wrapPair puts the highlighted text between open and its closer, and keeps it highlighted.
func (m *Model) wrapPair(open rune) {
	b := m.buf
	s, e, _ := b.Selection()
	b.BreakUndo()
	b.Group(func() {
		b.Change(e, e, string(closerOf[open]), "")
		b.Change(s, s, string(open), "")
	})
	b.BreakUndo()
	if e.Row == s.Row {
		e.Col++
	}
	a := Pos{s.Row, s.Col + 1}
	b.Anchor, b.Cursor, b.WantCol = &a, e, -1
}

// backspacePair deletes both halves of an empty pair, like (|).
func (m *Model) backspacePair() bool {
	b := m.buf
	if _, _, sel := b.Selection(); sel || !m.pairsOn() {
		return false
	}
	l, col := m.line(b.Cursor.Row), b.Cursor.Col
	if col == 0 || col >= len(l) || closerOf[l[col-1]] == 0 || closerOf[l[col-1]] != l[col] {
		return false
	}
	if openerOf[l[col]] != 0 && !balanced(l, l[col-1], l[col]) {
		return false
	}
	b.Change(Pos{b.Cursor.Row, col - 1}, Pos{b.Cursor.Row, col + 1}, "", "delete")
	return true
}

// cssColon adds the ";" after "color:|" when the word before ":" is a CSS
// property and nothing follows on the line.
func (m *Model) cssColon() {
	if !m.pairsOn() || !m.insideBraces() && m.webSyntax() != "sass" && m.webSyntax() != "stylus" {
		return
	}
	if m.webSyntax() == "sass" || m.webSyntax() == "stylus" {
		return // no ; in these
	}
	b := m.buf
	l, col := m.line(b.Cursor.Row), b.Cursor.Col
	if strings.TrimSpace(string(l[col:])) != "" && strings.TrimSpace(string(l[col:])) != "}" {
		return
	}
	name := strings.TrimSpace(strings.TrimSuffix(string(l[:col]), ":"))
	if i := strings.LastIndexAny(name, "{;"); i >= 0 {
		name = strings.TrimSpace(name[i+1:])
	}
	if !emmet.LikelyCSSProperty(name) {
		return
	}
	m.insertPair("", ";")
}

// balanced tells whether the line has as many closers as openers.
func balanced(l []rune, open, close rune) bool {
	n := 0
	for _, r := range l {
		switch r {
		case open:
			n++
		case close:
			n--
		}
	}
	return n == 0
}

// strPrefix: the letters before a quote are a string prefix like f, r, b (Python) or u8 (C++).
func strPrefix(before []rune) bool {
	i := len(before)
	for i > 0 && isWord(before[i-1]) {
		i--
	}
	switch strings.ToLower(string(before[i:])) {
	case "f", "r", "b", "u", "rb", "br", "fr", "rf", "u8", "l":
		return true
	}
	return false
}
