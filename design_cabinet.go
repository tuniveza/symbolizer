package main

// ---- the cabinet: two doors, shelves behind them ---------------------------------------
//
// A cabinet is a cupboard with two doors. It is the same size shut or
// open. Shut, it shows its name, how many things it holds, and the two
// doors with their knobs; a click on the doors (or Tab, with it chosen)
// opens them: they swing open inside its frame, one each side, and between
// them is a shelf for each
// kind of sheet (pages, books, slides, notes, drawings): the things on it
// side by side, each its icon and title, and under them the shelf's board
// with its name and a + that makes a new one. A click on a thing takes it
// out (it follows the mouse); "+N ▸" at a shelf's end shows the rest of
// it; the last row brings pages in from the Diary or the open files. A
// click on an open door shuts them.
//
// The doors being open is looking around, not a change: undo passes over
// it (it is the saved DrawerOpen).

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// cabDoorW is how wide each open door is, swung open inside the cabinet's
// frame, one at each side.
const cabDoorW = 2

// cabShelfW is how wide the shelves are between a cabinet's open doors,
// in a cabinet w wide on the screen.
func cabShelfW(w int) int { return max(w-2-2*cabDoorW, 1) }

// cabinetLines is how many rows a cabinet takes, shut or open: its frame
// and name, and inside a shelf (two rows) for each kind and a row for
// bringing pages in.
func cabinetLines(o *dOct) int { return 3 + 2*len(stackKinds) + 1 + 1 }

// cabSeg is a piece of a row behind the doors: its words, what a click on
// it does, and how it looks.
type cabSeg struct {
	text, act string
	look      int
}

const (
	lookPlain = iota
	lookItem
	lookBoard
	lookDim
	lookAct
)

func shelfKey(o *dOct, kind string) string { return itoa(o.ID) + ":" + kind }

// cabinetShelves are the rows behind the open doors, sw wide: each shelf's
// things and its board, then the row for bringing pages in.
func (d *designState) cabinetShelves(o *dOct, sw int) [][]cabSeg {
	var rows [][]cabSeg
	for _, k := range stackKinds {
		var idx []int
		for j, it := range o.Items {
			if it != nil && it.Kind == k {
				idx = append(idx, j)
			}
		}
		start := d.shelfAt[shelfKey(o, k)]
		if start >= len(idx) {
			start = 0
		}
		row := []cabSeg{{" ", "", lookPlain}}
		used, shown := 1, 0
		for n, j := range idx[start:] {
			tok := kindIcons[k] + " " + ansi.Truncate(itemTitle(o.Items[j]), 10, "…")
			gap := 0
			if shown > 0 {
				gap = 2
			}
			rest := len(idx) - start - n - 1
			more := 0
			if rest > 0 || start > 0 {
				more = len([]rune("+"+itoa(rest)+" ▸")) + 2
			}
			if used+gap+lipgloss.Width(tok)+more > sw {
				if shown > 0 {
					break
				}
				tok = ansi.Truncate(tok, max(sw-used-more, 1), "…") // one at least, cut short
			}
			if gap > 0 {
				row = append(row, cabSeg{"  ", "", lookPlain})
			}
			row = append(row, cabSeg{tok, "item:" + itoa(j), lookItem})
			used += gap + lipgloss.Width(tok)
			shown++
		}
		switch hidden := len(idx) - start - shown; {
		case len(idx) == 0:
			row = append(row, cabSeg{"empty", "", lookDim})
		case hidden > 0:
			row = append(row, cabSeg{"  ", "", lookPlain}, cabSeg{"+" + itoa(hidden) + " ▸", "shelf:" + k, lookAct})
		case start > 0:
			row = append(row, cabSeg{"  ", "", lookPlain}, cabSeg{"◂ first", "shelf:" + k, lookAct})
		}
		rows = append(rows, row)
		// The shelf's board: its name, and + for a new one.
		label := "━ " + kindIcons[k] + " " + stackNames[k] + " "
		label = ansi.Truncate(label, max(sw-4, 1), "")
		fill := max(sw-lipgloss.Width(label)-4, 0)
		rows = append(rows, []cabSeg{{label, "", lookBoard}, {strings.Repeat("━", fill), "", lookBoard},
			{" + ", "new:" + k, lookAct}, {"━", "", lookBoard}})
	}
	rows = append(rows, []cabSeg{{" ", "", lookPlain}, {"⇣ diary", "diary", lookAct}, {"  ", "", lookPlain}, {"⇣ files", "files", lookAct}})
	return rows
}

