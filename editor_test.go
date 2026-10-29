package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// TestMain keeps every test away from the real home folder: sessions and
// rescue files go to a throwaway folder.
func TestMain(m *testing.M) {
	dir, _ := os.MkdirTemp("", "sy-test-home-")
	os.Setenv("HOME", dir)
	os.Setenv("XDG_DATA_HOME", filepath.Join(dir, "data"))
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	os.Setenv("SY_NO_NETWORK", "1") // never fetch the weather in tests
	os.Setenv("PATH", "")           // no wl-copy, gio ...: tests never touch the desktop
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// press sends one key, written the way Bubble Tea names it.
func press(m *Model, name string) {
	k := tea.Key{}
	parts := strings.Split(name, "+")
	for _, p := range parts[:len(parts)-1] {
		switch p {
		case "ctrl":
			k.Mod |= tea.ModCtrl
		case "alt":
			k.Mod |= tea.ModAlt
		case "shift":
			k.Mod |= tea.ModShift
		}
	}
	codes := map[string]rune{"up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft,
		"right": tea.KeyRight, "enter": tea.KeyEnter, "tab": tea.KeyTab, "esc": tea.KeyEscape,
		"backspace": tea.KeyBackspace, "delete": tea.KeyDelete, "home": tea.KeyHome,
		"end": tea.KeyEnd, "f1": tea.KeyF1, "f2": tea.KeyF2, "f3": tea.KeyF3, "f5": tea.KeyF5,
		"f7": tea.KeyF7, "pgup": tea.KeyPgUp, "pgdown": tea.KeyPgDown, "space": tea.KeySpace}
	last := parts[len(parts)-1]
	if c, ok := codes[last]; ok {
		k.Code = c
	} else {
		k.Code = []rune(last)[0]
	}
	m.Update(tea.KeyPressMsg(k))
}

func release(m *Model, name string) {
	codes := map[string]rune{"up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft, "right": tea.KeyRight}
	m.Update(tea.KeyReleaseMsg(tea.Key{Code: codes[name], Mod: tea.ModAlt}))
}

func typeStr(m *Model, s string) {
	for _, r := range s {
		if r == '\n' {
			press(m, "enter")
			continue
		}
		m.Update(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
	}
}

func newTest(t *testing.T, text string) *Model {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir()) // its own sessions: never another test's
	m, err := NewModel("", 0)
	if err != nil {
		t.Fatal(err)
	}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if text != "" {
		m.buf.Insert(text, "")
		m.buf.Cursor = Pos{}
		m.buf.undo = nil
	}
	return m
}

func want(t *testing.T, m *Model, text string, cur Pos) {
	t.Helper()
	if got := m.buf.Text(); got != text {
		t.Fatalf("text = %q, want %q", got, text)
	}
	if m.buf.Cursor != cur {
		t.Fatalf("cursor = %v, want %v", m.buf.Cursor, cur)
	}
}

func TestTypingUndoRedo(t *testing.T) {
	m := newTest(t, "")
	typeStr(m, "hello world")
	want(t, m, "hello world", Pos{0, 11})
	press(m, "ctrl+z")
	want(t, m, "hello ", Pos{0, 6})
	press(m, "ctrl+z")
	want(t, m, "", Pos{0, 0})
	press(m, "ctrl+y")
	press(m, "ctrl+y")
	want(t, m, "hello world", Pos{0, 11})
}

func TestAutoIndent(t *testing.T) {
	m := newTest(t, "")
	m.hl.Name = "Python"
	typeStr(m, "def f():\nreturn 1")
	want(t, m, "def f():\n    return 1", Pos{1, 12})
	press(m, "enter")
	want(t, m, "def f():\n    return 1\n    ", Pos{2, 4})
	press(m, "backspace")
	want(t, m, "def f():\n    return 1\n", Pos{2, 0})
}

func TestNavigationKeys(t *testing.T) {
	m := newTest(t, "one two\nthree\nfour five six")
	m.buf.Cursor = Pos{0, 3}
	press(m, "end")
	want(t, m, m.buf.Text(), Pos{0, 7})
	press(m, "alt+b")
	want(t, m, m.buf.Text(), Pos{0, 0})
	press(m, "alt+e")
	want(t, m, m.buf.Text(), Pos{2, 13})
	press(m, "alt+s")
	want(t, m, m.buf.Text(), Pos{0, 0})
	press(m, "ctrl+e") // does nothing now
	want(t, m, m.buf.Text(), Pos{0, 0})
	press(m, "ctrl+end")
	want(t, m, m.buf.Text(), Pos{2, 13})
	press(m, "ctrl+home")
	want(t, m, m.buf.Text(), Pos{0, 0})
}

func TestAltArrowsMoveText(t *testing.T) {
	m := newTest(t, "one two three\nfour\nfive")
	m.buf.Cursor = Pos{0, 1} // in "one"
	press(m, "alt+right")
	want(t, m, "two one three\nfour\nfive", Pos{0, 5})
	press(m, "alt+right")
	want(t, m, "two three one\nfour\nfive", Pos{0, 11})
	press(m, "alt+right") // last on the line: stays
	want(t, m, "two three one\nfour\nfive", Pos{0, 11})
	press(m, "alt+left")
	want(t, m, "two one three\nfour\nfive", Pos{0, 5})
	press(m, "ctrl+z")
	want(t, m, "two three one\nfour\nfive", Pos{0, 11})

	// highlighted text moves one letter at a time
	m = newTest(t, "abcdef")
	m.buf.Cursor = Pos{0, 1}
	press(m, "ctrl+shift+right")
	press(m, "ctrl+shift+right") // "bc" highlighted
	press(m, "alt+right")
	want(t, m, "adbcef", Pos{0, 4})
	if got := m.buf.SelectedText(); got != "bc" {
		t.Fatalf("still highlighted: %q", got)
	}
	press(m, "alt+left")
	press(m, "alt+left")
	want(t, m, "bcadef", Pos{0, 2})

	// whole words: a name with _ moves as one, and so does an operator
	m = newTest(t, "x = MAX_PARTS + 1")
	m.buf.Cursor = Pos{0, 13} // just after MAX_PARTS
	press(m, "alt+left")
	want(t, m, "x MAX_PARTS = + 1", Pos{0, 11})

	// Alt + Up / Down still move the line
	m = newTest(t, "one\ntwo\nthree")
	m.buf.Cursor = Pos{1, 1}
	press(m, "alt+down")
	want(t, m, "one\nthree\ntwo", Pos{2, 1})
	press(m, "alt+up")
	press(m, "alt+up")
	want(t, m, "two\none\nthree", Pos{0, 1})
}

func TestAltUpDownMovesHighlight(t *testing.T) {
	// part of a line highlighted: only that text moves, in its column
	m := newTest(t, "aaaa bbbb\ncccc dddd eeee")
	m.buf.Cursor = Pos{1, 5}
	press(m, "shift+right") // "dddd"
	press(m, "alt+up")
	want(t, m, "aaaa ddddbbbb\ncccc  eeee", Pos{0, 9})
	if got := m.buf.SelectedText(); got != "dddd" {
		t.Fatalf("still highlighted: %q", got)
	}
	press(m, "alt+down")
	want(t, m, "aaaa bbbb\ncccc dddd eeee", Pos{1, 9})
	press(m, "ctrl+z") // one undo per move
	want(t, m, "aaaa ddddbbbb\ncccc  eeee", Pos{0, 9})
	press(m, "ctrl+z")
	want(t, m, "aaaa bbbb\ncccc dddd eeee", Pos{1, 9})

	// a short line gets spaces, and loses them when the text moves on
	m = newTest(t, "ab\none two three\nx")
	m.buf.Cursor = Pos{1, 8}
	press(m, "shift+right") // "three"
	press(m, "alt+up")
	want(t, m, "ab      three\none two\nx", Pos{0, 13})
	press(m, "alt+down")
	press(m, "alt+down")
	want(t, m, "ab\none two\nx       three", Pos{2, 13})
	press(m, "alt+down") // last line: stays
	want(t, m, "ab\none two\nx       three", Pos{2, 13})

	// moved onto a blank line and back: the padding is not an indent
	m = newTest(t, "one two\n\nend")
	m.buf.Cursor = Pos{0, 4}
	press(m, "shift+right") // "two"
	press(m, "alt+down")
	want(t, m, "one\n    two\nend", Pos{1, 7})
	press(m, "alt+up")
	want(t, m, "one two\n\nend", Pos{0, 7})

	// whole line highlighted: the line moves
	m = newTest(t, "one\n  two\nthree")
	m.buf.Cursor = Pos{1, 0}
	press(m, "shift+end")
	press(m, "alt+up")
	want(t, m, "  two\none\nthree", Pos{0, 5})
	// several lines from their indent: the lines move
	m = newTest(t, "one\n  two\n  three\nfour")
	m.buf.Cursor = Pos{1, 2}
	press(m, "shift+down")
	press(m, "shift+end")
	press(m, "alt+down")
	want(t, m, "one\nfour\n  two\n  three", Pos{3, 7})
}

func TestCtrlArrowsAlone(t *testing.T) {
	m := newTest(t, "one two\nthree\nfour five six")
	m.buf.Cursor = Pos{1, 2}
	press(m, "ctrl+right")
	want(t, m, m.buf.Text(), Pos{1, 3})
	time.Sleep(repeatWindow + 20*time.Millisecond)
	press(m, "ctrl+left")
	press(m, "ctrl+left")
	want(t, m, m.buf.Text(), Pos{1, 1})
	time.Sleep(repeatWindow + 20*time.Millisecond)
	press(m, "ctrl+down")
	want(t, m, m.buf.Text(), Pos{2, 1})
	time.Sleep(repeatWindow + 20*time.Millisecond)
	press(m, "ctrl+up")
	press(m, "ctrl+up")
	want(t, m, m.buf.Text(), Pos{0, 1})
}

// Terminals that do not report key release: two arrows within the window.
func TestCtrlDiagonalByTiming(t *testing.T) {
	m := newTest(t, "abcdef\nabcdef\nabcdef")
	m.buf.Cursor = Pos{1, 3}
	press(m, "ctrl+left")
	press(m, "ctrl+up") // within 150 ms: one diagonal
	want(t, m, "abcdef\nabcdef\nabcdef", Pos{0, 2})
	press(m, "ctrl+up") // still the pair: up the top edge, one more letter left
	want(t, m, "abcdef\nabcdef\nabcdef", Pos{0, 1})

	m.buf.Cursor = Pos{1, 0}
	time.Sleep(repeatWindow + 20*time.Millisecond)
	press(m, "ctrl+right")
	press(m, "ctrl+down")
	want(t, m, "abcdef\nabcdef\nabcdef", Pos{2, 1})
}

// kitty reports key release, so held keys are known exactly.
func TestCtrlDiagonalHeld(t *testing.T) {
	m := newTest(t, "a0cdef\nb1cdef\nc2cdef\nd3cdef")
	m.releases = true
	m.buf.Cursor = Pos{3, 4}
	press(m, "ctrl+left")
	time.Sleep(300 * time.Millisecond) // slow second key: still held, so diagonal
	press(m, "ctrl+up")
	want(t, m, m.buf.Text(), Pos{2, 3})
	press(m, "ctrl+up") // key repeat while both held
	want(t, m, m.buf.Text(), Pos{1, 2})
	release(m, "up")
	release(m, "left")
	press(m, "ctrl+up") // alone again: just up a line
	want(t, m, m.buf.Text(), Pos{0, 2})
}

func TestKeyModeWord(t *testing.T) {
	m := newTest(t, "abc")
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 24}) // room for the whole word
	m.releases = true
	bar := func() string { return ansi.Strip(m.statusBar()) }
	if !strings.Contains(bar(), "text") {
		t.Fatalf("no key mode in the status bar: %q", bar())
	}
	for _, c := range []struct {
		code rune
		word string
	}{{tea.KeyLeftShift, "highlighter"}, {tea.KeyLeftCtrl, "position"}, {tea.KeyLeftAlt, "movement"}} {
		m.Update(tea.KeyPressMsg(tea.Key{Code: c.code}))
		if !strings.Contains(bar(), c.word) {
			t.Fatalf("holding the key did not show %q: %q", c.word, bar())
		}
		m.Update(tea.KeyReleaseMsg(tea.Key{Code: c.code}))
		if strings.Contains(bar(), c.word) {
			t.Fatalf("%q stayed after the key was let go", c.word)
		}
	}
	// Right Alt is AltGr: it types letters, so it is not movement
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRightAlt}))
	if strings.Contains(bar(), "movement") {
		t.Fatal("AltGr counted as Alt")
	}
	m.Update(tea.KeyReleaseMsg(tea.Key{Code: tea.KeyRightAlt}))
	// terminals without key-up: the word follows the last key
	m.releases = false
	press(m, "shift+left")
	if !strings.Contains(bar(), "highlighter") {
		t.Fatal("Shift+Left did not show highlighter")
	}
	press(m, "left")
	if !strings.Contains(bar(), "text") {
		t.Fatal("a plain key did not go back to text")
	}
}

