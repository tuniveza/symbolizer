package emmet

// Emmet's "evaluate math expression" and "increment / decrement number".

import (
	"errors"
	"math"
	"strconv"
	"strings"
)

func actIsDigit(r rune) bool { return r >= '0' && r <= '9' }

func actIsWordChar(r rune) bool { return actIsNameStart(r) || actIsDigit(r) || r == '_' }

// EvalMath works out the highlighted expression, or else the one that ends at
// the cursor ("10*3+2|"), and puts the result in its place. Operators:
// + - * / % ( ) and \ (integer division). Up to 4 decimals, no trailing zeros.
// The cursor goes after the result.
func EvalMath(text []rune, sel Range) (Change, bool) {
	s, e := sel.Start, sel.End
	if s == e {
		var ok bool
		if s, ok = actExtractMath(text, e); !ok {
			return Change{}, false
		}
		for e > s && text[e-1] == ' ' {
			e--
		}
	}
	if s < 0 || e > len(text) || s >= e {
		return Change{}, false
	}
	expr := strings.TrimSpace(string(text[s:e]))
	if expr == "" {
		return Change{}, false
	}
	if _, err := strconv.ParseFloat(expr, 64); err == nil {
		return Change{}, false // a plain number: nothing to work out
	}
	v, err := actEvalExpr(expr)
	if err != nil {
		return Change{}, false
	}
	res := actFormatNum(v, 4)
	// keep surrounding spaces of a highlighted expression out of the edit
	for s < e && actIsSpace(text[s]) {
		s++
	}
	for e > s && actIsSpace(text[e-1]) {
		e--
	}
	p := s + len([]rune(res))
	return Change{Edits: []Edit{{s, e, res}}, Sel: Range{p, p}}, true
}

// actExtractMath walks back from pos over a math expression and gives its start.
func actExtractMath(text []rune, pos int) (int, bool) {
	e := pos
	for e > 0 && text[e-1] == ' ' {
		e--
	}
	s := e
	depth := 0
	for s > 0 {
		r := text[s-1]
		switch {
		case r == ')':
			depth++
		case r == '(':
			if depth == 0 {
				goto done
			}
			depth--
		case actIsDigit(r) || strings.ContainsRune(".+-*/%\\ ", r):
		default:
			goto done
		}
		s--
	}
done:
	if depth != 0 {
		return 0, false
	}
	for s < e && (text[s] == ' ' || strings.ContainsRune("*/%\\", text[s])) {
		s++
	}
	// "item-10+5": the '-' joins a word, it is not a sign
	if s < e && (text[s] == '-' || text[s] == '+') && s > 0 && actIsWordChar(text[s-1]) {
		s++
	}
	if s >= e {
		return 0, false
	}
	return s, true
}

type actMathParser struct {
	s   string
	pos int
}

func actEvalExpr(expr string) (float64, error) {
	p := &actMathParser{s: expr}
	v, err := p.sum()
	if err != nil {
		return 0, err
	}
	p.space()
	if p.pos != len(p.s) {
		return 0, errors.New("unexpected " + p.s[p.pos:])
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, errors.New("not a number")
	}
	return v, nil
}

func (p *actMathParser) space() {
	for p.pos < len(p.s) && p.s[p.pos] == ' ' {
		p.pos++
	}
}

func (p *actMathParser) peek() byte {
	p.space()
	if p.pos < len(p.s) {
		return p.s[p.pos]
	}
	return 0
}

func (p *actMathParser) sum() (float64, error) {
	v, err := p.product()
	if err != nil {
		return 0, err
	}
	for {
		switch p.peek() {
		case '+':
			p.pos++
			w, err := p.product()
			if err != nil {
				return 0, err
			}
			v += w
		case '-':
			p.pos++
			w, err := p.product()
			if err != nil {
				return 0, err
			}
			v -= w
		default:
			return v, nil
		}
	}
}

func (p *actMathParser) product() (float64, error) {
	v, err := p.unary()
	if err != nil {
		return 0, err
	}
	for {
		op := p.peek()
		if op != '*' && op != '/' && op != '%' && op != '\\' {
			return v, nil
		}
		p.pos++
		w, err := p.unary()
		if err != nil {
			return 0, err
		}
		if w == 0 && op != '*' {
			return 0, errors.New("division by zero")
		}
		switch op {
		case '*':
			v *= w
		case '/':
			v /= w
		case '%':
			v = math.Mod(v, w)
		case '\\':
			v = math.Floor(v / w)
		}
	}
}

