package emmet

// Emmet CSS abbreviations: a port of the stylesheet half of emmet 2.4
// (@emmetio/css-abbreviation and emmet/src/stylesheet), the version VS Code
// uses, with VS Code's settings (fuzzy match threshold 0.3).
//
// On top of Emmet it adds a few things (switched off by cssPure for tests
// that compare with real Emmet output):
//   - full property names: "font-size12" → "font-size: 12px;", "display" uses the d snippet
//   - property name slips: "dispaly" → display, "colour" → color
//   - Emmet 1 vendor prefixes: "-bdrs", "-wm-trf"
//   - more unitless properties (order, grid-row, ...)
//   - "!imp" → "!important" (as VS Code does), "@cs" → @charset

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

// ---------------------------------------------------------------- tokens

const (
	cssTField = iota + 1
	cssTCustom
	cssTNumber
	cssTColor
	cssTString
	cssTBracket
	cssTOperator
	cssTSpace
	cssTLiteral
	cssTFunc
)

type cssTok struct {
	kind       int
	start, end int    // -1: not from the abbreviation
	value      string // literal text, custom property, string text, function name
	full       bool   // literal is a whole CSS property name (our addition)

	index    int    // field
	hasIndex bool   // field
	name     string // field placeholder

	num  float64 // number
	raw  string  // number as typed
	unit string  // number

	r, g, b int     // colour
	a       float64 // colour

	quote rune // string
	open  bool // bracket
	op    rune // operator

	args []*cssVal // function call
}

type cssVal struct{ toks []*cssTok }

type cssNode struct {
	name      string
	value     []*cssVal
	important bool
	full      bool
	snippet   *cssSnippet
	vendors   []string

	how   int     // how the name was resolved (cssHow*)
	score float64 // fuzzy score when how == cssHowFuzzy
	note  string
}

const (
	cssHowNone   = iota
	cssHowExact  // the name is a snippet key
	cssHowFull   // the name is a whole property name
	cssHowTypo   // the name is a slip of a whole property name
	cssHowJoined // a key with a keyword joined on: "dib", "posa"
	cssHowFuzzy  // Emmet's fuzzy match
	cssHowRaw    // a text snippet (@m, cm)
)

// ---------------------------------------------------------------- scanner

type cssScanner struct {
	s          []rune
	pos, start int
}

func (sc *cssScanner) eof() bool { return sc.pos >= len(sc.s) }

func (sc *cssScanner) peek() rune {
	if sc.eof() {
		return -1
	}
	return sc.s[sc.pos]
}

func (sc *cssScanner) eat(r rune) bool {
	if !sc.eof() && sc.s[sc.pos] == r {
		sc.pos++
		return true
	}
	return false
}

func (sc *cssScanner) eatF(f func(rune) bool) bool {
	if !sc.eof() && f(sc.s[sc.pos]) {
		sc.pos++
		return true
	}
	return false
}

func (sc *cssScanner) eatWhile(f func(rune) bool) bool {
	start := sc.pos
	for sc.eatF(f) {
	}
	return sc.pos != start
}

func (sc *cssScanner) current() string { return string(sc.s[sc.start:sc.pos]) }

func (sc *cssScanner) sub(a, b int) string {
	if b > len(sc.s) {
		b = len(sc.s)
	}
	if a > b {
		return ""
	}
	return string(sc.s[a:b])
}

func (sc *cssScanner) errorf(msg string, pos int) error {
	return fmt.Errorf("%s at %d", msg, pos)
}

func cssIsNum(c rune) bool { return c > 47 && c < 58 }

func cssIsAlpha(c rune) bool {
	u := c &^ 32
	return c >= 0 && u >= 'A' && u <= 'Z'
}

func cssIsAlphaWord(c rune) bool    { return c == '_' || cssIsAlpha(c) }
func cssIsAlphaNumWord(c rune) bool { return cssIsNum(c) || cssIsAlphaWord(c) }
func cssIsSpace(c rune) bool        { return c == ' ' || c == '\t' || c == 0xa0 || c == '\n' || c == '\r' }
func cssIsQuote(c rune) bool        { return c == '\'' || c == '"' }
func cssIsHex(c rune) bool          { u := c &^ 32; return cssIsNum(c) || (c >= 0 && u >= 'A' && u <= 'F') }
func cssIsKeyword(c rune) bool      { return cssIsAlphaNumWord(c) || c == '-' }
func cssIsLiteralCh(c rune) bool    { return cssIsAlphaWord(c) || c == '%' || c == '/' }
func cssIsIdentPrefix(c rune) bool  { return c == '@' || c == '$' }

// ---------------------------------------------------------------- tokenizer

func cssTokenize(abbr string, isValue, fullNames bool) ([]*cssTok, error) {
	brackets := 0
	sc := &cssScanner{s: []rune(abbr)}
	var tokens []*cssTok
	if fullNames && !isValue {
		if n := cssFullNamePrefix(sc.s); n > 0 {
			tokens = append(tokens, &cssTok{kind: cssTLiteral, value: string(sc.s[:n]), start: 0, end: n, full: true})
			sc.pos = n
		} else if word, prop := cssHyphenSlip(sc.s); prop != "" {
			// "background-colour": a slip in a hyphenated property name
			n := len([]rune(word))
			tokens = append(tokens, &cssTok{kind: cssTLiteral, value: prop, raw: word, start: 0, end: n, full: true})
			sc.pos = n
		}
	}
	for !sc.eof() {
		tok, err := cssGetToken(sc, brackets == 0 && !isValue)
		if err != nil {
			return nil, err
		}
		if tok == nil {
			return nil, sc.errorf("Unexpected character", sc.pos)
		}
		if tok.kind == cssTBracket {
			if brackets == 0 && tok.open {
				tokens = cssMergeTokens(sc, tokens)
			}
			if tok.open {
				brackets++
			} else {
				brackets--
			}
			if brackets < 0 {
				return nil, sc.errorf("Unexpected bracket", tok.start)
			}
		}
		tokens = append(tokens, tok)
		// After a unit-less number or a colour, the next operator (a dash)
		// is a value delimiter, not a minus sign.
		if tok.kind == cssTColor || (tok.kind == cssTNumber && tok.unit == "") {
			if op := cssOperatorTok(sc); op != nil {
				tokens = append(tokens, op)
			}
		}
	}
	return tokens, nil
}

// cssFullNamePrefix gives the length of the whole CSS property name that
// starts s ("font-size" in "font-size12"), or 0. Snippet keys and names
// shorter than 3 letters are left to Emmet.
func cssFullNamePrefix(s []rune) int {
	for n := len(s); n >= 3; n-- {
		name := string(s[:n])
		if !cssPropSet[name] || cssSnippetKeys()[name] {
			continue
		}
		if n < len(s) {
			c := s[n]
			if cssIsAlpha(c) || c == '(' || c == '_' {
				continue
			}
			if c == '-' && n+1 < len(s) && cssIsAlpha(s[n+1]) {
				continue
			}
		}
		return n
	}
	return 0
}

