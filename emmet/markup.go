package emmet

// The markup (HTML, XHTML, XML, JSX) half of Emmet.
//
// Steps, as in Emmet itself:
//  1. mkParse: the abbreviation becomes a tree (groups, repeats, raw text).
//  2. mkConv: repeats are unrolled, $ numbering and $# (wrapped text) are filled.
//  3. mkResolver.resolve: snippet names ("a", "link:css", "!") become their trees.
//  4. mkResolver.transform: implicit tag names, merged attributes, lorem text.
//  5. mkFmt: the tree is written out with Emmet's line-break rules.

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ---------------------------------------------------------------- parsing

type mkPNode struct {
	group     bool
	name      string
	attrs     []mkPAttr
	text      string
	hasText   bool
	children  []*mkPNode
	repeat    int  // *N (0 = not repeated)
	implicit  bool // * with no number: once per wrapped line
	selfClose bool
}

type mkPAttr struct {
	name, value                      string
	hasValue, expr, boolean, implied bool
}

type mkParser struct {
	src []rune
	pos int
	jsx bool
}

const mkMaxNodes = 10000

func (p *mkParser) eof() bool { return p.pos >= len(p.src) }

func (p *mkParser) peek() rune { return p.at(0) }

func (p *mkParser) at(i int) rune {
	if p.pos+i >= len(p.src) || p.pos+i < 0 {
		return 0
	}
	return p.src[p.pos+i]
}

func (p *mkParser) errf(format string, a ...any) error {
	return fmt.Errorf("emmet: "+format+" (at %d)", append(a, p.pos)...)
}

// mkParse reads an abbreviation into a tree of groups and elements.
func mkParse(abbr, syntax string) ([]*mkPNode, error) {
	p := &mkParser{src: []rune(abbr), jsx: syntax == "jsx"}
	root := &mkPNode{group: true}
	if err := p.seq(root, 0); err != nil {
		return nil, err
	}
	if !p.eof() {
		return nil, p.errf("unexpected %q", p.peek())
	}
	return root.children, nil
}

func (p *mkParser) seq(root *mkPNode, depth int) error {
	stack := []*mkPNode{root}
	for {
		if p.eof() || p.peek() == ')' {
			return p.errf("missing element")
		}
		item, err := p.item(depth)
		if err != nil {
			return err
		}
		top := stack[len(stack)-1]
		top.children = append(top.children, item)
		if p.eof() {
			return nil
		}
		switch p.peek() {
		case '>':
			p.pos++
			stack = append(stack, item)
		case '+':
			p.pos++
		case '^':
			for p.peek() == '^' {
				p.pos++
				if len(stack) > 1 {
					stack = stack[:len(stack)-1]
				}
			}
		case ')':
			if depth == 0 {
				return p.errf("unexpected )")
			}
			return nil
		default:
			return p.errf("unexpected %q", p.peek())
		}
	}
}

func (p *mkParser) item(depth int) (*mkPNode, error) {
	if p.peek() == '(' {
		p.pos++
		g := &mkPNode{group: true}
		if err := p.seq(g, depth+1); err != nil {
			return nil, err
		}
		if p.peek() == ')' {
			p.pos++
		} // an unclosed group at the very end is allowed
		p.repeat(g)
		return g, nil
	}
	return p.element()
}

func (p *mkParser) repeat(n *mkPNode) {
	if p.peek() != '*' {
		return
	}
	p.pos++
	start := p.pos
	for p.peek() >= '0' && p.peek() <= '9' {
		p.pos++
	}
	if start == p.pos {
		n.implicit = true
		return
	}
	v, err := strconv.Atoi(string(p.src[start:p.pos]))
	if err != nil || v > mkMaxNodes {
		v = mkMaxNodes + 1 // caught as "too big" later
	}
	if v < 1 {
		v = 1
	}
	n.repeat = v
}

func mkNameRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == ':' || r == '$' || r == '@'
}

func (p *mkParser) name() string {
	start := p.pos
	for p.peek() == '!' {
		p.pos++
	}
	for !p.eof() {
		r := p.peek()
		if mkNameRune(r) {
			p.pos++
			continue
		}
		// JSX: Foo.Bar is one component name
		if r == '.' && p.jsx && p.pos > start && unicode.IsUpper(p.src[start]) && unicode.IsUpper(p.at(1)) {
			p.pos++
			continue
		}
		break
	}
	name := string(p.src[start:p.pos])
	// "ul+", "table+" are snippet names, not a sibling with nothing after it
	if name != "" && p.peek() == '+' {
		if _, ok := mkSnippets[name+"+"]; ok {
			switch p.at(1) {
			case 0, ')', '+', '^', '|':
				p.pos++
				name += "+"
			}
		}
	}
	return name
}

