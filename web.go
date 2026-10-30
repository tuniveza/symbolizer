package main

// Emmet in the editor: Tab and Ctrl+E expand abbreviations, Emmet's actions
// on tags and numbers, tags that close and rename themselves, and auto-correct
// for CSS and file paths. The work itself is done in the emmet package.

import (
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"symbolizer/emmet"
)

// snippetStops are the places Tab still jumps to after an expansion. Each is
// kept as its distance from the end of the text, so typing before it (in an
// earlier stop) does not move it.
type snippetStops struct {
	buf      *Buffer
	from     int // where the expansion starts (letters from the top)
	toEnd    int // where it ends (letters from the end)
	stops    [][2]int
	lastWrap string // the last abbreviation used to wrap
	lastTag  string // the last Update tag answer

	// auto-correct: what the last fix changed, and fixes taken back with Ctrl+Z
	fixVersion int
	fixedFrom  []string
	noFix      map[string]bool
}

// webSyntax gives the Emmet syntax at the cursor, or "" where Emmet is off.
func (m *Model) webSyntax() string {
	syn := ""
	switch strings.ToLower(filepath.Ext(m.buf.Path)) {
	case ".jsx", ".tsx":
		syn = "jsx"
	case ".xhtml":
		syn = "xhtml"
	case ".xml", ".svg", ".xsl", ".xslt", ".plist", ".xaml":
		syn = "xml"
	case ".less":
		syn = "less"
	case ".astro", ".njk", ".liquid", ".ejs", ".hbs", ".handlebars", ".mustache":
		syn = "html"
	}
	if syn == "" {
		switch strings.ToLower(m.hl.Name) {
		case "html", "vue", "svelte", "php", "erb", "twig", "handlebars", "django/jinja", "genshi html", "html+php":
			syn = "html"
		case "react":
			syn = "jsx"
		case "xml":
			syn = "xml"
		case "css", "scss", "sass", "stylus":
			syn = strings.ToLower(m.hl.Name)
		}
	}
	if (syn == "html" || syn == "xhtml") && m.inStyleBlock() {
		return "css"
	}
	return syn
}

// inStyleBlock tells whether the cursor is between <style> and </style>.
func (m *Model) inStyleBlock() bool {
	c := m.buf.Cursor
	for r := c.Row; r >= 0 && r > c.Row-3000; r-- {
		l := strings.ToLower(string(m.line(r)))
		if r == c.Row {
			l = strings.ToLower(string(m.line(r)[:c.Col]))
		}
		open, shut := strings.LastIndex(l, "<style"), strings.LastIndex(l, "</style")
		if open >= 0 || shut >= 0 {
			return open > shut
		}
	}
	return false
}

// webCode tells whether paths in this file are worth checking (web files and scripts).
func (m *Model) webCode() bool {
	if m.webSyntax() != "" {
		return true
	}
	switch strings.ToLower(m.hl.Name) {
	case "javascript", "typescript", "react", "tsx", "jsx":
		return true
	}
	return false
}

func (m *Model) emmetOpts(syn string) emmet.Options {
	return emmet.Options{Syntax: syn, Indent: m.buf.Indent, BaseIndent: leadingSpace(m.line(m.buf.Cursor.Row))}
}

// ---- letters and places ------------------------------------------------------------

func (m *Model) allRunes() []rune {
	n := 0
	for _, l := range m.buf.Lines {
		n += len(l) + 1
	}
	out := make([]rune, 0, n)
	for i, l := range m.buf.Lines {
		if i > 0 {
			out = append(out, '\n')
		}
		out = append(out, l...)
	}
	return out
}

func (m *Model) textLen() int {
	n := len(m.buf.Lines) - 1
	for _, l := range m.buf.Lines {
		n += len(l)
	}
	return n
}

func (m *Model) posToOff(p Pos) int {
	n := 0
	for r := 0; r < p.Row && r < len(m.buf.Lines); r++ {
		n += len(m.buf.Lines[r]) + 1
	}
	return n + p.Col
}

func (m *Model) offToPos(o int) Pos {
	for r, l := range m.buf.Lines {
		if o <= len(l) {
			return Pos{r, max(o, 0)}
		}
		o -= len(l) + 1
	}
	return m.endOfFile()
}