// cssHyphenSlip reads the hyphenated word that starts s ("background-colour"
// in "background-colour#f") and gives the property it is a slip of.
func cssHyphenSlip(s []rune) (word, prop string) {
	n := 0
	for n < len(s) && (cssIsAlpha(s[n]) || s[n] == '-') {
		n++
	}
	for n > 0 && s[n-1] == '-' {
		n--
	}
	word = strings.ToLower(string(s[:n]))
	if n < 5 || !strings.Contains(strings.Trim(word, "-"), "-") || word[0] == '-' || cssPropSet[word] {
		return "", ""
	}
	if n < len(s) && (s[n] == '(' || s[n] == '_') {
		return "", ""
	}
	if p := corFixName(word); p != "" {
		return string(s[:n]), p
	}
	return "", ""
}

func cssGetToken(sc *cssScanner, short bool) (*cssTok, error) {
	if t, err := cssFieldTok(sc); t != nil || err != nil {
		return t, err
	}
	if t := cssCustomTok(sc); t != nil {
		return t, nil
	}
	if t := cssNumberTok(sc); t != nil {
		return t, nil
	}
	if t := cssColorTok(sc); t != nil {
		return t, nil
	}
	if t := cssStringTok(sc); t != nil {
		return t, nil
	}
	if t := cssBracketTok(sc); t != nil {
		return t, nil
	}
	if t := cssOperatorTok(sc); t != nil {
		return t, nil
	}
	if t := cssSpaceTok(sc); t != nil {
		return t, nil
	}
	return cssLiteralTok(sc, short), nil
}

func cssFieldTok(sc *cssScanner) (*cssTok, error) {
	start := sc.pos
	if sc.eat('$') && sc.eat('{') {
		sc.start = sc.pos
		t := &cssTok{kind: cssTField, start: start}
		if sc.eatWhile(cssIsNum) {
			t.index, _ = strconv.Atoi(sc.current())
			t.hasIndex = true
			if sc.eat(':') {
				name, err := cssPlaceholder(sc)
				if err != nil {
					return nil, err
				}
				t.name = name
			}
		} else if cssIsAlpha(sc.peek()) {
			name, err := cssPlaceholder(sc)
			if err != nil {
				return nil, err
			}
			t.name = name
		}
		if sc.eat('}') {
			t.end = sc.pos
			return t, nil
		}
		return nil, sc.errorf("Expecting }", sc.pos)
	}
	sc.pos = start
	return nil, nil
}

func cssPlaceholder(sc *cssScanner) (string, error) {
	var stack []int
	sc.start = sc.pos
	for !sc.eof() {
		if sc.eat('{') {
			stack = append(stack, sc.pos)
		} else if sc.eat('}') {
			if len(stack) == 0 {
				sc.pos--
				break
			}
			stack = stack[:len(stack)-1]
		} else {
			sc.pos++
		}
	}
	if len(stack) > 0 {
		sc.pos = stack[len(stack)-1]
		return "", sc.errorf("Expecting }", sc.pos)
	}
	return sc.current(), nil
}

func cssLiteralTok(sc *cssScanner, short bool) *cssTok {
	start := sc.pos
	if sc.eatF(cssIsIdentPrefix) {
		// SCSS or LESS variable
		if start != 0 {
			sc.eatWhile(cssIsKeyword)
		} else {
			sc.eatWhile(cssIsLiteralCh)
		}
	} else if sc.eatF(cssIsAlphaWord) {
		if short {
			sc.eatWhile(cssIsLiteralCh)
		} else {
			sc.eatWhile(cssIsKeyword)
		}
	} else {
		sc.eat('.') // dots only at the start of a literal
		sc.eatWhile(cssIsLiteralCh)
	}
	if start != sc.pos {
		sc.start = start
		return cssLiteralAt(sc, start, sc.pos)
	}
	return nil
}

func cssLiteralAt(sc *cssScanner, start, end int) *cssTok {
	return &cssTok{kind: cssTLiteral, value: sc.sub(start, end), start: start, end: end}
}

func cssNumberTok(sc *cssScanner) *cssTok {
	start := sc.pos
	if cssConsumeNumber(sc) {
		sc.start = start
		raw := sc.current()
		sc.start = sc.pos
		if !sc.eat('%') {
			sc.eatWhile(cssIsAlphaWord)
		}
		v, _ := strconv.ParseFloat(raw, 64)
		return &cssTok{kind: cssTNumber, num: v, raw: raw, unit: sc.current(), start: start, end: sc.pos}
	}
	return nil
}

func cssConsumeNumber(sc *cssScanner) bool {
	start := sc.pos
	sc.eat('-')
	afterNegative := sc.pos
	hasDecimal := sc.eatWhile(cssIsNum)
	prev := sc.pos
	if sc.eat('.') {
		// "1." is fine: it makes the number a float
		hasFloat := sc.eatWhile(cssIsNum)
		if !hasDecimal && !hasFloat {
			sc.pos = prev // a lone dot
		}
	}
	if sc.pos == afterNegative {
		sc.pos = start // only a dash: not a number
	}
	return sc.pos != start
}

func cssStringTok(sc *cssScanner) *cssTok {
	ch := sc.peek()
	start := sc.pos
	if !cssIsQuote(ch) {
		return nil
	}
	finished := false
	sc.pos++
	for !sc.eof() {
		if sc.eat(ch) {
			finished = true
			break
		}
		sc.pos++
	}
	sc.start = start
	end := sc.pos
	if finished {
		end--
	}
	return &cssTok{kind: cssTString, value: sc.sub(start+1, end), quote: ch, start: start, end: sc.pos}
}

// cssColorTok reads #abc, #0, #fff.5 (with alpha) and #t (transparent).
func cssColorTok(sc *cssScanner) *cssTok {
	start := sc.pos
	if !sc.eat('#') {
		sc.pos = start
		return nil
	}
	valueStart := sc.pos
	color, alpha := "", ""
	if sc.eatWhile(cssIsHex) {
		color = sc.sub(valueStart, sc.pos)
		alpha = cssColorAlpha(sc)
	} else if sc.eat('t') {
		color = "0"
		alpha = cssColorAlpha(sc)
		if alpha == "" {
			alpha = "0"
		}
	} else {
		alpha = cssColorAlpha(sc)
	}
	if color != "" || alpha != "" || sc.eof() {
		t := &cssTok{kind: cssTColor, raw: sc.sub(start+1, sc.pos), start: start, end: sc.pos}
		t.r, t.g, t.b, t.a = cssParseColor(color, alpha)
		return t
	}
	// a # with no value: a literal
	return cssLiteralAt(sc, start, sc.pos)
}

func cssColorAlpha(sc *cssScanner) string {
	start := sc.pos
	if sc.eat('.') {
		sc.start = start
		if sc.eatWhile(cssIsNum) {
			return sc.current()
		}
		return "1"
	}
	return ""
}

