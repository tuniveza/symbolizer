package main

// Learn (AltGr + L): a calm box with the recommended way to write the
// language of the file. It opens on the card for the file's ending, at the
// section and line for what the cursor is on: on 16px it opens Units at px.
//
//	Up / Down      the line before / after
//	Left / Right   the section before / after
//	Tab            the next language (Shift + Tab: the one before)
//	Esc, AltGr + L, Enter   close
//
// The layout never changes: title, language, section, the list of lines,
// then the chosen line in full (what it is, and what it is like), then keys.

import (
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var learnTopics = []*learnTopic{
	&learnCSS, &learnHTML, &learnJavaScript, &learnTypeScript, &learnMarkdown, &learnJSON,
	&learnXML, &learnGo, &learnPython, &learnRust, &learnC, &learnJava, &learnKotlin, &learnCSharp,
	&learnPHP, &learnSQL, &learnLua, &learnShell,
}

type learnState struct {
	topic, sec, item int
	found            string // what under the cursor opened it ("" when nothing did)
}

// openLearn opens (or closes) the box at what the cursor is on.
func (m *Model) openLearn() tea.Cmd {
	if m.learn != nil {
		m.learn = nil
		return nil
	}
	m.help, m.picker, m.modesOpen, m.themeOpen, m.sugg = false, nil, false, false, nil
	m.closePrompt()
	ti, ok := m.learnTopicFor()
	st := &learnState{topic: ti}
	st.sec, st.item, st.found = learnFind(learnTopics[ti], m.cursorWords(), m.learnHint())
	m.learn = st
	if !ok {
		m.say("No card for " + m.hl.Name + " yet. Tab shows the other languages.")
	}
	return nil
}

// learnHint names the kind of file for cards that hold several (YAML on
// the JSON / YAML / TOML card, C on the C / C++ card).
func (m *Model) learnHint() string {
	switch e := strings.ToLower(filepath.Ext(m.buf.Path)); e {
	case ".yml", ".yaml":
		return "yaml"
	case ".c", ".h":
		return "c"
	case "":
		return ""
	default:
		return e[1:]
	}
}

// learnTopicFor picks the card by the file's ending, then by its language.
func (m *Model) learnTopicFor() (int, bool) {
	ext := strings.ToLower(filepath.Ext(m.buf.Path))
	if m.buf.Draft {
		ext = ""
	}
	wants := []string{}
	if m.webSyntax() == "css" {
		wants = append(wants, "css") // inside <style>
	}
	if ext != "" {
		wants = append(wants, ext)
	}
	wants = append(wants, strings.ToLower(m.hl.Name))
	for _, w := range wants {
		for i, t := range learnTopics {
			for _, mt := range t.Match {
				if mt == w {
					return i, true
				}
			}
		}
	}
	return 0, false
}

// ---- what the cursor is on ----------------------------------------------------------

// cursorWords lists the words and symbols on the cursor's line, the one
// under the cursor first, then the others by how near they are.
func (m *Model) cursorWords() []string {
	l := m.line(m.buf.Cursor.Row)
	return wordsNear(l, m.buf.Cursor.Col)
}

const learnSymbols = "?.=!<>&|:+*/%^~-#@$`"

// learnFiller are names that stand for 'anything' in a card (function
// name(), $var): they never open a line by themselves.
var learnFiller = map[string]bool{"name": true, "value": true, "var": true, "key": true, "item": true,
	"items": true, "text": true, "file": true, "path": true, "data": true, "list": true, "foo": true,
	"bar": true, "obj": true, "fn": true, "args": true, "arg": true, "msg": true, "err2": true}

func isPieceRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '@' || r == '#' || r == '$' || r == '.' || r == '%'
}

