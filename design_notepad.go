package main

// ---- a notepad on the Design board (notes.go) ------------------------------------------------
//
// A notepad (kNotepad) shows notepad N's notes (From "notepad:N"): up to 16
// saved .txt files. On the board it is seen from the front: its binding at
// the top, its cover with its name and how full it is, and the top page
// with the newest note's title and the stickers in it. Each notepad has its
// own look (notepadStyle).
//
//	Enter · double-click   open it: the viewer (padViewer, below)
//
// The viewer lists the notepad's notes by day, newest first, with a
// preview of the chosen one. Its keys:
//
//	↑ ↓ · Home · End   choose a note
//	Enter              open it in Text mode
//	F                  give it a sticker (or change it)
//	/ · S              show only one sticker's notes (each press: the next, then all)
//	X X                take it out of the notepad (its .txt file stays)
//	PgUp · PgDn        the notepad before · after
//	Esc                close

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

const kNotepad = "notepad"

const notepadW, notepadH = 26, 16

func init() {
	kindIcons[kNotepad] = "▤"
	kindNames[kNotepad] = "notepad"
}

// ---- each notepad's own look -----------------------------------------------------------------
//
// notepadStyle(n) is notepad n's look. Each part changes from one notepad
// to the next, and they repeat at different rates (4 bindings, 5 rulings,
// 7 patterns, 9 covers), so no two of the first 4×5×7×9 = 1260 notepads
// look the same. Shared by the notepad itself, its viewer and the desk's
// racks (design_desk.go).

// Bindings: how the pages are held at the top.
const (
	bindSpiral   = iota // a row of wire rings
	bindGlued           // a glued band
	bindStitched        // stitches across
	bindClip            // a clipboard clip
	bindCount
)

// Rulings: what the pages are printed with.
const (
	ruleLined = iota
	ruleGrid
	ruleDots
	ruleBlank
	ruleMargin // lined with a red margin
	ruleCount
)

type padStyle struct {
	Cover   string // the cover's colour
	Paper   string // the pages' colour
	Ink     string // writing on the pages
	Rule    string // the ruling's colour
	Binding int    // bindSpiral …
	Ruling  int    // ruleLined …
	Corner  int    // 0 square, 1 rounded, 2 dog-eared, 3 cut
	Pattern int    // the cover's pattern: 0 plain, 1 stripes, 2 dots, 3 checks, 4 waves, 5 grain, 6 diamonds
}

// notepadCovers: warm theme-friendly cover colours, each different enough to tell apart.
var notepadCovers = []string{"#c0392b", "#d35400", "#b9770e", "#7d6608", "#6e2c00", "#a04000", "#8e3b46", "#4e342e", "#9a5b13"}

// notepadPapers: page colours from cream to dark.
var notepadPapers = []string{"#f3e9d2", "#efe4c4", "#e8dcc8", "#f5efe0", "#2b1d16"}

func notepadStyle(n int) padStyle {
	if n < 1 {
		n = 1
	}
	i := n - 1
	s := padStyle{
		Binding: i % bindCount,
		Cover:   notepadCovers[(i*4)%len(notepadCovers)], // steps of 4: side by side, the colours differ more
		Ruling:  i % ruleCount,
		Pattern: (i*3 + 1) % 7, // the first has stripes, not a plain cover
		Corner:  (i + i/bindCount) % 4,
	}
	s.Paper = notepadPapers[(i*2+i/5)%len(notepadPapers)]
	if lum(s.Paper) > 0.5 {
		s.Ink, s.Rule = "#2b1608", mixHex(s.Paper, "#5a7fa8", 0.35)
	} else {
		s.Ink, s.Rule = "#f3e9d2", mixHex(s.Paper, cEmber, 0.35)
	}
	return s
}

// ---- reading the notes for drawing ---------------------------------------------------------

// notepadOf is the notepad number structure o shows (0: none).
func notepadOf(o *dOct) int {
	if o == nil || o.Kind != kNotepad {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimPrefix(o.From, "notepad:"))
	return n
}

var (
	notesSeenMod  time.Time // notes.json's time when last looked at
	notesSeenPath string
)

// notesFresh forgets the notes read before when notes.json changed on disk
// (another sy saved a note): the next notesLoad reads it again. Notes are
// saved as soon as they change, so nothing is lost by reading them again.
func notesFresh() {
	var mod time.Time
	if fi, err := os.Stat(notesPath()); err == nil {
		mod = fi.ModTime()
	}
	notesMu.Lock()
	defer notesMu.Unlock()
	if notesSeenPath == notesPath() && !mod.Equal(notesSeenMod) {
		notesCache = nil
	}
	notesSeenMod, notesSeenPath = mod, notesPath()
}