// applyChange makes an emmet.Change as one undo step and puts the cursor where it says.
func (m *Model) applyChange(ch emmet.Change) {
	b := m.buf
	edits := append([]emmet.Edit(nil), ch.Edits...)
	sort.Slice(edits, func(i, j int) bool { return edits[i].Start > edits[j].Start })
	b.BreakUndo()
	b.Group(func() {
		for _, e := range edits {
			b.Change(m.offToPos(e.Start), m.offToPos(e.End), e.New, "")
		}
	})
	b.BreakUndo()
	m.selectOffs(emmet.Range{Start: ch.Sel.Start, End: ch.Sel.End})
	if ch.Note != "" {
		m.say(sentence(ch.Note))
	}
}

// selectOffs highlights a stretch (or just places the cursor when it is empty).
func (m *Model) selectOffs(r emmet.Range) {
	b := m.buf
	if r.Start == r.End {
		b.Cursor, b.Anchor = m.offToPos(r.Start), nil
	} else {
		a := m.offToPos(r.Start)
		b.Anchor, b.Cursor = &a, m.offToPos(r.End)
	}
	b.WantCol = -1
	m.ensureVisible()
}

// selRange is the highlight (or the cursor) as letters from the top.
func (m *Model) selRange() emmet.Range {
	if s, e, ok := m.buf.Selection(); ok {
		return emmet.Range{Start: m.posToOff(s), End: m.posToOff(e)}
	}
	c := m.posToOff(m.buf.Cursor)
	return emmet.Range{Start: c, End: c}
}

func sentence(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	s = string(r)
	if !strings.HasSuffix(s, ".") {
		s += "."
	}
	return s
}

// ---- expanding ---------------------------------------------------------------------

// tabKey: jump to the next place in a fresh expansion, else expand the
// abbreviation before the cursor, else indent.
func (m *Model) tabKey() {
	if _, _, sel := m.buf.Selection(); !sel {
		if m.nextStop() || m.expand(false) {
			return
		}
	}
	m.tab()
}

// expand turns the abbreviation before the cursor into text. force (Ctrl+E)
// expands any word; Tab only what is clearly an abbreviation.
func (m *Model) expand(force bool) bool {
	syn := m.webSyntax()
	if (syn == "" && m.hl.Name == "Text" || syn == "html") && m.pageTemplate() {
		return true
	}
	if syn == "" {
		if force {
			m.say("Emmet works in HTML, CSS, JSX and XML files. This is " + m.hl.Name + ".")
		}
		return force
	}
	b := m.buf
	row, col := b.Cursor.Row, b.Cursor.Col
	line := m.line(row)
	opt := m.emmetOpts(syn)
	var start int
	var exp emmet.Expansion
	if force {
		abbr, s, ok := emmet.Extract(line, col, syn)
		if !ok {
			m.say("No abbreviation before the cursor.")
			return true
		}
		e, err := emmet.Expand(abbr, opt)
		if err != nil {
			m.sayErr("Cannot expand " + abbr + ": " + err.Error())
			return true
		}
		start, exp = s, e
	} else {
		if emmet.IsStylesheet(syn) && syn != "sass" && syn != "stylus" && !m.insideBraces() {
			return false // a selector, not a property
		}
		s, e, ok := emmet.TabExpand(line, col, opt)
		if !ok {
			return false
		}
		start, exp = s, e
	}
	from := Pos{row, start}
	m.putExpansion(from, b.Cursor, exp)
	if exp.Note != "" {
		m.say(sentence(exp.Note))
	}
	return true
}

// templateWords, typed alone in an empty file, give the HTML5 page: Emmet's
// "!" and "html:5", and the "html" / "html:" people reach for first.
var templateWords = map[string]bool{"!": true, "html:5": true, "html": true, "html:": true, "doc": true}

