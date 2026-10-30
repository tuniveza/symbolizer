package emmet

import (
	"sort"
	"strings"
	"testing"
)

// cssTestMark writes the text with each stop wrapped in [[ ]], the way the
// expected outputs were taken from real Emmet (fields as [[placeholder]]).
func cssTestMark(e Expansion) string {
	rs := []rune(e.Text)
	stops := append([]Range{}, e.Stops...)
	sort.SliceStable(stops, func(i, j int) bool { return stops[i].Start < stops[j].Start })
	var b strings.Builder
	pos := 0
	for _, s := range stops {
		if s.Start < pos {
			continue // a stop inside another (cnt): marked by the outer one
		}
		b.WriteString(string(rs[pos:s.Start]))
		b.WriteString("[[" + string(rs[s.Start:s.End]) + "]]")
		pos = s.End
	}
	b.WriteString(string(rs[pos:]))
	return b.String()
}

// cssEmmetCases are real Emmet 2.4.11 outputs (VS Code settings: fuzzy
// threshold 0.3), taken by running emmet's expand() with fields as [[...]].
var cssEmmetCases = []struct{ syntax, abbr, want string }{
	{"css", "m10", "margin: 10px;"},
	{"css", "m-10", "margin: -10px;"},
	{"css", "m10-20", "margin: 10px 20px;"},
	{"css", "m10--20", "margin: 10px -20px;"},
	{"css", "m0", "margin: 0;"},
	{"css", "m.5", "margin: 0.5em;"},
	{"css", "m1.5", "margin: 1.5em;"},
	{"css", "m-a", "margin: auto;"},
	{"css", "m0-a", "margin: 0 auto;"},
	{"css", "p10e", "padding: 10em;"},
	{"css", "p0-10-0", "padding: 0 10px 0;"},
	{"css", "w100p", "width: 100%;"},
	{"css", "w1fr", "width: 1fr;"},
	{"css", "h10vh", "height: 10vh;"},
	{"css", "w1/3", "width: 1px / 3px;"},
	{"css", "mah100", "max-height: 100px;"},
	{"css", "maw100p", "max-width: 100%;"},
	{"css", "miw0", "min-width: 0;"},
	{"css", "fz12", "font-size: 12px;"},
	{"css", "fz1.2r", "font-size: 1.2rem;"},
	{"css", "lts.1e", "letter-spacing: 0.1em;"},
	{"css", "ti-9999", "text-indent: -9999px;"},
	{"css", "lh1.5", "line-height: 1.5;"},
	{"css", "lh.5", "line-height: 0.5;"},
	{"css", "z2", "z-index: 2;"},
	{"css", "zi", "z-index: inherit;"},
	{"css", "op.5", "opacity: 0.5;"},
	{"css", "fx1", "flex: 1;"},
	{"css", "fw700", "font-weight: 700;"},
	{"css", "fw:b", "font-weight: bold;"},
	{"css", "fw:n", "font-weight: normal;"},
	{"css", "pos", "position: [[relative]];"},
	{"css", "posa", "position: absolute;"},
	{"css", "posr", "position: relative;"},
	{"css", "pos:f", "position: fixed;"},
	{"css", "t0", "top: 0;"},
	{"css", "r0", "right: 0;"},
	{"css", "b0", "bottom: 0;"},
	{"css", "l0", "left: 0;"},
	{"css", "t-10", "top: -10px;"},
	{"css", "d", "display: [[block]];"},
	{"css", "dib", "display: inline-block;"},
	{"css", "df", "display: flex;"},
	{"css", "d:f", "display: flex;"},
	{"css", "dif", "display: inline-flex;"},
	{"css", "dg", "display: grid;"},
	{"css", "dig", "display: inline-grid;"},
	{"css", "d:n", "display: none;"},
	{"css", "v:h", "visibility: hidden;"},
	{"css", "ov:h", "overflow: hidden;"},
	{"css", "ovh", "overflow: hidden;"},
	{"css", "ovxh", "overflow-x: hidden;"},
	{"css", "fl:l", "float: left;"},
	{"css", "fll", "float: left;"},
	{"css", "flr", "float: right;"},
	{"css", "cl:b", "clear: both;"},
	{"css", "clb", "clear: both;"},
	{"css", "cur:p", "cursor: pointer;"},
	{"css", "c", "color: [[#000]];"},
	{"css", "c#f", "color: #fff;"},
	{"css", "c#fc0", "color: #fc0;"},
	{"css", "c#e0", "color: #e0e0e0;"},
	{"css", "c#00ff00", "color: #0f0;"},
	{"css", "c#f.5", "color: rgba(255, 255, 255, 0.5);"},
	{"css", "c#0.25", "color: rgba(0, 0, 0, 0.25);"},
	{"css", "c#t", "color: transparent;"},
	{"css", "c:t", "color: t;"},
	{"css", "bgc#e0", "background-color: #e0e0e0;"},
	{"css", "bgc:t", "background-color: t;"},
	{"css", "bg#f.5", "background: rgba(255, 255, 255, 0.5);"},
	{"css", "bd", "border: [[1px]] [[solid]] [[#000]];"},
	{"css", "bd1-s#f", "border: 1px solid #fff;"},
	{"css", "bd0", "border: 0;"},
	{"css", "bdn", "border: none;"},
	{"css", "bds:d", "border-style: double;"},
	{"css", "bdrs5", "border-radius: 5px;"},
	{"css", "bdrs5-10", "border-radius: 5px 10px;"},
	{"css", "bdw2", "border-width: 2px;"},
	{"css", "bxsh", "box-shadow: [[inset ]][[hoff]] [[voff]] [[blur]] [[#000]];"},
	{"css", "bxz", "box-sizing: [[border-box]];"},
	{"css", "bxsz", "box-sizing: [[border-box]];"},
	{"css", "ta:c", "text-align: center;"},
	{"css", "tac", "text-align: center;"},
	{"css", "tar", "text-align: right;"},
	{"css", "td:n", "text-decoration: none;"},
	{"css", "tdu", "text-decoration: underline;"},
	{"css", "tt:u", "text-transform: uppercase;"},
	{"css", "va:m", "vertical-align: middle;"},
	{"css", "whsn", "white-space: normal;"},
	{"css", "ws:n", "word-spacing: none;"},
	{"css", "ff:a", "font-family: auto;"},
	{"css", "ffs", "font-family: serif;"},
	{"css", "ffss", "font-family: sans-serif;"},
	{"css", "fs:i", "font-style: italic;"},
	{"css", "jc:sb", "justify-content: space-between;"},
	{"css", "jcsb", "justify-content: space-between;"},
	{"css", "ai:c", "align-items: center;"},
	{"css", "aic", "align-items: center;"},
	{"css", "fxd:c", "flex-direction: column;"},
	{"css", "fxdc", "flex-direction: column;"},
	{"css", "fxw:w", "flex-wrap: wrap;"},
	{"css", "gtc", "grid-template-columns: repeat([[]]);"},
	{"css", "gtr", "grid-template-rows: repeat([[]]);"},
	{"css", "gg10", "grid-gap: 10px;"},
	{"css", "rg10", "row-gap: 10px;"},
	{"css", "cg10", "column-gap: 10px;"},
	{"css", "trf", "transform: [[]];"},
	{"css", "trf:sc", "transform: scale([[x]], [[y]]);"},
	{"css", "trf:t", "transform: translate([[x]], [[y]]);"},
	{"css", "trfr", "transform: rotate([[angle]]);"},
	{"css", "trs", "transition: [[prop]] [[time]];"},
	{"css", "trsde.5s", "transition-delay: 0.5s;"},
	{"css", "anim", "animation: [[name]] [[duration]] [[timing-function]] [[delay]] [[iteration-count]] [[direction]] [[fill-mode]];"},
	{"css", "animtf", "animation-timing-function: [[linear]];"},
	{"css", "animtf:cb", "animation-timing-function: cubic-bezier([[0.1]], [[0.7]], [[1.0]], [[0.1]]);"},
	{"css", "cnt:a", "content: attr([[]]);"},
	{"css", "q", "quotes: [[]];"},
	{"css", "lisn", "list-style: none;"},
	{"css", "list:d", "list-style-type: disc;"},
	{"css", "bgr:n", "background-repeat: no-repeat;"},
	{"css", "bgi", "background-image: url([[]]);"},
	{"css", "bgpx10", "background-position-x: 10px;"},
	{"css", "bgsz:cv", "background-size: cover;"},
	{"css", "lg", "background-image: linear-gradient([[]]);"},
	{"css", "lg(top,#f,#0)", "background-image: linear-gradient(top, #fff, #000);"},
	{"css", "m10+p5", "margin: 10px;\npadding: 5px;"},
	{"css", "m10+p5+c#f", "margin: 10px;\npadding: 5px;\ncolor: #fff;"},
	{"css", "p!+m!", "padding: [[]] !important;\nmargin: [[]] !important;"},
	{"css", "m10!", "margin: 10px !important;"},
	{"css", "c#f!", "color: #fff !important;"},
	{"css", "!", "!important"},
	{"css", "@m", "@media [[screen]] {\n\t[[]]\n}"},
	{"css", "@media", "@media [[screen]] {\n\t[[]]\n}"},
	{"css", "@i", "@import url([[]]);"},
	{"css", "@kf", "@keyframes [[identifier]] {\n\t[[]]\n}"},
	{"css", "@f", "@font-face {\n\tfont-family: [[]];\n\tsrc: url([[]]);\n}"},
	{"css", "@ff", "@font-face {\n\tfont-family: '[[FontName]]';\n\tsrc: url('[[FileName]].eot');\n\tsrc: url('[[FileName]].eot?#iefix') format('embedded-opentype'),\n\t\t url('[[FileName]].woff') format('woff'),\n\t\t url('[[FileName]].ttf') format('truetype'),\n\t\t url('[[FileName]].svg#[[FontName]]') format('svg');\n\tfont-style: [[normal]];\n\tfont-weight: [[normal]];\n}"},
	{"css", "cm", "/* [[]] */"},
	{"css", "foo", "font: optional;"},
	{"css", "header", "header: [[]];"},
	{"css", "colr", "column-rule: [[]];"},
	{"css", "olo5", "outline-offset: 5px;"},
	{"css", "op0", "opacity: 0;"},
	{"css", "ord1", "order: 1px;"},
	{"css", "display", "display: [[]];"},
	{"css", "c:r", "color: r;"},
	{"css", "zom1", "zoom: 1;"},
	{"scss", "m10", "margin: 10px;"},
	{"scss", "c#f.5", "color: rgba(255, 255, 255, 0.5);"},
	{"scss", "posa", "position: absolute;"},
	{"scss", "m10+p5", "margin: 10px;\npadding: 5px;"},
	{"scss", "@m", "@media [[screen]] {\n\t[[]]\n}"},
	{"scss", "bd1-s#f", "border: 1px solid #fff;"},
	{"less", "p10-20", "padding: 10px 20px;"},
	{"less", "dib", "display: inline-block;"},
	{"less", "lg", "background-image: linear-gradient([[]]);"},
	{"sass", "m10", "margin: 10px"},
	{"sass", "m10+p5", "margin: 10px\npadding: 5px"},
	{"sass", "c#fc0", "color: #fc0"},
	{"sass", "d:f", "display: flex"},
	{"sass", "@m", "@media [[screen]] {\n\t[[]]\n}"},
	{"sass", "bd", "border: [[1px]] [[solid]] [[#000]]"},
	{"sass", "pos", "position: [[relative]]"},
	{"sass", "m10!", "margin: 10px !important"},
	{"stylus", "m10", "margin 10px"},
	{"stylus", "m10+p5", "margin 10px\npadding 5px"},
	{"stylus", "c#fc0", "color #fc0"},
	{"stylus", "d:f", "display flex"},
	{"stylus", "@m", "@media [[screen]] {\n\t[[]]\n}"},
	{"stylus", "bd", "border [[1px]] [[solid]] [[#000]]"},
	{"stylus", "pos", "position [[relative]]"},
	{"stylus", "posa", "position absolute"},
	{"stylus", "w100p+h50p", "width 100%\nheight 50%"},
}

