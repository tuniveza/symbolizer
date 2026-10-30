package emmet

import (
	"strings"
	"testing"
)

func TestExpandMarkup(t *testing.T) {
	tests := []struct {
		abbr, syntax, indent, want string
	}{
		// elements, ids, classes, attributes, text
		{"div", "", "\t", `<div></div>`},
		{"div#header", "", "\t", `<div id="header"></div>`},
		{"div.a.b", "", "\t", `<div class="a b"></div>`},
		{"div#a.b.c", "", "\t", `<div id="a" class="b c"></div>`},
		{".item", "", "\t", `<div class="item"></div>`},
		{"#main", "", "\t", `<div id="main"></div>`},
		{`td[title="Hello world!" colspan=3]`, "", "\t", `<td title="Hello world!" colspan="3"></td>`},
		{`div[a b='c']`, "", "\t", `<div a="" b="c"></div>`},
		{`div.x[class=y]`, "", "\t", `<div class="x y"></div>`},
		{`div[data-on.]`, "", "\t", `<div data-on></div>`},
		{"a{Click me}", "", "\t", `<a href="">Click me</a>`},
		{"p{Hello}", "", "\t", `<p>Hello</p>`},
		{"{just text}", "", "\t", `just text`},
		{"p>{Click }+a{here}+{ to continue}", "", "\t", `<p>Click <a href="">here</a> to continue</p>`},
		{"div/", "", "\t", `<div>`},
		{"div/", "xhtml", "\t", `<div />`},

		// child, sibling, climb up, groups
		{"div>p", "", "\t", "<div>\n\t<p></p>\n</div>"},
		{"div>p", "", "  ", "<div>\n  <p></p>\n</div>"},
		{"div+p+bq", "", "\t", "<div></div>\n<p></p>\n<blockquote></blockquote>"},
		{"div+div>p>span+em^bq", "", "\t", "<div></div>\n<div>\n\t<p><span></span><em></em></p>\n\t<blockquote></blockquote>\n</div>"},
		{"div+div>p>span+em^^bq", "", "\t", "<div></div>\n<div>\n\t<p><span></span><em></em></p>\n</div>\n<blockquote></blockquote>"},
		{"div>(header>ul>li*2>a)+footer>p", "", "\t", "<div>\n\t<header>\n\t\t<ul>\n\t\t\t<li><a href=\"\"></a></li>\n\t\t\t<li><a href=\"\"></a></li>\n\t\t</ul>\n\t</header>\n\t<footer>\n\t\t<p></p>\n\t</footer>\n</div>"},
		{"(div>dl>(dt+dd)*2)+footer>p", "", "  ", "<div>\n  <dl>\n    <dt></dt>\n    <dd></dd>\n    <dt></dt>\n    <dd></dd>\n  </dl>\n</div>\n<footer>\n  <p></p>\n</footer>"},
		{"(a+b)*2", "", "\t", "<a href=\"\"></a>\n<b></b>\n<a href=\"\"></a>\n<b></b>"},

		// multiply and numbering
		{"ul>li*3", "", "\t", "<ul>\n\t<li></li>\n\t<li></li>\n\t<li></li>\n</ul>"},
		{"ul>li.item$*3", "", "  ", "<ul>\n  <li class=\"item1\"></li>\n  <li class=\"item2\"></li>\n  <li class=\"item3\"></li>\n</ul>"},
		{"ol>li.i$$*2", "", "\t", "<ol>\n\t<li class=\"i01\"></li>\n\t<li class=\"i02\"></li>\n</ol>"},
		{"ul>li.item$@-*3", "", "\t", "<ul>\n\t<li class=\"item3\"></li>\n\t<li class=\"item2\"></li>\n\t<li class=\"item1\"></li>\n</ul>"},
		{"ul>li.item$@3*3", "", "\t", "<ul>\n\t<li class=\"item3\"></li>\n\t<li class=\"item4\"></li>\n\t<li class=\"item5\"></li>\n</ul>"},
		{"ul>li.item$@-3*3", "", "\t", "<ul>\n\t<li class=\"item5\"></li>\n\t<li class=\"item4\"></li>\n\t<li class=\"item3\"></li>\n</ul>"},
		{"h$*3", "", "\t", "<h1></h1>\n<h2></h2>\n<h3></h3>"},
		{"ul>li[title=$]{Item $}*2", "", "\t", "<ul>\n\t<li title=\"1\">Item 1</li>\n\t<li title=\"2\">Item 2</li>\n</ul>"},
		{"ul>li*2>a{$}", "", "\t", "<ul>\n\t<li><a href=\"\">1</a></li>\n\t<li><a href=\"\">2</a></li>\n</ul>"},
		{`p{\$5}`, "", "\t", `<p>$5</p>`},

		// inline-level formatting
		{"div>span", "", "\t", `<div><span></span></div>`},
		{"div>span+em", "", "\t", `<div><span></span><em></em></div>`},
		{"div>span+div", "", "\t", "<div>\n\t<span></span>\n\t<div></div>\n</div>"},
		{"p>a", "", "\t", `<p><a href=""></a></p>`},
		{"span*2", "", "\t", `<span></span><span></span>`},
		{"span*3", "", "\t", "<span></span>\n<span></span>\n<span></span>"},
		{"b+i+u", "", "\t", "<b></b>\n<i></i>\n<u></u>"},
		{"p>b+i+u", "", "\t", "<p>\n\t<b></b>\n\t<i></i>\n\t<u></u>\n</p>"},

		// implicit tag names
		{"ul>.x", "", "\t", "<ul>\n\t<li class=\"x\"></li>\n</ul>"},
		{"table>.row>.cell", "", "\t", "<table>\n\t<tr class=\"row\">\n\t\t<td class=\"cell\"></td>\n\t</tr>\n</table>"},
		{"select>.o", "", "\t", "<select name=\"\" id=\"\">\n\t<option class=\"o\"></option>\n</select>"},
		{"em>.x", "", "\t", `<em><span class="x"></span></em>`},
		{"p>.x", "", "\t", `<p><span class="x"></span></p>`},
		{"tbody>.r", "", "\t", "<tbody>\n\t<tr class=\"r\"></tr>\n</tbody>"},

		// snippets
		{"a", "", "\t", `<a href=""></a>`},
		{"a:link", "", "\t", `<a href="http://"></a>`},
		{"a:mail", "", "\t", `<a href="mailto:"></a>`},
		{"abbr", "", "\t", `<abbr title=""></abbr>`},
		{"base", "", "\t", `<base href="">`},
		{"link", "", "\t", `<link rel="stylesheet" href="">`},
		{"link:css", "", "\t", `<link rel="stylesheet" href="style.css">`},
		{"link:favicon", "", "\t", `<link rel="shortcut icon" href="favicon.ico" type="image/x-icon">`},
		{"link:print", "", "\t", `<link rel="stylesheet" href="print.css" media="print">`},
		{"meta:vp", "", "\t", `<meta name="viewport" content="width=device-width, initial-scale=1.0">`},
		{"meta:utf", "", "\t", `<meta http-equiv="Content-Type" content="text/html;charset=UTF-8">`},
		{"script:src", "", "\t", `<script src=""></script>`},
		{"img", "", "\t", `<img src="" alt="">`},
		{"pic", "", "\t", `<picture></picture>`},
		{"src", "", "\t", `<source>`},
		{"input", "", "\t", `<input type="text">`},
		{"input:email", "", "\t", `<input type="email" name="" id="">`},
		{"input:p", "", "\t", `<input type="password" name="" id="">`},
		{"input:c", "", "\t", `<input type="checkbox" name="" id="">`},
		{"input:h", "", "\t", `<input type="hidden" name="">`},
		{"input:submit", "", "\t", `<input type="submit" value="">`},
		{"input:reset", "", "\t", `<input type="reset" value="">`},
		{"btn", "", "\t", `<button></button>`},
		{"btn:s", "", "\t", `<button type="submit"></button>`},
		{"btn:r", "", "\t", `<button type="reset"></button>`},
		{"btn:d", "", "\t", `<button disabled></button>`},
		{"form:get", "", "\t", `<form action="" method="get"></form>`},
		{"form:post", "", "\t", `<form action="" method="post"></form>`},
		{"select", "", "\t", `<select name="" id=""></select>`},
		{"opt", "", "\t", `<option value=""></option>`},
		{"textarea", "", "\t", `<textarea name="" id="" cols="30" rows="10"></textarea>`},
		{"label", "", "\t", `<label for=""></label>`},
		{"ol+", "", "\t", "<ol>\n\t<li></li>\n</ol>"},
		{"ul+", "", "\t", "<ul>\n\t<li></li>\n</ul>"},
		{"dl+", "", "\t", "<dl>\n\t<dt></dt>\n\t<dd></dd>\n</dl>"},
		{"table+", "", "\t", "<table>\n\t<tr>\n\t\t<td></td>\n\t</tr>\n</table>"},
		{"tr+", "", "\t", "<tr>\n\t<td></td>\n</tr>"},
		{"emb", "", "\t", `<embed src="" type="">`},
		{"obj", "", "\t", `<object data="" type=""></object>`},
		{"ifr", "", "\t", `<iframe src="" frameborder="0"></iframe>`},
		{"video", "", "\t", `<video src=""></video>`},
		{"audio", "", "\t", `<audio src=""></audio>`},
		{"bq", "", "\t", `<blockquote></blockquote>`},
		{"fig>figc", "", "\t", "<figure>\n\t<figcaption></figcaption>\n</figure>"},
		{"br", "", "\t", `<br>`},
		{"hr", "", "\t", `<hr>`},
		{"c", "", "\t", `<!--  -->`},
		{"cc:ie", "", "\t", `<!--[if IE]><![endif]-->`},
		{"a.btn{Go}", "", "\t", `<a href="" class="btn">Go</a>`},
		{"ri:a", "", "\t", "<picture>\n\t<source media=\"(min-width: )\" srcset=\"\">\n\t<img src=\"\" alt=\"\">\n</picture>"},
		{"!", "", "\t", "<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n\t<meta charset=\"UTF-8\">\n\t<meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\">\n\t<title>Document</title>\n</head>\n<body>\n\t\n</body>\n</html>"},
		{"html:5", "", "  ", "<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n  <meta charset=\"UTF-8\">\n  <meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\">\n  <title>Document</title>\n</head>\n<body>\n  \n</body>\n</html>"},
		{"!>p", "", "\t", "<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n\t<meta charset=\"UTF-8\">\n\t<meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\">\n\t<title>Document</title>\n</head>\n<body>\n\t<p></p>\n</body>\n</html>"},

		// boolean attributes and syntaxes
		{"input[disabled]", "", "\t", `<input type="text" disabled>`},
		{"input[disabled]", "xhtml", "\t", `<input type="text" disabled="disabled" />`},
		{"br+img", "xhtml", "\t", "<br /><img src=\"\" alt=\"\" />"},
		{"br", "xml", "\t", `<br/>`},
		{"item/", "xml", "\t", `<item/>`},
		{"catalog>book#b$*2>title", "xml", "  ", "<catalog>\n  <book id=\"b1\">\n    <title></title>\n  </book>\n  <book id=\"b2\">\n    <title></title>\n  </book>\n</catalog>"},
		{".card>h2.title", "jsx", "  ", "<div className=\"card\">\n  <h2 className=\"title\"></h2>\n</div>"},
		{"label.x", "jsx", "\t", `<label htmlFor="" className="x"></label>`},
		{"img", "jsx", "\t", `<img src="" alt="" />`},
		{"input[disabled]", "jsx", "\t", `<input type="text" disabled />`},
		{"button[onClick={handle}]", "jsx", "\t", `<button onClick={handle}></button>`},
		{"Foo.Bar>Baz/", "jsx", "\t", "<Foo.Bar>\n\t<Baz />\n</Foo.Bar>"},

		// filters
		{"ul>li*2|s", "", "\t", `<ul><li></li><li></li></ul>`},
		{"div#page>p.title+p|c", "", "\t", "<div id=\"page\">\n\t<p class=\"title\"></p>\n\t<!-- /.title -->\n\t<p></p>\n</div>\n<!-- /#page -->"},
		{"c>div", "", "\t", `<!-- <div></div> -->`},
		{"c{note}", "", "\t", `<!-- note -->`},

		// lorem
		{"p>lorem5", "", "\t", `<p>Lorem ipsum dolor sit amet.</p>`},
		{"lorem4", "", "\t", `Lorem ipsum dolor sit.`},
	}
	for _, tc := range tests {
		syn := tc.syntax
		if syn == "" {
			syn = "html"
		}
		got, err := Expand(tc.abbr, Options{Syntax: syn, Indent: tc.indent})
		if err != nil {
			t.Errorf("%s (%s): error %v", tc.abbr, syn, err)
			continue
		}
		if got.Text != tc.want {
			t.Errorf("%s (%s):\n got: %q\nwant: %q", tc.abbr, syn, got.Text, tc.want)
		}
	}
}

