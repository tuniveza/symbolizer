package main

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// The textual kinds of Design mode, the cabinet, and the drawer every
// structure has.
//
//	page     text of any length (it scrolls); only text and text-like media
//	note     one page of text: noteRows lines of noteCols letters, no more
//	book, slide, drawing   kept in cabinets for the Book, Slides and Draw
//	         modes; on the board they show like a page
//	cabinet  a stack for each textual kind: pages, books, slides, notes,
//	         drawings. Click a stack to pull it out; drag a thing out of it
//	         onto the board; drop a page or note on the cabinet to file it.
//
// Every other structure has a drawer: the ▾ on its bottom edge pulls it out
// (a small cabinet, the same stacks) and puts it back.

const (
	kOct     = ""
	kCabinet = "cabinet"
	kPage    = "page"
	kNote    = "note"
	kBook    = "book"
	kSlide   = "slide"
	kDrawing = "drawing"
)

// stackKinds are what a cabinet (and every drawer) holds, a stack of each.
var stackKinds = []string{kPage, kBook, kSlide, kNote, kDrawing}

var kindIcons = map[string]string{kOct: "◆", kCabinet: "≣", kPage: "≡", kNote: "✎", kBook: "◫", kSlide: "☐", kDrawing: "✐"}
var kindNames = map[string]string{kOct: "structure", kCabinet: "cabinet", kPage: "page", kNote: "note", kBook: "book", kSlide: "slide", kDrawing: "drawing"}
var stackNames = map[string]string{kPage: "Pages", kBook: "Books", kSlide: "Slides", kNote: "Notes", kDrawing: "Drawings"}

const (
	pageNewW, pageNewH = 32, 14
	pageMinW, pageMinH = 14, 6
	noteCols, noteRows = 28, 8 // one page of a note
	noteW, noteH       = noteCols + 4, noteRows + 4
	cabinetNewW        = 30
	cabinetMinW        = 22
	drawerMinW         = 24
)

// textual: a page, note, book, slide or drawing (they go into cabinets and drawers).
func textual(kind string) bool {
	for _, k := range stackKinds {
		if k == kind {
			return true
		}
	}
	return false
}

func minSize(kind string) (int, int) {
	switch kind {
	case kOct:
		return octMinW, octMinH
	case kNote:
		return noteW, noteH
	case kCabinet:
		return cabinetMinW, 4
	}
	return pageMinW, pageMinH
}

func resizable(o *dOct) bool { return o.Kind != kNote && o.Kind != kCabinet }

// size is a structure's width and height on the board. A note is always one
// page; a cabinet is as tall as its stacks.
func (d *designState) size(o *dOct) (int, int) {
	switch o.Kind {
	case kNote:
		return noteW, noteH
	case kCabinet:
		return max(o.W, cabinetMinW), 3 + stackRowCount(o, false) + 1
	}
	return o.W, o.H
}

// highestID is the highest ID in o and everything in its drawer.
func highestID(o *dOct) int {
	n := o.ID
	for _, it := range o.Items {
		if it != nil {
			n = max(n, highestID(it))
		}
	}
	return n
}

func (d *designState) newKind(kind string, cx, cy int) *dOct {
	o := d.newOct(cx, cy)
	o.Kind = kind
	switch kind {
	case kPage, kBook, kSlide, kDrawing:
		o.W, o.H = pageNewW, pageNewH
	case kNote:
		o.W, o.H = noteW, noteH
		o.Color = hslHex(40+rand.Float64()*30, 0.85, 0.62) // sticky-note yellows
	case kCabinet:
		o.W, o.H = cabinetNewW, 4
		o.Color = hslHex(25+rand.Float64()*20, 0.35, 0.55) // wood
	}
	o.X, o.Y = cx-o.W/2, cy-o.H/2
	return o
}

func newHint(kind string) string {
	switch kind {
	case kOct:
		return "New structure: type its header · Tab goes to the text, then the footer · Esc stops."
	case kCabinet:
		return "New cabinet: type its name · Enter or Esc stops · drop pages and notes on it to file them."
	case kNote:
		return fmt.Sprintf("New note: type its title · Tab goes to the text (one page: %d lines) · Esc stops.", noteRows)
	}
	return "New " + kindNames[kind] + ": type its title · Tab goes to the text (any length) · Esc stops."
}

