package main

// ---- containers in a box: folded until chosen ------------------------------------------
//
// A box can hold containers too: a cabinet, a drawer unit, a rack, a board
// or another box. In the box such a thing lies folded: a small tile, lower
// than a miniature, with a ▸, its icon and name, and under them the icons
// of what it holds. Chosen (a click on it, Tab, or put in just now) it
// opens out where it lies: a card in its own outline, its name on its top
// edge, and a row for each thing it holds (the rest as "+N more"). It is
// lifted over the other things, full size and bright, kept inside the box's
// walls, until it is no longer chosen (a click elsewhere, Esc, another
// thing chosen); then it folds up again.
//
// Open or folded is only looking: it is not saved and not an undo step
// (it is just whether the thing is the box's chosen one, d.boxPick).
// Drawing, clicking, the 3D slabs, dropping and Alt+arrows all take a
// thing's size from boxThingSize, so what is clicked is what is drawn.

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// boxTileH is how many board rows a folded container takes in a box;
// boxOpenW and boxOpenRows are how wide an open one is, and how many of
// its things it lists at most.
const (
	boxTileH    = 2
	boxOpenW    = miniW + 8
	boxOpenRows = 6
)

// boxInner is the size of box o's inside, in board cells.
func boxInner(o *dOct) (int, int) {
	bw, bh := boxBoardSize(o)
	sb := boxShapeOf(boxDesign(o), bw, bh)
	return max(1, bw-sb.left-sb.right), max(1, bh-sb.top-sb.bottom)
}

// boxThingSize is how big thing it lies in box o, in board cells: a
// miniature; a container folded (a tile), or open (a card listing what it
// holds, no bigger than the box's inside).
func boxThingSize(o, it *dOct, open bool) (int, int) {
	if !isContainer(it.Kind) {
		return miniW, miniH
	}
	if !open {
		return miniW, boxTileH
	}
	inW, inH := boxInner(o)
	h := 2 + min(max(len(drawerThings(it)), 1), boxOpenRows)
	return max(1, min(boxOpenW, inW)), max(1, min(max(h, miniH), inH))
}

// boxThingOpen is whether thing it of box o is drawn open: a container
// that is the box's chosen thing.
func (d *designState) boxThingOpen(o, it *dOct) bool {
	if it == nil || !isContainer(it.Kind) {
		return false
	}
	b, c := d.chosenBoxThing()
	return b == o && c == it
}

// boxLifted is whether thing it of box o is drawn over all the layers:
// open, and not on a layer made see-through (looking at a layer below).
func (d *designState) boxLifted(o, it *dOct) bool {
	view := d.boxView(o) - 1
	return d.boxThingOpen(o, it) && !(view >= 0 && boxLayerOf(it) < view)
}

// boxRoomOf is how far thing it's place can go in box o (board cells from
// its inside corner) for it to lie inside, open or not.
func boxRoomOf(o, it *dOct, open bool) (int, int) {
	inW, inH := boxInner(o)
	tw, th := boxThingSize(o, it, open)
	return max(0, inW-tw), max(0, inH-th)
}

// boxClampOpen keeps thing it inside box o, sized as it is drawn now.
func (d *designState) boxClampOpen(o, it *dOct) {
	mx, my := boxRoomOf(o, it, d.boxThingOpen(o, it))
	it.X, it.Y = min(max(it.X, 0), mx), min(max(it.Y, 0), my)
}

// boxThingCells is thing it of box o drawn tw × th: its miniature, or, a
// container, its folded tile or its open card.
func boxThingCells(it *dOct, tw, th int, sel, open bool) [][]miniCell {
	switch {
	case !isContainer(it.Kind):
		return miniOfSize(it, tw, th, sel, true)
	case open:
		return boxOpenCard(it, tw, th, sel)
	}
	return boxTile(it, tw, th, sel)
}

// boxHeldIcons are the icons of what container it holds, as many as fit in
// room cells ("…" for the rest), or "empty".
func boxHeldIcons(it *dOct, room int) string {
	idx := drawerThings(it)
	if len(idx) == 0 {
		return "empty"
	}
	var sb strings.Builder
	n := 0
	for k, j := range idx {
		ic := kindIcons[it.Items[j].Kind]
		need := 2
		if k < len(idx)-1 {
			need = 4 // its icon, a space, and room for "…" after
		}
		if n+need > room {
			sb.WriteString("…")
			break
		}
		sb.WriteString(ic + " ")
		n += 2
	}
	return strings.TrimRight(sb.String(), " ")
}

// boxTile is a folded container, tw × th: its colour, a ▸ (a click opens
// it), its icon and name, and under them the icons of what it holds.
func boxTile(it *dOct, tw, th int, sel bool) [][]miniCell {
	fill, edge := miniColours(it, sel)
	g := make([][]miniCell, th)
	for y := range g {
		g[y] = make([]miniCell, tw)
		for x := range g[y] {
			g[y][x] = miniCell{" ", cText, fill}
		}
	}
	put := func(x, y int, s, fg string) {
		for _, r := range s {
			if x >= tw || y < 0 || y >= th {
				return
			}
			if x >= 0 {
				g[y][x] = miniCell{string(r), fg, fill}
			}
			x++
		}
	}
	top := max(0, (th-2)/2)
	put(0, top, "▸", edge)
	put(1, top, ansi.Truncate(kindIcons[it.Kind]+" "+itemTitle(it), tw-1, "…"), cText)
	if th >= 2 {
		put(2, top+1, boxHeldIcons(it, tw-2), mixHex(cText, fill, 0.4))
	}
	return g
}

// boxOpenCard is an open container, tw × th: its own outline, a ▾, its
// icon and name on its top edge, and a row for each thing it holds.
func boxOpenCard(it *dOct, tw, th int, sel bool) [][]miniCell {
	g := miniOfSize(it, tw, th, sel, false)
	fill, edge := miniColours(it, sel)
	put := func(x, y, end int, s, fg string) {
		for _, r := range s {
			if x > end || x >= tw || y < 0 || y >= th {
				return
			}
			if x >= 0 {
				g[y][x] = miniCell{string(r), fg, fill}
			}
			x++
		}
	}
	title := ansi.Truncate(" ▾ "+kindIcons[it.Kind]+" "+itemTitle(it)+" ", max(tw-2, 1), "…")
	put(max(1, (tw-ansi.StringWidth(title))/2), 0, tw-2, title, edge)
	rows := th - 2
	if rows < 1 {
		return g
	}
	idx := drawerThings(it)
	if len(idx) == 0 {
		put(max(1, (tw-5)/2), 1+(rows-1)/2, tw-2, "empty", mixHex(cText, fill, 0.45))
		return g
	}
	show := idx
	if len(idx) > rows {
		show = idx[:rows-1]
	}
	for k, j := range show {
		t := it.Items[j]
		put(2, 1+k, tw-3, ansi.Truncate(kindIcons[t.Kind]+" "+itemTitle(t), max(tw-4, 1), "…"), cText)
	}
	if len(show) < len(idx) {
		put(2, th-2, tw-3, "+"+itoa(len(idx)-len(show))+" more", mixHex(cText, fill, 0.4))
	}
	return g
}
