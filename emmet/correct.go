package emmet

// CSS autocorrect: fixes typing slips in the declarations on one line when
// the user types ";", ":" or Enter.

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// CorrectCSSLine fixes typing slips in the CSS declarations on one line
// ("colr: red" → "color: red", "display: flx" → "display: flex",
// "10xp" → "10px", ";;" → ";").
// Edits are rune offsets into line and do not overlap. Note says what was
// fixed, e.g. "fixed: colr → color" ("" when nothing was).
//
// Comments, strings, url(...) and other functions, selectors, @-rule lines,
// custom properties (--x), SCSS $vars and LESS @vars are never changed.
func CorrectCSSLine(line []rune) (edits []Edit, note string) {
	first := -1
	for i, c := range line {
		if !unicode.IsSpace(c) {
			first = i
			break
		}
	}
	if first < 0 || line[first] == '@' {
		return nil, ""
	}
	mask := cssMaskLine(line)
	depth := corDepths(line, mask)
	add := func(start, end int, newText string) {
		edits = append(edits, Edit{Start: start, End: end, New: newText})
	}

	// statements end at ";", "{" or "}"; one ended by "{" is a selector
	start := 0
	for i := 0; i <= len(line); i++ {
		if i < len(line) && (mask[i] != cssMaskCode || depth[i] != 0 || !strings.ContainsRune(";{}", line[i])) {
			continue
		}
		if i == len(line) || line[i] != '{' {
			corDeclaration(line, mask, depth, start, i, add)
		}
		start = i + 1
	}

	// ";;" → ";"
	for i := 0; i < len(line); i++ {
		if line[i] != ';' || mask[i] != cssMaskCode || depth[i] != 0 {
			continue
		}
		j := i + 1
		for j < len(line) && line[j] == ';' && mask[j] == cssMaskCode {
			j++
		}
		if j-i >= 2 {
			add(i, j, ";")
		}
		i = j - 1
	}

	if len(edits) == 0 {
		return nil, ""
	}
	sort.SliceStable(edits, func(i, j int) bool { return edits[i].Start < edits[j].Start })
	var order []string
	for _, e := range edits {
		order = append(order, string(line[e.Start:e.End])+" → "+e.New)
	}
	return edits, "fixed: " + strings.Join(order, ", ")
}

// corDepths gives the bracket depth at each letter (code only).
func corDepths(line []rune, mask []int) []int {
	d := make([]int, len(line))
	depth := 0
	for i, c := range line {
		if mask[i] == cssMaskCode && c == ')' && depth > 0 {
			depth--
		}
		d[i] = depth
		if mask[i] == cssMaskCode && c == '(' {
			depth++
		}
	}
	return d
}

// corDeclaration checks one "name: value" statement in line[s:e].
func corDeclaration(line []rune, mask, depth []int, s, e int, add func(int, int, string)) {
	colon := -1
	for i := s; i < e; i++ {
		if line[i] == ':' && mask[i] == cssMaskCode && depth[i] == 0 {
			colon = i
			break
		}
	}
	if colon < 0 {
		return
	}
	ns, ne := s, colon
	for ns < ne && unicode.IsSpace(line[ns]) {
		ns++
	}
	for ne > ns && unicode.IsSpace(line[ne-1]) {
		ne--
	}
	if ns == ne {
		return
	}
	for i := ns; i < ne; i++ {
		if mask[i] != cssMaskCode {
			return
		}
	}
	name := string(line[ns:ne])
	if !cssIsWord(name) || strings.HasPrefix(name, "--") {
		return // $var, @var, .class, &:hover, #id, --custom
	}
	if colon+1 < e && line[colon+1] == ':' {
		return // a::before
	}
	lower := strings.ToLower(name)
	prop := lower
	if strings.HasPrefix(lower, "-") {
		// a vendor property: keep the name, check values as the plain one
		if i := strings.IndexByte(lower[1:], '-'); i >= 0 {
			prop = lower[i+2:]
		}
	} else if !cssPropSet[lower] {
		if cssHTMLTags[lower] || corStartsWithPseudo(line, colon+1, e) {
			return // a selector: li:hover
		}
		fixed := corFixName(lower)
		if fixed == "" {
			return
		}
		add(ns, ne, fixed)
		prop = fixed
	}
	corValues(line, mask, depth, colon+1, e, prop, add)
}

// corStartsWithPseudo tells whether the text after the colon is a
// pseudo-class or pseudo-element name ("hover", "nth-child(").
func corStartsWithPseudo(line []rune, s, e int) bool {
	i := s
	for i < e && cssIsKeyword(line[i]) {
		i++
	}
	if i == s {
		return false
	}
	return cssPseudoNames[strings.ToLower(string(line[s:i]))]
}

var corBritish = []struct{ from, to string }{
	{"colour", "color"}, {"centre", "center"}, {"behaviour", "behavior"}, {"grey", "gray"},
}

// corKnownSlips are slips the distance rule alone would miss or find unclear.
var corKnownSlips = map[string]string{
	"backgroud": "background", "dispaly": "display", "widht": "width", "heigth": "height",
	"maring": "margin", "pading": "padding", "boder": "border", "postion": "position",
	"opactiy": "opacity", "visiblity": "visibility", "colr": "color", "fotn-size": "font-size",
	"text-algin": "text-align", "transtion": "transition", "tranform": "transform",
	"z-indx": "z-index", "zindex": "z-index",
}

