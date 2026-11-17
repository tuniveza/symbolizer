package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// drawTest is Draw mode on a big screen, with its own data folder.
func drawTest(t *testing.T) *Model {
	t.Helper()
	m := newTest(t, "")
	notesCache = nil
	m.Update(tea.WindowSizeMsg{Width: 180, Height: 50})
	m.setMode(drawMode)
	return m
}

// stroke drags the mouse from screen cell (x0, y0) to (x1, y1), a cell at a time.
func stroke(m *Model, x0, y0, x1, y1 int) {
	m.Update(tea.MouseClickMsg{X: x0, Y: y0, Button: tea.MouseLeft})
	n := max(abs(x1-x0), abs(y1-y0))
	for i := 1; i <= n; i++ {
		m.Update(tea.MouseMotionMsg{X: x0 + (x1-x0)*i/n, Y: y0 + (y1-y0)*i/n, Button: tea.MouseLeft})
	}
	m.Update(tea.MouseReleaseMsg{X: x1, Y: y1, Button: tea.MouseLeft})
}

// drawnCanvas is the board's cells as Draw mode draws them now.
func drawnCanvas(m *Model) *boardCanvas {
	cv := newBoardCanvas(m.width, m.boardH())
	if m.dw().f == nil {
		m.drawStart(cv)
	} else {
		m.drawCanvas(cv)
		m.drawTray(cv)
	}
	return cv
}

// canvasRow is row y of the canvas as text.
func canvasRow(cv *boardCanvas, y int) string {
	var sb strings.Builder
	for _, c := range cv.Lines[y] {
		sb.WriteString(c.Content)
	}
	return sb.String()
}

func isBraille(s string) bool {
	for _, r := range s {
		if r > 0x2800 && r <= 0x28ff {
			return true
		}
	}
	return false
}

// Draw mode is ready, and sy draw's word starts it.
func TestDrawModeReady(t *testing.T) {
	if !modes[drawMode].ready || modes[drawMode].name != "Draw" || modeWords["draw"] != drawMode {
		t.Fatalf("Draw mode should be ready as mode %d: %+v", drawMode, modes[drawMode])
	}
	m := drawTest(t)
	s := screenOf(m)
	for _, w := range []string{"New drawing", "DRAWINGS", "NOTES", "No drawings yet"} {
		if !strings.Contains(s, w) {
			t.Fatalf("the start screen should show %q:\n%s", w, s)
		}
	}
	if strings.Contains(s, "Still empty") {
		t.Fatalf("Draw mode is not empty any more:\n%s", s)
	}
	// F1 opens the keys at Draw mode's.
	press(m, "f1")
	if s := screenOf(m); !m.help || !strings.Contains(s, "Draw mode") || !strings.Contains(s, "glitter pen") {
		t.Fatalf("F1 should show Draw mode's keys:\n%s", s)
	}
}

// The start screen lists the drawings (newest first) and the notes by
// notepad, oldest first, with ✎ ink on a note that has ink.
func TestDrawStartScreenLists(t *testing.T) {
	m := drawTest(t)
	dir := t.TempDir()
	now := time.Now()
	addNote(t, m, dir, "apples.txt", "red and green", now.Add(-2*time.Hour))
	addNote(t, m, dir, "boats.txt", "sails up", now.Add(-time.Hour))
	// Two drawings, the second made later.
	for i := 0; i < 2; i++ {
		press(m, "n")
		stroke(m, 20+i*10, 10, 40+i*10, 14)
		m.dw().f.Name = "Sketch " + itoa(i+1)
		m.dw().saved = ""
		m.drawSave()
		press(m, "esc")
		time.Sleep(10 * time.Millisecond)
	}
	m.drawRefresh()
	s := screenOf(m)
	for _, w := range []string{"New drawing", "Sketch 1", "Sketch 2", "Notepad 1", "apples.txt", "boats.txt", "red and green"} {
		if !strings.Contains(s, w) {
			t.Fatalf("the start screen should show %q:\n%s", w, s)
		}
	}
	if strings.Index(s, "Sketch 2") > strings.Index(s, "Sketch 1") {
		t.Fatalf("the newest drawing comes first:\n%s", s)
	}
	if strings.Index(s, "apples.txt") > strings.Index(s, "boats.txt") {
		t.Fatalf("notes go oldest to newest, as in the notepads:\n%s", s)
	}
	// Each drawing has a preview: half blocks in its ink.
	cv := drawnCanvas(m)
	found := false
	for y := 0; y < cv.Height(); y++ {
		if r := canvasRow(cv, y); strings.ContainsAny(r[:min(len(r), 60)], "▀▄") {
			found = true
		}
	}
	if !found {
		t.Fatal("a drawing on the start screen should have a preview")
	}
	// Right goes to the notes; Enter opens the first.
	press(m, "right")
	press(m, "enter")
	if f := m.dw().f; f == nil || filepath.Base(f.Note) != "apples.txt" {
		t.Fatalf("Enter on the first note should open it to draw on: %+v", f)
	}
}