func TestExpandErrors(t *testing.T) {
	for _, a := range []string{"", "div[x", "p{x", "div>", "a+", "a..b", "3div", "a)", "Hello!"} {
		if got, err := Expand(a, Options{Syntax: "html"}); err == nil {
			t.Errorf("%q: want an error, got %q", a, got.Text)
		}
	}
	if _, err := Expand("div*100000", Options{Syntax: "html"}); err == nil {
		t.Errorf("huge repeat: want an error")
	}
}

func TestLorem(t *testing.T) {
	opt := Options{Syntax: "html", Indent: "\t"}
	a, _ := Expand("lorem", opt)
	b, _ := Expand("lorem", opt)
	if a.Text != b.Text {
		t.Errorf("lorem is not the same each time")
	}
	if !strings.HasPrefix(a.Text, "Lorem ipsum dolor sit amet") || len(strings.Fields(a.Text)) != 30 {
		t.Errorf("lorem: %q", a.Text)
	}
	e, _ := Expand("p*3>lorem5", opt)
	lines := strings.Split(e.Text, "\n")
	if len(lines) != 3 || lines[0] != "<p>Lorem ipsum dolor sit amet.</p>" || strings.Contains(lines[1], "Lorem ipsum") {
		t.Errorf("p*3>lorem5: %q", e.Text)
	}
	for _, l := range lines {
		inner := strings.TrimSuffix(strings.TrimPrefix(l, "<p>"), "</p>")
		if n := len(strings.Fields(inner)); n != 5 {
			t.Errorf("p*3>lorem5: %d words in %q", n, l)
		}
	}
	e, _ = Expand("ul>lorem3*2", opt)
	if !strings.HasPrefix(e.Text, "<ul>\n\t<li>Lorem ipsum dolor.</li>\n\t<li>") {
		t.Errorf("ul>lorem3*2: %q", e.Text)
	}
	e, _ = Expand("lorem10-20", opt)
	if n := len(strings.Fields(e.Text)); n < 10 || n > 20 {
		t.Errorf("lorem10-20 gave %d words", n)
	}
}

