package emmet

// Emmet's editing actions on tags: balance, matching pair, remove / split /
// update tag, edit points, select item, block comments, and the auto-close and
// tag-rename helpers. All positions are rune offsets.

import (
	"sort"
	"strings"
)

// ---- small helpers ----

func actContains(outer, in Range) bool { return outer.Start <= in.Start && in.End <= outer.End }

func actLineStart(text []rune, pos int) int {
	for pos > 0 && text[pos-1] != '\n' {
		pos--
	}
	return pos
}

func actLineEnd(text []rune, pos int) int {
	for pos < len(text) && text[pos] != '\n' {
		pos++
	}
	return pos
}

// actNextLineStart gives the position after the '\n' that ends pos's line (or len).
func actNextLineStart(text []rune, pos int) int {
	e := actLineEnd(text, pos)
	if e < len(text) {
		return e + 1
	}
	return e
}

func actBlank(text []rune, s, e int) bool {
	for i := s; i < e; i++ {
		if text[i] != ' ' && text[i] != '\t' && text[i] != '\r' {
			return false
		}
	}
	return true
}

// actLineIndent gives the leading spaces and tabs of the line holding pos.
func actLineIndent(text []rune, pos int) string {
	s := actLineStart(text, pos)
	e := s
	for e < len(text) && (text[e] == ' ' || text[e] == '\t') {
		e++
	}
	return string(text[s:e])
}

// actWholeLine widens [s,e) to its whole line(s), newline included, when
// nothing but blanks shares those lines with it.
func actWholeLine(text []rune, s, e int) (int, int, bool) {
	ls, le := actLineStart(text, s), actLineEnd(text, e)
	if !actBlank(text, ls, s) || !actBlank(text, e, le) {
		return s, e, false
	}
	if le < len(text) {
		return ls, le + 1, true
	}
	if ls > 0 { // last line: take the newline before it instead
		return ls - 1, le, true
	}
	return ls, le, true
}

// actNormalize sorts edits and joins ones that touch, so they never overlap.
func actNormalize(edits []Edit) []Edit {
	sort.SliceStable(edits, func(i, j int) bool { return edits[i].Start < edits[j].Start })
	var out []Edit
	for _, e := range edits {
		if len(out) > 0 && out[len(out)-1].End >= e.Start {
			last := &out[len(out)-1]
			last.New += e.New
			if e.End > last.End {
				last.End = e.End
			}
			continue
		}
		out = append(out, e)
	}
	return out
}

// actMapPos moves an offset of the original text to the text after edits.
// An offset inside a replaced stretch goes to the start of the new text.
func actMapPos(edits []Edit, pos int) int {
	shift := 0
	for _, e := range edits {
		switch {
		case e.End <= pos && e.Start < e.End, e.Start == e.End && e.Start < pos:
			shift += len([]rune(e.New)) - (e.End - e.Start)
		case e.Start < pos && pos < e.End:
			return e.Start + shift
		}
	}
	return pos + shift
}

// actApply gives the text after edits (sorted, not overlapping).
func actApply(text []rune, edits []Edit) string {
	var b strings.Builder
	last := 0
	for _, e := range edits {
		b.WriteString(string(text[last:e.Start]))
		b.WriteString(e.New)
		last = e.End
	}
	b.WriteString(string(text[last:]))
	return b.String()
}

func actMarkupSyntax(s string) string {
	if s == "" {
		return "html"
	}
	return s
}

// ---- balance ----

// BalanceOut grows the selection to the next bigger tag range: the element's
// inner content, then the whole element, then the parent's content, the
// parent, and so on. false when there is nothing bigger.
func BalanceOut(text []rune, sel Range) (Range, bool) {
	d := actParse(text, "html")
	for nd := d.innermost(sel, false); nd != nil; nd = nd.parent {
		if nd.close != nil {
			if r := nd.inner(); actContains(r, sel) && r != sel {
				return r, true
			}
		}
		if r := nd.outer(); actContains(r, sel) && r != sel {
			return r, true
		}
	}
	return sel, false
}