func wordsNear(l []rune, col int) []string {
	type tok struct {
		s, e int
		vs   []string
	}
	var toks []tok
	var later []string
	for i := 0; i < len(l); {
		switch {
		case l[i] == '$' && i+1 < len(l) && (l[i+1] == '(' || l[i+1] == '{'):
			j := i + 2 // $( $(( ${: shell expansions
			if l[i+1] == '(' && j < len(l) && l[j] == '(' {
				j++
			}
			toks = append(toks, tok{i, j, []string{string(l[i:j])}})
			later = append(later, "$") // a plain $ only if nothing nearer says more
			i = j
		case isPieceRune(l[i]) && !(l[i] == '.' && (i+1 >= len(l) || !isPieceRune(l[i+1]))) && !(l[i] == '-' && i+1 < len(l) && l[i+1] == '>'):
			j := i
			for j < len(l) && isPieceRune(l[j]) {
				j++
			}
			// a ":" or "::" right before a name is part of it (:has, ::before)
			s := i
			for s > 0 && l[s-1] == ':' && s > i-2 {
				s--
			}
			vs := pieceVariants(string(l[s:j]))
			if s > 0 && l[s-1] == '<' || s > 1 && l[s-1] == '/' && l[s-2] == '<' {
				vs = append([]string{"<" + strings.ToLower(string(l[s:j]))}, vs...) // a tag: <img
			}
			toks = append(toks, tok{s, j, vs})
			i = j
		case strings.ContainsRune(learnSymbols, l[i]):
			j := i
			for j < len(l) && l[j] == ':' {
				j++
			}
			if j > i && j < len(l) && unicode.IsLetter(l[j]) {
				i = j // :has, ::before: the name takes its colons
				continue
			}
			j = i
			for j < len(l) && strings.ContainsRune(learnSymbols, l[j]) && !(j > i && isPieceRune(l[j]) && l[j] != '-' && l[j] != '.') {
				j++
			}
			toks = append(toks, tok{i, j, []string{string(l[i:j])}})
			i = j
		default:
			i++
		}
	}
	// how far from the cursor: inside a token beats touching its end
	dist := func(t tok) int {
		switch {
		case col >= t.s && col < t.e:
			return 0
		case col == t.e:
			return 1
		case col < t.s:
			return 2 * (t.s - col)
		}
		return 2 * (col - t.e)
	}
	// nearest first; a symbol and a word both touching the cursor: the one it is inside first
	order := make([]tok, len(toks))
	copy(order, toks)
	for i := 1; i < len(order); i++ {
		for j := i; j > 0 && dist(order[j]) < dist(order[j-1]); j-- {
			order[j], order[j-1] = order[j-1], order[j]
		}
	}
	var out []string
	seen := map[string]bool{}
	order = append(order, tok{vs: later})
	for _, t := range order {
		for _, v := range t.vs {
			if v != "" && !seen[v] && !(len([]rune(v)) == 1 && unicode.IsLetter([]rune(v)[0])) {
				seen[v] = true
				out = append(out, v)
			}
		}
	}
	return out
}

// pieceVariants are the ways one word can be looked up: 16px → px, 16px;
// --brand → --brand, --; .card:has → :has, .card, card.
func pieceVariants(p string) []string {
	p = strings.ToLower(strings.TrimRight(p, ".:"))
	var out []string
	// a number with a unit: the unit
	i := 0
	for i < len(p) && (p[i] >= '0' && p[i] <= '9' || p[i] == '.' || p[i] == '-' || p[i] == '+') {
		i++
	}
	if i > 0 && i < len(p) && p[i-1] >= '0' && p[i-1] <= '9' {
		out = append(out, p[i:])
	}
	out = append(out, p)
	if strings.HasPrefix(p, "--") {
		out = append(out, "--")
	}
	for _, sym := range []string{"#", "$"} {
		if strings.HasPrefix(p, sym) && p != sym {
			out = append(out, sym)
		}
	}
	trimmed := strings.TrimLeft(p, ".#@$:")
	if trimmed != p {
		out = append(out, trimmed)
	}
	// obj.method → method, obj
	if strings.Contains(trimmed, ".") {
		parts := strings.Split(trimmed, ".")
		for k := len(parts) - 1; k >= 0; k-- {
			out = append(out, parts[k])
		}
	}
	return out
}

