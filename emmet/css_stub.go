package emmet

import "errors"

// Stub: replaced by css.go and correct.go.

func expandCSS(abbr string, opt Options) (Expansion, error) { return Expansion{}, errors.New("todo") }
func extractCSS(line []rune, col int) (string, int, bool)   { return "", 0, false }
func cssTabWorthy(abbr string) bool                         { return false }

// CorrectCSSLine fixes typing slips in the CSS declarations on one line
// ("colr: red" → "color: red", "display: flx" → "display: flex").
// Edits are rune offsets into line. Note says what was fixed.
func CorrectCSSLine(line []rune) (edits []Edit, note string) { return nil, "" }
