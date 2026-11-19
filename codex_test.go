package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func codexTest(t *testing.T) *Model {
	t.Helper()
	m := newTest(t, "text stays")
	m.Update(tea.WindowSizeMsg{Width: 110, Height: 34})
	m.setMode(codexMode)
	return m
}

// Make a codex from the library: its title, a text, a button, a second
// page; then read it, and it is all still there after loading it again.
func TestCodexMakeAndRead(t *testing.T) {
	m := codexTest(t)
	if s := screenOf(m); !strings.Contains(s, "No codexes yet") {
		t.Fatalf("the empty library should say so:\n%s", s)
	}
	press(m, "alt+n")
	c := m.cx()
	if c.cf == nil || !c.edit || c.writing != codexWriteTitle {
		t.Fatalf("Alt+N should open a new codex writing its title: %+v", c)
	}
	typeStr(m, "Sky Book")
	press(m, "tab") // on to the subtitle
	typeStr(m, "stars")
	press(m, "enter")
	if c.cf.Title != "Sky Book" || c.cf.Subtitle != "stars" || c.writing != 0 {
		t.Fatalf("title %q subtitle %q writing %d", c.cf.Title, c.cf.Subtitle, c.writing)
	}
	if s := screenOf(m); !strings.Contains(s, "stars") {
		t.Fatalf("the cover should show the subtitle:\n%s", s)
	}
	press(m, "pgdown")
	if c.page != 0 {
		t.Fatalf("PgDn from the cover: page %d", c.page)
	}
	press(m, "alt+n")
	press(m, "t")
	typeStr(m, "Hello\nthere")
	press(m, "esc")
	p := c.pageOf()
	if len(p.Els) != 1 || p.Els[0].Text != "Hello\nthere" {
		t.Fatalf("the text: %+v", p.Els)
	}
	press(m, "-") // it went into the title slot, big: back to normal size, so its words show as typed
	press(m, "alt+n")
	press(m, "b")
	press(m, "esc") // keeps the label it has
	if len(p.Els) != 2 || p.Els[1].Kind != ceButton || p.Els[1].Text != "Next" {
		t.Fatalf("the button: %+v", p.Els[1])
	}
	press(m, "alt+p")
	if len(c.cf.Pages) != 2 || c.page != 1 {
		t.Fatalf("Alt+P: %d pages, on %d", len(c.cf.Pages), c.page)
	}
	press(m, "pgup")
	press(m, "e") // reading
	if c.edit {
		t.Fatal("E should go back to reading")
	}
	if s := screenOf(m); !strings.Contains(s, "Hello") || !strings.Contains(s, "Next") || !strings.Contains(s, "page 1 of 2") {
		t.Fatalf("page 1 while reading:\n%s", s)
	}
	for range 3 { // Tab reaches the text first, then the button
		if c.focus == p.Els[1].ID {
			break
		}
		press(m, "tab")
	}
	press(m, "enter") // the Next button
	if c.page != 1 {
		t.Fatalf("the Next button should turn to page 2: on %d", c.page)
	}
	press(m, "home")
	if c.page != -1 {
		t.Fatal("Home should go to the cover")
	}
	cf, err := loadCodex(c.id)
	if err != nil || cf.Title != "Sky Book" || len(cf.Pages) != 2 || len(cf.Pages[0].Els) != 2 {
		t.Fatalf("on disk: %v %+v", err, cf)
	}
	press(m, "esc")
	if c.cf != nil || len(c.lib) != 1 {
		t.Fatalf("Esc should close it into the library: %d in it", len(c.lib))
	}
	if s := screenOf(m); !strings.Contains(s, "Sky Book") && !strings.Contains(s, "▄") {
		t.Fatalf("the library should show its cover:\n%s", s)
	}
}