func TestStops(t *testing.T) {
	tests := []struct {
		abbr  string
		stops []Range
	}{
		{"div", []Range{{5, 5}}},
		{"a", []Range{{9, 9}, {11, 11}}},
		{"br", nil},
		{"link:css", []Range{{29, 34}}},
		{"input", []Range{{13, 17}}},
		{"a:link", []Range{{16, 16}, {18, 18}}},
		{"c", []Range{{5, 5}}},
		{"p{${1:hello} ${2:world}}", []Range{{3, 8}, {9, 14}}},
		{"p{é}+a", []Range{{18, 18}, {20, 20}}},
		{"div{x}", nil},
	}
	for _, tc := range tests {
		got, err := Expand(tc.abbr, Options{Syntax: "html", Indent: "\t"})
		if err != nil {
			t.Errorf("%s: %v", tc.abbr, err)
			continue
		}
		if !sameRanges(got.Stops, tc.stops) {
			t.Errorf("%s: stops %v, want %v (text %q)", tc.abbr, got.Stops, tc.stops, got.Text)
		}
	}
	// "!": the title text first, then the empty body line
	e, _ := Expand("!", Options{Syntax: "html", Indent: "\t"})
	r := []rune(e.Text)
	if len(e.Stops) != 2 || string(r[e.Stops[0].Start:e.Stops[0].End]) != "Document" ||
		!strings.HasSuffix(string(r[:e.Stops[1].Start]), "<body>\n\t") {
		t.Errorf("!: stops %v", e.Stops)
	}
}

