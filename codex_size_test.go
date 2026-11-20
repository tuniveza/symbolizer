package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// + and − make every kind of thing bigger and smaller.
func TestCodexSizeKeys(t *testing.T) {
	m := codexTest(t)
	c := framesCodex(t, m) // 1 text, 2 button (slot 0), 3 picture (slot 1)
	canvasRows(m)
	full := c.rects[3]
	press(m, "-")
	press(m, "-")
	if e := c.el(3); e.W != 80 || e.H != 80 {
		t.Fatalf("− twice on the picture: W %d H %d, want 80 80", e.W, e.H)
	}
	canvasRows(m)
	if r := c.rects[3]; r[2] >= full[2] || r[3] >= full[3] {
		t.Fatalf("a smaller picture %v should be smaller than %v", r, full)
	}
	press(m, "=")
	press(m, "=")
	if e := c.el(3); e.W != 0 || e.H != 100 {
		t.Fatalf("+ twice back: W %d H %d", e.W, e.H)
	}
	c.sel = 2
	canvasRows(m)
	b := c.rects[2]
	press(m, "=")
	canvasRows(m)
	if r := c.rects[2]; r[2] <= b[2] || r[3] != 5 {
		t.Fatalf("+ on the button: %v, was %v (want wider and 5 rows)", r, b)
	}
	press(m, "-")
	if e := c.el(2); e.H != 0 {
		t.Fatalf("− on the button: back to 3 rows, H %d", e.H)
	}
	c.sel = 1
	press(m, "=")
	if c.el(1).Size != 2 {
		t.Fatal("+ should still grow a text")
	}
	m.codexUndo(false)
	if c.el(1).Size > 1 {
		t.Fatal("Ctrl+Z should undo the size")
	}
}

// Dragging the chosen thing's corner ◢ resizes it; one Ctrl+Z undoes the drag.
func TestCodexSizeDrag(t *testing.T) {
	m := codexTest(t)
	c := framesCodex(t, m)
	screenOf(m)
	if !strings.Contains(screenOf(m), "◢") {
		t.Fatal("the chosen thing should show its corner handle ◢")
	}
	r := c.rects[3]
	hx, hy := codexHandle(r)
	top := m.boardTop()
	steps := len(c.undo)
	m.Update(tea.MouseClickMsg{X: hx, Y: hy + top, Button: tea.MouseLeft})
	if c.sizing == nil {
		t.Fatal("a press on the handle should start a resize")
	}
	m.Update(tea.MouseMotionMsg{X: hx - r[2]/4, Y: hy - r[3]/2 + top, Button: tea.MouseLeft})
	m.Update(tea.MouseReleaseMsg{X: hx - r[2]/4, Y: hy - r[3]/2 + top, Button: tea.MouseLeft})
	e := c.el(3)
	if c.sizing != nil || e.W == 0 || e.H == 0 || e.H > 60 {
		t.Fatalf("after the drag: sizing %v W %d H %d", c.sizing, e.W, e.H)
	}
	canvasRows(m)
	if n := c.rects[3]; n[2] >= r[2] || n[3] >= r[3] {
		t.Fatalf("the picture should be smaller after the drag: %v, was %v", n, r)
	}
	if len(c.undo) != steps+1 {
		t.Fatalf("one drag should be one undo step: %d → %d", steps, len(c.undo))
	}
	// A text dragged down grows its letters.
	c.sel = 1
	canvasRows(m)
	r = c.rects[1]
	hx, hy = codexHandle(r)
	m.Update(tea.MouseClickMsg{X: hx, Y: hy + top, Button: tea.MouseLeft})
	m.Update(tea.MouseMotionMsg{X: hx, Y: hy + 6 + top, Button: tea.MouseLeft})
	m.Update(tea.MouseReleaseMsg{X: hx, Y: hy + 6 + top, Button: tea.MouseLeft})
	if c.el(1).Size < 2 {
		t.Fatalf("a text dragged down should grow: size %d", c.el(1).Size)
	}
}

// B on the cover steps through its frames: gilt, the 16 page borders, none.
func TestCodexCoverFrames(t *testing.T) {
	m, c := openLook(t)
	m.codexShowPage(-1)
	gilt := screenOf(m)
	if !strings.Contains(gilt, "◆") {
		t.Fatal("the cover's own frame is gilt (◆ corners)")
	}
	press(m, "b")
	press(m, "b")
	press(m, "b")
	if c.cf.CoverBorder != 3 || c.cf.PageBorder != 0 {
		t.Fatalf("B ×3 on the cover: frame %d, page border %d", c.cf.CoverBorder, c.cf.PageBorder)
	}
	if s := screenOf(m); !strings.Contains(s, "╔") {
		t.Fatal("cover frame 3 should be the double border")
	}
	press(m, "shift+b")
	press(m, "shift+b")
	press(m, "shift+b")
	press(m, "shift+b")
	if c.cf.CoverBorder != codexCoverNone {
		t.Fatalf("Shift+B back past gilt: %d, want none", c.cf.CoverBorder)
	}
	if s := strings.Join(canvasRows(m), "\n"); strings.Contains(s, "◆") { // the codex only: sy's top bar has a ◆ that comes and goes
		t.Fatal("no frame: no gilt corners")
	}
}

// Q goes back to the Design board when the codex was opened from there.
func TestCodexQBackToDesign(t *testing.T) {
	m, c := openLook(t)
	press(m, "q")
	if !m.inCodex() {
		t.Fatal("Q should do nothing in a codex not opened from Design")
	}
	c.fromDesign = true
	press(m, "q")
	if m.inCodex() || m.mode != designMode {
		t.Fatal("Q should go back to the Design board")
	}
}
