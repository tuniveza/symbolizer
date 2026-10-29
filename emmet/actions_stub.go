package emmet

// Stub: replaced by actions.go.

func BalanceOut(text []rune, sel Range) (Range, bool)                         { return sel, false }
func BalanceIn(text []rune, sel Range) (Range, bool)                          { return sel, false }
func MatchingPair(text []rune, pos int) (int, bool)                           { return pos, false }
func RemoveTag(text []rune, pos int, opt Options) (Change, bool)              { return Change{}, false }
func SplitJoinTag(text []rune, pos int, opt Options) (Change, bool)           { return Change{}, false }
func UpdateTag(text []rune, pos int, abbr string, opt Options) (Change, bool) { return Change{}, false }
func NextEditPoint(text []rune, pos, dir int) (int, bool)                     { return pos, false }
func SelectItem(text []rune, sel Range, dir int, syntax string) (Range, bool) { return sel, false }
func EvalMath(text []rune, sel Range) (Change, bool)                          { return Change{}, false }
func IncrementNumber(text []rune, sel Range, delta float64) (Change, bool)    { return Change{}, false }
func UpdateImageSize(text []rune, pos int, opt Options, baseDir string) (Change, error) {
	return Change{}, nil
}
func ToggleBlockComment(text []rune, sel Range, syntax string) (Change, bool) { return Change{}, false }
func AutoCloseTag(text []rune, pos int, syntax string) (string, bool)         { return "", false }

// TagNamePair finds the tag name at pos (in an opening or closing tag, or just after it)
// and gives the name ranges of both tags of the pair.
func TagNamePair(text []rune, pos int) (open, close Range, ok bool) { return Range{}, Range{}, false }
