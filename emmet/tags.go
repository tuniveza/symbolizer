package emmet

// A forgiving HTML / XML / JSX tag scanner, and the element tree built from it.
// It never fails: broken markup gives fewer or shorter elements, not errors.

import (
	"strings"
	"unicode"
)

type actTokKind int

const (
	actTokOpen    actTokKind = iota // <name ...> or <name ... /> or <> (JSX fragment)
	actTokClose                     // </name> or </>
	actTokComment                   // <!-- ... -->
	actTokCDATA                     // <![CDATA[ ... ]]>
	actTokDoctype                   // <!DOCTYPE ...> and other <!...>
	actTokPI                        // <? ... ?>
)

// actAttr is one attribute of an opening tag.
type actAttr struct {
	name               string
	start, end         int // the whole attribute, quotes included
	nameStart, nameEnd int
	valStart, valEnd   int  // the value without its quotes; -1 when there is no value
	quote              rune // '"', '\'', '{' (JSX) or 0 (no quotes)
}

func (a actAttr) hasValue() bool { return a.valStart >= 0 }

// actTok is one tag-like token.
type actTok struct {
	kind               actTokKind
	start, end         int
	name               string
	nameStart, nameEnd int
	attrs              []actAttr
	selfClose          bool // ends with "/>"
	closed             bool // ended properly ('>' or the comment's own end)
	attrEnd            int  // where the attribute area stops: the '/' of "/>", the '>' or the end
}

var actVoidTags = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true, "hr": true,
	"img": true, "input": true, "link": true, "meta": true, "param": true, "source": true,
	"track": true, "wbr": true, "keygen": true, "command": true, "basefont": true, "frame": true,
}

// Tags whose content is raw text in HTML (no tags inside).
var actRawTags = map[string]bool{"script": true, "style": true, "textarea": true, "title": true}

func actIsVoid(name string) bool { return actVoidTags[strings.ToLower(name)] }

func actIsNameStart(r rune) bool {
	if r < 128 {
		return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
	}
	return unicode.IsLetter(r)
}

func actIsNameChar(r rune) bool {
	return actIsNameStart(r) || r >= '0' && r <= '9' || r == '-' || r == '_' || r == ':' || r == '.' || r >= 128 && unicode.IsDigit(r)
}

func actIsSpace(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f' }

func actHasPrefix(text []rune, i int, s string) bool {
	for _, r := range s {
		if i >= len(text) || text[i] != r {
			return false
		}
		i++
	}
	return true
}

// actHasPrefixFold is actHasPrefix ignoring ASCII case.
func actHasPrefixFold(text []rune, i int, s string) bool {
	for _, r := range s {
		if i >= len(text) || unicode.ToLower(text[i]) != unicode.ToLower(r) {
			return false
		}
		i++
	}
	return true
}

func actIndex(text []rune, from int, s string) int {
	for i := from; i < len(text); i++ {
		if actHasPrefix(text, i, s) {
			return i
		}
	}
	return -1
}

func actIndexFold(text []rune, from int, s string) int {
	for i := from; i < len(text); i++ {
		if actHasPrefixFold(text, i, s) {
			return i
		}
	}
	return -1
}

// actSkipBraces gives the index just after the '}' that closes the '{' at i,
// skipping strings and nested braces (JSX attribute values). len(text) if unclosed.
func actSkipBraces(text []rune, i int) int {
	depth := 0
	for j := i; j < len(text); j++ {
		switch r := text[j]; r {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return j + 1
			}
		case '"', '\'', '`':
			k := j + 1
			for k < len(text) && text[k] != r {
				if text[k] == '\\' {
					k++
				}
				k++
			}
			j = k
		}
	}
	return len(text)
}