func TestSelectWordsAndLetters(t *testing.T) {
	m := newTest(t, "alpha beta gamma")
	press(m, "shift+right")
	if got := m.buf.SelectedText(); got != "alpha" {
		t.Fatalf("shift+right selected %q", got)
	}
	press(m, "shift+right")
	if got := m.buf.SelectedText(); got != "alpha beta" {
		t.Fatalf("second shift+right selected %q", got)
	}
	press(m, "esc")
	m.buf.Cursor = Pos{0, 6}
	m.buf.Anchor = &Pos{0, 6}
	m.buf.Cursor = Pos{0, 7} // "b" highlighted
	typeStr(m, "B")
	want(t, m, "alpha Beta gamma", Pos{0, 7})
}

func TestArrowsMoveByWordCtrlArrowsByCharacter(t *testing.T) {
	m := newTest(t, "x += foo(bar)  baz")
	stops := []int{1, 4, 8, 9, 12, 13, 18}
	for _, c := range stops {
		press(m, "right")
		want(t, m, m.buf.Text(), Pos{0, c})
		if m.buf.SelectedText() != "" {
			t.Fatal("Right must not highlight")
		}
	}
	// Going left it stops at the start of each word or symbol run.
	for _, c := range []int{15, 12, 9, 8, 5, 2, 0} {
		press(m, "left")
		want(t, m, m.buf.Text(), Pos{0, c})
	}
	// Ctrl+Left / Right move one character, no highlight.
	press(m, "ctrl+right")
	press(m, "ctrl+right")
	want(t, m, m.buf.Text(), Pos{0, 2})
	press(m, "ctrl+left")
	want(t, m, m.buf.Text(), Pos{0, 1})
	if m.buf.SelectedText() != "" {
		t.Fatal("Ctrl+Left/Right must not highlight")
	}
	// Ctrl+Shift+Left / Right highlight one character at a time.
	press(m, "ctrl+shift+right")
	press(m, "ctrl+shift+right")
	if got := m.buf.SelectedText(); got != " +" {
		t.Fatalf("Ctrl+Shift+Right x2 highlighted %q", got)
	}
	press(m, "ctrl+shift+left")
	if got := m.buf.SelectedText(); got != " " {
		t.Fatalf("Ctrl+Shift+Left took back to %q", got)
	}
	// Shift+Left / Right highlight in the same steps the arrows move.
	press(m, "esc")
	m.buf.Cursor = Pos{0, 0}
	for _, want := range []string{"x", "x +=", "x += foo", "x += foo("} {
		press(m, "shift+right")
		if got := m.buf.SelectedText(); got != want {
			t.Fatalf("Shift+Right highlighted %q, want %q", got, want)
		}
	}
	press(m, "shift+left")
	if got := m.buf.SelectedText(); got != "x += foo" {
		t.Fatalf("Shift+Left took back to %q", got)
	}
}