// Every layout, every kind, animation and font draws at any size, wide or upright.
func TestCodexDrawsEverything(t *testing.T) {
	m := codexTest(t)
	id, cf, err := createCodex("All")
	if err != nil {
		t.Fatal(err)
	}
	for li := range 16 {
		p := &codexPage{Layout: li}
		for s := range len(codexLayouts[li].slots) + 1 {
			for _, k := range []string{ceText, ceButton, ceImage, ceSymbol, ceDrawing} {
				e := &codexEl{ID: cf.NextID, Kind: k, Slot: s, Text: "Words to show " + k, Font: cf.NextID % 16,
					Size: cf.NextID%4 + 1, Anim: cf.NextID % 16, Symbol: cf.NextID, Align: cf.NextID % 3,
					Ink: []codexInk{{1, 1, 2}, {50, 20, 9}}}
				cf.NextID++
				p.Els = append(p.Els, e)
			}
		}
		cf.Pages = append(cf.Pages, p)
	}
	if err := saveCodex(id, cf); err != nil {
		t.Fatal(err)
	}
	for _, size := range [][2]int{{110, 34}, {44, 14}, {220, 60}} {
		for _, tall := range []bool{false, true} {
			m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			m.codexOpen(id, true)
			c := m.cx()
			c.cf.Tall = tall
			for pi := -1; pi < len(c.cf.Pages); pi++ {
				m.codexShowPage(pi)
				c.pageAt = time.Now().Add(-1500 * time.Millisecond)
				_ = screenOf(m)
				c.edit = !c.edit
				_ = screenOf(m)
				c.edit = !c.edit
			}
		}
	}
}

// Activation: a button aimed at a text hides it, and again shows it; a
// "go to page" button turns there; a click on a thing activates it.
func TestCodexActivation(t *testing.T) {
	m := codexTest(t)
	id, cf, _ := createCodex("Play")
	txt := &codexEl{ID: 1, Kind: ceText, Text: "Peekaboo", On: codexOn{Do: "hide"}}
	btn := &codexEl{ID: 2, Kind: ceButton, Text: "Hide it", Slot: 1, Action: caActivate, Target: 1}
	jump := &codexEl{ID: 3, Kind: ceButton, Text: "To 3", Slot: 1, Action: caGoto, Target: 3}
	col := &codexEl{ID: 4, Kind: ceSymbol, Slot: 1, On: codexOn{Do: "colour", Color: "#ff0000"}}
	cf.Pages = []*codexPage{{Layout: 1, Els: []*codexEl{txt, btn, jump, col}}, {Layout: 0}, {Layout: 0}}
	cf.NextID = 5
	saveCodex(id, cf)
	m.codexOpen(id, false)
	press(m, "right") // the cover opens
	c := m.cx()
	if c.page != 0 || !strings.Contains(screenOf(m), "Peekaboo") {
		t.Fatalf("page 1: %d\n%s", c.page, screenOf(m))
	}
	c.turn = nil
	m.codexActivate(c.el(2), 0)
	if !c.look(c.el(1), time.Now()).hidden || strings.Contains(screenOf(m), "Peekaboo") {
		t.Fatal("the button should hide the text")
	}
	m.codexActivate(c.el(2), 0)
	if c.look(c.el(1), time.Now()).hidden {
		t.Fatal("again: the text should show")
	}
	// A click on the symbol changes its colour.
	screenOf(m)
	r := c.rects[4]
	click(m, r[0]+r[2]/2, r[1]+m.boardTop(), tea.MouseLeft)
	if l := c.look(c.el(4), time.Now()); l.color != "#ff0000" {
		t.Fatalf("a click should activate the symbol: colour %q", l.color)
	}
	// Tab reaches the buttons; Enter on "To 3" turns to page 3.
	for range 4 {
		if c.focus == 3 {
			break
		}
		press(m, "tab")
	}
	press(m, "enter")
	if c.page != 2 {
		t.Fatalf("the go-to button: page %d", c.page)
	}
}