// cssEnhanced are the abbreviations where sy knowingly differs from Emmet 2.
var cssEnhanced = map[string]bool{
	"ord1":    true, // order is unitless here: "order: 1;"
	"display": true, // a whole property name uses its snippet: "display: [[block]];"
}

func TestExpandCSSMatchesEmmet(t *testing.T) {
	if len(cssEmmetCases) < 70 {
		t.Fatalf("only %d cases", len(cssEmmetCases))
	}
	for _, c := range cssEmmetCases {
		opt := Options{Syntax: c.syntax, Indent: "\t"}
		e, err := cssExpand(c.abbr, opt, true)
		if err != nil {
			t.Errorf("%s %q: %v", c.syntax, c.abbr, err)
			continue
		}
		if got := cssTestMark(e); got != c.want {
			t.Errorf("pure %s %q:\n got  %q\n want %q", c.syntax, c.abbr, got, c.want)
		}
		if cssEnhanced[c.abbr] {
			continue
		}
		e, err = Expand(c.abbr, opt)
		if err != nil {
			t.Errorf("%s %q: %v", c.syntax, c.abbr, err)
			continue
		}
		if got := cssTestMark(e); got != c.want {
			t.Errorf("Expand %s %q:\n got  %q\n want %q", c.syntax, c.abbr, got, c.want)
		}
	}
}

