package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// visualCodex makes a codex that shows the hard cases: big text that must
// shrink and wrap, a long paragraph, a right-aligned button, a hidden thing,
// a slot too full, the card layout and light paper.
func visualCodex(t *testing.T) (string, *codexFile) {
	t.Helper()
	id, cf, err := createCodex("The Sky Book of Stars and Far Away Moons")
	if err != nil {
		t.Fatal(err)
	}
	cf.Subtitle = "notes from the night garden"
	long := strings.Repeat("The night sky is full of quiet fires that burn far away. ", 6)
	p1 := &codexPage{Layout: 1, Els: []*codexEl{
		{ID: 1, Kind: ceText, Text: "Welcome to the Sky Book", Size: 4},
		{ID: 2, Kind: ceText, Slot: 1, Text: long, Align: 1},
		{ID: 3, Kind: ceButton, Slot: 1, Text: "Next", Align: 2, Action: caNext},
		{ID: 4, Kind: ceText, Slot: 1, Text: "A secret line", On: codexOn{Do: "show"}},
	}}
	var full []*codexEl
	for i := range 9 {
		full = append(full, &codexEl{ID: 10 + i, Kind: ceText, Text: "Line number " + itoa(i+1) + " of a slot that is far too full to hold them"})
	}
	full = append(full, &codexEl{ID: 30, Kind: ceImage, Slot: 1}, &codexEl{ID: 31, Kind: ceDrawing, Slot: 1})
	p2 := &codexPage{Layout: 2, Els: full}
	p3 := &codexPage{Layout: 15, Els: []*codexEl{
		{ID: 40, Kind: ceText, Text: "On a card", Size: 2},
		{ID: 41, Kind: ceButton, Text: "Back", Action: caGoto, Target: 1},
	}}
	p4 := &codexPage{Layout: 8, Paper: "#fff0d6", Els: []*codexEl{
		{ID: 50, Kind: ceSymbol, Symbol: 2, Size: 2, On: codexOn{Do: "flash"}},
		{ID: 51, Kind: ceText, Slot: 1, Text: "Light paper words"},
		{ID: 52, Kind: ceButton, Slot: 2, Text: "Show", Action: caActivate, Target: 51},
	}}
	cf.Pages = []*codexPage{p1, p2, p3, p4}
	cf.NextID = 60
	if err := saveCodex(id, cf); err != nil {
		t.Fatal(err)
	}
	return id, cf
}

var visualSizes = [][2]int{{180, 50}, {120, 40}, {92, 30}}

// TestCodexVisualDump writes every page at every size to SY_CODEX_DUMP (a
// folder), to look at by eye. Without it, it only draws them.
func TestCodexVisualDump(t *testing.T) {
	dir := os.Getenv("SY_CODEX_DUMP")
	m := codexTest(t)
	id, cf := visualCodex(t)
	for _, size := range visualSizes {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for _, edit := range []bool{true, false} {
			m.codexOpen(id, edit)
			c := m.cx()
			for pi := -1; pi < len(cf.Pages); pi++ {
				m.codexShowPage(pi)
				c.pageAt = time.Now().Add(-5 * time.Second)
				c.sel, c.focus = 0, 0
				if pi == 0 {
					c.sel, c.focus = 3, 3
				}
				if pi == 3 {
					c.sel, c.focus = 52, 50
				}
				s := screenOf(m)
				if dir != "" {
					name := itoa(size[0]) + "x" + itoa(size[1]) + map[bool]string{true: "-edit", false: "-read"}[edit] + "-p" + itoa(pi+1) + ".txt"
					os.WriteFile(filepath.Join(dir, name), []byte(s), 0o644)
				}
			}
		}
	}
	// Writing: big words on page 1, and the cover's title.
	m.Update(tea.WindowSizeMsg{Width: 180, Height: 50})
	m.codexOpen(id, true)
	m.codexShowPage(0)
	c := m.cx()
	c.sel = 1
	m.codexStartWrite(codexWriteEl)
	typeStr(m, " now")
	big := screenOf(m)
	press(m, "esc")
	m.codexShowPage(-1)
	m.codexStartWrite(codexWriteTitle)
	typeStr(m, " X")
	cover := screenOf(m)
	press(m, "esc")
	if dir != "" {
		os.WriteFile(filepath.Join(dir, "writing-big.txt"), []byte(big), 0o644)
		os.WriteFile(filepath.Join(dir, "writing-cover.txt"), []byte(cover), 0o644)
	}
}

