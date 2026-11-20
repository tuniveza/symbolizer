package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// framesCodex: a page of two columns with a text, a button and a picture.
func framesCodex(t *testing.T, m *Model) *codexState {
	t.Helper()
	m.Update(tea.WindowSizeMsg{Width: 180, Height: 50})
	id, cf, _ := createCodex("Frames")
	png := writePNG(t, t.TempDir())
	cf.Pages[0].Layout = 2 // two columns
	cf.Pages[0].Els = []*codexEl{
		{ID: 1, Kind: ceText, Slot: 0, Text: "Framed words"},
		{ID: 2, Kind: ceButton, Slot: 0, Text: "Go on", Action: caNext},
		{ID: 3, Kind: ceImage, Slot: 1},
	}
	cf.NextID = 4
	saveCodex(id, cf)
	m.codexOpen(id, true)
	press(m, "pgdown")
	c := m.cx()
	c.sel = 3
	m.codexUseImage(png, false)
	return c
}

// B steps through 16 borders and none; each one draws its corner round the thing.
func TestCodexBorders(t *testing.T) {
	m := codexTest(t)
	c := framesCodex(t, m)
	for _, id := range []int{1, 2, 3} {
		c.sel = id
		e := c.el(id)
		for i := 1; i <= 16; i++ {
			press(m, "b")
			if e.Border != i {
				t.Fatalf("B %d on thing %d: border %d", i, id, e.Border)
			}
			rows := canvasRows(m)
			r := c.rects[id]
			tl := runeAt(rows[r[1]], r[0])
			if want := codexBorders[i-1].tl; tl != want {
				t.Fatalf("thing %d, border %q: its top left corner is %q, want %q", id, codexBorders[i-1].name, tl, want)
			}
			if !strings.Contains(m.msg, codexBorders[i-1].name) {
				t.Fatalf("B should say the border's name: %q", m.msg)
			}
		}
		press(m, "b")
		if e.Border != 0 {
			t.Fatal("after 16 borders B should take it off")
		}
		press(m, "shift+b")
		if e.Border != 16 {
			t.Fatal("Shift+B should go back to the last border")
		}
	}
	// The words are still all there inside their border.
	if !strings.Contains(strings.Join(canvasRows(m), "\n"), "Framed words") {
		t.Fatal("a border must not cover the words")
	}
}

// Y steps a button through 16 looks; every look shows its label.
func TestCodexButtonLooks(t *testing.T) {
	m := codexTest(t)
	c := framesCodex(t, m)
	c.sel = 2
	e := c.el(2)
	seen := map[string]bool{}
	for i := range 16 {
		if e.Style != i {
			t.Fatalf("look %d: style %d", i, e.Style)
		}
		all := strings.Join(canvasRows(m), "\n")
		if !strings.Contains(all, "Go on") {
			t.Fatalf("look %q: the label is gone", codexButtonStyles[i].name)
		}
		r := c.rects[2]
		rows := canvasRows(m)
		var b strings.Builder
		for y := r[1]; y < r[1]+r[3]; y++ {
			for x := r[0]; x < r[0]+r[2]; x++ {
				b.WriteString(runeAt(rows[y], x))
			}
		}
		seen[b.String()] = true
		press(m, "y")
	}
	if e.Style != 0 {
		t.Fatal("16 Ys should come round to the first look")
	}
	if len(seen) < 11 { // five pairs differ only in how they are filled
		t.Fatalf("the looks should look different: %d kinds of glyphs", len(seen))
	}
}