func actSameName(a, b string, html bool) bool {
	if html {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// actScan finds all tag-like tokens. Text between them is skipped; so is the
// content of script/style/textarea/title in HTML.
func actScan(text []rune, syntax string) []actTok {
	var toks []actTok
	n := len(text)
	html := syntax != "xml" && syntax != "jsx"
	for i := 0; i < n; {
		if text[i] != '<' {
			i++
			continue
		}
		t, ok := actScanTag(text, i)
		if !ok {
			i++
			continue
		}
		toks = append(toks, t)
		i = t.end
		if i <= t.start {
			i = t.start + 1
		}
		if html && t.kind == actTokOpen && t.closed && !t.selfClose && actRawTags[strings.ToLower(t.name)] {
			j := actIndexFold(text, i, "</"+t.name)
			if j < 0 {
				i = n
			} else {
				i = j
			}
		}
	}
	return toks
}

// actScanTag reads the token that starts with the '<' at i. ok is false when
// the '<' does not start a tag (e.g. "a < b").
func actScanTag(text []rune, i int) (actTok, bool) {
	n := len(text)
	until := func(kind actTokKind, from int, end string) actTok {
		j := actIndex(text, from, end)
		if j < 0 {
			return actTok{kind: kind, start: i, end: n, attrEnd: n}
		}
		return actTok{kind: kind, start: i, end: j + len(end), closed: true, attrEnd: j}
	}
	switch {
	case actHasPrefix(text, i, "<!--"):
		return until(actTokComment, i+4, "-->"), true
	case actHasPrefix(text, i, "<![CDATA["):
		return until(actTokCDATA, i+9, "]]>"), true
	case actHasPrefix(text, i, "<!"):
		return until(actTokDoctype, i+2, ">"), true
	case actHasPrefix(text, i, "<?"):
		return until(actTokPI, i+2, "?>"), true
	case actHasPrefix(text, i, "</>"):
		return actTok{kind: actTokClose, start: i, end: i + 3, nameStart: i + 2, nameEnd: i + 2, closed: true, attrEnd: i + 2}, true
	case actHasPrefix(text, i, "</"):
		j := i + 2
		if j >= n || !actIsNameStart(text[j]) {
			return actTok{}, false
		}
		k := j
		for k < n && actIsNameChar(text[k]) {
			k++
		}
		t := actTok{kind: actTokClose, start: i, name: string(text[j:k]), nameStart: j, nameEnd: k}
		m := k
		for m < n && text[m] != '>' && text[m] != '<' {
			m++
		}
		t.attrEnd = m
		if m < n && text[m] == '>' {
			t.end, t.closed = m+1, true
		} else {
			t.end = m
		}
		return t, true
	case actHasPrefix(text, i, "<>"):
		return actTok{kind: actTokOpen, start: i, end: i + 2, nameStart: i + 1, nameEnd: i + 1, closed: true, attrEnd: i + 1}, true
	case i+1 < n && actIsNameStart(text[i+1]):
		return actScanOpen(text, i), true
	}
	return actTok{}, false
}

func actScanOpen(text []rune, i int) actTok {
	n := len(text)
	j := i + 1
	for j < n && actIsNameChar(text[j]) {
		j++
	}
	t := actTok{kind: actTokOpen, start: i, name: string(text[i+1 : j]), nameStart: i + 1, nameEnd: j}
	k := j
	for k < n {
		r := text[k]
		switch {
		case actIsSpace(r):
			k++
		case r == '>':
			t.attrEnd, t.end, t.closed = k, k+1, true
			return t
		case r == '/' && k+1 < n && text[k+1] == '>':
			t.attrEnd, t.end, t.closed, t.selfClose = k, k+2, true, true
			return t
		case r == '<':
			t.attrEnd, t.end = k, k
			return t
		case r == '{':
			e := actSkipBraces(text, k)
			ve := e - 1
			if e == n && (n == 0 || text[n-1] != '}') {
				ve = n
			}
			t.attrs = append(t.attrs, actAttr{start: k, end: e, nameStart: k, nameEnd: k, valStart: k + 1, valEnd: ve, quote: '{'})
			k = e
		default:
			ns := k
			for k < n {
				c := text[k]
				if actIsSpace(c) || c == '=' || c == '>' || c == '<' || c == '"' || c == '\'' || c == '/' || c == '{' {
					break
				}
				k++
			}
			if k == ns { // a stray character
				k++
				continue
			}
			a := actAttr{name: string(text[ns:k]), start: ns, end: k, nameStart: ns, nameEnd: k, valStart: -1, valEnd: -1}
			m := k
			for m < n && actIsSpace(text[m]) {
				m++
			}
			if m < n && text[m] == '=' {
				m++
				for m < n && actIsSpace(text[m]) {
					m++
				}
				if m < n {
					switch q := text[m]; q {
					case '"', '\'':
						e := m + 1
						for e < n && text[e] != q {
							e++
						}
						a.quote, a.valStart, a.valEnd = q, m+1, e
						if e >= n { // the quote never closes
							a.end = n
							t.attrs = append(t.attrs, a)
							t.attrEnd, t.end = n, n
							return t
						}
						a.end = e + 1
					case '{':
						e := actSkipBraces(text, m)
						ve := e - 1
						if e == n && text[n-1] != '}' {
							ve = n
						}
						a.quote, a.valStart, a.valEnd, a.end = '{', m+1, ve, e
					case '>', '<':
						// "a= >": no value
					default:
						e := m
						for e < n && !actIsSpace(text[e]) && text[e] != '>' && text[e] != '<' && !(text[e] == '/' && e+1 < n && text[e+1] == '>') {
							e++
						}
						a.valStart, a.valEnd, a.end = m, e, e
					}
					k = a.end
				}
			}
			t.attrs = append(t.attrs, a)
		}
	}
	t.attrEnd, t.end = n, n
	return t
}

func (t *actTok) attr(name string) *actAttr {
	for i := range t.attrs {
		if t.attrs[i].name != "" && strings.EqualFold(t.attrs[i].name, name) {
			return &t.attrs[i]
		}
	}
	return nil
}

// actNode is one element: an opening tag, maybe its closing tag, and its children.
type actNode struct {
	name       string
	open       *actTok
	close      *actTok // nil: void, self-closing, or never closed
	start, end int     // the whole element
	parent     *actNode
	children   []*actNode
	depth      int
}

// inner gives the content range. Only meaningful when close != nil.
func (nd *actNode) inner() Range {
	if nd.close != nil {
		return Range{nd.open.end, nd.close.start}
	}
	return Range{nd.open.end, nd.end}
}

func (nd *actNode) outer() Range { return Range{nd.start, nd.end} }

type actDoc struct {
	text  []rune
	toks  []actTok
	nodes []*actNode // in document order
}

// Closing an element of the key implicitly when an element of one of the values opens (HTML).
var actPCloser = map[string]bool{
	"address": true, "article": true, "aside": true, "blockquote": true, "details": true, "div": true,
	"dl": true, "fieldset": true, "figcaption": true, "figure": true, "footer": true, "form": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true, "header": true, "hgroup": true,
	"hr": true, "main": true, "menu": true, "nav": true, "ol": true, "p": true, "pre": true,
	"section": true, "table": true, "ul": true,
}

func actImpliedClose(top, next string) bool {
	top, next = strings.ToLower(top), strings.ToLower(next)
	switch top {
	case "p":
		return actPCloser[next]
	case "li":
		return next == "li"
	case "dt", "dd":
		return next == "dt" || next == "dd"
	case "option":
		return next == "option" || next == "optgroup"
	case "optgroup":
		return next == "optgroup"
	case "td", "th":
		return next == "td" || next == "th" || next == "tr"
	case "tr":
		return next == "tr"
	case "thead", "tbody":
		return next == "tbody" || next == "tfoot"
	}
	return false
}

// actParse builds the element tree. syntax "" acts like "html".
func actParse(text []rune, syntax string) *actDoc {
	d := &actDoc{text: text, toks: actScan(text, syntax)}
	html := syntax != "xml" && syntax != "jsx"
	var stack []*actNode
	closeTop := func(at int) {
		nd := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		e := at
		for e > nd.open.end && actIsSpace(text[e-1]) {
			e--
		}
		nd.end = e
	}
	for idx := range d.toks {
		t := &d.toks[idx]
		switch t.kind {
		case actTokOpen:
			if html {
				for len(stack) > 0 && actImpliedClose(stack[len(stack)-1].name, t.name) {
					closeTop(t.start)
				}
			}
			nd := &actNode{name: t.name, open: t, start: t.start, end: t.end, depth: len(stack)}
			if len(stack) > 0 {
				nd.parent = stack[len(stack)-1]
				nd.parent.children = append(nd.parent.children, nd)
			}
			d.nodes = append(d.nodes, nd)
			if t.selfClose || !t.closed {
				continue
			}
			if html && actIsVoid(t.name) && !actVoidUsedAsPair(d.toks, idx) {
				continue
			}
			stack = append(stack, nd)
		case actTokClose:
			match := -1
			for k := len(stack) - 1; k >= 0; k-- {
				if actSameName(stack[k].name, t.name, html) {
					match = k
					break
				}
			}
			if match < 0 {
				continue // a stray closing tag
			}
			for len(stack)-1 > match {
				closeTop(t.start)
			}
			nd := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			nd.close, nd.end = t, t.end
		}
	}
	for len(stack) > 0 {
		closeTop(len(text))
	}
	return d
}

// actVoidUsedAsPair: a void-named tag directly followed by its own closing tag
// (RSS <link>…</link>, XML-ish markup) is a normal element after all.
func actVoidUsedAsPair(toks []actTok, idx int) bool {
	for k := idx + 1; k < len(toks); k++ {
		switch toks[k].kind {
		case actTokOpen:
			return false
		case actTokClose:
			return strings.EqualFold(toks[k].name, toks[idx].name)
		}
	}
	return false
}

// innermost gives the deepest element around sel. A cursor counts as inside an
// element from its '<' up to (inclusive: and including) its last '>'.
func (d *actDoc) innermost(sel Range, inclusive bool) *actNode {
	var best *actNode
	for _, nd := range d.nodes {
		var ok bool
		if sel.Start == sel.End {
			p := sel.Start
			ok = nd.start <= p && (p < nd.end || inclusive && p == nd.end)
		} else {
			ok = nd.start <= sel.Start && sel.End <= nd.end
		}
		if ok && (best == nil || nd.depth >= best.depth) {
			best = nd
		}
	}
	return best
}

// tokAt gives the token whose range holds pos (start ≤ pos < end), or nil.
func (d *actDoc) tokAt(pos int) *actTok {
	for i := range d.toks {
		if d.toks[i].start <= pos && pos < d.toks[i].end {
			return &d.toks[i]
		}
	}
	return nil
}
