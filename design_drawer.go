package main

// ---- the drawer unit: drawers stacked, each a front with a handle, a tray behind it --------
//
// A drawer unit (Alt+N, D) is a frame with its name at the top and one or
// more drawers stacked under it ([ and ], with it chosen, or the [−] and
// [+] on its bottom edge: fewer or more). Each drawer is a box of its own:
// shut, it is its front, a low row with a metal handle (and, with more than
// one, its number and how many things it holds). A click on a front pulls
// that drawer out: its tray slides down out of the front, each thing in it
// small (a miniature: its outline, icon and one label), side by side, row
// after row. A click on one takes it out (it follows the mouse). The front
// again pushes it in. Space or Tab, with it chosen, pulls out the next
// drawer (and pushes the open one in). Drop anything on a drawer to put it
// in that drawer.
//
// Being pulled out is looking around, not a change (the saved Pulled and
// DrawerOpen); how many drawers there are, and which drawer each thing is
// in, are changes (Drawers, and each thing's Slot).

import (
	"math"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// drawerMax is the most drawers a unit can have.
const drawerMax = 12

// drawerW is a drawer's width on the board.
func drawerW(o *dOct) int { return max(o.W, cabinetMinW) }

// drawerCols is how many miniatures go side by side in a drawer's tray (its
// frame, the tray's two sides and a cell of margin take the rest).
func drawerCols(o *dOct) int { return max(1, (drawerW(o)-2-2-1)/(miniW+1)) }

// drawerCount is how many drawers a unit has.
func drawerCount(o *dOct) int { return min(max(o.Drawers, 1), drawerMax) }

// slotOf is which of n drawers thing it is in.
func slotOf(it *dOct, n int) int { return min(max(it.Slot, 0), n-1) }

// slotOpen is whether drawer i of o is pulled out. (Boards saved before a
// unit had several drawers keep only DrawerOpen: its one drawer.)
func slotOpen(o *dOct, i int) bool {
	if len(o.Pulled) == 0 {
		return i == 0 && o.DrawerOpen
	}
	return i >= 0 && i < len(o.Pulled) && o.Pulled[i]
}

// setSlotOpen pulls drawer i of o out (or pushes it in); DrawerOpen says
// whether any is out.
func setSlotOpen(o *dOct, i int, open bool) {
	n := drawerCount(o)
	p := make([]bool, n)
	out := false
	for j := range p {
		p[j] = slotOpen(o, j)
		if j == i {
			p[j] = open
		}
		out = out || p[j]
	}
	o.Pulled, o.DrawerOpen = p, out
	if !out {
		o.Pulled = nil
	}
}

// drawerThings are the indexes (in o.Items) of everything a drawer unit
// (or a board) holds.
func drawerThings(o *dOct) []int {
	var idx []int
	for j, it := range o.Items {
		if it != nil {
			idx = append(idx, j)
		}
	}
	return idx
}

// slotThings are the indexes (in o.Items) of what drawer i holds.
func slotThings(o *dOct, i int) []int {
	n := drawerCount(o)
	var idx []int
	for j, it := range o.Items {
		if it != nil && slotOf(it, n) == i {
			idx = append(idx, j)
		}
	}
	return idx
}

// slotRows is how many rows of miniatures drawer i's tray has, cols side by side.
func slotRows(o *dOct, i, cols int) int {
	return max(1, (len(slotThings(o, i))+cols-1)/cols)
}

// drawerFrontLines is how many rows a one-drawer unit takes shut at 100%:
// its edge, its name, its handle, and its bottom edge.
const drawerFrontLines = 4

// drawerHeadLines is how many rows a unit takes at 100% with every drawer
// pushed in: its top edge, its name, each drawer's front (and, with more
// than one, a line above each), and its bottom edge.
func drawerHeadLines(o *dOct) int {
	n := drawerCount(o)
	if n == 1 {
		return drawerFrontLines
	}
	return 3 + 2*n
}

// slotTrayLines is how many rows drawer i's tray adds at 100% pulled right
// out: a line under its front, and its rows of miniatures.
func slotTrayLines(o *dOct, i int) int { return 1 + slotRows(o, i, drawerCols(o))*miniH }

// drawerOpenLines is how many rows a unit takes at 100% with every drawer
// pulled out.
func drawerOpenLines(o *dOct) int {
	n := drawerHeadLines(o)
	for i := range drawerCount(o) {
		n += slotTrayLines(o, i)
	}
	return n
}

// drawerLines is how many rows a unit takes at 100% when still.
func drawerLines(o *dOct) int {
	n := drawerHeadLines(o)
	for i := range drawerCount(o) {
		if slotOpen(o, i) {
			n += slotTrayLines(o, i)
		}
	}
	return n
}

// slotLinesNow is how many rows drawer i's tray adds as it is drawn now:
// opening or shutting, it slides down out of its front, or back up into it.
func (d *designState) slotLinesNow(o *dOct, i int) int {
	p := d.slotOpenness(o, i, slotOpen(o, i))
	return int(math.Round(p * float64(slotTrayLines(o, i))))
}

// drawerLinesNow is drawerLines as it is drawn now.
func (d *designState) drawerLinesNow(o *dOct) int {
	n := drawerHeadLines(o)
	for i := range drawerCount(o) {
		n += d.slotLinesNow(o, i)
	}
	return n
}

// What each line of a drawer unit on the screen is.
const (
	dlTop     = iota // its top edge (zoomed far out, with its name in it)
	dlName           // its name
	dlSep            // the line above a drawer's front
	dlFront          // a drawer's front (row: which of its rows, handle: the one with the handle)
	dlTraySep        // the line between a drawer's front and its tray
	dlTray           // a row of a drawer's tray (row: which, from its top)
	dlBottom         // its bottom edge
)

type drawerLine struct{ kind, slot, row int }

// slotLay is how one drawer's tray is drawn now.
type slotLay struct {
	things    []int // what it holds (indexes in o.Items)
	rows      int   // rows of miniatures in it
	vis       int   // tray rows showing now
	mh, fit   int   // a miniature's height, and how many rows of them show
	handleRow int   // which row of its front has the handle
	sepAbove  bool  // a line above its front (else it is shaded, to tell it from the one above)
}

// shareRows gives out rows by weight, rounded so they add up (the largest
// remainders get the odd rows).
func shareRows(rows int, weight []int) []int {
	out := make([]int, len(weight))
	total := 0
	for _, w := range weight {
		total += w
	}
	if total <= 0 || rows <= 0 {
		return out
	}
	given := 0
	rem := make([]int, len(weight))
	for i, w := range weight {
		out[i] = rows * w / total
		rem[i] = rows * w % total
		given += out[i]
	}
	for given < rows {
		best := -1
		for i, w := range weight {
			if w > 0 && (best < 0 || rem[i] > rem[best]) {
				best = i
			}
		}
		out[best]++
		rem[best] = -1
		given++
	}
	return out
}

func slicesRepeat(v, n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = v
	}
	return out
}