// The activation table changes what activating does, and its value.
func TestCodexActivationTable(t *testing.T) {
	m := codexTest(t)
	id, cf, _ := createCodex("Table")
	cf.Pages[0].Els = []*codexEl{{ID: 1, Kind: ceText, Text: "Hi"}, {ID: 2, Kind: ceButton, Text: "Go", Action: caNext}}
	cf.NextID = 3
	saveCodex(id, cf)
	m.codexOpen(id, true)
	press(m, "pgdown")
	c := m.cx()
	c.sel = 1
	press(m, "t")
	if !c.table || !strings.Contains(screenOf(m), "Activation table") {
		t.Fatalf("T should open the table:\n%s", screenOf(m))
	}
	for c.el(1).On.Do != "colour" {
		press(m, "right")
	}
	press(m, "tab")
	was := c.el(1).On.Color
	press(m, "right")
	if c.el(1).On.Color == was || c.el(1).On.Color == "" {
		t.Fatalf("the value column should change the colour: %q → %q", was, c.el(1).On.Color)
	}
	// Swap: Enter writes the other words.
	press(m, "tab")
	for c.el(1).On.Do != "swap" {
		press(m, "right")
	}
	press(m, "enter")
	typeStr(m, "Bye")
	press(m, "esc")
	if c.el(1).On.Alt != "Bye" || !c.table {
		t.Fatalf("swap words %q, table %v", c.el(1).On.Alt, c.table)
	}
	// The button's row: its action.
	press(m, "down")
	press(m, "right")
	if c.el(2).Action != caPrev {
		t.Fatalf("the button's action: %q", c.el(2).Action)
	}
	press(m, "esc")
	if c.table {
		t.Fatal("Esc should close the table")
	}
	cf2, _ := loadCodex(id)
	if cf2.Pages[0].Els[0].On.Alt != "Bye" {
		t.Fatal("the table's changes should be saved")
	}
}

// Drawing: the brush, pen down, and the rubber.
func TestCodexDrawing(t *testing.T) {
	m := codexTest(t)
	id, _, _ := createCodex("Ink")
	m.codexOpen(id, true)
	press(m, "pgdown")
	press(m, "alt+n")
	press(m, "d")
	c := m.cx()
	if !c.drawing {
		t.Fatal("Alt+N, D should start drawing")
	}
	screenOf(m)
	press(m, "space")
	for range 5 {
		press(m, "right")
	}
	press(m, "space")
	e := c.el(c.sel)
	n := len(e.Ink)
	if n == 0 {
		t.Fatal("no ink")
	}
	press(m, "r")
	press(m, "space")
	press(m, "left")
	press(m, "space")
	if len(e.Ink) >= n {
		t.Fatalf("the rubber should take ink off: %d → %d", n, len(e.Ink))
	}
	press(m, "esc")
	if c.drawing {
		t.Fatal("Esc should stop drawing")
	}
	if !strings.ContainsAny(screenOf(m), "▀▄") {
		t.Fatal("the drawing should show")
	}
}

// Pictures are copied into the codex.
func TestCodexPicture(t *testing.T) {
	m := codexTest(t)
	id, _, _ := createCodex("Pics")
	m.codexOpen(id, true)
	press(m, "pgdown")
	press(m, "alt+n")
	press(m, "i")
	if m.prompt != pCodexImage {
		t.Fatal("Alt+N, I should ask for a picture")
	}
	m.closePrompt()
	png := writePNG(t, t.TempDir())
	m.codexUseImage(png, false)
	e := m.cx().el(m.cx().sel)
	if e.Image == "" {
		t.Fatal("no picture")
	}
	if _, err := os.Stat(filepath.Join(codexDir(id), "images", e.Image)); err != nil {
		t.Fatalf("the picture was not copied in: %v", err)
	}
	if !strings.Contains(screenOf(m), "▀") {
		t.Fatal("the picture should show")
	}
}