// A picture fills its box by default; Z shows it whole, Z again fills.
func TestCodexPictureFills(t *testing.T) {
	m := codexTest(t)
	c := framesCodex(t, m)
	canvasRows(m)
	full := c.rects[3]
	ix, iy, iw, ih := c.inner[0], c.inner[1], c.inner[2], c.inner[3]
	slot := codexSlotRects(2, iw, ih)[1]
	if full[0] != ix+slot[0] || full[2] != slot[2] || full[3] != slot[3] || full[1] != iy+slot[1] {
		t.Fatalf("the picture should fill its slot %v: %v", slot, full)
	}
	press(m, "z")
	if !c.el(3).Whole {
		t.Fatal("Z should show the picture whole")
	}
	canvasRows(m)
	if w := c.rects[3]; w[2]*w[3] >= full[2]*full[3] {
		t.Fatalf("a whole picture (%v) should be smaller than a filling one (%v) in a wide slot", w, full)
	}
	press(m, "alt+left") // whole: it lines up on the left of its slot first
	if c.el(3).Slot != 1 || c.el(3).Align != 1 {
		t.Fatalf("Alt+Left on a whole picture: slot %d align %d", c.el(3).Slot, c.el(3).Align)
	}
	press(m, "z")
	if c.el(3).Whole {
		t.Fatal("Z again should make it fill its box")
	}
}

// Alt+Up and Alt+Down move a thing among its slot's things, then into the slot that way.
func TestCodexMoveUpDown(t *testing.T) {
	m := codexTest(t)
	m.Update(tea.WindowSizeMsg{Width: 180, Height: 50})
	id, cf, _ := createCodex("Moves")
	cf.Pages[0].Layout = 12 // header, body, button bar
	cf.Pages[0].Els = []*codexEl{
		{ID: 1, Kind: ceText, Slot: 1, Text: "one"},
		{ID: 2, Kind: ceText, Slot: 1, Text: "two"},
		{ID: 3, Kind: ceText, Slot: 0, Text: "head"},
	}
	cf.NextID = 4
	saveCodex(id, cf)
	m.codexOpen(id, true)
	press(m, "pgdown")
	c := m.cx()
	c.sel = 2
	press(m, "alt+up")
	if s := elsIn(c.pageOf(), 1); s[0].ID != 2 {
		t.Fatal("Alt+Up should move it up in its slot")
	}
	press(m, "alt+up")
	if e := c.el(2); e.Slot != 1 || codexVPos(e.VAlign) != 0 {
		t.Fatalf("Alt+Up at the top of the middle things should go to the slot's top: slot %d, valign %d", e.Slot, e.VAlign)
	}
	press(m, "alt+up")
	if e := c.el(2); e.Slot != 0 {
		t.Fatalf("Alt+Up at the top of a slot should go to the slot above: %d", e.Slot)
	}
	if e := c.el(2); codexVPos(e.VAlign) != 2 {
		t.Fatal("going up it comes in at the slot's bottom")
	}
	if s := elsIn(c.pageOf(), 0); s[len(s)-1].ID != 2 {
		t.Fatal("going up it lands at the bottom of the slot above")
	}
	press(m, "alt+down")
	if e := c.el(2); e.Slot != 1 || elsIn(c.pageOf(), 1)[0].ID != 2 {
		t.Fatal("Alt+Down from the bottom of a slot lands at the top of the slot below")
	}
	press(m, "ctrl+z")
	if c.el(2).Slot != 0 {
		t.Fatal("Ctrl+Z should undo a move")
	}
}

// Alt + the arrows walk a thing through all 9 places of its slot (top left
// … bottom right), and each is where it is drawn (the user asked, 19 Nov 2026).
func TestCodexNinePlaces(t *testing.T) {
	m := codexTest(t)
	m.Update(tea.WindowSizeMsg{Width: 180, Height: 50})
	id, cf, _ := createCodex("Places")
	cf.Pages[0].Layout = 0 // one area
	cf.Pages[0].Els = []*codexEl{{ID: 1, Kind: ceButton, Slot: 0, Text: "Press", W: 30}}
	cf.NextID = 2
	saveCodex(id, cf)
	m.codexOpen(id, true)
	press(m, "pgdown")
	c := m.cx()
	c.sel = 1
	canvasRows(m)
	place := c.place[1]
	at := func() [4]int { canvasRows(m); return c.rects[1] }
	mid := at()
	press(m, "alt+up")
	top := at()
	if top[1] != place[1] || codexPlaceName(c.el(1)) != "at the top" {
		t.Fatalf("Alt+Up: at the top of its place (%v), got %v, says %q", place, top, codexPlaceName(c.el(1)))
	}
	press(m, "alt+left")
	tl := at()
	if tl[0] != place[0] || tl[1] != place[1] || codexPlaceName(c.el(1)) != "top left" {
		t.Fatalf("top left: %v in %v, says %q", tl, place, codexPlaceName(c.el(1)))
	}
	press(m, "alt+down")
	press(m, "alt+down")
	bl := at()
	if bl[1]+bl[3] != place[1]+place[3] || codexPlaceArrow(c.el(1)) != "↙" {
		t.Fatalf("bottom left: %v should end at the bottom of %v (%s)", bl, place, codexPlaceArrow(c.el(1)))
	}
	press(m, "alt+right")
	press(m, "alt+right")
	br := at()
	if br[0]+br[2] != place[0]+place[2] || br[1] != bl[1] || codexPlaceName(c.el(1)) != "bottom right" {
		t.Fatalf("bottom right: %v in %v, says %q", br, place, codexPlaceName(c.el(1)))
	}
	if !(top[1] < mid[1] && mid[1] < bl[1]) {
		t.Fatalf("top %v, middle %v, bottom %v should go down the page", top, mid, bl)
	}
	press(m, "alt+down")
	if !strings.Contains(m.msg, "as far down") {
		t.Fatalf("at the bottom of the only slot Alt+Down should say so: %q", m.msg)
	}
}