// drawerLay is where a unit's parts are as it is drawn now, w × h on the
// screen.
type drawerLay struct {
	iw, cw   int // inside its frame, and inside a tray
	cols, mw int // miniatures side by side, and their width
	named    bool
	lines    []drawerLine
	slots    []slotLay
}

// drawerLayout works out a unit drawn w × h on the screen. At 100% each
// part has its own rows. Zoomed out, its top part (name, fronts and the
// lines between them) has as many rows as the unit shut at this zoom:
// first the lines between drawers keep theirs, then its name's own row
// (without it, its name goes into its top edge), then the line above the
// first drawer. So pulling a drawer out moves only what is under it. The
// trays get the rest, and the lines under their fronts if there is room.
// Zoomed in, the fronts grow. Only if even a front each does not fit is
// it cut short.
func (d *designState) drawerLayout(o *dOct, w, h int) drawerLay {
	n := drawerCount(o)
	l := drawerLay{iw: w - 2}
	l.cw = max(l.iw-2, 1)
	l.cols = drawerCols(o)
	l.mw = max(3, (l.cw-1)/l.cols-1) // zoomed in, the things grow with the tray
	if l.cols*(l.mw+1)+1 > l.cw {    // zoomed far out: fewer side by side
		l.cols = max(1, (l.cw-1)/(l.mw+1))
	}
	l.slots = make([]slotLay, n)
	now := make([]int, n)  // each tray's rows at 100% as it is now, with its line
	want := make([]int, n) // and its rows of miniatures
	opened, total, natural := 0, 0, drawerHeadLines(o)
	for i := range n {
		l.slots[i].things = slotThings(o, i)
		l.slots[i].rows = max(1, (len(l.slots[i].things)+l.cols-1)/l.cols)
		now[i] = d.slotLinesNow(o, i)
		want[i] = max(now[i]-1, 0)
		natural += now[i]
		total += want[i]
		if want[i] > 0 {
			opened++
		}
	}
	// Its name, fronts and the lines between them are as they are with
	// every drawer in (headH rows at this zoom), so pulling one out moves
	// only what is under it; the trays get the rest.
	room := max(h-2-n, 0) // inside, past a row for each front
	headH := int(math.Round(float64(drawerHeadLines(o)) * d.z()))
	optAvail := n // the lines between drawers, its name's row, and (with several) the line above the first
	if n > 1 {
		optAvail++
	}
	headExtra := min(max(headH-2-n, 0), room)
	optHead := min(headExtra, optAvail)
	grow := headExtra - optHead // zoomed in: the fronts grow
	trays := room - headExtra
	if short := opened - trays; total > 0 && short > 0 { // a row for each tray out: from the fronts, then the lines
		g := min(grow, short)
		grow, short = grow-g, short-g
		g = min(optHead, short)
		optHead, trays = optHead-g, opened
	}
	sepBefore := make([]bool, n)
	traySep := make([]bool, n)
	take := func() bool {
		if optHead > 0 {
			optHead--
			return true
		}
		return false
	}
	for i := 1; i < n; i++ {
		sepBefore[i] = take() // the lines between drawers
	}
	l.named = take()
	if n > 1 {
		sepBefore[0] = take()
	}
	cnt := 0
	for i := range n {
		if now[i] > 0 {
			cnt++
		}
	}
	if trays-cnt >= int(math.Round(float64(total)*d.z())) && trays >= cnt+opened { // room for the lines under the fronts too
		for i := range n {
			traySep[i] = now[i] > 0
		}
		trays -= cnt
	}
	frontH := make([]int, n)
	for i := range frontH {
		frontH[i] = 1
	}
	vis := make([]int, n)
	if total > 0 {
		if trays >= opened {
			for i := range n {
				if want[i] > 0 {
					vis[i] = 1 // a row each first
				}
			}
			trays -= opened
		}
		for i, v := range shareRows(trays, want) {
			vis[i] += v
		}
	} else {
		grow += trays
	}
	for i, v := range shareRows(grow, slicesRepeat(1, n)) {
		frontH[i] += v
	}

	l.lines = append(l.lines, drawerLine{kind: dlTop, slot: -1})
	if l.named {
		l.lines = append(l.lines, drawerLine{kind: dlName, slot: -1})
	}
	for i := range n {
		s := &l.slots[i]
		s.sepAbove = sepBefore[i] || (i == 0 && n == 1)
		if sepBefore[i] {
			l.lines = append(l.lines, drawerLine{kind: dlSep, slot: i})
		}
		s.handleRow = (frontH[i] - 1) / 2
		for r := range frontH[i] {
			l.lines = append(l.lines, drawerLine{kind: dlFront, slot: i, row: r})
		}
		if traySep[i] {
			l.lines = append(l.lines, drawerLine{kind: dlTraySep, slot: i})
		}
		s.vis = vis[i]
		for r := range vis[i] {
			l.lines = append(l.lines, drawerLine{kind: dlTray, slot: i, row: r})
		}
		// Its miniatures: as tall as the tray pulled right out allows.
		all := max(vis[i], 1)
		if want[i] > 0 && want[i] < slotTrayLines(o, i)-1 { // still sliding: as tall as when right out
			all = max(int(math.Round(float64(vis[i])*float64(slotTrayLines(o, i)-1)/float64(want[i]))), 1)
		}
		s.mh = max(1, all/s.rows)
		s.fit = min(s.rows, all/s.mh)
	}
	l.lines = append(l.lines, drawerLine{kind: dlBottom, slot: -1})
	if len(l.lines) > h { // still too many: cut short, keeping its bottom edge
		cut := max(h-1, 0)
		l.lines = append(l.lines[:cut:cut], drawerLine{kind: dlBottom, slot: -1})
	}
	return l
}

