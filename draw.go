package main

// ---- Draw mode: the terminal as the drawing tool (the user asked, 17 Nov 2026) --------------
//
// "apparatuses like pens, glitter pens and so on which can be used on those
// notepads and notes in sy draw mode and in sy design directly."
//
// Draw mode draws on a blank drawing, or on a note. On a note the ink is a
// layer over its text: the .txt file itself is never changed, and the ink
// also shows on that note in its notepad on the Design board.
//
// The devices are Design's (design_desk.go): pen, fineliner, pencil,
// marker, highlighter, glitter pen and brush, with the same colours and
// the same stroke look (inkColours, highlightColours, inkGlyphFor). They
// lie in a tray along the bottom of the canvas.
//
// Strokes are kept in dots, two across and four down a cell, as braille
// letters draw them: a pen's line is finer than a cell. A stroke is the
// points it went through; the dots are worked out from them (drawInk), so
// undo is taking the last stroke off.
//
// Kept in the data folder (next to sessions/, as codex/ is):
//
//	symbolizer/drawings/<id>.json   a drawing
//	symbolizer/ink/<hash>.json      a note's ink (the hash of the note's path)
//	symbolizer/draw.json            the pen in hand, its size, each device's colour
//
// The keys are in draw_keys.go, the drawing of the screen in draw_paint.go.

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

const drawMode = 5 // its place in modes

func (m *Model) inDraw() bool { return m.mode == drawMode }

// drawStroke is one stroke: a device, its colour and size, and the dots it
// went through (a rubber's stroke takes ink away).
type drawStroke struct {
	Dev  int8     `json:"dev"`
	Ink  string   `json:"ink,omitempty"`
	Size int      `json:"size"`
	Rub  bool     `json:"rub,omitempty"`
	Pts  [][2]int `json:"pts"`
}

// drawFile is a drawing, or a note's ink (Note is the note's path).
type drawFile struct {
	Name    string       `json:"name,omitempty"`
	Note    string       `json:"note,omitempty"`
	Made    time.Time    `json:"made"`
	Changed time.Time    `json:"changed"`
	W       int          `json:"w"` // the canvas it was drawn on, in cells (for its preview)
	H       int          `json:"h"`
	Strokes []drawStroke `json:"strokes"`

	ink    drawInk // the dots, worked out from the strokes (not saved)
	inkN   int     // the stroke ink has got to
	inkPts int     // how many of that stroke's points are in ink
}

// drawCellInk is the ink in one cell: the stroke (from 1; 0 none) that
// put each of its 8 dots, dot dx + 2·dy.
type drawCellInk [8]int32

// drawInk is the ink, cell by cell (cells with none are left out).
type drawInk map[[2]int]*drawCellInk

// drawState is Draw mode: the start screen, or the drawing (or note) open.
type drawState struct {
	// the start screen
	drawings []drawEntry
	notes    []drawNoteRow
	col      int    // 0 the drawings, 1 the notes
	sel      [2]int // the chosen row in each column
	top      [2]int // the first row shown in each column
	xAt      time.Time
	lastPick time.Time

	// what is open
	f          *drawFile
	path       string // where it is kept
	saved      string // as last saved, as JSON
	text       []string
	style      padStyle // a note's notepad look
	redo       []drawStroke
	drawing    bool // a stroke is being drawn (mouse held, or the pen down)
	penDown    bool // the keyboard's pen is down
	bx, by     int  // the keyboard's brush, in dots
	brushShown bool // the brush shows (after the keyboard moved it)
	scroll     int  // a note's first row shown
	fromDesign bool // opened from a notepad on the Design board: Esc goes back there
	err        string

	// the pen in hand (kept in draw.json)
	pen drawPen

	twinkle   int  // glitter's sparkle frame
	glitterOn bool // glitter ink showed in the last frame: it twinkles
	tick      bool // a twinkle is on its way
}

// drawPen is the pen in hand, remembered between sessions.
type drawPen struct {
	Dev    int               `json:"dev"`
	Size   int               `json:"size"`
	Rubber bool              `json:"rubber,omitempty"`
	Inks   map[string]string `json:"inks"` // each device's colour, by its name
}

// drawEntry is a drawing on the start screen.
type drawEntry struct {
	id string
	f  *drawFile
}

// drawNoteRow is a line of the notes column: a notepad's heading, or a note.
type drawNoteRow struct {
	pad  int
	note *noteRec // nil: the heading
}