func sameRanges(a, b []Range) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestBaseIndent(t *testing.T) {
	e, err := Expand("ul>li*2", Options{Syntax: "html", Indent: "  ", BaseIndent: "    "})
	if err != nil {
		t.Fatal(err)
	}
	want := "<ul>\n      <li></li>\n      <li></li>\n    </ul>"
	if e.Text != want {
		t.Errorf("got %q, want %q", e.Text, want)
	}
	if len(e.Stops) != 2 || e.Stops[0] != (Range{15, 15}) {
		t.Errorf("stops %v", e.Stops)
	}
	for i, l := range strings.Split(e.Text, "\n") {
		if i > 0 && !strings.HasPrefix(l, "    ") {
			t.Errorf("line %d lacks the base indent: %q", i, l)
		}
	}
	e, _ = Expand("!", Options{Syntax: "html", Indent: "\t", BaseIndent: "\t"})
	for i, l := range strings.Split(e.Text, "\n") {
		if i > 0 && !strings.HasPrefix(l, "\t") {
			t.Errorf("!: line %d lacks the base indent: %q", i, l)
		}
	}
}

func TestTypoFix(t *testing.T) {
	tests := []struct{ abbr, syntax, want, note string }{
		{"dvi", "html", `<div></div>`, "fixed: dvi → div"},
		{"dvi.x*2", "html", "<div class=\"x\"></div>\n<div class=\"x\"></div>", "fixed: dvi → div"},
		{"sapn", "html", `<span></span>`, "fixed: sapn → span"},
		{"secton", "html", `<section></section>`, "fixed: secton → section"},
		{"buton", "html", `<button></button>`, "fixed: buton → button"},
		{"ul>il", "html", "<ul>\n\t<il></il>\n</ul>", ""}, // 2 letters: left alone
		{"dvi", "xml", `<dvi></dvi>`, ""},
		{"Dvi", "jsx", `<Dvi></Dvi>`, ""},
		{"my-dvi", "html", `<my-dvi></my-dvi>`, ""},
		{"from", "html", `<from></from>`, ""},
		{"divs", "html", `<divs></divs>`, ""},
	}
	for _, tc := range tests {
		e, err := Expand(tc.abbr, Options{Syntax: tc.syntax, Indent: "\t"})
		if err != nil {
			t.Errorf("%s: %v", tc.abbr, err)
			continue
		}
		if e.Text != tc.want || e.Note != tc.note {
			t.Errorf("%s (%s): got %q note %q, want %q note %q", tc.abbr, tc.syntax, e.Text, e.Note, tc.want, tc.note)
		}
	}
}