func (p *mkParser) element() (*mkPNode, error) {
	n := &mkPNode{}
	n.name = p.name()
	if n.name != "" && unicode.IsDigit([]rune(n.name)[0]) {
		return nil, p.errf("a name cannot start with a digit")
	}
loop:
	for !p.eof() {
		switch c := p.peek(); c {
		case '#', '.':
			p.pos++
			v := p.ident()
			if v == "" {
				return nil, p.errf("empty %c", c)
			}
			name := "id"
			if c == '.' {
				name = "class"
			}
			n.attrs = append(n.attrs, mkPAttr{name: name, value: v, hasValue: true})
		case '[':
			if err := p.attrList(n); err != nil {
				return nil, err
			}
		case '{':
			t, err := p.braced()
			if err != nil {
				return nil, err
			}
			n.text += t
			n.hasText = true
		case '*':
			p.repeat(n)
		case '/':
			p.pos++
			n.selfClose = true
		default:
			break loop
		}
	}
	if n.name == "" && len(n.attrs) == 0 && !n.hasText {
		return nil, p.errf("missing element")
	}
	return n, nil
}

func (p *mkParser) ident() string {
	start := p.pos
	for !p.eof() && !strings.ContainsRune("#.[{(*/>+^)|]}'\"= \t", p.peek()) {
		p.pos++
	}
	return string(p.src[start:p.pos])
}

func (p *mkParser) attrList(n *mkPNode) error {
	p.pos++ // [
	for {
		for p.peek() == ' ' || p.peek() == '\t' {
			p.pos++
		}
		if p.eof() {
			return p.errf("missing ]")
		}
		if p.peek() == ']' {
			p.pos++
			return nil
		}
		a := mkPAttr{}
		if p.peek() == '!' {
			a.implied = true
			p.pos++
		}
		if q := p.peek(); q == '"' || q == '\'' {
			v, err := p.quoted()
			if err != nil {
				return err
			}
			a.value, a.hasValue = v, true // a value with no name: dropped
		} else {
			start := p.pos
			for !p.eof() && !strings.ContainsRune("= \t]", p.peek()) {
				p.pos++
			}
			a.name = string(p.src[start:p.pos])
			if strings.HasSuffix(a.name, ".") {
				a.boolean = true
				a.name = strings.TrimSuffix(a.name, ".")
			}
			if p.peek() == '=' {
				p.pos++
				a.hasValue = true
				switch p.peek() {
				case '"', '\'':
					v, err := p.quoted()
					if err != nil {
						return err
					}
					a.value = v
				case '{':
					v, err := p.braced()
					if err != nil {
						return err
					}
					a.value, a.expr = v, true
				default:
					s := p.pos
					for !p.eof() && !strings.ContainsRune(" \t]", p.peek()) {
						p.pos++
					}
					a.value = string(p.src[s:p.pos])
				}
			}
		}
		if a.name != "" {
			n.attrs = append(n.attrs, a)
		}
	}
}

// quoted reads '...' or "..." and gives the inside (escapes kept for later).
func (p *mkParser) quoted() (string, error) {
	q := p.peek()
	p.pos++
	start := p.pos
	for !p.eof() && p.peek() != q {
		if p.peek() == '\\' {
			p.pos++
		}
		p.pos++
	}
	if p.eof() {
		return "", p.errf("missing %c", q)
	}
	v := string(p.src[start:p.pos])
	p.pos++
	return v, nil
}

// braced reads {...} with nested braces and gives the inside.
func (p *mkParser) braced() (string, error) {
	end := mkMatchBrace(p.src, p.pos)
	if end < 0 {
		return "", p.errf("missing }")
	}
	v := string(p.src[p.pos+1 : end])
	p.pos = end + 1
	return v, nil
}