// trayDevs are the devices in the tray, in the order keys 1–7 pick them.
var trayDevs = []int{devPen, devFineliner, devPencil, devMarker, devHighlighter, devGlitter, devBrush}

// devStartInk is each device's first colour (the desk's pens start the same).
var devStartInk = [devCount]string{
	devHighlighter: "#ffd400", devPen: "#e8743b", devFineliner: "#5b8fd6", devPencil: "#9a9aa0",
	devMarker: "#d64541", devGlitter: "#ff4fa3", devBrush: "#a259ff",
}

const drawMaxSize = 6

func (m *Model) dw() *drawState {
	if m.draw == nil {
		m.draw = &drawState{pen: loadDrawPen()}
	}
	return m.draw
}

// ---- where things are kept ---------------------------------------------------------------

func drawDataDir() string          { return filepath.Dir(sessionDir()) }
func drawingsDir() string          { return filepath.Join(drawDataDir(), "drawings") }
func inkDir() string               { return filepath.Join(drawDataDir(), "ink") }
func drawPenPath() string          { return filepath.Join(drawDataDir(), "draw.json") }
func drawingPath(id string) string { return filepath.Join(drawingsDir(), id+".json") }

// inkPath is where note path's ink is kept.
func inkPath(note string) string {
	sum := sha1.Sum([]byte(filepath.Clean(note)))
	return filepath.Join(inkDir(), fmt.Sprintf("%x.json", sum[:8]))
}

func loadDrawFile(path string) (*drawFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f := &drawFile{}
	if err := json.Unmarshal(b, f); err != nil {
		return nil, err
	}
	return f, nil
}

// writeJSON writes b to path whole (a half-written file is never left).
func writeJSON(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// drawingList is every drawing, the newest change first.
func drawingList() []drawEntry {
	ents, _ := os.ReadDir(drawingsDir())
	var out []drawEntry
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		f, err := loadDrawFile(filepath.Join(drawingsDir(), name))
		if err != nil {
			continue
		}
		out = append(out, drawEntry{id: strings.TrimSuffix(name, ".json"), f: f})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].f.Changed.After(out[j].f.Changed) })
	return out
}

// newDrawingID is a name for a new drawing: when it was made.
func newDrawingID(now time.Time) string {
	base := now.Format("20060102-150405")
	id := base
	for i := 2; fileExists(drawingPath(id)); i++ {
		id = base + "-" + itoa(i)
	}
	return id
}

// ---- a note's ink, for Design's notepad viewer -----------------------------------------------

type inkCached struct {
	mod time.Time
	f   *drawFile
}

var inkCache = map[string]inkCached{}

// noteInk is note path's ink (nil: none). It is read again when its file changes.
func noteInk(note string) *drawFile {
	p := inkPath(note)
	st, err := os.Stat(p)
	if err != nil {
		delete(inkCache, p)
		return nil
	}
	if c, ok := inkCache[p]; ok && c.mod.Equal(st.ModTime()) {
		return c.f
	}
	f, err := loadDrawFile(p)
	if err != nil || len(f.Strokes) == 0 {
		f = nil
	}
	inkCache[p] = inkCached{st.ModTime(), f}
	return f
}

// noteHasInk: has note path been drawn on?
func noteHasInk(note string) bool { return noteInk(note) != nil }

// ---- the pen in hand -------------------------------------------------------------------------

func loadDrawPen() drawPen {
	p := drawPen{Dev: devPen, Size: 2}
	if b, err := os.ReadFile(drawPenPath()); err == nil {
		_ = json.Unmarshal(b, &p)
	}
	if p.Dev < 0 || p.Dev >= devCount {
		p.Dev = devPen
	}
	p.Size = min(max(p.Size, 1), drawMaxSize)
	if p.Inks == nil {
		p.Inks = map[string]string{}
	}
	return p
}

func (p drawPen) save() {
	b, _ := json.MarshalIndent(p, "", " ")
	_ = writeJSON(drawPenPath(), b)
}

// ink is device dev's colour.
func (p drawPen) ink(dev int) string {
	if c := p.Inks[devNames[dev]]; c != "" {
		return c
	}
	return devStartInk[dev]
}

// ---- the mode starts and ends ----------------------------------------------------------------

// enterDraw: the mode starts (or comes back): the start screen, or what is open.
func (m *Model) enterDraw() tea.Cmd {
	dw := m.dw()
	if dw.f == nil {
		m.drawRefresh()
		m.say(modes[drawMode].Icon() + "  Draw mode. N: a new drawing · arrows and Enter: open a drawing or a note · F1: all keys.")
	}
	return nil
}

