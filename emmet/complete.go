package emmet

// Word lists for the editor's suggestion list: CSS property names, the values
// a property takes, pseudo-classes and HTML tag names, filtered by what is typed.

import (
	"sort"
	"strings"
)

// IsCSSProperty tells whether name is a known CSS property.
func IsCSSProperty(name string) bool { return cssPropSet[strings.ToLower(name)] }

// LikelyCSSProperty tells whether name is a CSS property, or a slip of one
// that auto-correct will fix. Names under 2 letters (Emmet's "d:f") are not.
func LikelyCSSProperty(name string) bool {
	name = strings.ToLower(name)
	if len([]rune(name)) < 2 {
		return false
	}
	return cssPropSet[name] || corFixName(name) != ""
}

// CSSProperties gives the property names that start with prefix, the most
// common first.
func CSSProperties(prefix string) []string {
	return byPrefix(cssPropNames, strings.ToLower(prefix), cssCommonProps)
}

// CSSValues gives the keyword values of prop that start with prefix (with
// colour names for colour properties, and inherit, initial … last).
func CSSValues(prop, prefix string) []string {
	prop = strings.ToLower(prop)
	var list []string
	list = append(list, cssKeywords[prop]...)
	if cssColourProps[prop] {
		list = append(list, cssKwColour...)
		list = append(list, sortedSet(cssColourNames)...)
	}
	if len(list) == 0 && !cssPropSet[prop] {
		return nil
	}
	list = append(list, "inherit", "initial", "unset", "revert")
	seen := map[string]bool{}
	var out []string
	for _, v := range list {
		if !seen[v] && strings.HasPrefix(v, strings.ToLower(prefix)) {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

// CSSPseudo gives the pseudo-classes and pseudo-elements that start with prefix.
func CSSPseudo(prefix string) []string {
	return byPrefix(sortedSet(cssPseudoNames), strings.ToLower(prefix), nil)
}

// HTMLTags gives the HTML tag names that start with prefix, the common ones first.
func HTMLTags(prefix string) []string {
	return byPrefix(sortedSet(mkTags), strings.ToLower(prefix), mkCommonTags)
}

// cssCommonProps come first in the list when they fit.
var cssCommonProps = cssMakeSet([]string{
	"display", "color", "background", "background-color", "margin", "padding", "width",
	"height", "border", "border-radius", "font-size", "font-family", "font-weight",
	"position", "top", "left", "right", "bottom", "flex", "flex-direction", "gap",
	"justify-content", "align-items", "grid-template-columns", "text-align", "line-height",
	"overflow", "opacity", "transition", "transform", "z-index", "box-shadow", "cursor",
	"max-width", "min-height", "content", "text-decoration", "box-sizing", "outline",
})

func byPrefix(list []string, prefix string, common map[string]bool) []string {
	var first, rest []string
	for _, w := range list {
		if !strings.HasPrefix(w, prefix) {
			continue
		}
		if common[w] {
			first = append(first, w)
		} else {
			rest = append(rest, w)
		}
	}
	short := func(s []string) {
		sort.SliceStable(s, func(i, j int) bool { return len(s[i]) < len(s[j]) })
	}
	short(first)
	short(rest)
	return append(first, rest...)
}

func sortedSet(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for w := range set {
		out = append(out, w)
	}
	sort.Strings(out)
	return out
}