func TestExpandCSSAdditions(t *testing.T) {
	cases := []struct{ syntax, abbr, want, note string }{
		{"css", "ord1", "order: 1;", ""},
		{"css", "display", "display: [[block]];", ""},
		{"css", "color", "color: [[#000]];", ""},
		{"css", "font-size12", "font-size: 12px;", ""},
		{"css", "margin-top10", "margin-top: 10px;", ""},
		{"css", "margin-10", "margin: -10px;", ""},
		{"css", "top10", "top: 10px;", ""},
		{"css", "gap1r", "gap: 1rem;", ""},
		{"css", "gr1", "grid-row: 1;", ""},
		{"css", "flex1", "flex: 1;", ""},
		{"css", "display:f", "display: flex;", ""},
		{"css", "dispaly", "display: [[block]];", "fixed: dispaly → display"},
		{"css", "colour#f", "color: #fff;", "fixed: colour → color"},
		{"css", "background-colour#e0", "background-color: #e0e0e0;", "fixed: background-colour → background-color"},
		{"css", "heigth20", "height: 20px;", "fixed: heigth → height"},
		{"css", "maring", "margin: [[]];", "fixed: maring → margin"},
		{"css", "-bdrs", "-webkit-border-radius: [[]];\n-moz-border-radius: [[]];\nborder-radius: [[]];", ""},
		{"css", "-bdrs5", "-webkit-border-radius: 5px;\n-moz-border-radius: 5px;\nborder-radius: 5px;", ""},
		{"css", "-wm-trf", "-webkit-transform: [[]];\n-moz-transform: [[]];\ntransform: [[]];", ""},
		{"css", "-trf", "-webkit-transform: [[]];\n-moz-transform: [[]];\n-ms-transform: [[]];\n-o-transform: [[]];\ntransform: [[]];", ""},
		{"css", "-o-trs", "-o-transition: [[prop]] [[time]];\ntransition: [[prop]] [[time]];", ""},
		{"stylus", "-bdrs5", "-webkit-border-radius 5px\n-moz-border-radius 5px\nborder-radius 5px", ""},
		{"css", "!imp", "!important", ""},
		{"css", "@cs", "@charset \"[[UTF-8]]\";", ""},
		{"css", "cnt", "content: [['']];", ""},
		{"css", "m10+-bdrs3", "margin: 10px;\n-webkit-border-radius: 3px;\n-moz-border-radius: 3px;\nborder-radius: 3px;", ""},
	}
	for _, c := range cases {
		e, err := Expand(c.abbr, Options{Syntax: c.syntax, Indent: "\t"})
		if err != nil {
			t.Errorf("%q: %v", c.abbr, err)
			continue
		}
		if got := cssTestMark(e); got != c.want {
			t.Errorf("%s %q:\n got  %q\n want %q", c.syntax, c.abbr, got, c.want)
		}
		if e.Note != c.note {
			t.Errorf("%q: note %q, want %q", c.abbr, e.Note, c.note)
		}
	}
}