func TestCopyPasteLine(t *testing.T) {
	m := newTest(t, "first\nsecond")
	press(m, "ctrl+c") // nothing selected: whole line
	if m.clip != "first\n" || !m.clipLine {
		t.Fatalf("clip = %q", m.clip)
	}
	m.buf.Cursor = Pos{1, 3}
	m.Update(pasteMsg("first\n"))
	want(t, m, "first\nfirst\nsecond", Pos{2, 3})
	// kitty's Ctrl+V arrives as a bracketed paste
	m.buf.Cursor = Pos{2, 6}
	m.Update(tea.PasteMsg{Content: " pasted"})
	want(t, m, "first\nfirst\nsecond pasted", Pos{2, 13})
}

func TestCut(t *testing.T) {
	m := newTest(t, "a\nb\nc")
	m.buf.Cursor = Pos{1, 0}
	press(m, "ctrl+x")
	want(t, m, "a\nc", Pos{1, 0})
	press(m, "ctrl+z")
	want(t, m, "a\nb\nc", Pos{1, 0})
}

func TestMoveLines(t *testing.T) {
	m := newTest(t, "1\n2\n3")
	press(m, "alt+down")
	want(t, m, "2\n1\n3", Pos{1, 0})
	press(m, "alt+down")
	want(t, m, "2\n3\n1", Pos{2, 0})
	press(m, "alt+up")
	want(t, m, "2\n1\n3", Pos{1, 0})
	press(m, "ctrl+z")
	press(m, "ctrl+z")
	press(m, "ctrl+z")
	want(t, m, "1\n2\n3", Pos{0, 0})
}

func TestMoveTextSideways(t *testing.T) {
	m := newTest(t, "abcd")
	m.buf.Cursor = Pos{0, 1}
	m.buf.Anchor = &Pos{0, 1}
	m.buf.Cursor = Pos{0, 2} // "b" highlighted
	press(m, "alt+shift+right")
	want(t, m, "acbd", Pos{0, 3})
	if m.buf.SelectedText() != "b" {
		t.Fatalf("selection lost: %q", m.buf.SelectedText())
	}
}

func TestFindAndReplace(t *testing.T) {
	m := newTest(t, "cat dog\nCat cat")
	press(m, "ctrl+f")
	typeStr(m, "cat")
	if s, _, _ := m.buf.Selection(); s != (Pos{0, 0}) {
		t.Fatalf("first match at %v", s)
	}
	press(m, "enter") // close; keep the match
	press(m, "f3")
	if s, _, _ := m.buf.Selection(); s != (Pos{1, 0}) {
		t.Fatalf("F3 went to %v (small letters should match Cat)", s)
	}
	press(m, "ctrl+r")
	m.input.SetValue("cat")
	press(m, "enter")
	m.input.SetValue("fox")
	press(m, "enter")
	typeStr(m, "a")
	want(t, m, "fox dog\nfox fox", m.buf.Cursor)
}

func TestGotoAndDuplicateAndComment(t *testing.T) {
	m := newTest(t, "x = 1\ny = 2\nz = 3")
	m.hl.Name = "Python"
	press(m, "ctrl+g")
	typeStr(m, "2")
	press(m, "enter")
	want(t, m, m.buf.Text(), Pos{1, 0})
	press(m, "ctrl+d")
	want(t, m, "x = 1\ny = 2\ny = 2\nz = 3", Pos{2, 0})
	press(m, "ctrl+/")
	want(t, m, "x = 1\ny = 2\n# y = 2\nz = 3", Pos{2, 0})
	press(m, "ctrl+/")
	want(t, m, "x = 1\ny = 2\ny = 2\nz = 3", Pos{2, 0})
	press(m, "ctrl+k")
	want(t, m, "x = 1\ny = 2\nz = 3", Pos{2, 0})
}

func TestIndentSelection(t *testing.T) {
	m := newTest(t, "a\nb")
	press(m, "shift+down")
	press(m, "shift+right")
	press(m, "alt+tab")
	want(t, m, "    a\n    b", Pos{1, 5})
	press(m, "alt+shift+tab")
	want(t, m, "a\nb", Pos{1, 1})
	press(m, "alt+tab")
	press(m, "shift+tab")
	want(t, m, "a\nb", Pos{1, 1})
}

func TestHighlightRuns(t *testing.T) {
	h := NewHighlighter("x.py", "")
	lines := tokenise(h.lexer, "def f(x):\n    return 'hi'  # note")
	if len(lines) < 2 || len(lines[0]) == 0 {
		t.Fatalf("no colours: %v", lines)
	}
	if lines[0][0].Style != sKeyword {
		t.Fatalf("def coloured %d, want keyword", lines[0][0].Style)
	}
	found := map[uint8]bool{}
	for _, s := range lines[1] {
		found[s.Style] = true
	}
	if !found[sString] || !found[sComment] {
		t.Fatalf("line 2 styles %v: want string and comment", lines[1])
	}
}

func TestViewDraws(t *testing.T) {
	m := newTest(t, "func main() {\n\tprintln(\"hi\")\n}")
	m.buf.Cursor = Pos{0, 12}
	v := m.View()
	if plain := ansi.Strip(v.Content); !strings.Contains(plain, "S Y M B O L I Z E R") || !strings.Contains(plain, "println") {
		t.Fatal("view is missing the top panel or the text")
	}
	if v.Cursor == nil || v.Cursor.Y != textTop {
		t.Fatalf("cursor %+v", v.Cursor)
	}
	press(m, "f1")
	if !strings.Contains(ansi.Strip(m.View().Content), "try me") {
		t.Fatal("help page missing")
	}
}

func TestCtrlHeldLongThenUp(t *testing.T) {
	m := newTest(t, "abcdef\nabcdef")
	m.releases = true
	m.buf.Cursor = Pos{1, 4}
	press(m, "ctrl+left")
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft, Mod: tea.ModCtrl, IsRepeat: true}))
	press(m, "ctrl+up")
	want(t, m, m.buf.Text(), Pos{0, 1})
}

// runSave does what Bubble Tea would: the save tick, then the background write.
func runSave(m *Model) {
	m.Update(saveTick{})
	if cmd := m.startSave(); cmd != nil {
		m.Update(cmd())
	}
}

func TestAutosaveNamedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	os.WriteFile(path, []byte("disk\r\ntext\r\n"), 0o640)
	m, _ := NewModel(path, 2)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	typeStr(m, "more ")
	if m.saveWaiting != true {
		t.Fatal("a change did not ask for a save")
	}
	runSave(m)
	data, _ := os.ReadFile(path)
	if string(data) != "disk\r\nmore text\r\n" {
		t.Fatalf("saved %q (Windows line endings must stay)", data)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o640 {
		t.Fatalf("permissions changed to %v", info.Mode().Perm())
	}
	if m.buf.Modified() {
		t.Fatal("still marked as changed after saving")
	}
}