// BalanceIn shrinks the selection the other way: a whole element (or a cursor
// in one of its tags) → its inner content; content → its first child element.
func BalanceIn(text []rune, sel Range) (Range, bool) {
	d := actParse(text, "html")
	nd := d.innermost(sel, false)
	if nd == nil {
		return sel, false
	}
	if nd.close != nil {
		whole := sel == nd.outer()
		inOpen := sel.Start > nd.open.start && sel.Start < nd.open.end
		inClose := sel.Start > nd.close.start && sel.Start < nd.close.end
		if whole || inOpen || inClose {
			if r := nd.inner(); r != sel {
				return r, true
			}
		}
	}
	if len(nd.children) == 0 {
		return sel, false
	}
	c := nd.children[0]
	r := c.outer()
	if r == sel && c.close != nil {
		r = c.inner()
	}
	return r, r != sel
}

// MatchingPair: in an opening tag → the '<' of its closing tag; in a closing
// tag → the '<' of its opening tag.
func MatchingPair(text []rune, pos int) (int, bool) {
	d := actParse(text, "html")
	for _, nd := range d.nodes {
		if nd.close == nil {
			continue
		}
		if nd.close.start <= pos && pos < nd.close.end {
			return nd.open.start, true
		}
		if nd.open.start <= pos && pos < nd.open.end {
			return nd.close.start, true
		}
	}
	for _, nd := range d.nodes { // the cursor just after an opening tag's '>'
		if nd.close != nil && pos == nd.open.end {
			return nd.close.start, true
		}
	}
	return pos, false
}

// ---- remove / split-join / update tag ----

// RemoveTag removes the tags of the innermost element around pos and keeps
// its content. When the tags sit on their own lines, those lines go and the
// content moves one indent level to the left.
func RemoveTag(text []rune, pos int, opt Options) (Change, bool) {
	d := actParse(text, actMarkupSyntax(opt.Syntax))
	nd := d.innermost(Range{pos, pos}, true)
	if nd == nil {
		return Change{}, false
	}
	ot := nd.open
	var edits []Edit
	if nd.close == nil {
		s, e, _ := actWholeLine(text, ot.start, ot.end)
		edits = append(edits, Edit{s, e, ""})
	} else {
		ct := nd.close
		if actLineStart(text, ot.start) == actLineStart(text, ct.start) {
			if s, e, ok := actWholeLine(text, ot.start, ct.end); ok && actBlank(text, ot.end, ct.start) {
				edits = append(edits, Edit{s, e, ""})
			} else {
				edits = append(edits, Edit{ot.start, ot.end, ""}, Edit{ct.start, ct.end, ""})
			}
		} else {
			os, oe, oOwn := actWholeLine(text, ot.start, ot.end)
			cs, ce, cOwn := actWholeLine(text, ct.start, ct.end)
			if oOwn && cOwn && ce == len(text) && cs < len(text) && text[cs] == '\n' && cs+1 == oe {
				cs++ // keep one newline between
			}
			edits = append(edits, Edit{os, oe, ""})
			if oOwn && cOwn {
				base := len(actLineIndent(text, ot.start))
				for ls := actNextLineStart(text, ot.end); ls < actLineStart(text, ct.start); ls = actNextLineStart(text, ls) {
					le := actLineEnd(text, ls)
					if actBlank(text, ls, le) {
						continue
					}
					ind := actLineIndent(text, ls)
					if len(ind) <= base {
						continue
					}
					if n := actDedentWidth(ind, opt.Indent); n > 0 {
						edits = append(edits, Edit{ls, ls + n, ""})
					}
				}
			}
			edits = append(edits, Edit{cs, ce, ""})
		}
	}
	edits = actNormalize(edits)
	return Change{Edits: edits, Sel: Range{actMapPos(edits, pos), actMapPos(edits, pos)}}, true
}

// actDedentWidth says how many leading runes of ind make one indent level.
func actDedentWidth(ind, unit string) int {
	if unit != "" && strings.HasPrefix(ind, unit) {
		return len([]rune(unit))
	}
	if strings.HasPrefix(ind, "\t") {
		return 1
	}
	w := len(unit)
	if w == 0 || strings.Contains(unit, "\t") {
		w = 4
		if unit == "" {
			w = 2
		}
	}
	n := 0
	for n < len(ind) && n < w && ind[n] == ' ' {
		n++
	}
	return n
}

