package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestWrapLine(t *testing.T) {
	cases := []struct {
		line   string
		w      int
		starts []int
		indent int
	}{
		{"aaa bbb ccc", 8, []int{0, 8}, 0},                // breaks after the space
		{"short", 8, []int{0}, 0},                         // fits: one row
		{"abcdefghij", 4, []int{0, 4, 8}, 0},              // one long word breaks where the row ends
		{"    foo bar baz qux", 12, []int{0, 12}, 4},      // carries on at the line's indent
		{"well-known", 6, []int{0, 5}, 0},                 // after a hyphen between letters
		{"aaa      bbb", 5, []int{0, 9}, 0},               // spaces at the break hang past the edge
		{"\tx = aaaa bbbb cccc", 12, []int{0, 10, 15}, 4}, // a tab counts to its tab stop
		{"            ab cd", 12, []int{0, 12}, 0},        // an indent over half the row is not kept
	}
	for _, c := range cases {
		got := wrapLine([]rune(c.line), c.w)
		if !reflect.DeepEqual(got.start, c.starts) || got.indent != c.indent {
			t.Errorf("wrapLine(%q, %d) = %v indent %d, want %v indent %d", c.line, c.w, got.start, got.indent, c.starts, c.indent)
		}
	}
}

func TestWrapOnByDefault(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	long := strings.Repeat("word ", 39) + "last"
	m := newTest(t, "func f() {\n\t"+long+"\n}")
	m.hl.Name = "Go" // code: not centred
	if !m.wrap {
		t.Fatal("word wrap should be on to start with")
	}
	screen := ansi.Strip(m.View().Content)
	if !strings.Contains(screen, "last") || strings.Count(screen, "word") != 39 {
		t.Fatalf("the whole long line should show:\n%s", screen)
	}
	rows := m.rowsOf(1)
	if len(rows.start) != 3 || rows.indent != tabWidth {
		t.Fatalf("line 2 rows = %v indent %d, want 3 rows indented %d", rows.start, rows.indent, tabWidth)
	}
	// Down goes one screen row at a time, keeping the column.
	m.buf.Cursor = Pos{1, 3}
	press(m, "down")
	if m.buf.Cursor.Row != 1 || m.rowAt(1, m.buf.Cursor.Col).k != 1 {
		t.Fatalf("Down should go to the 2nd row of line 2, cursor %v", m.buf.Cursor)
	}
	press(m, "down")
	press(m, "down")
	if m.buf.Cursor.Row != 2 {
		t.Fatalf("three Downs should reach line 3, cursor %v", m.buf.Cursor)
	}
	press(m, "up")
	if m.buf.Cursor.Row != 1 || m.rowAt(1, m.buf.Cursor.Col).k != 2 {
		t.Fatalf("Up should go to the last row of line 2, cursor %v", m.buf.Cursor)
	}
	// The cursor shows on its row.
	c := m.cursor()
	if c == nil || c.Y != textTop+3 {
		t.Fatalf("cursor on screen = %v, want row %d", c, textTop+3)
	}
	// A click on the 2nd row lands in that row.
	p, ok := m.mouseToPos(m.gutterWidth()+tabWidth, textTop+2)
	if !ok || p.Row != 1 || p.Col != rows.start[1] {
		t.Fatalf("click on row 2 = %v, want {1 %d}", p, rows.start[1])
	}
}

func TestWrapScrollsByRows(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	long := strings.Repeat("word ", 200)
	m := newTest(t, strings.Repeat(long+"\n", 20))
	h := m.textHeight()
	m.buf.Cursor = Pos{0, 0}
	for i := 0; i < 3*h; i++ {
		press(m, "down")
	}
	c := m.cursor()
	if c == nil {
		t.Fatalf("cursor went off screen at %v (top %d / %d)", m.buf.Cursor, m.top, m.topSub)
	}
	if m.top == 0 && m.topSub == 0 {
		t.Fatal("the text should have scrolled")
	}
	press(m, "ctrl+home")
	if m.top != 0 || m.topSub != 0 {
		t.Fatalf("Ctrl+Home: top %d / %d", m.top, m.topSub)
	}
}