func TestAutosavePausesWhenAnotherProgramChangesTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shared.txt")
	os.WriteFile(path, []byte("mine\n"), 0o644)
	m, _ := NewModel(path, 0)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	os.WriteFile(path, []byte("theirs\n"), 0o644)
	later := time.Now().Add(2 * time.Second)
	os.Chtimes(path, later, later)
	typeStr(m, "x")
	runSave(m)
	if data, _ := os.ReadFile(path); string(data) != "theirs\n" {
		t.Fatalf("overwrote the other program's text: %q", data)
	}
	if !m.savePaused {
		t.Fatal("saving did not pause")
	}
	press(m, "alt+l") // the list: first row = this file from disk
	press(m, "enter")
	want(t, m, "theirs", m.buf.Cursor)
	if m.savePaused {
		t.Fatal("still paused after loading their version")
	}
}

func TestQuitSavesFirst(t *testing.T) {
	path := filepath.Join(t.TempDir(), "q.txt")
	m, _ := NewModel(path, 0)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	typeStr(m, "typed just before quitting")
	press(m, "alt+q") // no save tick has run yet
	if !m.quitClean || m.prompt != pNone {
		t.Fatal("Alt+Q should quit at once: the text is saved first")
	}
	if data, _ := os.ReadFile(path); string(data) != "typed just before quitting\n" {
		t.Fatalf("file holds %q", data)
	}
}

func TestSessions(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	m, _ := NewModel("", 0)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if !m.buf.Draft || m.buf.Path != "" {
		t.Fatal("sy with no file should start a session")
	}
	typeStr(m, "first idea")
	runSave(m)
	first := m.buf.Path
	if !isSession(first) || !fileExists(first) {
		t.Fatalf("session not saved: %q", first)
	}
	// sy again the same day: carries on with it
	m2, _ := NewModel("", 0)
	want(t, m2, "first idea", Pos{0, 10})
	// 25 hours later: concluded, so a new session starts
	old := time.Now().Add(-25 * time.Hour)
	os.Chtimes(first, old, old)
	m3, _ := NewModel("", 0)
	m3.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	want(t, m3, "", Pos{0, 0})
	// the old one is still in the Alt+L list and can be written to
	press(m3, "alt+l")
	if m3.picker == nil || len(m3.picker.items) != 1 {
		t.Fatal("Alt+L does not list the concluded session")
	}
	if !strings.Contains(ansi.Strip(m3.View().Content), "concluded") {
		t.Fatal("the list does not say concluded")
	}
	press(m3, "enter")
	want(t, m3, "first idea", Pos{0, 10})
	typeStr(m3, " and more")
	runSave(m3)
	if data, _ := os.ReadFile(first); string(data) != "first idea and more\n" {
		t.Fatalf("writing to the concluded session: %q", data)
	}
	// written today, so it is open again
	if s, ok := currentSession(); !ok || s.Path != first {
		t.Fatal("a concluded session written to again should be open again")
	}
}

func TestCtrlSNamesSessionWithDetectedEnding(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir := t.TempDir()
	m, _ := NewModel("", 0)
	m.startDir = dir
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	typeStr(m, "#include <iostream>\nint main() {\nstd::cout << 1;\n}")
	runSave(m)
	session := m.buf.Path
	if m.hl.Name != "C++" {
		t.Fatalf("session coloured as %q, want C++", m.hl.Name)
	}
	press(m, "ctrl+s")
	if m.prompt != pName || !strings.Contains(m.input.Prompt, ".cpp") {
		t.Fatalf("Alt+S prompt = %q", m.input.Prompt)
	}
	typeStr(m, "hello")
	press(m, "enter")
	named := filepath.Join(dir, "hello.cpp")
	data, err := os.ReadFile(named)
	if err != nil || !strings.Contains(string(data), "std::cout") {
		t.Fatalf("hello.cpp: %v %q", err, data)
	}
	if m.buf.Draft || m.buf.Path != named || fileExists(session) {
		t.Fatal("the session should have moved into hello.cpp")
	}
	// After naming, Ctrl+S just saves.
	typeStr(m, "\n// done")
	press(m, "ctrl+s")
	if data, _ := os.ReadFile(named); !strings.HasSuffix(string(data), "// done\n") {
		t.Fatalf("Alt+S after naming: %q", data)
	}
}

func TestTabs(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.txt"), filepath.Join(dir, "b.go")
	os.WriteFile(a, []byte("aaa\n"), 0o644)
	os.WriteFile(b, []byte("package b\n"), 0o644)
	m, _ := NewModel(a, 0)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.openPath(b)
	if len(m.tabs) != 2 || m.buf.Path != b {
		t.Fatal("opening a second file should add a tab")
	}
	typeStr(m, "// edit ")
	press(m, "ctrl+pgup") // switching saves the tab being left
	if m.buf.Path != a {
		t.Fatal("Ctrl+PgUp did not go to the first tab")
	}
	if data, _ := os.ReadFile(b); !strings.HasPrefix(string(data), "// edit package b") {
		t.Fatalf("b.go not saved on switching: %q", data)
	}
	press(m, "alt+2")
	if m.buf.Path != b || m.hl.Name != "Go" {
		t.Fatal("Alt+2 did not show b.go")
	}
	m.openPath(a) // already open: just shows it
	if len(m.tabs) != 2 || m.buf.Path != a {
		t.Fatal("opening an open file should show its tab")
	}
	press(m, "ctrl+w")
	if len(m.tabs) != 1 || m.buf.Path != b {
		t.Fatal("Ctrl+W did not close the tab")
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "b.go") {
		t.Fatal("tab bar does not show the file")
	}
}

func TestFileManager(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "src"), 0o755)
	os.WriteFile(filepath.Join(dir, "src", "main.go"), []byte("package main\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# hi\n"), 0o644)
	os.WriteFile(filepath.Join(dir, ".secret"), []byte("x"), 0o644)
	m, err := NewModel(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if !m.showTree || !m.treeFocus || m.tree.root != dir {
		t.Fatal("sy FOLDER should open the folder as a project")
	}
	screen := ansi.Strip(m.View().Content)
	if !strings.Contains(screen, "src/") || !strings.Contains(screen, "README.md") || strings.Contains(screen, ".secret") {
		t.Fatalf("tree shows the wrong files:\n%s", screen)
	}
	// type to find, Enter opens
	typeStr(m, "mai")
	press(m, "enter")
	if m.buf.Path != filepath.Join(dir, "src", "main.go") || m.treeFocus {
		t.Fatalf("finding main.go opened %q", m.buf.Path)
	}
	// back to the tree; new file in the project
	press(m, "alt+o")
	if !m.treeFocus {
		t.Fatal("Alt+O did not go to the file manager")
	}
	press(m, "home")
	press(m, "ctrl+n")
	typeStr(m, "notes.txt")
	press(m, "enter")
	if !fileExists(filepath.Join(dir, "notes.txt")) || filepath.Base(m.buf.Path) != "notes.txt" {
		t.Fatal("new file was not made and opened")
	}
	// rename it
	press(m, "ctrl+o")
	m.tree.reveal(filepath.Join(dir, "notes.txt"))
	press(m, "f2")
	m.input.SetValue("todo.txt")
	press(m, "enter")
	if !fileExists(filepath.Join(dir, "todo.txt")) || filepath.Base(m.buf.Path) != "todo.txt" {
		t.Fatal("rename did not rename the file and its tab")
	}
	// new folder
	press(m, "f7")
	typeStr(m, "docs")
	press(m, "enter")
	if info, err := os.Stat(filepath.Join(dir, "docs")); err != nil || !info.IsDir() {
		t.Fatal("F7 did not make the folder")
	}
	// Delete with no Trash tool (PATH is empty in tests): nothing is removed
	m.tree.reveal(filepath.Join(dir, "todo.txt"))
	press(m, "delete")
	typeStr(m, "y")
	if !fileExists(filepath.Join(dir, "todo.txt")) {
		t.Fatal("deleted a file without a Trash tool")
	}
	// Left on the top row goes up a folder
	press(m, "home")
	press(m, "left")
	if m.tree.root != filepath.Dir(dir) {
		t.Fatalf("Left on top did not go up: %s", m.tree.root)
	}
	// Ctrl+B hides it
	press(m, "ctrl+b")
	if m.showTree {
		t.Fatal("Ctrl+B did not hide the file manager")
	}
}

