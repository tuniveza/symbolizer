package main

// ---- the drawer: a front with a handle, a tray behind it ---------------------------------
//
// Shut, a drawer is its front, a long low box: its name, how many things
// it holds, and a metal handle. A click on the front (or Space or Tab, with
// it chosen) pulls it out: its tray slides down out of the front, each
// thing in it small (a miniature: its outline, icon and one label), side by
// side, row after row. A click on one takes it out (it follows the mouse). The
// handle again pushes it in. Drop anything on a drawer to put it in.
//
// Being pulled out is looking around, not a change (the saved DrawerOpen).

import (
	"math"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// drawerW is a drawer's width on the board.
func drawerW(o *dOct) int { return max(o.W, cabinetMinW) }

// drawerCols is how many miniatures go side by side in a drawer's tray (its
// frame, the tray's two sides and a cell of margin take the rest).
func drawerCols(o *dOct) int { return max(1, (drawerW(o)-2-2-1)/(miniW+1)) }

// drawerThings are the indexes (in o.Items) of what a drawer holds.
func drawerThings(o *dOct) []int {
	var idx []int
	for j, it := range o.Items {
		if it != nil {
			idx = append(idx, j)
		}
	}
	return idx
}

// drawerRows is how many rows of miniatures a drawer's tray has.
func drawerRows(o *dOct) int {
	cols := drawerCols(o)
	return max(1, (len(drawerThings(o))+cols-1)/cols)
}

// drawerFrontLines is how many rows a drawer's front takes at 100%: its
// edge, its name, its handle, and its bottom edge. Shut, that is all of
// it: a long low box.
const drawerFrontLines = 4

// drawerOpenLines is how many rows a pulled-out drawer takes at 100%: its
// front, and under it its tray and the tray's bottom edge.
func drawerOpenLines(o *dOct) int { return drawerFrontLines + drawerRows(o)*miniH + 1 }

// drawerLines is how many rows a drawer takes at 100% when still: its front,
// and pulled out, its tray under it.
func drawerLines(o *dOct) int {
	if o.DrawerOpen {
		return drawerOpenLines(o)
	}
	return drawerFrontLines
}

// drawerLinesNow is drawerLines as it is drawn now: opening or shutting,
// the tray slides down out of the front, or back up into it.
func (d *designState) drawerLinesNow(o *dOct) int {
	p := d.openness(o)
	return drawerFrontLines + int(math.Round(p*float64(drawerOpenLines(o)-drawerFrontLines)))
}

// drawerLay is where a drawer's parts are as it is drawn now, w × h on the
// screen at zoom z: line numbers from its top edge.
type drawerLay struct {
	iw                int // inside its frame
	fr                int // rows its front takes (edge, name, handle, bottom edge)
	titleRow, handle  int // the lines of its name and its handle (the same when the front is low)
	trayVis, trayAll  int // tray rows showing now, and pulled right out
	cols, fit, mw, mh int // miniatures: side by side, rows of them that fit, and their size
	cw                int // the tray's inside width
}

func drawerLayout(o *dOct, w, h int, z float64) drawerLay {
	l := drawerLay{iw: w - 2}
	l.fr = min(max(int(math.Round(drawerFrontLines*z)), 3), drawerFrontLines, h)
	l.titleRow, l.handle = 1, 2
	if l.fr < drawerFrontLines {
		l.handle = 1 // low: the handle beside its name
	}
	l.trayVis = max(h-l.fr-1, 0)
	l.trayAll = max(int(math.Round(float64(drawerOpenLines(o))*z))-l.fr-1, 1)
	l.cw = max(l.iw-2, 1)
	l.cols = drawerCols(o)
	l.mw = max(3, (l.cw-1)/l.cols-1) // zoomed in, the things grow with the tray
	if l.cols*(l.mw+1)+1 > l.cw {    // zoomed far out: fewer side by side
		l.cols = max(1, (l.cw-1)/(l.mw+1))
	}
	rows := max(1, (len(drawerThings(o))+l.cols-1)/l.cols)
	l.mh = max(1, l.trayAll/rows)
	l.fit = min(rows, l.trayAll/l.mh)
	return l
}

// drawerHit is what a click does on line li of a drawer w × h on the
// screen (0: its top edge), col cells into its inside.
func (d *designState) drawerHit(o *dOct, li, col, w, h int) string {
	l := drawerLayout(o, w, h, d.z())
	if li == l.handle || li == l.titleRow {
		return "pull" // its handle, or its name beside it: the front is what pulls
	}
	t := li - l.fr // the tray's row
	if !o.DrawerOpen || t < 0 || t >= min(l.trayVis, l.fit*l.mh) {
		return ""
	}
	c := col - 1 - 1 // the tray's side, and its margin
	if c < 0 || c%(l.mw+1) >= l.mw {
		return ""
	}
	mc, mr := c/(l.mw+1), t/l.mh
	if mc >= l.cols {
		return ""
	}
	things := drawerThings(o)
	if k := mr*l.cols + mc; k < len(things) {
		return "item:" + itoa(things[k])
	}
	return ""
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

// drawerLayers draws a drawer: its front (a long low box: its name and a
// metal handle) and, pulled out, its tray under it, the things in it small,
// side by side. Opening, the tray slides down out of the front; shutting,
// back up into it (design_anim.go). At any zoom every thing in it shows,
// smaller.
func (d *designState) drawerLayers(o *dOct) []*lipgloss.Layer {
	x, y, w, h := d.rect(o)
	selected := d.sel == o.ID
	edge, fill := o.Color, mixHex(cBG, o.Color, 0.16)
	if selected {
		edge = mixHex(o.Color, cText, 0.35)
	}
	if d.dropOn == o.ID {
		edge, fill = cAccent, mixHex(cBG, cAccent, 0.2)
	}
	if w < 8 || h < 3 {
		pill := mixHex(cBG, o.Color, 0.55)
		return []*lipgloss.Layer{pillLayer(x, y, w, h, ansi.Truncate(kindIcons[o.Kind]+o.Header, w, ""), pill)}
	}
	l := drawerLayout(o, w, h, d.z())
	iw := l.iw
	e := st(edge, fill)
	front := mixHex(o.Color, fill, 0.5)  // the front's wood
	inside := mixHex(cBG, o.Color, 0.08) // the tray, in its shadow
	wall := mixHex(o.Color, cBG, 0.45)
	name := o.Header
	if n := len(drawerThings(o)); n > 0 {
		name = strings.TrimSpace(name + " · " + itoa(n))
	}
	head := st(mixHex(o.Color, cText, 0.45), front).Bold(true)
	title := head.Render(ansi.Truncate(kindIcons[o.Kind]+" "+name, iw-1, "…"))
	switch {
	case selected && d.editing == partHeader:
		title = head.Render(ansi.TruncateLeft(kindIcons[o.Kind]+" "+o.Header, max(0, lipgloss.Width(o.Header)+2-(iw-2)), "…")) +
			st(cAccent, front).Render("▏")
	case o.Header == "":
		title = st(mixHex(cDim, front, 0.25), front).Italic(true).Render(ansi.Truncate(kindIcons[o.Kind]+" drawer", iw, ""))
	}
	handleW := min(max(iw/2, 7), 26)
	row := func(s, bg string) string { return e.Render("│") + fitBlockLine(s, iw, bg) + e.Render("│") }

	// The front: its edge, its name, its handle (beside the name when low).
	lines := []string{e.Render("┌" + strings.Repeat("─", iw) + "┐")}
	if l.handle == l.titleRow {
		short := title + st(cText, front).Render("  ") + drawerHandle(min(handleW, max(iw-lipgloss.Width(title)-4, 4)), front, o.Color)
		lines = append(lines, row(centreLine(short, iw, front), front))
	} else {
		lines = append(lines, row(centreLine(title, iw, front), front), row(centreLine(drawerHandle(handleW, front, o.Color), iw, front), front))
	}
	if l.trayVis == 0 {
		lines = append(lines, e.Render("└"+strings.Repeat("─", iw)+"┘"))
		layers := []*lipgloss.Layer{lipgloss.NewLayer(strings.Join(lines, "\n")).X(x).Y(y)}
		return d.octExtras(layers, o, x, y, w, h, edge, fill, false, d.linkMode && d.linkFrom == o.ID)
	}
	lines = append(lines, e.Render("├"+strings.Repeat("─", iw)+"┤"))

	// The tray, as much as has slid out: its rows of miniatures.
	var tray []string
	things := drawerThings(o)
	side := st(wall, inside).Render("▐")
	sideR := st(wall, inside).Render("▌")
	trayRow := func(s string) string {
		return e.Render("│") + side + fitBlockLine(s, l.cw, inside) + sideR + e.Render("│")
	}
	if len(things) == 0 {
		tray = append(tray, trayRow(centreLine(st(mixHex(cDim, inside, 0.15), inside).Italic(true).Render(ansi.Truncate("drop anything here", l.cw, "…")), l.cw, inside)))
	}
	shown := min(len(things), l.fit*l.cols)
	hidden := len(things) - shown
	if hidden > 0 && shown > 1 { // zoomed far out: the last place says how many more
		shown--
		hidden++
	}
	for r := 0; r < l.fit && r*l.cols < len(things); r++ {
		minis := make([][]string, 0, l.cols)
		for c := 0; c < l.cols && r*l.cols+c < len(things); c++ {
			k := r*l.cols + c
			if k == shown && hidden > 0 {
				more := st(mixHex(cDim, cText, 0.4), inside).Italic(true).Render(padTo(ansi.Truncate("+"+itoa(hidden), l.mw, ""), l.mw))
				ml := make([]string, l.mh)
				for i := range ml {
					ml[i] = st(cText, inside).Render(strings.Repeat(" ", l.mw))
				}
				ml[(l.mh-1)/2] = more
				minis = append(minis, ml)
				break
			}
			minis = append(minis, miniLines(miniOfSize(o.Items[things[k]], l.mw, l.mh, false, true), inside))
		}
		for my := range l.mh {
			var sb strings.Builder
			sb.WriteString(st(cText, inside).Render(" "))
			for _, mini := range minis {
				sb.WriteString(mini[my])
				sb.WriteString(st(cText, inside).Render(" "))
			}
			tray = append(tray, trayRow(sb.String()))
		}
	}
	for len(tray) < l.trayVis {
		tray = append(tray, trayRow(""))
	}
	lines = append(lines, tray[:l.trayVis]...)
	lines = append(lines, e.Render("└"+strings.Repeat("─", iw)+"┘"))
	if len(lines) > h {
		lines = append(lines[:h-1:h-1], lines[len(lines)-1])
	}
	layers := []*lipgloss.Layer{lipgloss.NewLayer(strings.Join(lines, "\n")).X(x).Y(y)}
	return d.octExtras(layers, o, x, y, w, h, edge, fill, false, d.linkMode && d.linkFrom == o.ID)
}

// designPull pulls a drawer out, or pushes it in.
func (m *Model) designPull(o *dOct) {
	d := m.ds()
	o.DrawerOpen = !o.DrawerOpen
	d.startOpenAnim(o)
	d.dirty = true
	if o.DrawerOpen {
		d.sel = o.ID
		d.toFront(o)
		m.say(octName(o) + " pulled out: click a thing to take it out · the handle pushes it in.")
	} else {
		m.say(octName(o) + " pushed in.")
	}
}
