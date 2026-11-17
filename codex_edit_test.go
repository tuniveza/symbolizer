package main

import (
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// codexEditOpen makes a codex called title and opens it to edit, on page 1.
func codexEditOpen(t *testing.T, m *Model, title string) *codexState {
	t.Helper()
	id, _, err := createCodex(title)
	if err != nil {
		t.Fatal(err)
	}
	m.codexOpen(id, true)
	press(m, "pgdown")
	return m.cx()
}

// codexSettled: nothing is left half done (writing, drawing, the add menu, the table).
func codexSettled(t *testing.T, c *codexState, when string) {
	t.Helper()
	if c.writing != 0 || c.drawing || c.placing || c.table || c.fresh || c.newPic != nil {
		t.Fatalf("%s: stuck: writing %d drawing %v placing %v table %v fresh %v newPic %v",
			when, c.writing, c.drawing, c.placing, c.table, c.fresh, c.newPic != nil)
	}
}

// 1 + 8: new things go into the first empty slot (title, then text); a
// text in the title slot is big; texts line up left, buttons and symbols in
// the middle; with every slot full, a new thing goes after the chosen one.
func TestCodexNewThingsFillSlots(t *testing.T) {
	for _, size := range [][2]int{{180, 50}, {44, 14}} {
		m := codexTest(t)
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		c := codexEditOpen(t, m, "Slots")
		press(m, "alt+n")
		press(m, "t")
		typeStr(m, "Big")
		press(m, "esc")
		press(m, "alt+n")
		press(m, "t")
		typeStr(m, "Body words")
		press(m, "esc")
		p := c.pageOf()
		if len(p.Els) != 2 {
			t.Fatalf("%v: two texts: %d", size, len(p.Els))
		}
		title, body := p.Els[0], p.Els[1]
		if title.Slot != 0 || body.Slot != 1 {
			t.Fatalf("%v: slots %d, %d: want the title slot, then the text slot", size, title.Slot, body.Slot)
		}
		if title.Size != 2 || max(body.Size, 1) != 1 {
			t.Fatalf("%v: sizes %d, %d: the title should be big, the text normal", size, title.Size, body.Size)
		}
		if title.Align != 0 || body.Align != 0 {
			t.Fatalf("%v: texts should be in the middle: %d %d", size, title.Align, body.Align)
		}
		if s := screenOf(m); !strings.Contains(s, "Body words") && size[0] > 100 {
			t.Fatalf("%v: the text should show:\n%s", size, s)
		}
		// Every slot is full: a button goes after the chosen thing (the body), in its slot.
		press(m, "alt+n")
		press(m, "b")
		press(m, "enter")
		btn := p.Els[len(p.Els)-1]
		if btn.Kind != ceButton || btn.Slot != 1 || btn.Align != 0 {
			t.Fatalf("%v: the button: kind %s slot %d align %d", size, btn.Kind, btn.Slot, btn.Align)
		}
		// Choosing the title and adding a symbol puts it after the title, in the title slot.
		c.sel = title.ID
		press(m, "alt+n")
		press(m, "s")
		if p.Els[1].Kind != ceSymbol || p.Els[1].Slot != 0 || p.Els[1].Align != 0 {
			t.Fatalf("%v: the symbol should follow the title: %+v", size, p.Els[1])
		}
		_ = screenOf(m)
		codexSettled(t, c, "after adding")
	}
}

// 2: cream paper chosen with W stays, after reopening and after Ctrl+Z; an
// old file (no version) still has its old colours moved to the theme's.
func TestCodexCreamPaperKept(t *testing.T) {
	m := codexTest(t)
	m.Update(tea.WindowSizeMsg{Width: 180, Height: 50})
	c := codexEditOpen(t, m, "Cream")
	id := c.id
	press(m, "home") // the cover: W is the pages' paper
	for range len(codexPapers) {
		if c.cf.Paper == "#f4ead5" {
			break
		}
		press(m, "w")
	}
	if c.cf.Paper != "#f4ead5" {
		t.Fatalf("W should reach cream paper: %q", c.cf.Paper)
	}
	press(m, "c") // the cover's colour: a change to undo
	press(m, "ctrl+z")
	if c.cf.Paper != "#f4ead5" {
		t.Fatalf("Ctrl+Z lost the cream paper: %q", c.cf.Paper)
	}
	press(m, "esc") // close
	if !m.codexOpen(id, true) || m.cx().cf.Paper != "#f4ead5" {
		t.Fatalf("reopened: paper %q", m.cx().cf.Paper)
	}
	// An old file, from before versions.
	old := &codexFile{Title: "Old", Color: "#7a2e1d", Paper: "#f4ead5", Pages: []*codexPage{{Layout: 1}}}
	oid := newCodexID() + "-old"
	if err := saveCodex(oid, old); err != nil {
		t.Fatal(err)
	}
	cf, err := loadCodex(oid)
	if err != nil || cf.Paper != "" || cf.Color != "" || cf.V != codexFileV {
		t.Fatalf("an old file: %v paper %q colour %q v %d", err, cf.Paper, cf.Color, cf.V)
	}
}

// 3: a new button's label: the first key typed replaces "Next"; Enter alone keeps it.
func TestCodexNewButtonLabel(t *testing.T) {
	m := codexTest(t)
	c := codexEditOpen(t, m, "Buttons")
	press(m, "alt+n")
	press(m, "b")
	typeStr(m, "Go")
	press(m, "enter")
	e := c.el(c.sel)
	if e.Text != "Go" {
		t.Fatalf("typed label: %q, want Go", e.Text)
	}
	press(m, "alt+n")
	press(m, "b")
	press(m, "enter")
	if e := c.el(c.sel); e.Text != "Next" {
		t.Fatalf("Enter with no typing: %q, want Next", e.Text)
	}
	// Writing an old button's label again adds to it, as normal.
	press(m, "enter")
	typeStr(m, "!")
	press(m, "enter")
	if e := c.el(c.sel); e.Text != "Next!" {
		t.Fatalf("an old label: %q, want Next!", e.Text)
	}
	codexSettled(t, c, "buttons")
}

// 4: Alt+N T then Esc, Alt+N I then the prompt cancelled (or a bad file),
// Alt+N D then Esc: nothing is left behind, and no undo step either.
func TestCodexEmptyNewThingsGo(t *testing.T) {
	m := codexTest(t)
	m.Update(tea.WindowSizeMsg{Width: 180, Height: 50})
	c := codexEditOpen(t, m, "Empty")
	p := c.pageOf()
	press(m, "alt+n")
	press(m, "t")
	press(m, "esc")
	if len(p.Els) != 0 || len(c.undo) != 0 {
		t.Fatalf("an empty new text should go: %d things, %d undo steps", len(p.Els), len(c.undo))
	}
	press(m, "alt+n")
	press(m, "i")
	if m.prompt != pCodexImage || len(p.Els) != 0 {
		t.Fatalf("Alt+N, I should ask first and add nothing yet: prompt %v, %d things", m.prompt, len(p.Els))
	}
	press(m, "esc") // cancel the prompt
	press(m, "right")
	if len(p.Els) != 0 || m.prompt != pNone {
		t.Fatalf("a cancelled picture should leave nothing: %d things", len(p.Els))
	}
	press(m, "alt+n")
	press(m, "i")
	typeStr(m, "/no/such/picture.png")
	press(m, "enter")
	if len(p.Els) != 0 {
		t.Fatalf("a bad picture should leave nothing: %d things", len(p.Els))
	}
	press(m, "alt+n")
	press(m, "d")
	press(m, "esc")
	if len(p.Els) != 0 || len(c.undo) != 0 {
		t.Fatalf("an empty new drawing should go: %d things, %d undo steps", len(p.Els), len(c.undo))
	}
	codexSettled(t, c, "after the empty things")
	// A good picture is added at once, as one step.
	press(m, "alt+n")
	press(m, "i")
	typeStr(m, writePNG(t, t.TempDir()))
	press(m, "enter")
	if len(p.Els) != 1 || p.Els[0].Kind != ceImage || p.Els[0].Image == "" || len(c.undo) != 1 {
		t.Fatalf("a picture: %d things, %d undo steps", len(p.Els), len(c.undo))
	}
	if s := screenOf(m); !strings.Contains(s, "▀") {
		t.Fatalf("the picture should show:\n%s", s)
	}
	cf, _ := loadCodex(c.id)
	if len(cf.Pages[0].Els) != 1 {
		t.Fatalf("on disk: %d things", len(cf.Pages[0].Els))
	}
}

// 5: adding and writing a text is one undo step; Enter then Esc with no
// change is none; a real change is one.
func TestCodexUndoSteps(t *testing.T) {
	m := codexTest(t)
	c := codexEditOpen(t, m, "Steps")
	press(m, "alt+n")
	press(m, "t")
	typeStr(m, "Hi")
	press(m, "esc")
	if len(c.undo) != 1 {
		t.Fatalf("adding a text: %d undo steps, want 1", len(c.undo))
	}
	press(m, "enter")
	press(m, "esc")
	if len(c.undo) != 1 {
		t.Fatalf("Enter, Esc with no change: %d undo steps, want 1", len(c.undo))
	}
	press(m, "enter")
	typeStr(m, " there")
	press(m, "esc")
	if len(c.undo) != 2 || c.el(c.sel).Text != "Hi there" {
		t.Fatalf("a change: %d undo steps, text %q", len(c.undo), c.el(c.sel).Text)
	}
	press(m, "ctrl+z")
	if e := c.el(c.sel); e == nil || e.Text != "Hi" {
		t.Fatalf("one Ctrl+Z: %+v", e)
	}
	press(m, "ctrl+z")
	if len(c.pageOf().Els) != 0 {
		t.Fatal("a second Ctrl+Z should take the whole text away")
	}
	press(m, "ctrl+y")
	press(m, "ctrl+y")
	if e := c.el(c.sel); e == nil || e.Text != "Hi there" {
		t.Fatalf("redo twice: %+v", e)
	}
	codexSettled(t, c, "undo")
}

// 6: words being typed are kept when the mode changes and when sy quits.
func TestCodexWordsKeptOnLeave(t *testing.T) {
	m := codexTest(t)
	c := codexEditOpen(t, m, "Leave")
	id := c.id
	press(m, "alt+n")
	press(m, "t")
	typeStr(m, "Half typed")
	m.setMode(0) // Alt+M, then Text mode
	if c.writing != 0 {
		t.Fatal("leaving Codex mode should end writing")
	}
	cf, _ := loadCodex(id)
	if len(cf.Pages[0].Els) != 1 || cf.Pages[0].Els[0].Text != "Half typed" {
		t.Fatalf("the words should be saved: %+v", cf.Pages[0].Els)
	}
	m.setMode(codexMode)
	press(m, "alt+n")
	press(m, "t")
	typeStr(m, "Before quitting")
	press(m, "alt+q")
	if c.writing != 0 {
		t.Fatal("Alt+Q should end writing")
	}
	cf, _ = loadCodex(id)
	if len(cf.Pages[0].Els) != 2 || cf.Pages[0].Els[1].Text != "Before quitting" {
		t.Fatalf("the words should be saved on quitting: %d things", len(cf.Pages[0].Els))
	}
}

// 7: keys that do nothing for a kind of thing say so and change nothing;
// the activation table offers only what does something.
func TestCodexKeysRefuse(t *testing.T) {
	m := codexTest(t)
	m.Update(tea.WindowSizeMsg{Width: 180, Height: 50})
	id, cf, _ := createCodex("Refuse")
	cf.Pages[0].Layout = 8 // four slots
	cf.Pages[0].Els = []*codexEl{
		{ID: 1, Kind: ceImage, Slot: 0},
		{ID: 2, Kind: ceDrawing, Slot: 1, Ink: []codexInk{{1, 1, 2}}},
		{ID: 3, Kind: ceButton, Slot: 2, Text: "Go", Action: caNext},
		{ID: 4, Kind: ceSymbol, Slot: 3},
		{ID: 5, Kind: ceText, Slot: 3, Text: "Words"},
	}
	cf.NextID = 6
	saveCodex(id, cf)
	m.codexOpen(id, true)
	press(m, "pgdown")
	c := m.cx()
	for _, k := range []struct {
		id   int
		key  string
		says string
	}{{1, "c", "Colour is for"}, {2, "c", "Colour is for"}, {1, "j", "Lining up is for"}, {2, "j", "Lining up is for"},
		{3, "=", "Size is for texts and symbols."}, {3, "-", "Size is for texts and symbols."}, {1, "=", "Size is for"}, {2, "-", "Size is for"}} {
		c.sel = k.id
		was := *c.el(k.id)
		steps := len(c.undo)
		press(m, k.key)
		now := *c.el(k.id)
		if now.Color != was.Color || now.Align != was.Align || now.Size != was.Size || len(c.undo) != steps {
			t.Fatalf("%s on thing %d changed it", k.key, k.id)
		}
		if !strings.Contains(m.msg, k.says) {
			t.Fatalf("%s on thing %d says %q, want %q", k.key, k.id, m.msg, k.says)
		}
	}
	c.sel = 5
	press(m, "=")
	if c.el(5).Size != 2 {
		t.Fatal("+ should still grow a text")
	}
	// The activation table: only effects that do something.
	press(m, "t")
	for row, el := range c.pageOf().Els {
		if el.Kind == ceButton {
			continue
		}
		c.tableRow, c.tableCol = row, 0
		seen := map[string]bool{}
		for range len(codexDos) + 1 {
			press(m, "right")
			seen[el.On.Do] = true
		}
		for do := range seen {
			if !codexDoFits(el.Kind, do) {
				t.Fatalf("the table offered %q for a %s", do, el.Kind)
			}
		}
		if !seen["flash"] || !seen["hide"] {
			t.Fatalf("the table should still offer flash and hide for a %s: %v", el.Kind, seen)
		}
	}
	for _, bad := range [][2]string{{ceDrawing, "font"}, {ceDrawing, "grow"}, {ceDrawing, "swap"}, {ceSymbol, "font"}, {ceImage, "colour"}, {ceImage, "grow"}} {
		if codexDoFits(bad[0], bad[1]) {
			t.Fatalf("%s should not offer %s", bad[0], bad[1])
		}
	}
	press(m, "esc")
	codexSettled(t, c, "the table")
}

// 8: PgDn on the last page says Alt+P adds one; Esc in edit mode closes;
// a new codex left empty is not kept (but one on the Design board is).
func TestCodexEditDefaults(t *testing.T) {
	m := codexTest(t)
	m.Update(tea.WindowSizeMsg{Width: 180, Height: 50})
	c := codexEditOpen(t, m, "Defaults")
	press(m, "pgdown")
	if c.page != 0 || !strings.Contains(m.msg, "Alt+P adds a page") {
		t.Fatalf("PgDn on the last page: page %d, says %q", c.page, m.msg)
	}
	press(m, "alt+n")
	press(m, "b")
	press(m, "enter")
	press(m, "space")
	if !strings.Contains(m.msg, "Alt+P adds a page") {
		t.Fatalf("Space on Next on the last page says %q", m.msg)
	}
	press(m, "esc")
	if c.cf != nil {
		t.Fatal("Esc in edit mode should close the codex")
	}
	if len(codexList()) != 1 {
		t.Fatalf("a codex with a title is kept: %d in the library", len(codexList()))
	}
	// Alt+N in the library, then Esc (the title) and Esc (close): not kept.
	press(m, "alt+n")
	if c.cf == nil || c.writing != codexWriteTitle {
		t.Fatal("Alt+N should open a new codex, writing its title")
	}
	newID := c.id
	press(m, "esc")
	codexSettled(t, c, "after the title")
	press(m, "esc")
	if c.cf != nil {
		t.Fatal("two Escs should close a new codex")
	}
	if _, err := os.Stat(codexDir(newID)); !os.IsNotExist(err) {
		t.Fatalf("an empty new codex should not be kept: %v", err)
	}
	if len(c.lib) != 1 || !strings.Contains(m.msg, "not kept") {
		t.Fatalf("the library: %d, says %q", len(c.lib), m.msg)
	}
	if s := screenOf(m); strings.Contains(s, "Untitled") {
		t.Fatalf("no Untitled codex should be left in the library:\n%s", s)
	}
	// Reading: E then Esc still closes; E toggles.
	m.codexOpen(c.lib[0].id, true)
	press(m, "e")
	if c.edit {
		t.Fatal("E should go to reading")
	}
	press(m, "e")
	if !c.edit {
		t.Fatal("E again should edit")
	}
	press(m, "esc")
	if c.cf != nil {
		t.Fatal("Esc should close")
	}
}

// 8: an empty codex on the Design board is kept when closed.
func TestCodexEmptyOnBoardKept(t *testing.T) {
	m := designTest(t)
	placeKey(m, "k")
	o := m.ds().byID(m.ds().sel)
	id := codexIDOf(o)
	press(m, "e")
	press(m, "esc") // the title
	press(m, "esc") // close: back to the board
	if !m.inDesign() {
		t.Fatal("two Escs should go back to the board")
	}
	if _, err := os.Stat(codexDir(id)); err != nil {
		t.Fatalf("a codex on the board must be kept: %v", err)
	}
	// Opened from the library (not from the board), it is still kept: the board shows it.
	m.setMode(codexMode)
	if !m.codexOpen(id, true) {
		t.Fatal("open")
	}
	press(m, "esc")
	if _, err := os.Stat(codexDir(id)); err != nil {
		t.Fatalf("a codex the board shows must be kept: %v", err)
	}
}

// 9: Esc and Ctrl+Z leave nothing stuck, from any state.
func TestCodexNothingStuck(t *testing.T) {
	m := codexTest(t)
	m.Update(tea.WindowSizeMsg{Width: 44, Height: 14})
	c := codexEditOpen(t, m, "Stuck")
	press(m, "alt+n")
	press(m, "d")
	screenOf(m)
	press(m, "space")
	press(m, "right")
	press(m, "ctrl+z") // the ink and the drawing's step go
	press(m, "esc")
	codexSettled(t, c, "draw, undo, Esc")
	press(m, "alt+n")
	press(m, "ctrl+z") // the add menu takes any key: nothing added
	codexSettled(t, c, "the add menu")
	press(m, "alt+n")
	press(m, "t")
	typeStr(m, "x")
	press(m, "esc")
	press(m, "t")
	press(m, "ctrl+z") // nothing in the table: Ctrl+Z is not for it
	press(m, "esc")
	codexSettled(t, c, "the table")
	press(m, "ctrl+z")
	press(m, "ctrl+z")
	press(m, "ctrl+z")
	codexSettled(t, c, "undo all")
	if c.cf == nil {
		t.Fatal("still open")
	}
	_ = screenOf(m)
}

// Codex files from before version 3 have every text and button centred once.
func TestCodexOldTextCentred(t *testing.T) {
	cf := &codexFile{V: 2, Pages: []*codexPage{{Els: []*codexEl{{ID: 1, Kind: ceText, Align: 1}, {ID: 2, Kind: ceButton, Align: 2}}}}}
	cf.tidy()
	if cf.Pages[0].Els[0].Align != 0 || cf.Pages[0].Els[1].Align != 0 || cf.V != codexFileV {
		t.Fatalf("not centred: %+v", cf.Pages[0].Els)
	}
	cf.Pages[0].Els[0].Align = 1 // lined up left later: kept
	cf.tidy()
	if cf.Pages[0].Els[0].Align != 1 {
		t.Fatal("a later choice must stay")
	}
}