// frontHandleW is how wide a drawer's handle is on a front iw wide.
func frontHandleW(n, iw int) int {
	if n == 1 {
		return min(max(iw/2, 7), 26)
	}
	return min(max(iw/3, 6), 18)
}

// drawerCtl is the [−] N drawers [+] on a chosen unit's bottom edge, and
// where it starts in its inside iw wide (-1: it does not fit).
func drawerCtl(o *dOct, iw int) (string, int) {
	s := "[−] " + plural(drawerCount(o), "drawer") + " [+]"
	n := lipgloss.Width(s)
	if n+2 > iw {
		return s, -1
	}
	return s, (iw - n) / 2
}

// drawerHit is what a click does on line li of a unit w × h on the screen
// (0: its top edge), col cells into its inside.
func (d *designState) drawerHit(o *dOct, li, col, w, h int) string {
	l := d.drawerLayout(o, w, h)
	if li < 0 || li >= len(l.lines) {
		return ""
	}
	ln := l.lines[li]
	switch ln.kind {
	case dlName:
		if drawerCount(o) == 1 {
			return "pull:0" // one drawer: its name is on its front, which pulls
		}
	case dlFront:
		if n := drawerCount(o); n > 1 && d.sel == o.ID && ln.row == l.slots[ln.slot].handleRow && l.iw-4 > (l.iw-frontHandleW(n, l.iw))/2+frontHandleW(n, l.iw) {
			switch col {
			case l.iw - 4:
				return "look:" + itoa(ln.slot) // its colour dot
			case l.iw - 2:
				return "tex:" + itoa(ln.slot) // its texture swatch
			}
		}
		return "pull:" + itoa(ln.slot)
	case dlBottom:
		if d.sel != o.ID {
			return ""
		}
		s, at := drawerCtl(o, l.iw)
		if at < 0 {
			return ""
		}
		n := lipgloss.Width(s)
		switch {
		case col >= at && col < at+3:
			return "drawers:-1"
		case col >= at+n-3 && col < at+n:
			return "drawers:+1"
		}
	case dlTray:
		s := l.slots[ln.slot]
		if ln.row >= s.fit*s.mh {
			return ""
		}
		c := col - 1 - 1 // the tray's side, and its margin
		if c < 0 || c%(l.mw+1) >= l.mw {
			return ""
		}
		mc, mr := c/(l.mw+1), ln.row/s.mh
		if mc >= l.cols {
			return ""
		}
		if k := mr*l.cols + mc; k < len(s.things) {
			return "item:" + itoa(s.things[k])
		}
	}
	return ""
}