// SplitJoinTag turns <div …>…</div> into a self-closing <div … /> (the content
// goes, as in Emmet) and <div … /> back into <div …></div>. HTML writes
// "<div/>" like Emmet's html profile; xhtml, jsx and xml write "<div />".
// HTML void elements without a slash (<br>, <img …>) are left alone.
func SplitJoinTag(text []rune, pos int, opt Options) (Change, bool) {
	syn := actMarkupSyntax(opt.Syntax)
	d := actParse(text, syn)
	nd := d.innermost(Range{pos, pos}, true)
	if nd == nil || !nd.open.closed || nd.name == "" {
		return Change{}, false
	}
	ot := nd.open
	s := ot.attrEnd
	for s > ot.nameEnd && actIsSpace(text[s-1]) {
		s--
	}
	if nd.close != nil { // join
		tail := "/>"
		if syn == "xhtml" || syn == "jsx" || syn == "xml" {
			tail = " />"
		}
		edits := []Edit{{s, nd.close.end, tail}}
		c := Change{Edits: edits}
		p := pos
		if p > s {
			p = s
		}
		c.Sel = Range{p, p}
		if in := nd.inner(); !actBlank(text, in.Start, in.End) && strings.TrimSpace(string(text[in.Start:in.End])) != "" {
			c.Note = "joined <" + nd.name + " />; its content was removed"
		}
		return c, true
	}
	if !ot.selfClose {
		return Change{}, false
	}
	if (syn == "html" || syn == "xhtml") && actIsVoid(nd.name) {
		return Change{}, false
	}
	edits := []Edit{{s, ot.end, "></" + nd.name + ">"}}
	return Change{Edits: edits, Sel: Range{s + 1, s + 1}}, true
}

type actAbbrAttr struct{ name, value string }

// actParseUpdateAbbr reads "name#id.c1.c2[a=b c='d e']".
func actParseUpdateAbbr(abbr string) (name string, attrs []actAbbrAttr, classes []string, ok bool) {
	r := []rune(strings.TrimSpace(abbr))
	i := 0
	for i < len(r) && (actIsNameChar(r[i]) && r[i] != '.') {
		i++
	}
	name = string(r[:i])
	if name != "" && !actIsNameStart(r[0]) {
		return "", nil, nil, false
	}
	word := func() string {
		s := i
		for i < len(r) && r[i] != '.' && r[i] != '#' && r[i] != '[' && !actIsSpace(r[i]) {
			i++
		}
		return string(r[s:i])
	}
	for i < len(r) {
		switch r[i] {
		case '#':
			i++
			w := word()
			if w == "" {
				return "", nil, nil, false
			}
			attrs = append(attrs, actAbbrAttr{"id", w})
		case '.':
			i++
			w := word()
			if w == "" {
				return "", nil, nil, false
			}
			classes = append(classes, w)
		case '[':
			i++
			for {
				for i < len(r) && actIsSpace(r[i]) {
					i++
				}
				if i >= len(r) {
					return "", nil, nil, false
				}
				if r[i] == ']' {
					i++
					break
				}
				s := i
				for i < len(r) && r[i] != '=' && r[i] != ']' && !actIsSpace(r[i]) {
					i++
				}
				an := string(r[s:i])
				if an == "" {
					return "", nil, nil, false
				}
				val := ""
				if i < len(r) && r[i] == '=' {
					i++
					if i < len(r) && (r[i] == '"' || r[i] == '\'') {
						q := r[i]
						i++
						s := i
						for i < len(r) && r[i] != q {
							i++
						}
						if i >= len(r) {
							return "", nil, nil, false
						}
						val = string(r[s:i])
						i++
					} else {
						s := i
						for i < len(r) && r[i] != ']' && !actIsSpace(r[i]) {
							i++
						}
						val = string(r[s:i])
					}
				}
				attrs = append(attrs, actAbbrAttr{an, val})
			}
		default:
			return "", nil, nil, false
		}
	}
	return name, attrs, classes, name != "" || len(attrs) > 0 || len(classes) > 0
}