// mkMatchBrace finds the } that closes the { at open, or -1.
func mkMatchBrace(rs []rune, open int) int {
	depth := 0
	for i := open; i < len(rs); i++ {
		switch rs[i] {
		case '\\':
			i++
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// ---------------------------------------------------------------- unrolling

// mkTok is a piece of text: plain, or a tab stop field (text = placeholder).
type mkTok struct {
	text  string
	field bool
	index int
}

type mkAttr struct {
	name                   string
	value                  []mkTok // empty = no value (gets a tab stop)
	boolean, implied, expr bool
}

type mkRep struct {
	count, value int
	implicit     bool
}

type mkElem struct {
	name      string // "" = text only
	attrs     []*mkAttr
	value     []mkTok // text inside
	children  []*mkElem
	selfClose bool
	rep       *mkRep // this element's own repeat
}

var mkCaret = []mkTok{{field: true}}

type mkConv struct {
	reps     []mkRep
	hasText  bool     // wrapping
	lines    []string // the wrapped text, all lines
	clean    []string // the non-empty lines
	usedText bool
	copyUsed bool // $# used in the current implicit copy
	total    int
}

var errMkTooBig = errors.New("emmet: abbreviation makes too many elements")

func (c *mkConv) list(nodes []*mkPNode) ([]*mkElem, error) {
	var out []*mkElem
	for _, n := range nodes {
		count := 1
		repeated := n.repeat > 0 || n.implicit
		if n.repeat > 0 {
			count = n.repeat
		}
		if n.implicit && c.hasText && len(c.clean) > 0 {
			count = len(c.clean)
		}
		if count > mkMaxNodes {
			return nil, errMkTooBig
		}
		for v := 0; v < count; v++ {
			var rep *mkRep
			if repeated {
				r := mkRep{count: count, value: v, implicit: n.implicit}
				rep = &r
				c.reps = append(c.reps, r)
			}
			saved := c.copyUsed
			c.copyUsed = false
			var made []*mkElem
			if n.group {
				m, err := c.list(n.children)
				if err != nil {
					return nil, err
				}
				made = m
			} else {
				e, err := c.elem(n, rep)
				if err != nil {
					return nil, err
				}
				made = []*mkElem{e}
			}
			if n.implicit && c.hasText && !c.copyUsed && len(made) > 0 && v < len(c.clean) {
				mkInsertText(mkDeepest(made[len(made)-1]), strings.TrimSpace(c.clean[v]))
				c.usedText = true
			}
			c.copyUsed = saved || c.copyUsed
			if repeated {
				c.reps = c.reps[:len(c.reps)-1]
			}
			out = append(out, made...)
		}
	}
	return out, nil
}

func (c *mkConv) elem(n *mkPNode, rep *mkRep) (*mkElem, error) {
	c.total++
	if c.total > mkMaxNodes {
		return nil, errMkTooBig
	}
	e := &mkElem{name: mkFlat(c.tokens(n.name)), selfClose: n.selfClose, rep: rep}
	for _, a := range n.attrs {
		ma := &mkAttr{name: mkFlat(c.tokens(a.name)), boolean: a.boolean, implied: a.implied, expr: a.expr}
		if a.hasValue {
			ma.value = c.tokens(a.value)
		}
		e.attrs = append(e.attrs, ma)
	}
	if n.hasText {
		e.value = c.tokens(n.text)
	}
	kids, err := c.list(n.children)
	if err != nil {
		return nil, err
	}
	e.children = kids
	return e, nil
}

// tokens turns raw text into plain text and fields: \x escapes, ${1:x} fields,
// ${lang} variables, $# wrapped text, $$$@-3 numbering.
func (c *mkConv) tokens(raw string) []mkTok {
	var toks []mkTok
	var lit strings.Builder
	flush := func() {
		if lit.Len() > 0 {
			toks = append(toks, mkTok{text: lit.String()})
			lit.Reset()
		}
	}
	rs := []rune(raw)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case r == '\\' && i+1 < len(rs):
			i++
			lit.WriteRune(rs[i])
		case r == '$' && i+1 < len(rs) && rs[i+1] == '{':
			end := mkMatchBrace(rs, i+1)
			if end < 0 {
				lit.WriteRune(r)
				continue
			}
			inner := string(rs[i+2 : end])
			i = end
			num, rest, hasColon := strings.Cut(inner, ":")
			if idx, err := strconv.Atoi(num); err == nil {
				flush()
				ph := ""
				if hasColon {
					ph = mkFlat(c.tokens(rest))
				}
				toks = append(toks, mkTok{field: true, index: idx, text: ph})
			} else if v, ok := mkVariables[inner]; ok {
				lit.WriteString(v)
			} else {
				lit.WriteString(inner)
			}
		case r == '$' && i+1 < len(rs) && rs[i+1] == '#':
			i++
			lit.WriteString(c.insertText())
		case r == '$':
			j := i
			for j < len(rs) && rs[j] == '$' {
				j++
			}
			size, reverse, base := j-i, false, 1
			if j < len(rs) && rs[j] == '@' {
				j++
				if j < len(rs) && rs[j] == '-' {
					reverse = true
					j++
				}
				k := j
				for k < len(rs) && rs[k] >= '0' && rs[k] <= '9' {
					k++
				}
				if k > j {
					if b, err := strconv.Atoi(string(rs[j:k])); err == nil {
						base = b
					}
				}
				j = k
			}
			lit.WriteString(c.number(size, reverse, base))
			i = j - 1
		default:
			lit.WriteRune(r)
		}
	}
	flush()
	return toks
}

func (c *mkConv) number(size int, reverse bool, base int) string {
	v := base
	if len(c.reps) > 0 {
		r := c.reps[len(c.reps)-1]
		if reverse {
			v = base + r.count - r.value - 1
		} else {
			v = base + r.value
		}
	}
	s := strconv.Itoa(v)
	for len(s) < size {
		s = "0" + s
	}
	return s
}

func (c *mkConv) insertText() string {
	if !c.hasText {
		return ""
	}
	c.usedText = true
	c.copyUsed = true
	for i := len(c.reps) - 1; i >= 0; i-- {
		if c.reps[i].implicit {
			if v := c.reps[i].value; v < len(c.clean) {
				return strings.TrimSpace(c.clean[v])
			}
			break
		}
	}
	return strings.Join(c.lines, "\n")
}

// mkFlat joins tokens into plain text (fields give their placeholder).
func mkFlat(toks []mkTok) string {
	var b strings.Builder
	for _, t := range toks {
		b.WriteString(t.text)
	}
	return b.String()
}

func mkDeepest(e *mkElem) *mkElem {
	for len(e.children) > 0 {
		e = e.children[len(e.children)-1]
	}
	return e
}

func mkInsertText(e *mkElem, text string) {
	if n := len(e.value); n > 0 && !e.value[n-1].field {
		e.value[n-1].text += text
		return
	}
	e.value = append(e.value, mkTok{text: text})
}

var (
	mkURLRe   = regexp.MustCompile(`^(?:https?|ftp|file)://\S+$`)
	mkWWWRe   = regexp.MustCompile(`^www\.\S+$`)
	mkEmailRe = regexp.MustCompile(`^[\w.+\-]+@[\w\-]+(?:\.[\w\-]+)+$`)
)

// mkInsertHref puts a wrapped URL or e-mail address into <a href>.
func mkInsertHref(e *mkElem, text string) {
	text = strings.TrimSpace(text)
	href := ""
	switch {
	case mkURLRe.MatchString(text):
		href = text
	case mkWWWRe.MatchString(text):
		href = "http://" + text
	case mkEmailRe.MatchString(text):
		href = "mailto:" + text
	}
	if href == "" {
		return
	}
	for _, a := range e.attrs {
		if a.name == "href" {
			if len(a.value) == 0 {
				a.value = []mkTok{{text: href}}
			}
			return
		}
	}
	e.attrs = append(e.attrs, &mkAttr{name: "href", value: []mkTok{{text: href}}})
}

// ---------------------------------------------------------------- snippets

type mkResolver struct {
	syntax string
	stack  []string
	notes  []string
	rng    *mkRand
}

var mkLoremRe = regexp.MustCompile(`^(?:lorem|lipsum|ipsum)(\d*)(-\d*)?$`)

func mkIsLorem(name string) bool { return mkLoremRe.MatchString(name) }

func mkUpperFirst(s string) bool {
	r, _ := utf8.DecodeRuneInString(s)
	return unicode.IsUpper(r)
}

// lookup gives the snippet for an element's name; it fixes typos on the way.
func (r *mkResolver) lookup(e *mkElem) (string, bool) {
	if e.name == "" || (r.syntax == "jsx" && mkUpperFirst(e.name)) {
		return "", false
	}
	if s, ok := mkSnippets[e.name]; ok {
		return s, true
	}
	if mkTags[e.name] || mkIsLorem(e.name) || r.syntax == "xml" {
		return "", false
	}
	if fix := mkTypoFix(e.name); fix != "" {
		note := "fixed: " + e.name + " → " + fix
		dup := false
		for _, n := range r.notes {
			dup = dup || n == note
		}
		if !dup {
			r.notes = append(r.notes, note)
		}
		e.name = fix
		s, ok := mkSnippets[fix]
		return s, ok
	}
	return "", false
}

func (r *mkResolver) resolve(list []*mkElem) ([]*mkElem, error) {
	var out []*mkElem
	for _, child := range list {
		src, ok := r.lookup(child)
		busy := false
		for _, s := range r.stack {
			busy = busy || s == src
		}
		if !ok || busy {
			kids, err := r.resolve(child.children)
			if err != nil {
				return nil, err
			}
			child.children = kids
			out = append(out, child)
			continue
		}
		nodes, err := mkParse(src, r.syntax)
		if err != nil {
			return nil, err
		}
		made, err := (&mkConv{}).list(nodes)
		if err != nil {
			return nil, err
		}
		r.stack = append(r.stack, src)
		made, err = r.resolve(made)
		r.stack = r.stack[:len(r.stack)-1]
		if err != nil {
			return nil, err
		}
		for _, top := range made {
			for _, a := range child.attrs {
				cp := *a
				cp.value = append([]mkTok(nil), a.value...)
				top.attrs = append(top.attrs, &cp)
			}
			if child.selfClose {
				top.selfClose = true
			}
			if len(child.value) > 0 {
				top.value = mkFillValue(top.value, child.value)
			}
			if child.rep != nil {
				top.rep = child.rep
			}
		}
		kids, err := r.resolve(child.children)
		if err != nil {
			return nil, err
		}
		if len(made) > 0 {
			d := mkDeepest(made[len(made)-1])
			d.children = append(d.children, kids...)
		}
		out = append(out, made...)
	}
	return out, nil
}

// mkFillValue puts text given to a snippet into the snippet's first field
// ("c{note}" gives "<!-- note -->"), or replaces the snippet's text.
func mkFillValue(snip, text []mkTok) []mkTok {
	for i, t := range snip {
		if t.field {
			out := append([]mkTok(nil), snip[:i]...)
			out = append(out, text...)
			return append(out, snip[i+1:]...)
		}
	}
	return text
}

func (r *mkResolver) transform(list []*mkElem, anc []*mkElem) {
	for _, n := range list {
		if n.name == "" && len(n.attrs) > 0 {
			n.name = mkImplicitTag(anc)
		}
		mkMergeAttrs(n)
		if m := mkLoremRe.FindStringSubmatch(n.name); m != nil {
			r.lorem(n, anc, m)
		}
		r.transform(n.children, append(anc[:len(anc):len(anc)], n))
	}
}

func mkImplicitTag(anc []*mkElem) string {
	parent := ""
	for i := len(anc) - 1; i >= 0; i-- {
		if anc[i].name != "" {
			parent = strings.ToLower(anc[i].name)
			break
		}
	}
	if t, ok := mkImplicit[parent]; ok {
		return t
	}
	if mkInline[parent] {
		return "span"
	}
	return "div"
}

func mkMergeAttrs(n *mkElem) {
	var out []*mkAttr
	seen := map[string]*mkAttr{}
	for _, a := range n.attrs {
		prev, ok := seen[a.name]
		if a.name == "" || !ok {
			out = append(out, a)
			if a.name != "" {
				seen[a.name] = a
			}
			continue
		}
		if a.name == "class" {
			switch {
			case len(prev.value) == 0:
				prev.value = a.value
			case len(a.value) > 0:
				v := append([]mkTok(nil), prev.value...)
				v = append(v, mkTok{text: " "})
				prev.value = append(v, a.value...)
			}
			continue
		}
		prev.value = a.value
		prev.implied = prev.implied || a.implied
		prev.boolean = prev.boolean || a.boolean
		prev.expr = prev.expr || a.expr
	}
	n.attrs = out
}

func (r *mkResolver) lorem(n *mkElem, anc []*mkElem, m []string) {
	lo := 30
	if m[1] != "" {
		lo, _ = strconv.Atoi(m[1])
	}
	lo = min(max(lo, 1), mkMaxNodes)
	hi := lo
	if len(m[2]) > 1 {
		hi, _ = strconv.Atoi(m[2][1:])
		hi = min(max(hi, lo), mkMaxNodes)
	}
	count := r.rng.between(lo, hi)
	rep := n.rep
	for i := len(anc) - 1; rep == nil && i >= 0; i-- {
		rep = anc[i].rep
	}
	n.name, n.attrs = "", nil
	n.value = []mkTok{{text: r.rng.paragraph(count, rep == nil || rep.value == 0)}}
	if n.rep != nil && len(anc) > 0 {
		n.name = mkImplicitTag(anc)
	}
}

// mkTypoFix gives the common tag that word is one slip away from, or "".
// Two swapped neighbours count for any tag; one wrong, missing or extra
// letter only for tags of 5+ letters (short words are too often real words).
func mkTypoFix(word string) string {
	if len(word) < 3 || mkTypoStop[word] {
		return ""
	}
	for i := 0; i < len(word); i++ {
		if word[i] < 'a' || word[i] > 'z' {
			return ""
		}
	}
	found := ""
	for tag := range mkCommonTags {
		if len(tag) < 3 || word == tag || word == tag+"s" || !mkOneSlip(word, tag) {
			continue
		}
		if found != "" {
			return ""
		}
		found = tag
	}
	return found
}

func mkOneSlip(a, b string) bool {
	if len(a) == len(b) {
		var diff []int
		for i := 0; i < len(a); i++ {
			if a[i] != b[i] {
				diff = append(diff, i)
			}
		}
		if len(diff) == 2 && diff[1] == diff[0]+1 && a[diff[0]] == b[diff[1]] && a[diff[1]] == b[diff[0]] {
			return true
		}
		return len(diff) == 1 && len(b) >= 5
	}
	if len(b) < 5 {
		return false
	}
	long, short := a, b
	if len(long) < len(short) {
		long, short = short, long
	}
	if len(long)-len(short) != 1 {
		return false
	}
	i := 0
	for i < len(short) && long[i] == short[i] {
		i++
	}
	return long[i+1:] == short[i:]
}

// ---------------------------------------------------------------- output

type mkField struct{ index, start, end int }

type mkOut struct {
	sb           strings.Builder
	n            int // runes written
	lines        int
	base, indent string
	level        int
	fields       []mkField
}

func (o *mkOut) raw(s string) {
	o.sb.WriteString(s)
	o.n += utf8.RuneCountInString(s)
	o.lines += strings.Count(s, "\n")
}

func (o *mkOut) newline(level int) {
	o.raw("\n" + o.base + strings.Repeat(o.indent, max(level, 0)))
}

// text writes s; its line breaks are indented to the current level.
func (o *mkOut) text(s string) {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if i > 0 {
			if l == "" && i < len(lines)-1 {
				o.raw("\n" + o.base)
			} else {
				o.newline(o.level)
			}
		}
		o.raw(l)
	}
}