// drawerSlotAt is which drawer of unit o is at screen cell (sx, sy): its
// front, the line above it, or its tray (-1: none, or not a drawer unit).
func (d *designState) drawerSlotAt(o *dOct, sx, sy int) int {
	if o == nil || o.Kind != kDrawer {
		return -1
	}
	x, y, w, h := d.rect(o)
	if sx < x || sx >= x+w || w < 8 || h < 3 {
		return -1
	}
	l := d.drawerLayout(o, w, h)
	if li := sy - y; li >= 0 && li < len(l.lines) {
		return l.lines[li].slot
	}
	return -1
}

// drawerHandle is a drawer's handle, n cells wide on wood front: a round
// metal bar, lit from above (light along its top, a shine a third of the
// way along, shade along its bottom), on a dark mount at each end.
func drawerHandle(n int, front, tint string) string {
	n = max(n, 4)
	base := mixHex("#aeb4ba", tint, 0.12)
	mount := mixHex(base, "#20242a", 0.3)
	var sb strings.Builder
	sb.WriteString(st(mount, front).Render("▐"))
	for i := range n - 2 {
		t := (float64(i) + 0.5) / float64(n-2)
		shine := math.Exp(-math.Pow((t-0.32)/0.22, 2))
		top := mixHex(base, "#ffffff", 0.25+0.6*shine)
		bottom := mixHex(base, "#1a1d22", 0.45-0.15*shine)
		sb.WriteString(st(top, bottom).Render("▀"))
	}
	sb.WriteString(st(mount, front).Render("▌"))
	return sb.String()
}