// notepadStamp is what notepad n's front shows, as one string: its drawing
// is made again when this changes.
func notepadStamp(n int) string {
	notesFresh()
	var sb strings.Builder
	for _, r := range notesLoad().inPad(n) {
		sb.WriteString(r.Path + "\x00" + r.Sticker + "\n")
	}
	return sb.String()
}

// noteTitle is a note's name: its file name without .txt.
func noteTitle(r *noteRec) string {
	return strings.TrimSuffix(filepath.Base(r.Path), filepath.Ext(r.Path))
}

// padStickers are the stickers in these notes, the newest note's first, each once.
func padStickers(notes []*noteRec) []string {
	var out []string
	seen := map[string]bool{}
	for i := len(notes) - 1; i >= 0; i-- {
		if s := notes[i].Sticker; s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// stickerColour is a sticker's own colour: the same words, the same colour.
func stickerColour(s string) string {
	h := 0
	for _, r := range strings.ToLower(s) {
		h = (h*31 + int(r)) % 100003
	}
	return hslHex(float64(h%360), 0.55, 0.62)
}

// noteHead is the first lines of a note's file (nil: it cannot be read).
// Tabs become spaces and other control letters are left out.
func noteHead(path string, lines int) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	buf := make([]byte, 8192)
	k, _ := f.Read(buf)
	text := strings.ReplaceAll(string(buf[:k]), "\t", "    ")
	out := []string{}
	for _, l := range strings.Split(text, "\n") {
		if len(out) >= lines {
			break
		}
		out = append(out, strings.Map(func(r rune) rune {
			if r < 0x20 || r == 0x7f {
				return -1
			}
			return r
		}, l))
	}
	return out
}

// firstWords is the first line with something on it ("" none).
func firstWords(head []string) string {
	for _, l := range head {
		if t := strings.TrimSpace(l); t != "" {
			return t
		}
	}
	return ""
}

// ---- drawing cells ----------------------------------------------------------------------

// padPen writes the cells of a notepad drawing onto a canvas, from a
// corner; nothing is written outside its w × h.
type padPen struct {
	c      *boardCanvas
	x0, y0 int
	w, h   int
	paper  string // writing on this colour leaves out its spaces: the lines show between the words
}

// set writes one cell s (bg "": the board shows through).
func (p padPen) set(x, y int, s, fg, bg string, bold bool) {
	p.setW(x, y, s, 1, fg, bg, bold)
}

// setW writes s, wide cells wide.
func (p padPen) setW(x, y int, s string, wide int, fg, bg string, bold bool) {
	if x < 0 || y < 0 || x+wide > p.w || y >= p.h {
		return
	}
	cx, cy := p.x0+x, p.y0+y
	if cx < 0 || cy < 0 || cx+wide > p.c.Width() || cy >= p.c.Height() {
		return
	}
	cell := uv.Cell{Content: s, Width: wide}
	if fg != "" {
		cell.Style.Fg = rgbaOf(fg)
	}
	if bg != "" {
		cell.Style.Bg = rgbaOf(bg)
	}
	if bold {
		cell.Style.Attrs = uv.AttrBold
	}
	p.c.SetCell(cx, cy, &cell)
}

// text writes s from column x of row y, at most room cells (cut with …).
// It gives the column after it.
func (p padPen) text(x, y int, s string, room int, fg, bg string, bold bool) int {
	if room <= 0 {
		return x
	}
	for _, r := range ansi.Truncate(s, room, "…") {
		rw := runewidth.RuneWidth(r)
		if rw == 0 {
			continue
		}
		if r == ' ' && p.paper != "" && bg == p.paper {
			x++
			continue
		}
		p.setW(x, y, string(r), rw, fg, bg, bold)
		x += rw
	}
	return x
}

// fill paints cells x0 … x1-1 of row y with s.
func (p padPen) fill(x0, x1, y int, s, fg, bg string) {
	for x := x0; x < x1; x++ {
		p.set(x, y, s, fg, bg, false)
	}
}

// ---- a notepad's parts, as cells ----------------------------------------------------------

// padBinding draws row y: what holds the pages at the top, w wide. The
// cover's top row is y+1 (the spiral's holes and the clip's jaw go there).
func padBinding(p padPen, s padStyle, y, w int, chosen bool) {
	metal := "#c9c4bc"
	dark := mixHex(s.Cover, "#000000", 0.4)
	if chosen {
		metal, dark = cAccent, mixHex(cAccent, "#000000", 0.25)
	}
	switch s.Binding {
	case bindSpiral: // wire rings through holes in the cover
		for x := 1; x < w-1; x += 2 {
			p.set(x, y, "⌀", metal, "", true)
			p.set(x, y+1, "•", mixHex(s.Cover, "#000000", 0.55), s.Cover, false)
		}
	case bindGlued: // a band of glue along the top
		p.fill(0, w, y, "▆", dark, "")
		for x := 2; x < w-1; x += 5 {
			p.set(x, y, "▇", mixHex(dark, "#ffffff", 0.15), "", false)
		}
	case bindStitched: // a cloth tape, sewn
		p.fill(0, w, y, " ", "", dark)
		for x := 1; x < w-1; x += 3 {
			p.set(x, y, "╳", mixHex(s.Paper, metal, 0.3), dark, false)
		}
	case bindClip: // a clipboard's clip in the middle
		cw := min(max(w/3, 4), w-2)
		cx := (w - cw) / 2
		p.set(cx, y, "▗", metal, "", false)
		p.fill(cx+1, cx+cw-1, y, "▄", metal, "")
		p.set(cx+cw-1, y, "▖", metal, "", false)
		p.fill(cx, cx+cw, y+1, "▀", metal, s.Cover)
		p.set(cx+cw/2, y, "◦", mixHex(metal, "#000000", 0.5), metal, false)
	}
}

// padPattern is the cover's pattern at (x, y): a letter, and whether that
// cell is the darker shade (checks).
func padPattern(pat, x, y int) (string, bool) {
	switch pat {
	case 1: // stripes
		if (x+y)%4 == 0 {
			return "╱", false
		}
	case 2: // dots
		if x%3 == 1 && y%2 == 0 {
			return "·", false
		}
	case 3: // checks
		return " ", (x/2+y)%2 == 0
	case 4: // waves
		if y%2 == 0 {
			return []string{"⌒", " "}[(x/2)%2], false
		}
		return []string{" ", "‿"}[(x/2)%2], false
	case 5: // grain
		if (x*7+y*13)%9 == 0 {
			return "┈", false
		}
	case 6: // diamonds
		if (x+2*y)%6 == 0 {
			return "◇", false
		}
	}
	return " ", false
}

// padCover paints rows y0 … y1-1 (columns x0 … x1-1) in the cover's colour and pattern.
func padCover(p padPen, s padStyle, x0, x1, y0, y1 int) {
	mark := mixHex(s.Cover, inkOn(s.Cover), 0.22)
	shade := mixHex(s.Cover, "#000000", 0.18)
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			ch, dark := padPattern(s.Pattern, x, y)
			bg := s.Cover
			if dark {
				bg = shade
			}
			p.set(x, y, ch, mark, bg, false)
		}
	}
}