func (p *actMathParser) unary() (float64, error) {
	switch p.peek() {
	case '-':
		p.pos++
		v, err := p.unary()
		return -v, err
	case '+':
		p.pos++
		return p.unary()
	case '(':
		p.pos++
		v, err := p.sum()
		if err != nil {
			return 0, err
		}
		if p.peek() != ')' {
			return 0, errors.New("missing )")
		}
		p.pos++
		return v, nil
	}
	st := p.pos
	for p.pos < len(p.s) && (p.s[p.pos] >= '0' && p.s[p.pos] <= '9' || p.s[p.pos] == '.') {
		p.pos++
	}
	if st == p.pos {
		return 0, errors.New("number expected")
	}
	return strconv.ParseFloat(p.s[st:p.pos], 64)
}

// actFormatNum rounds to at most dec decimals and drops trailing zeros.
func actFormatNum(v float64, dec int) string {
	m := math.Pow(10, float64(dec))
	v = math.Round(v*m) / m
	if v == 0 {
		v = 0 // no "-0"
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// IncrementNumber adds delta to the number at (or just before) the cursor, or
// to the highlighted number. Decimals, negatives, zero padding ("007") and the
// number of decimal places ("1.50") are kept; a unit after it is untouched.
// Sel highlights the new number.
func IncrementNumber(text []rune, sel Range, delta float64) (Change, bool) {
	s, e, ok := actNumberAt(text, sel)
	if !ok {
		return Change{}, false
	}
	old := string(text[s:e])
	res, ok := actUpdateNumber(old, delta)
	if !ok {
		return Change{}, false
	}
	return Change{Edits: []Edit{{s, e, res}}, Sel: Range{s, s + len([]rune(res))}}, true
}

func actNumberAt(text []rune, sel Range) (int, int, bool) {
	isNum := func(r rune) bool { return actIsDigit(r) || r == '.' }
	var s, e int
	if sel.Start < sel.End {
		s, e = sel.Start, sel.End
		for s < e && actIsSpace(text[s]) {
			s++
		}
		for e > s && actIsSpace(text[e-1]) {
			e--
		}
		for i := s; i < e; i++ {
			if !isNum(text[i]) && !(i == s && text[i] == '-') {
				return 0, 0, false
			}
		}
	} else {
		p := sel.Start
		if p > len(text) {
			return 0, 0, false
		}
		s, e = p, p
		for s > 0 && isNum(text[s-1]) {
			s--
		}
		for e < len(text) && isNum(text[e]) {
			e++
		}
		// a trailing '.' is not part of the number ("10." at a sentence end)
		for e > s && text[e-1] == '.' && (e == len(text) || !actIsDigit(text[e])) {
			e--
		}
		for s < e && text[s] == '.' && !(s+1 < e && actIsDigit(text[s+1])) {
			s++
		}
		if s > 0 && text[s-1] == '-' && (s < 2 || !(actIsWordChar(text[s-2]) || text[s-2] == ')' || text[s-2] == ']')) {
			s--
		}
	}
	if e <= s {
		return 0, 0, false
	}
	if strings.Count(string(text[s:e]), ".") > 1 {
		return 0, 0, false // "1.2.3": a version, not a number
	}
	hasDigit := false
	for _, r := range text[s:e] {
		if actIsDigit(r) {
			hasDigit = true
		}
	}
	return s, e, hasDigit
}

// actUpdateNumber adds delta to num and writes it in the same style.
func actUpdateNumber(num string, delta float64) (string, bool) {
	v, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return "", false
	}
	decimals := 0
	if i := strings.IndexByte(num, '.'); i >= 0 {
		decimals = len(num) - i - 1
	}
	dd := 0
	if ds := strconv.FormatFloat(math.Abs(delta), 'f', -1, 64); strings.Contains(ds, ".") {
		dd = len(ds) - strings.IndexByte(ds, '.') - 1
	}
	if dd > decimals {
		decimals = dd
	}
	v += delta
	m := math.Pow(10, float64(decimals))
	v = math.Round(v*m) / m
	neg := v < 0
	out := strconv.FormatFloat(math.Abs(v), 'f', decimals, 64)
	// zero padding: "007" stays three digits wide
	digits := strings.TrimPrefix(num, "-")
	if i := strings.IndexByte(digits, '.'); i >= 0 {
		digits = digits[:i]
	}
	if len(digits) > 1 && digits[0] == '0' {
		ip := out
		frac := ""
		if i := strings.IndexByte(out, '.'); i >= 0 {
			ip, frac = out[:i], out[i:]
		}
		for len(ip) < len(digits) {
			ip = "0" + ip
		}
		out = ip + frac
	}
	if strings.HasPrefix(strings.TrimPrefix(num, "-"), ".") && strings.HasPrefix(out, "0.") {
		out = out[1:]
	}
	if neg && strings.Trim(out, "0.") != "" {
		out = "-" + out
	}
	return out, true
}