type mkFmt struct {
	o         *mkOut
	syntax    string
	format    bool // false with |s
	comment   bool // |c
	fieldBase int
}

func mkIsText(n *mkElem) bool { return n != nil && n.name == "" && len(n.attrs) == 0 }

func mkIsInline(n *mkElem) bool {
	if n == nil {
		return false
	}
	if n.name != "" {
		return mkInline[strings.ToLower(n.name)]
	}
	return len(n.value) > 0 && len(n.attrs) == 0
}

func mkHasNewline(toks []mkTok) bool {
	for _, t := range toks {
		if strings.Contains(t.text, "\n") {
			return true
		}
	}
	return false
}

func mkHasField(toks []mkTok) bool {
	for _, t := range toks {
		if t.field {
			return true
		}
	}
	return false
}

var mkBlockTagRe = regexp.MustCompile(`^<([\w\-:]+)[\s>]`)

func mkStartsWithBlock(toks []mkTok) bool {
	if len(toks) == 0 || toks[0].field {
		return false
	}
	m := mkBlockTagRe.FindStringSubmatch(toks[0].text)
	return m != nil && !mkInline[strings.ToLower(m[1])]
}

func (f *mkFmt) shouldFormat(n *mkElem, index int, items []*mkElem, parent *mkElem) bool {
	if !f.format || (index == 0 && parent == nil) {
		return false
	}
	if parent != nil && mkIsText(parent) && len(items) == 1 {
		return false
	}
	at := func(i int) *mkElem {
		if i < 0 || i >= len(items) {
			return nil
		}
		return items[i]
	}
	if mkIsText(n) {
		if mkIsText(at(index-1)) || mkIsText(at(index+1)) || mkHasNewline(n.value) ||
			(mkHasField(n.value) && len(n.children) > 0) {
			return true
		}
	}
	if !mkIsInline(n) {
		return true
	}
	if index == 0 {
		for _, it := range items {
			if !mkIsInline(it) {
				return true
			}
		}
	} else if !mkIsInline(items[index-1]) {
		return true
	}
	// Emmet's inlineBreak: 3+ inline elements in a row go on their own lines
	// (text does not count: "p>{Click }+a{here}+{ to go}" stays on one line)
	named := func(e *mkElem) bool { return e != nil && e.name != "" && mkIsInline(e) }
	adjacent := 1
	for i := index - 1; named(at(i)); i-- {
		adjacent++
	}
	for i := index + 1; named(at(i)); i++ {
		adjacent++
	}
	if adjacent >= 3 {
		return true
	}
	for i, ch := range n.children {
		if f.shouldFormat(ch, i, n.children, n) {
			return true
		}
	}
	return false
}