// drawerLayers draws a drawer unit: its frame and name, and each drawer's
// front (a low row with a metal handle) and, pulled out, its tray under
// it, the things in it small, side by side. Opening, a tray slides down
// out of its front; shutting, back up into it (design_anim.go). At any
// zoom every thing in an open drawer shows, smaller.
func (d *designState) drawerLayers(o *dOct) []*lipgloss.Layer {
	x, y, w, h := d.rect(o)
	selected := d.sel == o.ID
	edge, fill := o.Color, mixHex(boardBG(), o.Color, 0.16)
	if selected {
		edge = mixHex(o.Color, cText, 0.35)
	}
	if d.dropOn == o.ID {
		edge, fill = cAccent, mixHex(boardBG(), cAccent, 0.2)
	}
	if w < 8 || h < 3 {
		pill := mixHex(boardBG(), o.Color, 0.55)
		return []*lipgloss.Layer{pillLayer(x, y, w, h, ansi.Truncate(kindIcons[o.Kind]+o.Header, w, ""), pill)}
	}
	n := drawerCount(o)
	l := d.drawerLayout(o, w, h)
	iw := l.iw
	e := st(edge, fill)
	wood := mixHex(o.Color, fill, 0.5)         // the fronts' wood
	inside := mixHex(boardBG(), o.Color, 0.08) // a tray, in its shadow (tinted with its drawer's colour below)
	wall := mixHex(o.Color, boardBG(), 0.45)
	headBG := fill // with several drawers the name is on the frame, with one on its front
	if n == 1 {
		headBG = wood
	}
	name := o.Header
	if c := len(drawerThings(o)); c > 0 {
		name = strings.TrimSpace(name + " · " + itoa(c))
	}
	head := st(mixHex(o.Color, cText, 0.45), headBG).Bold(true)
	title := head.Render(ansi.Truncate(kindIcons[o.Kind]+" "+name, iw-1, "…"))
	switch {
	case selected && d.editing == partHeader:
		title = head.Render(ansi.TruncateLeft(kindIcons[o.Kind]+" "+o.Header, max(0, lipgloss.Width(o.Header)+2-(iw-2)), "…")) +
			st(cAccent, headBG).Render("▏")
	case o.Header == "":
		title = st(mixHex(cDim, headBG, 0.25), headBG).Italic(true).Render(ansi.Truncate(kindIcons[o.Kind]+" drawer", iw, ""))
	}
	row := func(s, bg string) string { return e.Render("│") + fitBlockLine(s, iw, bg) + e.Render("│") }
	trayRowIn := func(i int, s string) string {
		in := mixHex(boardBG(), drawerColor(o, i), 0.1)
		wl := mixHex(drawerColor(o, i), boardBG(), 0.4)
		return e.Render("│") + st(wl, in).Render("▐") + fitBlockLine(s, l.cw, in) + st(wl, in).Render("▌") + e.Render("│")
	}
	// Each tray's rows, as they are when it is pulled right out.
	trays := make([][]string, n)
	for i, s := range l.slots {
		if s.vis == 0 {
			continue
		}
		inside := inside
		if n > 1 {
			inside = mixHex(boardBG(), drawerColor(o, i), 0.1) // tinted with its drawer's colour
		}
		trayRow := func(s string) string { return trayRowIn(i, s) }
		if n == 1 {
			trayRow = func(s string) string {
				return e.Render("│") + st(wall, inside).Render("▐") + fitBlockLine(s, l.cw, inside) + st(wall, inside).Render("▌") + e.Render("│")
			}
		}
		var tray []string
		things := s.things
		if len(things) == 0 {
			tray = append(tray, trayRow(centreLine(st(mixHex(cDim, inside, 0.15), inside).Italic(true).Render(ansi.Truncate("drop anything here", l.cw, "…")), l.cw, inside)))
		}
		shown := min(len(things), s.fit*l.cols)
		hidden := len(things) - shown
		if hidden > 0 && shown > 1 { // zoomed far out: the last place says how many more
			shown--
			hidden++
		}
		for r := 0; r < s.fit && r*l.cols < len(things); r++ {
			minis := make([][]string, 0, l.cols)
			for c := 0; c < l.cols && r*l.cols+c < len(things); c++ {
				k := r*l.cols + c
				if k == shown && hidden > 0 {
					more := st(mixHex(cDim, cText, 0.4), inside).Italic(true).Render(padTo(ansi.Truncate("+"+itoa(hidden), l.mw, ""), l.mw))
					ml := make([]string, s.mh)
					for j := range ml {
						ml[j] = st(cText, inside).Render(strings.Repeat(" ", l.mw))
					}
					ml[(s.mh-1)/2] = more
					minis = append(minis, ml)
					break
				}
				minis = append(minis, miniLines(miniOfSize(o.Items[things[k]], l.mw, s.mh, false, true), inside))
			}
			for my := range s.mh {
				var sb strings.Builder
				sb.WriteString(st(cText, inside).Render(" "))
				for _, mini := range minis {
					sb.WriteString(mini[my])
					sb.WriteString(st(cText, inside).Render(" "))
				}
				tray = append(tray, trayRow(sb.String()))
			}
		}
		for len(tray) < s.vis {
			tray = append(tray, trayRow(""))
		}
		trays[i] = tray[:s.vis]
	}

	var lines []string
	for _, ln := range l.lines {
		switch ln.kind {
		case dlTop:
			if l.named {
				lines = append(lines, e.Render("┌"+strings.Repeat("─", iw)+"┐"))
				break
			}
			label := ansi.Truncate(" "+kindIcons[o.Kind]+" "+name+" ", max(iw-1, 0), "…")
			lines = append(lines, e.Render("┌")+st(mixHex(o.Color, cText, 0.45), fill).Bold(true).Render(label)+
				e.Render(strings.Repeat("─", max(iw-lipgloss.Width(label), 0))+"┐"))
		case dlName:
			lines = append(lines, row(centreLine(title, iw, headBG), headBG))
		case dlSep, dlTraySep:
			lines = append(lines, e.Render("├"+strings.Repeat("─", iw)+"┤"))
		case dlFront:
			s := l.slots[ln.slot]
			front := mixHex(drawerColor(o, ln.slot), fill, 0.5) // a one-drawer unit: its wood, as before
			if n > 1 {
				front = mixHex(drawerColor(o, ln.slot), fill, 0.2) // each drawer in its own colour
			}
			if slotOpen(o, ln.slot) {
				front = mixHex(front, cText, 0.06) // pulled out: lit a little more
			}
			if !s.sepAbove && ln.slot%2 == 1 {
				front = mixHex(front, "#000000", 0.14) // no line above it: shaded, to tell it from the one above
			}
			if d.dropOn == o.ID && d.dropSlot == ln.slot {
				front = mixHex(front, cAccent, 0.45) // a drag would go in here
			}
			y := 0 // the row's place in the unit, for its texture
			for k, l2 := range l.lines {
				if l2 == ln {
					y = k
				}
			}
			if ln.row != s.handleRow {
				lines = append(lines, e.Render("│")+frontRow(o, ln.slot, front, y, iw, "", "", 0, false)+e.Render("│"))
				break
			}
			// Its number and what it holds at the left (with several), its
			// handle in the middle, and chosen, its colour dot and texture swatch.
			hw, label := frontHandleW(n, iw), ""
			if n > 1 {
				label = " " + itoa(ln.slot+1)
				if c := len(s.things); c > 0 {
					label += " · " + itoa(c)
				}
			}
			lines = append(lines, e.Render("│")+frontRow(o, ln.slot, front, y, iw, label, inkOn(front), hw, selected && n > 1)+e.Render("│"))
		case dlTray:
			if t := trays[ln.slot]; ln.row < len(t) {
				lines = append(lines, t[ln.row])
			} else {
				lines = append(lines, trayRowIn(ln.slot, ""))
			}
		case dlBottom:
			bottom := e.Render("└" + strings.Repeat("─", iw) + "┘")
			if selected {
				if s, at := drawerCtl(o, iw); at >= 0 {
					ctl := st(cAccent, fill).Bold(true)
					cnt := strings.TrimSuffix(strings.TrimPrefix(s, "[−]"), "[+]")
					bottom = e.Render("└"+strings.Repeat("─", at)) + ctl.Render("[−]") + st(mixHex(o.Color, cText, 0.5), fill).Render(cnt) +
						ctl.Render("[+]") + e.Render(strings.Repeat("─", max(iw-at-lipgloss.Width(s), 0))+"┘")
				}
			}
			lines = append(lines, bottom)
		}
	}
	layers := []*lipgloss.Layer{lipgloss.NewLayer(strings.Join(lines, "\n")).X(x).Y(y)}
	return d.octExtras(layers, o, x, y, w, h, edge, fill, false, d.linkMode && d.linkFrom == o.ID)
}

