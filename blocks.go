package main

import (
	"regexp"
	"strings"
)

// Blocks for Alt+Up / Alt+Down. An empty line always ends a block; past
// that, where a block starts depends on the language:
//
//	Markdown        a heading (with the words right under it), a fenced code
//	                block (``` … ``` is one block), a list, a quote or a
//	                table, each as a whole
//	HTML, XML       an element that spans lines, and whatever follows the
//	                line that closes one
//	Python, YAML    a line ending in ":" (def, class, if …), and the line
//	                after a body ends (a step back in indent); a docstring
//	                is part of its block
//	(code)          an opening line straight inside another opening line
//	                is part of that block
//	other code      a line that opens { ( [, and the line after the line
//	                that closes it: each bracket is paired with its own
//	                partner (bracketSpans), so strings, comments and
//	                brackets that open and close on one line do not count;
//	                the comment lines just above an opening line belong to
//	                its block
//	plain text      paragraphs
//
// A run of short lines (statements, list items, single-line tags) stays one
// block, so each step lands on something worth landing on.

type blockFamily int

const (
	famText blockFamily = iota
	famMarkdown
	famMarkup
	famIndent
	famCode
)

func blockFamilyOf(lang string) blockFamily {
	switch strings.ToLower(lang) {
	case "", "plaintext", "text", "plain text":
		return famText
	case "markdown", "restructuredtext", "rst", "org", "asciidoc":
		return famMarkdown
	case "html", "xml", "svg", "vue", "svelte", "xslt", "handlebars", "django/jinja", "twig":
		return famMarkup
	case "python", "python 2", "yaml", "nim", "coffeescript", "haskell", "elm", "f#", "gdscript", "toml", "ini", "makefile":
		return famIndent
	}
	return famCode
}

var (
	mdHeading = regexp.MustCompile(`^\s{0,3}#{1,6}(\s|$)|^\s{0,3}(=+|-+)\s*$`)
	mdFence   = regexp.MustCompile("^\\s*(```|~~~)")
	mdList    = regexp.MustCompile(`^\s*([-*+]|\d+[.)])\s`)
	tagOpen   = regexp.MustCompile(`<([A-Za-z][\w:.-]*)`)
	tagClose  = regexp.MustCompile(`</[A-Za-z][\w:.-]*\s*>|/>`)
)

// HTML tags that never have a closing tag.
var voidTags = map[string]bool{"area": true, "base": true, "br": true, "col": true, "embed": true, "hr": true,
	"img": true, "input": true, "link": true, "meta": true, "param": true, "source": true, "track": true, "wbr": true}

// tagOpens counts the tags on a line that need closing.
func tagOpens(t string) int {
	n := 0
	for _, m := range tagOpen.FindAllStringSubmatch(t, -1) {
		if !voidTags[strings.ToLower(m[1])] {
			n++
		}
	}
	return n
}

func indentOf(l string) int {
	n := 0
	for _, c := range l {
		switch c {
		case ' ':
			n++
		case '\t':
			n += 4
		default:
			return n
		}
	}
	return n
}

func isCommentLine(t string) bool {
	for _, p := range []string{"//", "#", "/*", "*", "--", ";", "<!--"} {
		if strings.HasPrefix(t, p) {
			return true
		}
	}
	return false
}

// mdKind sorts a Markdown line: heading, fence, list, quote, table or text.
func mdKind(l string) string {
	t := strings.TrimSpace(l)
	switch {
	case mdHeading.MatchString(l):
		return "heading"
	case mdFence.MatchString(l):
		return "fence"
	case mdList.MatchString(l):
		return "list"
	case strings.HasPrefix(t, ">"):
		return "quote"
	case strings.HasPrefix(t, "|"):
		return "table"
	}
	return "text"
}