// drawRefresh reads the drawings and the notes again, for the start screen.
func (m *Model) drawRefresh() {
	dw := m.dw()
	dw.drawings = drawingList()
	notesFresh()
	nf := notesLoad()
	var pads []int
	for _, p := range nf.Pads {
		pads = append(pads, p.N)
	}
	sort.Ints(pads)
	dw.notes = nil
	for _, n := range pads {
		notes := nf.inPad(n)
		if len(notes) == 0 {
			continue
		}
		dw.notes = append(dw.notes, drawNoteRow{pad: n})
		for _, r := range notes {
			dw.notes = append(dw.notes, drawNoteRow{pad: n, note: r})
		}
	}
	dw.sel[0] = min(dw.sel[0], len(dw.drawings)) // row 0 is "New drawing"
	dw.sel[1] = min(dw.sel[1], max(len(dw.notes)-1, 0))
	if dw.col == 1 && len(dw.notes) == 0 {
		dw.col = 0
	}
	dw.fixNoteSel(1)
}

// fixNoteSel moves the notes column's choice off a heading (dir: which way).
func (dw *drawState) fixNoteSel(dir int) {
	n := len(dw.notes)
	if n == 0 {
		return
	}
	i := min(max(dw.sel[1], 0), n-1)
	for i >= 0 && i < n && dw.notes[i].note == nil {
		i += dir
	}
	if i < 0 || i >= n {
		i = min(max(dw.sel[1], 0), n-1)
		for i >= 0 && i < n && dw.notes[i].note == nil {
			i -= dir
		}
	}
	dw.sel[1] = min(max(i, 0), n-1)
}

// drawLeave keeps everything when the drawing is left (closed, another
// mode, sy quits): a stroke being drawn ends, and it is saved.
func (m *Model) drawLeave() {
	dw := m.draw
	if dw == nil || dw.f == nil {
		return
	}
	dw.drawing, dw.penDown = false, false
	m.drawSave()
}

// drawOpenNew opens a new, blank drawing (it is kept once it has ink).
func (m *Model) drawOpenNew() {
	now := time.Now()
	id := newDrawingID(now)
	w, h := m.drawCanvasSize()
	f := &drawFile{Name: "Drawing " + now.Format("Mon 2 Jan 15:04"), Made: now, Changed: now, W: w, H: h}
	m.drawOpenFile(f, drawingPath(id))
	m.say("A new drawing. Drag with the mouse to draw · 1–7 pick a pen · C colour · [ ] size · Esc: done.")
}

// drawOpenDrawing opens the drawing kept at entry e.
func (m *Model) drawOpenDrawing(e drawEntry) {
	f, err := loadDrawFile(drawingPath(e.id))
	if err != nil {
		m.sayErr("Could not open that drawing: " + err.Error())
		return
	}
	m.drawOpenFile(f, drawingPath(e.id))
	m.say("“" + f.Name + "”. Drag to draw · Ctrl+Z undo · Esc: back to the drawings.")
}

// drawOpenNote opens note r to draw on: its ink as it was left. The note's
// .txt file is only read.
func (m *Model) drawOpenNote(r *noteRec) bool {
	if !fileExists(r.Path) {
		m.sayErr(filepath.Base(r.Path) + " is missing: the file is not at " + shortPath(r.Path) + " any more.")
		return false
	}
	f, err := loadDrawFile(inkPath(r.Path))
	if err != nil {
		now := time.Now()
		f = &drawFile{Note: filepath.Clean(r.Path), Made: now, Changed: now}
	}
	f.Note = filepath.Clean(r.Path)
	m.drawOpenFile(f, inkPath(r.Path))
	dw := m.dw()
	dw.text = noteLines(r.Path)
	dw.style = notepadStyle(r.Pad)
	m.say("Drawing on “" + filepath.Base(r.Path) + "”: the ink is a layer over it, the file is never changed. Esc: done.")
	return true
}

func (m *Model) drawOpenFile(f *drawFile, path string) {
	dw := m.dw()
	dw.f, dw.path, dw.redo, dw.err = f, path, nil, ""
	b, _ := json.Marshal(f)
	dw.saved = string(b)
	if f.ink == nil && len(f.Strokes) == 0 {
		dw.saved = "" // never saved: it is kept as soon as it has ink
	}
	dw.text, dw.scroll = nil, 0
	dw.drawing, dw.penDown, dw.brushShown = false, false, false
	w, h := m.drawCanvasSize()
	dw.bx, dw.by = w, h*2 // the middle, in dots
	dw.glitterOn = false
	for _, s := range f.Strokes {
		if int(s.Dev) == devGlitter && !s.Rub {
			dw.glitterOn = true
		}
	}
}