// drawerName is "Box" for a one-drawer unit, "drawer 2 of Box" for a drawer of several.
func drawerName(o *dOct, i int) string {
	if drawerCount(o) == 1 {
		return octName(o)
	}
	return "drawer " + itoa(i+1) + " of " + octName(o)
}

// designPullSlot pulls drawer i of o out, or pushes it in.
func (m *Model) designPullSlot(o *dOct, i int) {
	d := m.ds()
	if i < 0 || i >= drawerCount(o) {
		return
	}
	open := !slotOpen(o, i)
	setSlotOpen(o, i, open)
	d.startSlotAnim(o, i, open)
	d.dirty = true
	if open {
		d.sel = o.ID
		d.toFront(o)
		m.say(drawerName(o, i) + " pulled out: click a thing to take it out · its front pushes it in.")
	} else {
		m.say(drawerName(o, i) + " pushed in.")
	}
}

// designPull (Space, Tab) pulls out o's next drawer and pushes in the one
// that was out; after the last one, all are in. back: the one before.
func (m *Model) designPull(o *dOct, back bool) {
	d := m.ds()
	n := drawerCount(o)
	cur := -1
	for i := range n {
		if slotOpen(o, i) {
			cur = i
			if back {
				break
			}
		}
	}
	next := cur + 1
	if back {
		next = cur - 1
		if cur < 0 {
			next = n - 1
		}
	}
	if next >= n {
		next = -1
	}
	for i := range n {
		if slotOpen(o, i) && i != next {
			setSlotOpen(o, i, false)
			d.startSlotAnim(o, i, false)
		}
	}
	d.dirty = true
	if next < 0 {
		m.say(octName(o) + " pushed in.")
		return
	}
	setSlotOpen(o, next, true)
	d.startSlotAnim(o, next, true)
	d.sel = o.ID
	d.toFront(o)
	if n == 1 {
		m.say(octName(o) + " pulled out: click a thing to take it out · the handle pushes it in.")
		return
	}
	m.say(drawerName(o, next) + " pulled out: click a thing to take it out · Tab: the next drawer.")
}

