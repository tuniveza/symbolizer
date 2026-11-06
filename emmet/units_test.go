package emmet

import "testing"

func TestUnitHintAt(t *testing.T) {
	for _, c := range []struct {
		line       string
		col        int
		ghost, new string
	}{
		{"  font-size: 1", 14, "rem", "rem"},
		{"  height: 100", 13, "dvh", "dvh"},
		{"  width: 50", 11, "%", "%"},
		{"  transition: opacity 200", 25, "ms", "ms"},
		{"  border: 1", 11, "px", "px"},
		{"  font-size: ", 13, "1rem", "1rem"},
		{"  font-size:", 12, " 1rem", " 1rem"},
		{"  line-height: ", 15, "1.5", "1.5"},
	} {
		h, ok := UnitHintAt([]rune(c.line), c.col)
		if !ok || h.Ghost != c.ghost || h.New != c.new {
			t.Fatalf("%q at %d: ghost %q new %q (ok %v), want %q / %q", c.line, c.col, h.Ghost, h.New, ok, c.ghost, c.new)
		}
	}
	for _, c := range []struct {
		line string
		col  int
	}{
		{"  margin: 0", 11},         // 0 needs no unit
		{"  z-index: 10", 13},       // takes no unit
		{"  line-height: 1.5", 18},  // right already
		{"  font-size: 1rem;", 17},  // right already
		{"  color: red", 12},        // no unit rules
		{"  border: 1px solid", 13}, // px is right for borders
		{"  font-size: 16px;", 4},   // a value already there: no hint
		{"  min-height: calc(100vh - 2vh);", 22},
		{"  line-height: 24px;", 3},
		{"  font-size: 16", 15}, // a big number: no arrow hint
		{"  margin: 24", 12},
	} {
		if h, ok := UnitHintAt([]rune(c.line), c.col); ok {
			t.Fatalf("%q: unexpected hint %+v", c.line, h)
		}
	}
}
