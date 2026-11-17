package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// padScreen is the screen with a lined page's lines as spaces.
func padScreen(m *Model) string { return strings.ReplaceAll(screenOf(m), "▁", " ") }

// notepadTest is Design mode on a big screen, with no notes yet, and a
// folder for the notes' files.
func notepadTest(t *testing.T) (*Model, string) {
	t.Helper()
	m := newTest(t, "text stays")
	notesCache = nil
	m.Update(tea.WindowSizeMsg{Width: 180, Height: 50})
	m.setMode(designMode)
	return m, t.TempDir()
}

// addNote writes name with text in dir and makes it a note first saved at
// at (a new notepad goes on the board).
func addNote(t *testing.T, m *Model, dir, name, text string, at time.Time) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	nf := notesLoad()
	_, fresh := nf.noteSaved(p, at)
	if err := nf.save(); err != nil {
		t.Fatal(err)
	}
	if fresh > 0 {
		m.placeNotepad(fresh)
	}
	return p
}

// padOnBoard is notepad n's structure on the board shown.
func padOnBoard(t *testing.T, m *Model, n int) *dOct {
	t.Helper()
	for _, o := range m.ds().octs {
		if notepadOf(o) == n {
			return o
		}
	}
	t.Fatalf("notepad %d is not on the board", n)
	return nil
}

// openPad chooses notepad n on the board and presses Enter.
func openPad(t *testing.T, m *Model, n int) *padViewer {
	t.Helper()
	m.ds().sel = padOnBoard(t, m, n).ID
	press(m, "enter")
	if m.padView == nil || m.padView.n != n {
		t.Fatalf("Enter on notepad %d should open it: %+v", n, m.padView)
	}
	return m.padView
}

// The notepad on the board shows its name, how many notes, the newest
// note's title and its stickers.
func TestNotepadFront(t *testing.T) {
	m, dir := notepadTest(t)
	now := time.Now()
	addNote(t, m, dir, "groceries.txt", "milk\neggs", now.Add(-time.Hour))
	addNote(t, m, dir, "ideas.txt", "a lamp that hums", now.Add(-30*time.Minute))
	p := addNote(t, m, dir, "plans.txt", "\nthe garden first", now)
	notesLoad().note(p).Sticker = "home"
	notesLoad().save()
	s := padScreen(m)
	for _, w := range []string{"Notepad 1", "3/16 notes", "plans", "the garden first", "home"} {
		if !strings.Contains(s, w) {
			t.Fatalf("the notepad should show %q:\n%s", w, s)
		}
	}
	// A new note is drawn at once (the drawing is made again when the notes change).
	addNote(t, m, dir, "zebra.txt", "stripes", now.Add(time.Minute))
	if s := padScreen(m); !strings.Contains(s, "4/16 notes") || !strings.Contains(s, "zebra") {
		t.Fatalf("the fourth note should show:\n%s", s)
	}
	// Zoomed far out: a block with its name.
	for i := 0; i < 6; i++ {
		press(m, "-")
	}
	if s := padScreen(m); strings.Contains(s, "4/16 notes") {
		t.Fatalf("zoomed far out the notepad is a small block:\n%s", s)
	}
}

// Two notepads side by side look different in every part, and their
// drawings differ.
func TestNotepadsLookDifferent(t *testing.T) {
	for n := 1; n < 60; n++ {
		a, b := notepadStyle(n), notepadStyle(n+1)
		if a.Binding == b.Binding || a.Cover == b.Cover || a.Ruling == b.Ruling || a.Pattern == b.Pattern {
			t.Fatalf("notepads %d and %d share a part: %+v / %+v", n, n+1, a, b)
		}
	}
	seen := map[padStyle]int{}
	for n := 1; n <= 1260; n++ {
		s := notepadStyle(n)
		s.Paper, s.Ink, s.Rule, s.Corner = "", "", "", 0
		if k, ok := seen[s]; ok {
			t.Fatalf("notepads %d and %d look the same", k, n)
		}
		seen[s] = n
	}
	newTest(t, "")
	notesCache = nil
	one := ansi.Strip(notepadFront(1, "Pad", notepadW, notepadH, false).Render())
	two := ansi.Strip(notepadFront(2, "Pad", notepadW, notepadH, false).Render())
	if one == two {
		t.Fatalf("two notepads drawn the same:\n%s", one)
	}
}

// The viewer lists the notes by day, newest first, with the chosen one's
// first lines; a missing file says so.
func TestNotepadViewerByDay(t *testing.T) {
	m, dir := notepadTest(t)
	now := time.Now()
	old := now.AddDate(0, 0, -3)
	addNote(t, m, dir, "old.txt", "from before", old)
	addNote(t, m, dir, "yday.txt", "yesterday's words", now.AddDate(0, 0, -1))
	gone := addNote(t, m, dir, "gone.txt", "soon gone", now.Add(-time.Minute))
	addNote(t, m, dir, "fresh.txt", "line one\nline two\nline three", now)
	os.Remove(gone)
	openPad(t, m, 1)
	s := padScreen(m)
	day := old.Format("Mon 2 Jan 2006")
	for _, w := range []string{"Today", "Yesterday", day, "fresh.txt", "yday.txt", "old.txt", "line three", "newest first", "missing"} {
		if !strings.Contains(s, w) {
			t.Fatalf("the open notepad should show %q:\n%s", w, s)
		}
	}
	if !(strings.Index(s, "Today") < strings.Index(s, "Yesterday") && strings.Index(s, "Yesterday") < strings.Index(s, day)) {
		t.Fatalf("newest first: Today, Yesterday, then %s:\n%s", day, s)
	}
	if !strings.Contains(m.designStatus(), "Esc: close") {
		t.Fatalf("the status bar should give the notepad's keys: %q", m.designStatus())
	}
	press(m, "esc")
	if m.padView != nil {
		t.Fatal("Esc should close the notepad")
	}
}