func (f *mkFmt) list(items []*mkElem, parent *mkElem) {
	for i, n := range items {
		f.elem(n, i, items, parent)
	}
}

func (f *mkFmt) indentFor(parent *mkElem) int {
	if parent == nil || mkIsText(parent) || strings.EqualFold(parent.name, "html") {
		return 0
	}
	return 1
}

func (f *mkFmt) selfCloseStr() string {
	switch f.syntax {
	case "xhtml", "jsx":
		return " /"
	case "xml":
		return "/"
	}
	return ""
}

func (f *mkFmt) elem(n *mkElem, index int, items []*mkElem, parent *mkElem) {
	o := f.o
	format := f.shouldFormat(n, index, items, parent)
	level := f.indentFor(parent)
	o.level += level
	if format {
		o.newline(o.level)
	}
	if n.name != "" {
		o.raw("<" + n.name)
		for _, a := range n.attrs {
			if a.name != "" {
				f.attr(a)
			}
		}
		empty := len(n.children) == 0 && len(n.value) == 0
		if empty && (n.selfClose || (f.syntax != "xml" && mkVoid[strings.ToLower(n.name)])) {
			o.raw(f.selfCloseStr() + ">")
		} else {
			o.raw(">")
			if !f.snippet(n) {
				if len(n.value) > 0 {
					inner := f.format && (mkHasNewline(n.value) || mkStartsWithBlock(n.value))
					if inner {
						o.level++
						o.newline(o.level)
					}
					f.tokens(n.value)
					if inner {
						o.level--
						o.newline(o.level)
					}
				}
				f.list(n.children, n)
				if empty {
					inner := f.format && strings.EqualFold(n.name, "body")
					if inner {
						o.level++
						o.newline(o.level)
					}
					f.tokens(mkCaret)
					if inner {
						o.level--
						o.newline(o.level)
					}
				}
			}
			o.raw("</" + n.name + ">")
		}
		f.commentAfter(n)
	} else if !f.snippet(n) {
		f.tokens(n.value)
		f.list(n.children, n)
	}
	if format && index == len(items)-1 && parent != nil {
		off := 1
		if mkIsText(parent) {
			off = 0
		}
		o.newline(o.level - off)
	}
	o.level -= level
}