// blockStarts marks each line that starts a block.
func blockStarts(lines [][]rune, lang string) []bool {
	fam := blockFamilyOf(lang)
	n := len(lines)
	start := make([]bool, n)
	text := make([]string, n)
	blank := make([]bool, n)
	for r := range lines {
		text[r] = string(lines[r])
		blank[r] = strings.TrimSpace(text[r]) == ""
	}
	prev := -1 // the last line that was not empty
	closed := false
	inFence := false
	prevKind := ""
	inDoc := false                // inside a Python """ string
	prevOpens := false            // the last line opened a block
	spanOpens := make([]bool, n)  // a bracket opens here and closes on a later line
	spanCloses := make([]bool, n) // a bracket closes here that opened on an earlier line
	if fam == famCode {
		for _, sp := range bracketSpans(lines, lang) {
			spanOpens[sp.top], spanCloses[sp.bottom] = true, true
		}
	}
	for r := 0; r < n; r++ {
		if blank[r] {
			if !inFence {
				prevKind = ""
			}
			continue
		}
		t := strings.TrimSpace(text[r])
		s := prev < 0 || blank[r-1] && !inFence
		switch fam {
		case famMarkdown:
			if inFence {
				if mdFence.MatchString(text[r]) {
					inFence = false
					prevKind = "fence-end"
				}
				prev = r
				continue
			}
			k := mdKind(text[r])
			if k == "text" && (prevKind == "list" || prevKind == "quote") && indentOf(text[r]) > 0 {
				k = prevKind // a list item's or quote's next line
			}
			switch {
			case k == "heading" && strings.Trim(t, "=-") == "":
				// a === or --- line under a heading: part of it
			case k == "heading" || k == "fence":
				s = true
			case k != prevKind && !(prevKind == "heading" && k == "text") || prevKind == "fence-end":
				s = true
			}
			if k == "fence" {
				inFence = true
			}
			prevKind = k
		case famMarkup:
			opens := tagOpens(t)
			closes := len(tagClose.FindAllString(t, -1))
			if closed && !strings.HasPrefix(t, "</") {
				s = true
			}
			if opens > closes && !strings.HasPrefix(t, "<!") {
				s = true
			}
			closed = strings.HasPrefix(t, "</") || opens < closes
		case famIndent:
			quotes := strings.Count(t, `"""`) + strings.Count(t, `'''`)
			if inDoc {
				inDoc = quotes%2 == 0 // a docstring's lines are one block
				prev = r
				continue
			}
			inDoc = quotes%2 == 1
			opens := strings.HasSuffix(t, ":") && !strings.HasPrefix(t, "#")
			if opens && !(prevOpens && indentOf(text[r]) > indentOf(text[prev])) {
				s = true // not a header straight inside another one
			}
			if prev >= 0 && indentOf(text[r]) < indentOf(text[prev]) {
				s = true
			}
			prevOpens = opens
		case famCode:
			closesFirst := strings.IndexAny(t, "})]") == 0
			if closed && !closesFirst {
				s = true
			}
			opens := spanOpens[r]
			if opens && !(prevOpens && indentOf(text[r]) > indentOf(text[prev])) {
				s = true // opens a block (} else { counts), unless straight inside another
			}
			prevOpens = opens
			closed = spanCloses[r] && !opens
		}
		start[r] = s
		prev = r
	}
	if fam == famCode || fam == famMarkup || fam == famIndent {
		// Comment lines just above a block's first line belong to it.
		for r := 0; r < n; r++ {
			if !start[r] || isCommentLine(strings.TrimSpace(text[r])) {
				continue
			}
			c := r
			for c > 0 && !blank[c-1] && isCommentLine(strings.TrimSpace(text[c-1])) &&
				indentOf(text[c-1]) == indentOf(text[r]) {
				c--
			}
			if c < r {
				for k := c; k < r; k++ {
					start[k] = false
				}
				start[c], start[r] = true, false
			}
		}
	}
	return start
}

// blockSpan is a block marked precisely: from its top line to its bottom line.
type blockSpan struct{ top, bottom int }

// Languages where # does not start a comment (CSS ids, C's #include,
// Rust's #[…], JavaScript's #private).
var hashNotComment = map[string]bool{"css": true, "scss": true, "sass": true, "less": true, "c": true,
	"c++": true, "c#": true, "objective-c": true, "rust": true, "go": true, "javascript": true,
	"typescript": true, "typescriptreact": true, "jsx": true, "tsx": true, "java": true,
	"kotlin": true, "swift": true, "json": true, "dart": true, "scala": true, "zig": true}

// Languages where -- starts a comment.
var dashComment = map[string]bool{"sql": true, "lua": true, "haskell": true, "elm": true, "ada": true,
	"vhdl": true, "plpgsql": true, "postgresql sql dialect": true, "mysql": true, "transact-sql": true}

// bracketSpans pairs every { ( [ with the bracket that closes it and gives
// the pairs whose two brackets are on different lines. Strings ("…", '…',
// `…`, """…""") and comments (//, /* */, and # or -- where the language
// uses them) are skipped.
func bracketSpans(lines [][]rune, lang string) []blockSpan {
	lang = strings.ToLower(lang)
	hash, dash := !hashNotComment[lang], dashComment[lang]
	type opener struct {
		c   rune
		row int
	}
	var stack []opener
	var out []blockSpan
	partner := map[rune]rune{'}': '{', ')': '(', ']': '['}
	inStr := ""         // the quote a string that goes past the line end waits for
	inBlockCmt := false // inside /* */
	for r, l := range lines {
		for i := 0; i < len(l); i++ {
			c := l[i]
			at := func(s string) bool { return strings.HasPrefix(string(l[i:min(len(l), i+len(s))]), s) }
			switch {
			case inBlockCmt:
				if at("*/") {
					inBlockCmt = false
					i++
				}
				continue
			case inStr != "":
				if c == '\\' {
					i++
				} else if at(inStr) {
					i += len(inStr) - 1
					inStr = ""
				}
				continue
			}
			lineStart := strings.TrimSpace(string(l[:i])) == ""
			afterSpace := i == 0 || l[i-1] == ' ' || l[i-1] == '\t'
			switch {
			case at("//") && lang != "python", hash && c == '#' && (lineStart || afterSpace), dash && at("--"):
				i = len(l) // the rest of the line is a comment
			case at("/*"):
				inBlockCmt = true
				i++
			case at(`"""`) || at("'''"):
				inStr = string(l[i : i+3])
				i += 2
			case c == '`':
				inStr = "`"
			case c == '"' || c == '\'':
				// a string on one line; a lone ' (Rust's 'a, English it's) is just a letter
				j := i + 1
				for j < len(l) && l[j] != c {
					if l[j] == '\\' {
						j++
					}
					j++
				}
				if j < len(l) {
					i = j
				}
			case c == '{' || c == '(' || c == '[':
				stack = append(stack, opener{c, r})
			case c == '}' || c == ')' || c == ']':
				// the nearest opener of the same kind; any unclosed ones above it are dropped
				for k := len(stack) - 1; k >= 0; k-- {
					if stack[k].c == partner[c] {
						if stack[k].row < r {
							out = append(out, blockSpan{stack[k].row, r})
						}
						stack = stack[:k]
						break
					}
				}
			}
		}
	}
	return out
}