// UpdateTag changes the tag around pos from an abbreviation, as Emmet's
// "update tag": a name renames both tags, "#id" sets the id, ".c" adds
// classes (kept ones are not doubled), "[a=b]" sets attributes.
// Example: "section.wide[title='Hi there']".
func UpdateTag(text []rune, pos int, abbr string, opt Options) (Change, bool) {
	syn := actMarkupSyntax(opt.Syntax)
	name, attrs, classes, ok := actParseUpdateAbbr(abbr)
	if !ok {
		return Change{}, false
	}
	d := actParse(text, syn)
	nd := d.innermost(Range{pos, pos}, true)
	if nd == nil || !nd.open.closed {
		return Change{}, false
	}
	ot := nd.open
	var edits []Edit
	if name != "" && name != nd.name {
		edits = append(edits, Edit{ot.nameStart, ot.nameEnd, name})
		if nd.close != nil {
			edits = append(edits, Edit{nd.close.nameStart, nd.close.nameEnd, name})
		}
	}
	classAttr := "class"
	if syn == "jsx" {
		classAttr = "className"
	}
	quote := "\""
	for _, a := range ot.attrs {
		if a.quote == '\'' {
			quote = "'"
			break
		}
		if a.quote == '"' {
			break
		}
	}
	if len(classes) > 0 {
		var words []string
		if a := ot.attr(classAttr); a != nil && a.hasValue() {
			if a.quote == '{' {
				return Change{}, false // a JSX expression: cannot add words to it
			}
			words = strings.Fields(string(text[a.valStart:a.valEnd]))
		}
		have := map[string]bool{}
		for _, w := range words {
			have[w] = true
		}
		changed := false
		for _, c := range classes {
			if !have[c] {
				words = append(words, c)
				have[c] = true
				changed = true
			}
		}
		if changed {
			attrs = append([]actAbbrAttr{{classAttr, strings.Join(words, " ")}}, attrs...)
		}
	}
	insertAt := ot.nameEnd
	if len(ot.attrs) > 0 {
		insertAt = ot.attrs[len(ot.attrs)-1].end
	}
	var added strings.Builder
	for _, at := range attrs {
		if syn == "jsx" && at.name == "class" {
			at.name = "className"
		}
		q := quote
		if strings.Contains(at.value, q) {
			if q == "\"" {
				q = "'"
			} else {
				q = "\""
			}
		}
		a := ot.attr(at.name)
		switch {
		case a == nil:
			added.WriteString(" " + at.name + "=" + q + at.value + q)
		case !a.hasValue():
			edits = append(edits, Edit{a.nameEnd, a.end, "=" + q + at.value + q})
		case a.quote == '{' || a.quote == 0 || strings.ContainsRune(at.value, a.quote):
			if string(text[a.valStart:a.valEnd]) != at.value || a.quote == '{' {
				vs, ve := a.valStart, a.valEnd
				if a.quote != 0 {
					vs, ve = a.valStart-1, a.end
				}
				edits = append(edits, Edit{vs, ve, q + at.value + q})
			}
		default:
			if string(text[a.valStart:a.valEnd]) != at.value {
				edits = append(edits, Edit{a.valStart, a.valEnd, at.value})
			}
		}
	}
	if added.Len() > 0 {
		edits = append(edits, Edit{insertAt, insertAt, added.String()})
	}
	if len(edits) == 0 {
		return Change{}, false
	}
	edits = actNormalize(edits)
	p := actMapPos(edits, pos)
	return Change{Edits: edits, Sel: Range{p, p}}, true
}

// ---- edit points ----

// NextEditPoint finds the next (dir ≥ 0) or previous (dir < 0) place to type:
// between the quotes of an empty attribute value (="" or ={}), between "><"
// of an empty element, or at the end of a blank line between tags.
func NextEditPoint(text []rune, pos, dir int) (int, bool) {
	posLine := actLineStart(text, pos)
	if dir >= 0 {
		for p := pos + 1; p <= len(text); p++ {
			if actIsEditPoint(text, p, posLine) {
				return p, true
			}
		}
	} else {
		for p := pos - 1; p >= 0; p-- {
			if actIsEditPoint(text, p, posLine) {
				return p, true
			}
		}
	}
	return pos, false
}

