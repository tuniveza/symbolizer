package emmet

import (
	"fmt"
	"strings"
	"testing"
)

// Test text marks: '|' is the cursor; '‹' and '›' are the ends of a selection.

func actMarks(s string) ([]rune, Range) {
	var out []rune
	r := Range{-1, -1}
	for _, c := range s {
		switch c {
		case '|':
			r = Range{len(out), len(out)}
		case '‹':
			r.Start = len(out)
		case '›':
			r.End = len(out)
		default:
			out = append(out, c)
		}
	}
	if r.Start < 0 {
		r.Start = r.End
	}
	if r.End < 0 {
		r.End = r.Start
	}
	return out, r
}

func actShow(text []rune, r Range) string {
	if r.Start == r.End {
		return string(text[:r.Start]) + "|" + string(text[r.Start:])
	}
	return string(text[:r.Start]) + "‹" + string(text[r.Start:r.End]) + "›" + string(text[r.End:])
}

// actResult applies a change and marks its Sel. "" when ok is false.
func actResult(t *testing.T, text []rune, c Change, ok bool) string {
	t.Helper()
	if !ok {
		return ""
	}
	for i := 1; i < len(c.Edits); i++ {
		if c.Edits[i].Start < c.Edits[i-1].End {
			t.Fatalf("edits overlap: %+v", c.Edits)
		}
	}
	out := []rune(actApply(text, c.Edits))
	if c.Sel.Start < 0 || c.Sel.End > len(out) || c.Sel.Start > c.Sel.End {
		t.Fatalf("bad Sel %+v for %q", c.Sel, string(out))
	}
	return actShow(out, c.Sel)
}