func TestExpandCSSIndent(t *testing.T) {
	opt := Options{Syntax: "css", Indent: "  ", BaseIndent: "    "}
	e, err := Expand("m10+p5", opt)
	if err != nil || e.Text != "margin: 10px;\n    padding: 5px;" {
		t.Errorf("chain: %q %v", e.Text, err)
	}
	e, err = Expand("@m", opt)
	if err != nil || e.Text != "@media screen {\n      \n    }" {
		t.Errorf("@m: %q %v", e.Text, err)
	}
	e, err = Expand("@kf", Options{Syntax: "scss", Indent: "\t"})
	if err != nil || e.Text != "@keyframes identifier {\n\t\n}" {
		t.Errorf("@kf: %q %v", e.Text, err)
	}
}

func TestExpandCSSStops(t *testing.T) {
	cases := []struct {
		abbr  string
		stops []string // the text at each stop, in Tab order; "|" for an empty stop
	}{
		{"m10", nil},
		{"posa", nil},
		{"c", []string{"#000"}},
		{"m", []string{"|"}},
		{"pos", []string{"relative"}},
		{"bd", []string{"1px", "solid", "#000"}},
		{"@m", []string{"screen", "|"}},
		{"@kf", []string{"identifier", "|"}},
		{"trf:sc", []string{"x", "y"}},
		{"bd+c", []string{"1px", "solid", "#000", "#000"}},
		{"m+p", []string{"|", "|"}},
		{"cnt", []string{"''", "|"}},
		{"-bxsh", []string{"inset ", "hoff", "voff", "blur", "#000", "inset ", "hoff", "voff", "blur", "#000", "inset ", "hoff", "voff", "blur", "#000"}},
	}
	for _, c := range cases {
		e, err := Expand(c.abbr, Options{Syntax: "css", Indent: "\t"})
		if err != nil {
			t.Errorf("%q: %v", c.abbr, err)
			continue
		}
		rs := []rune(e.Text)
		var got []string
		for _, s := range e.Stops {
			if s.Start < 0 || s.End > len(rs) || s.Start > s.End {
				t.Fatalf("%q: bad stop %v in %q", c.abbr, s, e.Text)
			}
			txt := string(rs[s.Start:s.End])
			if txt == "" {
				txt = "|"
			}
			got = append(got, txt)
		}
		if strings.Join(got, ",") != strings.Join(c.stops, ",") {
			t.Errorf("%q: stops %q, want %q (text %q)", c.abbr, got, c.stops, e.Text)
		}
	}
	// a stop in the empty value sits right after "margin: "
	e, _ := Expand("m", Options{Syntax: "css"})
	if len(e.Stops) != 1 || e.Stops[0] != (Range{8, 8}) {
		t.Errorf("m: %v", e.Stops)
	}
}