// snippet writes text that has a field and child elements: the children go
// where the first field is ("c>div" gives "<!-- <div></div> -->").
func (f *mkFmt) snippet(n *mkElem) bool {
	if len(n.value) == 0 || len(n.children) == 0 {
		return false
	}
	ix := -1
	for i, t := range n.value {
		if t.field {
			ix = i
			break
		}
	}
	if ix < 0 {
		return false
	}
	f.tokens(n.value[:ix])
	line := f.o.lines
	f.list(n.children, n)
	rest := n.value[ix+1:]
	if f.o.lines != line && len(rest) > 0 && !rest[0].field {
		rest = append([]mkTok{{text: strings.TrimLeft(rest[0].text, " \t")}}, rest[1:]...)
	}
	f.tokens(rest)
	return true
}

func (f *mkFmt) tokens(toks []mkTok) {
	largest := -1
	for _, t := range toks {
		if !t.field {
			f.o.text(t.text)
			continue
		}
		start := f.o.n
		f.o.raw(t.text)
		f.o.fields = append(f.o.fields, mkField{f.fieldBase + t.index, start, f.o.n})
		largest = max(largest, t.index)
	}
	if largest >= 0 {
		f.fieldBase += largest + 1
	}
}

func (f *mkFmt) attr(a *mkAttr) {
	if a.implied && len(a.value) == 0 {
		return
	}
	name := a.name
	if f.syntax == "jsx" {
		switch name {
		case "class":
			name = "className"
		case "for":
			name = "htmlFor"
		}
	}
	val := a.value
	if len(val) == 0 && (a.boolean || mkBoolAttrs[strings.ToLower(a.name)]) {
		if f.syntax == "html" || f.syntax == "jsx" {
			f.o.raw(" " + name)
			return
		}
		val = []mkTok{{text: name}}
	}
	if len(val) == 0 {
		val = mkCaret
	}
	open, close := `"`, `"`
	if a.expr && f.syntax == "jsx" {
		open, close = "{", "}"
	}
	f.o.raw(" " + name + "=" + open)
	f.tokens(val)
	f.o.raw(close)
}