func cssSpaceTok(sc *cssScanner) *cssTok {
	start := sc.pos
	if sc.eatWhile(cssIsSpace) {
		return &cssTok{kind: cssTSpace, start: start, end: sc.pos}
	}
	return nil
}

func cssCustomTok(sc *cssScanner) *cssTok {
	start := sc.pos
	if sc.eat('-') && sc.eat('-') {
		sc.start = start
		sc.eatWhile(cssIsKeyword)
		return &cssTok{kind: cssTCustom, value: sc.current(), start: start, end: sc.pos}
	}
	sc.pos = start
	return nil
}

func cssBracketTok(sc *cssScanner) *cssTok {
	ch := sc.peek()
	if ch == '(' || ch == ')' {
		t := &cssTok{kind: cssTBracket, open: ch == '(', start: sc.pos, end: sc.pos + 1}
		sc.pos++
		return t
	}
	return nil
}

func cssOperatorTok(sc *cssScanner) *cssTok {
	switch ch := sc.peek(); ch {
	case '+', '!', ',', ':', '-':
		t := &cssTok{kind: cssTOperator, op: ch, start: sc.pos, end: sc.pos + 1}
		sc.pos++
		return t
	}
	return nil
}

func cssParseColor(value, alpha string) (r, g, b int, a float64) {
	rs, gs, bs := "0", "0", "0"
	a = 1
	if alpha != "" {
		a, _ = strconv.ParseFloat(alpha, 64)
	}
	if value == "t" {
		a = 0
	} else {
		switch len(value) {
		case 0:
		case 1:
			rs = value + value
			gs, bs = rs, rs
		case 2:
			rs, gs, bs = value, value, value
		case 3:
			rs = value[0:1] + value[0:1]
			gs = value[1:2] + value[1:2]
			bs = value[2:3] + value[2:3]
		default:
			value += value
			rs, gs, bs = value[0:2], value[2:4], value[4:6]
		}
	}
	p := func(s string) int { v, _ := strconv.ParseInt(s, 16, 64); return int(v) }
	return p(rs), p(gs), p(bs), a
}

// cssMergeTokens joins the literal and number tokens just before a "(" into
// one literal, so "scale3d(" is a function name, not "scale" and "3d".
func cssMergeTokens(sc *cssScanner, tokens []*cssTok) []*cssTok {
	start, end := 0, 0
	for len(tokens) > 0 {
		t := tokens[len(tokens)-1]
		if t.kind == cssTLiteral || t.kind == cssTNumber {
			start = t.start
			if end == 0 {
				end = t.end
			}
			tokens = tokens[:len(tokens)-1]
		} else {
			break
		}
	}
	if start != end {
		tokens = append(tokens, cssLiteralAt(sc, start, end))
	}
	return tokens
}

// ---------------------------------------------------------------- parser

type cssTokScanner struct {
	toks []*cssTok
	pos  int
}

func (ts *cssTokScanner) peek() *cssTok {
	if ts.pos < len(ts.toks) {
		return ts.toks[ts.pos]
	}
	return nil
}

func (ts *cssTokScanner) readable() bool { return ts.pos < len(ts.toks) }

func (ts *cssTokScanner) consume(test func(*cssTok) bool) bool {
	if t := ts.peek(); t != nil && test(t) {
		ts.pos++
		return true
	}
	return false
}

func (ts *cssTokScanner) errorf(msg string) error {
	if t := ts.peek(); t != nil && t.start >= 0 {
		return fmt.Errorf("%s at %d", msg, t.start)
	}
	return errors.New(msg)
}

func cssIsLiteral(t *cssTok) bool      { return t != nil && t.kind == cssTLiteral }
func cssIsOpenBracket(t *cssTok) bool  { return t != nil && t.kind == cssTBracket && t.open }
func cssIsCloseBracket(t *cssTok) bool { return t != nil && t.kind == cssTBracket && !t.open }
func cssIsWhiteSpace(t *cssTok) bool   { return t != nil && t.kind == cssTSpace }
func cssIsOp(t *cssTok, op rune) bool  { return t != nil && t.kind == cssTOperator && t.op == op }
func cssIsSibling(t *cssTok) bool      { return cssIsOp(t, '+') }
func cssIsArgDelim(t *cssTok) bool     { return cssIsOp(t, ',') }
func cssIsImportant(t *cssTok) bool    { return cssIsOp(t, '!') }
func cssIsValueDelim(t *cssTok) bool   { return cssIsOp(t, ':') || cssIsOp(t, '-') }

func cssIsValueTok(t *cssTok) bool {
	switch t.kind {
	case cssTString, cssTColor, cssTNumber, cssTLiteral, cssTField, cssTCustom:
		return true
	}
	return false
}

func cssParse(tokens []*cssTok, valueMode bool) ([]*cssNode, error) {
	ts := &cssTokScanner{toks: tokens}
	var result []*cssNode
	for ts.readable() {
		p, err := cssConsumeProperty(ts, valueMode)
		if err != nil {
			return nil, err
		}
		if p != nil {
			result = append(result, p)
		} else if !ts.consume(cssIsSibling) {
			return nil, ts.errorf("Unexpected token")
		}
	}
	return result, nil
}

func cssConsumeProperty(ts *cssTokScanner, valueMode bool) (*cssNode, error) {
	node := &cssNode{}
	t := ts.peek()
	if !valueMode && cssIsLiteral(t) && !cssIsFunctionStart(ts) {
		ts.pos++
		node.name = t.value
		node.full = t.full
		if t.full && t.raw != "" {
			node.note = "fixed: " + t.raw + " → " + t.value
		}
		ts.consume(cssIsValueDelim)
	}
	if valueMode {
		ts.consume(cssIsWhiteSpace)
	}
	for ts.readable() {
		if ts.consume(cssIsImportant) {
			node.important = true
			continue
		}
		v, err := cssConsumeValue(ts, valueMode)
		if err != nil {
			return nil, err
		}
		if v != nil {
			node.value = append(node.value, v)
		} else if !ts.consume(cssIsArgDelim) {
			break
		}
	}
	if node.name != "" || len(node.value) > 0 || node.important {
		return node, nil
	}
	return nil, nil
}

func cssConsumeValue(ts *cssTokScanner, inArgument bool) (*cssVal, error) {
	var result []*cssTok
	for ts.readable() {
		t := ts.peek()
		if cssIsValueTok(t) {
			ts.pos++
			if cssIsLiteral(t) {
				args, ok, err := cssConsumeArguments(ts)
				if err != nil {
					return nil, err
				}
				if ok {
					result = append(result, &cssTok{kind: cssTFunc, value: t.value, args: args, start: -1, end: -1})
					continue
				}
			}
			result = append(result, t)
		} else if cssIsValueDelim(t) || (inArgument && cssIsWhiteSpace(t)) {
			ts.pos++
		} else {
			break
		}
	}
	if len(result) > 0 {
		return &cssVal{toks: result}, nil
	}
	return nil, nil
}