// coverText writes text on the cover, in the middle of row y, on a plain
// strip so the pattern does not run through it.
func coverText(p padPen, s padStyle, y, w int, text, fg string, bold bool) {
	text = ansi.Truncate(text, max(w-4, 1), "…")
	tw := ansi.StringWidth(text)
	x := (w - tw) / 2
	p.fill(x-1, x+tw+1, y, " ", "", s.Cover)
	p.text(x, y, text, tw, fg, s.Cover, bold)
}

// ruleAt is the page's ruling at (x, y) of the page (margin: the margin's column).
func ruleAt(ruling, x, y, margin int) string {
	switch ruling {
	case ruleLined:
		return "▁"
	case ruleMargin:
		if x == margin {
			return "│"
		}
		return "▁"
	case ruleGrid:
		if y%2 == 0 {
			if x%2 == 0 {
				return "┼"
			}
			return "─"
		}
		if x%2 == 0 {
			return "│"
		}
	case ruleDots:
		if x%2 == 0 {
			return "·"
		}
	}
	return " "
}

// padPaper paints rows y0 … y1-1 (columns x0 … x1-1) as the page, with its ruling.
func padPaper(p padPen, s padStyle, x0, x1, y0, y1 int) {
	margin := 3
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			fg := mixHex(s.Paper, s.Rule, 0.55)
			if s.Ruling == ruleMargin && x-x0 == margin {
				fg = mixHex(s.Paper, "#c0392b", 0.6)
			}
			p.set(x, y, ruleAt(s.Ruling, x-x0, y-y0, margin), fg, s.Paper, false)
		}
	}
}

// linedPaper is the paper's colour if its pages are lined (words are
// written on the lines, which show between them), else "".
func linedPaper(s padStyle) string {
	if s.Ruling == ruleLined || s.Ruling == ruleMargin {
		return s.Paper
	}
	return ""
}

// padTextX is where writing starts on a page that starts at x0 (past its margin).
func padTextX(s padStyle, x0 int) int {
	if s.Ruling == ruleMargin {
		return x0 + 5
	}
	return x0 + 2
}

