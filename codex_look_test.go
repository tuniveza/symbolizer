package main

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// openLook makes a codex with a text on page 1 and a second page, open for editing.
func openLook(t *testing.T) (*Model, *codexState) {
	t.Helper()
	m := codexTest(t)
	m.Update(tea.WindowSizeMsg{Width: 150, Height: 46})
	id, cf, _ := createCodex("Looks")
	cf.Pages[0].Layout = 0
	cf.Pages[0].Els = []*codexEl{{ID: 1, Kind: ceText, Text: "Hello there"}}
	cf.Pages = append(cf.Pages, &codexPage{Layout: 0, Els: []*codexEl{{ID: 2, Kind: ceText, Text: "Second page"}}})
	cf.NextID = 3
	saveCodex(id, cf)
	m.codexOpen(id, true)
	c := m.cx()
	m.codexShowPage(0)
	c.sel = 1
	return m, c
}

// Every one of the 16 turns runs from start to end, both ways, and ends
// on the new page exactly.
func TestCodexTurns16(t *testing.T) {
	m, c := openLook(t)
	c.edit = false
	m.codexShowPage(1)
	from := m.codexCanvas()
	m.codexShowPage(0)
	for anim := 0; anim < 16; anim++ {
		for _, ahead := range []bool{true, false} {
			for _, p := range []float64{0, 0.2, 0.5, 0.8, 0.999} {
				cv := m.codexCanvas()
				codexTurnApply(cv, from, anim, p, ahead, c.pageRect)
				codexTurnFix(cv, c.pageRect)
			}
			want := m.codexCanvas()
			cv := m.codexCanvas()
			codexTurnApply(cv, from, anim, 1, ahead, c.pageRect)
			if cv.Render() != want.Render() {
				t.Fatalf("turn %q (ahead %v) should end on the new page", codexTurns16[anim], ahead)
			}
		}
	}
}

// V chooses this page's turn and plays it; Alt+V gives it to every page;
// on the cover V is how it opens.
func TestCodexTurnKeys(t *testing.T) {
	codexTurns = true
	defer func() { codexTurns = false }()
	m, c := openLook(t)
	press(m, "v")
	if c.cf.Pages[0].Turn != 1 || c.turn == nil || c.turn.anim != 1 {
		t.Fatalf("V: page turn shown, got turn %d", c.cf.Pages[0].Turn)
	}
	press(m, "alt+v")
	if c.cf.Pages[1].Turn != 1 {
		t.Fatal("Alt+V: every page")
	}
	m.codexShowPage(-1)
	press(m, "v")
	if c.cf.coverTurn() != turnPage+1 || c.turn == nil {
		t.Fatalf("V on the cover: the next opening, got %d", c.cf.coverTurn())
	}
	if got := c.cf.turnBetween(-1, 0); got != c.cf.coverTurn() {
		t.Fatal("the cover's turn opens it")
	}
}

// Headings: 1 makes heading 1 (bigger, a line under it), 0 body text.
func TestCodexHeadings(t *testing.T) {
	m, c := openLook(t)
	press(m, "3")
	e := c.el(1)
	if e.Head != 3 || e.Size != 1 {
		t.Fatalf("3: heading 3, got %d size %d", e.Head, e.Size)
	}
	if s := screenOf(m); !strings.Contains(s, "┄┄┄") {
		t.Fatalf("heading 3 should have its line:\n%s", s)
	}
	press(m, "1")
	if e.Head != 1 || e.Size != 3 {
		t.Fatal("1: heading 1, large")
	}
	press(m, "0")
	if e.Head != 0 || e.Size != 1 {
		t.Fatal("0: body text")
	}
}

// Fills: D steps the looks, M the colours; the fill shows behind the thing.
func TestCodexFills(t *testing.T) {
	m, c := openLook(t)
	e := c.el(1)
	for i := 1; i < len(codexFills); i++ {
		press(m, "d")
		if e.FillBy != i {
			t.Fatalf("D: look %d, got %d", i, e.FillBy)
		}
		cv := m.codexCanvas()
		r := c.rects[1]
		under := colorHex(cv.Lines[r[1]][r[0]-1].Style.Bg, "")
		if under == c.paper(c.pageOf()) && i != 11 && i != 12 {
			t.Fatalf("fill %q should colour the cell beside the words", codexFills[i])
		}
	}
	press(m, "m")
	if e.Fill == "" {
		t.Fatal("M: a fill colour")
	}
}