// drawClose closes what is open: back to the notepad on the Design board
// if it came from there, else to the start screen.
func (m *Model) drawClose() tea.Cmd {
	dw := m.dw()
	m.drawLeave()
	f := dw.f
	dw.f, dw.text = nil, nil
	if dw.fromDesign {
		dw.fromDesign = false
		cmd := m.setMode(designMode)
		m.ds().dirty = true
		if m.padView != nil {
			m.say("Back in notepad " + itoa(m.padView.n) + ".")
		} else {
			m.say("Back on the board.")
		}
		return cmd
	}
	m.drawRefresh()
	if f != nil && f.Note == "" {
		for i, e := range dw.drawings {
			if drawingPath(e.id) == dw.path {
				dw.col, dw.sel[0] = 0, i+1
			}
		}
	}
	m.say("Closed. N: a new drawing · Enter opens the chosen one.")
	return nil
}

// drawSave keeps what is open, if it changed. With no ink left it is not
// kept at all (a blank drawing, a note with its ink all undone).
func (m *Model) drawSave() {
	dw := m.dw()
	f := dw.f
	if f == nil || dw.path == "" {
		return
	}
	if len(f.Strokes) == 0 {
		if fileExists(dw.path) {
			if err := os.Remove(dw.path); err != nil {
				dw.err = err.Error()
				return
			}
		}
		dw.saved, dw.err = "", ""
		return
	}
	b, _ := json.Marshal(f)
	if string(b) == dw.saved {
		return
	}
	f.Changed = time.Now()
	b, _ = json.Marshal(f)
	if err := writeJSON(dw.path, b); err != nil {
		dw.err = err.Error()
		m.sayErr("Not saved: " + dw.err)
		return
	}
	dw.saved, dw.err = string(b), ""
}

// drawDelete deletes drawing e (X twice on the start screen).
func (m *Model) drawDelete(e drawEntry) {
	if err := os.Remove(drawingPath(e.id)); err != nil {
		m.sayErr("Could not delete it: " + err.Error())
		return
	}
	m.drawRefresh()
	m.say("Deleted “" + e.f.Name + "”.")
}

// noteLines is a note's text as it is shown: tabs as four spaces, control
// letters left out (as the notepad viewer's preview shows it, so the ink
// lies on the same letters in both).
func noteLines(path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	text := strings.ReplaceAll(string(b), "\t", "    ")
	var out []string
	for _, l := range strings.Split(text, "\n") {
		out = append(out, strings.Map(func(r rune) rune {
			if r < 0x20 || r == 0x7f {
				return -1
			}
			return r
		}, l))
	}
	return out
}

// ---- strokes -----------------------------------------------------------------------------------

// drawBegin starts a stroke at dot (x, y) with the pen in hand.
func (m *Model) drawBegin(x, y int) {
	dw := m.dw()
	if dw.f == nil {
		return
	}
	dev := dw.pen.Dev
	s := drawStroke{Dev: int8(dev), Ink: dw.pen.ink(dev), Size: dw.pen.Size, Rub: dw.pen.Rubber, Pts: [][2]int{{x, y}}}
	if s.Rub {
		s.Ink = ""
	}
	dw.f.Strokes = append(dw.f.Strokes, s)
	dw.redo = nil
	dw.drawing = true
	if dev == devGlitter && !s.Rub {
		dw.glitterOn = true
	}
}

// drawTo carries the stroke being drawn on to dot (x, y).
func (m *Model) drawTo(x, y int) {
	dw := m.dw()
	if dw.f == nil || !dw.drawing || len(dw.f.Strokes) == 0 {
		return
	}
	s := &dw.f.Strokes[len(dw.f.Strokes)-1]
	if last := s.Pts[len(s.Pts)-1]; last == [2]int{x, y} {
		return
	}
	s.Pts = append(s.Pts, [2]int{x, y})
}

// drawEnd ends the stroke being drawn, and keeps the drawing.
func (m *Model) drawEnd() {
	dw := m.dw()
	if !dw.drawing {
		return
	}
	dw.drawing = false
	if dw.f != nil {
		dw.f.grow()
	}
	m.drawSave()
}