// On the Design board: Alt+N, K places a codex (its cover); Enter opens it
// in Codex mode; Esc comes back; it goes into a rack; P in the library
// puts one on the board.
func TestCodexOnDesignBoard(t *testing.T) {
	m := designTest(t)
	placeKey(m, "k")
	d := m.ds()
	o := d.byID(d.sel)
	if o == nil || o.Kind != kCodex || codexIDOf(o) == "" {
		t.Fatalf("Alt+N, K: %+v", o)
	}
	if s := screenOf(m); !strings.Contains(s, "Untitled") {
		t.Fatalf("the board should show its cover:\n%s", s)
	}
	press(m, "e")
	if !m.inCodex() || m.cx().cf == nil || !m.cx().edit || m.cx().writing != codexWriteTitle {
		t.Fatal("E should open it in Codex mode, writing the title")
	}
	typeStr(m, "Atlas")
	press(m, "enter")
	press(m, "esc") // edit mode: Esc closes it, back to the board
	if !m.inDesign() || d.sel != o.ID || o.Header != "Atlas" {
		t.Fatalf("back on the board: design %v, chosen %d, header %q", m.inDesign(), d.sel, o.Header)
	}
	if s := screenOf(m); !strings.Contains(s, "Atlas") && !strings.Contains(s, "▄") {
		t.Fatalf("the cover should show the new title:\n%s", s)
	}
	press(m, "enter")
	if !m.inCodex() || m.cx().edit {
		t.Fatal("Enter should open it to read")
	}
	press(m, "esc")
	rack := d.newKind(kRack, 0, 0)
	if !accepts(rack, o) {
		t.Fatal("a rack should take a codex")
	}
	// P in the library puts the codex on the board.
	m.setMode(codexMode)
	n := len(d.octs)
	press(m, "p")
	if !m.inDesign() || len(d.octs) != n+1 || d.byID(d.sel).Kind != kCodex {
		t.Fatal("P should put the chosen codex on the board")
	}
}

// Alt+N on the cover goes to the first page; layouts step through all 16;
// Alt+arrows move a thing between slots; X X deletes it, Ctrl+Z brings it back.
func TestCodexEditKeys(t *testing.T) {
	m := codexTest(t)
	id, _, _ := createCodex("Keys")
	m.codexOpen(id, true)
	c := m.cx()
	press(m, "alt+n")
	press(m, "s")
	if c.page != 0 || len(c.pageOf().Els) != 1 {
		t.Fatal("Alt+N, S on the cover should add a symbol to page 1")
	}
	seen := map[int]bool{}
	for range 16 {
		press(m, "l")
		seen[c.pageOf().Layout] = true
	}
	if len(seen) != 16 {
		t.Fatalf("L should step through 16 layouts: %d", len(seen))
	}
	for c.pageOf().Layout != 3 { // three columns
		press(m, "l")
	}
	e := c.el(c.sel)
	// Alt+Right: middle → right of slot 1, then slot 2 left, middle, right, then slot 3.
	for i, want := range [][2]int{{0, 2}, {1, 1}, {1, 0}, {1, 2}, {2, 1}} {
		press(m, "alt+right")
		if e.Slot != want[0] || e.Align != want[1] {
			t.Fatalf("Alt+Right %d: slot %d align %d, want %v", i+1, e.Slot, e.Align, want)
		}
	}
	press(m, "alt+right")
	press(m, "alt+right")
	press(m, "alt+right")
	if e.Slot != 2 || e.Align != 2 || !strings.Contains(m.msg, "as far right") {
		t.Fatalf("Alt+Right at the right edge should stay: slot %d, %q", e.Slot, m.msg)
	}
	press(m, "x")
	press(m, "x")
	if len(c.pageOf().Els) != 0 {
		t.Fatal("X X should delete it")
	}
	press(m, "ctrl+z")
	if len(c.pageOf().Els) != 1 {
		t.Fatal("Ctrl+Z should bring it back")
	}
}