// cabinetHit is what a click does on row r under a cabinet's name (0: the
// first row below its name's line), col cells into its inside iw wide;
// rows is how many rows it has inside on the screen.
func (d *designState) cabinetHit(o *dOct, r, col, iw, rows int) string {
	if r < 0 || r >= rows {
		return ""
	}
	if !o.DrawerOpen {
		return "doors" // a click on the shut doors opens them
	}
	if col < cabDoorW || col >= iw-cabDoorW {
		return "doors" // an open door: a click shuts them
	}
	shelves := d.cabinetShelves(o, cabShelfW(iw+2))
	if r >= len(shelves) {
		return ""
	}
	x := col - cabDoorW
	for _, s := range shelves[r] {
		w := lipgloss.Width(s.text)
		if x < w {
			return s.act
		}
		x -= w
	}
	return ""
}

// cabinetDoorHit is whether screen cell (sx, sy) is on one of an open
// cabinet's doors, inside its frame at (x, y), w × h.
func cabinetDoorHit(o *dOct, x, y, w, h, sx, sy int) bool {
	if o.Kind != kCabinet || !o.DrawerOpen || w < 10 || h < 5 || sy < y+3 || sy > y+h-2 {
		return false
	}
	return (sx > x && sx <= x+cabDoorW) || (sx >= x+w-1-cabDoorW && sx < x+w-1)
}