func TestExtractMarkup(t *testing.T) {
	tests := []struct {
		line  string
		col   int // -1 = end of line
		want  string
		start int
		ok    bool
	}{
		{"ul>li", -1, "ul>li", 0, true},
		{"  foo ul>li.a", -1, "ul>li.a", 6, true},
		{"<div>ul>li", -1, "ul>li", 5, true},
		{`<p class="x">a[title="x y"]`, -1, `a[title="x y"]`, 13, true},
		{"text p{hello world}", -1, "p{hello world}", 5, true},
		{"(ul>li)*2", -1, "(ul>li)*2", 0, true},
		{"foo(ul>li", -1, "ul>li", 4, true},
		{"it's ul>li", -1, "ul>li", 5, true},
		{"ul>li|c", -1, "ul>li|c", 0, true},
		{"div.a$@-*3 rest", 10, "div.a$@-*3", 0, true},
		{"a[title='a]b']", -1, "a[title='a]b']", 0, true},
		{`<a href="x`, -1, "", 0, false},
		{`<div cla`, -1, "", 0, false},
		{`x = "ul>li`, -1, "", 0, false},
		{"x  ", -1, "", 0, false},
		{"", -1, "", 0, false},
		{"a]", -1, "", 0, false},
	}
	for _, tc := range tests {
		line := []rune(tc.line)
		col := tc.col
		if col < 0 {
			col = len(line)
		}
		got, start, ok := Extract(line, col, "html")
		if ok != tc.ok || (ok && (got != tc.want || start != tc.start)) {
			t.Errorf("%q@%d: got %q %d %v, want %q %d %v", tc.line, col, got, start, ok, tc.want, tc.start, tc.ok)
		}
	}
}