// pageTemplate is Tab on a template word that is all the file holds, in an
// HTML file or one with no language yet (a new tab, a .txt). A file with no
// language is coloured as HTML from then on.
func (m *Model) pageTemplate() bool {
	b := m.buf
	row, col := b.Cursor.Row, b.Cursor.Col
	word := strings.TrimSpace(string(m.line(row)))
	if !templateWords[word] || col != len(strings.TrimRight(string(m.line(row)), " \t")) {
		return false
	}
	for r := range b.Lines {
		if r != row && strings.TrimSpace(string(b.Lines[r])) != "" {
			return false
		}
	}
	exp, err := emmet.Expand("!", m.emmetOpts("html"))
	if err != nil {
		return false
	}
	wasText := m.webSyntax() == ""
	m.putExpansion(Pos{row, 0}, b.Cursor, exp)
	if wasText {
		m.resetHighlighter()
		if m.webSyntax() == "" { // a .txt keeps its name's colours: use HTML's
			m.hl = NewHighlighter("page.html", "")
			m.buf.OnSplice = m.hl.Splice
			m.tabs[m.cur].hl = m.hl
		}
		m.say("HTML5 page. Tab goes to the title, then the body. Save it with a .html name to keep it HTML.")
	} else {
		m.say("HTML5 page. Tab goes to the title, then the body.")
	}
	return true
}

// putExpansion replaces from..to with the expansion and starts Tab's stops.
func (m *Model) putExpansion(from, to Pos, exp emmet.Expansion) {
	b := m.buf
	b.BreakUndo()
	b.Change(from, to, exp.Text, "")
	b.BreakUndo()
	startOff := m.posToOff(from)
	total := m.textLen()
	m.snip.buf, m.snip.stops = b, nil
	m.snip.from, m.snip.toEnd = startOff, total-(startOff+len([]rune(exp.Text)))
	for _, s := range exp.Stops {
		m.snip.stops = append(m.snip.stops, [2]int{total - (startOff + s.Start), total - (startOff + s.End)})
	}
	if !m.nextStop() {
		m.ensureVisible()
	}
}

// nextStop moves to the next stop of the last expansion, if the cursor is still in it.
func (m *Model) nextStop() bool {
	s := &m.snip
	if s.buf != m.buf || len(s.stops) == 0 {
		s.stops = nil
		return false
	}
	total := m.textLen()
	c := m.posToOff(m.buf.Cursor)
	if c < s.from || c > total-s.toEnd {
		s.stops = nil
		return false
	}
	st := s.stops[0]
	s.stops = s.stops[1:]
	a, e := total-st[0], total-st[1]
	if a < s.from || e > total-s.toEnd || a > e {
		s.stops = nil
		return false
	}
	m.selectOffs(emmet.Range{Start: a, End: e})
	return true
}

// insideBraces tells whether the cursor is inside a { } block (for CSS properties).
func (m *Model) insideBraces() bool {
	depth := 0
	c := m.buf.Cursor
	inComment := false
	for r := 0; r <= c.Row; r++ {
		l := m.line(r)
		if r == c.Row {
			l = l[:c.Col]
		}
		for i := 0; i < len(l); i++ {
			switch {
			case inComment:
				if l[i] == '*' && i+1 < len(l) && l[i+1] == '/' {
					inComment = false
					i++
				}
			case l[i] == '/' && i+1 < len(l) && l[i+1] == '*':
				inComment = true
				i++
			case l[i] == '{':
				depth++
			case l[i] == '}':
				depth = max(0, depth-1)
			}
		}
	}
	if m.webSyntax() == "css" && m.hl.Name != "CSS" {
		return true // inside <style>: a bare property line still counts
	}
	return depth > 0
}

// ctrlE expands, or wraps the highlighted text.
func (m *Model) ctrlE() tea.Cmd {
	if _, _, ok := m.buf.Selection(); ok {
		return m.askWrap()
	}
	m.expand(true)
	return nil
}

func (m *Model) askWrap() tea.Cmd {
	syn := m.webSyntax()
	if !emmet.IsMarkup(syn) {
		m.say("Wrapping works in HTML, JSX and XML files.")
		return nil
	}
	return m.ask(pWrap, "Wrap with: ", m.snip.lastWrap)
}

// wrapWith wraps the highlighted text (or the current line) in the abbreviation.
func (m *Model) wrapWith(abbr string) tea.Cmd {
	abbr = strings.TrimSpace(abbr)
	if abbr == "" {
		m.say("Cancelled.")
		return nil
	}
	m.snip.lastWrap = abbr
	b := m.buf
	s, e, ok := b.Selection()
	if !ok {
		row := b.Cursor.Row
		s, e = Pos{row, len(leadingSpace(m.line(row)))}, Pos{row, len(m.line(row))}
	}
	// Whole lines highlighted: wrap from the first line's indent to the end of the last.
	if e.Col == 0 && e.Row > s.Row {
		e = Pos{e.Row - 1, len(m.line(e.Row - 1))}
	}
	if ind := len(leadingSpace(m.line(s.Row))); s.Col <= ind {
		s.Col = ind
	}
	opt := m.emmetOpts(m.webSyntax())
	opt.BaseIndent = leadingSpace(m.line(s.Row))
	exp, err := emmet.Wrap(abbr, b.Get(s, e), opt)
	if err != nil {
		m.sayErr("Cannot wrap with " + abbr + ": " + err.Error())
		return nil
	}
	b.ClearSelect()
	m.putExpansion(s, e, exp)
	return nil
}

