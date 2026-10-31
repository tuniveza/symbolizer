package main

// The suggestion list: while a word is typed, a small list under the cursor
// offers ways to finish it.
//
//	CSS        property names; after "prop:" the values that property takes;
//	           after ":" in a selector, pseudo-classes
//	HTML       tag names after "<"
//	other code words already in the file (nearest first)
//
// Tab takes the chosen one. Up / Down choose. Esc closes the list. Any other
// key closes it and does what it always does.

import (
	"sort"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"symbolizer/emmet"
)

const suggestShown = 6 // rows the list shows at once

type suggestions struct {
	buf   *Buffer
	items []string
	sel   int
	row   int    // the line being typed on
	start int    // where the word being finished starts
	kind  string // "prop", "value", "pseudo", "tag", "word"
}

// suggestOn is the list for this buffer and cursor line, or nil.
func (m *Model) suggestOn() *suggestions {
	s := m.sugg
	if s == nil || s.buf != m.buf || m.prompt != pNone || s.row != m.buf.Cursor.Row || s.start > m.buf.Cursor.Col {
		return nil
	}
	return s
}

// refreshSuggest works out the list after a letter is typed or deleted.
// open: show values even before a letter of one is typed (straight after a property is chosen).
func (m *Model) refreshSuggest(open ...bool) {
	m.sugg = nil
	b := m.buf
	if _, _, sel := b.Selection(); sel || m.prompt != pNone || len(b.Lines) > 50000 {
		return
	}
	l, col := m.line(b.Cursor.Row), b.Cursor.Col
	if col < len(l) && (isWord(l[col]) || l[col] == '-') {
		return // in the middle of a word
	}
	start := col
	for start > 0 && (isWord(l[start-1]) || l[start-1] == '-') {
		start--
	}
	prefix := string(l[start:col])
	items, kind := m.suggestFor(l, start, prefix)
	if prefix == "" && len(open) == 0 {
		return
	}
	if len(items) == 0 || len(items) == 1 && items[0] == prefix {
		return
	}
	if len(items) > 40 {
		items = items[:40]
	}
	m.sugg = &suggestions{buf: b, items: items, row: b.Cursor.Row, start: start, kind: kind}
}

// suggestFor gives the words that could finish prefix (which starts at column start of l).
func (m *Model) suggestFor(l []rune, start int, prefix string) ([]string, string) {
	syn := m.webSyntax()
	before := strings.TrimRight(string(l[:start]), " \t")
	switch {
	case emmet.IsStylesheet(syn):
		if m.insideBraces() || syn == "sass" || syn == "stylus" {
			decl := before
			if i := strings.LastIndexAny(decl, "{;"); i >= 0 {
				decl = decl[i+1:]
			}
			if i := strings.Index(decl, ":"); i >= 0 {
				prop := strings.TrimSpace(decl[:i])
				if !emmet.IsCSSProperty(prop) {
					return nil, ""
				}
				return without(emmet.CSSValues(prop, prefix), prefix), "value"
			}
			// 3 letters: shorter words are Emmet's (fl, ta, op) and Tab expands them
			if strings.TrimSpace(decl) != "" || len([]rune(prefix)) < 3 || prefix[0] == '-' {
				return nil, ""
			}
			return emmet.CSSProperties(prefix), "prop"
		}
		if strings.HasSuffix(string(l[:start]), ":") && prefix != "" {
			return emmet.CSSPseudo(prefix), "pseudo"
		}
		return nil, ""
	case emmet.IsMarkup(syn):
		if strings.HasSuffix(string(l[:start]), "<") && prefix != "" {
			return emmet.HTMLTags(prefix), "tag"
		}
		return nil, ""
	case !m.pairsOn():
		return nil, "" // plain text and Markdown: no list
	}
	if len([]rune(prefix)) < 2 || !isWord([]rune(prefix)[0]) || unicodeDigit([]rune(prefix)[0]) {
		return nil, ""
	}
	return m.fileWords(prefix), "word"
}

func unicodeDigit(r rune) bool { return r >= '0' && r <= '9' }

func without(list []string, w string) []string {
	out := list[:0:0]
	for _, x := range list {
		if x != w {
			out = append(out, x)
		}
	}
	return out
}

