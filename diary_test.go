package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// makeDiary writes a diary with the given days and page texts, and opens sy in Diary mode.
func makeDiary(t *testing.T, days map[string][]string) *Model {
	t.Helper()
	m := newTest(t, "") // its own data folder
	for day, pages := range days {
		dir := filepath.Join(diaryDir(), day)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		for i, text := range pages {
			os.WriteFile(filepath.Join(dir, fmt.Sprintf("%03d.txt", i+1)), []byte(text), 0o600)
		}
	}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.setMode(diaryMode)
	return m
}

func diaryAt(t *testing.T, m *Model, day string, page int) {
	t.Helper()
	d := m.diary
	if d.chapters[d.ch].day != day || d.pg != page-1 {
		t.Fatalf("on %s page %d, want %s page %d", d.chapters[d.ch].day, d.pg+1, day, page)
	}
}

func screen(m *Model) string { return ansi.Strip(m.View().Content) }

func TestDiaryPagesAndChapters(t *testing.T) {
	m := makeDiary(t, map[string][]string{
		"2026-10-24": {"first day, page one", "first day, page two"},
		"2026-10-25": {"second day"},
		"2026-10-26": {"third day, one", "third day, two", "third day, three"},
	})
	diaryAt(t, m, "2026-10-26", 3) // opens on the newest page
	if s := screen(m); !strings.Contains(s, "third day, three") || !strings.Contains(s, "page three of three") {
		t.Fatalf("page not shown:\n%s", s)
	}
	press(m, "left")
	diaryAt(t, m, "2026-10-26", 2)
	press(m, "up") // previous chapter, its first page
	diaryAt(t, m, "2026-10-25", 1)
	press(m, "left") // back over the chapter edge: last page of the one before
	diaryAt(t, m, "2026-10-24", 2)
	press(m, "right")
	diaryAt(t, m, "2026-10-25", 1)
	press(m, "down")
	diaryAt(t, m, "2026-10-26", 1)
	press(m, "down") // no chapter after: stays
	diaryAt(t, m, "2026-10-26", 1)
	press(m, "up")
	press(m, "up")
	press(m, "up") // no chapter before: stays
	diaryAt(t, m, "2026-10-24", 1)
	if !strings.Contains(screen(m), "Saturday the twenty-fourth of October") {
		t.Fatalf("chapter title missing:\n%s", screen(m))
	}
}

func TestDiaryNewPageAndWriting(t *testing.T) {
	m := makeDiary(t, nil)
	if !strings.Contains(screen(m), "No pages yet") {
		t.Fatalf("empty diary not shown:\n%s", screen(m))
	}
	press(m, "alt+n")
	if !m.diary.writing {
		t.Fatal("Alt+N should open the new page for writing")
	}
	typeStr(m, "Dear diary")
	press(m, "esc")
	if m.diary.writing {
		t.Fatal("Esc should go back to the pages")
	}
	if s := screen(m); !strings.Contains(s, "Dear diary") || !strings.Contains(s, "page one of one") {
		t.Fatalf("written page not shown:\n%s", s)
	}
	// AltGr+N on a UK keyboard types ” : also a new page
	m.Update(tea.KeyPressMsg(tea.Key{Code: 'n', Text: "”"}))
	if !m.diary.writing || m.diary.pg != 1 {
		t.Fatalf("AltGr+N should make page two and open it (writing %v, page %d)", m.diary.writing, m.diary.pg+1)
	}
	if strings.Contains(string(m.buf.Lines[0]), "”") {
		t.Fatal("AltGr+N typed a ” instead of making a page")
	}
	press(m, "esc")
	for i := range m.tabs {
		if p := m.tabBuf(i).Path; strings.HasPrefix(p, diaryDir()) {
			t.Fatalf("Esc should close the page's tab; still open: %s", p)
		}
	}
	files, _ := os.ReadDir(filepath.Dir(m.diary.page()))
	if len(files) != 2 {
		t.Fatalf("want 2 page files, got %d", len(files))
	}
}

func TestDiaryAllPages(t *testing.T) {
	m := makeDiary(t, map[string][]string{
		"2026-10-25": {"alpha"},
		"2026-10-26": {"beta", "gamma"},
	})
	press(m, "alt+a")
	if !m.diary.all || !strings.Contains(screen(m), "All pages · 3 in 2 chapters") {
		t.Fatalf("all pages not shown:\n%s", screen(m))
	}
	press(m, "up")
	diaryAt(t, m, "2026-10-25", 1)
	m.Update(tea.KeyPressMsg(tea.Key{Code: 'a', Text: "æ"})) // AltGr+A closes it again
	if m.diary.all {
		t.Fatal("AltGr+A should close all pages")
	}
	if !strings.Contains(screen(m), "alpha") {
		t.Fatalf("the page chosen should be in focus:\n%s", screen(m))
	}
}
