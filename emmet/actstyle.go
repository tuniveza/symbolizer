package emmet

// A small, forgiving CSS / SCSS / LESS block scanner for the actions
// (select item, toggle comment, update image size).

type actCSSDecl struct {
	start, end         int // property start to after ';' (or value end when there is none)
	nameStart, nameEnd int
	valStart, valEnd   int // trimmed value
	semi               int // index of ';' or -1
}

type actCSSRule struct {
	preStart, preEnd int // selector / at-rule prelude, trimmed
	open, close      int // the '{' and the '}' (close = len(text) when missing)
	decls            []actCSSDecl
	parent           *actCSSRule
	depth            int
}

func (r *actCSSRule) end() int {
	return r.close + 1
}

type actCSSSheet struct {
	rules    []*actCSSRule // in the order their '{' appears
	comments []Range       // /* ... */
}

// actParseCSS scans braces, declarations and comments. It skips strings,
// url(...) and (for scss/less) // line comments.
func actParseCSS(text []rune) *actCSSSheet {
	sh := &actCSSSheet{}
	n := len(text)
	var stack []*actCSSRule
	stmt := -1 // start of the current statement
	paren := 0
	finishDecl := func(end, semi int) {
		if stmt < 0 || len(stack) == 0 {
			stmt = -1
			return
		}
		s, e := stmt, end
		for e > s && actIsSpace(text[e-1]) {
			e--
		}
		colon := -1
		p := 0
		for i := s; i < e; i++ {
			switch text[i] {
			case '(':
				p++
			case ')':
				p--
			case ':':
				if p == 0 && colon < 0 {
					colon = i
				}
			}
		}
		stmt = -1
		if colon < 0 || colon == s {
			return
		}
		ne := colon
		for ne > s && actIsSpace(text[ne-1]) {
			ne--
		}
		vs := colon + 1
		for vs < e && actIsSpace(text[vs]) {
			vs++
		}
		d := actCSSDecl{start: s, end: e, nameStart: s, nameEnd: ne, valStart: vs, valEnd: e, semi: semi}
		if semi >= 0 {
			d.end = semi + 1
		}
		top := stack[len(stack)-1]
		top.decls = append(top.decls, d)
	}
	for i := 0; i < n; i++ {
		r := text[i]
		switch {
		case r == '/' && i+1 < n && text[i+1] == '*':
			j := actIndex(text, i+2, "*/")
			e := n
			if j >= 0 {
				e = j + 2
			}
			sh.comments = append(sh.comments, Range{i, e})
			i = e - 1
			continue
		case r == '/' && i+1 < n && text[i+1] == '/' && paren == 0 && (i == 0 || text[i-1] != ':'):
			for i < n && text[i] != '\n' {
				i++
			}
			continue
		case actIsSpace(r):
			continue
		}
		if stmt < 0 {
			stmt = i
		}
		switch r {
		case '"', '\'':
			j := i + 1
			for j < n && text[j] != r && text[j] != '\n' {
				if text[j] == '\\' {
					j++
				}
				j++
			}
			i = j
		case '(':
			paren++
		case ')':
			if paren > 0 {
				paren--
			}
		case '{':
			if paren > 0 {
				continue
			}
			if i > 0 && text[i-1] == '#' { // #{interpolation}
				i = actSkipBraces(text, i) - 1
				continue
			}
			ps, pe := stmt, i
			for pe > ps && actIsSpace(text[pe-1]) {
				pe--
			}
			rule := &actCSSRule{preStart: ps, preEnd: pe, open: i, close: n, depth: len(stack)}
			if len(stack) > 0 {
				rule.parent = stack[len(stack)-1]
			}
			sh.rules = append(sh.rules, rule)
			stack = append(stack, rule)
			stmt = -1
		case ';':
			if paren > 0 {
				continue
			}
			finishDecl(i, i)
		case '}':
			if paren > 0 {
				paren = 0
			}
			finishDecl(i, -1)
			if len(stack) > 0 {
				stack[len(stack)-1].close = i
				stack = stack[:len(stack)-1]
			}
			stmt = -1
		}
	}
	finishDecl(n, -1)
	return sh
}

// ruleAt gives the innermost rule whose braces (or selector) hold pos.
func (sh *actCSSSheet) ruleAt(pos int) *actCSSRule {
	var best *actCSSRule
	for _, r := range sh.rules {
		if r.preStart <= pos && pos <= r.close && (best == nil || r.depth >= best.depth) {
			best = r
		}
	}
	return best
}

func (sh *actCSSSheet) commentAt(sel Range) (Range, bool) {
	for _, c := range sh.comments {
		if c.Start <= sel.Start && sel.End <= c.End {
			return c, true
		}
	}
	return Range{}, false
}

// actValueParts splits a CSS value at top-level spaces and commas.
func actValueParts(text []rune, s, e int) []Range {
	var parts []Range
	start := -1
	depth := 0
	var quote rune
	for i := s; i < e; i++ {
		r := text[i]
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			continue
		}
		switch {
		case r == '"' || r == '\'':
			quote = r
			if start < 0 {
				start = i
			}
		case r == '(':
			depth++
			if start < 0 {
				start = i
			}
		case r == ')':
			if depth > 0 {
				depth--
			}
		case depth == 0 && (actIsSpace(r) || r == ','):
			if start >= 0 {
				parts = append(parts, Range{start, i})
				start = -1
			}
		default:
			if start < 0 {
				start = i
			}
		}
	}
	if start >= 0 {
		parts = append(parts, Range{start, e})
	}
	return parts
}