// ---- Emmet's actions ------------------------------------------------------------------

func (m *Model) needMarkup(what string) (string, bool) {
	syn := m.webSyntax()
	if !emmet.IsMarkup(syn) {
		m.say(what + " works in HTML, JSX and XML files.")
		return syn, false
	}
	return syn, true
}

func (m *Model) balance(out bool) {
	if _, ok := m.needMarkup("Balance"); !ok {
		return
	}
	var r emmet.Range
	var ok bool
	if out {
		r, ok = emmet.BalanceOut(m.allRunes(), m.selRange())
	} else {
		r, ok = emmet.BalanceIn(m.allRunes(), m.selRange())
	}
	if !ok {
		m.say("No tag to highlight.")
		return
	}
	m.selectOffs(r)
}

func (m *Model) matchingPair() {
	if _, ok := m.needMarkup("Go to the matching tag"); !ok {
		return
	}
	p, ok := emmet.MatchingPair(m.allRunes(), m.posToOff(m.buf.Cursor))
	if !ok {
		m.say("The cursor is not in a tag that has a pair.")
		return
	}
	m.selectOffs(emmet.Range{Start: p, End: p})
}

// tagChange runs a tag action that gives a Change.
func (m *Model) tagChange(what string, fn func(text []rune, pos int, opt emmet.Options) (emmet.Change, bool)) {
	syn, ok := m.needMarkup(what)
	if !ok {
		return
	}
	ch, ok := fn(m.allRunes(), m.posToOff(m.buf.Cursor), m.emmetOpts(syn))
	if !ok {
		m.say("No tag at the cursor.")
		return
	}
	m.applyChange(ch)
}

func (m *Model) askUpdateTag() tea.Cmd {
	if _, ok := m.needMarkup("Update tag"); !ok {
		return nil
	}
	return m.ask(pUpdateTag, "Update tag (name .class #id [attr=value]): ", m.snip.lastTag)
}

func (m *Model) updateTag(abbr string) tea.Cmd {
	abbr = strings.TrimSpace(abbr)
	if abbr == "" {
		m.say("Cancelled.")
		return nil
	}
	m.snip.lastTag = abbr
	m.tagChange("Update tag", func(t []rune, p int, o emmet.Options) (emmet.Change, bool) {
		return emmet.UpdateTag(t, p, abbr, o)
	})
	return nil
}

func (m *Model) editPoint(d int) {
	p, ok := emmet.NextEditPoint(m.allRunes(), m.posToOff(m.buf.Cursor), d)
	if !ok {
		m.say("No more empty places to fill in.")
		return
	}
	m.selectOffs(emmet.Range{Start: p, End: p})
}

func (m *Model) selectItem(d int) {
	syn := m.webSyntax()
	if syn == "" {
		m.say("Select item works in HTML, CSS, JSX and XML files.")
		return
	}
	r, ok := emmet.SelectItem(m.allRunes(), m.selRange(), d, syn)
	if !ok {
		m.say("No more items.")
		return
	}
	m.selectOffs(r)
}

func (m *Model) evalMath() {
	ch, ok := emmet.EvalMath(m.allRunes(), m.selRange())
	if !ok {
		m.say("No sum before the cursor (for example 10*3+2).")
		return
	}
	m.applyChange(ch)
}

func (m *Model) incNumber(d float64) {
	ch, ok := emmet.IncrementNumber(m.allRunes(), m.selRange(), d)
	if !ok {
		m.say("No number at the cursor.")
		return
	}
	m.applyChange(ch)
}