func cssConsumeArguments(ts *cssTokScanner) ([]*cssVal, bool, error) {
	if !ts.consume(cssIsOpenBracket) {
		return nil, false, nil
	}
	args := []*cssVal{}
	for ts.readable() && !ts.consume(cssIsCloseBracket) {
		v, err := cssConsumeValue(ts, true)
		if err != nil {
			return nil, false, err
		}
		if v != nil {
			args = append(args, v)
		} else if !ts.consume(cssIsWhiteSpace) && !ts.consume(cssIsArgDelim) {
			return nil, false, ts.errorf("Unexpected token")
		}
	}
	return args, true, nil
}

func cssIsFunctionStart(ts *cssTokScanner) bool {
	if ts.pos+1 >= len(ts.toks) {
		return false
	}
	return cssIsLiteral(ts.toks[ts.pos]) && ts.toks[ts.pos+1].kind == cssTBracket
}

// ---------------------------------------------------------------- snippets

type cssSnippet struct {
	raw      bool
	key      string
	text     string      // raw snippet text
	property string      // property snippet
	value    [][]*cssVal // property snippet: the values, first is the default
	kwKeys   []string    // keyword names in JavaScript object key order
	kw       map[string]*cssTok
	deps     []*cssSnippet // more specific properties: background → background-position
}

var cssReProperty = regexp.MustCompile(`^([a-z-]+)(?:\s*:\s*([^\n\r;]+?);*)?$`)

func cssCreateSnippet(key, value string) *cssSnippet {
	m := cssReProperty.FindStringSubmatch(value)
	if m == nil {
		return &cssSnippet{raw: true, key: key, text: value}
	}
	s := &cssSnippet{key: key, property: m[1], kw: map[string]*cssTok{}}
	if m[2] != "" {
		for _, part := range strings.Split(m[2], "|") {
			toks, err := cssTokenize(strings.TrimSpace(part), true, false)
			if err != nil {
				continue
			}
			nodes, err := cssParse(toks, true)
			if err != nil || len(nodes) == 0 {
				continue
			}
			s.value = append(s.value, nodes[0].value)
		}
	}
	set := func(k string, t *cssTok) {
		if _, ok := s.kw[k]; !ok {
			s.kwKeys = append(s.kwKeys, k)
		}
		s.kw[k] = t
	}
	for _, item := range s.value {
		for _, v := range item {
			for _, t := range v.toks {
				switch t.kind {
				case cssTLiteral:
					set(t.value, t)
				case cssTFunc:
					set(t.value, t)
				case cssTField:
					if n := strings.TrimSpace(t.name); n != "" {
						set(n, &cssTok{kind: cssTLiteral, value: n, start: -1, end: -1})
					}
				}
			}
		}
	}
	// JavaScript lists integer-like object keys first, in number order.
	sort.SliceStable(s.kwKeys, func(i, j int) bool {
		ai, aok := cssArrayIndex(s.kwKeys[i])
		bi, bok := cssArrayIndex(s.kwKeys[j])
		if aok && bok {
			return ai < bi
		}
		return aok && !bok
	})
	return s
}

