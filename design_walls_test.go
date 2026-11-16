package main

import (
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// altGrR presses AltGr+R (¶ on a UK keyboard); shift: AltGr+Shift+R (®).
func altGrR(m *Model, shift bool) {
	if shift {
		m.Update(tea.KeyPressMsg(tea.Key{Code: '®', Text: "®"}))
		return
	}
	m.Update(tea.KeyPressMsg(tea.Key{Code: '¶', Text: "¶"}))
}

func altGrF(m *Model) { m.Update(tea.KeyPressMsg(tea.Key{Code: 'đ', Text: "đ"})) }

// tapAlt is a clean tap of the left Alt key (pressed and let go alone).
func tapAlt(m *Model) {
	k := tea.Key{Code: tea.KeyLeftAlt}
	m.Update(tea.KeyPressMsg(k))
	m.Update(tea.KeyReleaseMsg(k))
}

func TestWallsEachWallItsOwnBoard(t *testing.T) {
	m := designTest(t)
	h := m.hs()
	if r := h.room(h.Current); r.walls() != 4 || r.at() != 0 {
		t.Fatalf("a new room: %d walls, at %d; want 4, at 0", r.walls(), r.at())
	}
	placeKey(m, "p") // a page on wall 1
	first := m.ds()
	if len(first.octs) != 1 {
		t.Fatalf("wall 1 has %d structures, want 1", len(first.octs))
	}
	altGrR(m, false)
	if r := h.room(h.Current); r.at() != 1 || m.ds() == first || len(m.ds().octs) != 0 {
		t.Fatalf("AltGr+R: at wall %d with %d structures; want wall 2 (index 1), empty", r.at(), len(m.ds().octs))
	}
	if !strings.Contains(ansi.Strip(m.designToolbar()), "wall 2/4") {
		t.Fatalf("the toolbar does not say wall 2/4: %q", ansi.Strip(m.designToolbar()))
	}
	placeKey(m, "n") // a note on wall 2
	for range 3 {    // round the room: walls 3, 4, then 1 again
		altGrR(m, false)
	}
	if r := h.room(h.Current); r.at() != 0 || len(m.ds().octs) != 1 || m.ds().octs[0].Kind != kPage {
		t.Fatalf("4 turns should be back at wall 1 with its page: at %d, %d structures", r.at(), len(m.ds().octs))
	}
	altGrR(m, true) // AltGr+Shift+R: the one before, wall 4
	if r := h.room(h.Current); r.at() != 3 {
		t.Fatalf("AltGr+Shift+R from wall 1: at %d, want 3 (wall 4)", r.at())
	}
	// Kept on disk: a new start finds wall 2's note in wall 2's own file.
	m.ds().flushSave()
	if b := loadDesign(boardPath(h.Current, 1)); len(b.octs) != 1 || b.octs[0].Kind != kNote {
		t.Fatalf("wall 2's own file should hold its note: %d structures", len(b.octs))
	}
	m2, _ := NewModel("", 0)
	m2.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m2.setMode(designMode)
	if r := m2.hs().room(m2.hs().Current); r.at() != 3 {
		t.Fatalf("a new start: at wall %d, want the wall left on (3)", r.at())
	}
}

func TestWallsFloor(t *testing.T) {
	m := designTest(t)
	h := m.hs()
	placeKey(m, "p")
	altGrF(m)
	if r := h.room(h.Current); !r.Down || r.at() != wallFloor || len(m.ds().octs) != 0 {
		t.Fatalf("AltGr+F: down %v at %d, %d structures; want the empty floor", r.Down, r.at(), len(m.ds().octs))
	}
	if !strings.Contains(ansi.Strip(m.designToolbar()), "floor") {
		t.Fatalf("the toolbar does not say floor: %q", ansi.Strip(m.designToolbar()))
	}
	placeKey(m, "n") // something not finished, on the floor
	altGrR(m, false) // no turning while looking down
	if !h.room(h.Current).Down {
		t.Fatal("AltGr+R on the floor should not turn")
	}
	altGrF(m)
	if r := h.room(h.Current); r.Down || r.at() != 0 || len(m.ds().octs) != 1 {
		t.Fatalf("AltGr+F again: down %v at %d, %d structures; want wall 1 with its page", r.Down, r.at(), len(m.ds().octs))
	}
	altGrF(m)
	if n := len(m.ds().octs); n != 1 || m.ds().octs[0].Kind != kNote {
		t.Fatalf("the floor kept %d structures, want its note", n)
	}
}

func TestWallsAddWall(t *testing.T) {
	m := designTest(t)
	h := m.hs()
	altGrR(m, false) // wall 2
	placeKey(m, "p")
	altGrR(m, true) // wall 1
	roomKey(m, "w") // a new wall after wall 1: the old wall 2 is wall 3 now
	r := h.room(h.Current)
	if r.walls() != 5 || r.at() != 1 || len(m.ds().octs) != 0 {
		t.Fatalf("AltGr+N, W: %d walls, at %d, %d structures; want 5, at the new empty wall 2", r.walls(), r.at(), len(m.ds().octs))
	}
	altGrR(m, false)
	if len(m.ds().octs) != 1 || m.ds().octs[0].Kind != kPage {
		t.Fatalf("the old wall 2 should now be wall 3, with its page: %d structures", len(m.ds().octs))
	}
	for range 4 {
		altGrR(m, false)
	}
	if r.at() != 1 {
		t.Fatalf("5 walls round: at %d, want 1", r.at())
	}
}

func TestWallsSpinFrame(t *testing.T) {
	a, b := newBoardCanvas(20, 4), newBoardCanvas(20, 4)
	for y := range 4 {
		for x := range 20 {
			a.Lines[y][x].Content, a.Lines[y][x].Width = "a", 1
			b.Lines[y][x].Content, b.Lines[y][x].Width = "b", 1
		}
	}
	count := func(c *boardCanvas, s string) int {
		n := 0
		for _, l := range c.Lines {
			for _, cell := range l {
				if cell.Content == s {
					n++
				}
			}
		}
		return n
	}
	for _, dir := range []int{1, -1} {
		for _, vertical := range []bool{false, true} {
			s := &wallSpin{from: a, to: b, dir: dir, vertical: vertical}
			if f := spinFrame(s, 0); count(f, "a") != 80 {
				t.Fatalf("dir %d vertical %v: at the start all should be the wall left", dir, vertical)
			}
			if f := spinFrame(s, 1); count(f, "b") != 80 {
				t.Fatalf("dir %d vertical %v: at the end all should be the new wall", dir, vertical)
			}
			mid := spinFrame(s, 0.5)
			if count(mid, "a") == 0 || count(mid, "b") == 0 {
				t.Fatalf("dir %d vertical %v: half way both should show", dir, vertical)
			}
			first := mid.Lines[0][0].Content
			if vertical {
				first = mid.Lines[0][0].Content
			}
			if (dir == 1) != (first == "a") {
				t.Fatalf("dir %d vertical %v: the wall left should be on the side turned away from (got %q first)", dir, vertical, first)
			}
		}
	}
}

func TestWallsSpinShows(t *testing.T) {
	m := designTest(t)
	animAdvance := animClock(t)
	placeKey(m, "p")
	altGrR(m, false)
	if m.spin == nil {
		t.Fatal("AltGr+R should start a spin")
	}
	animAdvance(spinFor / 2)
	var sb strings.Builder
	m.designRows(&sb)
	if rows := strings.Split(strings.TrimSuffix(sb.String(), "\n"), "\n"); len(rows) != m.boardH() {
		t.Fatalf("a spin frame has %d rows, want %d", len(rows), m.boardH())
	}
	animAdvance(spinFor)
	m.spinStep()
	if m.spin != nil {
		t.Fatal("the spin should be over")
	}
}

func TestWallsAltSpaceAndAttach(t *testing.T) {
	m := designTest(t)
	d := m.ds()
	a := d.newKind(kPage, 0, 0)
	b := d.newKind(kNote, 40, 0)
	c := d.newKind(kPage, 80, 0)
	for _, o := range []*dOct{a, b} {
		d.sel = o.ID
		tapAlt(m)
		press(m, "space")
		m.Update(tea.KeyReleaseMsg(tea.Key{Code: tea.KeySpace}))
	}
	if !d.multi[a.ID] || !d.multi[b.ID] || d.multi[c.ID] {
		t.Fatalf("Alt, then Space on A and B: chosen %v", d.multi)
	}
	// Space alone (no Alt first) does not add.
	d.sel = c.ID
	d.altTapAt = time.Time{}
	press(m, "space")
	m.Update(tea.KeyReleaseMsg(tea.Key{Code: tea.KeySpace}))
	if d.multi[c.ID] {
		t.Fatal("Space without Alt first should not add to those chosen")
	}
	d.multi = map[int]bool{a.ID: true, b.ID: true}
	d.sel = a.ID
	press(m, "alt+a")
	if d.editing != partPlate || m.prompt != pNone {
		t.Fatalf("Alt+A should start writing a name plate on the board: editing %d, prompt %d", d.editing, m.prompt)
	}
	typeStr(m, "Pair")
	if !strings.Contains(screenOf(m), "Pair") {
		t.Fatalf("the plate is not written on the board:\n%s", screenOf(m))
	}
	press(m, "enter")
	if a.Plate != "Pair" || b.Plate != "Pair" || c.Plate != "" || d.editing != partNone {
		t.Fatalf("plates after Alt+A: %q %q %q", a.Plate, b.Plate, c.Plate)
	}
	// Alt+A with one chosen only says how.
	d.multi = map[int]bool{c.ID: true}
	press(m, "alt+a")
	if d.editing != partNone || !strings.Contains(m.msg, "two or more") {
		t.Fatalf("Alt+A on one: editing %d, said %q", d.editing, m.msg)
	}
}

func TestWallsMusicToggle(t *testing.T) {
	m := designTest(t)
	was := soundOn
	soundOn = true
	defer func() { soundOn = was }()
	if m.soundWanted() {
		t.Fatal("music should start off")
	}
	press(m, "m")
	if !m.hs().Music || !m.soundWanted() {
		t.Fatalf("M should turn the music on: %q", m.msg)
	}
	data, _ := os.ReadFile(housePath())
	if !strings.Contains(string(data), `"music": true`) {
		t.Fatalf("the choice is not kept: %s", data)
	}
	press(m, "m")
	if m.hs().Music || m.soundWanted() {
		t.Fatal("M again should turn it off")
	}
}

func TestWallsWindowsShowTown(t *testing.T) {
	townShown = true
	defer func() { townShown = false }()
	m := designTest(t)
	m.Update(tea.WindowSizeMsg{Width: 220, Height: 50})
	h := m.hs()
	r := h.room(h.Current)
	seed := roomSeed(r.ID)
	n := winCount(seed)
	if n < 1 || n > 4 {
		t.Fatalf("room 1 has %d windows, want 1 to 4", n)
	}
	total := 0
	for w := range r.walls() {
		r.Wall = w
		here := len(m.windowsHere())
		total += here
		if w == 0 && here == 0 {
			t.Fatal("the first wall always has a window")
		}
	}
	if total < n {
		t.Fatalf("the walls have %d windows, the room %d", total, n)
	}
	r.Wall = 0
	d := m.ds()
	d.camX, d.camY = 0, 0
	m.design.frame = ""
	if !m.windowsOnScreen() {
		t.Fatal("a new board's first view should show its window")
	}
	if c := m.windowsCmd(); c == nil {
		t.Log("(no tick in tests: designAnims is off)")
	}
	screen := screenOf(m)
	if !strings.ContainsAny(screen, "▓▒░█▄▀") {
		t.Fatalf("no window drawn:\n%s", screen)
	}
	r.Down = true
	if len(m.windowsHere()) != 0 {
		t.Fatal("the floor has no windows")
	}
}