func (m *Model) imageSize() {
	syn := m.webSyntax()
	if syn == "" {
		m.say("Image size works in HTML, CSS, JSX and XML files.")
		return
	}
	ch, err := emmet.UpdateImageSize(m.allRunes(), m.posToOff(m.buf.Cursor), m.emmetOpts(syn), m.fileDir())
	if err != nil {
		m.sayErr(sentence(err.Error()))
		return
	}
	if len(ch.Edits) == 0 {
		m.say("No image at the cursor.")
		return
	}
	m.applyChange(ch)
}

// blockComment is Ctrl+/ in web files: <!-- --> or /* */. It says false
// where line comments are used instead.
func (m *Model) blockComment() bool {
	syn := m.webSyntax()
	if syn == "" || syn == "sass" || syn == "stylus" {
		return false
	}
	ch, ok := emmet.ToggleBlockComment(m.allRunes(), m.selRange(), syn)
	if !ok {
		m.say("Nothing to comment out here.")
		return true
	}
	m.applyChange(ch)
	return true
}

// fileDir is the folder paths in the file are relative to.
func (m *Model) fileDir() string {
	if m.buf.Path != "" && !m.buf.Draft {
		return filepath.Dir(m.buf.Path)
	}
	return m.startDir
}

// pathList offers the files that fit the path being typed (Alt+P).
func (m *Model) pathList() tea.Cmd {
	text := m.allRunes()
	pos := m.posToOff(m.buf.Cursor)
	names := emmet.PathCandidates(text, pos, m.fileDir())
	if len(names) == 0 {
		m.say("No files fit the path before the cursor.")
		return nil
	}
	// what is typed of the last part of the path
	l := m.line(m.buf.Cursor.Row)
	i := m.buf.Cursor.Col
	for i > 0 && !strings.ContainsRune("\"'`( =/\t", l[i-1]) {
		i--
	}
	typed := m.buf.Cursor.Col - i
	p := &picker{title: "Paths", note: "Files and folders that fit what is typed. Folders end in /."}
	for _, n := range names {
		base := filepath.Base(strings.TrimSuffix(n, "/"))
		if strings.HasSuffix(n, "/") {
			base += "/"
		}
		p.items = append(p.items, pickItem{
			label: stHelpText.Render(n),
			open: func(m *Model) tea.Cmd {
				row, col := m.buf.Cursor.Row, m.buf.Cursor.Col
				m.buf.BreakUndo()
				m.buf.Change(Pos{row, col - typed}, Pos{row, col}, base, "")
				m.buf.BreakUndo()
				m.ensureVisible()
				return nil
			},
		})
	}
	m.picker = p
	return nil
}

// webMenu (Alt+W) lists every web action with its key.
func (m *Model) webMenu() tea.Cmd {
	type act struct {
		keys, what string
		fn         command
	}
	acts := []act{
		{"Tab / Ctrl+E", "expand the abbreviation", func(m *Model) tea.Cmd { m.expand(true); return nil }},
		{"Ctrl+E on highlight", "wrap it in an abbreviation", func(m *Model) tea.Cmd { return m.askWrap() }},
		{"Alt+U", "highlight the tag around", func(m *Model) tea.Cmd { m.balance(true); return nil }},
		{"", "highlight inside the tag", func(m *Model) tea.Cmd { m.balance(false); return nil }},
		{"Alt+G", "go to the matching tag", func(m *Model) tea.Cmd { m.matchingPair(); return nil }},
		{"Alt+R", "remove the tag, keep its inside", func(m *Model) tea.Cmd {
			m.tagChange("Remove tag", emmet.RemoveTag)
			return nil
		}},
		{"Alt+K", "split or join: <p></p> ↔ <p/>", func(m *Model) tea.Cmd {
			m.tagChange("Split / join tag", emmet.SplitJoinTag)
			return nil
		}},
		{"Alt+Y", "update: name .class #id [a=b]", func(m *Model) tea.Cmd { return m.askUpdateTag() }},
		{"Alt+. / Alt+,", "next / previous empty place", func(m *Model) tea.Cmd { m.editPoint(1); return nil }},
		{"Alt+] / Alt+[", "next / previous item", func(m *Model) tea.Cmd { m.selectItem(1); return nil }},
		{"Alt+X", "work out the sum", func(m *Model) tea.Cmd { m.evalMath(); return nil }},
		{"Alt+= / Alt+-", "number ±1 (Shift: ±10)", func(m *Model) tea.Cmd { m.incNumber(1); return nil }},
		{"Alt+I", "image width and height", func(m *Model) tea.Cmd { m.imageSize(); return nil }},
		{"Alt+P", "files that fit the path", func(m *Model) tea.Cmd { return m.pathList() }},
		{"Ctrl+/", "comment: <!-- --> or /* */", func(m *Model) tea.Cmd {
			if !m.blockComment() {
				m.toggleComment()
			}
			return nil
		}},
	}
	p := &picker{title: "Web (Emmet)", note: "Enter does it. Or press the key in the text."}
	for _, a := range acts {
		p.items = append(p.items, pickItem{
			label: stHelpKey.Render(padRight(a.keys, 20)) + stHelpText.Render(a.what),
			open:  a.fn,
		})
	}
	m.picker = p
	return nil
}