func cssArrayIndex(s string) (int, bool) {
	if s == "" || (len(s) > 1 && s[0] == '0') {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 || strconv.Itoa(n) != s {
		return 0, false
	}
	return n, true
}

// cssNest sorts the snippets by key and makes each property snippet list
// its more specific properties: background → background-position → background-position-x.
func cssNest(snippets []*cssSnippet) []*cssSnippet {
	sort.SliceStable(snippets, func(i, j int) bool { return snippets[i].key < snippets[j].key })
	var stack []*cssSnippet
	for _, cur := range snippets {
		if cur.raw {
			continue
		}
		for len(stack) > 0 {
			prev := stack[len(stack)-1]
			if strings.HasPrefix(cur.property, prev.property) && len(cur.property) > len(prev.property) &&
				cur.property[len(prev.property)] == '-' {
				prev.deps = append(prev.deps, cur)
				stack = append(stack, cur)
				break
			}
			stack = stack[:len(stack)-1]
		}
		if len(stack) == 0 {
			stack = append(stack, cur)
		}
	}
	return snippets
}

// cssSnippetsExtra are snippets Emmet 2 does not have.
var cssSnippetsExtra = [][2]string{
	{"@cs|@charset", "@charset \"${1:UTF-8}\";"},
}

var (
	cssSnipOnce    sync.Once
	cssSnipPure    []*cssSnippet
	cssSnipFull    []*cssSnippet
	cssSnipKeySet  map[string]bool
	cssSnipByProp  map[string]*cssSnippet
	cssUnitlessPur = cssMakeSet([]string{"z-index", "line-height", "opacity", "font-weight", "zoom",
		"flex", "flex-grow", "flex-shrink"})
	cssUnitlessMore = cssMakeSet([]string{"order", "orphans", "widows", "column-count",
		"animation-iteration-count", "fill-opacity", "stroke-opacity", "flood-opacity",
		"stop-opacity", "tab-size", "scale", "line-clamp", "grid-row", "grid-column",
		"grid-row-start", "grid-row-end", "grid-column-start", "grid-column-end", "grid-area",
		"aspect-ratio", "font-size-adjust", "stroke-miterlimit"})
)

func cssBuildSnippets(raw [][2]string) []*cssSnippet {
	var list []*cssSnippet
	for _, kv := range raw {
		for _, k := range strings.Split(kv[0], "|") {
			list = append(list, cssCreateSnippet(k, kv[1]))
		}
	}
	return cssNest(list)
}

func cssLoadSnippets() {
	cssSnipOnce.Do(func() {
		cssSnipPure = cssBuildSnippets(cssSnippetsRaw)
		cssSnipFull = cssBuildSnippets(append(append([][2]string{}, cssSnippetsRaw...), cssSnippetsExtra...))
		cssSnipKeySet = map[string]bool{}
		cssSnipByProp = map[string]*cssSnippet{}
		for _, s := range cssSnipFull {
			cssSnipKeySet[s.key] = true
			if s.raw {
				continue
			}
			// the shortest key is the main snippet of a property: color → c (not cr, cra)
			if old, ok := cssSnipByProp[s.property]; !ok || len(s.key) < len(old.key) {
				cssSnipByProp[s.property] = s
			}
		}
	})
}

func cssSnippets(pure bool) []*cssSnippet {
	cssLoadSnippets()
	if pure {
		return cssSnipPure
	}
	return cssSnipFull
}

func cssSnippetKeys() map[string]bool {
	cssLoadSnippets()
	return cssSnipKeySet
}

// ---------------------------------------------------------------- fuzzy match

// cssScoreMatch is Emmet's fuzzy score of how well abbreviation a matches s:
// 1 for the same, 0 for no match. Both must start with the same letter.
func cssScoreMatch(a, s string, partial bool) float64 {
	s1 := []rune(strings.ToLower(a))
	s2 := []rune(strings.ToLower(s))
	if string(s1) == string(s2) {
		return 1
	}
	if len(s1) == 0 || len(s2) == 0 || s1[0] != s2[0] {
		return 0
	}
	l1, l2 := len(s1), len(s2)
	if !partial && l1 > l2 {
		return 0
	}
	minLen, maxLen := l1, l2
	if minLen > maxLen {
		minLen, maxLen = maxLen, minLen
	}
	i, j := 1, 1
	score := maxLen
	for i < l1 {
		ch1 := s1[i]
		found, acronym := false, false
		for j < l2 {
			ch2 := s2[j]
			if ch1 == ch2 {
				found = true
				if acronym {
					score += maxLen - i
				} else {
					score += maxLen - j
				}
				break
			}
			acronym = ch2 == '-' // bonus for a match right after "-"
			j++
		}
		if !found {
			if !partial {
				return 0
			}
			break
		}
		i++
	}
	ratio := float64(i) / float64(maxLen)
	delta := maxLen - minLen
	maxScore := cssSum(maxLen) - cssSum(delta)
	return float64(score) * ratio / maxScore
}

func cssSum(n int) float64 { return float64(n*(n+1)) / 2 }

func cssBestSnippet(abbr string, items []*cssSnippet, minScore float64, partial bool) (*cssSnippet, float64) {
	var best *cssSnippet
	max := 0.0
	for _, it := range items {
		sc := cssScoreMatch(abbr, it.key, partial)
		if sc == 1 {
			return it, 1
		}
		if sc != 0 && sc >= max {
			max, best = sc, it
		}
	}
	if max >= minScore {
		return best, max
	}
	return nil, 0
}

func cssBestString(abbr string, items []string, minScore float64) string {
	best := ""
	max := 0.0
	for _, it := range items {
		sc := cssScoreMatch(abbr, it, false)
		if sc == 1 {
			return it
		}
		if sc != 0 && sc >= max {
			max, best = sc, it
		}
	}
	if max >= minScore {
		return best
	}
	return ""
}

// cssUnmatchedPart is the part of abbr not found in order in s:
// "poas" against "position" leaves "as".
func cssUnmatchedPart(abbr, s string) string {
	ar := []rune(abbr)
	sr := []rune(s)
	last := 0
	for i, c := range ar {
		k := -1
		for j := last; j < len(sr); j++ {
			if sr[j] == c {
				k = j
				break
			}
		}
		if k == -1 {
			return string(ar[i:])
		}
		last = k + 1
	}
	return ""
}

// ---------------------------------------------------------------- resolving

const cssFuzzyMinScore = 0.3 // VS Code's emmet.preferences css.fuzzySearchMinScore

var cssGlobalKw = []string{"auto", "inherit", "unset", "none"}

func cssLit(v string) *cssTok { return &cssTok{kind: cssTLiteral, value: v, start: -1, end: -1} }

func cssFieldT(index int, name string) *cssTok {
	return &cssTok{kind: cssTField, index: index, hasIndex: true, name: name, start: -1, end: -1}
}

func cssCloneTok(t *cssTok) *cssTok {
	c := *t
	if t.args != nil {
		c.args = cssCloneVals(t.args)
	}
	return &c
}

func cssCloneVals(vs []*cssVal) []*cssVal {
	out := make([]*cssVal, len(vs))
	for i, v := range vs {
		nv := &cssVal{toks: make([]*cssTok, len(v.toks))}
		for k, t := range v.toks {
			nv.toks[k] = cssCloneTok(t)
		}
		out[i] = nv
	}
	return out
}

func cssResolveNode(node *cssNode, snippets []*cssSnippet, pure bool) {
	if !cssResolveGradient(node) && node.name != "" {
		cssResolveName(node, snippets, pure)
	}
	if node.name != "" {
		cssResolveNumbers(node, pure)
	}
}

func cssResolveName(node *cssNode, snippets []*cssSnippet, pure bool) {
	name := node.name
	if !pure {
		if node.full {
			cssResolveFull(node, name)
			node.how = cssHowFull
			if node.note != "" {
				node.how = cssHowTypo
			}
			return
		}
		if !cssSnippetKeys()[name] && !cssHTMLTags[name] && cssIsWord(name) && len([]rune(name)) >= 5 {
			// a slip in a whole property name ("dispaly"), unless Emmet reads
			// it as a key with a keyword joined on
			s, _ := cssBestSnippet(name, snippets, cssFuzzyMinScore, true)
			joined := s != nil && !s.raw && len(node.value) == 0 && cssUnmatchedPart(name, s.key) != "" &&
				cssResolveKeyword(cssUnmatchedPart(name, s.key), s, 0) != nil
			if !joined {
				if p := cssClosestWord(strings.ToLower(name), cssPropNames, cssPropSet); p != "" {
					cssResolveFull(node, p)
					node.how = cssHowTypo
					node.note = "fixed: " + name + " → " + p
					return
				}
			}
		}
	}
	snippet, score := cssBestSnippet(name, snippets, cssFuzzyMinScore, true)
	node.snippet = snippet
	if snippet == nil {
		return
	}
	if snippet.raw {
		cssResolveAsSnippet(node, snippet)
		node.how = cssHowRaw
		return
	}
	how := cssHowFuzzy
	if score == 1 {
		how = cssHowExact
	}
	inline := cssUnmatchedPart(name, snippet.key)
	if inline != "" {
		if len(node.value) > 0 {
			return // a value as well: the match is wrong
		}
		kw := cssResolveKeyword(inline, snippet, 0)
		if kw == nil {
			return
		}
		node.value = append(node.value, &cssVal{toks: []*cssTok{cssCloneTok(kw)}})
		how = cssHowJoined
	}
	node.how, node.score = how, score
	cssResolveAsProperty(node, snippet)
}

func cssIsWord(s string) bool {
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '-') {
			return false
		}
	}
	return s != ""
}

// cssResolveFull resolves a node named with a whole property name, using the
// property's snippet for its keywords and default value when there is one.
func cssResolveFull(node *cssNode, prop string) {
	cssLoadSnippets()
	node.name = prop
	if s := cssSnipByProp[prop]; s != nil {
		node.snippet = s
		cssResolveAsProperty(node, s)
	}
}

func cssResolveAsProperty(node *cssNode, s *cssSnippet) {
	node.name = s.property
	if len(node.value) > 0 {
		cssResolveValueKeywords(node, s, 0)
	} else if len(s.value) > 0 {
		def := cssCloneVals(s.value[0])
		hasField := false
		for _, v := range def {
			if cssHasField(v) {
				hasField = true
			}
		}
		// with several values to pick from, the default is a placeholder
		if len(s.value) == 1 || hasField {
			node.value = def
		} else {
			for i, v := range def {
				idx := 1 // Emmet numbers each value's fields from 1
				def[i] = cssWrapWithField(v, &idx)
			}
			node.value = def
		}
	}
}

