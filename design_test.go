package main

import (
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

func designTest(t *testing.T) *Model {
	t.Helper()
	m := newTest(t, "text stays")
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.setMode(designMode)
	return m
}

func click(m *Model, x, y int, b tea.MouseButton) {
	m.Update(tea.MouseClickMsg(tea.Mouse{X: x, Y: y, Button: b}))
	m.Update(tea.MouseReleaseMsg(tea.Mouse{X: x, Y: y, Button: b}))
}

func dragTo(m *Model, x0, y0, x1, y1 int, b tea.MouseButton) {
	m.Update(tea.MouseClickMsg(tea.Mouse{X: x0, Y: y0, Button: b}))
	m.Update(tea.MouseMotionMsg(tea.Mouse{X: x1, Y: y1, Button: b}))
	m.Update(tea.MouseReleaseMsg(tea.Mouse{X: x1, Y: y1, Button: b}))
}

func typeKeys(m *Model, s string) {
	for _, r := range s {
		m.Update(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
	}
}

func TestDesignBoard(t *testing.T) {
	m := designTest(t)
	d := m.ds()

	// Double-click on the empty board places a structure and writes its header.
	click(m, 30, 12, tea.MouseLeft)
	click(m, 30, 12, tea.MouseLeft)
	if len(d.octs) != 1 || d.editing != partHeader {
		t.Fatalf("double-click: %d structures, editing %d", len(d.octs), d.editing)
	}
	typeKeys(m, "Start")
	press(m, "tab")
	typeKeys(m, "the body text")
	press(m, "tab")
	typeKeys(m, "foot")
	press(m, "esc")
	a := d.octs[0]
	if a.Header != "Start" || a.Body != "the body text" || a.Footer != "foot" {
		t.Fatalf("written: %+v", a)
	}
	screen := ansi.Strip(m.View().Content)
	for _, w := range []string{"Start", "the body text", "foot", "╱", "╲", "1 structures"} {
		if !strings.Contains(screen, w) {
			t.Fatalf("board is missing %q:\n%s", w, screen)
		}
	}
	if string(m.buf.Lines[0]) != "text stays" {
		t.Fatal("typing on the board changed the text file")
	}

	// Drag moves it; the ◢ corner resizes it.
	x, y, w, h := d.rect(a)
	ox, oy := a.X, a.Y
	time.Sleep(450 * time.Millisecond) // not a double-click
	dragTo(m, x+5, y+textTop+3, x+15, y+textTop+5, tea.MouseLeft)
	if a.X != ox+10 || a.Y != oy+2 {
		t.Fatalf("move: %d,%d want %d,%d", a.X, a.Y, ox+10, oy+2)
	}
	x, y, w, h = d.rect(a)
	ow, oh := a.W, a.H
	dragTo(m, x+w-1, y+h-1+textTop, x+w+5, y+h+2+textTop, tea.MouseLeft)
	if a.W != ow+6 || a.H != oh+3 {
		t.Fatalf("resize: %dx%d want %dx%d", a.W, a.H, ow+6, oh+3)
	}

	// A second structure, then link mode joins them.
	time.Sleep(450 * time.Millisecond)
	click(m, 80, 8, tea.MouseLeft)
	click(m, 80, 8, tea.MouseLeft)
	press(m, "esc")
	b := d.octs[1]
	if a.Color == b.Color {
		t.Fatal("two structures got the same colour")
	}
	m.Update(tea.KeyPressMsg(tea.Key{Code: 'ł', Text: "ł"}))
	if !d.linkMode {
		t.Fatal("AltGr+L did not start link mode")
	}
	if m.learn != nil {
		t.Fatal("AltGr+L opened Learn in Design mode")
	}
	ax, ay, _, _ := d.rect(a)
	bx, by, _, _ := d.rect(b)
	time.Sleep(450 * time.Millisecond)
	click(m, bx+6, by+textTop+3, tea.MouseLeft) // b chosen (it was selected: click again unchooses)
	if d.linkFrom == b.ID {
		click(m, ax+6, ay+textTop+3, tea.MouseLeft)
	} else {
		click(m, bx+6, by+textTop+3, tea.MouseLeft)
		click(m, ax+6, ay+textTop+3, tea.MouseLeft)
	}
	if len(d.links) != 1 {
		t.Fatalf("links: %v", d.links)
	}
	press(m, "esc")
	if d.linkMode {
		t.Fatal("Esc did not leave link mode")
	}

	// Middle-drag moves the board; the wheel zooms.
	cx := d.camX
	dragTo(m, 50, 20, 40, 20, tea.MouseMiddle)
	if d.camX != cx+10 {
		t.Fatalf("pan: camX %v want %v", d.camX, cx+10)
	}
	m.Update(tea.MouseWheelMsg(tea.Mouse{X: 50, Y: 15, Button: tea.MouseWheelDown}))
	if d.z() >= 1 {
		t.Fatalf("wheel down did not zoom out: %v", d.z())
	}
	m.Update(tea.MouseWheelMsg(tea.Mouse{X: 50, Y: 15, Button: tea.MouseWheelUp}))
	if d.z() != 1 {
		t.Fatalf("wheel up did not zoom back: %v", d.z())
	}

	// The pen: 3 takes the red one; a drag colours cells; right-drag rubs out.
	press(m, "3")
	if d.pen != 2 {
		t.Fatalf("pen %d", d.pen)
	}
	dragTo(m, 2, 25, 12, 25, tea.MouseLeft)
	if len(d.marks) != 11 {
		t.Fatalf("pen marked %d cells, want 11", len(d.marks))
	}
	dragTo(m, 2, 25, 6, 25, tea.MouseRight)
	if len(d.marks) != 6 {
		t.Fatalf("after rubbing out: %d cells, want 6", len(d.marks))
	}
	press(m, "o")
	if !d.over {
		t.Fatal("O did not switch to over")
	}
	press(m, "esc")

	// It is all kept.
	d.save()
	if _, err := os.Stat(designPath()); err != nil {
		t.Fatal(err)
	}
	again := loadDesign(designPath())
	if len(again.octs) != 2 || len(again.links) != 1 || len(again.marks) != 6 || again.octs[0].Header != "Start" {
		t.Fatalf("loaded: %d structures, %d links, %d marks", len(again.octs), len(again.links), len(again.marks))
	}

	// Delete removes a structure and its links.
	d.sel = a.ID
	press(m, "delete")
	if len(d.octs) != 1 || len(d.links) != 0 {
		t.Fatalf("delete: %d structures, %d links", len(d.octs), len(d.links))
	}
}

func TestDesignMarks(t *testing.T) {
	m := designTest(t)
	d := m.ds()
	o := d.newOct(10, 5)
	o.Body = "hello"
	d.sel = -1
	find := func() (int, int, uv.Cell) {
		c := m.designCanvas()
		for y := range c.Height() {
			for x := range c.Width() {
				if cell := c.CellAt(x, y); cell != nil && cell.Content == "h" {
					return x, y, *cell
				}
			}
		}
		t.Fatal("body text not drawn")
		return 0, 0, uv.Cell{}
	}
	hx, hy, before := find()
	d.pen = 0
	d.mark(hx, hy, false)
	_, _, under := find()
	if under.Content != "h" || colorHex(under.Style.Fg, "") != colorHex(before.Style.Fg, "") ||
		colorHex(under.Style.Bg, "") == colorHex(before.Style.Bg, "") {
		t.Fatalf("under: the letter should stay and the background change: %+v (was %+v)", under.Style, before.Style)
	}
	d.over = true
	d.mark(hx, hy, false)
	d.mark(0, 0, false)
	_, _, over := find()
	if over.Content != "h" || colorHex(over.Style.Fg, "") == colorHex(before.Style.Fg, "") {
		t.Fatalf("over: the letter should show through, tinted: %+v", over.Style)
	}
	if got := m.designCanvas().CellAt(0, 0).Content; got != "░" {
		t.Fatalf("over on the empty board: %q", got)
	}
}
