package main

import (
	"sort"
	"strings"
	"unicode"
)

// Where Left / Right (and Shift+Left / Right, Alt+Backspace) stop.
// A line is read the way a programmer reads it:
//
//	parts of names      MAX _ PARTS   max Parts     one step each
//	                    parse HTTP Request   __ init __
//	words               parts  don't               one step
//	numbers             3 . 14   0x FF   1 e-5   10 px   one step each part
//	big numbers         1 234 567   0x DEAD BEEF   1_000 _000   a group each
//	brackets, quotes    ( ) [ ] { } " ' `          one step each
//	known operators     += -> :: == && << // /*    one step
//	anything else       ; , . * < >                one step each
//	spaces              skipped
//
// So "char *parts[MAX_PARTS];" stops after: char * parts [ MAX _ PARTS ] ;

// operators that read as one symbol, longest first.
var operators = func() []string {
	ops := []string{
		">>>=", "<<=", ">>=", "...", "===", "!==", "**=", "//=", "<=>", "&&=", "||=", "??=",
		"->", "=>", "::", "++", "--", "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=",
		"==", "!=", "<=", ">=", "&&", "||", "<<", ">>", ":=", "..", "??", "?.", "**",
		"//", "/*", "*/", "<-", "|>", "#!", "<!--", "-->", "</", "/>",
	}
	sort.SliceStable(ops, func(i, j int) bool { return len(ops[i]) > len(ops[j]) })
	return ops
}()

func isBracket(r rune) bool {
	switch r {
	case '(', ')', '[', ']', '{', '}', '"', '\'', '`':
		return true
	}
	return false
}

// tokenEnd is where the step that begins at l[i] ends (l[i] is not a space).
func tokenEnd(l []rune, i int) int {
	n := len(l)
	r := l[i]
	switch {
	case unicode.IsDigit(r):
		j := i + 1
		for j < n {
			c := l[j]
			switch {
			case isWord(c):
				j++
			case c == '.' && j+1 < n && unicode.IsDigit(l[j+1]):
				j++
			case (c == '+' || c == '-') && (l[j-1] == 'e' || l[j-1] == 'E') && j+1 < n && unicode.IsDigit(l[j+1]):
				j++
			default:
				return j
			}
		}
		return j
	case r == '_':
		j := i + 1
		for j < n && l[j] == '_' {
			j++
		}
		return j
	case isWord(r):
		return namePartEnd(l, i)
	case isBracket(r):
		return i + 1
	}
	for _, op := range operators {
		if matchAt(l, i, op) {
			return i + len([]rune(op))
		}
	}
	return i + 1
}

// namePartEnd is the end of one part of a name: MAX in MAX_PARTS, max and
// Parts in maxParts, HTTP and Request in HTTPRequest. An apostrophe between
// letters stays inside (don't).
func namePartEnd(l []rune, i int) int {
	n := len(l)
	j := i
	if unicode.IsUpper(l[j]) {
		for j < n && unicode.IsUpper(l[j]) {
			j++
		}
		if j-i > 1 && j < n && unicode.IsLower(l[j]) {
			return j - 1 // HTTPRequest: HTTP, then Request
		}
	}
	for j < n {
		c := l[j]
		if c == '\'' && j > i && unicode.IsLetter(l[j-1]) && j+1 < n && unicode.IsLetter(l[j+1]) {
			j++
			continue
		}
		if c == '_' || !isWord(c) || unicode.IsUpper(c) {
			break
		}
		j++
	}
	return j
}

func matchAt(l []rune, i int, s string) bool {
	for _, c := range s {
		if i >= len(l) || l[i] != c {
			return false
		}
		i++
	}
	return true
}

// tokens lists the [start, end) of every token on a line, in order; a
// number is one token.
func tokens(l []rune) [][2]int {
	var out [][2]int
	for i := 0; i < len(l); {
		if unicode.IsSpace(l[i]) {
			i++
			continue
		}
		e := tokenEnd(l, i)
		out = append(out, [2]int{i, e})
		i = e
	}
	return out
}

// steps lists the [start, end) of every step on a line, in order: the
// tokens, with each number split into its parts.
func steps(l []rune) [][2]int {
	var out [][2]int
	for _, t := range tokens(l) {
		if unicode.IsDigit(l[t[0]]) {
			out = append(out, numberParts(l, t[0], t[1])...)
		} else {
			out = append(out, t)
		}
	}
	return out
}