// padChip writes a sticker tag at (x, y), at most room wide, and gives the column after it.
func padChip(p padPen, x, y int, sticker string, room int) int {
	if room < 3 {
		return x
	}
	bg := stickerColour(sticker)
	return p.text(x, y, " "+sticker+" ", room, inkOn(bg), bg, false)
}

// padCorners shapes the corners of a notepad w wide: top is the cover's
// first row, bottom the last row (the pages' edges, drawn in edge).
func padCorners(p padPen, s padStyle, top, bottom, w int, edge string) {
	switch s.Corner {
	case 1: // rounded
		p.set(0, top, "▟", s.Cover, "", false)
		p.set(w-1, top, "▙", s.Cover, "", false)
		p.set(0, bottom, "▝", edge, "", false)
		p.set(w-1, bottom, "▘", edge, "", false)
	case 2: // dog-eared: the top page's corner folded over
		p.set(w-1, bottom-1, "◤", mixHex(s.Paper, "#000000", 0.22), mixHex(s.Paper, "#000000", 0.1), false)
	case 3: // cut
		p.set(0, top, "◢", s.Cover, "", false)
		p.set(w-1, top, "◣", s.Cover, "", false)
		p.set(0, bottom, " ", "", "", false)
		p.set(w-1, bottom, " ", "", "", false)
	}
}

// ---- the notepad seen from the front ------------------------------------------------------

// notepadFront draws notepad n, called name, w × h, seen from the front:
// its binding, its cover with its name and how full it is, and the top
// page with the newest note's title, its first words and the stickers in
// the notepad. chosen: drawn as the chosen structure.
func notepadFront(n int, name string, w, h int, chosen bool) *boardCanvas {
	c := newBoardCanvas(w, h)
	s := notepadStyle(n)
	p := padPen{c: c, w: w, h: h, paper: linedPaper(s)}
	notes := notesLoad().inPad(n)
	coverH := max(2, (h-2)*2/5)
	pageTop := 1 + coverH
	left, right := 0, w // the page's columns: a clipboard's board shows round its page
	if s.Binding == bindClip {
		left, right = 1, w-1
		padCover(p, s, 0, w, pageTop, h)
	}
	padCover(p, s, 0, w, 1, pageTop)
	padBinding(p, s, 0, w, chosen)

	// The cover: the name, how many notes, a bar for how full.
	coverInk := inkOn(s.Cover)
	nameRow := 1 + max(0, (coverH-2)/2)
	nameFG := coverInk
	if chosen {
		nameFG = mixHex(coverInk, cAccent, 0.6)
	}
	coverText(p, s, nameRow, w, name, nameFG, true)
	if nameRow+1 < pageTop {
		coverText(p, s, nameRow+1, w, itoa(len(notes))+"/"+itoa(notesPerPad)+" notes", coverInk, false)
	}
	if nameRow+2 < pageTop && w-4 >= notesPerPad {
		coverText(p, s, nameRow+2, w, strings.Repeat("▰", len(notes))+strings.Repeat("▱", max(notesPerPad-len(notes), 0)),
			mixHex(coverInk, s.Cover, 0.25), false)
	}

	// The top page.
	pageEnd := h - 1
	padPaper(p, s, left, right, pageTop, pageEnd)
	tx := padTextX(s, left)
	room := right - 1 - tx
	if len(notes) == 0 {
		p.text(tx, pageTop, "no notes yet", room, mixHex(s.Ink, s.Paper, 0.45), s.Paper, false)
	} else {
		newest := notes[len(notes)-1]
		p.text(tx, pageTop, noteTitle(newest), room, s.Ink, s.Paper, true)
		if pageEnd-pageTop >= 4 {
			words := firstWords(noteHead(newest.Path, 6))
			if !fileExists(newest.Path) {
				words = "missing"
			}
			p.text(tx, pageTop+1, words, room, mixHex(s.Ink, s.Paper, 0.4), s.Paper, false)
		}
	}
	if stickers := padStickers(notes); len(stickers) > 0 && pageEnd-pageTop >= 3 {
		x := tx
		for _, st := range stickers {
			if right-1-x < 4 {
				break
			}
			x = padChip(p, x, pageEnd-1, st, min(right-1-x, ansi.StringWidth(st)+2)) + 1
		}
	}

	// The pages under it, at the bottom edge.
	edge := mixHex(s.Paper, "#000000", 0.28)
	if chosen {
		edge = cAccent
	}
	under := ""
	if s.Binding == bindClip {
		under = s.Cover
	}
	p.fill(left, right, h-1, "▀", edge, under)
	padCorners(p, s, 1, h-1, w, edge)
	return c
}