// canvasRows is the codex canvas as plain text rows (board coordinates).
func canvasRows(m *Model) []string {
	cv := m.codexCanvas()
	rows := make([]string, cv.Height())
	for y := range rows {
		var b strings.Builder
		for x := 0; x < cv.Width(); x++ {
			if g := cv.Lines[y][x].Content; g != "" {
				b.WriteString(g)
			} else {
				b.WriteString(" ")
			}
		}
		rows[y] = b.String()
	}
	return rows
}

// runeAt is the letter at column x of row s.
func runeAt(s string, x int) string {
	rs := []rune(s)
	if x < 0 || x >= len(rs) {
		return ""
	}
	return string(rs[x])
}

// Labels read on every colour a button can be: the ink with more contrast wins.
func TestCodexInkContrast(t *testing.T) {
	if r := contrastOf(codexInkOn(cEmber), cEmber); r < 4.5 {
		t.Fatalf("the default button's label: contrast %.2f", r)
	}
	for _, col := range codexPalette {
		ink := codexInkOn(col)
		other := cStatusText
		if ink == cStatusText {
			other = "#2b1608"
		}
		if contrastOf(ink, col) < contrastOf(other, col) {
			t.Fatalf("on %s the ink %s reads worse than %s", col, ink, other)
		}
	}
	if r := contrastOf(codexAtLeast(cAccent, "#fff0d6", 3), "#fff0d6"); r < 3 {
		t.Fatalf("gold on light paper: contrast %.2f", r)
	}
	// The page stands out from the desk.
	if relLum(codexThemePaper()) < 2*relLum(codexDesk()) {
		t.Fatalf("paper %s too close to desk %s", codexThemePaper(), codexDesk())
	}
}

// Big text takes the biggest size that fits, wraps by words, and drops to
// normal letters when no big size fits.
func TestCodexBigTextFits(t *testing.T) {
	w3 := codexMeasure([]rune("Welcome"), 0, 3) // "Welcome" fits at size 3, not 4
	for _, w := range []int{200, w3, 30, 12} {
		rows, size, _ := codexTextRows(codexLook{text: "Welcome to the sky", size: 4}, w, 0)
		for _, r := range rows {
			if codexWidth(r) > w {
				t.Fatalf("width %d size %d: a row %d wide", w, size, codexWidth(r))
			}
		}
		if w == 200 && size != 4 || w == w3 && size != 3 || w == 12 && size != 1 {
			t.Fatalf("width %d: size %d", w, size)
		}
	}
	// Size 3 is tried before size 2, and big text is broken between words.
	lay := codexLay([]rune("Hi there you"), 0, 3, 40, 40, 0)
	if lay.size != 3 || len(lay.lines) < 2 {
		t.Fatalf("size %d lines %d", lay.size, len(lay.lines))
	}
	for _, l := range lay.lines {
		if s := string(trimSpaces(l.s)); s != "Hi" && s != "there" && s != "you" && s != "Hi there" && s != "there you" {
			t.Fatalf("a big line cut in a word: %q", s)
		}
	}
	// A height limit picks a smaller size.
	if lay := codexLay([]rune("Hi"), 0, 4, 100, 100, 6); lay.size != 3 {
		t.Fatalf("6 rows high: size %d", lay.size)
	}
}

// The slot edges sit outside the slots, so a full slot's text is not drawn
// over; nothing spills out of the writing area; things that do not fit say so.
func TestCodexSlotsAndOverflow(t *testing.T) {
	m := codexTest(t)
	id, _ := visualCodex(t)
	m.Update(tea.WindowSizeMsg{Width: 92, Height: 24})
	m.codexOpen(id, true)
	m.codexShowPage(1)
	c := m.cx()
	rows := canvasRows(m)
	all := strings.Join(rows, "\n")
	if strings.Contains(all, "┈") {
		t.Fatal("no dashed slot edges inside the slots any more")
	}
	if !strings.Contains(all, "▾ cut off") {
		t.Fatalf("a slot too full should say so:\n%s", all)
	}
	m.Update(tea.WindowSizeMsg{Width: 92, Height: 14}) // too low for one row each
	if all := strings.Join(canvasRows(m), "\n"); !strings.Contains(all, " more") && !strings.Contains(all, "▾6") {
		t.Fatalf("things with no room should say ▾ N more:\n%s", all)
	}
	m.Update(tea.WindowSizeMsg{Width: 92, Height: 24})
	rows = canvasRows(m)
	in := c.inner
	for id, r := range c.rects {
		if r[0] < in[0] || r[1] < in[1] || r[0]+r[2] > in[0]+in[2] || r[1]+r[3] > in[1]+in[3] {
			t.Fatalf("thing %d at %v outside %v", id, r, in)
		}
	}
	// Selection: corners only on empty paper, never over the next thing's letters.
	c.sel = 11
	sel := canvasRows(m)
	r := c.rects[11]
	for _, y := range []int{r[1] - 1, r[1] + r[3]} {
		for x := r[0] - 1; x <= r[0]+r[2]; x++ {
			if was := runeAt(rows[y], x); was != " " && runeAt(sel[y], x) != was {
				t.Fatalf("the selection drew over %q at %d,%d", was, x, y)
			}
		}
	}
}

