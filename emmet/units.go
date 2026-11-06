package emmet

// Unit hints for CSS: the recommended unit for each property, shown as a
// dim "shadow" in the editor before or while a value is typed.

import (
	"strconv"
	"strings"
	"unicode"
)

// UnitHint says what to show and what Tab does. Tab puts New in place of
// the line's letters Start..End. The shadow text Ghost is drawn at End.
type UnitHint struct {
	Start, End int
	New        string
	Ghost      string
	Why        string // one short sentence for the message line
	AtCursor   bool   // the cursor is on the value, so Tab can take it
}

// unitRule is the advice for one kind of property.
type unitRule struct {
	unit   string // the unit to add to a plain number; "" = none needed
	sample string // the value to offer when nothing is typed yet
	why    string
}

var (
	ruleRem      = unitRule{"rem", "1rem", "rem follows the user's font size"}
	ruleFont     = unitRule{"rem", "1rem", "rem font sizes grow with the user's font setting"}
	ruleWidth    = unitRule{"rem", "100%", "% of the parent, or rem for fixed sizes"}
	ruleHeight   = unitRule{"rem", "auto", "let content set the height; rem when it must be fixed"}
	ruleMinH     = unitRule{"rem", "100dvh", "dvh fits the visible screen on phones"}
	ruleMaxW     = unitRule{"rem", "60rem", "rem keeps the limit in step with the text size"}
	ruleRadius   = unitRule{"rem", "0.5rem", "rem rounds corners in step with the text"}
	ruleLine     = unitRule{"", "1.5", "a plain number: each element multiplies its own font size"}
	ruleSpacing  = unitRule{"em", "0.02em", "em follows the element's own font size"}
	ruleIndent   = unitRule{"em", "1em", "em follows the element's own font size"}
	ruleHair     = unitRule{"px", "1px", "px for thin lines: they stay sharp"}
	ruleBorder   = unitRule{"px", "1px solid currentColor", "px for thin lines: they stay sharp"}
	ruleShadow   = unitRule{"px", "0 1px 3px rgb(0 0 0 / 20%)", "px for shadows"}
	ruleTime     = unitRule{"ms", "200ms", "ms: 150–300ms feels quick but visible"}
	ruleTrans    = unitRule{"ms", "opacity 200ms ease", "name the property and a time in ms"}
	ruleAnim     = unitRule{"ms", "fade 300ms ease both", "a name, then a time in ms"}
	ruleAngle    = unitRule{"deg", "45deg", "deg for angles"}
	ruleTrack    = unitRule{"fr", "repeat(auto-fit, minmax(16rem, 1fr))", "fr shares the free space"}
	ruleBasis    = unitRule{"rem", "16rem", "rem, or % of the flex container"}
	rulePlain    = unitRule{"", "", "this property takes a plain number, no unit"}
	ruleFontSize = ruleFont
)

var unitRules = map[string]unitRule{
	"font-size": ruleFontSize, "line-height": ruleLine,
	"letter-spacing": ruleSpacing, "word-spacing": ruleSpacing, "text-indent": ruleIndent,
	"width": ruleWidth, "min-width": ruleRem, "max-width": ruleMaxW,
	"inline-size": ruleWidth, "min-inline-size": ruleRem, "max-inline-size": ruleMaxW,
	"height": ruleHeight, "min-height": ruleMinH, "max-height": ruleRem,
	"block-size": ruleHeight, "min-block-size": ruleMinH, "max-block-size": ruleRem,
	"border-radius": ruleRadius,
	"border":        ruleBorder, "border-top": ruleBorder, "border-right": ruleBorder, "border-bottom": ruleBorder,
	"border-left": ruleBorder, "outline": ruleBorder, "border-block": ruleBorder, "border-inline": ruleBorder,
	"border-width": ruleHair, "outline-width": ruleHair, "outline-offset": {"px", "2px", "px for thin lines: they stay sharp"},
	"box-shadow": ruleShadow, "text-shadow": ruleShadow,
	"transition": ruleTrans, "transition-duration": ruleTime, "transition-delay": ruleTime,
	"animation": ruleAnim, "animation-duration": ruleTime, "animation-delay": ruleTime,
	"rotate":                ruleAngle,
	"grid-template-columns": ruleTrack, "grid-template-rows": {"fr", "auto 1fr auto", "fr shares the free space"},
	"grid-auto-columns": {"fr", "1fr", "fr shares the free space"}, "grid-auto-rows": {"rem", "minmax(10rem, auto)", "rem, or auto to fit the content"},
	"flex-basis": ruleBasis,
	"z-index":    {"", "1", "a plain whole number, no unit"}, "opacity": {"", "1", "a plain number from 0 to 1"},
	"flex-grow": rulePlain, "flex-shrink": rulePlain, "order": rulePlain,
	"font-weight":  {"", "600", "a plain number from 1 to 1000, or bold"},
	"aspect-ratio": {"", "16 / 9", "width / height, no unit"},
}