// notepadLayers draws notepad structure o: its front, or, zoomed far out,
// a block of its cover's colour with its name.
func (d *designState) notepadLayers(o *dOct) []*lipgloss.Layer {
	x, y, w, h := d.rect(o)
	n := notepadOf(o)
	name := strings.TrimSpace(o.Header)
	if name == "" {
		name = "Notepad " + itoa(n)
	}
	if w < 12 || h < 7 {
		label := kindIcons[kNotepad] + name + " " + itoa(len(notesLoad().inPad(n))) + "/" + itoa(notesPerPad)
		return []*lipgloss.Layer{pillLayer(x, y, w, h, ansi.Truncate(label, w, ""), notepadStyle(n).Cover)}
	}
	l := lipgloss.NewLayer("").X(x).Y(y)
	d.preCells(l, notepadFront(n, name, w, h, d.sel == o.ID))
	return []*lipgloss.Layer{l}
}

// ---- the viewer: an open notepad over the board --------------------------------------------

// padViewer is a notepad opened over the board (Enter on it): its notes by
// day, newest first, and the chosen one's first lines.
type padViewer struct {
	n          int       // the notepad shown
	sel        int       // the chosen note, in the order shown
	top        int       // the first list line shown
	filter     string    // only the notes with this sticker ("": all)
	xAt        time.Time // X pressed once: again soon takes the note out
	stickerFor string    // the note whose sticker is being typed

	box       [4]int      // where the viewer is on the board: x, y, w, h
	rowNote   map[int]int // a board row → the note on it
	lastClick time.Time
	lastY     int
}

// shown are the notes the viewer lists: newest first, only the filter's.
func (v *padViewer) shown() []*noteRec {
	all := notesLoad().inPad(v.n)
	var out []*noteRec
	for i := len(all) - 1; i >= 0; i-- {
		if v.filter == "" || all[i].Sticker == v.filter {
			out = append(out, all[i])
		}
	}
	return out
}

// chosen is the chosen note (nil: none).
func (v *padViewer) chosen() *noteRec {
	notes := v.shown()
	if len(notes) == 0 {
		return nil
	}
	v.sel = min(max(v.sel, 0), len(notes)-1)
	return notes[v.sel]
}

// padDay is a day's heading: Today, Yesterday, or the date.
func padDay(t, now time.Time) string {
	t, now = t.Local(), now.Local()
	same := func(a, b time.Time) bool {
		ay, am, ad := a.Date()
		by, bm, bd := b.Date()
		return ay == by && am == bm && ad == bd
	}
	switch {
	case same(t, now):
		return "Today"
	case same(t, now.AddDate(0, 0, -1)):
		return "Yesterday"
	}
	return t.Format("Mon 2 Jan 2006")
}

// padLine is one line of the viewer's list: a day's heading or a note.
type padLine struct {
	day  string
	note int // its place in shown (-1: a heading)
}

// padLines are the list's lines: each day's heading, then its notes.
func padLines(notes []*noteRec, now time.Time) []padLine {
	var out []padLine
	last := ""
	for i, r := range notes {
		if day := padDay(r.Added, now); day != last {
			out = append(out, padLine{day: day, note: -1})
			last = day
		}
		out = append(out, padLine{note: i})
	}
	return out
}

// padViewOpen opens notepad n over the board.
func (m *Model) padViewOpen(n int) {
	if n < 1 {
		return
	}
	notesFresh()
	m.padView = &padViewer{n: n}
	m.ds().dirty = true
	if len(notesLoad().inPad(n)) == 0 {
		m.say("Notepad " + itoa(n) + " has no notes. Every .txt saved with a name in Text mode becomes a note.")
		return
	}
	m.say("Notepad " + itoa(n) + ", newest first. Enter opens the chosen note in Text mode · Esc closes.")
}

func (m *Model) padViewClose() {
	m.padView = nil
	m.ds().dirty = true
	m.say("")
}

// padViewHint is the status bar while a notepad is open.
func padViewHint() string {
	return "↑↓ choose  ·  Enter: open in Text mode  ·  F: sticker  ·  / or S: filter  ·  X X: take out  ·  PgUp / PgDn: other notepads  ·  Esc: close"
}