// A new drawing: strokes are saved at once, and come back when it is opened again.
func TestDrawNewDrawingSavedAndReloaded(t *testing.T) {
	m := drawTest(t)
	press(m, "n")
	dw := m.dw()
	if dw.f == nil || dw.f.Note != "" {
		t.Fatal("N should open a new drawing")
	}
	top := m.boardTop()
	stroke(m, 30, top+5, 70, top+5)
	stroke(m, 30, top+8, 50, top+15)
	if len(dw.f.Strokes) != 2 {
		t.Fatalf("two strokes drawn, got %d", len(dw.f.Strokes))
	}
	path := dw.path
	if !strings.HasPrefix(path, filepath.Join(filepath.Dir(sessionDir()), "drawings")) {
		t.Fatalf("drawings are kept in symbolizer/drawings: %s", path)
	}
	f, err := loadDrawFile(path)
	if err != nil || len(f.Strokes) != 2 {
		t.Fatalf("the drawing should be saved after each stroke: %v %+v", err, f)
	}
	if !isBraille(screenOf(m)) {
		t.Fatalf("a pen's line is braille dots:\n%s", screenOf(m))
	}
	before := screenOf(m)
	press(m, "esc")
	if m.dw().f != nil {
		t.Fatal("Esc should close the drawing")
	}
	if len(m.dw().drawings) != 1 {
		t.Fatalf("the drawing should be listed, got %d", len(m.dw().drawings))
	}
	press(m, "enter") // the drawing just closed is chosen
	if m.dw().f == nil || len(m.dw().f.Strokes) != 2 {
		t.Fatalf("Enter should open the drawing again with its strokes: %+v", m.dw().f)
	}
	if after := screenOf(m); canvasPart(after) != canvasPart(before) {
		t.Fatalf("the drawing should look the same opened again:\n%s\n---\n%s", before, after)
	}
}

// canvasPart is the board's rows of a screen (no messages).
func canvasPart(s string) string {
	lines := strings.Split(s, "\n")
	return strings.Join(lines[3:len(lines)-6], "\n")
}

// A blank drawing left without ink is not kept.
func TestDrawBlankNotKept(t *testing.T) {
	m := drawTest(t)
	press(m, "n")
	press(m, "esc")
	if n := len(drawingList()); n != 0 {
		t.Fatalf("an empty drawing should not be kept, got %d", n)
	}
}

// Each device leaves its own look: lines of braille dots, a marker's
// solid band, a highlighter's wash, a brush's bristles, glitter's sparkles.
func TestDrawEachDeviceLook(t *testing.T) {
	m := drawTest(t)
	top := m.boardTop()
	looks := map[int]func(row string) bool{
		devPen:       isBraille,
		devFineliner: isBraille,
		devPencil:    isBraille,
		devMarker:    func(r string) bool { return strings.ContainsAny(r, "▀▄") || strings.Contains(r, "     ") },
		devGlitter:   func(r string) bool { return strings.ContainsAny(r, "✦✧⋆") },
		devBrush:     func(r string) bool { return strings.ContainsAny(r, "▓▒") },
	}
	for key, dev := range trayDevs {
		press(m, "n")
		press(m, itoa(key+1))
		if m.dw().pen.Dev != dev {
			t.Fatalf("key %d should pick the %s", key+1, devNames[dev])
		}
		stroke(m, 20, top+10, 120, top+10)
		cv := drawnCanvas(m)
		row := canvasRow(cv, 10)
		bg := colorHex(cv.Lines[10][70].Style.Bg, "")
		if dev == devHighlighter {
			if bg == drawSheet() || strings.TrimSpace(row) != "" && isBraille(row) {
				t.Fatalf("a highlighter washes the paper: bg %s row %q", bg, row)
			}
		} else if !looks[dev](row) {
			t.Fatalf("the %s should leave its own look, row %q", devNames[dev], row)
		}
		press(m, "esc")
	}
	// The tray shows all seven, by name.
	press(m, "n")
	s := screenOf(m)
	for _, name := range devNames {
		if !strings.Contains(s, name) {
			t.Fatalf("the tray should show the %s:\n%s", name, s)
		}
	}
}