// corFixName gives the property a slip in name was meant to be, or "".
func corFixName(name string) string {
	if len([]rune(name)) < 3 {
		return ""
	}
	if p, ok := corKnownSlips[name]; ok {
		return p
	}
	for _, b := range corBritish {
		if b.from == "grey" {
			continue // grey is only a colour
		}
		if p := strings.ReplaceAll(name, b.from, b.to); p != name && cssPropSet[p] {
			return p
		}
	}
	return cssClosestWord(name, cssPropNames, cssPropSet)
}

// cssClosestWord gives the one word in list that w is a slip of: at most 1
// edit (letters added, dropped, changed or swapped) for words up to 5
// letters, 2 for longer ones (then the first letter must match). "" when
// w is valid, too short, or two words are equally close.
func cssClosestWord(w string, list []string, valid map[string]bool) string {
	n := len([]rune(w))
	if n < 3 || valid[w] {
		return ""
	}
	max := 1
	if n > 5 {
		max = 2
	}
	best, bestD, count := "", max+1, 0
	for _, c := range list {
		cn := len([]rune(c))
		if cn < 3 || cn-n > max || n-cn > max {
			continue
		}
		d := corDistance(w, c)
		if d == 2 && []rune(w)[0] != []rune(c)[0] {
			continue
		}
		if d < bestD {
			best, bestD, count = c, d, 1
		} else if d == bestD && c != best {
			count++
		}
	}
	if bestD <= max && count == 1 {
		return best
	}
	return ""
}

// corDistance is the Damerau-Levenshtein distance (optimal string
// alignment): letters added, dropped, changed, or two next to each other swapped.
func corDistance(a, b string) int {
	x, y := []rune(a), []rune(b)
	d := make([][]int, len(x)+1)
	for i := range d {
		d[i] = make([]int, len(y)+1)
		d[i][0] = i
	}
	for j := 0; j <= len(y); j++ {
		d[0][j] = j
	}
	for i := 1; i <= len(x); i++ {
		for j := 1; j <= len(y); j++ {
			cost := 1
			if x[i-1] == y[j-1] {
				cost = 0
			}
			v := d[i-1][j] + 1
			if w := d[i][j-1] + 1; w < v {
				v = w
			}
			if w := d[i-1][j-1] + cost; w < v {
				v = w
			}
			if i > 1 && j > 1 && x[i-1] == y[j-2] && x[i-2] == y[j-1] {
				if w := d[i-2][j-2] + 1; w < v {
					v = w
				}
			}
			d[i][j] = v
		}
	}
	return d[len(x)][len(y)]
}

var corReNumUnit = regexp.MustCompile(`^[+-]?(?:\d+\.?\d*|\.\d+)([a-zA-Z]+)$`)

// corValues checks the words of a value in line[s:e] for property prop.
func corValues(line []rune, mask, depth []int, s, e int, prop string, add func(int, int, string)) {
	set := cssKeywords[prop]
	colourTarget := cssColourTargets[prop]
	i := s
	for i < e {
		c := line[i]
		if unicode.IsSpace(c) || c == ',' || c == '/' || mask[i] != cssMaskCode {
			i++
			continue
		}
		// a word: up to a space, comma or slash at depth 0
		ws := i
		clean := true
		for i < e {
			c := line[i]
			if depth[i] == 0 && mask[i] == cssMaskCode && (unicode.IsSpace(c) || c == ',' || c == '/') {
				break
			}
			if mask[i] != cssMaskCode || c == '(' || c == ')' || depth[i] > 0 {
				clean = false // strings, comments, functions: url(), var(), calc()
			}
			i++
		}
		if !clean {
			continue
		}
		word := string(line[ws:i])
		lower := strings.ToLower(word)
		switch {
		case strings.HasPrefix(word, "!"):
			if lower != "!important" && len(lower) >= 4 && corDistance(lower, "!important") <= 2 {
				add(ws, i, "!important")
			}
		case corReNumUnit.MatchString(word):
			m := corReNumUnit.FindStringSubmatchIndex(word)
			unit := strings.ToLower(word[m[2]:m[3]])
			if cssUnitsAll[unit] {
				continue
			}
			if fix := corClosestUnit(unit); fix != "" {
				add(ws, i, word[:m[2]]+fix) // the whole "10xp", so the note reads 10xp → 10px
			}
		case cssIsWord(word) && !strings.HasPrefix(word, "-"):
			if set == nil && !colourTarget {
				continue
			}
			if cssGlobalKeywords[lower] || cssColourNames[lower] || lower == "auto" || lower == "none" {
				continue
			}
			valid := map[string]bool{}
			list := append([]string{}, set...)
			for _, k := range set {
				valid[k] = true
			}
			if valid[lower] {
				continue
			}
			if colourTarget {
				for c := range cssColourNames {
					list = append(list, c)
				}
				sort.Strings(list)
			}
			fix := ""
			for _, b := range corBritish {
				if p := strings.ReplaceAll(lower, b.from, b.to); p != lower && valid[p] {
					fix = p
					break
				}
			}
			if fix == "" {
				fix = cssClosestWord(lower, list, valid)
			}
			if fix != "" {
				add(ws, i, fix)
			}
		}
	}
}

// corClosestUnit gives the common unit a slip like "xp" or "pz" was meant to
// be, or "" when it is not clear.
func corClosestUnit(unit string) string {
	max := 1
	if len(unit) >= 4 {
		max = 2
	}
	best, bestD, count := "", max+1, 0
	for _, u := range cssUnitsCommon {
		if u == "%" {
			continue
		}
		d := corDistance(unit, u)
		if d < bestD {
			best, bestD, count = u, d, 1
		} else if d == bestD {
			count++
		}
	}
	if bestD <= max && count == 1 {
		return best
	}
	return ""
}