func selHint(o *dOct) string {
	switch o.Kind {
	case kOct:
		hint := "Drag moves · ◢ resizes · double-click writes · [" + octLayouts[o.Layout%len(octLayouts)].icon + "] or L layout · I image · ▾ or D drawer · ✕ or Delete deletes"
		if o.Image != "" {
			hint += " · ✕ on the picture (Shift+Delete) deletes just it"
		}
		return hint + " · Ctrl+Z brings back"
	case kCabinet:
		return "Click a stack to pull it out · drag a thing out onto the board · drop pages/notes on it · a row's ✕ deletes it · ✕ or Delete deletes the cabinet · Ctrl+Z brings back"
	case kNote:
		return "Drag moves · double-click writes (one page) · drag onto a cabinet or an open drawer to file it · ▾ or D drawer · ✕ or Delete deletes · Ctrl+Z brings back"
	}
	return "Drag moves · ◢ resizes · double-click writes · wheel or PgUp/PgDn scrolls · drag onto a cabinet or open drawer to file it · I text file · ✕ or Delete deletes · Ctrl+Z brings back"
}

// ---- writing --------------------------------------------------------------------

func partsOf(o *dOct) []int {
	switch {
	case o.Kind == kOct:
		return []int{partHeader, partBody, partFooter}
	case o.Kind == kCabinet:
		return []int{partHeader}
	}
	return []int{partHeader, partBody}
}

func hasPart(o *dOct, p int) bool {
	for _, q := range partsOf(o) {
		if q == p {
			return true
		}
	}
	return false
}

// nextPart is the part after p (step 1) or before it (step -1). A cabinet
// has only its name, so moving on ends the writing.
func nextPart(o *dOct, p, step int) int {
	ps := partsOf(o)
	if len(ps) == 1 {
		return partNone
	}
	for i, q := range ps {
		if q == p {
			return ps[(i+step+len(ps))%len(ps)]
		}
	}
	return ps[0]
}

// partAt is the part of o a double-click at row r (of h) writes in.
func partAt(o *dOct, r, h int) int {
	switch o.Kind {
	case kOct:
		return octPartAt(r, h)
	case kCabinet:
		return partHeader
	}
	if r <= 1 {
		return partHeader
	}
	return partBody
}

// textLike keeps text and text-like media (letters, emoji, symbols, box
// drawing, text art) and drops what is not text: control codes and
// terminal colour codes. Tabs become spaces.
func textLike(s string) string {
	s = ansi.Strip(s)
	s = strings.NewReplacer("\r\n", "\n", "\r", "\n", "\t", "    ").Replace(s)
	return strings.Map(func(r rune) rune {
		if r == '\n' || !unicode.IsControl(r) && r != utf8.RuneError {
			return r
		}
		return -1
	}, s)
}

// isText: is this file's content text (not a picture, a program or other data)?
func isText(data []byte) bool {
	return utf8.Valid(data) && !bytes.Contains(data, []byte{0})
}

// noteFits: does this text fit on one note page?
func noteFits(s string) bool {
	return len(strings.Split(ansi.Wrap(s, noteCols, ""), "\n")) <= noteRows
}

// ---- cabinets and drawers --------------------------------------------------------

// A stackRow is one row of a cabinet or drawer: a stack, a thing in it, or
// a way to add one.
type stackRow struct {
	act  string // "stack:page", "item:3" (index into Items), "new:page", "diary", "files", "" (a line)
	text string
}

// stackRows are the rows of o's stacks. Not compact (a cabinet), a line
// parts one stack's drawer from the next.
// stackRowCount is len(stackRows(o, compact)), without making the rows.
func stackRowCount(o *dOct, compact bool) int {
	n := 0
	for i, k := range stackKinds {
		if i > 0 && !compact {
			n++
		}
		n++
		if o.OpenStack != k {
			continue
		}
		for _, it := range o.Items {
			if it != nil && it.Kind == k {
				n++
			}
		}
		n++
		if k == kPage {
			n += 2
		}
	}
	return n
}

func stackRows(o *dOct, compact bool) []stackRow {
	var rows []stackRow
	for i, k := range stackKinds {
		if i > 0 && !compact {
			rows = append(rows, stackRow{"", ""})
		}
		n := 0
		for _, it := range o.Items {
			if it != nil && it.Kind == k {
				n++
			}
		}
		arrow := "▸"
		if o.OpenStack == k {
			arrow = "▾"
		}
		rows = append(rows, stackRow{"stack:" + k, fmt.Sprintf("%s %s %s\x00%d", arrow, kindIcons[k], stackNames[k], n)})
		if o.OpenStack != k {
			continue
		}
		for j, it := range o.Items {
			if it != nil && it.Kind == k {
				rows = append(rows, stackRow{fmt.Sprintf("item:%d", j), "  " + kindIcons[k] + " " + itemTitle(it)})
			}
		}
		rows = append(rows, stackRow{"new:" + k, "  + new " + kindNames[k]})
		if k == kPage {
			rows = append(rows, stackRow{"diary", "  ⇣ pages from the Diary"}, stackRow{"files", "  ⇣ open files (Text mode)"})
		}
	}
	return rows
}