// Page borders: Alt+B gives this page one; Alt+B on the cover every page.
func TestCodexPageBorders(t *testing.T) {
	m, c := openLook(t)
	press(m, "alt+b")
	if c.cf.Pages[0].Border != 1 || !strings.Contains(screenOf(m), "┌") {
		t.Fatal("Alt+B: this page's thin border")
	}
	m.codexShowPage(-1)
	press(m, "alt+b")
	press(m, "alt+b")
	press(m, "alt+b")
	if c.cf.PageBorder != 3 {
		t.Fatal("Alt+B on the cover: every page's border")
	}
	m.codexShowPage(1)
	if s := screenOf(m); !strings.Contains(s, "╔") {
		t.Fatalf("page 2 should have the double border:\n%s", s)
	}
}

// Header and footer: H and U write them; they show on every page with
// {page} filled in; Alt+H takes them off one page.
func TestCodexHeaderFooter(t *testing.T) {
	m, c := openLook(t)
	press(m, "h")
	typeStr(m, "My header")
	press(m, "enter")
	press(m, "u")
	typeStr(m, "Page {page} of {pages}")
	press(m, "enter")
	if c.cf.Header != "My header" || c.cf.Footer != "Page {page} of {pages}" {
		t.Fatalf("header %q footer %q", c.cf.Header, c.cf.Footer)
	}
	m.codexShowPage(1)
	s := screenOf(m)
	if !strings.Contains(s, "My header") || !strings.Contains(s, "Page 2 of 2") {
		t.Fatalf("page 2 should show both:\n%s", s)
	}
	press(m, "alt+h")
	if s := screenOf(m); strings.Contains(s, "My header") {
		t.Fatal("Alt+H: none on this page")
	}
}

// The font list: F opens it, ↓ shows the next font at once, Esc puts the
// old one back, Enter keeps.
func TestCodexFontPick(t *testing.T) {
	m, c := openLook(t)
	e := c.el(1)
	press(m, "f")
	if c.pick == nil {
		t.Fatal("F should open the font list")
	}
	if s := screenOf(m); !strings.Contains(s, "fraktur") || !strings.Contains(s, codexFont("Hello there", 6)) {
		t.Fatalf("the list should show each font on the text's own words:\n%s", s)
	}
	press(m, "down")
	press(m, "down")
	if e.Font != 2 {
		t.Fatal("↓ shows the font at once")
	}
	press(m, "esc")
	if e.Font != 0 || c.pick != nil {
		t.Fatal("Esc: as it was")
	}
	press(m, "f")
	press(m, "down")
	press(m, "enter")
	if e.Font != 1 || c.pick != nil {
		t.Fatal("Enter keeps it")
	}
	m.codexShowPage(-1)
	press(m, "f")
	press(m, "down")
	press(m, "enter")
	if c.cf.TitleFont != 1 {
		t.Fatal("F on the cover: the title's font")
	}
}

// The cover is a front: no dark spine down its left side, a frame both sides.
func TestCodexCoverFront(t *testing.T) {
	cf := newCodexFile("Front")
	cv := codexCover(cf, 60, 24, 0, false)
	left := colorHex(cv.Lines[10][1].Style.Bg, "")
	right := colorHex(cv.Lines[10][58].Style.Bg, "")
	if lum(left) < lum(right)-0.05 {
		t.Fatal("the left edge should not be a dark spine")
	}
	if cv.Lines[10][3].Content != "║" || cv.Lines[10][56].Content != "║" {
		t.Fatalf("the gilt frame should sit evenly on both sides: %q %q", cv.Lines[10][3].Content, cv.Lines[10][56].Content)
	}
	_ = time.Now
}