func TestExpandCSSErrors(t *testing.T) {
	for _, a := range []string{"", "m10*2", "m10)", "m${1"} {
		if _, err := Expand(a, Options{Syntax: "css"}); err == nil {
			t.Errorf("%q: no error", a)
		}
	}
}

func TestCSSScoreMatch(t *testing.T) {
	cases := []struct {
		a, s    string
		partial bool
		want    float64
	}{
		{"bd", "bd", false, 1},
		{"x", "bd", true, 0},
		{"a", "ac", false, 0.5},
		{"ib", "inline-block", false, 23.0 * 2 / 12 / 23},
		{"dispaly", "display", false, 0},
		{"colr", "color", false, 13.0 * 4 / 5 / 14},
	}
	for _, c := range cases {
		if got := cssScoreMatch(c.a, c.s, c.partial); got-c.want > 1e-9 || c.want-got > 1e-9 {
			t.Errorf("score(%q, %q) = %v, want %v", c.a, c.s, got, c.want)
		}
	}
	// fuzzy: slightly wrong abbreviations still find a property (as Emmet)
	fuzzy := map[string]string{
		"bxz": "box-sizing", "colr": "column-rule", "poa": "position", "trsd": "transition-duration",
		"fxd": "flex-direction", "lisp": "list-style-position", "bdtrrs": "border-top-right-radius",
	}
	for a, want := range fuzzy {
		nodes, err := cssExpandNodes(a, true)
		if err != nil || len(nodes) != 1 || nodes[0].name != want {
			name := ""
			if len(nodes) > 0 {
				name = nodes[0].name
			}
			t.Errorf("fuzzy %q → %q, want %q (%v)", a, name, want, err)
		}
	}
}