func init() {
	for _, p := range []string{"margin", "padding", "gap", "row-gap", "column-gap", "inset", "top", "right", "bottom", "left",
		"scroll-margin", "scroll-padding", "margin-block", "margin-inline", "padding-block", "padding-inline"} {
		unitRules[p] = ruleRem
	}
	for _, side := range []string{"top", "right", "bottom", "left", "block", "inline", "block-start", "block-end", "inline-start", "inline-end"} {
		unitRules["margin-"+side] = ruleRem
		unitRules["padding-"+side] = ruleRem
		unitRules["inset-"+side] = ruleRem
		unitRules["border-"+side+"-width"] = ruleHair
	}
}

// UnitHintAt looks at the CSS declaration the cursor (col) is in and gives
// the hint to show, if any.
func UnitHintAt(line []rune, col int) (UnitHint, bool) {
	// the declaration around the cursor: from the last { or ; to the next ; or }
	ds := 0
	for i := min(col, len(line)) - 1; i >= 0; i-- {
		if line[i] == '{' || line[i] == ';' {
			ds = i + 1
			break
		}
	}
	de := len(line)
	for i := ds; i < len(line); i++ {
		if i >= col && (line[i] == ';' || line[i] == '}') {
			de = i
			break
		}
	}
	colon := -1
	for i := ds; i < de; i++ {
		if line[i] == ':' {
			colon = i
			break
		}
	}
	if colon < 0 {
		return UnitHint{}, false
	}
	prop := strings.ToLower(strings.TrimSpace(string(line[ds:colon])))
	rule, ok := unitRules[prop]
	if !ok {
		return UnitHint{}, false
	}
	vs := colon + 1
	inValue := col > colon
	before, after := "", ""
	if inValue {
		before = strings.TrimSpace(string(line[vs:col]))
		after = strings.TrimSpace(string(line[col:de]))
	}

	// nothing typed yet: offer the usual value
	if inValue && before == "" && after == "" && rule.sample != "" {
		g := rule.sample
		if col == vs || line[col-1] != ' ' {
			g = " " + g
		}
		return UnitHint{Start: col, End: col, New: g, Ghost: g, Why: sentenceOf(prop + ": " + rule.why), AtCursor: true}, true
	}

	// a number just typed: the unit it needs
	ns := max(col, vs)
	for ns > vs && (unicode.IsDigit(line[ns-1]) || line[ns-1] == '.') {
		ns--
	}
	nextOK := col >= len(line) || !(unicode.IsLetter(line[col]) || unicode.IsDigit(line[col]) || line[col] == '%' || line[col] == '.')
	if inValue && ns < col && nextOK && (ns == vs || strings.ContainsRune(" :(,/", line[ns-1])) {
		if h, ok := numberHint(prop, rule, string(line[ns:col]), ns, col); ok {
			return h, true
		}
	}

	return UnitHint{}, false
}

// numberHint is the unit for a plain number typed at start..end.
func numberHint(prop string, rule unitRule, num string, start, end int) (UnitHint, bool) {
	n, err := strconv.ParseFloat(num, 64)
	if err != nil || n == 0 || rule.unit == "" {
		return UnitHint{}, false // 0 needs no unit; some properties take none
	}
	add := func(unit, why string) (UnitHint, bool) {
		return UnitHint{Start: end, End: end, New: unit, Ghost: unit, Why: sentenceOf(why), AtCursor: true}, true
	}
	whole := !strings.Contains(num, ".")
	switch {
	case (prop == "height" || prop == "min-height" || prop == "block-size" || prop == "min-block-size") && n == 100:
		return add("dvh", "100dvh is the visible screen height on phones")
	case (prop == "width" || prop == "inline-size") && n <= 100 && whole && n >= 10:
		return add("%", "% of the parent's width")
	case rule.unit == "ms" && n < 10:
		return add("s", "seconds; 0.2s is the same as 200ms")
	case rule.unit == "rem" && whole && n > 6:
		return UnitHint{}, false // a big plain number is pixels in mind: "16rem" would be wrong
	}
	return add(rule.unit, prop+": "+rule.why)
}

func sentenceOf(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	s = string(r)
	if !strings.HasSuffix(s, ".") {
		s += "."
	}
	return s
}