// Marks stay on the page: a right-aligned button's tag, "hidden until
// activated" next to the thing, not over its words.
func TestCodexMarksStayOnPage(t *testing.T) {
	m := codexTest(t)
	id, _ := visualCodex(t)
	for _, size := range visualSizes {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m.codexOpen(id, true)
		m.codexShowPage(0)
		c := m.cx()
		c.sel = 3
		rows := canvasRows(m)
		in := c.inner
		found := false
		for y, row := range rows {
			if y < in[1]-1 { // the inspector above the page
				continue
			}
			if i := strings.Index(row, "next page"); i >= 0 {
				found = true
				x := len([]rune(row[:i]))
				if x+len("next page") > in[0]+in[2]+1 || y < in[1] || y > in[1]+in[3] {
					t.Fatalf("%v: the button's tag at %d,%d is off the page's area %v", size, x, y, in)
				}
			}
		}
		if !found {
			t.Fatalf("%v: no button tag:\n%s", size, strings.Join(rows, "\n"))
		}
		r := c.rects[4]
		if !strings.Contains(rows[r[1]], "A secret line") {
			t.Fatalf("%v: the hidden thing's words are covered: %q", size, rows[r[1]])
		}
		if !strings.Contains(strings.Join(rows, "\n"), "hidden until activated") {
			t.Fatalf("%v: no hidden tag", size)
		}
	}
}