func padRight(s string, n int) string {
	if w := len([]rune(s)); w < n {
		return s + strings.Repeat(" ", n-w)
	}
	return s + " "
}

// ---- while typing -------------------------------------------------------------------

// tagPair is the tag name being edited and its partner, found before a key.
type tagPair struct {
	ok          bool
	edited, the emmet.Range
	total       int
}

// pairBefore looks for a tag name at the cursor before a letter is typed or deleted.
func (m *Model) pairBefore() tagPair {
	if !emmet.IsMarkup(m.webSyntax()) || len(m.buf.Lines) > 20000 {
		return tagPair{}
	}
	if _, _, sel := m.buf.Selection(); sel {
		return tagPair{}
	}
	if !strings.ContainsRune(string(m.line(m.buf.Cursor.Row)[:m.buf.Cursor.Col]), '<') {
		return tagPair{}
	}
	text := m.allRunes()
	c := m.posToOff(m.buf.Cursor)
	open, shut, ok := emmet.TagNamePair(text, c)
	if !ok {
		return tagPair{}
	}
	tp := tagPair{ok: true, total: len(text)}
	if c >= open.Start && c <= open.End {
		tp.edited, tp.the = open, shut
	} else if c >= shut.Start && c <= shut.End {
		tp.edited, tp.the = shut, open
	} else {
		return tagPair{}
	}
	if string(text[tp.edited.Start:tp.edited.End]) != string(text[tp.the.Start:tp.the.End]) {
		return tagPair{} // already different: leave both alone
	}
	return tp
}

// mirrorTag gives the partner tag the edited tag's new name.
func (m *Model) mirrorTag(tp tagPair) {
	if !tp.ok {
		return
	}
	delta := m.textLen() - tp.total
	if delta == 0 {
		return
	}
	end := tp.edited.End + delta
	c := m.posToOff(m.buf.Cursor)
	if end < tp.edited.Start || c < tp.edited.Start || c > end {
		return
	}
	name := m.buf.Get(m.offToPos(tp.edited.Start), m.offToPos(end))
	for _, r := range name {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("-_:.", r)) {
			return
		}
	}
	other := tp.the
	if other.Start > tp.edited.Start {
		other.Start += delta
		other.End += delta
	}
	m.buf.Change(m.offToPos(other.Start), m.offToPos(other.End), name, "")
	if other.Start < tp.edited.Start {
		c += len([]rune(name)) - (other.End - other.Start)
	}
	m.buf.Cursor = m.offToPos(c)
	m.buf.BreakUndo()
}

// afterTyped runs after a letter is typed: close tags, fix CSS and paths.
func (m *Model) afterTyped(s string) {
	syn := m.webSyntax()
	switch {
	case (s == ">" || s == "/") && emmet.IsMarkup(syn):
		text := m.allRunes()
		add, ok := emmet.AutoCloseTag(text, m.posToOff(m.buf.Cursor), syn)
		if !ok {
			return
		}
		b := m.buf
		c := b.Cursor
		b.BreakUndo()
		end := b.Change(c, c, add, "")
		b.BreakUndo()
		if s == ">" {
			b.Cursor = c // the cursor stays between the tags
		} else {
			b.Cursor = end
		}
	case (s == ";" || s == ":") && emmet.IsStylesheet(syn):
		m.correctCSS()
	case s == "\"" || s == "'" || s == ")" || s == "`":
		m.correctPath()
	}
}