// Glitter sparkles, and the sparkles move on with each twinkle (a tick is
// only asked for while glitter shows).
func TestDrawGlitterSparkles(t *testing.T) {
	m := drawTest(t)
	press(m, "n")
	if m.drawGlitterCmd() != nil {
		t.Fatal("no glitter on the canvas: no twinkle ticks")
	}
	press(m, "6")
	top := m.boardTop()
	stroke(m, 20, top+6, 150, top+6)
	stroke(m, 20, top+8, 150, top+8)
	one := canvasPart(screenOf(m))
	if !strings.ContainsAny(one, "✦✧⋆") {
		t.Fatalf("glitter should sparkle:\n%s", one)
	}
	if !m.dw().glitterOn {
		t.Fatal("glitter shows: it should twinkle")
	}
	m.dw().tick = false
	if m.drawGlitterCmd() == nil {
		t.Fatal("glitter showing should ask for a twinkle")
	}
	m.Update(drawTickMsg{})
	if two := canvasPart(screenOf(m)); two == one {
		t.Fatal("the sparkles should move on after a twinkle")
	}
}

// Undo and redo take a stroke off and put it back, and keep the file up to date.
func TestDrawUndoRedo(t *testing.T) {
	m := drawTest(t)
	press(m, "n")
	top := m.boardTop()
	stroke(m, 20, top+4, 60, top+4)
	plain := canvasPart(screenOf(m))
	stroke(m, 20, top+12, 60, top+16)
	two := canvasPart(screenOf(m))
	press(m, "ctrl+z")
	if n := len(m.dw().f.Strokes); n != 1 {
		t.Fatalf("undo should leave one stroke, got %d", n)
	}
	if canvasPart(screenOf(m)) != plain {
		t.Fatal("after undo the canvas should look as it did before the second stroke")
	}
	if f, _ := loadDrawFile(m.dw().path); f == nil || len(f.Strokes) != 1 {
		t.Fatal("undo should be saved")
	}
	press(m, "ctrl+y")
	if canvasPart(screenOf(m)) != two {
		t.Fatal("redo should bring the stroke back")
	}
	// Undoing everything: the drawing is not kept.
	press(m, "ctrl+z")
	press(m, "ctrl+z")
	if fileExists(m.dw().path) {
		t.Fatal("a drawing with all its ink undone is not kept")
	}
}

// The rubber takes ink away.
func TestDrawRubber(t *testing.T) {
	m := drawTest(t)
	press(m, "n")
	top := m.boardTop()
	stroke(m, 20, top+5, 60, top+5)
	press(m, "e")
	if !m.dw().pen.Rubber {
		t.Fatal("E should take the rubber")
	}
	stroke(m, 10, top+5, 70, top+5)
	if isBraille(canvasRow(drawnCanvas(m), 5)) {
		t.Fatal("the rubber should take the line away")
	}
}

// The keyboard draws too: arrows move the brush, Space puts the pen down.
func TestDrawKeyboard(t *testing.T) {
	m := drawTest(t)
	press(m, "n")
	press(m, "space")
	for i := 0; i < 10; i++ {
		press(m, "right")
	}
	press(m, "space")
	f := m.dw().f
	if len(f.Strokes) != 1 || len(f.Strokes[0].Pts) != 11 {
		t.Fatalf("Space, ten arrows, Space: one stroke of 11 points, got %+v", f.Strokes)
	}
	if !isBraille(canvasPart(screenOf(m))) {
		t.Fatal("the keyboard's line should show")
	}
}

// The pen in hand, its size and colour are remembered between sessions.
func TestDrawPenRemembered(t *testing.T) {
	m := drawTest(t)
	press(m, "n")
	press(m, "6")
	press(m, "c")
	press(m, "]")
	want := m.dw().pen
	m2, err := NewModel("", 0)
	if err != nil {
		t.Fatal(err)
	}
	got := m2.dw().pen
	if got.Dev != devGlitter || got.Size != want.Size || got.ink(devGlitter) != want.ink(devGlitter) {
		t.Fatalf("the pen should be remembered: got %+v, want %+v", got, want)
	}
}