// indentSpans gives, for Python, YAML and the like, each line ending in ":"
// with the deeper-indented lines under it.
func indentSpans(lines [][]rune) []blockSpan {
	var out []blockSpan
	for r := range lines {
		t := strings.TrimSpace(string(lines[r]))
		if !strings.HasSuffix(t, ":") || strings.HasPrefix(t, "#") {
			continue
		}
		in, bottom := indentOf(string(lines[r])), r
		for k := r + 1; k < len(lines); k++ {
			l := string(lines[k])
			if strings.TrimSpace(l) == "" {
				continue
			}
			if indentOf(l) <= in {
				break
			}
			bottom = k
		}
		if bottom > r {
			out = append(out, blockSpan{r, bottom})
		}
	}
	return out
}

// blockPairs gives the precise blocks of a file: bracket pairs in code;
// headers with their indented bodies, and bracket pairs, in Python and
// YAML; none in Markdown, plain text and markup (tags are paired by Emmet).
func blockPairs(lines [][]rune, lang string) []blockSpan {
	switch blockFamilyOf(lang) {
	case famCode:
		return bracketSpans(lines, lang)
	case famIndent:
		return append(indentSpans(lines), bracketSpans(lines, lang)...)
	}
	return nil
}

// pairedRow is where Alt+Down / Alt+Up jump from a line that opens / closes
// a block over several lines: down from the opening line to the line of the
// bracket that closes it (the outermost, when several open there), up from
// the closing line to the opening one. ok is false when the row does
// neither.
func pairedRow(spans []blockSpan, row, d int) (int, bool) {
	to, ok := row, false
	for _, s := range spans {
		if d > 0 && s.top == row && s.bottom > to {
			to, ok = s.bottom, true
		}
		if d < 0 && s.bottom == row && s.top < to {
			to, ok = s.top, true
		}
	}
	return to, ok
}

// blockAround is the block Alt+B deletes: the smallest bracket pair (or,
// in Python and YAML, header and indented body) that holds the row, from
// the line of its opening bracket to the line of its closing one. Pairs
// that share a line (} else {) go together, so the code stays whole, and
// the comment lines (and @decorators) just above come too. Outside any
// pair, or in Markdown and plain text, it is the Alt+Up / Alt+Down block
// the row is in. ok is false on an empty line outside any pair.
func blockAround(lines [][]rune, lang string, row int) (sp blockSpan, ok bool) {
	spans := blockPairs(lines, lang)
	blank := func(r int) bool { return strings.TrimSpace(string(lines[r])) == "" }
	best := -1
	for i, s := range spans {
		if s.top <= row && row <= s.bottom && (best < 0 || s.bottom-s.top < spans[best].bottom-spans[best].top) {
			best = i
		}
	}
	if best >= 0 {
		sp = spans[best]
		for grown := true; grown; {
			grown = false
			for _, s := range spans {
				if s.top == sp.bottom && s.bottom > sp.bottom {
					sp.bottom, grown = s.bottom, true
				}
				if s.bottom == sp.top && s.top < sp.top {
					sp.top, grown = s.top, true
				}
			}
		}
		in := indentOf(string(lines[sp.top]))
		for sp.top > 0 && !blank(sp.top-1) && indentOf(string(lines[sp.top-1])) == in {
			t := strings.TrimSpace(string(lines[sp.top-1]))
			if !isCommentLine(t) && !strings.HasPrefix(t, "@") {
				break
			}
			sp.top--
		}
		return sp, true
	}
	if blank(row) {
		return blockSpan{}, false
	}
	starts := blockStarts(lines, lang)
	sp = blockSpan{row, row}
	for sp.top > 0 && !starts[sp.top] {
		sp.top--
	}
	for sp.bottom+1 < len(lines) && !starts[sp.bottom+1] {
		sp.bottom++
	}
	for blank(sp.bottom) {
		sp.bottom--
	}
	// never cut a bracket pair in half
	for _, s := range spans {
		if s.top >= sp.top && s.top <= sp.bottom && s.bottom > sp.bottom {
			sp.bottom = s.bottom
		}
	}
	return sp, true
}