func TestOpenFolderAsTabs(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, text string) {
		p := filepath.Join(dir, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(text), 0o644)
	}
	write("sub/deep.txt", "deep\n")
	write("sub/inner/notes.txt", "inner\n")
	write("notes.txt", "top\n")
	write("b.go", "package b\n")
	write("A.md", "# a\n")
	write(".hidden", "x\n")
	write(".git/config", "x\n")
	write("node_modules/x.js", "x\n")
	write("pic.png", "\x89PNG\r\n\x1a\n\x00\x00")
	write("latin1.txt", "caf\xe9\n")
	os.Symlink(dir, filepath.Join(dir, "sub", "loop"))
	m, err := NewModel(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	press(m, "shift+enter") // on the top row: the project folder itself
	var names []string
	for i := range m.tabs {
		names = append(names, m.tabName(i))
	}
	// Code before notes; within a kind, own files first, then subfolders.
	// One name twice shows its folder.
	want := "b.go A.md " + filepath.Base(dir) + "/notes.txt deep.txt inner/notes.txt"
	if strings.Join(names, " ") != want {
		t.Fatalf("tabs = %v, want %s", names, want)
	}
	if m.cur != 0 || m.showTree {
		t.Fatalf("should show the first tab with the file manager closed (cur %d, tree %v)", m.cur, m.showTree)
	}
	if !strings.Contains(m.msg, "Opened 5 files") || !strings.Contains(m.msg, "Skipped 2") {
		t.Fatalf("message = %q", m.msg)
	}
	// Again on a file inside sub: its files are open already, so it shows them.
	press(m, "alt+o")
	m.tree.reveal(filepath.Join(dir, "sub", "deep.txt"))
	press(m, "ctrl+enter")
	if len(m.tabs) != 5 || filepath.Base(m.buf.Path) != "deep.txt" {
		t.Fatalf("second folder: %d tabs, showing %s", len(m.tabs), m.buf.Path)
	}
	// A folder with nothing to open says so and leaves the tabs alone.
	empty := filepath.Join(dir, "sub", "empty")
	os.MkdirAll(empty, 0o755)
	if m.openFolder(empty) || len(m.tabs) != 5 {
		t.Fatal("an empty folder should open no tabs")
	}
}

func TestAltGrPMakesProject(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	m, err := NewModel(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	m.tree.reveal(filepath.Join(dir, "sub"))
	m.Update(tea.KeyPressMsg(tea.Key{Code: 'þ', Text: "þ"})) // AltGr+P on a UK keyboard
	if m.tree.root != filepath.Join(dir, "sub") || m.tree.query != "" {
		t.Fatalf("AltGr+P: project %s, find %q", m.tree.root, m.tree.query)
	}
	m.altGr.down = true // kitty says AltGr is held and sends a plain p
	m.tree.reveal(filepath.Join(dir, "sub"))
	press(m, "p")
	m.altGr.down = false
	if m.tree.query != "" {
		t.Fatalf("AltGr held + p typed %q into find", m.tree.query)
	}
}

func TestOpenFolderCodeFirst(t *testing.T) {
	dir := t.TempDir()
	files := []string{"data.json", "go.sum", "notes.md", "app.ts", "Makefile", "main.go", "style.css",
		"index.html", "config.yaml", "run.sh", "util.js", "package-lock.json", "weird.xyz"}
	for _, f := range files {
		os.WriteFile(filepath.Join(dir, f), []byte("x\n"), 0o644)
	}
	os.MkdirAll(filepath.Join(dir, "web"), 0o755)
	os.WriteFile(filepath.Join(dir, "web", "about.html"), []byte("x\n"), 0o644)
	m, err := NewModel(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	m.openFolder(dir)
	var names []string
	for i := range m.tabs {
		names = append(names, m.tabName(i))
	}
	want := "index.html about.html app.ts util.js style.css main.go run.sh Makefile weird.xyz " +
		"notes.md config.yaml data.json go.sum package-lock.json"
	if strings.Join(names, " ") != want {
		t.Fatalf("order:\n got %s\nwant %s", strings.Join(names, " "), want)
	}
}

func TestCodeKinds(t *testing.T) {
	// Each ending and name is in one kind only.
	seen := map[string]int{}
	for rank, k := range codeKinds {
		for _, w := range append(strings.Fields(k.endings), strings.Fields(k.names)...) {
			if was, ok := seen[w]; ok && was != rank {
				t.Errorf("%q is in kind %d and kind %d", w, was, rank)
			}
			seen[w] = rank
		}
	}
	// One file of each kind, in order.
	order := []string{"index.html", "App.tsx", "theme.scss", "main.rs", "build.sh", "Dockerfile",
		"thing.unknown", "notes.md", "app.yaml", "icon.svg", "people.csv", "tsconfig.json", "Cargo.lock"}
	for i := 1; i < len(order); i++ {
		if codeRank(order[i-1]) >= codeRank(order[i]) {
			t.Errorf("%s (%d) should come before %s (%d)", order[i-1], codeRank(order[i-1]), order[i], codeRank(order[i]))
		}
	}
	// These go with the first of the pair.
	for _, pair := range [][2]string{{"index.html", "page.svelte"}, {"main.rs", "Main.java"},
		{"Dockerfile", "Cargo.toml"}, {"notes.md", "README"}, {"Cargo.lock", "package-lock.json"}} {
		if codeRank(pair[0]) != codeRank(pair[1]) {
			t.Errorf("%s and %s should be the same kind", pair[0], pair[1])
		}
	}
}

func TestOpenFolderStopsAtLimit(t *testing.T) {
	dir := t.TempDir()
	for i := range maxFolderTabs + 5 {
		os.WriteFile(filepath.Join(dir, "f"+strconv.Itoa(100+i)+".txt"), []byte("x\n"), 0o644)
	}
	m, err := NewModel(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !m.openFolder(dir) || len(m.tabs) != maxFolderTabs || !strings.Contains(m.msg, "Stopped at") {
		t.Fatalf("%d tabs, message %q", len(m.tabs), m.msg)
	}
}

func TestCtrlSSavesAltSStartAltQQuits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keys.txt")
	m, _ := NewModel(path, 0)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	typeStr(m, "abc")
	press(m, "alt+s") // start of file, not save
	press(m, "ctrl+q")
	if fileExists(path) || m.quitClean || m.prompt != pNone || m.buf.Cursor != (Pos{}) {
		t.Fatal("Alt+S should only go to the start; Ctrl+Q should do nothing")
	}
	press(m, "ctrl+s")
	if data, _ := os.ReadFile(path); string(data) != "abc\n" {
		t.Fatalf("Ctrl+S did not save: %q", data)
	}
	press(m, "alt+q")
	if !m.quitClean {
		t.Fatal("Alt+Q did not quit")
	}
}

func TestAltOAltL(t *testing.T) {
	m := newTest(t, "text")
	press(m, "alt+o")
	if !m.showTree || !m.treeFocus {
		t.Fatal("Alt+O did not open the file manager")
	}
	press(m, "alt+o")
	if !m.showTree || !m.treeFocus {
		t.Fatal("Alt+O in the file manager should keep it open")
	}
	press(m, "alt+e")
	if m.showTree || m.treeFocus {
		t.Fatal("Alt+E did not exit the file manager")
	}
	typeStr(m, " more")
	runSave(m) // makes a session, so the load list has a row
	press(m, "alt+l")
	if m.picker == nil {
		t.Fatal("Alt+L did not open the load list")
	}
	press(m, "alt+l")
	if m.picker != nil {
		t.Fatal("Alt+L again should close the load list")
	}
	press(m, "ctrl+o") // the old keys still work
	if !m.showTree {
		t.Fatal("Ctrl+O should open the file manager")
	}
	press(m, "esc") // Esc closes the floating file manager
	if m.showTree {
		t.Fatal("Esc should close the file manager")
	}
	press(m, "ctrl+l")
	if m.picker == nil {
		t.Fatal("Ctrl+L should still open the load list")
	}
}

// UK keyboard: Right Alt+S types ß, Right Alt+O types ø. They are commands.
func TestEszettSavesOSlashOpens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "uk.txt")
	m, _ := NewModel(path, 0)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	typeStr(m, "abc")
	typeStr(m, "ß") // arrives as typed text, like a real keyboard sends it
	if data, _ := os.ReadFile(path); string(data) != "abc\n" {
		t.Fatalf("ß did not save (or was typed): %q", data)
	}
	typeStr(m, "ø")
	if !m.showTree || !m.treeFocus {
		t.Fatal("ø did not open the file manager")
	}
	typeStr(m, "ø")
	if !m.treeFocus {
		t.Fatal("ø in the file manager should keep it open")
	}
	// AltGr+E types a plain "e" on a UK keyboard; with AltGr held it exits
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyIsoLevel3Shift}))
	m.Update(tea.KeyPressMsg(tea.Key{Code: 'e', Text: "e"}))
	m.Update(tea.KeyReleaseMsg(tea.Key{Code: tea.KeyIsoLevel3Shift}))
	if m.showTree || m.treeFocus {
		t.Fatal("AltGr+E did not exit the file manager")
	}
	want(t, m, "abc", Pos{0, 3})
}