// A page turns with a slide: part old page, part new, then the new one.
func TestCodexPageTurn(t *testing.T) {
	codexTurns = true
	defer func() { codexTurns = false }()
	m := codexTest(t)
	id, cf, _ := createCodex("Turn")
	cf.Pages[0].Els = []*codexEl{{ID: 1, Kind: ceText, Text: "Inside words"}}
	cf.NextID = 2
	saveCodex(id, cf)
	m.codexOpen(id, false)
	press(m, "right")
	c := m.cx()
	if c.turn == nil || c.page != 0 {
		t.Fatal("the page should be turning")
	}
	c.turn.at = time.Now().Add(-codexTurnDur(c.turn.anim) / 2)
	half := screenOf(m)
	if !strings.Contains(half, "═") { // the cover's frame, half covered
		t.Fatalf("half way: the cover should still show in part:\n%s", half)
	}
	c.turn.at = time.Now().Add(-2 * codexTurnDur(c.turn.anim))
	m.codexTicked()
	if c.turn != nil || !strings.Contains(screenOf(m), "Inside words") {
		t.Fatal("after the turn: the new page")
	}
}

// Editing on a wide screen: the inspector shows the chosen thing's options;
// Alt+N shows the add menu; a button's target and pressable things are marked.
func TestCodexSeeable(t *testing.T) {
	m := codexTest(t)
	id, cf, _ := createCodex("See")
	txt := &codexEl{ID: 1, Kind: ceText, Text: "Hello", On: codexOn{Do: "flash"}}
	hid := &codexEl{ID: 3, Kind: ceText, Text: "Secret", Slot: 1, On: codexOn{Do: "show"}}
	btn := &codexEl{ID: 2, Kind: ceButton, Text: "Go", Slot: 1, Action: caActivate, Target: 1}
	cf.Pages[0].Els = []*codexEl{txt, btn, hid}
	cf.NextID = 4
	saveCodex(id, cf)
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m.codexOpen(id, true)
	press(m, "pgdown")
	c := m.cx()
	c.sel = 1
	s := screenOf(m)
	for _, w := range []string{"Inspector", "font", "size", "colour", "animation", "activated", "flashes", "layout"} {
		if !strings.Contains(s, w) {
			t.Fatalf("the inspector should show %q:\n%s", w, s)
		}
	}
	if !strings.Contains(s, "hidden until activated") {
		t.Fatal("a thing hidden at first should say so while editing")
	}
	c.sel = 2
	if s := screenOf(m); !strings.Contains(s, "◎ target") || !strings.Contains(s, "action") {
		t.Fatalf("the button's target should be marked:\n%s", s)
	}
	press(m, "alt+n")
	if s := screenOf(m); !strings.Contains(s, "Add to this page") || !strings.Contains(s, "drawing") {
		t.Fatalf("Alt+N should show the add menu:\n%s", s)
	}
	press(m, "esc")
	press(m, "e") // reading
	if s := screenOf(m); !strings.Contains(s, "◈") || strings.Contains(s, "Inspector") {
		t.Fatalf("reading: pressable things marked, no inspector:\n%s", s)
	}
	press(m, "tab")
	if !strings.Contains(screenOf(m), "↵ press") {
		t.Fatal("the thing Tab reached should say ↵ press")
	}
	// A narrow screen has no room for the inspector.
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	press(m, "e")
	if strings.Contains(screenOf(m), "Inspector") {
		t.Fatal("no inspector on a narrow screen")
	}
}

// The first look's colours (cream paper, red cover, blue buttons) become the theme's.
func TestCodexOldColoursBecomeTheme(t *testing.T) {
	cf := &codexFile{Color: "#7a2e1d", Paper: "#f4ead5", Pages: []*codexPage{{Els: []*codexEl{{ID: 1, Kind: ceButton, Color: "#2c7fb8"}}}}}
	cf.tidy()
	if cf.Color != "" || cf.Paper != "" || cf.Pages[0].Els[0].Color != "" {
		t.Fatalf("old colours kept: %q %q %q", cf.Color, cf.Paper, cf.Pages[0].Els[0].Color)
	}
}