func actIsEditPoint(text []rune, p, skipLine int) bool {
	n := len(text)
	if p > 0 && p < n {
		a, b := text[p-1], text[p]
		if (a == '"' || a == '\'') && b == a && p >= 2 && text[p-2] == '=' {
			return true
		}
		if a == '{' && b == '}' && p >= 2 && text[p-2] == '=' {
			return true
		}
		if a == '>' && b == '<' {
			return true
		}
	}
	if p == n || text[p] == '\n' || text[p] == '\r' {
		ls := actLineStart(text, p)
		if ls == skipLine || !actBlank(text, ls, p) || ls == 0 {
			return false
		}
		b := ls - 1
		for b >= 0 && actIsSpace(text[b]) {
			b--
		}
		a := p
		for a < n && actIsSpace(text[a]) {
			a++
		}
		return b >= 0 && text[b] == '>' && a < n && text[a] == '<'
	}
	return false
}

// ---- select item ----

// SelectItem selects the next (dir ≥ 0) or previous item, as Emmet does.
// Markup: tag name → each whole attribute → its value → each class word, then
// the next tag. CSS: selector → each declaration → its value → each value part.
func SelectItem(text []rune, sel Range, dir int, syntax string) (Range, bool) {
	var items []Range
	if IsStylesheet(syntax) {
		items = actCSSItems(text)
	} else {
		items = actHTMLItems(text, actMarkupSyntax(syntax))
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Start != items[j].Start {
			return items[i].Start < items[j].Start
		}
		return items[i].End > items[j].End
	})
	// A cursor inside an item picks that item (the smallest one).
	if sel.Start == sel.End {
		var best *Range
		for i := range items {
			it := &items[i]
			if it.Start <= sel.Start && sel.Start < it.End && (best == nil || it.End-it.Start <= best.End-best.Start) {
				best = it
			}
		}
		if best != nil {
			return *best, true
		}
	}
	if dir >= 0 {
		for _, it := range items {
			if it.Start > sel.Start || it.Start == sel.Start && it.End < sel.End {
				return it, true
			}
		}
	} else {
		for i := len(items) - 1; i >= 0; i-- {
			it := items[i]
			if it.Start < sel.Start || it.Start == sel.Start && it.End > sel.End {
				return it, true
			}
		}
	}
	return sel, false
}

func actHTMLItems(text []rune, syntax string) []Range {
	var items []Range
	for _, t := range actScan(text, syntax) {
		if t.kind != actTokOpen || t.name == "" {
			continue
		}
		items = append(items, Range{t.nameStart, t.nameEnd})
		for _, a := range t.attrs {
			items = append(items, Range{a.start, a.end})
			if !a.hasValue() || a.valEnd <= a.valStart {
				continue
			}
			items = append(items, Range{a.valStart, a.valEnd})
			if strings.EqualFold(a.name, "class") || a.name == "className" {
				words := actWords(text, a.valStart, a.valEnd)
				if len(words) > 1 {
					items = append(items, words...)
				}
			}
		}
	}
	return items
}

func actWords(text []rune, s, e int) []Range {
	var out []Range
	for i := s; i < e; {
		for i < e && actIsSpace(text[i]) {
			i++
		}
		j := i
		for j < e && !actIsSpace(text[j]) {
			j++
		}
		if j > i {
			out = append(out, Range{i, j})
		}
		i = j
	}
	return out
}

func actCSSItems(text []rune) []Range {
	var items []Range
	for _, r := range actParseCSS(text).rules {
		if r.preEnd > r.preStart {
			items = append(items, Range{r.preStart, r.preEnd})
		}
		for _, d := range r.decls {
			items = append(items, Range{d.start, d.end})
			if d.valEnd > d.valStart {
				items = append(items, Range{d.valStart, d.valEnd})
				parts := actValueParts(text, d.valStart, d.valEnd)
				if len(parts) > 1 {
					items = append(items, parts...)
				}
			}
		}
	}
	return items
}

// ---- block comments ----

