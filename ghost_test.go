package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestUnitHintTyping(t *testing.T) {
	m := newWebTest(t, "a.css", "a {\n  \n}")
	m.buf.Cursor = Pos{1, 2}
	typeStr(m, "font-size: 16")
	if !strings.Contains(ansi.Strip(m.View().Content), "font-size: 16px → 1rem;") {
		t.Fatalf("no shadow on screen:\n%s", ansi.Strip(m.View().Content))
	}
	if !strings.Contains(m.msg, "1rem") {
		t.Fatalf("message = %q", m.msg)
	}
	press(m, "tab")
	want(t, m, lines("a {", "  font-size: 1rem;", "}"), Pos{1, 17})

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

func TestUnitHintOnValueThere(t *testing.T) {
	m := newWebTest(t, "a.css", "a {\n  font-size: 16px;\n}")
	m.buf.Cursor = Pos{1, 16} // on 16px
	m.Update(nil)
	if h, ok := m.showHint(); !ok || h.New != "1rem" {
		t.Fatalf("hint = %+v %v", h, ok)
	}
	press(m, "tab")
	want(t, m, lines("a {", "  font-size: 1rem;", "}"), Pos{1, 17})
	press(m, "ctrl+z")
	want(t, m, lines("a {", "  font-size: 16px;", "}"), Pos{1, 16})

	// Tab at the start of the line still indents
	m.buf.Cursor = Pos{1, 0}
	press(m, "tab")
	if got := string(m.buf.Lines[1]); got != "      font-size: 16px;" {
		t.Fatalf("tab did not indent: %q", got)
	}
}

func TestNoUnitHintOutsideCSS(t *testing.T) {
	m := newWebTest(t, "a.js", "")
	typeStr(m, "const width = 16")
	if _, ok := m.showHint(); ok {
		t.Fatalf("hint in a JS file")
	}
}