func TestActBalanceOut(t *testing.T) {
	cases := []struct{ in, want string }{
		{"<div><p>he|llo</p></div>", "<div><p>‹hello›</p></div>"},
		{"<div><p>‹hello›</p></div>", "<div>‹<p>hello</p>›</div>"},
		{"<div>‹<p>hello</p>›</div>", "‹<div><p>hello</p></div>›"},
		{"‹<div><p>hello</p></div>›", ""},
		{"<ul><li>a<li>|b</ul>", "<ul><li>a‹<li>b›</ul>"},
		{"<p>a <br> |b</p>", "<p>‹a <br> b›</p>"},
		{"<div><!-- <p> --><span>x|</span></div>", "<div><!-- <p> --><span>‹x›</span></div>"},
		{"<script>if (a<b) {}</script><b>|x</b>", "<script>if (a<b) {}</script><b>‹x›</b>"},
		{"<di|v>x</div>", "‹<div>x</div>›"},
		{"<a href='x>y'>t|</a>", "<a href='x>y'>‹t›</a>"},
		{"<Foo onClick={() => a > b}>t|</Foo>", "<Foo onClick={() => a > b}>‹t›</Foo>"},
		{"plain |text", ""},
	}
	for _, c := range cases {
		text, sel := actMarks(c.in)
		r, ok := BalanceOut(text, sel)
		got := ""
		if ok {
			got = actShow(text, r)
		}
		if got != c.want {
			t.Errorf("BalanceOut(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestActBalanceIn(t *testing.T) {
	cases := []struct{ in, want string }{
		{"‹<div><p>hi</p></div>›", "<div>‹<p>hi</p>›</div>"},
		{"<div>‹<p>hi</p><b>x</b>›</div>", "<div>‹<p>hi</p>›<b>x</b></div>"},
		{"<div>‹<p>hi</p>›<b>x</b></div>", "<div><p>‹hi›</p><b>x</b></div>"},
		{"<d|iv><p>hi</p></div>", "<div>‹<p>hi</p>›</div>"},
		{"<p>te|xt</p>", ""},
		{"<div>te|xt <b>x</b></div>", "<div>text ‹<b>x</b>›</div>"},
	}
	for _, c := range cases {
		text, sel := actMarks(c.in)
		r, ok := BalanceIn(text, sel)
		got := ""
		if ok {
			got = actShow(text, r)
		}
		if got != c.want {
			t.Errorf("BalanceIn(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestActMatchingPair(t *testing.T) {
	cases := []struct{ in, want string }{
		{"<d|iv class=\"a\"><p></p></div>", "<div class=\"a\"><p></p>|</div>"},
		{"<div class=\"a\"><p></p></d|iv>", "|<div class=\"a\"><p></p></div>"},
		{"<div><div>x</div>|</div>", "|<div><div>x</div></div>"},
		{"<DIV>x</d|iv>", "|<DIV>x</div>"},
		{"<p>a<br|/>b</p>", ""},
		{"text|", ""},
	}
	for _, c := range cases {
		text, sel := actMarks(c.in)
		p, ok := MatchingPair(text, sel.Start)
		got := ""
		if ok {
			got = actShow(text, Range{p, p})
		}
		if got != c.want {
			t.Errorf("MatchingPair(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestActRemoveTag(t *testing.T) {
	opt := Options{Syntax: "html", Indent: "  "}
	cases := []struct{ in, want string }{
		{"<p>Hello <b>wo|rld</b>!</p>", "<p>Hello wo|rld!</p>"},
		{"<ul>\n  <d|iv>\n    <li>a</li>\n\n    <li>b</li>\n  </div>\n</ul>", "<ul>\n|  <li>a</li>\n\n  <li>b</li>\n</ul>"},
		{"<p>a<br|>b</p>", "<p>a|b</p>"},
		{"a\n  <d|iv></div>\nb", "a\n|b"},
		{"<div>\n  x|\n</div>", "x|"},
		{"<li>a|", "a|"},
		{"<div>\n\tx|\n</div>\n", "x|\n"},
		{"no tags |here", ""},
	}
	for _, c := range cases {
		text, sel := actMarks(c.in)
		ch, ok := RemoveTag(text, sel.Start, opt)
		if got := actResult(t, text, ch, ok); got != c.want {
			t.Errorf("RemoveTag(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestActSplitJoinTag(t *testing.T) {
	cases := []struct{ syn, in, want, note string }{
		{"html", "<div class=\"a\">te|xt</div>", "<div class=\"a\"|/>", "content"},
		{"xhtml", "<d|iv></div>", "<d|iv />", ""},
		{"html", "<div |/>", "<div>|</div>", ""},
		{"jsx", "<Foo bar={1} |/>", "<Foo bar={1}>|</Foo>", ""},
		{"html", "<img src=\"a.png\"|>", "", ""},
		{"html", "<br|/>", "", ""},
		{"jsx", "<img src=\"a\" |/>", "<img src=\"a\">|</img>", ""},
		{"xml", "<item|></item>", "<item| />", ""},
	}
	for _, c := range cases {
		text, sel := actMarks(c.in)
		ch, ok := SplitJoinTag(text, sel.Start, Options{Syntax: c.syn})
		if got := actResult(t, text, ch, ok); got != c.want {
			t.Errorf("SplitJoinTag(%s %q) = %q, want %q", c.syn, c.in, got, c.want)
		}
		if c.note != "" && !strings.Contains(ch.Note, c.note) {
			t.Errorf("SplitJoinTag(%q) note %q", c.in, ch.Note)
		}
	}
}

func TestActUpdateTag(t *testing.T) {
	cases := []struct{ syn, in, abbr, want string }{
		{"html", "<div class=\"a\">x|</div>", "section", "<section class=\"a\">x|</section>"},
		{"html", "<div class=\"a\">x|</div>", ".b.a", "<div class=\"a b\">x|</div>"},
		{"html", "<div class=\"a\">x|</div>", "#main", "<div class=\"a\" id=\"main\">x|</div>"},
		{"html", "<div class=\"a\">x|</div>", "[title='Hi there' data-x=1]", "<div class=\"a\" title=\"Hi there\" data-x=\"1\">x|</div>"},
		{"html", "<p id='old'>|</p>", "#new", "<p id='new'>|</p>"},
		{"jsx", "<div>|</div>", "span.a", "<span className=\"a\">|</span>"},
		{"html", "<input disabled|>", "[disabled=yes]", "<input disabled|=\"yes\">"},
		{"html", "<div>|</div>", "div>p", ""},
		{"html", "<div>|</div>", "", ""},
		{"html", "<div class=\"a\">|</div>", ".a", ""},
	}
	for _, c := range cases {
		text, sel := actMarks(c.in)
		ch, ok := UpdateTag(text, sel.Start, c.abbr, Options{Syntax: c.syn})
		if got := actResult(t, text, ch, ok); got != c.want {
			t.Errorf("UpdateTag(%q, %q) = %q, want %q", c.in, c.abbr, got, c.want)
		}
	}
}

func TestActNextEditPoint(t *testing.T) {
	cases := []struct {
		in   string
		dir  int
		want string
	}{
		{"|<a href=\"\"></a>", 1, "<a href=\"|\"></a>"},
		{"<a href=\"|\"></a>", 1, "<a href=\"\">|</a>"},
		{"<a href=\"\"></a>|", -1, "<a href=\"\">|</a>"},
		{"<a href=\"\">|</a>", -1, "<a href=\"|\"></a>"},
		{"|<div>\n  \n</div>", 1, "<div>\n  |\n</div>"},
		{"|<p>x</p>", 1, ""},
		{"|<C a={}/>", 1, "<C a={|}/>"},
	}
	for _, c := range cases {
		text, sel := actMarks(c.in)
		p, ok := NextEditPoint(text, sel.Start, c.dir)
		got := ""
		if ok {
			got = actShow(text, Range{p, p})
		}
		if got != c.want {
			t.Errorf("NextEditPoint(%q, %d) = %q, want %q", c.in, c.dir, got, c.want)
		}
	}
}

func actChain(t *testing.T, name, start string, dir int, syntax string, want []string) {
	t.Helper()
	text, sel := actMarks(start)
	for i, w := range want {
		r, ok := SelectItem(text, sel, dir, syntax)
		if !ok {
			t.Errorf("%s step %d: no item, want %q", name, i, w)
			return
		}
		if got := string(text[r.Start:r.End]); got != w {
			t.Errorf("%s step %d: got %q, want %q", name, i, got, w)
			return
		}
		sel = r
	}
	if r, ok := SelectItem(text, sel, dir, syntax); ok {
		t.Errorf("%s: one more item %q, want none", name, string(text[r.Start:r.End]))
	}
}

func TestActSelectItem(t *testing.T) {
	actChain(t, "html next", "|<div id=\"x\" class=\"a b\"><img src=\"p.png\"></div>", 1, "html",
		[]string{"div", `id="x"`, "x", `class="a b"`, "a b", "a", "b", "img", `src="p.png"`, "p.png"})
	actChain(t, "html prev", "<div id=\"x\" class=\"a b\"></div>|", -1, "html",
		[]string{"b", "a", "a b", `class="a b"`, "x", `id="x"`, "div"})
	actChain(t, "css next", "|a { color: red; margin: 0 auto; }", 1, "css",
		[]string{"a", "color: red;", "red", "margin: 0 auto;", "0 auto", "0", "auto"})
	actChain(t, "css prev", "a { color: red; margin: 0 auto }|", -1, "css",
		[]string{"auto", "0", "0 auto", "margin: 0 auto", "red", "color: red;", "a"})
	// a cursor inside an item picks that item
	text, sel := actMarks("<div class=\"one tw|o\">")
	if r, ok := SelectItem(text, sel, 1, "html"); !ok || string(text[r.Start:r.End]) != "two" {
		t.Errorf("cursor in class word: %v %q", ok, string(text[r.Start:r.End]))
	}
	text, sel = actMarks("<d|iv class=\"x\">")
	if r, ok := SelectItem(text, sel, 1, "html"); !ok || string(text[r.Start:r.End]) != "div" {
		t.Errorf("cursor in tag name: %v %q", ok, string(text[r.Start:r.End]))
	}
	text, sel = actMarks("a { b: url(x.png) no-repeat, red; }\n.c|d:hover > e { f: g }")
	if r, ok := SelectItem(text, sel, 1, "scss"); !ok || string(text[r.Start:r.End]) != ".cd:hover > e" {
		t.Errorf("cursor in selector: %v %q", ok, string(text[r.Start:r.End]))
	}
}

func TestActToggleBlockComment(t *testing.T) {
	cases := []struct{ syn, in, want string }{
		{"html", "<div><p>x|</p></div>", "<div><!-- <p>x|</p> --></div>"},
		{"html", "<div><!-- <p>x|</p> --></div>", "<div><p>x|</p></div>"},
		{"html", "a ‹b› c", "a <!-- ‹b› --> c"},
		{"html", "‹a <!-- b --> c›", "<!-- ‹a b c› -->"},
		{"jsx", "<div>|x</div>", "{/* <div>|x</div> */}"},
		{"jsx", "{/* <div>|x</div> */}", "<div>|x</div>"},
		{"css", "a { color: r|ed; margin: 0; }", "a { /* color: r|ed; */ margin: 0; }"},
		{"css", "a|b { c: d; }", "/* a|b { c: d; } */"},
		{"css", "a { /* col|or: red; */ }", "a { col|or: red; }"},
		{"scss", "a { b { c: d|; } }", "a { b { /* c: d|; */ } }"},
		{"html", "just |text", ""},
		{"css", "a { |  }", ""},
	}
	for _, c := range cases {
		text, sel := actMarks(c.in)
		ch, ok := ToggleBlockComment(text, sel, c.syn)
		if got := actResult(t, text, ch, ok); got != c.want {
			t.Errorf("ToggleBlockComment(%s %q) = %q, want %q", c.syn, c.in, got, c.want)
		}
	}
}

func TestActAutoCloseTag(t *testing.T) {
	cases := []struct{ syn, in, want string }{
		{"html", "<div>|", "</div>"},
		{"html", "<ul>\n  <li class=\"a\">|", "</li>"},
		{"html", "<br>|", ""},
		{"xml", "<br>|", "</br>"},
		{"html", "<img src=\"x\"/>|", ""},
		{"html", "<!-- a>|", ""},
		{"html", "<div></div>|", ""},
		{"html", "<div>|</div>", ""},
		{"html", "<p>|</pre>", "</p>"},
		{"html", "a<b>|", ""},
		{"jsx", "const a: Array<string>|", ""},
		{"jsx", "return (<>|", "</>"},
		{"html", "<>|", ""},
		{"html", "<?php echo 1 ?>|", ""},
		{"html", "<!DOCTYPE html>|", ""},
		{"html", "<div attr=\"a>|", ""},
		{"jsx", "<Foo bar={a > b}>|", "</Foo>"},
		{"html", "<ul><li>a</li><li>b</|", "li>"},
		{"html", "<div><p>a</|", "p>"},
		{"html", "<div><br></|", "div>"},
		{"html", "</|", ""},
		{"html", "<div></|div>", ""},
		{"html", "<div>text <!-- </|", ""},
		{"html", "<div>\n  <p>x</p>\n</|", "div>"},
		{"html", "x|", ""},
	}
	for _, c := range cases {
		text, sel := actMarks(c.in)
		got, ok := AutoCloseTag(text, sel.Start, c.syn)
		if !ok {
			got = ""
		}
		if got != c.want {
			t.Errorf("AutoCloseTag(%s %q) = %q, want %q", c.syn, c.in, got, c.want)
		}
	}
}

func TestActTagNamePair(t *testing.T) {
	cases := []struct{ in, open, close string }{
		{"<div|>x</div>", "1-4", "8-11"},
		{"<div>x</d|iv>", "1-4", "8-11"},
		{"<|div>x</div>", "1-4", "8-11"},
		{"<div><div>x</div></d|iv>", "1-4", "19-22"},
		{"<divx|>x</div>", "1-5", "9-12"},
		{"<br|>", "", ""},
		{"<div>|x</div>", "", ""},
	}
	for _, c := range cases {
		text, sel := actMarks(c.in)
		o, cl, ok := TagNamePair(text, sel.Start)
		got := []string{"", ""}
		if ok {
			got = []string{actRangeStr(o), actRangeStr(cl)}
		}
		if got[0] != c.open || got[1] != c.close {
			t.Errorf("TagNamePair(%q) = %v, want %s %s", c.in, got, c.open, c.close)
		}
	}
}

func actRangeStr(r Range) string { return fmt.Sprintf("%d-%d", r.Start, r.End) }

func TestActScannerShapes(t *testing.T) {
	text := []rune(`<!DOCTYPE html><?xml x?><![CDATA[<a>]]><a href=x b='y' c="z" d {...p}/><p>`)
	toks := actScan(text, "html")
	kinds := []actTokKind{actTokDoctype, actTokPI, actTokCDATA, actTokOpen, actTokOpen}
	if len(toks) != len(kinds) {
		t.Fatalf("got %d tokens, want %d", len(toks), len(kinds))
	}
	for i, k := range kinds {
		if toks[i].kind != k {
			t.Errorf("token %d kind %d, want %d", i, toks[i].kind, k)
		}
	}
	a := toks[3]
	if !a.selfClose || len(a.attrs) != 5 {
		t.Fatalf("a: selfClose=%v attrs=%d", a.selfClose, len(a.attrs))
	}
	vals := []string{"x", "y", "z", "", "...p"}
	for i, v := range vals {
		at := a.attrs[i]
		got := ""
		if at.hasValue() {
			got = string(text[at.valStart:at.valEnd])
		}
		if got != v {
			t.Errorf("attr %d value %q, want %q", i, got, v)
		}
	}
	d := actParse([]rune("<table><tr><td>a<td>b<tr><td>c</table>"), "html")
	if len(d.nodes) != 6 || d.nodes[0].close == nil {
		t.Fatalf("table: %d nodes", len(d.nodes))
	}
	if n := len(d.nodes[0].children); n != 2 {
		t.Errorf("table has %d rows, want 2", n)
	}
}
