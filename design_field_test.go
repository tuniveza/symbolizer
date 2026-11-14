package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// writePage is a Design board with a page being written in, its text s
// typed, the cursor at its end.
func writePage(t *testing.T, s string) (*Model, *dOct) {
	m := designTest(t)
	d := m.ds()
	d.camX, d.camY, d.zoom = 0, 0, designZoom100
	o := d.newKind(kPage, 0, 0)
	o.X, o.Y, o.W, o.H = 10, 3, 40, 14
	d.dirty = true
	d.save() // placing it is a step of its own
	d.sel, d.editing = o.ID, partBody
	typeKeys(m, s)
	return m, o
}

// cursorAt is where the cursor is in the part being written (letters in).
func cursorAt(m *Model, o *dOct) int {
	cur, _, _, _ := m.ds().fieldMarks(o, m.ds().editing)
	return cur
}

// While writing, the keys move the cursor and edit at it as in Text mode.
func TestDesignFieldCursor(t *testing.T) {
	m, o := writePage(t, "hello world")
	press(m, "left") // a word back
	if c := cursorAt(m, o); c != 6 {
		t.Fatalf("Left: cursor at %d, want 6 (before world)", c)
	}
	typeKeys(m, "big ")
	if o.Body != "hello big world" {
		t.Fatalf("typed at the cursor: %q", o.Body)
	}
	press(m, "ctrl+left") // one letter
	if c := cursorAt(m, o); c != 9 {
		t.Fatalf("Ctrl+Left: cursor at %d, want 9", c)
	}
	press(m, "alt+left")
	if c := cursorAt(m, o); c != 0 {
		t.Fatalf("Alt+Left: cursor at %d, want 0", c)
	}
	press(m, "alt+right")
	if c := cursorAt(m, o); c != len("hello big world") {
		t.Fatalf("Alt+Right: cursor at %d, want the end", c)
	}
	press(m, "home")
	press(m, "delete")
	if o.Body != "ello big world" {
		t.Fatalf("Home, Delete: %q", o.Body)
	}
	press(m, "end")
	press(m, "ctrl+backspace")
	if o.Body != "ello big " {
		t.Fatalf("Ctrl+Backspace: %q", o.Body)
	}
	press(m, "backspace")
	if o.Body != "ello big" {
		t.Fatalf("Backspace: %q", o.Body)
	}
	// Enter: a new line at the cursor, in the text.
	press(m, "left")
	press(m, "enter")
	if o.Body != "ello \nbig" || m.ds().editing != partBody {
		t.Fatalf("Enter in the text: %q, editing %d", o.Body, m.ds().editing)
	}
	// Up / Down: a line; Alt + Shift + Down moves the line.
	press(m, "up")
	press(m, "alt+shift+down")
	if o.Body != "big\nello " {
		t.Fatalf("Alt+Shift+Down: %q", o.Body)
	}
	press(m, "ctrl+d") // duplicate the line
	if o.Body != "big\nello \nello " {
		t.Fatalf("Ctrl+D: %q", o.Body)
	}
	press(m, "ctrl+k") // delete it
	if o.Body != "big\nello " {
		t.Fatalf("Ctrl+K: %q", o.Body)
	}
	press(m, "alt+shift+right") // indent
	if !strings.HasPrefix(o.Body, "big\n    ello") {
		t.Fatalf("Alt+Shift+Right: %q", o.Body)
	}
}

// Shift highlights; typing, Backspace or Ctrl+X take the highlight;
// Ctrl+A highlights all; Ctrl+V pastes at the cursor.
func TestDesignFieldHighlight(t *testing.T) {
	m, o := writePage(t, "one two three")
	d := m.ds()
	press(m, "shift+left") // "three"
	if _, s, e, _ := d.fieldMarks(o, partBody); s != 8 || e != 13 {
		t.Fatalf("Shift+Left: highlight %d–%d, want 8–13", s, e)
	}
	press(m, "ctrl+x")
	if o.Body != "one two " || m.clip != "three" {
		t.Fatalf("Ctrl+X: %q, clipboard %q", o.Body, m.clip)
	}
	press(m, "home")
	m.Update(pasteMsg(m.clip)) // what Ctrl+V's clipboard read sends back
	if o.Body != "threeone two " {
		t.Fatalf("pasted at the cursor: %q", o.Body)
	}
	press(m, "ctrl+a")
	typeKeys(m, "new")
	if o.Body != "new" {
		t.Fatalf("Ctrl+A, typing: %q", o.Body)
	}
	press(m, "ctrl+shift+left") // one letter
	press(m, "backspace")
	if o.Body != "ne" {
		t.Fatalf("Ctrl+Shift+Left, Backspace: %q", o.Body)
	}
}