// itemTitle names a page or note: its title, else its first line.
func itemTitle(o *dOct) string {
	if s := strings.TrimSpace(o.Header); s != "" {
		return s
	}
	for _, l := range strings.Split(o.Body, "\n") {
		if s := strings.TrimSpace(l); s != "" {
			return s
		}
	}
	return "untitled " + kindNames[o.Kind]
}

// drawRows draws stack rows w wide.
func drawRows(rows []stackRow, w int, fill, edge string) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		switch {
		case r.act == "":
			out[i] = st(edge, fill).Render(strings.Repeat("─", w))
		case strings.HasPrefix(r.act, "stack:"):
			left, count, _ := strings.Cut(r.text, "\x00")
			left = ansi.Truncate(" "+left, max(w-len(count)-2, 1), "…")
			gap := max(w-lipgloss.Width(left)-len(count)-1, 1)
			out[i] = st(mixHex(edge, cText, 0.5), fill).Bold(true).Render(left) +
				st(mixHex(cDim, fill, 0.3), fill).Render(strings.Repeat("·", gap)) + st(cText, fill).Render(count+" ")
		case strings.HasPrefix(r.act, "item:") && w >= 6:
			// The title, and a ✕ that deletes the thing (itemDeleteAct).
			out[i] = fitBlockLine(centreLine(st(cText, fill).Render(ansi.Truncate(strings.TrimSpace(r.text), w-3, "…")), w-2, fill), w-2, fill) +
				st(cError, fill).Render("✕") + st(cText, fill).Render(" ")
		case strings.HasPrefix(r.act, "item:"):
			out[i] = st(cText, fill).Render(padTo(ansi.Truncate(" "+r.text, w, "…"), w))
		default:
			out[i] = st(mixHex(cDim, fill, 0.15), fill).Italic(true).Render(padTo(ansi.Truncate(" "+r.text, w, "…"), w))
		}
		out[i] = fitBlockLine(out[i], w, fill)
	}
	return out
}

