// Package emmet expands Emmet abbreviations (HTML, JSX, XML and CSS),
// fixes typing slips in CSS, and does Emmet's actions on tags and numbers.
//
// All positions are rune offsets (letters, not bytes). Text is []rune.
package emmet

import "strings"

// Options say how the output should look.
type Options struct {
	Syntax     string // "html", "xhtml", "xml", "jsx", "css", "scss", "sass", "less", "stylus"
	Indent     string // one level of indent, e.g. "  " or "\t"
	BaseIndent string // the line's own leading space: put after every new line of output
}

// Expansion is what an abbreviation turns into.
type Expansion struct {
	Text  string  // the output (lines after the first start with BaseIndent)
	Stops []Range // places in Text (rune offsets) for the cursor, in order; a stop with
	// Start < End covers placeholder text the editor highlights; none = the end
	Note string // a short note for the message line, e.g. "fixed: dvi → div" ("" = none)
}

// Range is a stretch of text: Start ≤ End, End not included. Start == End is a cursor.
type Range struct{ Start, End int }

// Edit replaces Text[Start:End] of the original text with New.
type Edit struct {
	Start, End int
	New        string
}

// Change is the result of an action: edits to make, and where the cursor ends up.
type Change struct {
	Edits []Edit // do not overlap; offsets are in the original text
	Sel   Range  // the cursor (or highlight) afterwards, in offsets of the new text
	Note  string // a short note for the message line ("" = none)
}

// IsMarkup tells whether syntax is one of the tag languages.
func IsMarkup(syntax string) bool {
	switch syntax {
	case "html", "xhtml", "xml", "jsx":
		return true
	}
	return false
}

// IsStylesheet tells whether syntax is one of the CSS languages.
func IsStylesheet(syntax string) bool {
	switch syntax {
	case "css", "scss", "sass", "less", "stylus":
		return true
	}
	return false
}

// Expand turns an abbreviation into text: "ul>li.item$*3", "m10-20", "!".
func Expand(abbr string, opt Options) (Expansion, error) {
	if IsStylesheet(opt.Syntax) {
		return expandCSS(abbr, opt)
	}
	return expandMarkup(colonDefault(abbr, opt.Syntax), opt)
}

// colonDefault reads a snippet name with nothing after its colon as its
// usual form: "html:" is the HTML5 page (html:5), "link:" a link, "input:" a
// text input. Real Emmet only lists the choices there.
func colonDefault(abbr, syntax string) string {
	a := strings.TrimSpace(abbr)
	if len(a) < 2 || !strings.HasSuffix(a, ":") {
		return abbr
	}
	base := strings.TrimSuffix(a, ":")
	if base == "html" && syntax != "xml" {
		return "!"
	}
	if mkPlainWordRe.MatchString(base) && mkKnownWord(base, syntax) {
		return base
	}
	return abbr
}

// Extract finds the abbreviation that ends at col on line.
// It gives the abbreviation and the column it starts at.
func Extract(line []rune, col int, syntax string) (abbr string, start int, ok bool) {
	if IsStylesheet(syntax) {
		return extractCSS(line, col)
	}
	return extractMarkup(line, col)
}

// TabExpand is what Tab does: expand the abbreviation that ends at col, but
// only when it is clearly meant as one (so plain words still get a Tab).
func TabExpand(line []rune, col int, opt Options) (start int, exp Expansion, ok bool) {
	abbr, start, ok := Extract(line, col, opt.Syntax)
	if !ok {
		return 0, Expansion{}, false
	}
	if IsStylesheet(opt.Syntax) {
		if !cssTabWorthy(abbr) {
			return 0, Expansion{}, false
		}
	} else if !markupTabWorthy(colonDefault(abbr, opt.Syntax), opt.Syntax) {
		return 0, Expansion{}, false
	}
	exp, err := Expand(abbr, opt)
	if err != nil {
		return 0, Expansion{}, false
	}
	return start, exp, true
}