// ToggleBlockComment comments or uncomments. html/xhtml/xml use <!-- -->, jsx
// {/* */}, the CSS languages /* */. Inside a comment: the comment marks go.
// Else the selection is commented; with only a cursor, the element at the
// cursor (markup), or the declaration at the cursor, or the whole rule when
// the cursor is on its selector (CSS).
func ToggleBlockComment(text []rune, sel Range, syntax string) (Change, bool) {
	if IsStylesheet(syntax) {
		return actToggleCSSComment(text, sel)
	}
	syn := actMarkupSyntax(syntax)
	open, close := "<!--", "-->"
	if syn == "jsx" {
		open, close = "{/*", "*/}"
	}
	// Inside a comment: remove it.
	if c, ok := actFindComment(text, sel, open, close); ok {
		return actUncomment(text, sel, c, open, close), true
	}
	target := sel
	if sel.Start == sel.End {
		d := actParse(text, syn)
		nd := d.innermost(sel, true)
		if nd == nil {
			return Change{}, false
		}
		target = nd.outer()
	}
	return actComment(text, sel, target, open, close), true
}

// actFindComment finds the comment (open … close) holding sel.
func actFindComment(text []rune, sel Range, open, close string) (Range, bool) {
	for i := 0; i < len(text); i++ {
		if !actHasPrefix(text, i, open) {
			continue
		}
		j := actIndex(text, i+len(open), close)
		if j < 0 {
			return Range{}, false
		}
		c := Range{i, j + len(close)}
		if c.Start <= sel.Start && sel.End <= c.End {
			return c, true
		}
		if c.Start > sel.End {
			break
		}
		i = c.End - 1
	}
	return Range{}, false
}

func actUncomment(text []rune, sel Range, c Range, open, close string) Change {
	os, oe := c.Start, c.Start+len([]rune(open))
	if oe < len(text) && text[oe] == ' ' {
		oe++
	}
	ce, cs := c.End, c.End-len([]rune(close))
	if cs > oe && text[cs-1] == ' ' {
		cs--
	}
	edits := []Edit{{os, oe, ""}, {cs, ce, ""}}
	return Change{Edits: edits, Sel: Range{actMapPos(edits, sel.Start), actMapPos(edits, sel.End)}}
}

// actComment wraps target in open/close. Comments already inside lose their
// marks first (comments cannot nest).
func actComment(text []rune, sel, target Range, open, close string) Change {
	edits := []Edit{{target.Start, target.Start, open + " "}}
	for i := target.Start; i < target.End; i++ {
		if !actHasPrefix(text, i, open) {
			continue
		}
		j := actIndex(text, i+len(open), close)
		if j < 0 || j+len(close) > target.End {
			break
		}
		inner := actUncomment(text, Range{i, i}, Range{i, j + len(close)}, open, close)
		edits = append(edits, inner.Edits...)
		i = j + len(close) - 1
	}
	edits = append(edits, Edit{target.End, target.End, " " + close})
	edits = actNormalize(edits)
	pl := len([]rune(open)) + 1
	var out Range
	if sel.Start == sel.End {
		p := actMapPos(edits, sel.Start)
		if sel.Start == target.Start {
			p += pl
		}
		out = Range{p, p}
	} else { // the commented text, without the marks
		cs := actMapPos(edits, target.Start)
		ce := cs + (target.End - target.Start)
		for _, e := range edits {
			ce += len([]rune(e.New)) - (e.End - e.Start)
		}
		out = Range{cs + pl, ce - len([]rune(close)) - 1}
	}
	return Change{Edits: edits, Sel: out}
}

func actToggleCSSComment(text []rune, sel Range) (Change, bool) {
	sh := actParseCSS(text)
	if c, ok := sh.commentAt(sel); ok {
		if c.End-c.Start >= 4 && actHasPrefix(text, c.End-2, "*/") {
			return actUncomment(text, sel, c, "/*", "*/"), true
		}
		return Change{}, false // an unclosed comment
	}
	target := sel
	if sel.Start == sel.End {
		pos := sel.Start
		r := sh.ruleAt(pos)
		if r == nil {
			return Change{}, false
		}
		target = Range{}
		if pos <= r.open {
			target = Range{r.preStart, r.end()}
			if r.close >= len(text) {
				target.End = len(text)
			}
		} else {
			for _, dc := range r.decls {
				if dc.start <= pos && pos <= dc.end {
					target = Range{dc.start, dc.end}
				}
			}
			if target.End == 0 {
				return Change{}, false
			}
		}
	}
	return actComment(text, sel, target, "/*", "*/"), true
}

// ---- auto-close and tag rename helpers ----

