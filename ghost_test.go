package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestUnitHintTyping(t *testing.T) {
	m := newWebTest(t, "a.css", "a {\n  \n}")
	m.buf.Cursor = Pos{1, 2}
	typeStr(m, "font-size: 1")
	if !strings.Contains(ansi.Strip(m.View().Content), "font-size: 1rem;") {
		t.Fatalf("no shadow on screen:\n%s", ansi.Strip(m.View().Content))
	}
	if !strings.Contains(m.msg, "rem") {
		t.Fatalf("message = %q", m.msg)
	}
	press(m, "tab")
	want(t, m, lines("a {", "  font-size: 1rem;", "}"), Pos{1, 17})

	// a big number gets no hint and no arrow
	m = newWebTest(t, "d.css", "a {\n  \n}")
	m.buf.Cursor = Pos{1, 2}
	typeStr(m, "font-size: 16")
	if _, ok := m.showHint(); ok || strings.Contains(ansi.Strip(m.View().Content), "→") {
		t.Fatalf("hint on a big number:\n%s", ansi.Strip(m.View().Content))
	}

	// a small number gets the unit added
	m = newWebTest(t, "b.css", "a {\n  \n}")
	m.buf.Cursor = Pos{1, 2}
	typeStr(m, "padding: 2")
	press(m, "tab")
	want(t, m, lines("a {", "  padding: 2rem;", "}"), Pos{1, 15})

	// typing the unit yourself works as always
	m = newWebTest(t, "c.css", "a {\n  \n}")
	m.buf.Cursor = Pos{1, 2}
	typeStr(m, "gap: 1em;")
	want(t, m, lines("a {", "  gap: 1em;", "}"), Pos{1, 11})
}

func TestNoUnitHintOnValueThere(t *testing.T) {
	m := newWebTest(t, "a.css", "a {\n  min-height: calc(100vh - 2vh);\n}")
	m.buf.Cursor = Pos{1, 22} // inside 100vh
	m.Update(nil)
	if h, ok := m.showHint(); ok {
		t.Fatalf("hint on a value already there: %+v", h)
	}
	if strings.Contains(ansi.Strip(m.View().Content), "→") {
		t.Fatalf("arrow on screen:\n%s", ansi.Strip(m.View().Content))
	}
}

func TestNoUnitHintOutsideCSS(t *testing.T) {
	m := newWebTest(t, "a.js", "")
	typeStr(m, "const width = 16")
	if _, ok := m.showHint(); ok {
		t.Fatalf("hint in a JS file")
	}
}