// learnKeys are the words and symbols that open each line of a card, with
// how sure the match is: 4 the whole Use, 3 its first part or a Key, 2 a
// later part, 1 a word inside it.
func learnKeys(it learnItem) map[string]int {
	keys := map[string]int{}
	put := func(k string, n int) {
		k = strings.ToLower(strings.TrimSpace(k))
		if k == "" || k == "…" || n < 3 && learnFiller[k] || strings.Trim(k, "0123456789") == "" {
			return
		}
		if keys[k] < n {
			keys[k] = n
		}
	}
	put(it.Use, 4)
	for _, k := range it.Keys {
		put(k, 3)
	}
	for i, part := range strings.FieldsFunc(it.Use, func(r rune) bool { return r == ' ' || r == '·' || r == ',' }) {
		part = strings.TrimSuffix(strings.Trim(part, ";:,\"'`"), "()")
		switch {
		case learnFiller[strings.ToLower(part)]:
		case i == 0:
			put(part, 3)
		default:
			put(part, 2)
		}
		if j := strings.IndexByte(part, '('); j > 0 && strings.IndexFunc(part[:j], unicode.IsLetter) >= 0 {
			put(part[:j], 3) // clamp(…) → clamp
		}
	}
	for _, w := range wordsNear([]rune(it.Use), 0) {
		if (len([]rune(w)) >= 2 || !unicode.IsLetter([]rune(w)[0])) && strings.Trim(w, "0123456789.%") != "" {
			put(w, 1)
		}
	}
	return keys
}

// learnFind gives the section and line for the words, nearest first. The
// first word that names a line clearly (or a whole section) wins; a word
// that only appears inside a line counts when nothing clearer is found.
// hint ("yaml", "c") prefers sections with that name on mixed cards.
// Nothing found: the first line of the card.
func learnFind(t *learnTopic, words []string, hint ...string) (sec, item int, found string) {
	h := ""
	if len(hint) > 0 {
		h = strings.ToLower(hint[0])
	}
	weakS, weakI, weak := 0, 0, ""
	for _, w := range words {
		best, bs, bi := 0, 0, 0
		for s, section := range t.Sections {
			bonus := 0
			if h != "" && strings.Contains(" "+strings.ToLower(section.Title)+" ", " "+h+" ") {
				bonus = 1
			}
			for i, it := range section.Items {
				if n := learnKeys(it)[w] * 2; n > 0 && n+bonus > best {
					best, bs, bi = n+bonus, s, i
				}
			}
		}
		if best >= 4 {
			return bs, bi, w
		}
		for s, section := range t.Sections {
			for _, k := range section.Keys {
				if k == w {
					return s, 0, w
				}
			}
		}
		if best > 0 && weak == "" {
			weakS, weakI, weak = bs, bi, w
		}
	}
	return weakS, weakI, weak
}

// ---- keys ------------------------------------------------------------------------------

func (m *Model) learnKey(name string) tea.Cmd {
	st := m.learn
	t := learnTopics[st.topic]
	switch name {
	case "esc", "enter", "ł", "altgr+l", "f1":
		m.learn = nil
	case "up":
		st.item = max(0, st.item-1)
	case "down":
		st.item = min(len(t.Sections[st.sec].Items)-1, st.item+1)
	case "left":
		if st.sec > 0 {
			st.sec, st.item = st.sec-1, 0
		}
	case "right":
		if st.sec < len(t.Sections)-1 {
			st.sec, st.item = st.sec+1, 0
		}
	case "home":
		st.sec, st.item = 0, 0
	case "tab", "shift+tab":
		d := 1
		if name == "shift+tab" {
			d = len(learnTopics) - 1
		}
		st.topic = (st.topic + d) % len(learnTopics)
		st.sec, st.item, st.found = 0, 0, ""
	}
	return nil
}

// ---- drawing ---------------------------------------------------------------------------

