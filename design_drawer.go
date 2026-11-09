package main

// ---- the drawer: a front with a handle, a tray behind it ---------------------------------
//
// A drawer shows its front: its name, how many things it holds, and a
// handle. A click on the handle (or Tab, with it chosen) pulls it out: under
// the front is its tray, and in it each thing small (a miniature: its
// outline, icon and one label), side by side, row after row. A click on
// one takes it out (it follows the mouse). The handle again pushes it in.
// Drop anything on a drawer to put it in; it opens to show it.
//
// Being pulled out is looking around, not a change (the saved DrawerOpen).

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// drawerW is a drawer's width on the board.
func drawerW(o *dOct) int { return max(o.W, cabinetMinW) }

// drawerCols is how many miniatures go side by side in a drawer's tray.
func drawerCols(o *dOct) int { return max(1, (drawerW(o)-2-1)/(miniW+1)) }

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

// drawerTrayH is how many screen rows a pulled-out drawer's tray takes,
// cols miniatures side by side.
func drawerTrayH(o *dOct, cols int) int {
	n := len(drawerThings(o))
	if n == 0 {
		return 1
	}
	return (n + cols - 1) / cols * miniH
}

// drawerShown is whether a drawer is shown whole: pulled out and chosen.
// Then, at any zoom, it is drawn at its full size, with every thing in it
// showing; if they would not fit the screen's height it is wider instead.
func (d *designState) drawerShown(o *dOct) bool {
	return o.Kind == kDrawer && o.DrawerOpen && d.sel == o.ID
}

// drawerColsNow is how many miniatures go side by side in a drawer's tray
// as it is drawn now.
func (d *designState) drawerColsNow(o *dOct) int {
	cols := drawerCols(o)
	if !d.drawerShown(o) || d.maxH <= 0 {
		return cols
	}
	n := len(drawerThings(o))
	rows := max(1, (d.maxH-5)/miniH) // the tray's rows that fit the screen
	if (n+cols-1)/cols > rows {
		cols = (n + rows - 1) / rows
	}
	return cols
}

// drawerShownSize is a shown drawer's size on the screen: all of it.
func (d *designState) drawerShownSize(o *dOct) (int, int) {
	cols := d.drawerColsNow(o)
	return max(drawerW(o), 2+1+cols*(miniW+1)), 3 + 1 + drawerTrayH(o, cols) + 1
}

// drawerLines is how many screen rows a drawer takes: its front (frame,
// name, handle), and, pulled out, its tray.
func drawerLines(o *dOct) int {
	if !o.DrawerOpen {
		return 4
	}
	return 3 + 1 + drawerTrayH(o, drawerCols(o)) + 1
}

// drawerHit is what a click does on line li of a drawer (0: its top edge),
// col cells into its inside.
func drawerHit(o *dOct, li, col, cols int) string {
	if li == 2 {
		return "pull" // the handle's row
	}
	if !o.DrawerOpen || li < 4 || li >= 4+drawerTrayH(o, cols) {
		return ""
	}
	c := col - 1
	if c < 0 || c%(miniW+1) >= miniW {
		return ""
	}
	mc, mr := c/(miniW+1), (li-4)/miniH
	if mc >= cols {
		return ""
	}
	things := drawerThings(o)
	if k := mr*cols + mc; k < len(things) {
		return "item:" + itoa(things[k])
	}
	return ""
}