// Normal text keeps to a readable block; symbols read on light paper.
func TestCodexReadable(t *testing.T) {
	m := codexTest(t)
	id, _ := visualCodex(t)
	m.Update(tea.WindowSizeMsg{Width: 180, Height: 50})
	m.codexOpen(id, false)
	m.codexShowPage(0)
	c := m.cx()
	screenOf(m)
	if r := c.rects[2]; r[2] > codexReadW {
		t.Fatalf("the paragraph is %d wide", r[2])
	}
	m.codexShowPage(3)
	c.pageAt = time.Now().Add(-5 * time.Second)
	cv := m.codexCanvas()
	r := c.rects[50]
	checked := 0
	for y := r[1]; y < r[1]+r[3]; y++ {
		for x := r[0]; x < r[0]+r[2]; x++ {
			cell := cv.Lines[y][x]
			if strings.TrimSpace(cell.Content) == "" {
				continue
			}
			fg, bg := colorHex(cell.Style.Fg, ""), colorHex(cell.Style.Bg, "")
			if contrastOf(fg, bg) < 3 {
				t.Fatalf("the symbol on light paper: %s on %s", fg, bg)
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("no symbol cells found")
	}
	// The default button's label reads (#ffa66b on #ff6600 was 1.5:1).
	m.codexShowPage(0)
	cv = m.codexCanvas()
	r = c.rects[3]
	label := 0
	for x := r[0]; x < r[0]+r[2]; x++ {
		cell := cv.Lines[r[1]+1][x]
		if strings.TrimSpace(cell.Content) == "" || cell.Content == "│" {
			continue
		}
		fg, bg := colorHex(cell.Style.Fg, ""), colorHex(cell.Style.Bg, "")
		if contrastOf(fg, bg) < 4.5 {
			t.Fatalf("the button label: %s on %s", fg, bg)
		}
		label++
	}
	if label != len("Next") {
		t.Fatalf("found %d label letters", label)
	}
}

// While writing, the words wrap and line up as they will when done.
func TestCodexWritingLooksDone(t *testing.T) {
	m := codexTest(t)
	id, cf, _ := createCodex("Write")
	words := "The night sky is full of quiet fires that burn far away and the moon is a lamp"
	cf.Pages[0].Els = []*codexEl{{ID: 1, Kind: ceText, Text: words}}
	cf.Pages[0].Layout = 2 // two columns: a narrow slot
	cf.NextID = 2
	saveCodex(id, cf)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.codexOpen(id, true)
	m.codexShowPage(0)
	c := m.cx()
	c.sel = 1
	done := canvasRows(m)
	box := c.rects[1]
	m.codexStartWrite(codexWriteEl)
	writing := canvasRows(m)
	cv, cursor := m.codexCanvas(), false
	for y := box[1]; y < box[1]+box[3]+1 && !cursor; y++ {
		for x := box[0] - 1; x <= box[0]+box[2]; x++ {
			if colorHex(cv.Lines[y][x].Style.Bg, "") == cAccent {
				cursor = true
			}
		}
	}
	if !cursor {
		t.Fatal("no cursor while writing")
	}
	lines := func(rows []string) []string { // the text's own columns
		var out []string
		for y, r := range rows {
			if y < box[1] || y >= c.inner[1]+c.inner[3] {
				continue
			}
			if strings.Contains(r, "night") || strings.Contains(r, "quiet") || strings.Contains(r, "lamp") {
				out = append(out, string([]rune(r)[box[0]:box[0]+box[2]]))
			}
		}
		return out
	}
	a, b := lines(done), lines(writing)
	if len(a) == 0 || len(a) != len(b) {
		t.Fatalf("writing breaks the lines differently:\n%q\n%q", a, b)
	}
	for i := range a { // the same words in the same places (the cursor aside)
		if strings.TrimRight(a[i], " ▕") != strings.TrimRight(b[i], " ▕") {
			t.Fatalf("line %d moves while writing:\n%q\n%q", i, a[i], b[i])
		}
	}
	for _, l := range b {
		for _, w := range strings.Fields(strings.Trim(l, " ▏▕")) {
			if !strings.Contains(words, w) {
				t.Fatalf("a word cut while writing: %q", w)
			}
		}
	}
}

// Typing a title or subtitle shows on the cover at once; so do the other
// words a swap changes to.
func TestCodexTypingShows(t *testing.T) {
	m := codexTest(t)
	press(m, "alt+n")
	typeStr(m, "Moon over the quiet water garden")
	if s := screenOf(m); !strings.Contains(s, "Moon over the") {
		t.Fatalf("the title being typed should show:\n%s", s)
	}
	press(m, "tab")
	typeStr(m, "little stars")
	if s := screenOf(m); !strings.Contains(s, "little stars") {
		t.Fatalf("the subtitle being typed should show:\n%s", s)
	}
	press(m, "enter")

	id, cf, _ := createCodex("Swap")
	cf.Pages[0].Els = []*codexEl{{ID: 1, Kind: ceText, Text: "Before", On: codexOn{Do: "swap"}}}
	cf.NextID = 2
	saveCodex(id, cf)
	m.codexOpen(id, true)
	m.codexShowPage(0)
	c := m.cx()
	c.sel = 1
	m.codexStartWrite(codexWriteAlt)
	typeStr(m, "After all")
	if s := screenOf(m); !strings.Contains(s, "After all") || !strings.Contains(s, "the other words") {
		t.Fatalf("the other words being typed should show:\n%s", s)
	}
}

// Library tiles grow on a wide screen, say when more are below, and cut
// long titles with "…".
func TestCodexLibraryTiles(t *testing.T) {
	m := codexTest(t)
	for i := range 8 {
		createCodex("A very long title for codex number " + itoa(i) + " that will not fit on its little cover at all")
	}
	m.Update(tea.WindowSizeMsg{Width: 92, Height: 30})
	m.setMode(0)
	m.setMode(codexMode)
	c := m.cx()
	s := screenOf(m)
	if !strings.Contains(s, "more") || !strings.Contains(s, "↓") {
		t.Fatalf("92×30 should say more codexes are below:\n%s", s)
	}
	if !strings.Contains(s, "…") {
		t.Fatalf("long titles should end in …:\n%s", s)
	}
	m.Update(tea.WindowSizeMsg{Width: 180, Height: 50})
	screenOf(m)
	if b := c.box["lib:0"]; b[2] <= codexTileW || b[2] > codexTileMaxW {
		t.Fatalf("180 wide: tile %d wide", b[2])
	}
	if cols := m.codexLibCols(); c.box["lib:"+itoa(cols)][1] <= c.box["lib:0"][1] {
		t.Fatal("the second row of tiles should be below the first, as the keys count columns")
	}
}