func padTo(s string, w int) string {
	if n := w - lipgloss.Width(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}

// fitBlockLine makes a drawn line exactly w wide.
func fitBlockLine(s string, w int, fill string) string {
	if lipgloss.Width(s) > w {
		s = ansi.Truncate(s, w, "")
	}
	if n := w - lipgloss.Width(s); n > 0 {
		s += st(cText, fill).Render(strings.Repeat(" ", n))
	}
	return s
}

// cabinetLayers draws a cabinet: a heavy frame, its name, and a drawer for
// each stack; the pulled-out one shows what it holds.
func (d *designState) cabinetLayers(o *dOct) []*lipgloss.Layer {
	x, y, w, h := d.rect(o)
	selected := d.sel == o.ID
	edge, fill := o.Color, mixHex(cBG, o.Color, 0.18)
	if selected {
		edge = mixHex(o.Color, cText, 0.35)
	}
	if d.dropOn == o.ID {
		edge, fill = cAccent, mixHex(cBG, cAccent, 0.2)
	}
	if w < 6 || h < 4 {
		pill := mixHex(cBG, o.Color, 0.55)
		return []*lipgloss.Layer{lipgloss.NewLayer(st(cBG, pill).Bold(true).Width(w).MaxWidth(w).
			Render(ansi.Truncate("≣"+o.Header, w, ""))).X(x).Y(y)}
	}
	iw := w - 2
	e := st(edge, fill)
	name := o.Header
	editing := selected && d.editing == partHeader
	title := centreLine(st(mixHex(o.Color, cText, 0.4), fill).Bold(true).Render(ansi.Truncate("≣ "+name, iw-1, "…")), iw, fill)
	switch {
	case editing:
		title = centreLine(st(mixHex(o.Color, cText, 0.4), fill).Bold(true).Render(ansi.TruncateLeft("≣ "+name, max(0, lipgloss.Width("≣ "+name)-(iw-2)), "…"))+
			st(cAccent, fill).Render("▏"), iw, fill)
	case name == "":
		title = centreLine(st(mixHex(cDim, fill, 0.25), fill).Italic(true).Render("≣ cabinet"), iw, fill)
	}
	lines := []string{
		e.Render("┏" + strings.Repeat("━", iw) + "┓"),
		e.Render("┃") + fitBlockLine(title, iw, fill) + e.Render("┃"),
		e.Render("┣" + strings.Repeat("━", iw) + "┫"),
	}
	for _, r := range drawRows(stackRows(o, false), iw, fill, edge) {
		lines = append(lines, e.Render("┃")+r+e.Render("┃"))
	}
	lines = append(lines, e.Render("┗"+strings.Repeat("━", iw)+"┛"))
	if len(lines) > h {
		lines = append(lines[:h-1], lines[len(lines)-1])
	}
	layers := []*lipgloss.Layer{lipgloss.NewLayer(strings.Join(lines, "\n")).X(x).Y(y)}
	return d.octExtras(layers, o, x, y, w, h, edge, fill, false, d.linkMode && d.linkFrom == o.ID)
}

// paperLayers draws a page, note, book, slide or drawing: a sheet with its
// title, its text, and how much text there is at the bottom.
func (d *designState) paperLayers(o *dOct) []*lipgloss.Layer {
	x, y, w, h := d.rect(o)
	selected := d.sel == o.ID
	editing := partNone
	if selected {
		editing = d.editing
	}
	fill := mixHex(cBG, cText, 0.07)
	if o.Kind == kNote {
		fill = mixHex(cBG, o.Color, 0.22)
	}
	edge := mixHex(o.Color, fill, 0.15)
	if selected {
		edge = mixHex(o.Color, cText, 0.35)
	}
	if d.dropOn == o.ID {
		edge = cAccent // something dragged over it would go in
	}
	if d.linkMode && d.linkFrom == o.ID {
		fill = mixHex(fill, o.Color, 0.2)
	}
	if h < 4 || w < 8 {
		pill := mixHex(cBG, o.Color, 0.5)
		return []*lipgloss.Layer{lipgloss.NewLayer(st(cBG, pill).Bold(true).Width(w).MaxWidth(w).
			Render(ansi.Truncate(kindIcons[o.Kind]+itemTitle(o), w, ""))).X(x).Y(y)}
	}
	cw, rows := w-4, h-4
	e := st(edge, fill)
	dim := mixHex(cDim, fill, 0.25)
	caret := st(cAccent, fill).Render("▏")

	title := o.Header
	var head string
	switch {
	case editing == partHeader:
		head = centreLine(st(cText, fill).Bold(true).Render(ansi.TruncateLeft(kindIcons[o.Kind]+" "+title, max(0, lipgloss.Width(title)+2-(cw-1)), "…"))+caret, cw, fill)
	case title == "":
		head = centreLine(st(dim, fill).Italic(true).Render(kindIcons[o.Kind]+" "+kindNames[o.Kind]), cw, fill)
	default:
		head = centreLine(st(mixHex(o.Color, cText, 0.45), fill).Bold(true).Render(ansi.Truncate(kindIcons[o.Kind]+" "+title, cw, "…")), cw, fill)
	}

	// The text: wrapped (a drawing keeps its lines as drawn), from the scroll
	// position; while typing, the end shows.
	var lines []string
	if o.Kind == kDrawing {
		lines = strings.Split(o.Body, "\n")
	} else {
		lines = o.wrapped(cw)
	}
	total := len(lines)
	if o.Body == "" {
		total = 0
	}
	top := 0
	if editing == partBody {
		if last := lines[len(lines)-1]; lipgloss.Width(last) >= cw {
			lines = append(lines, "")
		}
		top = max(0, len(lines)-rows)
	} else {
		o.Scroll = max(0, min(o.Scroll, len(lines)-rows))
		top = o.Scroll
	}
	text := st(cText, fill)
	body := make([]string, rows)
	for i := range rows {
		l := ""
		if top+i < len(lines) {
			l = text.Render(strings.TrimRight(ansi.Truncate(lines[top+i], cw, ""), " "))
		}
		if editing == partBody && top+i == len(lines)-1 {
			l += caret
		}
		if o.Body == "" && editing != partBody && i == 0 {
			l = st(dim, fill).Italic(true).Render(ansi.Truncate("double-click to write", cw, "…"))
		}
		if l != "" && o.Kind != kDrawing { // all text on the board is centred (a drawing keeps its lines as drawn)
			l = centreLine(l, cw, fill)
		}
		body[i] = fitBlockLine(l, cw, fill)
	}

	// The right edge doubles as a scroll bar when the text is longer than the sheet.
	bar := make([]string, rows)
	for i := range bar {
		bar[i] = "│"
	}
	if len(lines) > rows && rows > 0 {
		th := max(1, rows*rows/len(lines))
		tp := (rows - th) * top / max(1, len(lines)-rows)
		for i := tp; i < tp+th && i < rows; i++ {
			bar[i] = "┃"
		}
	}

	info := fmt.Sprintf(" %d lines ", total)
	if o.Kind == kNote {
		info = fmt.Sprintf(" %d/%d lines ", len(strings.Split(ansi.Wrap(o.Body, noteCols, ""), "\n"))*boolInt(o.Body != ""), noteRows)
	}
	info = ansi.Truncate(info, max(w-4, 0), "")
	topLine := "╭" + strings.Repeat("─", w-2) + "╮"
	if o.Kind != kNote {
		topLine = "┌" + strings.Repeat("─", w-3) + "◹" // the folded corner of a sheet
	}
	out := []string{
		e.Render(topLine),
		e.Render("│ ") + fitBlockLine(head, cw, fill) + e.Render(" │"),
		e.Render("├" + strings.Repeat("┈", w-2) + "┤"),
	}
	for i := range rows {
		out = append(out, e.Render("│ ")+body[i]+e.Render(" "+bar[i]))
	}
	bl, br := "└", "┘"
	if o.Kind == kNote {
		bl, br = "╰", "╯"
	}
	out = append(out, e.Render(bl+strings.Repeat("─", max(w-3-lipgloss.Width(info), 0)))+st(dim, fill).Render(info)+e.Render("─"+br))
	layers := []*lipgloss.Layer{lipgloss.NewLayer(strings.Join(out, "\n")).X(x).Y(y)}
	return d.octExtras(layers, o, x, y, w, h, edge, fill, selected && resizable(o), d.linkMode && d.linkFrom == o.ID)
}

// wrapped is the body wrapped at w columns. It is worked out again only when
// the body or the width changes. The caller may append to it but not change it.
func (o *dOct) wrapped(w int) []string {
	if o.wrapLines != nil && o.wrapW == w && o.wrapBody == o.Body {
		return o.wrapLines[:len(o.wrapLines):len(o.wrapLines)]
	}
	if o.wrapW != w {
		o.wrapParas = nil
	}
	old := o.wrapParas
	paras := make(map[string][]string, len(old))
	lines := make([]string, 0, len(o.wrapLines)+1)
	for p := range strings.SplitSeq(o.Body, "\n") {
		wl, ok := paras[p]
		if !ok {
			if wl, ok = old[p]; !ok {
				wl = strings.Split(ansi.Wrap(p, w, ""), "\n")
			}
			paras[p] = wl
		}
		lines = append(lines, wl...)
	}
	o.wrapBody, o.wrapW, o.wrapLines, o.wrapParas = o.Body, w, lines, paras
	return o.wrapLines[:len(o.wrapLines):len(o.wrapLines)]
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// handleSpot is where a structure's drawer handle (▾) is on the screen.
func (d *designState) handleSpot(o *dOct, x, y, w, h int) (hx, hy int, label string, ok bool) {
	if o.Kind == kCabinet || w < 12 || h < 3 {
		return 0, 0, "", false
	}
	arrow := "▾"
	if o.DrawerOpen {
		arrow = "▴"
	}
	label = " " + arrow + " "
	if n := len(o.Items); n > 0 {
		label = fmt.Sprintf(" %s%d ", arrow, n)
	}
	return x + (w-lipgloss.Width(label))/2, y + h - 1, label, true
}

// drawerRect is where o's pulled-out drawer is: under it, a little narrower.
func (d *designState) drawerRect(o *dOct) (dx, dy, dw, dh int) {
	x, y, w, h := d.rect(o)
	dw = max(w-4, drawerMinW)
	return x + (w-dw)/2, y + h, dw, len(stackRows(o, true)) + 2
}

// structLayers draws any structure, with its drawer.
func (d *designState) structLayers(o *dOct) []*lipgloss.Layer {
	var layers []*lipgloss.Layer
	switch {
	case o.Kind == kOct:
		layers = d.octLayers(o)
	case o.Kind == kCabinet:
		return d.cabinetLayers(o)
	default:
		layers = d.paperLayers(o)
	}
	x, y, w, h := d.rect(o)
	hx, hy, label, ok := d.handleSpot(o, x, y, w, h)
	if !ok {
		return layers
	}
	edge := mixHex(o.Color, cText, 0.2)
	if d.dropOn == o.ID {
		edge = cAccent
	}
	if o.DrawerOpen {
		dx, dy, dw, _ := d.drawerRect(o)
		fill := mixHex(cBG, o.Color, 0.12)
		if d.dropOn == o.ID {
			fill = mixHex(cBG, cAccent, 0.2)
		}
		e := st(edge, fill)
		iw := dw - 2
		cap := " drawer "
		lines := []string{e.Render("┌" + cap + strings.Repeat("─", max(iw-len(cap), 0)) + "┐")}
		for _, r := range drawRows(stackRows(o, true), iw, fill, edge) {
			lines = append(lines, e.Render("│")+r+e.Render("│"))
		}
		pull := "═○═"
		left := (iw - 3) / 2
		lines = append(lines, e.Render("└"+strings.Repeat("─", left)+pull+strings.Repeat("─", max(iw-3-left, 0))+"┘"))
		layers = append(layers, lipgloss.NewLayer(strings.Join(lines, "\n")).X(dx).Y(dy))
	}
	layers = append(layers, lipgloss.NewLayer(st(cBG, edge).Bold(true).Render(label)).X(hx).Y(hy))
	return layers
}

// ---- clicking -------------------------------------------------------------------

type hitWhere int

const (
	hitNone hitWhere = iota
	hitBody
	hitHandle     // the drawer's ▾, or the pull of an open drawer
	hitToggle     // an octagon's layout toggle
	hitRow        // a row of a cabinet or an open drawer
	hitDrawer     // an open drawer, not on a row
	hitDelete     // the ✕ on the chosen structure
	hitDelPicture // the ✕ on the chosen octagon's picture
)

type hitInfo struct {
	o     *dOct
	where hitWhere
	act   string
}

// hit is what is under a canvas cell: the top structure (or its open
// drawer), and which part of it.
func (d *designState) hit(sx, sy int) hitInfo {
	for i := len(d.octs) - 1; i >= 0; i-- {
		o := d.octs[i]
		x, y, w, h := d.rect(o)
		if dx, dy, ok := d.deleteSpot(o, x, y, w, h); ok && sx == dx && sy == dy {
			return hitInfo{o, hitDelete, ""}
		}
		if dx, dy, ok := d.pictureDeleteSpot(o, x, y); ok && sx == dx && sy == dy {
			return hitInfo{o, hitDelPicture, ""}
		}
		if hx, hy, label, ok := d.handleSpot(o, x, y, w, h); ok {
			if sy == hy && sx >= hx && sx < hx+lipgloss.Width(label) {
				return hitInfo{o, hitHandle, ""}
			}
			if o.DrawerOpen {
				dx, dy, dw, dh := d.drawerRect(o)
				if sx >= dx && sx < dx+dw && sy >= dy && sy < dy+dh {
					rows := stackRows(o, true)
					if r := sy - dy - 1; r >= 0 && r < len(rows) && sx > dx && sx < dx+dw-1 {
						return hitInfo{o, hitRow, itemDeleteAct(rows[r].act, sx-dx-1, dw-2)}
					}
					if sy == dy+dh-1 {
						return hitInfo{o, hitHandle, ""}
					}
					return hitInfo{o, hitDrawer, ""}
				}
			}
		}
		if sx < x || sx >= x+w || sy < y || sy >= y+h {
			continue
		}
		if o.Kind == kOct {
			if tx, ty, ok := layoutToggleSpot(x, y, w, h); ok && sy == ty && sx >= tx && sx < tx+3 {
				return hitInfo{o, hitToggle, ""}
			}
		}
		if o.Kind == kCabinet && w >= 6 && h >= 4 {
			rows := stackRows(o, false)
			if r := sy - y - 3; r >= 0 && r < len(rows) && rows[r].act != "" && sx > x && sx < x+w-1 {
				return hitInfo{o, hitRow, itemDeleteAct(rows[r].act, sx-x-1, w-2)}
			}
		}
		return hitInfo{o, hitBody, ""}
	}
	return hitInfo{}
}

// designRow acts on a click on a row of a cabinet or drawer.
func (m *Model) designRow(o *dOct, act string, sx, sy int) tea.Cmd {
	d := m.ds()
	d.drag = dragNone
	d.dirty = true
	switch {
	case strings.HasPrefix(act, "stack:"):
		k := strings.TrimPrefix(act, "stack:")
		if o.OpenStack == k {
			o.OpenStack = ""
		} else {
			o.OpenStack = k
		}
	case strings.HasPrefix(act, "del:"):
		var i int
		fmt.Sscanf(act, "del:%d", &i)
		m.designDeleteItem(o, i)
	case strings.HasPrefix(act, "item:"):
		var i int
		fmt.Sscanf(act, "item:%d", &i)
		if i < 0 || i >= len(o.Items) || o.Items[i] == nil {
			return nil
		}
		// Take it out: it lands under the mouse and follows it until let go.
		it := o.Items[i]
		o.Items = append(o.Items[:i:i], o.Items[i+1:]...)
		wx, wy := d.world(sx, sy)
		iw, _ := d.size(it)
		it.X, it.Y = int(wx)-iw/2, int(wy)-1
		d.octs = append(d.octs, it)
		d.sel, d.editing = it.ID, partNone
		d.origX, d.origY = it.X, it.Y
		d.pressX, d.pressY = sx, sy
		d.drag = dragMove
		m.say(kindIcons[it.Kind] + " " + itemTitle(it) + " taken out of " + octName(o) + ".")
	case strings.HasPrefix(act, "new:"):
		k := strings.TrimPrefix(act, "new:")
		x, y, w, _ := d.rect(o)
		wx, wy := d.world(x+w+2, y)
		it := d.newKind(k, int(wx), int(wy))
		it.X, it.Y = int(wx), int(wy)
		d.editing = partHeader
		m.say("New " + kindNames[k] + " beside " + octName(o) + ": type its title · drag it back in to file it.")
	case act == "diary":
		m.designFromDiary(o)
	case act == "files":
		m.designFromFiles(o)
	}
	return nil
}

// designDrawer pulls o's drawer out, or puts it back.
func (m *Model) designDrawer(o *dOct) {
	if o.Kind == kCabinet {
		m.say("A cabinet is all drawers: click a stack to pull it out.")
		return
	}
	o.DrawerOpen = !o.DrawerOpen
	m.ds().dirty = true
	if o.DrawerOpen {
		m.say("Drawer of " + octName(o) + " pulled out: drop pages and notes in it; drag them out again.")
	}
}

// dropTarget is the cabinet or open drawer (or drawer handle) at (sx, sy)
// that o, being dragged, would go into.
func (d *designState) dropTarget(o *dOct, sx, sy int) *dOct {
	for i := len(d.octs) - 1; i >= 0; i-- {
		p := d.octs[i]
		if p == o {
			continue
		}
		x, y, w, h := d.rect(p)
		if hx, hy, label, ok := d.handleSpot(p, x, y, w, h); ok {
			if sy == hy && sx >= hx && sx < hx+lipgloss.Width(label) {
				return p
			}
			if p.DrawerOpen {
				dx, dy, dw, dh := d.drawerRect(p)
				if sx >= dx && sx < dx+dw && sy >= dy && sy < dy+dh {
					return p
				}
			}
		}
		if sx >= x && sx < x+w && sy >= y && sy < y+h {
			if p.Kind == kCabinet {
				return p
			}
			return nil // over another structure: not a place to file it
		}
	}
	return nil
}

// designFileInto puts the page or note it into the cabinet (or drawer) of to.
func (m *Model) designFileInto(it, to *dOct) {
	d := m.ds()
	if it == nil || to == nil || it == to || !textual(it.Kind) {
		return
	}
	d.remove(it.ID)
	it.X, it.Y = 0, 0
	to.Items = append(to.Items, it)
	to.OpenStack = it.Kind
	d.dirty = true
	where := "the drawer of " + octName(to)
	if to.Kind == kCabinet {
		where = octName(to)
	}
	m.say(kindIcons[it.Kind] + " " + itemTitle(it) + " put in " + where + " (" + stackNames[it.Kind] + ").")
}

func countItems(o *dOct) int {
	n := 0
	for _, it := range o.Items {
		if it != nil {
			n += 1 + countItems(it)
		}
	}
	return n
}

// ---- from other modes -------------------------------------------------------------

// designFromDiary files every Diary page into o's Pages stack (each once;
// a page already there is brought up to date).
func (m *Model) designFromDiary(o *dOct) {
	d := m.ds()
	added, fresh := 0, 0
	for _, ch := range loadDiary() {
		for n, path := range ch.pages {
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			title := fmt.Sprintf("%s · page %d", chapterTitle(ch.day), n+1)
			if m.designPutFrom(d, o, path, title, string(data)) {
				added++
			} else {
				fresh++
			}
		}
	}
	m.sayFiled(o, added, fresh, "Diary pages", "The Diary has no pages yet.")
}

// designFromFiles files the files open in Text mode into o's Pages stack.
func (m *Model) designFromFiles(o *dOct) {
	d := m.ds()
	added, fresh := 0, 0
	for _, t := range m.tabs {
		if t == nil || t.buf == nil {
			continue
		}
		text := t.buf.Text()
		if strings.TrimSpace(text) == "" && t.buf.Path == "" {
			continue
		}
		title := "untitled"
		if t.buf.Path != "" {
			title = filepath.Base(t.buf.Path)
		}
		if m.designPutFrom(d, o, t.buf.Path, title, text) {
			added++
		} else {
			fresh++
		}
	}
	m.sayFiled(o, added, fresh, "open files", "No files are open in Text mode.")
}

// designPutFrom adds a page from somewhere, or updates the one already
// there from the same place. It says whether it was added.
func (m *Model) designPutFrom(d *designState, o *dOct, from, title, text string) bool {
	text = textLike(text)
	if from != "" {
		for _, it := range o.Items {
			if it != nil && it.From == from {
				it.Body = text
				d.dirty = true
				return false
			}
		}
	}
	it := &dOct{ID: d.nextID, Kind: kPage, W: pageNewW, H: pageNewH, Header: title, Body: text, From: from, Color: randomOctColor()}
	d.nextID++
	o.Items = append(o.Items, it)
	o.OpenStack = kPage
	d.dirty = true
	return true
}

func (m *Model) sayFiled(o *dOct, added, fresh int, what, none string) {
	if added+fresh == 0 {
		m.say(none)
		return
	}
	msg := fmt.Sprintf("%d %s put in %s", added, what, octName(o))
	if fresh > 0 {
		msg += fmt.Sprintf(" (%d already there, brought up to date)", fresh)
	}
	m.say(msg + ".")
}

// designPaste: a paste (or a file dropped on the terminal) on the board.
// While writing, the text goes in. A dropped picture goes into the chosen
// octagon (or a new one); a dropped text file, or pasted text, goes into
// the chosen page or note (or a new page).
func (m *Model) designPaste(text string) tea.Cmd {
	d := m.ds()
	defer d.save()
	o := d.byID(d.sel)
	if paths := droppedPaths(text); len(paths) > 0 {
		// Only names of files that are there: a drop (terminals paste a
		// dropped file as its path), even while writing, so the file goes
		// in rather than its name being typed.
		d.editing = partNone
		cx, cy := d.world(m.width/2, m.textHeight()/2)
		return m.dropFiles(paths, o, cx, cy)
	}
	if d.editing != partNone && o != nil {
		field := map[int]*string{partHeader: &o.Header, partBody: &o.Body, partFooter: &o.Footer}[d.editing]
		add := textLike(text)
		if d.editing != partBody {
			add = strings.ReplaceAll(add, "\n", " ")
		}
		before := *field
		*field += add
		if o.Kind == kNote && d.editing == partBody && !noteFits(o.Body) {
			*field = before
			m.say(fmt.Sprintf("That does not fit: a note holds one page (%d lines). Use a page (P) for more.", noteRows))
			return nil
		}
		d.dirty = true
		if m.growToFit(o) {
			m.keepInView(o, d.editing == partBody)
		}
		return nil
	}
	if path := droppedPath(text); path != "" {
		return m.designUseFile(path)
	}
	if o == nil || o.Kind == kOct || o.Kind == kCabinet {
		cx, cy := d.world(m.width/2, m.textHeight()/2)
		o = d.newKind(kPage, int(cx), int(cy))
	}
	m.designAddText(o, textLike(text))
	return nil
}

// designAddText puts text at the end of a page or note.
func (m *Model) designAddText(o *dOct, text string) {
	d := m.ds()
	body := o.Body
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	body += text
	if o.Kind == kNote && !noteFits(body) {
		m.say(fmt.Sprintf("That does not fit: a note holds one page (%d lines). Use a page (P) for more.", noteRows))
		return
	}
	o.Body = body
	d.dirty = true
	m.growToFit(o) // it grows to show the text (up to the screen's height, then it scrolls)
	m.say(fmt.Sprintf("Text put in %s.", octName(o)))
}

// droppedPath is the file a paste names, when it is just one existing file
// (terminals paste a dropped file as its path, sometimes quoted or as a
// file:// address).
func droppedPath(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.Contains(s, "\n") {
		return ""
	}
	s = strings.Trim(s, `'"`)
	s = strings.TrimPrefix(s, "file://")
	s = strings.ReplaceAll(s, `\ `, " ")
	if strings.Contains(s, "%") {
		s = unescapeURL(s)
	}
	if !strings.HasPrefix(s, "/") && !strings.HasPrefix(s, "~") {
		return ""
	}
	p := expandPath(s)
	if info, err := os.Stat(p); err == nil && !info.IsDir() {
		return p
	}
	return ""
}

func unescapeURL(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			var b byte
			if _, err := fmt.Sscanf(s[i+1:i+3], "%02x", &b); err == nil {
				sb.WriteByte(b)
				i += 2
				continue
			}
		}
		sb.WriteByte(s[i])
	}
	return sb.String()
}