func cssResolveValueKeywords(node *cssNode, s *cssSnippet, minScore float64) {
	for _, v := range node.value {
		var out []*cssTok
		for _, t := range v.toks {
			switch t.kind {
			case cssTLiteral:
				if kw := cssResolveKeyword(t.value, s, minScore); kw != nil {
					out = append(out, cssCloneTok(kw))
				} else {
					out = append(out, t)
				}
			case cssTFunc:
				match := cssResolveKeyword(t.value, s, minScore)
				if match != nil && match.kind == cssTFunc {
					m := cssCloneTok(match)
					args := append([]*cssVal{}, t.args...)
					if len(m.args) > len(t.args) {
						args = append(args, m.args[len(t.args):]...)
					}
					m.args = args
					out = append(out, m)
				} else {
					out = append(out, t)
				}
			default:
				out = append(out, t)
			}
		}
		v.toks = out
	}
}

func cssResolveKeyword(kw string, s *cssSnippet, minScore float64) *cssTok {
	if s != nil {
		if ref := cssBestString(kw, s.kwKeys, minScore); ref != "" {
			return s.kw[ref]
		}
		for _, dep := range s.deps {
			if ref := cssBestString(kw, dep.kwKeys, minScore); ref != "" {
				return dep.kw[ref]
			}
		}
	}
	if ref := cssBestString(kw, cssGlobalKw, minScore); ref != "" {
		return cssLit(ref)
	}
	return nil
}

var cssReField = regexp.MustCompile(`\$\{(\d+)(:[^}]+)?\}`)

func cssResolveAsSnippet(node *cssNode, s *cssSnippet) {
	var in []*cssTok
	if len(node.value) > 0 {
		in = node.value[0].toks
	}
	var out []*cssTok
	offset := 0
	for _, m := range cssReField.FindAllStringSubmatchIndex(s.text, -1) {
		if offset != m[0] {
			out = append(out, cssLit(s.text[offset:m[0]]))
		}
		offset = m[1]
		if len(in) > 0 {
			out = append(out, in[0])
			in = in[1:]
		} else {
			idx, _ := strconv.Atoi(s.text[m[2]:m[3]])
			name := ""
			if m[4] >= 0 {
				name = s.text[m[4]+1 : m[5]]
			}
			out = append(out, cssFieldT(idx, name))
		}
	}
	if tail := s.text[offset:]; tail != "" {
		out = append(out, cssLit(tail))
	}
	node.name = ""
	node.value = []*cssVal{{toks: out}}
}

// cssResolveGradient turns "lg(...)" into a linear-gradient background.
func cssResolveGradient(node *cssNode) bool {
	var fn *cssTok
	if len(node.value) == 1 && len(node.value[0].toks) == 1 {
		if t := node.value[0].toks[0]; t.kind == cssTFunc && t.value == "lg" {
			fn = t
		}
	}
	if fn == nil && node.name != "lg" {
		return false
	}
	if fn == nil {
		fn = &cssTok{kind: cssTFunc, value: "linear-gradient", start: -1, end: -1,
			args: []*cssVal{{toks: []*cssTok{cssFieldT(0, "")}}}}
	} else {
		c := *fn
		c.value = "linear-gradient"
		fn = &c
	}
	node.name = "background-image"
	node.value = []*cssVal{{toks: []*cssTok{fn}}}
	return true
}

func cssResolveNumbers(node *cssNode, pure bool) {
	aliases := map[string]string{"e": "em", "p": "%", "x": "ex", "r": "rem"}
	unitless := cssUnitlessPur[node.name] || (!pure && cssUnitlessMore[node.name])
	for _, v := range node.value {
		for _, t := range v.toks {
			if t.kind != cssTNumber {
				continue
			}
			if t.unit != "" {
				if a, ok := aliases[t.unit]; ok {
					t.unit = a
				}
			} else if t.num != 0 && !unitless {
				if strings.Contains(t.raw, ".") {
					t.unit = "em"
				} else {
					t.unit = "px"
				}
			}
		}
	}
}

func cssHasField(v *cssVal) bool {
	for _, t := range v.toks {
		if t.kind == cssTField {
			return true
		}
		if t.kind == cssTFunc {
			for _, a := range t.args {
				if cssHasField(a) {
					return true
				}
			}
		}
	}
	return false
}

func cssWrapWithField(v *cssVal, idx *int) *cssVal {
	var out []*cssTok
	for _, t := range v.toks {
		switch t.kind {
		case cssTColor:
			out = append(out, cssFieldT(*idx, cssColorText(t)))
			*idx++
		case cssTLiteral:
			out = append(out, cssFieldT(*idx, t.value))
			*idx++
		case cssTNumber:
			out = append(out, cssFieldT(*idx, strconv.FormatFloat(t.num, 'f', -1, 64)+t.unit))
			*idx++
		case cssTString:
			q := "\""
			if t.quote == '\'' {
				q = "'"
			}
			out = append(out, cssFieldT(*idx, q+t.value+q))
			*idx++
		case cssTFunc:
			out = append(out, cssFieldT(*idx, t.value), cssLit("("))
			*idx++
			for i, a := range t.args {
				out = append(out, cssWrapWithField(a, idx).toks...)
				if i != len(t.args)-1 {
					out = append(out, cssLit(", "))
				}
			}
			out = append(out, cssLit(")"))
		default:
			out = append(out, t)
		}
	}
	return &cssVal{toks: out}
}

// ---------------------------------------------------------------- output

// cssFieldPos is a field written to the output. group is the declaration it
// is in: stops go declaration by declaration, by field number in each.
type cssFieldPos struct{ group, index, start, end int }

type cssOut struct {
	b       []rune
	fields  []cssFieldPos
	base    string
	indent  string
	between string
	after   string
	group   int
	nested  bool // read ${n:x} fields inside placeholders, as VS Code does
}

func (o *cssOut) push(s string) { o.b = append(o.b, []rune(s)...) }

func (o *cssOut) pushString(s string) {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.ReplaceAll(s, "\t", o.indent)
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		o.push(l)
		if i != len(lines)-1 {
			o.newline()
		}
	}
}

func (o *cssOut) newline() { o.push("\n" + o.base) }

func (o *cssOut) field(index int, ph string) {
	st := len(o.b)
	var inner []cssFieldPos
	if o.nested && strings.Contains(ph, "${") {
		// "'${0}'" (Emmet's cnt): the placeholder holds a field of its own
		rest := ph
		for {
			m := cssReField.FindStringSubmatchIndex(rest)
			if m == nil {
				break
			}
			o.push(rest[:m[0]])
			idx, _ := strconv.Atoi(rest[m[2]:m[3]])
			text := ""
			if m[4] >= 0 {
				text = rest[m[4]+1 : m[5]]
			}
			is := len(o.b)
			o.push(text)
			inner = append(inner, cssFieldPos{o.group, idx, is, len(o.b)})
			rest = rest[m[1]:]
		}
		o.push(rest)
	} else {
		o.push(ph)
	}
	o.fields = append(o.fields, cssFieldPos{o.group, index, st, len(o.b)})
	o.fields = append(o.fields, inner...)
}

