package main

import (
	"strings"
	"testing"
)

func TestBracketsCloseThemselves(t *testing.T) {
	m := newWebTest(t, "a.go", "")
	typeStr(m, "f(")
	want(t, m, "f()", Pos{0, 2})
	typeStr(m, "x")
	want(t, m, "f(x)", Pos{0, 3})
	typeStr(m, ")") // steps over the ) that is there
	want(t, m, "f(x)", Pos{0, 4})
	press(m, "ctrl+z")
	if got := m.buf.Text(); got != "" {
		t.Fatalf("one undo should take the typing and its closer back, left %q", got)
	}

	m = newWebTest(t, "b.go", "")
	typeStr(m, `s := "hi"`)
	want(t, m, `s := "hi"`, Pos{0, 9})
	typeStr(m, " + a[")
	want(t, m, `s := "hi" + a[]`, Pos{0, 14})
	press(m, "backspace") // an empty pair goes as one
	want(t, m, `s := "hi" + a`, Pos{0, 13})
}

func TestPairsLeaveWordsAlone(t *testing.T) {
	m := newWebTest(t, "a.go", "x")
	m.buf.Cursor = Pos{0, 0}
	typeStr(m, "(") // in front of a word: no closer
	want(t, m, "(x", Pos{0, 1})

	m = newWebTest(t, "b.py", "# it")
	typeStr(m, "'s") // an apostrophe, not a quote
	want(t, m, "# it's", Pos{0, 6})

	m = newWebTest(t, "c.py", "")
	typeStr(m, `f"`) // a string prefix
	want(t, m, `f""`, Pos{0, 2})

	m = newWebTest(t, "d.py", "")
	typeStr(m, `"""`)
	want(t, m, `""""""`, Pos{0, 3})

	m = newWebTest(t, "e.md", "")
	typeStr(m, "(it's")
	want(t, m, "(it's", Pos{0, 5}) // Markdown is prose: nothing added

	m = newWebTest(t, "f.go", "f(g(x)")
	m.buf.Cursor = Pos{0, 5}
	typeStr(m, ")") // the line is short of a ): type it, do not step over
	want(t, m, "f(g(x))", Pos{0, 6})
}

func TestPairWrapsHighlight(t *testing.T) {
	m := newWebTest(t, "a.js", "let a = b + c")
	a := Pos{0, 8}
	m.buf.Anchor, m.buf.Cursor = &a, Pos{0, 13}
	typeStr(m, "(")
	want(t, m, "let a = (b + c)", Pos{0, 14})
	if s, e, ok := m.buf.Selection(); !ok || m.buf.Get(s, e) != "b + c" {
		t.Fatalf("the wrapped text should stay highlighted")
	}
}

func TestEnterSplitsBrackets(t *testing.T) {
	m := newWebTest(t, "a.go", "func f() ")
	typeStr(m, "{")
	press(m, "enter")
	want(t, m, lines("func f() {", "  ", "}"), Pos{1, 2})
}

func TestCSSColonAndSemicolon(t *testing.T) {
	m := newWebTest(t, "a.css", "a {\n  \n}")
	m.buf.Cursor = Pos{1, 2}
	typeStr(m, "color:")
	want(t, m, lines("a {", "  color:;", "}"), Pos{1, 8})
	typeStr(m, " red;")
	want(t, m, lines("a {", "  color: red;", "}"), Pos{1, 13})

	// a selector's ":" is not a property's
	m = newWebTest(t, "b.css", "")
	typeStr(m, "a:hover ")
	want(t, m, "a:hover ", Pos{0, 8})
	typeStr(m, "{")
	want(t, m, "a:hover {}", Pos{0, 9})
}

func TestSuggestCSS(t *testing.T) {
	m := newWebTest(t, "a.css", "a {\n  \n}")
	m.buf.Cursor = Pos{1, 2}
	typeStr(m, "disp")
	s := m.suggestOn()
	if s == nil || s.items[0] != "display" {
		t.Fatalf("list = %v", s)
	}
	press(m, "tab")
	want(t, m, lines("a {", "  display: ;", "}"), Pos{1, 11})
	s = m.suggestOn()
	if s == nil || s.kind != "value" || s.items[0] != "block" {
		t.Fatalf("values list = %v", s)
	}
	typeStr(m, "fl")
	press(m, "tab")
	want(t, m, lines("a {", "  display: flex;", "}"), Pos{1, 15})
	typeStr(m, ";")
	want(t, m, lines("a {", "  display: flex;", "}"), Pos{1, 16})
	if m.suggestOn() != nil {
		t.Fatalf("list still open")
	}

	// Down chooses, Esc closes and keys work as always again
	m = newWebTest(t, "b.css", "a {\n  \n}")
	m.buf.Cursor = Pos{1, 2}
	typeStr(m, "bor")
	first := m.suggestOn().items[0]
	press(m, "down")
	if s := m.suggestOn(); s == nil || s.sel != 1 || s.items[1] == first {
		t.Fatalf("down did not choose the next")
	}
	press(m, "esc")
	press(m, "down")
	if m.buf.Cursor.Row != 2 {
		t.Fatalf("down after esc should move the cursor, at %v", m.buf.Cursor)
	}
}

func TestSuggestWordsAndTags(t *testing.T) {
	m := newWebTest(t, "a.go", "var counterValue int\n\n")
	typeStr(m, "co")
	s := m.suggestOn()
	if s == nil || s.items[0] != "counterValue" {
		t.Fatalf("list = %v", s)
	}
	press(m, "tab")
	want(t, m, "var counterValue int\ncounterValue", Pos{1, 12})

	m = newWebTest(t, "b.html", "")
	typeStr(m, "<secti")
	if s := m.suggestOn(); s == nil || s.items[0] != "section" {
		t.Fatalf("tag list = %v", s)
	}
	press(m, "tab")
	typeStr(m, ">")
	want(t, m, "<section></section>", Pos{0, 9})

	m = newWebTest(t, "c.txt", "elephant\n\n")
	typeStr(m, "el")
	if m.suggestOn() != nil {
		t.Fatalf("plain text should have no list")
	}
}

func TestSuggestDrawn(t *testing.T) {
	m := newWebTest(t, "a.css", "a {\n  \n}")
	m.buf.Cursor = Pos{1, 2}
	typeStr(m, "backg")
	screen := m.View().Content
	if !strings.Contains(screen, "background-color") {
		t.Fatalf("the list is not on screen")
	}
}
