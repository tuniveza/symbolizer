package main

import (
	"encoding/json"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// openCover opens the "Looks" codex on its cover, in edit mode, drawn once.
func openCover(t *testing.T) (*Model, *codexState) {
	t.Helper()
	m, c := openLook(t)
	c.cf.Subtitle = "a small book"
	m.codexShowPage(-1)
	canvasRows(m)
	return m, c
}

// + and − on the whole cover change its size on the screen; R puts it back.
func TestCodexCoverWholeSize(t *testing.T) {
	m, c := openCover(t)
	if c.coverPart != cpWhole {
		t.Fatalf("the whole cover is chosen first, got %d", c.coverPart)
	}
	full := c.box["cover"]
	press(m, "-")
	press(m, "-")
	if c.cf.CoverW != 80 || c.cf.CoverH != 80 {
		t.Fatalf("− twice: %d%% × %d%%, want 80 × 80", c.cf.CoverW, c.cf.CoverH)
	}
	canvasRows(m)
	small := c.box["cover"]
	if small[2] >= full[2] || small[3] >= full[3] {
		t.Fatalf("the cover should be smaller: %v, was %v", small, full)
	}
	if small[0] <= full[0] || small[1] <= full[1] {
		t.Fatalf("a smaller cover stays in the middle: %v, was %v", small, full)
	}
	for i := 0; i < 10; i++ {
		press(m, "-")
	}
	if c.cf.CoverW != 30 {
		t.Fatalf("the smallest cover is 30%%, got %d", c.cf.CoverW)
	}
	press(m, "r")
	if c.cf.CoverW != 0 || c.cf.CoverH != 0 {
		t.Fatal("R gives the cover the page's size again")
	}
}

// ↑ ↓ choose the parts; + makes the title bigger, − the subtitle; colour, hide, move.
func TestCodexCoverParts(t *testing.T) {
	m, c := openCover(t)
	press(m, "down")
	if c.coverPart != cpSymbol {
		t.Fatalf("↓ from the whole cover: the symbol, got %d", c.coverPart)
	}
	press(m, "down")
	if c.coverPart != cpTitle {
		t.Fatalf("↓ again: the title, got %d", c.coverPart)
	}
	canvasRows(m)
	before := c.cinfo().rects[cpTitle]
	press(m, "=")
	canvasRows(m)
	if c.cf.partOf(cpTitle).Size < 2 {
		t.Fatalf("+ on the title: size %d", c.cf.partOf(cpTitle).Size)
	}
	if after := c.cinfo().rects[cpTitle]; after[3] <= before[3] && after[2] <= before[2] {
		t.Fatalf("the title should be bigger on the screen: %v, was %v", after, before)
	}
	// Its colour, its place on the line.
	press(m, "c")
	if c.cf.partOf(cpTitle).Color != codexPalette[0] {
		t.Fatalf("C on the title: %q", c.cf.partOf(cpTitle).Color)
	}
	if c.cf.Color != "" {
		t.Fatal("C on the title must not change the leather")
	}
	press(m, "alt+left")
	if c.cf.partOf(cpTitle).Align != 1 {
		t.Fatalf("Alt+← on the title: align %d, want left", c.cf.partOf(cpTitle).Align)
	}
	// Moved above the symbol.
	press(m, "alt+up")
	if o := c.cf.coverOrder(); o[0] != cpTitle {
		t.Fatalf("Alt+↑ on the title: order %v", o)
	}
	canvasRows(m)
	if c.cinfo().rects[cpTitle][1] >= c.cinfo().rects[cpSymbol][1] {
		t.Fatal("the title should now be drawn above the symbol")
	}
	// The subtitle, hidden and shown.
	press(m, "down")
	press(m, "down")
	if c.coverPart != cpSubtitle {
		t.Fatalf("↓↓: the subtitle, got %d", c.coverPart)
	}
	press(m, "x")
	canvasRows(m)
	if !c.cf.partOf(cpSubtitle).Hide || c.cinfo().rects[cpSubtitle][2] != 0 {
		t.Fatal("X hides the subtitle")
	}
	press(m, "x")
	canvasRows(m)
	if c.cinfo().rects[cpSubtitle][2] == 0 {
		t.Fatal("X again shows the subtitle")
	}
	// The page count goes to the top of the frame.
	press(m, "down")
	if c.coverPart != cpCount {
		t.Fatalf("↓: the page count, got %d", c.coverPart)
	}
	press(m, "alt+up")
	canvasRows(m)
	if !c.cf.partOf(cpCount).Top || c.cinfo().rects[cpCount][1] > 2 {
		t.Fatalf("Alt+↑ on the page count: top %v, row %d", c.cf.partOf(cpCount).Top, c.cinfo().rects[cpCount][1])
	}
	// Saved in the file, and back as it came with R.
	b, _ := json.Marshal(c.cf)
	if !strings.Contains(string(b), "coverParts") || !strings.Contains(string(b), "coverOrder") {
		t.Fatalf("the parts should be in the file: %s", b)
	}
	press(m, "ctrl+z")
	if c.cf.partOf(cpCount).Top {
		t.Fatal("Ctrl+Z undoes the last change")
	}
}

// A click chooses a part; dragging the whole cover's corner resizes it.
func TestCodexCoverMouse(t *testing.T) {
	m, c := openCover(t)
	top := m.boardTop()
	tr := c.codexCoverRect(cpTitle)
	m.Update(tea.MouseClickMsg{X: tr[0] + 1, Y: tr[1] + top, Button: tea.MouseLeft})
	m.Update(tea.MouseReleaseMsg{X: tr[0] + 1, Y: tr[1] + top, Button: tea.MouseLeft})
	if c.coverPart != cpTitle {
		t.Fatalf("a click on the title chooses it, got %d", c.coverPart)
	}
	cr := c.box["cover"]
	m.Update(tea.MouseClickMsg{X: cr[0] + 1, Y: cr[1] + cr[3] - 2 + top, Button: tea.MouseLeft})
	m.Update(tea.MouseReleaseMsg{X: cr[0] + 1, Y: cr[1] + cr[3] - 2 + top, Button: tea.MouseLeft})
	if c.coverPart != cpWhole {
		t.Fatalf("a click on the leather chooses the whole cover, got %d", c.coverPart)
	}
	canvasRows(m)
	if !strings.Contains(screenOf(m), "◢") {
		t.Fatal("the chosen cover shows its corner handle")
	}
	hx, hy := codexHandle(cr)
	steps := len(c.undo)
	m.Update(tea.MouseClickMsg{X: hx, Y: hy + top, Button: tea.MouseLeft})
	if c.sizing == nil {
		t.Fatal("a press on the cover's corner starts a resize")
	}
	m.Update(tea.MouseMotionMsg{X: hx - cr[2]/5, Y: hy - cr[3]/5 + top, Button: tea.MouseLeft})
	m.Update(tea.MouseReleaseMsg{X: hx - cr[2]/5, Y: hy - cr[3]/5 + top, Button: tea.MouseLeft})
	if c.cf.CoverW == 0 || c.cf.CoverH == 0 || c.sizing != nil {
		t.Fatalf("after the drag: %d%% × %d%%", c.cf.CoverW, c.cf.CoverH)
	}
	if len(c.undo) != steps+1 {
		t.Fatalf("one drag, one undo step: %d → %d", steps, len(c.undo))
	}
	canvasRows(m)
	if n := c.box["cover"]; n[2] >= cr[2] || n[3] >= cr[3] {
		t.Fatalf("the cover should be smaller: %v, was %v", n, cr)
	}
}

// Old files (no cover settings) draw exactly as before, and save nothing new.
func TestCodexCoverOldFiles(t *testing.T) {
	cf := &codexFile{Title: "Old", Pages: []*codexPage{{}}}
	cf.tidy()
	b, _ := json.Marshal(cf)
	for _, k := range []string{"coverW", "coverParts", "coverOrder", "coverV"} {
		if strings.Contains(string(b), k) {
			t.Fatalf("an old file should not gain %s: %s", k, b)
		}
	}
	_, info := codexCoverLaid(cf, 60, 20, 0, false, 0, nil, 0)
	if info.sizes[cpTitle] != 2 || info.sizes[cpSymbol] != 2 {
		t.Fatalf("a 60×20 cover: title size %d, symbol size %d (as before: 2, 2)", info.sizes[cpTitle], info.sizes[cpSymbol])
	}
}