// numberParts splits the number l[i:e] into steps: a 0x / 0b / 0o prefix,
// the digits in groups (3 from the right, 4 for hex and binary, or at each
// _ when the number has them), the point, the fraction (groups of 3 from
// the left), the exponent (e-5) and a unit (px, ms, f).
func numberParts(l []rune, i, e int) [][2]int {
	var out [][2]int
	j := i
	group := 3
	if e-i > 2 && l[i] == '0' && strings.ContainsRune("xXbBoO", l[i+1]) {
		out = append(out, [2]int{i, i + 2})
		j = i + 2
		if l[i+1] != 'o' && l[i+1] != 'O' {
			group = 4
		}
	}
	hex := j > i && (l[i+1] == 'x' || l[i+1] == 'X')
	digit := func(c rune) bool {
		return unicode.IsDigit(c) || c == '_' || hex && strings.ContainsRune("abcdefABCDEF", c)
	}
	k := j
	for k < e && digit(l[k]) {
		k++
	}
	out = append(out, digitGroups(l, j, k, group, true)...)
	if k+1 < e && l[k] == '.' && unicode.IsDigit(l[k+1]) {
		out = append(out, [2]int{k, k + 1})
		j, k = k+1, k+1
		for k < e && (unicode.IsDigit(l[k]) || l[k] == '_') {
			k++
		}
		out = append(out, digitGroups(l, j, k, 3, false)...)
	}
	if k < e && (l[k] == 'e' || l[k] == 'E') && !hex {
		x := k + 1
		if x < e && (l[x] == '+' || l[x] == '-') {
			x++
		}
		if x < e && unicode.IsDigit(l[x]) {
			for x < e && unicode.IsDigit(l[x]) {
				x++
			}
			out = append(out, [2]int{k, x})
			k = x
		}
	}
	if k < e {
		out = append(out, [2]int{k, e})
	}
	return out
}

// digitGroups splits the digits l[i:e] at each _ (the _ starts the next
// group) or, without any, into groups of n counted from the right (fromRight)
// or the left.
func digitGroups(l []rune, i, e, n int, fromRight bool) [][2]int {
	if i >= e {
		return nil
	}
	var out [][2]int
	if strings.ContainsRune(string(l[i:e]), '_') {
		s := i
		for k := i + 1; k < e; k++ {
			if l[k] == '_' && l[k-1] != '_' {
				out = append(out, [2]int{s, k})
				s = k
			}
		}
		return append(out, [2]int{s, e})
	}
	s := i
	if fromRight && (e-i)%n != 0 {
		out = append(out, [2]int{i, i + (e-i)%n})
		s = i + (e-i)%n
	}
	for ; s < e; s += n {
		out = append(out, [2]int{s, min(s+n, e)})
	}
	return out
}

// NumberLeft / NumberRight are Ctrl+Left / Right: past the whole number
// next to the cursor, else one character.
func (b *Buffer) NumberLeft(p Pos) Pos {
	l := b.Lines[p.Row]
	for _, t := range tokens(l) {
		if unicode.IsDigit(l[t[0]]) && t[0] < p.Col && p.Col <= t[1] {
			return Pos{p.Row, t[0]}
		}
	}
	return b.LetterLeft(p)
}

func (b *Buffer) NumberRight(p Pos) Pos {
	l := b.Lines[p.Row]
	for _, t := range tokens(l) {
		if unicode.IsDigit(l[t[0]]) && t[0] <= p.Col && p.Col < t[1] {
			return Pos{p.Row, t[1]}
		}
	}
	return b.LetterRight(p)
}

// WordRight goes to the end of the next step (or of the one the cursor is in).
func (b *Buffer) WordRight(p Pos) Pos {
	l := b.Lines[p.Row]
	for _, s := range steps(l) {
		if s[1] > p.Col {
			return Pos{p.Row, s[1]}
		}
	}
	if p.Col < len(l) {
		return Pos{p.Row, len(l)} // only spaces left on the line
	}
	if p.Row < len(b.Lines)-1 {
		return Pos{p.Row + 1, 0}
	}
	return p
}

// WordLeft goes to the start of the step before the cursor.
func (b *Buffer) WordLeft(p Pos) Pos {
	if p.Col == 0 {
		if p.Row > 0 {
			return Pos{p.Row - 1, len(b.Lines[p.Row-1])}
		}
		return p
	}
	st := steps(b.Lines[p.Row])
	for i := len(st) - 1; i >= 0; i-- {
		if st[i][0] < p.Col {
			return Pos{p.Row, st[i][0]}
		}
	}
	return Pos{p.Row, 0} // only spaces before the cursor
}