func TestMarkupTabWorthy(t *testing.T) {
	tests := []struct {
		abbr, syntax string
		want         bool
	}{
		{"ul>li", "html", true},
		{"div", "html", true},
		{"a", "html", true},
		{"lorem", "html", true},
		{"lorem10", "html", true},
		{"!", "html", true},
		{"a:link", "html", true},
		{"ul+", "html", true},
		{".foo", "html", true},
		{"#main", "html", true},
		{"div.x", "html", true},
		{"my-card.x", "html", true},
		{"p*3", "html", true},
		{"foo[bar]", "html", true},
		{"p{hi}", "html", true},
		{"(a)", "html", true},
		{"dvi", "html", true},
		{"ul>li|c", "html", true},
		{"foo", "html", false},
		{"123", "html", false},
		{"3.14", "html", false},
		{"2*3", "html", false},
		{"index.html", "html", false},
		{"Hello!", "html", false},
		{"from", "html", false},
		{"http:", "html", false},
		{"e.g.", "html", false},
		{"", "html", false},
		{"div", "jsx", true},
		{"ul>li", "jsx", true},
		{".row", "jsx", true},
		{"map", "jsx", false},
		{"data", "jsx", false},
		{"title", "jsx", false},
		{"i", "jsx", false},
		{"Foo", "jsx", false},
		{"Foo.x", "jsx", true},
		{"this.props", "jsx", false},
		{"map.get", "jsx", false},
		{"dvi", "xml", false},
		{"foo>bar", "xml", true},
	}
	for _, tc := range tests {
		if got := markupTabWorthy(tc.abbr, tc.syntax); got != tc.want {
			t.Errorf("%q (%s): got %v, want %v", tc.abbr, tc.syntax, got, tc.want)
		}
	}
}

func TestTabExpandMarkup(t *testing.T) {
	line := []rune("    <div>ul>li*2")
	start, e, ok := TabExpand(line, len(line), Options{Syntax: "html", Indent: "\t", BaseIndent: "    "})
	if !ok || start != 9 || e.Text != "<ul>\n    \t<li></li>\n    \t<li></li>\n    </ul>" {
		t.Errorf("got %v %d %q", ok, start, e.Text)
	}
	line = []rune("plain words")
	if _, _, ok := TabExpand(line, len(line), Options{Syntax: "html"}); ok {
		t.Errorf("plain words should not expand")
	}
}

