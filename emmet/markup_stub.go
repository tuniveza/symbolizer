package emmet

import "errors"

// Stub: replaced by markup.go.

func expandMarkup(abbr string, opt Options) (Expansion, error) {
	return Expansion{}, errors.New("todo")
}
func extractMarkup(line []rune, col int) (string, int, bool) { return "", 0, false }
func markupTabWorthy(abbr, syntax string) bool               { return false }

// Wrap puts content inside the abbreviation: "ul>li*" wraps each line in its own li.
func Wrap(abbr string, content string, opt Options) (Expansion, error) {
	return Expansion{}, errors.New("todo")
}
