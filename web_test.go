package main

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// newWebTest opens a real file called name (so the language comes from its
// ending), indented with two spaces, cursor at the end.
func newWebTest(t *testing.T, name, text string) *Model {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := NewModel(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.buf.Indent = "  "
	m.buf.Cursor = m.endOfFile()
	return m
}

func lines(s ...string) string { return strings.Join(s, "\n") }

func TestEmmetTabExpandsAndJumps(t *testing.T) {
	m := newWebTest(t, "a.html", "")
	typeStr(m, "ul>li*2")
	press(m, "tab")
	want(t, m, lines("<ul>", "  <li></li>", "  <li></li>", "</ul>"), Pos{1, 6})
	typeStr(m, "one")
	press(m, "tab")
	want(t, m, lines("<ul>", "  <li>one</li>", "  <li></li>", "</ul>"), Pos{2, 6})
	press(m, "ctrl+z")
	if got := m.buf.Text(); got != "ul>li*2" {
		// typing "one" and the expansion are separate undo steps
		press(m, "ctrl+z")
	}
	if got := m.buf.Text(); got != "ul>li*2" {
		t.Fatalf("undo left %q", got)
	}
}

func TestEmmetTabPlainWordStillIndents(t *testing.T) {
	m := newWebTest(t, "a.html", "")
	typeStr(m, "hello")
	press(m, "tab")
	if got := m.buf.Text(); !strings.HasPrefix(got, "hello ") || strings.Contains(got, "<") {
		t.Fatalf("plain word got %q", got)
	}
}

func TestEmmetIndentKept(t *testing.T) {
	m := newWebTest(t, "a.html", "<body>\n  ")
	typeStr(m, "p.note")
	press(m, "tab")
	want(t, m, lines("<body>", `  <p class="note"></p>`), Pos{1, 18})
}

func TestEmmetCSS(t *testing.T) {
	m := newWebTest(t, "a.css", "a {\n  \n}")
	m.buf.Cursor = Pos{1, 2}
	typeStr(m, "m10-20")
	press(m, "tab")
	want(t, m, lines("a {", "  margin: 10px 20px;", "}"), Pos{1, 20})

	// a selector is not a property: Tab indents
	m = newWebTest(t, "b.css", "")
	typeStr(m, ".box")
	press(m, "tab")
	if got := m.buf.Text(); strings.Contains(got, ":") {
		t.Fatalf("selector expanded: %q", got)
	}
}

func TestEmmetStyleBlockInHTML(t *testing.T) {
	m := newWebTest(t, "a.html", "<style>\n  p {\n    \n  }\n</style>")
	m.buf.Cursor = Pos{2, 4}
	typeStr(m, "d:f")
	press(m, "tab")
	want(t, m, lines("<style>", "  p {", "    display: flex;", "  }", "</style>"), Pos{2, 18})
}

func TestCSSAutoCorrect(t *testing.T) {
	m := newWebTest(t, "a.css", "a {\n  \n}")
	m.buf.Cursor = Pos{1, 2}
	typeStr(m, "colour:")
	if !strings.Contains(m.msg, "color") {
		t.Fatalf("message = %q", m.msg)
	}
	typeStr(m, " red;")
	want(t, m, lines("a {", "  color: red;", "}"), Pos{1, 13})

	// Ctrl+Z straight after a fix takes back just the fix
	m = newWebTest(t, "b.css", "a {\n  \n}")
	m.buf.Cursor = Pos{1, 2}
	typeStr(m, "dispaly:")
	want(t, m, lines("a {", "  display:", "}"), Pos{1, 10})
	press(m, "ctrl+z")
	want(t, m, lines("a {", "  dispaly:", "}"), Pos{1, 10})
}

func TestTagClosesItself(t *testing.T) {
	m := newWebTest(t, "a.html", "")
	typeStr(m, "<div>")
	want(t, m, "<div></div>", Pos{0, 5})
	press(m, "enter")
	want(t, m, lines("<div>", "  ", "</div>"), Pos{1, 2})

	m = newWebTest(t, "b.html", "")
	typeStr(m, "<br>")
	want(t, m, "<br>", Pos{0, 4}) // a void tag has no partner

	m = newWebTest(t, "c.html", "<p>hi")
	typeStr(m, "</")
	want(t, m, "<p>hi</p>", Pos{0, 9})
}

func TestTagRenameFollows(t *testing.T) {
	m := newWebTest(t, "a.html", "<div>x</div>")
	m.buf.Cursor = Pos{0, 4}
	typeStr(m, "s")
	want(t, m, "<divs>x</divs>", Pos{0, 5})
	press(m, "backspace")
	press(m, "backspace")
	want(t, m, "<di>x</di>", Pos{0, 3})
}

func TestWrapWithAbbreviation(t *testing.T) {
	m := newWebTest(t, "a.html", "one\ntwo")
	press(m, "ctrl+a")
	press(m, "ctrl+e")
	if m.prompt != pWrap {
		t.Fatal("Ctrl+E with a highlight should ask what to wrap with")
	}
	typeStr(m, "ul>li*")
	press(m, "enter")
	if got := m.buf.Text(); got != lines("<ul>", "  <li>one</li>", "  <li>two</li>", "</ul>") {
		t.Fatalf("wrap gave %q", got)
	}
}

func TestEmmetActionsOnKeys(t *testing.T) {
	m := newWebTest(t, "a.html", "<div><p>hi</p></div>")
	m.buf.Cursor = Pos{0, 9}
	press(m, "alt+u")
	if got := m.buf.SelectedText(); got != "hi" {
		t.Fatalf("balance out 1 = %q", got)
	}
	press(m, "alt+u")
	if got := m.buf.SelectedText(); got != "<p>hi</p>" {
		t.Fatalf("balance out 2 = %q", got)
	}
	press(m, "esc")
	m.buf.Cursor = Pos{0, 9}
	press(m, "alt+r")
	want(t, m, "<div>hi</div>", m.buf.Cursor)

	m = newWebTest(t, "b.html", "width: 10*3+2")
	press(m, "alt+x")
	want(t, m, "width: 32", Pos{0, 9})
	press(m, "alt+=")
	if got := m.buf.Text(); got != "width: 33" {
		t.Fatalf("number up = %q", got)
	}
}

func TestPathAutoCorrect(t *testing.T) {
	m := newWebTest(t, "a.html", "")
	dir := filepath.Dir(m.buf.Path)
	os.MkdirAll(filepath.Join(dir, "images"), 0o755)
	f, _ := os.Create(filepath.Join(dir, "images", "logo.png"))
	png.Encode(f, image.NewRGBA(image.Rect(0, 0, 3, 2)))
	f.Close()
	typeStr(m, `<img src="Images/Logo.PNG"`)
	if got := m.buf.Text(); got != `<img src="images/logo.png"` {
		t.Fatalf("path = %q", got)
	}
	typeStr(m, ">")
	m.buf.Cursor = Pos{0, 12}
	press(m, "alt+i")
	if got := m.buf.Text(); !strings.Contains(got, `width="3"`) || !strings.Contains(got, `height="2"`) {
		t.Fatalf("image size = %q", got)
	}
}

func TestEmmetOffInOtherFiles(t *testing.T) {
	m := newWebTest(t, "a.go", "")
	typeStr(m, "ul>li")
	press(m, "tab")
	if got := m.buf.Text(); strings.Contains(got, "<") {
		t.Fatalf("Go file expanded: %q", got)
	}
	typeStr(m, "x := \"<div>\"")
	if strings.Contains(m.buf.Text(), "</div>") {
		t.Fatal("tags closed in a Go file")
	}
}