// A note's ink is kept apart from it: its .txt file stays byte for byte
// the same, and notes.json is untouched.
func TestDrawNoteInkKeptApart(t *testing.T) {
	m := drawTest(t)
	dir := t.TempDir()
	text := "first line\n\tindented\nlast line, no newline"
	p := addNote(t, m, dir, "plan.txt", text, time.Now())
	notesBefore, _ := os.ReadFile(notesPath())
	m.drawRefresh()
	press(m, "right")
	press(m, "enter")
	dw := m.dw()
	if dw.f == nil || dw.f.Note != p {
		t.Fatalf("the note should be open: %+v", dw.f)
	}
	s := padScreen(m)
	for _, w := range []string{"first line", "indented", "last line, no newline", "plan.txt"} {
		if !strings.Contains(s, w) {
			t.Fatalf("the note's text should show under the ink: %q\n%s", w, s)
		}
	}
	top := m.boardTop()
	press(m, "4") // a marker over the words
	stroke(m, 20, top+1, 80, top+1)
	press(m, "1")
	stroke(m, 20, top+6, 80, top+9)
	press(m, "esc")
	got, _ := os.ReadFile(p)
	if !bytes.Equal(got, []byte(text)) {
		t.Fatalf("the note's file must never change: %q", got)
	}
	if after, _ := os.ReadFile(notesPath()); !bytes.Equal(after, notesBefore) {
		t.Fatal("notes.json should not change")
	}
	f, err := loadDrawFile(inkPath(p))
	if err != nil || len(f.Strokes) != 2 || f.Note != p {
		t.Fatalf("the ink should be kept in symbolizer/ink: %v %+v", err, f)
	}
	if !strings.HasPrefix(inkPath(p), filepath.Join(filepath.Dir(sessionDir()), "ink")) {
		t.Fatalf("ink goes in symbolizer/ink: %s", inkPath(p))
	}
	if len(drawingList()) != 0 {
		t.Fatal("a note's ink is not a drawing")
	}
	if !noteHasInk(p) {
		t.Fatal("the note should have ink now")
	}
	// The start screen marks it.
	if s := screenOf(m); !strings.Contains(s, "✎ ink") {
		t.Fatalf("a note with ink shows ✎ ink:\n%s", s)
	}
	// Opened again, the ink is there.
	press(m, "enter")
	if len(m.dw().f.Strokes) != 2 {
		t.Fatal("the note's ink should come back")
	}
}

// In Design's notepad viewer D draws on the chosen note in Draw mode;
// Esc comes back to the viewer, which shows ✎ ink and the ink.
func TestDrawFromNotepadViewer(t *testing.T) {
	m, dir := notepadTest(t)
	p := addNote(t, m, dir, "garden.txt", "dig the beds\nplant the beans", time.Now())
	v := openPad(t, m, 1)
	if strings.Contains(padScreen(m), "✎ ink") {
		t.Fatal("no ink yet")
	}
	if !strings.Contains(padScreen(m), "D: draw on it") {
		t.Fatalf("the viewer's hint should name D:\n%s", padScreen(m))
	}
	press(m, "d")
	if !m.inDraw() || m.dw().f == nil || m.dw().f.Note != p || !m.dw().fromDesign {
		t.Fatalf("D should open the note in Draw mode: mode %d %+v", m.mode, m.dw().f)
	}
	top := m.boardTop()
	press(m, "4")
	stroke(m, 30, top+1, 60, top+1)
	press(m, "esc")
	if !m.inDesign() || m.padView != v {
		t.Fatalf("Esc should go back to the notepad viewer: mode %d", m.mode)
	}
	s := padScreen(m)
	if !strings.Contains(s, "✎ ink") {
		t.Fatalf("the note's row should show ✎ ink:\n%s", s)
	}
	if b, _ := os.ReadFile(p); string(b) != "dig the beds\nplant the beans" {
		t.Fatal("the note's file must not change")
	}
	// The preview shows the ink: the marker's colour lies over the first line.
	ink := m.dw().pen.ink(devMarker)
	cv := m.designCanvas()
	found := false
	for y := 0; y < cv.Height() && !found; y++ {
		for x := 0; x < cv.Width(); x++ {
			if bg := colorHex(cv.Lines[y][x].Style.Bg, ""); bg != "" && bg == mixHex(notepadStyle(1).Paper, ink, 0.75) {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatal("the viewer's preview should show the note's ink")
	}
	// The notepad on the board is marked too.
	press(m, "esc")
	if !strings.Contains(screenOf(m), "✎") {
		t.Fatalf("the notepad on the board should show ✎:\n%s", screenOf(m))
	}
}
