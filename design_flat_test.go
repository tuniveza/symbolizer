package main

// Flat-board tests kept when the 3D room was taken out (16 Nov 2026), and
// the helpers they share.

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

var _ = ansi.StringWidth
var _ = uv.Cell{}

// flatModel is a design-mode Model w × h with a structure of each kind.
func flatModel(t testing.TB, w, h int) *Model {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	m, _ := NewModel("", 0)
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	m.setMode(designMode)
	d := m.ds()
	d.camX, d.camY = 0, 0
	kinds := []string{kOct, kPage, kNote, kCabinet, kDrawer, kRack, kBoard, kOct}
	for i, k := range kinds {
		o := d.newKind(k, 0, 0)
		o.X, o.Y, o.Header = 3+(i%4)*36, 2+(i/4)*20, "Thing "+fmt.Sprint(i)
		o.Body = "Some words on it."
	}
	d.sel = -1
	return m
}

// boardRows is the board area as drawn, one string a row.
func boardRows(m *Model) []string {
	var sb strings.Builder
	m.designRows(&sb)
	return strings.Split(strings.TrimSuffix(sb.String(), "\n"), "\n")
}

// On the flat board, Alt+Space takes up the chosen structure: the arrows
// move it, Alt+Space puts it down (one undo step), Esc puts it back.
func TestDesignGrab2D(t *testing.T) {
	m := flatModel(t, 160, 50)
	d := m.ds()
	o := d.octs[1]
	d.sel = o.ID
	x0, y0 := o.X, o.Y
	d.save()
	steps := len(d.undo)
	press(m, "alt+space")
	if !d.grab.active || m.grabStatus() != grabHint2D {
		t.Fatal("Alt+Space on the flat board should take up the chosen structure")
	}
	press(m, "down")
	press(m, "down")
	if o.Y <= y0 || d.sel != o.ID {
		t.Fatalf("the arrows should move it: at %d (was %d)", o.Y, y0)
	}
	press(m, "esc")
	if d.grab.active || o.X != x0 || o.Y != y0 || len(d.undo) != steps {
		t.Fatalf("Esc should put it back with no step: at %d,%d", o.X, o.Y)
	}
	press(m, "alt+space")
	press(m, "down")
	press(m, "down")
	press(m, "alt+space")
	if d.grab.active || o.Y <= y0 {
		t.Fatal("Alt+Space should put it down where it was moved to")
	}
	if len(d.undo) != steps+1 {
		t.Fatalf("one undo step: %d, was %d", len(d.undo), steps)
	}
	press(m, "ctrl+z")
	if o := d.byID(o.ID); o == nil || o.Y != y0 {
		t.Fatal("Ctrl+Z should put it back")
	}
}

// On the flat board a box's chosen thing or a board's chosen pin is shown
// big in the middle of the screen; Esc puts it back where it was.
func TestDesignMaximize2D(t *testing.T) {
	m := flatModel(t, 160, 50)
	d := m.ds()
	bd := d.octs[6]
	bd.Items = []*dOct{{ID: 960, Kind: kPage, Header: "Milk", X: 3, Y: 2, W: 26, H: 9}}
	d.sel = bd.ID
	d.pin = pinSel{board: bd.ID, at: 0, active: true}
	press(m, "alt+space")
	it := d.byID(960)
	if it == nil || !d.opened.max || len(bd.Items) != 0 {
		t.Fatal("Alt+Space on a chosen pin should show it big")
	}
	x, y, w, h := d.rect(it)
	if w <= 26 || abs((x+w/2)-m.width/2) > 2 || abs((y+h/2)-m.boardH()/2) > 2 {
		t.Fatalf("shown big in the middle of the screen: %d,%d %dx%d", x, y, w, h)
	}
	boardRows(m)
	press(m, "esc")
	if d.byID(960) != nil || len(bd.Items) != 1 || it.X != 3 || it.Y != 2 || it.W != 26 {
		t.Fatalf("Esc should pin it back where it was: at %d,%d %d wide", it.X, it.Y, it.W)
	}
	// Tab shows the next thing big.
	bd.Items = append(bd.Items, &dOct{ID: 961, Kind: kNote, Header: "Bob", X: 30, Y: 2})
	d.sel = bd.ID
	d.pin = pinSel{board: bd.ID, at: 0, active: true}
	press(m, "alt+space")
	press(m, "tab")
	if d.opened.id != 961 || !d.opened.max || len(bd.Items) != 1 {
		t.Fatalf("Tab should show the next pin big: %+v", d.opened)
	}
	press(m, "alt+space")
	if d.opened.active || len(bd.Items) != 2 || bd.Items[0].ID != 960 {
		t.Fatal("Alt+Space should put it back")
	}
}

// accentCells are the letters drawn on the accent colour (the cursor) in
// what boardRows drew.
func accentCells(m *Model) string {
	s := strings.Join(boardRows(m), "\n")
	buf := uv.NewScreenBuffer(m.width, len(strings.Split(s, "\n")))
	uv.NewStyledString(s).Draw(buf, buf.Bounds())
	var out strings.Builder
	for y := range buf.Height() {
		for x := range buf.Width() {
			if c := buf.CellAt(x, y); c != nil && c.Style.Bg != nil && colorHex(c.Style.Bg, "") == colorHex(rgbaOf(cAccent), "") {
				out.WriteString(c.Content)
			}
		}
	}
	return out.String()
}

// The flat board draws the cursor where it is: on the letter it is
// before (and the caret ▏ after the end), in the text and in the title.
func TestDesignFieldDrawn(t *testing.T) {
	m, o := writePage(t, "hello world")
	press(m, "left")
	if got := accentCells(m); got != "w" {
		t.Fatalf("the cursor drawn on %q, want w", got)
	}
	if rows := strings.Join(boardRows(m), "\n"); !strings.Contains(ansi.Strip(rows), "hello world") {
		t.Fatal("the text should still read whole with the cursor in it")
	}
	m.ds().editing = partHeader
	typeKeys(m, "Plan")
	press(m, "ctrl+left")
	if got := accentCells(m); got != "n" {
		t.Fatalf("the title's cursor drawn on %q, want n", got)
	}
	m.ds().editing = partFooter
	typeKeys(m, "End")
	press(m, "ctrl+left")
	if got := accentCells(m); got != "d" {
		t.Fatalf("the footer's cursor drawn on %q, want d", got)
	}
	_ = o
	// A container's name, being written: the cursor in it too.
	for _, kind := range []string{kCabinet, kDrawer, kRack, kBoard, kBox, kOct} {
		m := designTest(t)
		d := m.ds()
		d.camX, d.camY, d.zoom = 0, 0, designZoom100
		c := d.newKind(kind, 0, 0)
		c.X, c.Y = 4, 2
		d.sel, d.editing = c.ID, partHeader
		typeKeys(m, "Shed")
		press(m, "ctrl+left")
		if got := accentCells(m); !strings.Contains(got, "d") {
			t.Errorf("%s: the name's cursor drawn on %q, want d", kind, got)
		}
		press(m, "shift+home")
		if cur, s, e, _ := d.fieldMarks(c, partHeader); cur != 0 || s != 0 || e != 3 {
			t.Errorf("%s: Shift+Home: cursor %d, highlight %d–%d", kind, cur, s, e)
		}
	}
}
