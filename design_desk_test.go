package main

import (
	"os"

	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// deskTest is the Design board, 180 × 50, with a new desk (Alt+N, T)
// placed in the middle.
func deskTest(t *testing.T) (*Model, *dOct) {
	t.Helper()
	m := newTest(t, "text stays")
	m.Update(tea.WindowSizeMsg{Width: 180, Height: 50})
	m.setMode(designMode)
	placeKey(m, "t")
	press(m, "esc") // stop writing its name
	d := m.ds()
	var desk *dOct
	for _, o := range d.octs {
		if o.Kind == kDesk {
			desk = o
		}
	}
	if desk == nil {
		t.Fatal("Alt+N, T placed no desk")
	}
	return m, desk
}

// deskSpot is the screen cell (counting the rows above the board) of the
// first cell of desk o that act names.
func deskSpot(t *testing.T, m *Model, o *dOct, act string) (int, int) {
	t.Helper()
	d := m.ds()
	m.View()
	x, y, w, h := d.rect(o)
	g := d.deskGridOf(o, w, h)
	for cy := range g.h {
		for cx := range g.w {
			if g.c[cy][cx].act == act {
				return x + cx, y + cy + m.boardTop()
			}
		}
	}
	t.Fatalf("no %q in the desk", act)
	return 0, 0
}

// deskMiddle is the screen cell in the middle of desk o's back panel.
func deskMiddle(m *Model, o *dOct) (int, int) {
	x, y, w, h := m.ds().rect(o)
	return x + w/3, y + h/3 + m.boardTop()
}

// deskPut puts a new structure of kind k on the board at board cell (x, y)
// and gives its screen cell (its first row, a few cells in).
func deskPut(m *Model, k string, x, y int) (*dOct, int, int) {
	d := m.ds()
	o := d.newKind(k, x, y)
	o.X, o.Y = x, y
	o.Header = "my " + k
	d.sel, d.editing = -1, partNone
	m.View()
	sx, sy, _, _ := d.rect(o)
	return o, sx + 2, sy + m.boardTop()
}

func inDesk(desk, o *dOct) bool {
	for _, it := range desk.Items {
		if it == o {
			return true
		}
	}
	return false
}

func TestDeskDraws(t *testing.T) {
	m, desk := deskTest(t)
	if len(desk.Items) != 4 {
		t.Fatalf("a new desk has %d things, want 4 pens", len(desk.Items))
	}
	want := map[int]bool{devPen: true, devPencil: true, devHighlighter: true, devGlitter: true}
	for _, it := range desk.Items {
		if it.Kind != kPen || !want[penDev(it)] || it.Slot != deskHolder {
			t.Fatalf("starter pen: %+v", it)
		}
		delete(want, penDev(it))
	}
	desk.Header = "Studio"
	m.ds().dirty = true
	screen := screenOf(m)
	for _, s := range []string{
		"⊓ Studio", // its name, on the back panel
		"•────",    // the metal board's rim and screws
		"░",        // brushed steel
		"┷━━┷",     // the racks' bottom bar
		"✦",        // the glitter pen's top, in the holder
		"▀▀▀▀▀▀▀▀", // the cup's rim and the table top
	} {
		if !strings.Contains(screen, s) {
			t.Fatalf("the desk is missing %q:\n%s", s, screen)
		}
	}
	// Every pen can be clicked, and so can the cup.
	for j := range desk.Items {
		deskSpot(t, m, desk, "desk:"+itoa(j))
	}
	deskSpot(t, m, desk, "deskholder")
	// Zoomed out it is still drawn (smaller), far out as a block.
	press(m, "-")
	if s := screenOf(m); !strings.Contains(s, "▀▀▀") {
		t.Fatalf("zoomed out, no desk:\n%s", s)
	}
}

func TestDeskTakesEachThingInItsPart(t *testing.T) {
	m, desk := deskTest(t)
	d := m.ds()
	dx, dy := deskMiddle(m, desk)
	for _, c := range []struct {
		kind string
		part int
	}{{kNotepad, deskRack}, {kNote, deskBoard}, {kPen, deskHolder}} {
		o, sx, sy := deskPut(m, c.kind, int(d.camX)+4+len(desk.Items)*3, int(d.camY)+2) // (not where the last drag began: no double-click)
		dragTo(m, sx, sy, dx, dy, tea.MouseLeft)
		if !inDesk(desk, o) || d.byID(o.ID) != nil {
			t.Fatalf("%s dropped on the desk is not in it (msg %q)", c.kind, m.msg)
		}
		if o.Slot != c.part || !strings.Contains(m.msg, deskPartNames[c.part]) {
			t.Fatalf("%s went in part %d (%q), want %s", c.kind, o.Slot, m.msg, deskPartNames[c.part])
		}
	}
	// Each shows in its part: the notepad's spine in the rack, the note on the metal board.
	n := len(desk.Items)
	deskSpot(t, m, desk, "desk:"+itoa(n-3))
	deskSpot(t, m, desk, "desk:"+itoa(n-2))
	if s := screenOf(m); !strings.Contains(s, "my note") {
		t.Fatalf("the note's title is not on the metal board:\n%s", s)
	}

	// Dragged out, the note is on the board again.
	nx, ny := deskSpot(t, m, desk, "desk:"+itoa(n-2))
	note := desk.Items[n-2]
	dragTo(m, nx, ny, 10, 40, tea.MouseLeft)
	if inDesk(desk, note) || d.byID(note.ID) == nil {
		t.Fatalf("the note dragged out is still in the desk (msg %q)", m.msg)
	}
}

func TestDeskRefusesOtherKinds(t *testing.T) {
	m, desk := deskTest(t)
	d := m.ds()
	dx, dy := deskMiddle(m, desk)
	for _, k := range []string{kBook, kDrawer} {
		o, sx, sy := deskPut(m, k, int(d.camX)+4+len(k)*2, int(d.camY)+2)
		n := len(desk.Items)
		dragTo(m, sx, sy, dx, dy, tea.MouseLeft)
		if inDesk(desk, o) || len(desk.Items) != n || d.byID(o.ID) == nil {
			t.Fatalf("a %s went into the desk", k)
		}
		if !strings.Contains(m.msg, "A desk takes") || !strings.Contains(m.msg, kindNames[k]) {
			t.Fatalf("refusing a %s says %q", k, m.msg)
		}
		// It is moved off the desk, not left lying on it.
		ox, oy, ow, oh := d.rect(o)
		x, y, w, h := d.rect(desk)
		if ox < x+w && x < ox+ow && oy < y+h && y < oy+oh {
			t.Fatalf("the refused %s still lies on the desk", k)
		}
	}
}

func TestDeskPenDraws(t *testing.T) {
	m, desk := deskTest(t)
	d := m.ds()
	glitter := desk.Items[3]
	if penDev(glitter) != devGlitter {
		t.Fatalf("pen 3 is a %s", devNames[penDev(glitter)])
	}
	// A click on the glitter pen in the holder picks it up.
	px, py := deskSpot(t, m, desk, "desk:3")
	click(m, px, py, tea.MouseLeft)
	if d.desk.held != glitter.ID || d.pen < 0 {
		t.Fatalf("the glitter pen is not in hand: held %d pen %d (msg %q)", d.desk.held, d.pen, m.msg)
	}
	if s := screenOf(m); !strings.Contains(s, "Glitter pen · pink") || !strings.Contains(s, "✒ glitter pen") {
		t.Fatalf("the status bar does not show the glitter pen:\n%s", s)
	}
	// Drawn on the empty board, its ink is glitter, pink, with sparkles.
	dragTo(m, 10, 36, 50, 36, tea.MouseLeft)
	if len(d.marks) < 30 {
		t.Fatalf("the stroke left %d marks", len(d.marks))
	}
	for _, mk := range d.marks {
		if mk.Dev != devGlitter || mk.Ink != "#ff4fa3" {
			t.Fatalf("a mark %+v, want glitter in #ff4fa3", mk)
		}
	}
	sparkles := func() string {
		c := m.designCanvas()
		var sb strings.Builder
		for x := 10; x <= 50; x++ {
			sb.WriteString(c.Lines[36-m.boardTop()][x].Content)
		}
		return sb.String()
	}
	before := sparkles()
	if !strings.ContainsAny(before, "✦✧·⋆") {
		t.Fatalf("no sparkles in the glitter stroke: %q", before)
	}
	if !d.desk.glitterOn {
		t.Fatal("glitter on the screen does not ask to twinkle")
	}
	m.glitterTicked()
	if after := sparkles(); after == before {
		t.Fatalf("the sparkles do not twinkle: %q", after)
	}

	// Esc puts it back in its holder; the highlighters work as before.
	press(m, "esc")
	if d.desk.held != 0 || d.pen != -1 || !strings.Contains(m.msg, "holder") {
		t.Fatalf("Esc: held %d pen %d (%q)", d.desk.held, d.pen, m.msg)
	}
	press(m, "3")
	dragTo(m, 10, 38, 20, 38, tea.MouseLeft)
	wx, wy := d.world(15, 38-m.boardTop())
	if mk := d.marks[[2]int{int(wx), int(wy)}]; mk.Dev != devHighlighter || mk.Pen != 2 || mk.Ink != "" {
		t.Fatalf("a highlighter's mark: %+v", mk)
	}
}

func TestDeskPenOnStickyNote(t *testing.T) {
	m, desk := deskTest(t)
	d := m.ds()
	note, nx, ny := deskPut(m, kNote, int(d.camX)+4, int(d.camY)+2)
	note.Body = "write over me"
	m.View()
	before := m.designCanvas()
	row := ny - m.boardTop() + 3
	bg0 := colorHex(before.Lines[row][nx+3].Style.Bg, "")
	// The marker: a thick stroke, its colour laid over the note.
	desk.Items[0].Layout, desk.Items[0].Color = devMarker, "#4ade80"
	press(m, "esc")
	d.sel = desk.ID
	press(m, "enter") // the first thing: the marker
	press(m, "enter") // picks it up
	if dev, ink, ok := d.device(); !ok || dev != devMarker || ink != "#4ade80" {
		t.Fatalf("in hand: %s %s %v (msg %q)", devNames[dev], ink, ok, m.msg)
	}
	dragTo(m, nx, row+m.boardTop(), nx+10, row+m.boardTop(), tea.MouseLeft)
	after := m.designCanvas()
	bg1 := colorHex(after.Lines[row][nx+3].Style.Bg, "")
	if bg1 == bg0 || bg1 != mixHex(bg0, "#4ade80", 0.7) && bg1 != mixHex(bg0, "#4ade80", 0.82) {
		t.Fatalf("the marker over the note: %s, was %s", bg1, bg0)
	}
	// Thick: the row under the stroke is coloured too.
	wx, wy := d.world(nx+3, row+1)
	if mk, ok := d.marks[[2]int{int(wx), int(wy)}]; !ok || mk.Dev != devMarker {
		t.Fatalf("the marker is one row thin")
	}
	// Enter on the desk again (the marker still chosen in it) puts it back.
	d.sel = desk.ID
	press(m, "enter")
	if _, _, ok := d.device(); ok {
		t.Fatal("Enter on the chosen pen in the desk did not put it back")
	}
}

func TestDeskKeys(t *testing.T) {
	m, desk := deskTest(t)
	d := m.ds()
	d.sel = desk.ID
	press(m, "enter")
	if !d.desk.pick.active || d.desk.pick.at != 0 {
		t.Fatalf("Enter on a desk chose %+v", d.desk.pick)
	}
	press(m, "right")
	press(m, "right")
	if d.desk.pick.at != 2 {
		t.Fatalf("two Rights: thing %d", d.desk.pick.at)
	}
	press(m, "left")
	press(m, "left")
	press(m, "left") // round to the last
	if d.desk.pick.at != 3 {
		t.Fatalf("three Lefts: thing %d", d.desk.pick.at)
	}
	press(m, "c")
	if desk.Items[3].Color == "#ff4fa3" {
		t.Fatal("C did not change the chosen pen's colour")
	}
	press(m, "d")
	if penDev(desk.Items[3]) != devBrush || desk.Items[3].Header != "brush" {
		t.Fatalf("D: the glitter pen is now a %s (%q)", devNames[penDev(desk.Items[3])], desk.Items[3].Header)
	}
	press(m, "p")
	if len(desk.Items) != 5 || desk.Items[4].Kind != kPen || penDev(desk.Items[4]) != devFineliner {
		t.Fatalf("P: %d things, the new one %+v", len(desk.Items), desk.Items[len(desk.Items)-1])
	}
	press(m, "esc")
	if d.desk.pick.active || d.sel != desk.ID {
		t.Fatalf("Esc: still choosing %+v, chosen %d", d.desk.pick, d.sel)
	}
	// A notepad in the rack: Enter chooses it; it opens through openNotepadHook.
	pad := d.newKind(kNotepad, 0, 0)
	d.remove(pad.ID)
	pad.From, pad.Slot = notepadLink(2), deskRack
	desk.Items = append(desk.Items, pad)
	d.sel = desk.ID
	var opened *dOct
	openNotepadHook = func(m *Model, desk, p *dOct) tea.Cmd { opened = p; return nil }
	defer func() { openNotepadHook = nil }()
	press(m, "enter")
	for d.desk.pick.at != len(desk.Items)-1 {
		press(m, "right")
	}
	press(m, "enter")
	if opened != pad {
		t.Fatal("Enter on the notepad in the rack did not open it")
	}
}

func TestDeskSavedAndLoaded(t *testing.T) {
	m, desk := deskTest(t)
	d := m.ds()
	dx, dy := deskMiddle(m, desk)
	pad, sx, sy := deskPut(m, kNotepad, int(d.camX)+4, int(d.camY)+2)
	pad.From = notepadLink(5)
	dragTo(m, sx, sy, dx, dy, tea.MouseLeft)
	px, py := deskSpot(t, m, desk, "desk:1") // the pencil
	click(m, px, py, tea.MouseLeft)
	dragTo(m, 10, 40, 30, 40, tea.MouseLeft)
	press(m, "esc")
	d.flushSave()

	again := loadDesign(d.loadedFrom)
	got := again.byID(desk.ID)
	if got == nil || got.Kind != kDesk || len(got.Items) != 5 {
		t.Fatalf("the desk came back as %+v", got)
	}
	if p := got.Items[4]; p.Kind != kNotepad || p.From != notepadLink(5) || p.Slot != deskRack {
		t.Fatalf("its notepad came back as %+v", p)
	}
	if p := got.Items[3]; p.Kind != kPen || penDev(p) != devGlitter || p.Color != "#ff4fa3" || p.W != penW || p.H != penH {
		t.Fatalf("its glitter pen came back as %+v", p)
	}
	n := 0
	for _, mk := range again.marks {
		if mk.Dev != devPencil || mk.Ink != "#9a9aa0" {
			t.Fatalf("a pencil mark came back as %+v", mk)
		}
		n++
	}
	if n == 0 || n != len(d.marks) {
		t.Fatalf("%d marks came back, %d drawn", n, len(d.marks))
	}
}

// Ink saved before there were pens (no dev, no ink) still loads as a highlighter's.
func TestDeskOldInkLoads(t *testing.T) {
	m := designTest(t)
	d := m.ds()
	press(m, "1")
	dragTo(m, 10, 20, 12, 20, tea.MouseLeft) // some ink, so the board keeps its ink in marks.json
	d.flushSave()
	path := marksPath(d.loadedFrom)
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`[{"x":3,"y":4,"pen":5,"over":true}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	again := loadDesign(d.loadedFrom)
	mk, ok := again.marks[[2]int{3, 4}]
	if !ok || mk.Pen != 5 || !mk.Over || mk.Dev != devHighlighter || mk.Ink != "" || inkOf(mk) != penColors[5] {
		t.Fatalf("old ink came back as %+v (%v)", mk, ok)
	}
}

func TestDeskDump(t *testing.T) {
	if os.Getenv("DESK_DUMP") == "" {
		t.Skip("DESK_DUMP=1 shows the desk")
	}
	m, desk := deskTest(t)
	d := m.ds()
	for _, n := range []int{3, 12, 7} {
		p := &dOct{ID: d.nextID, Kind: kNotepad, From: notepadLink(n), Header: "Notepad " + itoa(n), Slot: deskRack}
		d.nextID++
		desk.Items = append(desk.Items, p)
	}
	note := &dOct{ID: d.nextID, Kind: kNote, Header: "Buy milk", Body: "and eggs\nand bread", Color: "#f4d35e", Slot: deskBoard}
	d.nextID++
	desk.Items = append(desk.Items, note)
	desk.Header = "Studio"
	d.sel = desk.ID
	press(m, "enter")
	for _, j := range []int{1, 3} {
		px, py := deskSpot(t, m, desk, "desk:"+itoa(j))
		click(m, px, py, tea.MouseLeft)
		dragTo(m, 10, 36+j, 50, 38+j, tea.MouseLeft)
	}
	if z := os.Getenv("DESK_ZOOM"); z != "" {
		press(m, z)
	}
	t.Log("\n" + screenOf(m))
}