// fileWords gives the words in the file that start with prefix and are
// longer than it, nearest to the cursor first.
func (m *Model) fileWords(prefix string) []string {
	b := m.buf
	cur := b.Cursor
	near := map[string]int{}
	lo, hi := max(0, cur.Row-3000), min(len(b.Lines)-1, cur.Row+3000)
	pr := []rune(prefix)
	for r := lo; r <= hi; r++ {
		l := b.Lines[r]
		for i := 0; i < len(l); {
			if !isWord(l[i]) {
				i++
				continue
			}
			j := i
			for j < len(l) && isWord(l[j]) {
				j++
			}
			if j-i > len(pr) && hasRunePrefix(l[i:j], pr) && !(r == cur.Row && j == cur.Col) {
				w := string(l[i:j])
				d := r - cur.Row
				if d < 0 {
					d = -d
				}
				if old, ok := near[w]; !ok || d < old {
					near[w] = d
				}
			}
			i = j
		}
	}
	words := make([]string, 0, len(near))
	for w := range near {
		words = append(words, w)
	}
	sort.Slice(words, func(i, j int) bool {
		if near[words[i]] != near[words[j]] {
			return near[words[i]] < near[words[j]]
		}
		return words[i] < words[j]
	})
	return words
}

func hasRunePrefix(w, p []rune) bool {
	if len(w) < len(p) {
		return false
	}
	for i := range p {
		if w[i] != p[i] {
			return false
		}
	}
	return true
}

// suggestKey handles a key while the list is open. It says false for keys
// that close the list and then do what they always do.
func (m *Model) suggestKey(name string) bool {
	s := m.suggestOn()
	if s == nil {
		m.sugg = nil
		return false
	}
	switch name {
	case "tab":
		m.takeSuggestion()
		return true
	case "down":
		s.sel = (s.sel + 1) % len(s.items)
		return true
	case "up":
		s.sel = (s.sel + len(s.items) - 1) % len(s.items)
		return true
	case "esc":
		m.sugg = nil
		return true
	}
	m.sugg = nil
	return false
}

// takeSuggestion puts the chosen word in place of what is typed of it.
func (m *Model) takeSuggestion() {
	s := m.sugg
	m.sugg = nil
	b := m.buf
	row, col := b.Cursor.Row, b.Cursor.Col
	word := s.items[s.sel]
	b.BreakUndo()
	if s.kind == "prop" {
		rest := strings.TrimSpace(string(m.line(row)[col:]))
		switch {
		case strings.HasPrefix(rest, ":"):
			b.Change(Pos{row, s.start}, Pos{row, col}, word, "")
		case m.webSyntax() == "sass" || m.webSyntax() == "stylus" || rest != "" && rest != "}":
			b.Change(Pos{row, s.start}, Pos{row, col}, word+": ", "")
		default:
			end := b.Change(Pos{row, s.start}, Pos{row, col}, word+": ;", "")
			b.Cursor = Pos{row, end.Col - 1}
		}
		b.BreakUndo()
		m.refreshSuggest(true) // the values it takes
		m.ensureVisible()
		return
	}
	b.Change(Pos{row, s.start}, Pos{row, col}, word, "")
	b.BreakUndo()
	m.ensureVisible()
}

// suggestBox draws the list; x is where the word starts on screen.
func (m *Model) suggestBox(s *suggestions) string {
	first := 0
	if s.sel >= suggestShown {
		first = s.sel - suggestShown + 1
	}
	last := min(len(s.items), first+suggestShown)
	width := 0
	for _, w := range s.items[first:last] {
		width = max(width, lipgloss.Width(w))
	}
	width += 2
	var rows []string
	for i := first; i < last; i++ {
		st := lipgloss.NewStyle().Width(width).Padding(0, 1).Background(hex(cBarBG)).Foreground(hex(cText))
		if i == s.sel {
			st = st.Background(hex(cSelection)).Foreground(hex(cSelectedText)).Bold(true)
		}
		rows = append(rows, st.Render(s.items[i]))
	}
	hint := "Tab"
	if len(s.items) > suggestShown {
		hint = "Tab · " + strconv.Itoa(s.sel+1) + "/" + strconv.Itoa(len(s.items))
	}
	rows = append(rows, lipgloss.NewStyle().Width(width).Padding(0, 1).Background(hex(cBarBG)).
		Foreground(hex(cDim)).Render(hint))
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// overlaySuggest puts the list under the word being typed (over it near the bottom).
func (m *Model) overlaySuggest(screen string) string {
	s := m.suggestOn()
	if s == nil {
		return screen
	}
	c := m.cursor()
	if c == nil {
		return screen
	}
	box := m.suggestBox(s)
	bw, bh := lipgloss.Width(box), lipgloss.Height(box)
	l := m.line(s.row)
	x := c.X - (colToX(l, m.buf.Cursor.Col) - colToX(l, s.start)) - 1
	x = max(0, min(x, m.width-bw))
	y := c.Y + 1
	if y+bh > textTop+m.textHeight() {
		y = c.Y - bh
	}
	if y < textTop {
		return screen
	}
	return lipgloss.NewCompositor(
		lipgloss.NewLayer(screen),
		lipgloss.NewLayer(box).X(x).Y(y).Z(2),
	).Render()
}