func (m *Model) learnBox() string {
	st := m.learn
	t := learnTopics[st.topic]
	sec := t.Sections[st.sec]
	it := sec.Items[st.item]
	bg := cBarBG
	w := min(m.width-4, 88)
	inner := w - 4
	style := func(fg string) lipgloss.Style {
		return lipgloss.NewStyle().Background(hex(bg)).Foreground(hex(fg))
	}
	line := func(s string) string { return style(cText).Width(w).Padding(0, 2).Render(s) }
	blank := line("")
	rule := style(cGutterLine).Width(w).Padding(0, 2).Render(strings.Repeat("─", inner))

	var rows []string
	rows = append(rows, lipgloss.NewStyle().Width(w).Align(lipgloss.Center).Background(hex(bg)).
		Render(glowText(icon("\U000f02d1  ", "◆  ")+"L E A R N", bg)))
	rows = append(rows, blank)
	// the language, then where in the card
	lang := style(cAccent).Bold(true).Render(t.Name) + style(cDim).Render("   ·   Tab: another language")
	rows = append(rows, line(lang))
	where := style(cEmber).Bold(true).Render(sec.Title) +
		style(cDim).Render("   ·   section "+strconv.Itoa(st.sec+1)+" of "+strconv.Itoa(len(t.Sections)))
	rows = append(rows, line(where))
	rows = append(rows, rule)

	// the chosen line in full, and the keys: drawn first, so the list
	// gets the height that is left. On a small screen the empty lines and
	// the notes go first, so the words themselves always show.
	height := func(rs []string) int {
		n := 0
		for _, r := range rs {
			n += lipgloss.Height(r)
		}
		return n
	}
	const labelW = 9
	build := func(tight bool) []string {
		var d []string
		gap := func() {
			if !tight {
				d = append(d, blank)
			}
		}
		label := func(name, fg string, body string) {
			head := style(fg).Bold(true).Width(labelW).Render(name)
			d = append(d, lipgloss.NewStyle().Background(hex(bg)).Padding(0, 2).Width(w).
				Render(lipgloss.JoinHorizontal(lipgloss.Top, head, body)))
		}
		text := func(s string) string { return style(cText).Width(inner - labelW).Render(s) }
		d = append(d, rule, line(style(cAccent).Bold(true).Render(it.Use)))
		gap()
		label("What", cEmber, text(it.Why))
		gap()
		label("Like", cStatusText, text(it.Like))
		if it.Example != "" {
			gap()
			var code []string
			for _, l := range strings.Split(it.Example, "\n") {
				code = append(code, style(cGlow).Width(inner-labelW).MaxWidth(inner-labelW).Render(l))
			}
			label("Example", cAccent, lipgloss.JoinVertical(lipgloss.Left, code...))
		}
		d = append(d, rule)
		if !tight && st.found != "" && st.topic == m.learnOpenedTopic() {
			d = append(d, line(style(cDim).Render("Opened here because the cursor is on: "+st.found)))
		}
		return append(d, line(style(cDim).Render("↑ ↓ line  ·  ← → section  ·  Tab language  ·  Esc close")))
	}
	detail := build(false)
	free := m.height - 2 - height(rows) - height(detail)
	if free < min(len(sec.Items), 3) {
		detail = build(true)
		free = m.height - 2 - height(rows) - height(detail)
	}
	source := line(style(cDim).Italic(true).Render("Checked against: " + t.Source))
	if free-lipgloss.Height(source) >= min(len(sec.Items), 3) {
		detail = append(detail, source) // on a small screen the source line makes room
		free -= lipgloss.Height(source)
	}

	// the lines of this section: just what to use; the chosen one marked
	room := min(len(sec.Items), max(free, 1))
	first := max(0, min(st.item-room/2, len(sec.Items)-room))
	for i := first; i < first+room; i++ {
		u := sec.Items[i].Use
		if i == st.item {
			rows = append(rows, lipgloss.NewStyle().Width(w).Padding(0, 2).Background(hex(cSelection)).
				Foreground(hex(cSelectedText)).Bold(true).Render("▸ "+u))
		} else {
			rows = append(rows, line(style(cText).Render("  "+u)))
		}
	}
	rows = append(rows, detail...)
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// learnOpenedTopic is the card the box opened on (the found word belongs to it).
func (m *Model) learnOpenedTopic() int {
	i, _ := m.learnTopicFor()
	return i
}
