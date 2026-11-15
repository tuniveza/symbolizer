package engine

// A JSON parser that keeps what JSON.parse gives JavaScript: object keys in JS property order (array indices
// first, ascending; the rest in the order written; a repeated key keeps its first place and its last value).

import (
	"errors"
	"strconv"
	"unicode/utf16"
	"unicode/utf8"
)

type JKind byte

const (
	JNull JKind = iota
	JBool
	JNum
	JStr
	JArr
	JObj
)

// JV is a parsed JSON value. A nil *JV is JavaScript's undefined (a missing key).
type JV struct {
	Kind JKind
	B    bool
	N    float64
	S    string
	A    []*JV
	Keys []string // object keys in JS property order
	Vals []*JV
}

func (v *JV) Get(k string) *JV {
	if v == nil || v.Kind != JObj {
		return nil
	}
	for i, key := range v.Keys {
		if key == k {
			return v.Vals[i]
		}
	}
	return nil
}

// Truthy is JavaScript truthiness.
func (v *JV) Truthy() bool {
	if v == nil {
		return false
	}
	switch v.Kind {
	case JNull:
		return false
	case JBool:
		return v.B
	case JNum:
		return v.N != 0 && v.N == v.N
	case JStr:
		return v.S != ""
	}
	return true
}

// IsNullish is `v == null`.
func (v *JV) IsNullish() bool { return v == nil || v.Kind == JNull }

// Num is the value as a number where JS would use it as one (null -> 0, missing -> NaN).
func (v *JV) Num() float64 {
	if v == nil {
		return nan
	}
	switch v.Kind {
	case JNum:
		return v.N
	case JNull:
		return 0
	case JBool:
		if v.B {
			return 1
		}
		return 0
	case JStr:
		f, err := strconv.ParseFloat(v.S, 64)
		if err != nil {
			if v.S == "" {
				return 0
			}
			return nan
		}
		return f
	}
	return nan
}

// String is JavaScript's String(v) for the simple kinds.
func (v *JV) String() string {
	if v == nil {
		return "undefined"
	}
	switch v.Kind {
	case JNull:
		return "null"
	case JBool:
		if v.B {
			return "true"
		}
		return "false"
	case JNum:
		return jsNumber(v.N)
	case JStr:
		return v.S
	case JArr:
		s := ""
		for i, x := range v.A {
			if i > 0 {
				s += ","
			}
			if !x.IsNullish() {
				s += x.String()
			}
		}
		return s
	}
	return "[object Object]"
}

func (v *JV) write(w *jw) {
	if v == nil {
		w.raw("null")
		return
	}
	switch v.Kind {
	case JNull:
		w.raw("null")
	case JBool:
		w.boolean(v.B)
	case JNum:
		w.num(v.N)
	case JStr:
		w.str(v.S)
	case JArr:
		w.raw("[")
		for i, x := range v.A {
			if i > 0 {
				w.raw(",")
			}
			x.write(w)
		}
		w.raw("]")
	case JObj:
		w.raw("{")
		for i, k := range v.Keys {
			if i > 0 {
				w.raw(",")
			}
			w.key(k)
			v.Vals[i].write(w)
		}
		w.raw("}")
	}
}

type jparser struct {
	s   []byte
	i   int
	err error
}

func ParseJSON(b []byte) (*JV, error) {
	p := &jparser{s: b}
	p.ws()
	v := p.value()
	if p.err != nil {
		return nil, p.err
	}
	p.ws()
	if p.i != len(p.s) {
		return nil, errors.New("unexpected data after JSON value")
	}
	return v, nil
}

func (p *jparser) ws() {
	for p.i < len(p.s) {
		switch p.s[p.i] {
		case ' ', '\t', '\n', '\r':
			p.i++
		default:
			return
		}
	}
}

func (p *jparser) fail(msg string) *JV {
	if p.err == nil {
		p.err = errors.New("bad JSON: " + msg + " at " + strconv.Itoa(p.i))
	}
	return &JV{}
}

