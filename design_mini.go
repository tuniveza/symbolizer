package main

// ---- miniatures --------------------------------------------------------------------------
//
// In a drawer, and pinned on a board, a structure shows small: miniW × miniH
// cells, in its own outline (an octagon's cut corners, a sheet's folded
// corner, a note's round ones, a container's frame) and colour, with its
// icon and one label, its title.

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

const miniW, miniH = 12, 4

// miniCell is one cell of a miniature: its letter and colours (bg "": the
// cell is clear, what is behind shows).
type miniCell struct {
	r      string
	fg, bg string
}

// miniOf draws o as a miniature; sel picks it out.
func miniOf(o *dOct, sel bool) [][]miniCell { return miniOfSize(o, miniW, miniH, sel, true) }

// miniColours are a miniature's fill and edge colours.
func miniColours(o *dOct, sel bool) (fill, edge string) {
	fill = mixHex(cBG, o.Color, 0.22)
	if textual(o.Kind) && o.Kind != kNote {
		fill = mixHex(cBG, cText, 0.09) // a sheet is paper
	}
	edge = mixHex(o.Color, cText, 0.15)
	if sel {
		edge = cAccent
	}
	return fill, edge
}

// miniOfSize draws o as a miniature w × h cells big: its outline at its
// edges, and (with words) its icon and one label once, in the middle.
func miniOfSize(o *dOct, w, h int, sel, words bool) [][]miniCell {
	miniW, miniH := w, h
	fill, edge := miniColours(o, sel)
	g := make([][]miniCell, miniH)
	for y := range g {
		g[y] = make([]miniCell, miniW)
		for x := range g[y] {
			g[y][x] = miniCell{" ", cText, fill}
		}
	}
	set := func(x, y int, r string, fg, bg string) {
		if x >= 0 && y >= 0 && x < miniW && y < miniH {
			g[y][x] = miniCell{r, fg, bg}
		}
	}
	// The outline: corners, then the edges between them.
	tl, tr, bl, br, hz, vt := "┌", "┐", "└", "┘", "─", "│"
	switch o.Kind {
	case kOct:
		tl, tr, bl, br = "╱", "╲", "╲", "╱"
	case kNote:
		tl, tr, bl, br = "╭", "╮", "╰", "╯"
	case kCabinet:
		tl, tr, bl, br, hz, vt = "┏", "┓", "┗", "┛", "━", "┃"
	case kDrawer, kRack, kBoard:
		tl, tr, bl, br, hz, vt = "╔", "╗", "╚", "╝", "═", "║"
	default:
		tr = "◹" // a sheet's folded corner
	}
	for x := 1; x < miniW-1; x++ {
		set(x, 0, hz, edge, fill)
		set(x, miniH-1, hz, edge, fill)
	}
	for y := 1; y < miniH-1; y++ {
		set(0, y, vt, edge, fill)
		set(miniW-1, y, vt, edge, fill)
	}
	cornerBG := fill
	if o.Kind == kOct {
		cornerBG = "" // the cut corners are clear
	}
	set(0, 0, tl, edge, cornerBG)
	set(miniW-1, 0, tr, edge, cornerBG)
	set(0, miniH-1, bl, edge, cornerBG)
	set(miniW-1, miniH-1, br, edge, cornerBG)
	if h < 3 { // too low for an outline: just its colour
		for y := range g {
			for x := range g[y] {
				g[y][x] = miniCell{" ", cText, fill}
			}
		}
	}
	if !words {
		return g
	}
	// Its icon, and its one label.
	icon := kindIcons[o.Kind]
	iconRow, labelRow := miniH/2-1, miniH/2
	if miniH < 4 {
		iconRow = -1
		labelRow = miniH / 2
	}
	label := []rune(ansi.Truncate(itemTitle(o), max(miniW-2, 1), "…"))
	if iconRow < 0 {
		label = []rune(ansi.Truncate(icon+" "+itemTitle(o), max(miniW-2, 1), "…"))
	} else {
		set(miniW/2-1, iconRow, icon, mixHex(o.Color, cText, 0.3), fill)
	}
	start := max(1, (miniW-len(label))/2)
	for i, r := range label {
		set(start+i, labelRow, string(r), cText, fill)
	}
	return g
}

// miniLines is a miniature as styled lines, miniW wide each; a clear cell
// takes the colour bg.
func miniLines(g [][]miniCell, bg string) []string {
	out := make([]string, len(g))
	for y, row := range g {
		var sb strings.Builder
		for _, c := range row {
			b := c.bg
			if b == "" {
				b = bg
			}
			sb.WriteString(st(c.fg, b).Render(c.r))
		}
		out[y] = sb.String()
	}
	return out
}