// F gives the chosen note a sticker; / cycles through the stickers, then all.
func TestNotepadStickerAndFilter(t *testing.T) {
	m, dir := notepadTest(t)
	now := time.Now()
	a := addNote(t, m, dir, "a.txt", "aaa", now.Add(-3*time.Minute))
	b := addNote(t, m, dir, "b.txt", "bbb", now.Add(-2*time.Minute))
	c := addNote(t, m, dir, "c.txt", "ccc", now.Add(-time.Minute))
	v := openPad(t, m, 1)
	press(m, "f") // c.txt, the newest, is chosen
	if m.prompt != pPadSticker {
		t.Fatalf("F should ask for a sticker: prompt %d", m.prompt)
	}
	typeStr(m, "work")
	press(m, "enter")
	press(m, "down")
	press(m, "down") // a.txt
	press(m, "f")
	typeStr(m, "home")
	press(m, "enter")
	notesCache = nil
	if r := notesLoad().note(c); r == nil || r.Sticker != "work" {
		t.Fatalf("c.txt's sticker: %+v", r)
	}
	if r := notesLoad().note(a); r == nil || r.Sticker != "home" {
		t.Fatalf("a.txt's sticker: %+v", r)
	}
	if s := padScreen(m); !strings.Contains(s, "work") || !strings.Contains(s, "home") {
		t.Fatalf("the stickers should show:\n%s", s)
	}
	press(m, "/")
	if v.filter != "home" || len(v.shown()) != 1 || v.shown()[0].Path != a {
		t.Fatalf("the first filter: %q, %d notes", v.filter, len(v.shown()))
	}
	if s := padScreen(m); !strings.Contains(s, "showing sticker “home”") || strings.Contains(s, "b.txt") {
		t.Fatalf("only home's notes should show:\n%s", s)
	}
	press(m, "s")
	if v.filter != "work" || len(v.shown()) != 1 || v.shown()[0].Path != c {
		t.Fatalf("the second filter: %q", v.filter)
	}
	press(m, "/")
	if v.filter != "" || len(v.shown()) != 3 {
		t.Fatalf("after the last sticker, all: %q, %d", v.filter, len(v.shown()))
	}
	_ = b
}

// X X takes a note out of its notepad; its file stays.
func TestNotepadRemoveKeepsFile(t *testing.T) {
	m, dir := notepadTest(t)
	now := time.Now()
	keep := addNote(t, m, dir, "keep.txt", "stays", now.Add(-time.Minute))
	drop := addNote(t, m, dir, "drop.txt", "goes out", now)
	openPad(t, m, 1)
	press(m, "x")
	if notesLoad().note(drop) == nil {
		t.Fatal("one X only asks")
	}
	press(m, "x")
	notesCache = nil
	if notesLoad().note(drop) != nil || notesLoad().note(keep) == nil {
		t.Fatal("X X should take out the chosen note only")
	}
	if !fileExists(drop) {
		t.Fatal("the file must stay on disk")
	}
	s := padScreen(m)
	s = s[:strings.LastIndex(strings.TrimRight(s, "\n"), "\n")] // (the message line names it)
	if strings.Contains(s, "drop.txt") || !strings.Contains(s, "1/16 notes") || !strings.Contains(s, "keep.txt") {
		t.Fatalf("the note should be gone from the notepad:\n%s", s)
	}
}

// Enter opens the chosen note in Text mode; PgDn shows the next notepad.
func TestNotepadOpensNote(t *testing.T) {
	m, dir := notepadTest(t)
	now := time.Now().Add(-time.Hour)
	var paths []string
	for i := 1; i <= 17; i++ {
		paths = append(paths, addNote(t, m, dir, "n"+itoa(i)+".txt", "note "+itoa(i), now.Add(time.Duration(i)*time.Minute)))
	}
	v := openPad(t, m, 1)
	press(m, "pgdown")
	if v.n != 2 || len(v.shown()) != 1 {
		t.Fatalf("PgDn: notepad %d with %d notes", v.n, len(v.shown()))
	}
	press(m, "pgup")
	press(m, "down") // n15.txt: newest first
	press(m, "enter")
	if m.mode != 0 || m.buf.Path != paths[14] || !strings.Contains(m.buf.Text(), "note 15") {
		t.Fatalf("Enter should open n15.txt in Text mode: mode %d, %s", m.mode, m.buf.Path)
	}
}