func TestWrap(t *testing.T) {
	tests := []struct {
		abbr, content, indent, base, want string
	}{
		{"div", "hello", "\t", "", `<div>hello</div>`},
		{"ul>li*", "one\ntwo\n\nthree", "\t", "", "<ul>\n\t<li>one</li>\n\t<li>two</li>\n\t<li>three</li>\n</ul>"},
		{"ul>li*|t", "1. one\n2. two", "  ", "", "<ul>\n  <li>one</li>\n  <li>two</li>\n</ul>"},
		{"ol>li*|t", "- one\n* two\n• three", "\t", "", "<ol>\n\t<li>one</li>\n\t<li>two</li>\n\t<li>three</li>\n</ol>"},
		{"nav>ul>li*>a", "Home\nAbout", "\t", "", "<nav>\n\t<ul>\n\t\t<li><a href=\"\">Home</a></li>\n\t\t<li><a href=\"\">About</a></li>\n\t</ul>\n</nav>"},
		{"div.w", "  line1\n    line2\n", "\t", "", "<div class=\"w\">\n\tline1\n\t  line2\n</div>"},
		{"div", "<p>a</p>", "\t", "", "<div>\n\t<p>a</p>\n</div>"},
		{"p>span", "<b>x</b>", "\t", "", `<p><span><b>x</b></span></p>`},
		{"blockquote>p", "a\nb", "  ", "    ", "<blockquote>\n      <p>\n        a\n        b\n      </p>\n    </blockquote>"},
		{"p.x{$#!}", "hi", "\t", "", `<p class="x">hi!</p>`},
		{"ul>li.i$*>{[$#]}", "a\nb", "\t", "", "<ul>\n\t<li class=\"i1\">[a]</li>\n\t<li class=\"i2\">[b]</li>\n</ul>"},
		{"a", "http://example.com", "\t", "", `<a href="http://example.com">http://example.com</a>`},
		{"a", "www.example.com", "\t", "", `<a href="http://www.example.com">www.example.com</a>`},
		{"a", "me@example.com", "\t", "", `<a href="mailto:me@example.com">me@example.com</a>`},
		{"a", "Home", "\t", "", `<a href="">Home</a>`},
		{"c", "note", "\t", "", `<!-- note -->`},
		{"li*", "", "\t", "", `<li></li>`},
	}
	for _, tc := range tests {
		e, err := Wrap(tc.abbr, tc.content, Options{Syntax: "html", Indent: tc.indent, BaseIndent: tc.base})
		if err != nil {
			t.Errorf("%s: %v", tc.abbr, err)
			continue
		}
		if e.Text != tc.want {
			t.Errorf("%s around %q:\n got: %q\nwant: %q", tc.abbr, tc.content, e.Text, tc.want)
		}
	}
	e, _ := Wrap("ul>li*", "a\nb", Options{Syntax: "jsx", Indent: "\t"})
	if e.Text != "<ul>\n\t<li>a</li>\n\t<li>b</li>\n</ul>" {
		t.Errorf("jsx wrap: %q", e.Text)
	}
}

// TestMarkupNoPanic feeds random abbreviation-like text to every entry point.
func TestMarkupNoPanic(t *testing.T) {
	const chars = "ab ldiv>+^*()[]{}#.$@-=\"'|/!:3\\<"
	rng := mkNewRand("no panic")
	for i := 0; i < 20000; i++ {
		b := make([]byte, rng.between(1, 14))
		for j := range b {
			b[j] = chars[rng.between(0, len(chars)-1)]
		}
		s := string(b)
		for _, syn := range []string{"html", "jsx", "xml"} {
			opt := Options{Syntax: syn, Indent: "\t", BaseIndent: " "}
			Expand(s, opt)
			Wrap(s, "x\ny", opt)
			markupTabWorthy(s, syn)
			line := []rune(s)
			for col := 0; col <= len(line); col++ {
				TabExpand(line, col, opt)
			}
		}
	}
}

// A snippet name with nothing after its colon expands to its usual form.
func TestColonDefault(t *testing.T) {
	for abbr, want := range map[string]string{
		"html:":  "<!DOCTYPE html>",
		"link:":  `<link rel="stylesheet" href="">`,
		"input:": `<input type="text">`,
	} {
		l := []rune(abbr)
		_, e, ok := TabExpand(l, len(l), Options{Syntax: "html", Indent: "  "})
		if !ok || !strings.HasPrefix(e.Text, want) {
			t.Errorf("%q: ok %v, got %q, want it to start %q", abbr, ok, e.Text, want)
		}
	}
	l := []rune("foo:")
	if _, _, ok := TabExpand(l, len(l), Options{Syntax: "html"}); ok {
		t.Error("foo: should stay as typed")
	}
}