// cabinetLayers draws a cabinet: a heavy frame, its name, and its doors,
// shut over its shelves or swung open beside it.
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
	if w < 10 || h < 5 {
		pill := mixHex(cBG, o.Color, 0.55)
		return []*lipgloss.Layer{pillLayer(x, y, w, h, ansi.Truncate("≣"+o.Header, w, ""), pill)}
	}
	iw := w - 2
	e := st(edge, fill)
	door := mixHex(o.Color, fill, 0.55) // the doors' wood
	dst := st(edge, door)
	knob := st(mixHex(o.Color, cText, 0.6), door).Bold(true)
	name := o.Header
	if n := countItems(o); n > 0 {
		name = strings.TrimSpace(name + " · " + itoa(n))
	}
	head := st(mixHex(o.Color, cText, 0.4), fill).Bold(true)
	title := centreLine(head.Render(ansi.Truncate("≣ "+name, iw-1, "…")), iw, fill)
	switch {
	case selected && d.editing == partHeader:
		title = centreLine(head.Render(ansi.TruncateLeft("≣ "+o.Header, max(0, lipgloss.Width("≣ "+o.Header)-(iw-2)), "…"))+
			st(cAccent, fill).Render("▏"), iw, fill)
	case o.Header == "":
		title = centreLine(st(mixHex(cDim, fill, 0.25), fill).Italic(true).Render(ansi.Truncate("≣ cabinet", iw, "")), iw, fill)
	}
	lines := []string{
		e.Render("┏" + strings.Repeat("━", iw) + "┓"),
		e.Render("┃") + fitBlockLine(title, iw, fill) + e.Render("┃"),
	}
	inside := h - 4 // rows inside on the screen: the cabinet is its board size times the zoom
	var layers []*lipgloss.Layer
	if !o.DrawerOpen {
		// Shut: two doors, a knob each by the middle, all the way down.
		lw := (iw - 1) / 2
		rw := iw - 1 - lw
		lines = append(lines, e.Render("┣"+strings.Repeat("━", lw)+"┳"+strings.Repeat("━", rw)+"┫"))
		for i := range inside {
			l, r := dst.Render(strings.Repeat(" ", lw)), dst.Render(strings.Repeat(" ", rw))
			if i == inside/2 {
				l = dst.Render(strings.Repeat(" ", max(lw-2, 0))) + knob.Render("●") + dst.Render(" ")
				r = dst.Render(" ") + knob.Render("●") + dst.Render(strings.Repeat(" ", max(rw-2, 0)))
			}
			lines = append(lines, e.Render("┃")+l+e.Render("┃")+r+e.Render("┃"))
		}
		lines = append(lines, e.Render("┗"+strings.Repeat("━", lw)+"┻"+strings.Repeat("━", rw)+"┛"))
		layers = append(layers, lipgloss.NewLayer(strings.Join(lines, "\n")).X(x).Y(y))
	} else {
		// Open: the shelves, between the doors swung open inside its
		// frame, so it is the same size as shut.
		lines = append(lines, e.Render("┣"+strings.Repeat("━", iw)+"┫"))
		sw := cabShelfW(w)
		var rows []string
		for _, row := range d.cabinetShelves(o, sw) {
			var sb strings.Builder
			for _, s := range row {
				switch s.look {
				case lookItem:
					sb.WriteString(st(cText, fill).Render(s.text))
				case lookBoard:
					sb.WriteString(st(edge, fill).Render(s.text))
				case lookDim:
					sb.WriteString(st(mixHex(cDim, fill, 0.2), fill).Italic(true).Render(s.text))
				case lookAct:
					sb.WriteString(st(cAccent, fill).Bold(true).Render(s.text))
				default:
					sb.WriteString(st(cText, fill).Render(s.text))
				}
			}
			rows = append(rows, fitBlockLine(sb.String(), sw, fill))
		}
		blank := st(cText, fill).Render(strings.Repeat(" ", sw))
		if len(rows) > inside && inside >= 1 { // the rows that do not fit
			n := len(rows) - (inside - 1)
			rows = append(rows[:inside-1:inside-1], fitBlockLine(centreLine(st(mixHex(cDim, cText, 0.4), fill).Italic(true).Render(ansi.Truncate("+"+itoa(n)+" more", sw, "…")), sw, fill), sw, fill))
		}
		for len(rows) < inside {
			rows = append(rows, blank)
		}
		// Each door, open on its hinge at the frame: seen edge on, its knob
		// on the inside, a gap at its top and bottom.
		for i := range inside {
			l, r := dst.Render(" ")+dst.Render("▕"), dst.Render("▏")+dst.Render(" ")
			switch {
			case i == 0 || i == inside-1:
				l, r = st(cText, fill).Render(" ")+dst.Render("▕"), dst.Render("▏")+st(cText, fill).Render(" ")
			case i == inside/2:
				l, r = knob.Render("●")+dst.Render("▕"), dst.Render("▏")+knob.Render("●")
			}
			lines = append(lines, e.Render("┃")+l+rows[i]+r+e.Render("┃"))
		}
		lines = append(lines, e.Render("┗"+strings.Repeat("━", iw)+"┛"))
		layers = append(layers, lipgloss.NewLayer(strings.Join(lines, "\n")).X(x).Y(y))
	}
	return d.octExtras(layers, o, x, y, w, h, edge, fill, false, d.linkMode && d.linkFrom == o.ID)
}

// designDoors opens a cabinet's doors, or shuts them.
func (m *Model) designDoors(o *dOct) {
	o.DrawerOpen = !o.DrawerOpen
	m.ds().dirty = true
	if o.DrawerOpen {
		m.say(octName(o) + " open: click a thing to take it out · + makes a new one · a door shuts it.")
	} else {
		m.say(octName(o) + " shut.")
	}
}

// designShelf shows the next things on a shelf (or the first again).
func (m *Model) designShelf(o *dOct, kind string) {
	d := m.ds()
	if d.shelfAt == nil {
		d.shelfAt = map[string]int{}
	}
	n := 0
	for _, it := range o.Items {
		if it != nil && it.Kind == kind {
			n++
		}
	}
	// How many show now, from where it starts: the next ones start after them.
	_, _, w, _ := d.rect(o)
	sw := cabShelfW(w)
	start := d.shelfAt[shelfKey(o, kind)]
	shown := 0
	for i, k := range stackKinds {
		if k == kind {
			for _, s := range d.cabinetShelves(o, sw)[2*i] {
				if s.look == lookItem {
					shown++
				}
			}
		}
	}
	next := start + shown
	if next >= n {
		next = 0
	}
	d.shelfAt[shelfKey(o, kind)] = next
	d.dirty = true
}
