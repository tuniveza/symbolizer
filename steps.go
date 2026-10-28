package main

import (
	"sort"
	"unicode"
)

// Where Left / Right (and Shift+Left / Right, Alt+Backspace) stop.
// A line is read the way a programmer reads it:
//
//	parts of names      MAX _ PARTS   max Parts     one step each
//	                    parse HTTP Request   __ init __
//	words               parts  don't               one step
//	numbers             3.14  0xFF  1e-5  10px     one step
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

// steps lists the [start, end) of every step on a line, in order.
func steps(l []rune) [][2]int {
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