// correctCSS fixes typing slips on the cursor's line of CSS.
func (m *Model) correctCSS() {
	if !m.insideBraces() && m.webSyntax() != "sass" && m.webSyntax() != "stylus" {
		return
	}
	b := m.buf
	row := b.Cursor.Row
	edits, note := emmet.CorrectCSSLine(m.line(row))
	if len(edits) == 0 {
		return
	}
	l := m.line(row)
	kept := edits[:0]
	var from []string
	for _, e := range edits {
		if old := string(l[e.Start:e.End]); !m.snip.noFix[old] {
			kept = append(kept, e)
			from = append(from, old)
		}
	}
	edits = kept
	if len(edits) == 0 {
		return
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].Start > edits[j].Start })
	col := b.Cursor.Col
	shift := 0
	for _, e := range edits {
		if e.End <= col {
			shift += len([]rune(e.New)) - (e.End - e.Start)
		}
	}
	b.BreakUndo()
	b.Group(func() {
		for _, e := range edits {
			b.Change(Pos{row, e.Start}, Pos{row, e.End}, e.New, "")
		}
	})
	b.BreakUndo()
	b.Cursor = b.Clamp(Pos{row, col + shift})
	m.rememberFix(from)
	if note == "" {
		note = "fixed a typing slip"
	}
	m.say(fixNote(note))
}

// fixNote is the message after an auto-correct.
func fixNote(note string) string {
	return strings.TrimSuffix(sentence(note), ".") + " · Ctrl+Z undoes"
}

// correctPath fixes a file path that was just finished, if it points nowhere.
func (m *Model) correctPath() {
	if !m.webCode() || len(m.buf.Lines) > 20000 {
		return
	}
	text := m.allRunes()
	ch, ok := emmet.CorrectPathAt(text, m.posToOff(m.buf.Cursor), m.fileDir())
	if !ok || len(ch.Edits) == 0 {
		return
	}
	var from []string
	for _, e := range ch.Edits {
		old := string(text[e.Start:e.End])
		if m.snip.noFix[old] {
			return
		}
		from = append(from, old)
	}
	note := ch.Note
	ch.Note = ""
	m.applyChange(ch)
	m.rememberFix(from)
	if note == "" {
		note = "path fixed"
	}
	m.say(fixNote(note))
}

// beforeNewline runs on Enter before the new line: the same fixes as typing.
func (m *Model) beforeNewline() {
	switch syn := m.webSyntax(); {
	case emmet.IsStylesheet(syn):
		m.correctCSS()
	}
	m.correctPath()
}

// splitPair handles Enter between <div>|</div> or {|}: the inside gets its own line.
func (m *Model) splitPair() bool {
	syn := m.webSyntax()
	if syn == "" {
		return false
	}
	if _, _, sel := m.buf.Selection(); sel {
		return false
	}
	l, col := m.line(m.buf.Cursor.Row), m.buf.Cursor.Col
	if col == 0 || col >= len(l) {
		return false
	}
	before, after := string(l[:col]), string(l[col:])
	tag := emmet.IsMarkup(syn) && strings.HasSuffix(before, ">") && strings.HasPrefix(after, "</") &&
		!strings.HasSuffix(before, "/>") && !strings.HasSuffix(before, "-->")
	brace := strings.HasSuffix(strings.TrimRight(before, " "), "{") && strings.HasPrefix(after, "}")
	if !tag && !brace {
		return false
	}
	indent := leadingSpace(l)
	b := m.buf
	b.Insert("\n"+indent+b.Indent+"\n"+indent, "")
	b.Cursor = Pos{b.Cursor.Row - 1, len([]rune(indent + b.Indent))}
	m.ensureVisible()
	return true
}

// rememberFix notes what an auto-correct changed, so Ctrl+Z straight after
// it can tell sy to leave those words alone from now on.
func (m *Model) rememberFix(from []string) {
	m.snip.fixVersion, m.snip.fixedFrom = m.buf.Version(), from
}

// fixTakenBack runs before Ctrl+Z: undoing a fix means "I meant that".
func (m *Model) fixTakenBack() {
	if m.snip.fixedFrom == nil || m.buf.Version() != m.snip.fixVersion {
		return
	}
	if m.snip.noFix == nil {
		m.snip.noFix = map[string]bool{}
	}
	for _, w := range m.snip.fixedFrom {
		if strings.TrimSpace(w) == "" {
			continue // an added letter or space: nothing to remember
		}
		m.snip.noFix[w] = true
	}
	m.snip.fixedFrom = nil
}