func (f *mkFmt) commentAfter(n *mkElem) {
	if !f.comment {
		return
	}
	var id, cls string
	has := false
	for _, a := range n.attrs {
		switch a.name {
		case "id":
			id, has = mkFlat(a.value), true
		case "class":
			cls, has = mkFlat(a.value), true
		}
	}
	if !has {
		return
	}
	s := "<!-- /"
	if id != "" {
		s += "#" + id
	}
	if cls != "" {
		s += "." + cls
	}
	if f.format {
		f.o.newline(f.o.level)
	}
	f.o.raw(s + " -->")
}

// ---------------------------------------------------------------- entry points

// mkSplitFilters splits "ul>li|c|t" into "ul>li" and {c, t}.
func mkSplitFilters(s string) (string, map[string]bool) {
	depth := 0
	var quote rune
	for i, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '"' || r == '\'':
			if depth > 0 {
				quote = r
			}
		case r == '[' || r == '{' || r == '(':
			depth++
		case r == ']' || r == '}' || r == ')':
			depth--
		case r == '|' && depth == 0:
			filters := map[string]bool{}
			for _, f := range strings.Split(s[i+1:], "|") {
				if f == "" {
					continue
				}
				for _, c := range f {
					if c < 'a' || c > 'z' {
						return s, nil
					}
				}
				filters[f] = true
			}
			return s[:i], filters
		}
	}
	return s, nil
}

var mkTrimRe = regexp.MustCompile(`^[\s\x{00a0}]?[\d#\-*\x{2022}]+\.?\s*`)

func mkExpand(abbr string, opt Options, text []string, wrapping bool) (Expansion, error) {
	syntax := opt.Syntax
	if !IsMarkup(syntax) {
		syntax = "html"
	}
	indent := opt.Indent
	if indent == "" {
		indent = "\t"
	}
	core, filters := mkSplitFilters(strings.TrimSpace(abbr))
	if core == "" {
		return Expansion{}, errors.New("emmet: empty abbreviation")
	}
	nodes, err := mkParse(core, syntax)
	if err != nil {
		return Expansion{}, err
	}
	conv := &mkConv{}
	if wrapping {
		if filters["t"] {
			for i, l := range text {
				text[i] = mkTrimRe.ReplaceAllString(l, "")
			}
		}
		conv.hasText, conv.lines = true, text
		for _, l := range text {
			if strings.TrimSpace(l) != "" {
				conv.clean = append(conv.clean, l)
			}
		}
	}
	elems, err := conv.list(nodes)
	if err != nil {
		return Expansion{}, err
	}
	if wrapping && !conv.usedText && len(elems) > 0 {
		d := mkDeepest(elems[len(elems)-1])
		joined := strings.Join(text, "\n")
		mkInsertText(d, joined)
		if d.name == "a" && len(text) == 1 {
			mkInsertHref(d, joined)
		}
	}
	r := &mkResolver{syntax: syntax, rng: mkNewRand(abbr)}
	if elems, err = r.resolve(elems); err != nil {
		return Expansion{}, err
	}
	r.transform(elems, nil)
	f := &mkFmt{
		o:       &mkOut{base: opt.BaseIndent, indent: indent},
		syntax:  syntax,
		format:  !filters["s"],
		comment: filters["c"],
	}
	f.list(elems, nil)
	fields := f.o.fields
	sort.SliceStable(fields, func(i, j int) bool { return fields[i].index < fields[j].index })
	var stops []Range
	for i, fl := range fields {
		if i > 0 && fl.index == fields[i-1].index {
			continue
		}
		stops = append(stops, Range{fl.start, fl.end})
	}
	return Expansion{Text: f.o.sb.String(), Stops: stops, Note: strings.Join(r.notes, ", ")}, nil
}

func expandMarkup(abbr string, opt Options) (Expansion, error) {
	return mkExpand(abbr, opt, nil, false)
}

// Wrap puts content inside the abbreviation: "ul>li*" wraps each line in its own li.
// The content goes where $# is, or into the innermost last element. Its common
// indent is removed; "|t" takes list markers ("1.", "-", "*") off each line.
func Wrap(abbr string, content string, opt Options) (Expansion, error) {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		lines = []string{""}
	}
	prefix := ""
	first := true
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		lead := l[:len(l)-len(strings.TrimLeft(l, " \t"))]
		if first {
			prefix, first = lead, false
			continue
		}
		for !strings.HasPrefix(lead, prefix) {
			prefix = prefix[:len(prefix)-1]
		}
	}
	for i, l := range lines {
		lines[i] = strings.TrimRight(strings.TrimPrefix(l, prefix), " \t")
	}
	return mkExpand(abbr, opt, lines, true)
}