func TestExtractCSS(t *testing.T) {
	cases := []struct {
		line  string
		col   int // -1: end of line
		abbr  string
		start int
		ok    bool
	}{
		{"  m10", -1, "m10", 2, true},
		{"m10", -1, "m10", 0, true},
		{"a { m10", -1, "m10", 4, true},
		{"  color: red; p5", -1, "p5", 14, true},
		{"  c#f", -1, "c#f", 2, true},
		{"  d:f", -1, "d:f", 2, true},
		{"  m10+p5", -1, "m10+p5", 2, true},
		{"  -bdrs", -1, "-bdrs", 2, true},
		{"  @m", -1, "@m", 2, true},
		{"  m10 more", 5, "m10", 2, true},
		{"/* x */ m10", -1, "m10", 8, true},
		{"  color: re", -1, "", 0, false},
		{"  color:", -1, "", 0, false},
		{"  .foo", -1, "", 0, false},
		{"  &:hover", -1, "", 0, false},
		{"  :root", -1, "", 0, false},
		{"  #fff", -1, "", 0, false},
		{"  $x", -1, "", 0, false},
		{"  --x", -1, "", 0, false},
		{"  ul li", -1, "", 0, false},
		{"  ul>li", -1, "", 0, false},
		{"/* m10", -1, "", 0, false},
		{"  // m10", -1, "", 0, false},
		{"  content: 'm10", -1, "", 0, false},
		{"  'm10", -1, "", 0, false},
		{"  ", -1, "", 0, false},
	}
	for _, c := range cases {
		line := []rune(c.line)
		col := c.col
		if col < 0 {
			col = len(line)
		}
		abbr, start, ok := Extract(line, col, "css")
		if ok != c.ok || abbr != c.abbr || (ok && start != c.start) {
			t.Errorf("Extract(%q, %d) = %q %d %v, want %q %d %v", c.line, col, abbr, start, ok, c.abbr, c.start, c.ok)
		}
	}
}

func TestCSSTabWorthy(t *testing.T) {
	yes := []string{"m10", "m", "p", "c#f", "d:f", "dib", "posa", "m10+p5", "-bdrs", "-wm-trf", "@m", "@media", "@kf",
		"bd1-s#f", "lh1.5", "!", "!imp", "display", "color", "font-size12", "dispaly", "bxz", "w100p", "lg", "trf:sc", "cm"}
	no := []string{"", "body", "header", "button", "div", "red", "auto", "none", "solid", "bold", "block", "grey",
		"a:hover", "li:first-child", "texta", "m10 p5", "!foo", "--x", "inherit"}
	for _, a := range yes {
		if !cssTabWorthy(a) {
			t.Errorf("cssTabWorthy(%q) = false", a)
		}
	}
	for _, a := range no {
		if cssTabWorthy(a) {
			t.Errorf("cssTabWorthy(%q) = true", a)
		}
	}
}

func TestTabExpandCSS(t *testing.T) {
	line := []rune("  m10")
	start, e, ok := TabExpand(line, len(line), Options{Syntax: "scss", Indent: "  ", BaseIndent: "  "})
	if !ok || start != 2 || e.Text != "margin: 10px;" {
		t.Errorf("TabExpand m10: %d %q %v", start, e.Text, ok)
	}
	line = []rune("  body")
	if _, _, ok := TabExpand(line, len(line), Options{Syntax: "css"}); ok {
		t.Error("TabExpand body: expanded")
	}
	line = []rune("  color: re")
	if _, _, ok := TabExpand(line, len(line), Options{Syntax: "css"}); ok {
		t.Error("TabExpand in a value: expanded")
	}
}