func TestTabMode(t *testing.T) {
	dir := t.TempDir()
	var paths []string
	for _, n := range []string{"a.txt", "b.txt", "c.txt"} {
		p := filepath.Join(dir, n)
		os.WriteFile(p, []byte(n+"\n"), 0o644)
		paths = append(paths, p)
	}
	m, _ := NewModel(paths[0], 0)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.openPath(paths[1])
	m.openPath(paths[2])
	at := func(want int) {
		t.Helper()
		if m.cur != want {
			t.Fatalf("on tab %d, want %d", m.cur+1, want+1)
		}
	}
	altGrTab := func() {
		m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyIsoLevel3Shift}))
		press(m, "tab")
		m.Update(tea.KeyReleaseMsg(tea.Key{Code: tea.KeyIsoLevel3Shift}))
	}
	// Tab is a real Tab now.
	m.switchTab(1)
	press(m, "home")
	press(m, "tab")
	if m.buf.Text() != "    b.txt" || m.cur != 1 {
		t.Fatalf("Tab should indent: %q on tab %d", m.buf.Text(), m.cur+1)
	}
	press(m, "shift+tab")
	// AltGr+Tab, then arrows: the file on screen follows; Enter keeps it.
	altGrTab()
	if !m.tabMode || !strings.Contains(ansi.Strip(m.View().Content), "tabs") {
		t.Fatal("AltGr+Tab did not open tab mode (or the status bar does not say tabs)")
	}
	press(m, "right")
	at(2)
	press(m, "right") // round the end
	at(0)
	press(m, "left")
	at(2)
	press(m, "enter")
	if m.tabMode {
		t.Fatal("Enter did not leave tab mode")
	}
	at(2)
	press(m, "left") // the cursor again, not tabs
	at(2)
	// Esc goes back to where it started.
	altGrTab()
	press(m, "home")
	at(0)
	press(m, "esc")
	at(2)
	// Any other key leaves the mode and does its job.
	altGrTab()
	press(m, "left")
	typeStr(m, "x")
	if m.tabMode || !strings.HasPrefix(m.buf.Text(), "x") || m.cur != 1 {
		t.Fatalf("typing in tab mode: mode %v, text %q, tab %d", m.tabMode, m.buf.Text(), m.cur+1)
	}
	// Hyper+Tab (sent by the Hyprland helper) works like AltGr+Tab.
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModHyper}))
	if !m.tabMode {
		t.Fatal("Hyper+Tab did not open tab mode")
	}
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModHyper}))
	if m.tabMode {
		t.Fatal("Hyper+Tab again did not close tab mode")
	}
	// AltGr+Tab with one file open says so.
	m2 := newTest(t, "x")
	altGr := func() { m2.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyIsoLevel3Shift})) }
	altGr()
	press(m2, "tab")
	if m2.tabMode || !strings.Contains(m2.msg, "Only one file") {
		t.Fatalf("one file: mode %v, message %q", m2.tabMode, m2.msg)
	}
}

func TestReopenLast(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(dir, "data"))
	var paths []string
	for _, n := range []string{"a.txt", "b go.txt", "c.txt"} {
		p := filepath.Join(dir, n)
		os.WriteFile(p, []byte("one\ntwo\nthree\n"), 0o644)
		paths = append(paths, p)
	}
	// Nothing saved yet: sy starts as it always has.
	m0 := newTest(t, "")
	if m0.reopenLast() {
		t.Fatal("reopened with nothing saved")
	}
	m, _ := NewModel(dir, 0) // a project
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	for _, p := range paths {
		m.openPath(p)
	}
	m.switchTab(1)
	m.buf.Cursor = Pos{2, 3}
	m.saveLastOpen()
	saved, _ := os.ReadFile(lastOpenPath())
	if want := "project " + dir + "\nfile 0 0 0 " + paths[0] + "\nfile 2 3 0 " + paths[1] + "\nfile 0 0 0 " + paths[2] +
		"\ncurrent " + paths[1] + "\n"; string(saved) != want {
		t.Fatalf("last-open file:\n%s\nwant:\n%s", saved, want)
	}
	os.Remove(paths[2]) // gone by next time

	m2, _ := NewModel("", 0)
	m2.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if !m2.reopenLast() {
		t.Fatal("did not reopen")
	}
	var names []string
	for i := range m2.tabs {
		names = append(names, m2.tabName(i))
	}
	if strings.Join(names, ",") != "a.txt,b go.txt" {
		t.Fatalf("tabs = %v, want a.txt and b go.txt (c.txt is gone, no blank tab)", names)
	}
	if m2.buf.Path != paths[1] || m2.buf.Cursor != (Pos{2, 3}) {
		t.Fatalf("on %s at %v, want b go.txt at line 3 col 4", m2.buf.Path, m2.buf.Cursor)
	}
	if m2.tree == nil || m2.tree.root != dir || m2.showTree {
		t.Fatal("the project should be set again, with the file manager closed")
	}
	if !strings.Contains(m2.msg, "Opened again") {
		t.Fatalf("message = %q", m2.msg)
	}
}