// designDrawerCount gives unit o delta more drawers (or fewer). What was in
// a drawer that goes moves into the new last one.
func (m *Model) designDrawerCount(o *dOct, delta int) {
	d := m.ds()
	n := drawerCount(o)
	nn := min(max(n+delta, 1), drawerMax)
	if nn == n {
		if delta < 0 {
			m.say(octName(o) + " has one drawer: it cannot have fewer.")
		} else {
			m.say(octName(o) + " has " + itoa(drawerMax) + " drawers, the most it can have.")
		}
		return
	}
	open := make([]bool, nn)
	for i := range open {
		open[i] = slotOpen(o, i)
	}
	moved := 0
	for _, it := range o.Items {
		if it != nil && slotOf(it, n) >= nn {
			it.Slot = nn - 1
			moved++
		}
	}
	o.Drawers = nn
	if nn == 1 {
		o.Drawers = 0
	}
	o.Pulled, o.DrawerOpen = nil, false
	for i, v := range open {
		if v {
			setSlotOpen(o, i, true)
		}
	}
	d.dirty = true
	msg := octName(o) + ": " + plural(nn, "drawer") + "."
	if moved > 0 {
		msg += " " + plural(moved, "thing") + " moved into drawer " + itoa(nn) + "."
	}
	m.say(msg + " [ fewer · ] more.")
}