// Things at the top, middle and bottom of one slot stack from its top, sit
// in its middle and stack up from its bottom; all kinds can.
func TestCodexStackPlaces(t *testing.T) {
	m := codexTest(t)
	m.Update(tea.WindowSizeMsg{Width: 180, Height: 50})
	id, cf, _ := createCodex("Stacks")
	cf.Pages[0].Layout = 0
	cf.Pages[0].Els = []*codexEl{
		{ID: 1, Kind: ceText, Slot: 0, Text: "bottom words", VAlign: 2},
		{ID: 2, Kind: ceSymbol, Slot: 0, Symbol: 1},
		{ID: 3, Kind: ceButton, Slot: 0, Text: "Top", VAlign: 1},
		{ID: 4, Kind: ceDrawing, Slot: 0, H: 20, VAlign: 1, W: 40},
	}
	cf.NextID = 5
	saveCodex(id, cf)
	m.codexOpen(id, true)
	press(m, "pgdown")
	c := m.cx()
	canvasRows(m)
	pl := c.place[3]
	top, draw, sym, bot := c.rects[3], c.rects[4], c.rects[2], c.rects[1]
	if top[1] != pl[1] {
		t.Fatalf("the first top thing starts at the slot's top: %v in %v", top, pl)
	}
	if !(top[1] < draw[1] && draw[1] < sym[1] && sym[1] < bot[1]) {
		t.Fatalf("top button %v, top drawing %v, middle symbol %v, bottom text %v: wrong order", top, draw, sym, bot)
	}
	if bot[1]+bot[3] != pl[1]+pl[3] {
		t.Fatalf("the bottom text ends at the slot's bottom: %v in %v", bot, pl)
	}
}

// A text lined up left or right sits at the slot's own edge, not in a
// reading column in the middle (so "top left" is the real corner).
func TestCodexTextAtEdges(t *testing.T) {
	m := codexTest(t)
	m.Update(tea.WindowSizeMsg{Width: 180, Height: 50})
	id, cf, _ := createCodex("Edges")
	cf.Pages[0].Layout = 0
	cf.Pages[0].Els = []*codexEl{{ID: 1, Kind: ceText, Slot: 0, Text: "corner words", Align: 1, VAlign: 1}}
	cf.NextID = 2
	saveCodex(id, cf)
	m.codexOpen(id, true)
	press(m, "pgdown")
	c := m.cx()
	canvasRows(m)
	pl, r := c.place[1], c.rects[1]
	if r[0] != pl[0] || r[1] != pl[1] {
		t.Fatalf("top left text: at %v, the slot starts at %v", r, pl)
	}
	c.el(1).Align, c.el(1).VAlign = 2, 2
	canvasRows(m)
	r = c.rects[1]
	if r[0]+r[2] != pl[0]+pl[2] || r[1]+r[3] != pl[1]+pl[3] {
		t.Fatalf("bottom right text: at %v, the slot is %v", r, pl)
	}
}