func (o *cssOut) property(node *cssNode) {
	if node.name == "" {
		// a text snippet: the tokens as they are
		for _, v := range node.value {
			for _, t := range v.toks {
				o.token(t)
			}
		}
		o.important(node, len(node.value) > 0)
		return
	}
	for _, vendor := range node.vendors {
		o.declaration(node, "-"+vendor+"-"+node.name)
		o.newline()
		o.group++
	}
	o.declaration(node, node.name)
}

func (o *cssOut) declaration(node *cssNode, name string) {
	o.pushString(name + o.between)
	if len(node.value) > 0 {
		for i, v := range node.value {
			if i != 0 {
				o.push(", ")
			}
			o.value(v)
		}
	} else {
		o.field(0, "")
	}
	o.important(node, true)
	o.push(o.after)
}

func (o *cssOut) important(node *cssNode, sep bool) {
	if node.important {
		if sep {
			o.push(" ")
		}
		o.push("!important")
	}
}

func (o *cssOut) value(v *cssVal) {
	prevEnd := -1
	for i, t := range v.toks {
		// a field written right after the previous token ("foo${1}") gets no space
		if i != 0 && (t.kind != cssTField || t.start != prevEnd) {
			o.push(" ")
		}
		o.token(t)
		prevEnd = t.end
	}
}

func (o *cssOut) token(t *cssTok) {
	switch t.kind {
	case cssTColor:
		o.push(cssColorText(t))
	case cssTLiteral, cssTCustom:
		o.pushString(t.value)
	case cssTNumber:
		o.pushString(cssFrac(t.num, 4) + t.unit)
	case cssTString:
		q := "'"
		if t.quote == '"' {
			q = "\""
		}
		o.pushString(q + t.value + q)
	case cssTField:
		o.field(t.index, t.name)
	case cssTFunc:
		o.push(t.value + "(")
		for i, a := range t.args {
			if i != 0 {
				o.push(", ")
			}
			o.value(a)
		}
		o.push(")")
	}
}

var cssReTrailZeros = regexp.MustCompile(`\.?0+$`)

func cssFrac(n float64, digits int) string {
	return cssReTrailZeros.ReplaceAllString(strconv.FormatFloat(n, 'f', digits, 64), "")
}

func cssColorText(t *cssTok) string {
	if t.r == 0 && t.g == 0 && t.b == 0 && t.a == 0 {
		return "transparent"
	}
	if t.a == 1 {
		short := t.r%17 == 0 && t.g%17 == 0 && t.b%17 == 0
		f := func(n int) string {
			if short {
				return strconv.FormatInt(int64(n>>4), 16)
			}
			s := strconv.FormatInt(int64(n), 16)
			if len(s) < 2 {
				s = "0" + s
			}
			return s
		}
		return "#" + f(t.r) + f(t.g) + f(t.b)
	}
	return fmt.Sprintf("rgba(%d, %d, %d, %s)", t.r, t.g, t.b, cssFrac(t.a, 8))
}

// ---------------------------------------------------------------- expand

func expandCSS(abbr string, opt Options) (Expansion, error) {
	return cssExpand(abbr, opt, false)
}

// cssExpandNodes parses and resolves abbr. With pure set it is exactly
// Emmet 2; otherwise our additions are on.
func cssExpandNodes(abbr string, pure bool) ([]*cssNode, error) {
	snippets := cssSnippets(pure)
	var parts []string
	if pure {
		parts = []string{abbr}
	} else {
		parts = cssSplitChain(abbr)
	}
	var nodes []*cssNode
	for _, part := range parts {
		var vendors []string
		if !pure {
			part, vendors = cssVendorPrefix(part)
		}
		if part == "" {
			continue
		}
		toks, err := cssTokenize(part, false, !pure)
		if err != nil {
			return nil, err
		}
		ns, err := cssParse(toks, false)
		if err != nil {
			return nil, err
		}
		for _, n := range ns {
			cssResolveNode(n, snippets, pure)
			if vendors != nil && n.name != "" {
				n.vendors = vendors
				if len(vendors) == 1 && vendors[0] == "" {
					n.vendors = cssVendorsFor(n.name)
				}
			}
			nodes = append(nodes, n)
		}
	}
	return nodes, nil
}

func cssExpand(abbr string, opt Options, pure bool) (Expansion, error) {
	abbr = strings.TrimSpace(abbr)
	if abbr == "" {
		return Expansion{}, errors.New("empty abbreviation")
	}
	if !pure && len(abbr) > 1 && strings.HasPrefix("!important", abbr) {
		return Expansion{Text: "!important"}, nil
	}
	nodes, err := cssExpandNodes(abbr, pure)
	if err != nil {
		return Expansion{}, err
	}
	if len(nodes) == 0 {
		return Expansion{}, errors.New("nothing to expand")
	}
	o := &cssOut{base: opt.BaseIndent, indent: opt.Indent, between: ": ", after: ";", nested: !pure}
	if o.indent == "" {
		o.indent = "\t"
	}
	switch opt.Syntax {
	case "sass":
		o.after = ""
	case "stylus":
		o.between, o.after = " ", ""
	}
	var notes []string
	for i, n := range nodes {
		if i != 0 {
			o.newline()
			o.group++
		}
		o.property(n)
		if n.note != "" {
			notes = append(notes, strings.TrimPrefix(n.note, "fixed: "))
		}
	}
	exp := Expansion{Text: string(o.b)}
	if len(notes) > 0 {
		exp.Note = "fixed: " + strings.Join(notes, ", ")
	}
	// stops: fields in number order, $0 (the final place) last
	fields := append([]cssFieldPos{}, o.fields...)
	key := func(f cssFieldPos) int {
		if f.index <= 0 {
			return math.MaxInt32
		}
		return f.index
	}
	sort.SliceStable(fields, func(i, j int) bool {
		if fields[i].group != fields[j].group {
			return fields[i].group < fields[j].group
		}
		return key(fields[i]) < key(fields[j])
	})
	for _, f := range fields {
		exp.Stops = append(exp.Stops, Range{f.start, f.end})
	}
	return exp, nil
}

// cssSplitChain splits "m10+p5" at the "+" signs outside brackets and quotes.
func cssSplitChain(abbr string) []string {
	var parts []string
	depth := 0
	var quote rune
	start := 0
	rs := []rune(abbr)
	for i, c := range rs {
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		case c == '(':
			depth++
		case c == ')':
			depth--
		case c == '+' && depth == 0:
			parts = append(parts, string(rs[start:i]))
			start = i + 1
		}
	}
	return append(parts, string(rs[start:]))
}