// ---------------------------------------------------------------- extract

func mkAbbrRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("#.>+^*:$@-_!|/%", r)
}

func extractMarkup(line []rune, col int) (string, int, bool) {
	col = min(col, len(line))
	if col <= 0 || mkInsideTag(line, col) || mkInsideQuote(line, col) {
		return "", 0, false
	}
	i := col
	var stack []rune
scan:
	for i > 0 {
		c := line[i-1]
		top := rune(0)
		if len(stack) > 0 {
			top = stack[len(stack)-1]
		}
		if top == ']' || top == '}' {
			switch {
			case top == ']' && (c == '"' || c == '\''):
				j := i - 2
				for j >= 0 && line[j] != c {
					j--
				}
				if j < 0 {
					return "", 0, false
				}
				i = j
				continue
			case (top == ']' && c == '[') || (top == '}' && c == '{'):
				stack = stack[:len(stack)-1]
			case c == '}' || (top == ']' && c == ']'):
				stack = append(stack, c)
			}
			i--
			continue
		}
		switch {
		case c == ']' || c == '}' || c == ')':
			stack = append(stack, c)
		case c == '(':
			if top != ')' {
				break scan
			}
			stack = stack[:len(stack)-1]
		case c == '>' && mkTagEndsAt(line, i-1):
			break scan
		case mkAbbrRune(c):
		default:
			break scan
		}
		i--
	}
	if len(stack) > 0 {
		return "", 0, false
	}
	for i < col && strings.ContainsRune(">+^*|/", line[i]) {
		i++
	}
	if i >= col {
		return "", 0, false
	}
	return string(line[i:col]), i, true
}

// mkTagEndsAt tells whether the > at gt closes an HTML tag like <div class="x">.
func mkTagEndsAt(line []rune, gt int) bool {
	k := gt - 1
	for k >= 0 && line[k] != '<' && line[k] != '>' {
		k--
	}
	if k < 0 || line[k] != '<' || k+1 >= gt {
		return false
	}
	c := line[k+1]
	return unicode.IsLetter(c) || c == '/' || c == '!'
}

// mkInsideTag tells whether col is inside an open tag's attributes: <a href="x|
func mkInsideTag(line []rune, col int) bool {
	lt, gt := -1, -1
	for i := 0; i < col; i++ {
		switch line[i] {
		case '<':
			lt = i
		case '>':
			gt = i
		}
	}
	return lt > gt && lt+1 < col && (unicode.IsLetter(line[lt+1]) || line[lt+1] == '/')
}

// mkInsideQuote tells whether col is inside "..." (or '...' that is not an apostrophe).
func mkInsideQuote(line []rune, col int) bool {
	dq, sq := 0, 0
	for i := 0; i < col; i++ {
		switch line[i] {
		case '"':
			dq++
		case '\'':
			letterBefore := i > 0 && unicode.IsLetter(line[i-1])
			letterAfter := i+1 < len(line) && unicode.IsLetter(line[i+1])
			if !(letterBefore && letterAfter) {
				sq++
			}
		}
	}
	return dq%2 == 1 || sq%2 == 1
}

// ---------------------------------------------------------------- tab worthy

var (
	mkNumberRe    = regexp.MustCompile(`^[\d.+\-*/^()%]+$`)
	mkPlainWordRe = regexp.MustCompile(`^(?:!+[\w:]*|[A-Za-z][\w:\-]*\+?)$`)
	mkClassOnlyRe = regexp.MustCompile(`^([A-Za-z][\w:\-]*)?(?:[.#][^.#\s\[\]{}()>+^*/|]+)+$`)
)

// mkKnownWord tells whether a lone word is clearly an element for Tab.
func mkKnownWord(w, syntax string) bool {
	if syntax == "jsx" {
		if mkUpperFirst(w) || mkJSWords[w] || utf8.RuneCountInString(w) == 1 {
			return false
		}
	}
	if _, ok := mkSnippets[w]; ok {
		return true
	}
	if mkTags[w] || mkIsLorem(w) {
		return true
	}
	return syntax != "xml" && mkTypoFix(w) != ""
}

func markupTabWorthy(abbr, syntax string) bool {
	a := strings.TrimSpace(abbr)
	if a == "" || mkNumberRe.MatchString(a) {
		return false
	}
	if !IsMarkup(syntax) {
		syntax = "html"
	}
	core, _ := mkSplitFilters(a)
	if core == "" {
		return false
	}
	if _, err := mkParse(core, syntax); err != nil {
		return false
	}
	if mkPlainWordRe.MatchString(core) {
		return mkKnownWord(core, syntax)
	}
	// "word.word" or "word#word" alone: only when the word is a tag
	// (so "index.html" or "this.props" do not turn into elements)
	if m := mkClassOnlyRe.FindStringSubmatch(core); m != nil {
		name := m[1]
		switch {
		case name == "":
			return true
		case syntax == "jsx" && mkUpperFirst(name):
			return true
		case strings.Contains(name, "-"):
			return true
		}
		return mkKnownWord(name, syntax)
	}
	return true
}
