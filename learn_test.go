package main

import (
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func pressLearn(m *Model) { m.Update(tea.KeyPressMsg(tea.Key{Code: 'ł', Text: "ł"})) }

func TestLearnOpensAtTheCursor(t *testing.T) {
	for _, c := range []struct {
		line string
		col  int
		use  string
	}{
		{"  font-size: 16px;", 15, "px"},
		{"  font-size: 2rem;", 4, "rem"}, // no line for font-size: the nearest unit
		{"  display: grid;", 12, "display: grid"},
		{".card:has(img) {", 7, ":has(…)"},
		{"  height: 100dvh;", 13, "dvh · svh · lvh"},
		{"  --brand: #f60;", 4, "--brand: oklch(…)"},
		{"  float: left;", 3, "float for layout"},
	} {
		m := newWebTest(t, "a.css", "a {\n"+c.line+"\n}")
		m.buf.Cursor = Pos{1, c.col}
		pressLearn(m)
		if m.learn == nil {
			t.Fatalf("AltGr+L did not open Learn")
		}
		top := learnTopics[m.learn.topic]
		if top.Name != "CSS" {
			t.Fatalf("card = %s, want CSS", top.Name)
		}
		if got := top.Sections[m.learn.sec].Items[m.learn.item].Use; got != c.use {
			t.Fatalf("%q: opened at %q, want %q", c.line, got, c.use)
		}
	}
}

func TestLearnKeysAndScreen(t *testing.T) {
	m := newWebTest(t, "a.css", "a {\n  width: 10px;\n}")
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m.buf.Cursor = Pos{1, 10}
	pressLearn(m)
	screen := ansi.Strip(m.View().Content)
	for _, want := range []string{"L E A R N", "Units: which to use", "▸ px", "What", "Like", "Checked against"} {
		if !strings.Contains(screen, want) {
			t.Fatalf("screen has no %q:\n%s", want, screen)
		}
	}
	press(m, "right")
	if m.learn.sec != 1 || m.learn.item != 0 {
		t.Fatalf("right: section %d item %d", m.learn.sec, m.learn.item)
	}
	press(m, "down")
	if m.learn.item != 1 {
		t.Fatalf("down: item %d", m.learn.item)
	}
	press(m, "tab")
	if m.learn.topic == 0 && len(learnTopics) > 1 {
		t.Fatalf("tab did not change the language")
	}
	press(m, "esc")
	if m.learn != nil {
		t.Fatalf("esc did not close")
	}
	if m.buf.Text() != "a {\n  width: 10px;\n}" {
		t.Fatalf("keys in Learn changed the text: %q", m.buf.Text())
	}
}

// Every card is complete: a source, sections, and every line has all three parts.
func TestLearnCardsComplete(t *testing.T) {
	names := map[string]bool{}
	for _, tp := range learnTopics {
		if tp.Source == "" || len(tp.Sections) == 0 || len(tp.Match) == 0 {
			t.Fatalf("%s: no source, sections or matches", tp.Name)
		}
		if names[tp.Name] {
			t.Fatalf("two cards called %s", tp.Name)
		}
		names[tp.Name] = true
		for _, s := range tp.Sections {
			if s.Title == "" || len(s.Items) == 0 {
				t.Fatalf("%s: empty section %q", tp.Name, s.Title)
			}
			for _, it := range s.Items {
				if it.Use == "" || it.Why == "" || it.Like == "" || it.Example == "" {
					t.Fatalf("%s / %s / %q: missing Use, Why, Like or Example", tp.Name, s.Title, it.Use)
				}
				ex := strings.Split(it.Example, "\n")
				if len(ex) > 6 {
					t.Fatalf("%s / %q: the example has %d lines (6 at most)", tp.Name, it.Use, len(ex))
				}
				for _, l := range ex {
					if utf8.RuneCountInString(l) > 60 {
						t.Fatalf("%s / %q: example line over 60 letters: %q", tp.Name, it.Use, l)
					}
				}
			}
		}
	}
}

// The card follows the file's ending, and the cursor picks the line.
func TestLearnOtherLanguages(t *testing.T) {
	for _, c := range []struct {
		file, line string
		col        int
		card, in   string // the card, and a word the chosen line's Use holds
	}{
		{"a.js", "const n = user?.name;", 14, "JavaScript", "?."},
		{"a.go", "x := 5", 2, "Go", ":="},
		{"a.lua", "if a ~= b then", 5, "Lua", "~="},
		{"a.sh", "d=$(pwd)", 3, "Shell (Bash)", "$("},
		{"a.md", "```go", 1, "Markdown", "```"},
		{"a.html", `<a href="/x">go</a>`, 1, "HTML", "<a"},
		{"a.yaml", "name: no", 7, "JSON / YAML / TOML", `"no"`},
	} {
		m := newWebTest(t, c.file, c.line+"\n")
		m.buf.Cursor = Pos{0, c.col}
		pressLearn(m)
		if m.learn == nil {
			t.Fatalf("%s: Learn did not open", c.file)
		}
		top := learnTopics[m.learn.topic]
		use := top.Sections[m.learn.sec].Items[m.learn.item].Use
		if top.Name != c.card || !strings.Contains(use, c.in) {
			t.Fatalf("%s %q: opened %s / %q, want %s with %q", c.file, c.line, top.Name, use, c.card, c.in)
		}
	}
}