// grow makes the drawing's size take in all its ink (for its preview).
func (f *drawFile) grow() {
	if f.Note != "" {
		return
	}
	for at := range f.inked() {
		f.W, f.H = max(f.W, at[0]+1), max(f.H, at[1]+1)
	}
}

func (m *Model) drawUndo() {
	dw := m.dw()
	if dw.f == nil || len(dw.f.Strokes) == 0 {
		m.say("Nothing to undo.")
		return
	}
	dw.drawing, dw.penDown = false, false
	n := len(dw.f.Strokes)
	dw.redo = append(dw.redo, dw.f.Strokes[n-1])
	dw.f.Strokes = dw.f.Strokes[:n-1]
	dw.f.ink = nil // worked out again
	m.drawSave()
	m.say("Undone: one stroke. Ctrl+Y brings it back.")
}

func (m *Model) drawRedo() {
	dw := m.dw()
	if dw.f == nil || len(dw.redo) == 0 {
		m.say("Nothing to redo.")
		return
	}
	n := len(dw.redo)
	dw.f.Strokes = append(dw.f.Strokes, dw.redo[n-1])
	dw.redo = dw.redo[:n-1]
	m.drawSave()
	m.say("Redone.")
}

// ---- strokes into dots -------------------------------------------------------------------------

// inked is the drawing's ink, cell by cell, worked out from its strokes
// (only the new part of a stroke being drawn is worked out each time).
func (f *drawFile) inked() drawInk {
	if f.ink == nil || f.inkN > len(f.Strokes) {
		f.ink, f.inkN, f.inkPts = drawInk{}, 0, 0
	}
	for f.inkN < len(f.Strokes) {
		s := &f.Strokes[f.inkN]
		for k := f.inkPts; k < len(s.Pts); k++ {
			f.ink.segment(s, int32(f.inkN+1), k)
		}
		f.inkPts = len(s.Pts)
		if f.inkN == len(f.Strokes)-1 {
			break
		}
		f.inkN++
		f.inkPts = 0
	}
	return f.ink
}

// dabRadius is how many dots round each point stroke s colours, at its
// point k (a brush swells and thins along its stroke).
func dabRadius(s *drawStroke, k int) int {
	size := min(max(s.Size, 1), drawMaxSize)
	if s.Rub {
		return size + 1
	}
	switch int(s.Dev) {
	case devFineliner:
		return (size - 1) / 2
	case devMarker:
		return size + 1
	case devHighlighter:
		return size + 2
	case devGlitter:
		return size
	case devBrush:
		return size - 1 + brushWidths[(k/2)%len(brushWidths)]
	}
	return size - 1 // pen, pencil
}

// segment puts stroke s's ink (stroke number id) from its point k-1 to k.
func (in drawInk) segment(s *drawStroke, id int32, k int) {
	to := s.Pts[k]
	from := to
	if k > 0 {
		from = s.Pts[k-1]
	}
	r := dabRadius(s, k)
	x0, y0, x1, y1 := from[0], from[1], to[0], to[1]
	dx, dy := abs(x1-x0), -abs(y1-y0)
	sx, sy := 1, 1
	if x0 > x1 {
		sx = -1
	}
	if y0 > y1 {
		sy = -1
	}
	e := dx + dy
	for {
		in.dab(x0, y0, r, id, s.Rub)
		if x0 == x1 && y0 == y1 {
			return
		}
		if e2 := 2 * e; e2 >= dy {
			e += dy
			x0 += sx
		} else {
			e += dx
			y0 += sy
		}
	}
}

// dab colours the dots within r of dot (x, y) with stroke id (rub: clears them).
func (in drawInk) dab(x, y, r int, id int32, rub bool) {
	for yy := y - r; yy <= y+r; yy++ {
		for xx := x - r; xx <= x+r; xx++ {
			if (xx-x)*(xx-x)+(yy-y)*(yy-y) > r*r+r {
				continue
			}
			in.dot(xx, yy, id, rub)
		}
	}
}

// dot sets one dot.
func (in drawInk) dot(x, y int, id int32, rub bool) {
	cx, cy := floorDiv(x, 2), floorDiv(y, 4)
	i := (x - cx*2) + 2*(y-cy*4)
	at := [2]int{cx, cy}
	c := in[at]
	if rub {
		if c == nil {
			return
		}
		c[i] = 0
		if *c == (drawCellInk{}) {
			delete(in, at)
		}
		return
	}
	if c == nil {
		c = &drawCellInk{}
		in[at] = c
	}
	c[i] = id
}
