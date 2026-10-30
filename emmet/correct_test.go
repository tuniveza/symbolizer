package emmet

import "testing"

// corApply makes the edits on line (they must be in order and not overlap).
func corApply(t *testing.T, line string, edits []Edit) string {
	t.Helper()
	rs := []rune(line)
	out := ""
	pos := 0
	for _, e := range edits {
		if e.Start < pos || e.End < e.Start || e.End > len(rs) {
			t.Fatalf("%q: bad edit %+v", line, e)
		}
		out += string(rs[pos:e.Start]) + e.New
		pos = e.End
	}
	return out + string(rs[pos:])
}

func TestCorrectCSSLine(t *testing.T) {
	cases := []struct{ line, want, note string }{
		// property names
		{"  colr: red;", "  color: red;", "fixed: colr → color"},
		{"  colour: red;", "  color: red;", "fixed: colour → color"},
		{"  background-colour: #fff;", "  background-color: #fff;", "fixed: background-colour → background-color"},
		{"  backgroud: #fff;", "  background: #fff;", "fixed: backgroud → background"},
		{"  dispaly: block;", "  display: block;", "fixed: dispaly → display"},
		{"  widht: 10px;", "  width: 10px;", "fixed: widht → width"},
		{"  heigth: 10px;", "  height: 10px;", "fixed: heigth → height"},
		{"  maring: 0;", "  margin: 0;", "fixed: maring → margin"},
		{"  pading: 0;", "  padding: 0;", "fixed: pading → padding"},
		{"  boder: 0;", "  border: 0;", "fixed: boder → border"},
		{"  font-wieght: bold;", "  font-weight: bold;", "fixed: font-wieght → font-weight"},
		{"  colr:", "  color:", "fixed: colr → color"},
		{"a { colr: red; heigth: 10px }", "a { color: red; height: 10px }", "fixed: colr → color, heigth → height"},
		// keyword values
		{"  display: flx;", "  display: flex;", "fixed: flx → flex"},
		{"  display: gird;", "  display: grid;", "fixed: gird → grid"},
		{"  display: inline-blok;", "  display: inline-block;", "fixed: inline-blok → inline-block"},
		{"  text-align: centre;", "  text-align: center;", "fixed: centre → center"},
		{"  position: absolut;", "  position: absolute;", "fixed: absolut → absolute"},
		{"  float: rigth;", "  float: right;", "fixed: rigth → right"},
		{"  font-weight: bld;", "  font-weight: bold;", "fixed: bld → bold"},
		{"  cursor: poiner;", "  cursor: pointer;", "fixed: poiner → pointer"},
		{"  overflow: hiden;", "  overflow: hidden;", "fixed: hiden → hidden"},
		{"  white-space: no-wrap;", "  white-space: nowrap;", "fixed: no-wrap → nowrap"},
		{"  justify-content: space-betwen;", "  justify-content: space-between;", "fixed: space-betwen → space-between"},
		{"  border: 1px soild red;", "  border: 1px solid red;", "fixed: soild → solid"},
		{"  -webkit-box-sizing: border-bx;", "  -webkit-box-sizing: border-box;", "fixed: border-bx → border-box"},
		{"  dispaly: flx;", "  display: flex;", "fixed: dispaly → display, flx → flex"},
		{"  color: rde;", "  color: red;", "fixed: rde → red"},
		{"  color: red !importnt;", "  color: red !important;", "fixed: !importnt → !important"},
		// units
		{"  width: 10xp;", "  width: 10px;", "fixed: 10xp → 10px"},
		{"  width: 10pz;", "  width: 10px;", "fixed: 10pz → 10px"},
		{"  margin: 0 1.5rme;", "  margin: 0 1.5rem;", "fixed: 1.5rme → 1.5rem"},
		{"  transition: opacity 200mss;", "  transition: opacity 200ms;", "fixed: 200mss → 200ms"},
		// ;;
		{"  margin: 0 auto;;", "  margin: 0 auto;", "fixed: ;; → ;"},
		{"  colr: red;;", "  color: red;", "fixed: colr → color, ;; → ;"},
	}
	for _, c := range cases {
		edits, note := CorrectCSSLine([]rune(c.line))
		if got := corApply(t, c.line, edits); got != c.want {
			t.Errorf("%q:\n got  %q\n want %q", c.line, got, c.want)
		}
		if note != c.note {
			t.Errorf("%q: note %q, want %q", c.line, note, c.note)
		}
	}
}

func TestCorrectCSSLineLeavesAlone(t *testing.T) {
	lines := []string{
		"  color: red;",
		"  color: grey;",
		"  colour-scheme",
		"  margin: 10px",
		"  /* colr: red; */",
		"  // colr: red;",
		"  content: 'colr: red;;';",
		"  background: url(a.png;b);",
		"  background: url(data:image/png;base64,AAA);",
		"  width: calc(10xp + 2px);",
		"  color: var(--colr);",
		"  transform: rotate(45dg);",
		"@media (max-widht: 10px) {",
		"@import url(foo.css);;",
		"  --colr: red;",
		"  $colr: red;",
		"  @colr: red;",
		"a:hover {",
		"li:hover",
		"  button:hover",
		"  .nav:hover",
		"  &:hover",
		"  a::before",
		"  my-el:focus",
		"  font-family: Arial, sans-serif;",
		"  animation: spin 2s linear infinite;",
		"  grid-template-columns: 1fr 1fr;",
		"  font-size: 1.2em;",
		"  line-height: 1.5;",
		"  z-index: 10;",
		"  color: #ff0;",
		"  color: gren;",
		"  width: 1.5ermm;",
		"  transition: all 2sec;",
		"  transition: opacity 200sm;", // s or ms: not clear
		"  font-weight: 700;",
		"  list-style-type: my-counter;",
		"  -webkit-tap-highlight-color: transparent;",
		"  display: -webkit-box;",
		"  transform: translate3d(0, 0, 0);",
		"  width: 100dvh;",
		"  font-size: 12pt;",
		"  background-image: image-set('a.png' 1x, 'b.png' 2x);",
	}
	for _, l := range lines {
		if edits, note := CorrectCSSLine([]rune(l)); len(edits) != 0 || note != "" {
			t.Errorf("%q changed: %+v %q", l, edits, note)
		}
	}
}

func TestCorDistance(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"color", "color", 0}, {"colr", "color", 1}, {"heigth", "height", 1}, {"maring", "margin", 2},
		{"", "abc", 3}, {"xp", "px", 1}, {"ca", "abc", 3},
	}
	for _, c := range cases {
		if got := corDistance(c.a, c.b); got != c.want {
			t.Errorf("corDistance(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