// padViewKey is a key while a notepad is open over the board.
func (m *Model) padViewKey(name string) tea.Cmd {
	v := m.padView
	if name != "x" && name != "X" && name != "shift+x" {
		v.xAt = time.Time{}
	}
	n := len(v.shown())
	switch name {
	case "esc":
		m.padViewClose()
	case "f1":
		m.help = true
	case "up":
		v.sel = max(v.sel-1, 0)
	case "down":
		v.sel = min(v.sel+1, max(n-1, 0))
	case "home":
		v.sel = 0
	case "end":
		v.sel = max(n-1, 0)
	case "pgup":
		m.padViewTurn(-1)
	case "pgdown":
		m.padViewTurn(1)
	case "enter":
		return m.padViewOpenNote()
	case "f", "F", "shift+f":
		return m.padViewSticker()
	case "/", "s", "S", "shift+s":
		m.padViewFilter()
	case "x", "X", "shift+x":
		m.padViewRemove()
	}
	return nil
}

// padViewTurn shows the notepad before (-1) or after (1).
func (m *Model) padViewTurn(dir int) {
	v := m.padView
	var ns []int
	for _, p := range notesLoad().Pads {
		ns = append(ns, p.N)
	}
	sort.Ints(ns)
	next := 0
	for _, n := range ns {
		if (dir < 0 && n < v.n) || (dir > 0 && n > v.n && next == 0) {
			next = n
		}
	}
	if next == 0 {
		if dir < 0 {
			m.say("This is the first notepad.")
		} else {
			m.say("This is the last notepad. A new one starts when this one has 16 notes.")
		}
		return
	}
	*v = padViewer{n: next}
	m.say("Notepad " + itoa(next) + " · " + plural(len(notesLoad().inPad(next)), "note") + ".")
}

// padViewOpenNote opens the chosen note in Text mode. The notepad stays
// open: back in Design mode, it is still there.
func (m *Model) padViewOpenNote() tea.Cmd {
	r := m.padView.chosen()
	if r == nil {
		m.say("No note to open.")
		return nil
	}
	if !fileExists(r.Path) {
		m.sayErr(filepath.Base(r.Path) + " is missing: the file is not at " + shortPath(r.Path) + " any more. X X takes the note out.")
		return nil
	}
	m.ds().save()
	cmd := m.setMode(0)
	return tea.Batch(cmd, m.openPath(r.Path))
}

// padViewSticker asks for the chosen note's sticker.
func (m *Model) padViewSticker() tea.Cmd {
	v := m.padView
	r := v.chosen()
	if r == nil {
		m.say("No note chosen to give a sticker.")
		return nil
	}
	v.stickerFor = r.Path
	return m.ask(pPadSticker, "Sticker for “"+filepath.Base(r.Path)+"” (empty: none): ", r.Sticker)
}

// padStickerEntered gives the note asked about sticker s.
func (m *Model) padStickerEntered(s string) tea.Cmd {
	v := m.padView
	if v == nil {
		return nil
	}
	nf := notesLoad()
	r := nf.note(v.stickerFor)
	if r == nil {
		m.sayErr("That note is not in the notepad any more.")
		return nil
	}
	r.Sticker = strings.TrimSpace(s)
	if err := nf.save(); err != nil {
		m.sayErr("The sticker could not be kept: " + err.Error())
		return nil
	}
	if v.filter != "" && r.Sticker != v.filter {
		v.filter = "" // it would vanish from the list: show all again
	}
	for i, x := range v.shown() { // it stays the chosen one
		if x == r {
			v.sel = i
		}
	}
	if r.Sticker == "" {
		m.say("No sticker on " + filepath.Base(r.Path) + " now.")
	} else {
		m.say("Sticker “" + r.Sticker + "” on " + filepath.Base(r.Path) + ".")
	}
	return nil
}

// padViewFilter shows only the next sticker's notes; after the last, all.
func (m *Model) padViewFilter() {
	v := m.padView
	stickers := padStickers(notesLoad().inPad(v.n))
	if len(stickers) == 0 {
		v.filter = ""
		m.say("No stickers in this notepad yet: F gives the chosen note one.")
		return
	}
	sort.Strings(stickers)
	next := stickers[0]
	if v.filter != "" {
		next = ""
		for i, s := range stickers {
			if s == v.filter && i+1 < len(stickers) {
				next = stickers[i+1]
			}
		}
	}
	v.filter, v.sel, v.top = next, 0, 0
	if next == "" {
		m.say("All notes.")
	} else {
		m.say("Only the notes with the sticker “" + next + "” · / or S: the next sticker.")
	}
}