func (p *jparser) value() *JV {
	if p.err != nil {
		return &JV{}
	}
	if p.i >= len(p.s) {
		return p.fail("unexpected end")
	}
	switch c := p.s[p.i]; {
	case c == '{':
		p.i++
		o := &JV{Kind: JObj}
		var keys []string
		var vals []*JV
		pos := map[string]int{}
		p.ws()
		if p.i < len(p.s) && p.s[p.i] == '}' {
			p.i++
			return o
		}
		for {
			p.ws()
			if p.i >= len(p.s) || p.s[p.i] != '"' {
				return p.fail("expected key")
			}
			k := p.str()
			p.ws()
			if p.i >= len(p.s) || p.s[p.i] != ':' {
				return p.fail("expected colon")
			}
			p.i++
			p.ws()
			v := p.value()
			if p.err != nil {
				return o
			}
			if at, ok := pos[k]; ok {
				vals[at] = v
			} else {
				pos[k] = len(keys)
				keys = append(keys, k)
				vals = append(vals, v)
			}
			p.ws()
			if p.i < len(p.s) && p.s[p.i] == ',' {
				p.i++
				continue
			}
			if p.i < len(p.s) && p.s[p.i] == '}' {
				p.i++
				break
			}
			return p.fail("expected , or }")
		}
		order := jsKeyOrder(keys)
		o.Keys = make([]string, len(keys))
		o.Vals = make([]*JV, len(keys))
		for i, at := range order {
			o.Keys[i] = keys[at]
			o.Vals[i] = vals[at]
		}
		return o
	case c == '[':
		p.i++
		a := &JV{Kind: JArr}
		p.ws()
		if p.i < len(p.s) && p.s[p.i] == ']' {
			p.i++
			return a
		}
		for {
			p.ws()
			a.A = append(a.A, p.value())
			if p.err != nil {
				return a
			}
			p.ws()
			if p.i < len(p.s) && p.s[p.i] == ',' {
				p.i++
				continue
			}
			if p.i < len(p.s) && p.s[p.i] == ']' {
				p.i++
				return a
			}
			return p.fail("expected , or ]")
		}
	case c == '"':
		return &JV{Kind: JStr, S: p.str()}
	case c == 't' && p.lit("true"):
		return &JV{Kind: JBool, B: true}
	case c == 'f' && p.lit("false"):
		return &JV{Kind: JBool}
	case c == 'n' && p.lit("null"):
		return &JV{Kind: JNull}
	case c == '-' || (c >= '0' && c <= '9'):
		st := p.i
		if p.s[p.i] == '-' {
			p.i++
		}
		for p.i < len(p.s) {
			c := p.s[p.i]
			if (c >= '0' && c <= '9') || c == '.' || c == 'e' || c == 'E' || c == '+' || c == '-' {
				p.i++
			} else {
				break
			}
		}
		f, err := strconv.ParseFloat(string(p.s[st:p.i]), 64)
		if err != nil {
			if ne, ok := err.(*strconv.NumError); !ok || ne.Err != strconv.ErrRange {
				return p.fail("bad number")
			}
		}
		return &JV{Kind: JNum, N: f}
	}
	return p.fail("unexpected character")
}

func (p *jparser) lit(s string) bool {
	if p.i+len(s) <= len(p.s) && string(p.s[p.i:p.i+len(s)]) == s {
		p.i += len(s)
		return true
	}
	return false
}

func (p *jparser) str() string {
	p.i++ // opening quote
	st := p.i
	for p.i < len(p.s) && p.s[p.i] != '"' && p.s[p.i] != '\\' {
		p.i++
	}
	if p.i < len(p.s) && p.s[p.i] == '"' {
		s := string(p.s[st:p.i])
		p.i++
		return s
	}
	buf := append([]byte(nil), p.s[st:p.i]...)
	for p.i < len(p.s) {
		c := p.s[p.i]
		if c == '"' {
			p.i++
			return string(buf)
		}
		if c != '\\' {
			buf = append(buf, c)
			p.i++
			continue
		}
		p.i++
		if p.i >= len(p.s) {
			break
		}
		e := p.s[p.i]
		p.i++
		switch e {
		case '"', '\\', '/':
			buf = append(buf, e)
		case 'b':
			buf = append(buf, '\b')
		case 'f':
			buf = append(buf, '\f')
		case 'n':
			buf = append(buf, '\n')
		case 'r':
			buf = append(buf, '\r')
		case 't':
			buf = append(buf, '\t')
		case 'u':
			r := p.hex4()
			if utf16.IsSurrogate(r) && r < 0xDC00 && p.i+6 <= len(p.s) && p.s[p.i] == '\\' && p.s[p.i+1] == 'u' {
				save := p.i
				p.i += 2
				r2 := p.hex4()
				if r2 >= 0xDC00 && r2 <= 0xDFFF {
					r = utf16.DecodeRune(r, r2)
				} else {
					p.i = save
				}
			}
			buf = utf8.AppendRune(buf, r)
		default:
			p.fail("bad escape")
			return ""
		}
	}
	p.fail("unterminated string")
	return ""
}

func (p *jparser) hex4() rune {
	if p.i+4 > len(p.s) {
		p.fail("bad \\u escape")
		return 0
	}
	v, err := strconv.ParseUint(string(p.s[p.i:p.i+4]), 16, 32)
	if err != nil {
		p.fail("bad \\u escape")
	}
	p.i += 4
	return rune(v)
}