func TestTabModeRows(t *testing.T) {
	m := newTest(t, "x")
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 24})
	for range 11 {
		m.addTab(newDraft())
	}
	rows := m.tabLayout()
	if len(rows) < 2 {
		t.Fatalf("12 tabs at 60 columns should need more than one row, got %d", len(rows))
	}
	for _, row := range rows {
		last := row[len(row)-1]
		if last.x+last.w > 60 {
			t.Fatalf("a tab row is wider than the window: ends at %d", last.x+last.w)
		}
	}
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyIsoLevel3Shift}))
	press(m, "tab")
	m.switchTab(0)
	press(m, "down")
	if r, _ := tabCellAt(rows, m.cur); r != 1 {
		t.Fatalf("Down went to row %d, want 1", r)
	}
	press(m, "up")
	if m.cur != 0 {
		t.Fatalf("Up came back to tab %d, want 1", m.cur+1)
	}
	press(m, "up") // no row above: stays
	if m.cur != 0 || !m.tabMode {
		t.Fatal("Up on the first row should stay put in tab mode")
	}
	screen := strings.Split(ansi.Strip(m.View().Content), "\n")
	if !strings.Contains(screen[1], " 1 ") || !strings.Contains(screen[2], strconv.Itoa(rows[1][0].i+1)+" ") {
		t.Fatalf("the tab rows are not drawn:\n%s\n%s", screen[1], screen[2])
	}
	// A click on a tab in the second row chooses it and leaves the mode.
	cell := rows[1][0]
	m.Update(tea.MouseClickMsg(tea.Mouse{X: cell.x + 1, Y: 2, Button: tea.MouseLeft}))
	if m.cur != cell.i || m.tabMode {
		t.Fatalf("click: tab %d, mode %v", m.cur+1, m.tabMode)
	}
}

func TestModes(t *testing.T) {
	m := newTest(t, "text stays")
	press(m, "alt+m")
	if !m.modesOpen {
		t.Fatal("Alt+M did not open the modes box")
	}
	screen := ansi.Strip(m.View().Content)
	for _, w := range []string{"M O D E S", "Text", "Design", "Slides", "Diary", "Book", "Draw", "empty for now", "● now"} {
		if !strings.Contains(screen, w) {
			t.Fatalf("modes box is missing %q", w)
		}
	}
	press(m, "right")
	press(m, "enter")
	if m.mode != 1 || m.modesOpen {
		t.Fatal("Enter did not switch to Design")
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "Design mode") {
		t.Fatal("Design mode screen not shown")
	}
	typeStr(m, "xyz") // an empty mode ignores typing
	want(t, m, "text stays", m.buf.Cursor)
	press(m, "alt+m")
	m.Update(tea.KeyPressMsg(tea.Key{Code: '1', Text: "1"}))
	if m.mode != 0 {
		t.Fatal("1 did not switch back to Text")
	}
	press(m, "alt+m")
	press(m, "esc")
	if m.modesOpen {
		t.Fatal("Esc did not close the modes box")
	}
}

func TestModesBoxFits(t *testing.T) {
	for _, size := range [][2]int{{120, 40}, {80, 24}, {60, 24}, {50, 16}, {40, 14}} {
		m := newTest(t, "x")
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		press(m, "alt+m")
		box := m.modesBox()
		if lipgloss.Height(box) > size[1]-2 || lipgloss.Width(box) > size[0] {
			t.Errorf("%dx%d: box is %dx%d, too big", size[0], size[1], lipgloss.Width(box), lipgloss.Height(box))
		}
	}
	// arrows move through the tiles
	m := newTest(t, "x")
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	press(m, "alt+m")
	press(m, "down") // next row of 3
	if m.modeSel != 3 {
		t.Fatalf("Down went to tile %d, want 4", m.modeSel+1)
	}
	press(m, "left")
	if m.modeSel != 2 {
		t.Fatalf("Left went to tile %d, want 3", m.modeSel+1)
	}
}

func TestThemeSwitcher(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	defer applyTheme(0)
	m := newTest(t, "x")
	press(m, "alt+t")
	if !m.themeOpen {
		t.Fatal("Alt+T did not open the colour switcher")
	}
	press(m, "right") // shows Emberline at once
	if currentTheme != 1 || cBG != themes[1].BG {
		t.Fatal("moving did not show the theme straight away")
	}
	press(m, "esc") // puts the old one back
	if currentTheme != 0 || cBG != themes[0].BG || m.themeOpen {
		t.Fatal("Esc did not put the old colours back")
	}
	typeStr(m, "ŧ") // Right Alt+T on a UK keyboard arrives as the letter ŧ
	if !m.themeOpen {
		t.Fatal("ŧ did not open the colour switcher")
	}
	screen := ansi.Strip(m.View().Content)
	for _, n := range []string{"C O L O U R S", "Emberistal", "Molten", "Ash", "Cinder", "Daybreak"} {
		if !strings.Contains(screen, n) {
			t.Fatalf("switcher is missing %q", n)
		}
	}
	m.Update(tea.KeyPressMsg(tea.Key{Code: '3', Text: "3"}))
	press(m, "enter")
	if currentTheme != 2 || loadSettings()["theme"] != "Molten" {
		t.Fatal("Enter did not keep and remember Molten")
	}
	want(t, m, "x", m.buf.Cursor) // the text is untouched
	applyTheme(0)
	NewModel("", 0) // a new sy starts with the remembered theme
	if currentTheme != 2 {
		t.Fatal("the theme was not remembered")
	}
}

func TestInfoLine(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".cache", "emberline"), 0o755)
	os.WriteFile(filepath.Join(home, ".cache", "emberline", "weather.json"), []byte(`{
		"latitude": 51.5, "longitude": -0.13, "_fetched": 1,
		"current": {"temperature_2m": 12.2, "apparent_temperature": 7.6, "relative_humidity_2m": 75,
			"is_day": 0, "weather_code": 61, "wind_speed_10m": 16.6, "wind_direction_10m": 266, "wind_gusts_10m": 35.8},
		"daily": {"temperature_2m_max": [13.4], "temperature_2m_min": [8.6], "precipitation_probability_max": [98]},
		"hourly": {"time": [], "precipitation_probability": []}}`), 0o644)
	m := newTest(t, "x")
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 20})
	m.Update(loadWeather()())
	if m.weather == nil {
		t.Fatal("weather not read from the shared cache")
	}
	lines := strings.Split(ansi.Strip(m.View().Content), "\n")
	info := lines[len(lines)-3]
	now := time.Now()
	for _, want := range []string{"Light rain 12°", "feels 8°", "high 13° low 9°", "wind 17 mph W, gusts 36",
		"humidity 75%", strings.ToUpper(now.Weekday().String()), strings.ToUpper(now.Month().String())} {
		if !strings.Contains(info, want) {
			t.Errorf("info line is missing %q:\n%s", want, info)
		}
	}
	if !strings.Contains(info, strings.ToUpper(timeWords(now)[0].text)) {
		t.Errorf("info line has no time in words: %s", info)
	}
	// a narrow window keeps the weather before the date
	m.Update(tea.WindowSizeMsg{Width: 90, Height: 20})
	lines = strings.Split(ansi.Strip(m.View().Content), "\n")
	if info = lines[len(lines)-3]; !strings.Contains(info, "Light rain") {
		t.Errorf("narrow window dropped the weather: %s", info)
	}
}