// AutoCloseTag is called just after the user typed a letter at pos-1.
//
//   - Typed '>' finishing an opening tag "<name …>": gives "</name>" for the
//     editor to insert after the cursor. Not for void elements in html/xhtml,
//     "/>", comments, doctypes, "<?", closing tags, or when "</name" already
//     follows the cursor. In JSX, "<>" gives "</>".
//   - Typed '/' making "</": gives "name>" for the innermost element that is
//     still open there (not when "name>" already follows).
//
// ok is false when nothing should be inserted.
func AutoCloseTag(text []rune, pos int, syntax string) (string, bool) {
	syn := actMarkupSyntax(syntax)
	html := syn == "html" || syn == "xhtml"
	if pos < 1 || pos > len(text) {
		return "", false
	}
	switch text[pos-1] {
	case '>':
		toks := actScan(text[:pos], syn)
		if len(toks) == 0 {
			return "", false
		}
		t := toks[len(toks)-1]
		if t.kind != actTokOpen || t.end != pos || !t.closed || t.selfClose {
			return "", false
		}
		if t.name == "" && syn != "jsx" {
			return "", false
		}
		if t.start > 0 {
			if b := text[t.start-1]; actIsNameChar(b) || b == ')' || b == ']' || b == '"' || b == '\'' {
				return "", false // "a<b>" or "Array<string>": not a tag
			}
		}
		if html && actIsVoid(t.name) {
			return "", false
		}
		if actHasPrefixFold(text, pos, "</"+t.name) {
			if e := pos + 2 + len([]rune(t.name)); e >= len(text) || !actIsNameChar(text[e]) {
				return "", false
			}
		}
		return "</" + t.name + ">", true
	case '/':
		if pos < 2 || text[pos-2] != '<' {
			return "", false
		}
		d := actParse(text[:pos-2], syn)
		var open *actNode
		for _, nd := range d.nodes {
			if nd.close == nil && nd.open.closed && !nd.open.selfClose && nd.end >= actTrimEnd(text, pos-2) && !(html && actIsVoid(nd.name)) {
				if open == nil || nd.depth > open.depth {
					open = nd
				}
			}
		}
		if open == nil {
			return "", false
		}
		if t := d.tokAt(pos - 3); t != nil && t.kind != actTokOpen && t.kind != actTokClose && !t.closed {
			return "", false // inside a comment or similar
		}
		want := open.name + ">"
		if actHasPrefix(text, pos, want) {
			return "", false
		}
		return want, true
	}
	return "", false
}

func actTrimEnd(text []rune, e int) int {
	for e > 0 && actIsSpace(text[e-1]) {
		e--
	}
	return e
}

// TagNamePair finds the tag name at pos (in an opening or closing tag's name,
// or just after it) and gives the name ranges of both tags of the pair. The
// editor calls it before a name edit (while both names still match), then
// mirrors the edit into the partner. When the names already differ it pairs
// an opening tag with the closing tag that ends its element by position.
func TagNamePair(text []rune, pos int) (open, close Range, ok bool) {
	d := actParse(text, "html")
	for _, nd := range d.nodes {
		if nd.close == nil || nd.name == "" {
			continue
		}
		o := Range{nd.open.nameStart, nd.open.nameEnd}
		c := Range{nd.close.nameStart, nd.close.nameEnd}
		if (o.Start <= pos && pos <= o.End) || (c.Start <= pos && pos <= c.End) {
			return o, c, true
		}
	}
	// Mismatched names (one side already edited): pair by nesting.
	toks := actScan(text, "html")
	var stack []int
	for i, t := range toks {
		switch t.kind {
		case actTokOpen:
			if t.selfClose || !t.closed || actIsVoid(t.name) && !actVoidUsedAsPair(toks, i) {
				continue
			}
			stack = append(stack, i)
		case actTokClose:
			if len(stack) == 0 {
				continue
			}
			ot := toks[stack[len(stack)-1]]
			stack = stack[:len(stack)-1]
			o := Range{ot.nameStart, ot.nameEnd}
			c := Range{t.nameStart, t.nameEnd}
			if ot.name != "" && t.name != "" && ((o.Start <= pos && pos <= o.End) || (c.Start <= pos && pos <= c.End)) {
				return o, c, true
			}
		}
	}
	return Range{}, Range{}, false
}