// padViewRemove takes the chosen note out of its notepad (X twice). Its
// .txt file is not touched. (Saved again in Text mode, it is a note again.)
func (m *Model) padViewRemove() {
	v := m.padView
	r := v.chosen()
	if r == nil {
		m.say("No note chosen.")
		return
	}
	if v.xAt.IsZero() || time.Since(v.xAt) > xTwice {
		v.xAt = time.Now()
		m.say("Press X again to take " + filepath.Base(r.Path) + " out of the notepad (the file stays).")
		return
	}
	v.xAt = time.Time{}
	nf := notesLoad()
	for i, x := range nf.Notes {
		if x == r {
			nf.Notes = append(nf.Notes[:i:i], nf.Notes[i+1:]...)
			break
		}
	}
	if err := nf.save(); err != nil {
		m.sayErr("Could not save the notes: " + err.Error())
		return
	}
	m.say("Took " + filepath.Base(r.Path) + " out of notepad " + itoa(v.n) + ". The file is still at " + shortPath(r.Path) + ".")
}

// padViewClick is a click on the board while a notepad is open: on a note
// it chooses it (twice: opens it); outside the notepad it closes it.
func (m *Model) padViewClick(sx, sy int) tea.Cmd {
	v := m.padView
	b := v.box
	if sx < b[0] || sx >= b[0]+b[2] || sy < b[1] || sy >= b[1]+b[3] {
		m.padViewClose()
		return nil
	}
	i, ok := v.rowNote[sy]
	if !ok {
		return nil
	}
	double := i == v.sel && sy == v.lastY && time.Since(v.lastClick) < 400*time.Millisecond
	v.sel, v.lastY, v.lastClick = i, sy, time.Now()
	if double {
		v.lastClick = time.Time{}
		return m.padViewOpenNote()
	}
	return nil
}

// padViewWheel: the wheel chooses the note above or below.
func (m *Model) padViewWheel(up bool) {
	v := m.padView
	if up {
		v.sel = max(v.sel-1, 0)
	} else {
		v.sel = min(v.sel+1, max(len(v.shown())-1, 0))
	}
}

// paintPadView draws the open notepad in the middle of the board: its
// binding, its cover with what it shows, the list of notes on its page,
// and, if there is room, the chosen note's first lines beside it.
func (m *Model) paintPadView(c *boardCanvas) {
	v := m.padView
	if v == nil {
		return
	}
	s := notepadStyle(v.n)
	cw, ch := c.Width(), c.Height()
	lines := len(padLines(v.shown(), time.Now()))
	bw, bh := min(cw-2, 112), min(ch-2, max(21, lines+8)) // as tall as its list (the preview needs 21)
	if bw < 24 || bh < 8 {
		return
	}
	x0, y0 := (cw-bw)/2, (ch-bh)/2
	v.box = [4]int{x0, y0, bw, bh}
	v.rowNote = map[int]int{}
	p := padPen{c: c, x0: x0, y0: y0, w: bw, h: bh, paper: linedPaper(s)}
	all := notesLoad().inPad(v.n)
	coverInk := inkOn(s.Cover)

	// The binding and the cover band: the name, how full, what is shown.
	padCover(p, s, 0, bw, 1, 4)
	padBinding(p, s, 0, bw, true)
	coverText(p, s, 2, bw, "Notepad "+itoa(v.n)+"  ·  "+itoa(len(all))+"/"+itoa(notesPerPad)+" notes", coverInk, true)
	showing := "all notes"
	if v.filter != "" {
		showing = "sticker “" + v.filter + "”"
	}
	coverText(p, s, 3, bw, "newest first  ·  showing "+showing, mixHex(coverInk, s.Cover, 0.2), false)

	// The page, and the pages under it.
	padPaper(p, s, 0, bw, 4, bh-1)
	edge := mixHex(s.Paper, "#000000", 0.28)
	p.fill(0, bw, bh-1, "▀", edge, "")
	padCorners(p, s, 1, bh-1, bw, edge)

	lw := bw // the list's width; the preview has the rest
	if bw >= 72 {
		lw = bw * 11 / 20
		for y := 4; y < bh-1; y++ {
			p.set(lw, y, "┊", mixHex(s.Paper, s.Rule, 0.8), s.Paper, false)
		}
	}
	paintPadList(p, s, v, v.shown(), lw)
	if lw < bw {
		paintPadPreview(p, s, v.chosen(), lw+2, bw-2, 5, bh-1)
	}
}