// Ctrl+Z / Ctrl+Y undo and redo the typing while writing; once writing
// stops they are the board's undo again.
func TestDesignFieldUndo(t *testing.T) {
	m, o := writePage(t, "alpha")
	press(m, "ctrl+z")
	if o.Body != "" || m.ds().editing != partBody {
		t.Fatalf("Ctrl+Z while writing: %q, editing %d", o.Body, m.ds().editing)
	}
	press(m, "ctrl+y")
	if o.Body != "alpha" {
		t.Fatalf("Ctrl+Y while writing: %q", o.Body)
	}
	press(m, "esc")
	press(m, "ctrl+z") // the board's undo: the writing, as one step
	if o := m.ds().byID(o.ID); o == nil || o.Body != "" {
		t.Fatalf("Ctrl+Z after writing: %+v", o)
	}
}

// A header stays one line (Enter goes to the next part), and a note
// never holds more than a page.
func TestDesignFieldLimits(t *testing.T) {
	m, o := writePage(t, "")
	d := m.ds()
	d.editing = partHeader
	typeKeys(m, "Title")
	press(m, "ctrl+d")
	if o.Header != "Title" {
		t.Fatalf("Ctrl+D in a header: %q", o.Header)
	}
	m.Update(pasteMsg("a\nb"))
	if o.Header != "Titlea b" {
		t.Fatalf("a paste in a header: %q", o.Header)
	}
	press(m, "enter")
	if d.editing != partBody {
		t.Fatalf("Enter in a header: editing %d", d.editing)
	}
	n := d.newKind(kNote, 0, 0)
	d.sel, d.editing = n.ID, partBody
	for range noteRows - 1 {
		typeKeys(m, "x")
		press(m, "enter")
	}
	typeKeys(m, "x")
	full := n.Body
	press(m, "home")
	press(m, "enter")
	if n.Body != full || !noteFits(n.Body) {
		t.Fatalf("Enter in a full note: %q", n.Body)
	}
	press(m, "ctrl+z") // the refused key left nothing to undo but the typing
	if n.Body == full {
		t.Fatal("Ctrl+Z after a refused key: nothing undone")
	}
}

// Wrapped rows keep where each letter came from: the cursor is on the
// right row and letter, and after a full row it goes under it.
func TestDesignFieldWrap(t *testing.T) {
	cases := []struct {
		text       string
		w, cur     int
		row, col   int
		rows       int
		rowsString string
	}{
		{"hello world", 6, 0, 0, 0, 2, "hello|world"},
		{"hello world", 6, 5, 0, 5, 2, "hello|world"},
		{"hello world", 6, 6, 1, 0, 2, "hello|world"},
		{"hello world", 6, 11, 1, 5, 2, "hello|world"},
		{"a\n\nb", 6, 1, 0, 1, 3, "a||b"},
		{"a\n\nb", 6, 2, 1, 0, 3, "a||b"},
		{"abcdef", 6, 6, 1, 0, 2, "abcdef|"},
		{"abcdef ghi", 6, 6, 1, 0, 2, "abcdef|ghi"},
	}
	for _, c := range cases {
		rows, r, col := fieldWrap(c.text, c.w, c.cur)
		var parts []string
		for _, x := range rows {
			parts = append(parts, string(x.rs))
		}
		if got := strings.Join(parts, "|"); got != c.rowsString || r != c.row || col != c.col {
			t.Errorf("%q w%d cur %d: rows %q at %d,%d; want %q at %d,%d", c.text, c.w, c.cur, got, r, col, c.rowsString, c.row, c.col)
		}
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

// In 3D the arrows move the cursor while writing, and the card's words
// show it, on the letter, as on the flat board.
func TestDesignField3D(t *testing.T) {
	m := wordsScene(t, 140, 40)
	withPixels(t, m, 10, 20)
	d := m.ds()
	r := &d.room
	r.yaw, r.pitch = 20, 10
	var page *dOct
	for _, o := range d.octs {
		if o.Kind == kPage || o.Kind == kNote {
			page = o
			break
		}
	}
	if page == nil {
		t.Fatal("no page in the scene")
	}
	d.sel, d.editing = page.ID, partBody
	page.Body = "Bread and milk"
	press(m, "end")
	press(m, "left") // before "milk"
	if c := cursorAt(m, page); c != 10 {
		t.Fatalf("Left in 3D while writing: cursor %d, want 10", c)
	}
	boardRows(m)
	accent := hex32(cAccent)
	found := ""
	for i := range r.tex {
		if r.tex[i].bg == accent && r.tex[i].glyph != "" {
			found += r.tex[i].glyph
		}
	}
	if found != "m" {
		t.Fatalf("3D: the cursor painted on %q, want m", found)
	}
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if d.editing != partNone || !r.on() {
		t.Fatalf("Esc in 3D while writing: editing %d, 3D %v", d.editing, r.on())
	}
	d.sel, d.editing = page.ID, partBody
	m.designAltTap()
	m.designAltTap()
	if d.editing != partNone || !r.on() {
		t.Fatalf("Alt twice in 3D while writing: writing stops, still 3D: editing %d, 3D %v", d.editing, r.on())
	}
}