func TestCentralZone(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := newTest(t, "package main")
	off := false
	m.buf.Centre = &off // this test is about the zone, not centred lines
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 20})
	left, text, right := m.zone()
	if text != defaultZone || left < 40 || right < 40 || left-right > 1 || right-left > 1 {
		t.Fatalf("zone not centred: %d | %d | %d", left, text, right)
	}
	v := m.View()
	if v.Cursor == nil || v.Cursor.X != left+m.gutterWidth() {
		t.Fatalf("cursor at x=%v, want %d (inside the zone)", v.Cursor, left+m.gutterWidth())
	}
	// drag the left handle 10 columns inwards: 20 letters narrower, still centred
	lx, _, _ := m.marginHandles()
	m.Update(tea.MouseClickMsg(tea.Mouse{X: lx, Y: textTop + 3, Button: tea.MouseLeft}))
	if m.dragMargin != "left" {
		t.Fatal("pressing the left handle did not start a drag")
	}
	m.Update(tea.MouseMotionMsg(tea.Mouse{X: lx + 10, Y: textTop + 3, Button: tea.MouseLeft}))
	m.Update(tea.MouseReleaseMsg(tea.Mouse{X: lx + 10, Y: textTop + 3, Button: tea.MouseLeft}))
	l2, t2, r2 := m.zone()
	if nlx, _, _ := m.marginHandles(); nlx < lx+9 || nlx > lx+10 || t2 < defaultZone-21 || t2 > defaultZone-19 || l2-r2 > 1 || r2-l2 > 1 {
		t.Fatalf("after dragging: %d | %d | %d (handle at %d, mouse at %d)", l2, t2, r2, nlx, lx+10)
	}
	if loadSettings()["zone-width"] != strconv.Itoa(t2) {
		t.Fatal("the zone width was not remembered")
	}
	// the right handle works too, outwards
	_, rx, _ := m.marginHandles()
	m.Update(tea.MouseClickMsg(tea.Mouse{X: rx, Y: textTop, Button: tea.MouseLeft}))
	m.Update(tea.MouseMotionMsg(tea.Mouse{X: rx + 5, Y: textTop, Button: tea.MouseLeft}))
	m.Update(tea.MouseReleaseMsg(tea.Mouse{X: rx + 5, Y: textTop, Button: tea.MouseLeft}))
	if _, t3, _ := m.zone(); t3 < t2+9 || t3 > t2+11 {
		t.Fatalf("right handle: zone %d, want about %d", t3, t2+10)
	}
	// clicking text inside the zone still places the cursor
	l3, _, _ := m.zone()
	m.Update(tea.MouseClickMsg(tea.Mouse{X: l3 + m.gutterWidth() + 4, Y: textTop, Button: tea.MouseLeft}))
	m.Update(tea.MouseReleaseMsg(tea.Mouse{X: l3 + m.gutterWidth() + 4, Y: textTop, Button: tea.MouseLeft}))
	want(t, m, "package main", Pos{0, 4})
	// a narrow window has no margins
	m.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	if l, _, r := m.zone(); l != 0 || r != 0 {
		t.Fatalf("narrow window still has margins %d %d", l, r)
	}
}

func TestSessionNames(t *testing.T) {
	cases := map[string]string{
		"package main\n\nfunc greetAll(names []string) {}\n":        "greetall.go",
		"#include <stdio.h>\nint parse_args(int n) { return n; }\n": "parse_args.c",
		"def load_config(path):\n    return path\n":                 "load_config.py",
		"# My Holiday Plans\n\nPack the **bags**.\n":                "my-holiday-plans.md",
		"Dear diary, today the embers glowed.\n":                    "dear-diary-today.txt",
	}
	for text, want := range cases {
		b := NewBuffer()
		b.Draft = true
		b.Insert(text, "")
		if n, e := b.suggestName(); n+e != want {
			t.Errorf("%q -> %q, want %q", text, n+e, want)
		}
	}
	// an empty session: a random ember name that stays put
	b := NewBuffer()
	b.Draft = true
	n1, e1 := b.suggestName()
	n2, _ := b.suggestName()
	if e1 != ".txt" || n1 != n2 || !strings.Contains(n1, "-") {
		t.Fatalf("random name %q%s (then %q)", n1, e1, n2)
	}
	// the tab shows the name, the status bar shows the session
	m := newTest(t, "")
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
	typeStr(m, "def tidy_room():\n    pass")
	lines := strings.Split(ansi.Strip(m.View().Content), "\n")
	if !strings.Contains(lines[1], "tidy_room.py") || !strings.Contains(lines[len(lines)-2], "session") {
		t.Fatalf("tab %q / status %q", lines[1], lines[len(lines)-2])
	}
	// Ctrl+S offers it; Enter on its own takes it
	dir := t.TempDir()
	m.startDir = dir
	press(m, "ctrl+s")
	press(m, "enter")
	if !fileExists(filepath.Join(dir, "tidy_room.py")) {
		t.Fatal("Enter did not save under the suggested name")
	}
}

func TestCentredLines(t *testing.T) {
	m := newTest(t, "short\na much longer line of prose")
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 20})
	if !m.centred() {
		t.Fatal("prose should be centred by default")
	}
	left, text, _ := m.zone()
	o0, o1 := m.lineOffset(0), m.lineOffset(1)
	if o0 != (text-1-5)/2 || o1 != (text-1-27)/2 {
		t.Fatalf("offsets %d %d", o0, o1)
	}
	m.buf.Cursor = Pos{0, 2}
	if v := m.View(); v.Cursor.X != left+m.gutterWidth()+o0+2 {
		t.Fatalf("cursor x %d, want %d", v.Cursor.X, left+m.gutterWidth()+o0+2)
	}
	lines := strings.Split(ansi.Strip(m.View().Content), "\n")
	row := lines[textTop]
	if i := utf8.RuneCountInString(row[:strings.Index(row, "short")]); i != left+m.gutterWidth()+o0 {
		t.Fatalf("'short' drawn at column %d, want %d", i, left+m.gutterWidth()+o0)
	}
	// a click lands on the letter under the mouse
	m.Update(tea.MouseClickMsg(tea.Mouse{X: left + m.gutterWidth() + o1 + 2, Y: textTop + 1, Button: tea.MouseLeft}))
	m.Update(tea.MouseReleaseMsg(tea.Mouse{X: left + m.gutterWidth() + o1 + 2, Y: textTop + 1, Button: tea.MouseLeft}))
	if m.buf.Cursor != (Pos{1, 2}) {
		t.Fatalf("click went to %v", m.buf.Cursor)
	}
	// Alt+J puts them back on the left; code starts on the left
	press(m, "alt+j")
	if m.centred() || m.lineOffset(0) != 0 {
		t.Fatal("Alt+J did not turn centring off")
	}
	c := newTest(t, "")
	c.hl.Name = "Go"
	if c.centred() {
		t.Fatal("code should stay on the left")
	}
}

func TestAltGrDoubleTap(t *testing.T) {
	m := newTest(t, "x")
	altgr := tea.Key{Code: tea.KeyIsoLevel3Shift}
	tap := func() {
		m.Update(tea.KeyPressMsg(altgr))
		m.Update(tea.KeyReleaseMsg(altgr))
	}
	tap()
	if m.showTree {
		t.Fatal("one tap should not open the file manager")
	}
	tap()
	if !m.showTree || !m.treeFocus {
		t.Fatal("two quick taps should open the file manager and give it the keys")
	}
	tap()
	tap()
	if m.showTree {
		t.Fatal("two more taps should close it")
	}
	// AltGr used to type ß (a save, on a UK keyboard): never a tap
	m.Update(tea.KeyPressMsg(altgr))
	m.Update(tea.KeyPressMsg(tea.Key{Code: 's', Text: "ß"}))
	m.Update(tea.KeyReleaseMsg(altgr))
	tap()
	if m.showTree {
		t.Fatal("AltGr+S followed by one tap must not count as a double tap")
	}
	if strings.Contains(m.buf.Text(), "ß") {
		t.Fatal("ß was typed instead of saving")
	}
	// two slow taps do nothing
	time.Sleep(doubleGap + 50*time.Millisecond)
	tap()
	time.Sleep(doubleGap + 50*time.Millisecond)
	tap()
	if m.showTree {
		t.Fatal("slow taps should not open it")
	}
	// a modifier on its own keeps the message line
	m.say("hello")
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyLeftShift}))
	if m.msg != "hello" {
		t.Fatal("pressing Shift alone cleared the message")
	}
}