func TestAltZSwitchesWrap(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := newTest(t, strings.Repeat("x", 300))
	press(m, "alt+z")
	if m.wrap || len(m.rowsOf(0).start) != 1 {
		t.Fatal("Alt+Z should turn word wrap off")
	}
	press(m, "end")
	if m.left == 0 {
		t.Fatal("without wrap, End on a long line should scroll sideways")
	}
	if loadSettings()["wrap"] != "off" {
		t.Fatal("wrap off should be remembered")
	}
	m2, _ := NewModel("", 0)
	if m2.wrap {
		t.Fatal("a new start should keep wrap off")
	}
	press(m, "alt+z")
	if !m.wrap || m.left != 0 || len(m.rowsOf(0).start) < 4 {
		t.Fatalf("Alt+Z again should wrap (left %d, rows %v)", m.left, m.rowsOf(0).start)
	}
}

func TestRightOpensFolder(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, text string) {
		p := filepath.Join(dir, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(text), 0o644)
	}
	write("pics/a.go", "package a\n")
	write("pics/notes.txt", "notes\n")
	write("pics/blank.txt", "  \n\n")
	write("pics/empty.md", "")
	write("pics/logo.svg", "<svg></svg>\n")
	write("pics/photo.png", "\x89PNG\r\n\x1a\n\x00\x00")
	write("pics/sub/deep.py", "x = 1\n")
	tabNames := func(m *Model) string {
		var names []string
		for i := range m.tabs {
			names = append(names, m.tabName(i))
		}
		return strings.Join(names, " ")
	}
	open := func(key string) *Model {
		m, err := NewModel(dir, 0)
		if err != nil {
			t.Fatal(err)
		}
		m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
		m.tree.reveal(filepath.Join(dir, "pics"))
		press(m, key)
		return m
	}

	// Right: every file, the folder's folders too.
	m := open("right")
	if got := tabNames(m); got != "a.go deep.py photo.png blank.txt empty.md notes.txt logo.svg" {
		t.Fatalf("Right opened %q", got)
	}
	if m.showTree || !strings.Contains(m.msg, "Opened 7 files") {
		t.Fatalf("Right: tree %v, message %q", m.showTree, m.msg)
	}
	// A file that is not text is never saved, so its bytes stay.
	m.switchTab(m.findTab(filepath.Join(dir, "pics", "photo.png")))
	if !m.buf.Binary {
		t.Fatal("photo.png should be marked as not text")
	}
	press(m, "x")
	m.saveNow()
	if b, _ := os.ReadFile(filepath.Join(dir, "pics", "photo.png")); string(b) != "\x89PNG\r\n\x1a\n\x00\x00" {
		t.Fatalf("photo.png was changed: %q", b)
	}

	// Shift+Right: only text with something in it, no media.
	m = open("shift+right")
	if got := tabNames(m); got != "a.go deep.py notes.txt" {
		t.Fatalf("Shift+Right opened %q", got)
	}
	if !strings.Contains(m.msg, "Skipped 4 that are media, empty") {
		t.Fatalf("Shift+Right message %q", m.msg)
	}

	// Shift+Right on a picture leaves it; Right opens it.
	m = open("down") // any key: just a model with the tree open
	m.tree.reveal(filepath.Join(dir, "pics", "logo.svg"))
	press(m, "shift+right")
	if m.buf.Path != "" || !strings.Contains(m.msg, "Shift+Right leaves it") {
		t.Fatalf("Shift+Right on logo.svg: opened %q, message %q", m.buf.Path, m.msg)
	}
	press(m, "right")
	if filepath.Base(m.buf.Path) != "logo.svg" {
		t.Fatalf("Right on logo.svg opened %q", m.buf.Path)
	}

	// Enter still opens and closes a folder in the tree.
	m = open("enter")
	if m.tree.rows[m.tree.sel].open != true || len(m.tabs) > 1 {
		t.Fatal("Enter on a folder should open it in the tree, not as tabs")
	}
}
