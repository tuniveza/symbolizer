package main

// ---- a notepad on the Design board (notes.go) ------------------------------------------------
//
// A notepad (kNotepad) shows notepad N's notes (From "notepad:N"): up to 16
// saved .txt files, oldest first.

const kNotepad = "notepad"

const notepadW, notepadH = 26, 16

func init() {
	kindIcons[kNotepad] = "▤"
	kindNames[kNotepad] = "notepad"
}

// ---- each notepad's own look -----------------------------------------------------------------
//
// notepadStyle(n) is notepad n's look. Each part steps at its own rate, so
// no two of the first 4×6×5×7 = 840 notepads look the same. Shared by the
// notepad itself, its viewer and the desk's racks (design_desk.go).

// Bindings: how the pages are held at the top.
const (
	bindSpiral = iota // a row of wire rings
	bindGlued         // a glued band
	bindStitched      // stitches across
	bindClip          // a clipboard clip
	bindCount
)

// Rulings: what the pages are printed with.
const (
	ruleLined = iota
	ruleGrid
	ruleDots
	ruleBlank
	ruleMargin // lined with a red margin
	ruleCount
)

type padStyle struct {
	Cover   string // the cover's colour
	Paper   string // the pages' colour
	Ink     string // writing on the pages
	Rule    string // the ruling's colour
	Binding int    // bindSpiral …
	Ruling  int    // ruleLined …
	Corner  int    // 0 square, 1 rounded, 2 dog-eared, 3 cut
	Pattern int    // the cover's pattern: 0 plain, 1 stripes, 2 dots, 3 checks, 4 waves, 5 grain, 6 diamonds
}

// notepadCovers: warm theme-friendly cover colours, each different enough to tell apart.
var notepadCovers = []string{"#c0392b", "#d35400", "#b9770e", "#7d6608", "#6e2c00", "#a04000"}

// notepadPapers: page colours from cream to dark.
var notepadPapers = []string{"#f3e9d2", "#efe4c4", "#e8dcc8", "#f5efe0", "#2b1d16"}

func notepadStyle(n int) padStyle {
	if n < 1 {
		n = 1
	}
	i := n - 1
	s := padStyle{
		Binding: i % bindCount,
		Cover:   notepadCovers[(i/bindCount)%len(notepadCovers)],
		Ruling:  (i / (bindCount * len(notepadCovers))) % ruleCount,
		Pattern: (i / (bindCount * len(notepadCovers) * ruleCount)) % 7,
		Corner:  (i * 3) % 4,
	}
	s.Paper = notepadPapers[(i*2+i/7)%len(notepadPapers)]
	if lum(s.Paper) > 0.5 {
		s.Ink, s.Rule = "#2b1608", mixHex(s.Paper, "#5a7fa8", 0.35)
	} else {
		s.Ink, s.Rule = "#f3e9d2", mixHex(s.Paper, cEmber, 0.35)
	}
	return s
}