// cssVendorPrefix reads Emmet 1's vendor prefix: "-bdrs" (the engines that
// need it: vendors is [""]) or "-wm-trf" (webkit and moz).
func cssVendorPrefix(part string) (string, []string) {
	if len(part) < 2 || part[0] != '-' || !cssIsAlpha(rune(part[1])) {
		return part, nil
	}
	rest := part[1:]
	if i := strings.IndexByte(rest, '-'); i > 0 && i+1 < len(rest) && cssIsAlpha(rune(rest[i+1])) {
		letters := rest[:i]
		var vendors []string
		ok := true
		for _, v := range cssVendorOrder {
			if strings.ContainsRune(letters, v.letter) {
				vendors = append(vendors, v.name)
			}
		}
		for _, c := range letters {
			if !strings.ContainsRune("wmso", c) {
				ok = false
			}
		}
		if ok && len(vendors) > 0 {
			return rest[i+1:], vendors
		}
	}
	return rest, []string{""}
}

// cssVendorsFor lists the engines Emmet 1's data says need a prefix for
// prop; all of them when it does not know the property.
func cssVendorsFor(prop string) []string {
	var out []string
	for _, v := range cssVendorOrder {
		if cssVendorProps[v.name][prop] {
			out = append(out, v.name)
		}
	}
	if len(out) == 0 {
		for _, v := range cssVendorOrder {
			out = append(out, v.name)
		}
	}
	return out
}

// ---------------------------------------------------------------- extract

// extractCSS finds the abbreviation that ends at col: back to a space, "{",
// "}", ";" or the start of the line.
func extractCSS(line []rune, col int) (string, int, bool) {
	if col <= 0 || col > len(line) {
		return "", 0, false
	}
	if cssInCommentOrString(line, col) {
		return "", 0, false
	}
	start := col
	for start > 0 {
		c := line[start-1]
		if unicode.IsSpace(c) || c == '{' || c == '}' || c == ';' {
			break
		}
		start--
	}
	if start == col {
		return "", 0, false
	}
	abbr := line[start:col]
	for _, c := range abbr {
		if !(c < 128 && (cssIsAlphaNumWord(c) || strings.ContainsRune("-+!#.:%@$(),'\"/", c))) {
			return "", 0, false
		}
	}
	switch abbr[0] {
	case '.', '&', ':', '#', '$', '(', ')', ',', '/', '+':
		return "", 0, false
	}
	s := string(abbr)
	if strings.HasPrefix(s, "--") || strings.HasSuffix(s, ":") {
		return "", 0, false
	}
	// something written before it in the same statement: a selector, or the
	// value part of a declaration ("color: re")
	mask := cssMaskLine(line)
	j := start
	for j > 0 && (unicode.IsSpace(line[j-1]) || mask[j-1] == cssMaskComment) {
		j--
	}
	if j > 0 {
		if c := line[j-1]; c != '{' && c != '}' && c != ';' {
			return "", 0, false
		}
	}
	return s, start, true
}

// cssInCommentOrString tells whether col is inside a /* comment */, a
// // comment or a quoted string.
func cssInCommentOrString(line []rune, col int) bool {
	_, state := cssScan(line, col)
	return state != cssMaskCode
}

const (
	cssMaskCode = iota
	cssMaskComment
	cssMaskString
)

// cssMaskLine marks each letter of line as code, comment or string.
// Quote marks and comment marks count as string or comment.
func cssMaskLine(line []rune) []int {
	mask, _ := cssScan(line, len(line))
	return mask
}

// cssScan marks line[:upto] and gives the state at upto.
func cssScan(line []rune, upto int) ([]int, int) {
	if upto > len(line) {
		upto = len(line)
	}
	mask := make([]int, len(line))
	var quote rune
	inComment, lineComment := false, false
	depth := 0
	for i := 0; i < upto; i++ {
		c := line[i]
		switch {
		case lineComment:
			mask[i] = cssMaskComment
		case inComment:
			mask[i] = cssMaskComment
			if c == '*' && i+1 < len(line) && line[i+1] == '/' {
				if i+1 < upto {
					i++
					mask[i] = cssMaskComment
					inComment = false
				}
			}
		case quote != 0:
			mask[i] = cssMaskString
			if c == '\\' && i+1 < upto {
				i++
				mask[i] = cssMaskString
			} else if c == quote {
				quote = 0
			}
		case c == '/' && i+1 < len(line) && line[i+1] == '*':
			inComment = true
			mask[i] = cssMaskComment
			if i+1 < upto {
				i++
				mask[i] = cssMaskComment
			}
		case c == '/' && i+1 < len(line) && line[i+1] == '/' && depth == 0 &&
			(i == 0 || unicode.IsSpace(line[i-1]) || strings.ContainsRune(";{}", line[i-1])):
			lineComment = true
			mask[i] = cssMaskComment
		case c == '"' || c == '\'':
			quote = c
			mask[i] = cssMaskString
		case c == '(':
			depth++
		case c == ')':
			if depth > 0 {
				depth--
			}
		}
	}
	state := cssMaskCode
	if inComment || lineComment {
		state = cssMaskComment
	} else if quote != 0 {
		state = cssMaskString
	}
	return mask, state
}

// ---------------------------------------------------------------- tab

// cssTabWorthy tells whether Tab should expand abbr: a snippet, a property,
// a value form ("m10", "c#f", "d:f", "dib"), a chain, a vendor form or an
// @-snippet. Plain words that read as selectors or values are not.
func cssTabWorthy(abbr string) bool {
	if abbr == "" || strings.ContainsAny(abbr, " \t") {
		return false
	}
	if strings.HasPrefix(abbr, "!") {
		return strings.HasPrefix("!important", abbr)
	}
	parts := cssSplitChain(abbr)
	any := false
	for _, p := range parts {
		if p == "" {
			continue
		}
		if !cssTabWorthyPart(p) {
			return false
		}
		any = true
	}
	return any
}

func cssTabWorthyPart(part string) bool {
	part, _ = cssVendorPrefix(part)
	lower := strings.ToLower(part)
	if i := strings.IndexByte(lower, ':'); i >= 0 {
		// "a:hover", "li:first-child": a selector
		rest := []rune(lower[i+1:])
		if corStartsWithPseudo(rest, 0, len(rest)) || cssHTMLTags[lower[:i]] && !cssSnippetKeys()[lower[:i]] {
			return false
		}
	}
	if cssIsWord(part) && !cssSnippetKeys()[part] && !cssPropSet[lower] {
		// a plain word that is a tag name or a value is not an abbreviation
		if cssHTMLTags[lower] || cssColourNames[lower] || cssGlobalKeywords[lower] || cssIsKeywordValue(lower) {
			return false
		}
	}
	nodes, err := cssExpandNodes(part, false)
	if err != nil || len(nodes) == 0 {
		return false
	}
	for _, n := range nodes {
		switch n.how {
		case cssHowExact, cssHowFull, cssHowTypo, cssHowJoined, cssHowRaw:
		case cssHowFuzzy:
			if n.score < cssFuzzyMinScore {
				return false
			}
		default:
			if n.name == "background-image" && strings.HasPrefix(part, "lg") {
				continue // the gradient shortcut
			}
			return false
		}
	}
	return true
}

func cssIsKeywordValue(w string) bool {
	for _, set := range cssKeywords {
		for _, k := range set {
			if k == w {
				return true
			}
		}
	}
	return false
}