// drawerLayers draws a drawer: its front with the handle, and, pulled out,
// its tray of miniatures.
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
	if w < 8 || h < 4 {
		pill := mixHex(cBG, o.Color, 0.55)
		return []*lipgloss.Layer{pillLayer(x, y, w, h, ansi.Truncate(kindIcons[o.Kind]+o.Header, w, ""), pill)}
	}
	iw := w - 2
	e := st(edge, fill)
	front := mixHex(o.Color, fill, 0.5) // the front's wood
	name := o.Header
	if n := len(drawerThings(o)); n > 0 {
		name = strings.TrimSpace(name + " · " + itoa(n))
	}
	head := st(mixHex(o.Color, cText, 0.4), front).Bold(true)
	title := centreLine(head.Render(ansi.Truncate(kindIcons[o.Kind]+" "+name, iw-1, "…")), iw, front)
	switch {
	case selected && d.editing == partHeader:
		title = centreLine(head.Render(ansi.TruncateLeft(kindIcons[o.Kind]+" "+o.Header, max(0, lipgloss.Width(o.Header)+2-(iw-2)), "…"))+
			st(cAccent, front).Render("▏"), iw, front)
	case o.Header == "":
		title = centreLine(st(mixHex(cDim, front, 0.25), front).Italic(true).Render(ansi.Truncate(kindIcons[o.Kind]+" drawer", iw, "")), iw, front)
	}
	handle := "╺━━○━━╸"
	hs := st(mixHex(o.Color, cText, 0.65), front).Bold(true)
	lines := []string{
		e.Render("┌" + strings.Repeat("─", iw) + "┐"),
		e.Render("│") + fitBlockLine(title, iw, front) + e.Render("│"),
		e.Render("│") + fitBlockLine(centreLine(hs.Render(ansi.Truncate(handle, iw, "")), iw, front), iw, front) + e.Render("│"),
	}
	if o.DrawerOpen {
		lines = append(lines, e.Render("├"+strings.Repeat("─", iw)+"┤"))
		things := drawerThings(o)
		if len(things) == 0 {
			lines = append(lines, e.Render("│")+fitBlockLine(centreLine(st(mixHex(cDim, fill, 0.15), fill).Italic(true).Render(ansi.Truncate("drop anything here", iw, "…")), iw, fill), iw, fill)+e.Render("│"))
		}
		cols := d.drawerColsNow(o)
		rows := (len(things) + cols - 1) / cols
		hidden := 0 // things in rows that do not fit: only whole rows are drawn
		if 4+rows*miniH+1 > h && h >= 5 {
			fit := max(0, (h-6)/miniH)
			hidden = len(things) - fit*cols
			rows = fit
		}
		for r := 0; r < rows; r++ {
			minis := make([][]string, 0, cols)
			for c := 0; c < cols && r*cols+c < len(things); c++ {
				minis = append(minis, miniLines(miniOf(o.Items[things[r*cols+c]], false), fill))
			}
			for my := range miniH {
				var sb strings.Builder
				sb.WriteString(st(cText, fill).Render(" "))
				for _, mini := range minis {
					sb.WriteString(mini[my])
					sb.WriteString(st(cText, fill).Render(" "))
				}
				lines = append(lines, e.Render("│")+fitBlockLine(sb.String(), iw, fill)+e.Render("│"))
			}
		}
		if hidden > 0 {
			for len(lines) < h-2 {
				lines = append(lines, e.Render("│")+st(cText, fill).Render(strings.Repeat(" ", iw))+e.Render("│"))
			}
			lines = append(lines, e.Render("│")+fitBlockLine(centreLine(st(mixHex(cDim, cText, 0.4), fill).Italic(true).Render(ansi.Truncate("+"+itoa(hidden)+" more", iw, "…")), iw, fill), iw, fill)+e.Render("│"))
		}
	}
	lines = append(lines, e.Render("└"+strings.Repeat("─", iw)+"┘"))
	lines = moreLines(lines, h, func(n int) string { // the rows that do not fit
		return e.Render("│") + fitBlockLine(centreLine(st(mixHex(cDim, cText, 0.4), fill).Italic(true).Render(ansi.Truncate("+"+itoa(n)+" more", iw, "…")), iw, fill), iw, fill) + e.Render("│")
	}, e.Render("│")+st(cText, map[bool]string{true: fill, false: front}[o.DrawerOpen]).Render(strings.Repeat(" ", iw))+e.Render("│"))
	layers := []*lipgloss.Layer{lipgloss.NewLayer(strings.Join(lines, "\n")).X(x).Y(y)}
	return d.octExtras(layers, o, x, y, w, h, edge, fill, false, d.linkMode && d.linkFrom == o.ID)
}

// designPull pulls a drawer out, or pushes it in.
func (m *Model) designPull(o *dOct) {
	d := m.ds()
	o.DrawerOpen = !o.DrawerOpen
	d.dirty = true
	if o.DrawerOpen {
		d.sel = o.ID // chosen, it shows all it holds
		d.toFront(o)
		// The view moves so all of it shows, if it does not already.
		if x, y, w, h := d.rect(o); x < 0 || y < 0 || x+w > m.width || y+h > m.boardH() {
			m.centreOn(o)
		}
		m.say(octName(o) + " pulled out: click a thing to take it out · the handle pushes it in.")
	} else {
		m.say(octName(o) + " pushed in.")
	}
}