// paintPadList draws the notes, by day, on the page's first lw columns.
func paintPadList(p padPen, s padStyle, v *padViewer, notes []*noteRec, lw int) {
	tx := padTextX(s, 0)
	room := lw - 1 - tx
	dim := mixHex(s.Ink, s.Paper, 0.45)
	head := mixHex(s.Cover, s.Ink, 0.2)
	if lum(s.Paper) < 0.5 {
		head = mixHex(s.Cover, "#ffffff", 0.45)
	}
	top, listH := 5, p.h-7
	if len(notes) == 0 {
		msg := "No notes in this notepad yet. Every .txt saved with a name in Text mode is a note."
		if v.filter != "" {
			msg = "No notes with the sticker “" + v.filter + "”. / or S: the next sticker."
		}
		p.text(tx, top, msg, room, dim, s.Paper, false)
		return
	}
	v.chosen() // keeps sel in range
	lines := padLines(notes, time.Now())
	at := 0 // the chosen note's line
	for i, l := range lines {
		if l.note == v.sel {
			at = i
		}
	}
	if at < v.top+1 { // the chosen one shows, with its day's heading if that is just above
		v.top = at
		if at > 0 && lines[at-1].note < 0 {
			v.top = at - 1
		}
	}
	if at >= v.top+listH {
		v.top = at - listH + 1
	}
	v.top = min(max(v.top, 0), max(len(lines)-listH, 0))

	// The columns: time, file name, sticker, first words.
	nameW, chipW := 4, 0
	for _, r := range notes {
		nameW = max(nameW, ansi.StringWidth(filepath.Base(r.Path)))
		if r.Sticker != "" {
			chipW = max(chipW, ansi.StringWidth(r.Sticker)+2)
		}
	}
	nameW = min(nameW, max(room/3, 8))
	chipW = min(chipW, max(room/5, 6))

	for row := 0; row < listH && v.top+row < len(lines); row++ {
		l := lines[v.top+row]
		y := top + row
		if l.note < 0 {
			x := p.text(tx, y, l.day, room, head, s.Paper, true)
			p.fill(x+1, lw-1, y, "─", mixHex(s.Paper, head, 0.4), s.Paper)
			continue
		}
		r := notes[l.note]
		v.rowNote[p.y0+y] = l.note
		fg, bg := s.Ink, s.Paper
		chosen := l.note == v.sel
		if chosen {
			bg = mixHex(s.Paper, cAccent, 0.5)
			fg = inkOn(bg)
			p.fill(1, lw-1, y, " ", fg, bg)
			p.set(1, y, "▸", fg, bg, true)
		}
		x := tx
		p.text(x, y, r.Added.Local().Format("15:04"), 5, mixHex(fg, bg, 0.3), bg, false)
		x += 7
		p.text(x, y, filepath.Base(r.Path), min(nameW, lw-1-x), fg, bg, chosen)
		x += nameW + 2
		if chipW > 0 {
			if r.Sticker != "" {
				padChip(p, x, y, r.Sticker, min(chipW, lw-1-x))
			}
			x += chipW + 2
		}
		words, wfg := firstWords(noteHead(r.Path, 6)), mixHex(fg, bg, 0.4)
		if !fileExists(r.Path) {
			words, wfg = "missing", mixHex(cError, s.Ink, 0.25)
		}
		p.text(x, y, words, lw-1-x, wfg, bg, false)
	}
	if v.top > 0 {
		p.text(lw-3, top, "▲", 1, dim, s.Paper, false)
	}
	if v.top+listH < len(lines) {
		p.text(lw-3, top+listH-1, "▼", 1, dim, s.Paper, false)
	}
}

// paintPadPreview draws note r's details and first lines in columns x0 …
// x1-1, rows y0 … y1-1.
func paintPadPreview(p padPen, s padStyle, r *noteRec, x0, x1, y0, y1 int) {
	if r == nil {
		return
	}
	room := x1 - x0
	dim := mixHex(s.Ink, s.Paper, 0.45)
	y := y0
	line := func(text, fg string, bold bool) {
		if y < y1 {
			p.text(x0, y, text, room, fg, s.Paper, bold)
		}
		y++
	}
	line(filepath.Base(r.Path), s.Ink, true)
	if r.Sticker != "" {
		if y < y1 {
			padChip(p, x0, y, r.Sticker, room)
		}
		y++
	} else {
		line("no sticker  ·  F gives one", dim, false)
	}
	line("first saved "+r.Added.Local().Format("Mon 2 Jan 2006 15:04"), dim, false)
	line("last saved  "+r.Saved.Local().Format("Mon 2 Jan 2006 15:04"), dim, false)
	if !fileExists(r.Path) {
		y++
		line("missing", mixHex(cError, s.Ink, 0.25), true)
		line("The file is not there any more:", dim, false)
		line(shortPath(r.Path), dim, false)
		line("X X takes the note out of the notepad.", dim, false)
		return
	}
	if y < y1 {
		p.fill(x0, x1, y, "┄", mixHex(s.Paper, s.Rule, 0.7), s.Paper)
	}
	y++
	for _, l := range noteHead(r.Path, 10) {
		line(l, s.Ink, false)
	}
}
