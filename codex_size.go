package main

// Codex sizes: every thing on a page can be made bigger or smaller, with
// + and − or by dragging its corner with the mouse, and the cover can have
// any of the page borders as its frame (the user asked, 19 Nov 2026).
//
// A text or a symbol grows by its letter size (Size). A picture, a drawing
// and a button get their own width (W: a share of their place, the slot
// inside its border) and height (H: a picture's or drawing's share of its
// place; a button's rows). 0 is "as it comes".

import (
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// codexCoverNone is the CoverBorder with no frame at all.
const codexCoverNone = len(codexPageBorders) + 1

// codexCoverBorderName says a cover frame.
func codexCoverBorderName(i int) string {
	switch {
	case i <= 0:
		return "gilt frame"
	case i >= codexCoverNone:
		return "none"
	}
	return codexPageBorders[i-1].name
}

// codexSized is where thing e goes across its slot (x, w): its own width
// (W) lined up as it is, or the whole slot.
func codexSized(e *codexEl, x, w int) (int, int) {
	if e.W <= 0 || e.W >= 100 {
		return x, w
	}
	ew := min(max(w*e.W/100, 4), w)
	return x + codexAlign(e.Align, w, ew), ew
}

// codexSizeSays says thing e's size, for the status line.
func codexSizeSays(e *codexEl) string {
	switch e.Kind {
	case ceText:
		s := "Size " + itoa(max(e.Size, 1)) + " of " + itoa(codexSizes)
		if e.W > 0 {
			s += " · width " + itoa(e.W) + "%"
		}
		return s + "."
	case ceSymbol:
		return "Size " + itoa(max(e.Size, 1)) + " of " + itoa(codexSizes) + "."
	case ceButton:
		w := "as its label"
		if e.W > 0 {
			w = itoa(e.W) + "%"
		}
		return "Width " + w + " · height " + itoa(max(e.H, 3)) + " rows."
	}
	w, h := "whole place", "what is left"
	if e.W > 0 {
		w = itoa(e.W) + "%"
	}
	if e.H > 0 {
		h = itoa(e.H) + "%"
	}
	return "Width " + w + " · height " + h + "."
}

// codexPct is n as a share of of, in tens (10 … 100).
func codexPct(n, of int) int {
	if of <= 0 {
		return 100
	}
	return min(max((n*100/of+5)/10*10, 10), 100)
}

// codexSizeKey: + (grow) or − on the chosen thing.
func (m *Model) codexSizeKey(e *codexEl, grow bool) {
	c := m.cx()
	pl := c.place[e.ID]
	r := c.rects[e.ID]
	step := 1
	if !grow {
		step = -1
	}
	switch e.Kind {
	case ceText, ceSymbol:
		n := min(max(max(e.Size, 1)+step, 1), codexSizes)
		if n == max(e.Size, 1) {
			m.say(map[bool]string{true: "The biggest size.", false: "The smallest size."}[grow])
			return
		}
		m.codexStep()
		e.Size = n
	case ceImage, ceDrawing:
		w, h := e.W, e.H
		if w == 0 {
			w = 100
		}
		if h == 0 {
			h = codexPct(r[3], pl[3])
		}
		nw, nh := min(max(w+10*step, 20), 100), min(max(h+10*step, 20), 100)
		if nw == w && nh == h && e.H != 0 {
			m.say(map[bool]string{true: "As big as its place.", false: "As small as it goes."}[grow])
			return
		}
		m.codexStep()
		e.W, e.H = nw, nh
		if e.W == 100 {
			e.W = 0
		}
	case ceButton:
		w := e.W
		if w == 0 {
			w = codexPct(r[2], pl[2])
		}
		nw := min(max(w+10*step, 10), 100)
		nh := min(max(max(e.H, 3)+2*step, 3), max(pl[3], 3))
		if nw == w && nh == max(e.H, 3) && (e.W != 0 || !grow) {
			m.say(map[bool]string{true: "As big as its place.", false: "As small as it goes."}[grow])
			return
		}
		m.codexStep()
		e.W, e.H = nw, nh
		if !grow && pl[2]*nw/100 < r[2] && e.W != 0 && pl[2]*nw/100 < 8 {
			e.W = 0 // smaller than its label wants: back to its own width
		}
		if e.H == 3 {
			e.H = 0
		}
	}
	m.codexSave()
	m.say(codexSizeSays(e) + " + bigger · − smaller · or drag its corner ◢.")
}

// ---- the mouse: dragging a corner -----------------------------------------------------------

// codexSizing is a drag of a thing's corner or edge.
type codexSizing struct {
	id       int
	mx, my   int    // where the drag began
	r, place [4]int // the thing's box and place then
	w, h     bool   // it changes the width / the height
	size     int    // its letter size then
	bw, bh   int    // a picture's or drawing's box then
}

// codexHandle is where the chosen thing's corner handle is drawn.
func codexHandle(r [4]int) (int, int) { return r[0] + r[2], r[1] + r[3] }

// codexSizeStart: a press at (x, y) in edit mode. On the chosen thing's
// corner handle (or just outside its right or bottom side) it starts a drag.
func (m *Model) codexSizeStart(x, y int) bool {
	c := m.cx()
	e := c.el(c.sel)
	if e == nil || !c.edit || c.table || c.drawing || c.writing != 0 {
		return false
	}
	r, ok := c.rects[e.ID]
	if !ok {
		return false
	}
	hx, hy := codexHandle(r)
	s := &codexSizing{id: e.ID, mx: x, my: y, r: r, place: c.place[e.ID], size: max(e.Size, 1)}
	switch {
	case (x == hx || x == hx-1) && (y == hy || y == hy-1) && !(x == hx-1 && y == hy-1):
		s.w, s.h = true, true // the corner
	case x == hx && y >= r[1] && y < hy:
		s.w = true // the right side
	case y == hy && x >= r[0] && x < hx:
		s.h = true // the bottom
	default:
		return false
	}
	s.bw, s.bh = s.place[2], r[3]
	if e.W > 0 {
		s.bw = s.place[2] * e.W / 100
	}
	if e.H > 0 && (e.Kind == ceImage || e.Kind == ceDrawing) {
		s.bh = s.place[3] * e.H / 100
	}
	m.codexStep() // one undo for the whole drag
	c.sizing = s
	m.say("Drag: right and down bigger, left and up smaller. Let go to keep it.")
	return true
}

// codexSizeDrag: the mouse moved with a corner held.
func (m *Model) codexSizeDrag(ms tea.Mouse) tea.Cmd {
	c := m.cx()
	s := c.sizing
	e := c.el(s.id)
	if e == nil {
		c.sizing = nil
		return nil
	}
	x, y := ms.X, ms.Y-m.boardTop()
	dx, dy := x-s.mx, y-s.my
	if !s.w {
		dx = 0
	}
	if !s.h {
		dy = 0
	}
	k := 1
	if e.Align == 0 { // in the middle: both sides move, so the edge follows the mouse
		k = 2
	}
	pw, ph := max(s.place[2], 1), max(s.place[3], 1)
	share := func(n, least int) int { // n cells as a share of the place's width
		v := min(max(n*100/pw, least), 100)
		if v >= 100 {
			return 0
		}
		return v
	}
	switch e.Kind {
	case ceImage, ceDrawing:
		if s.w {
			e.W = share(s.bw+k*dx, 10)
		}
		if s.h {
			e.H = min(max((s.bh+dy)*100/ph, 10), 100)
		}
	case ceButton:
		if s.w {
			e.W = share(s.r[2]+k*dx, 10)
			if e.W > 0 && pw*e.W/100 < 8 {
				e.W = 0
			}
		}
		if s.h {
			e.H = min(max(s.r[3]+dy, 3), max(ph, 3))
			if e.H == 3 {
				e.H = 0
			}
		}
	case ceText:
		if s.w && dx != 0 { // from the width its words wrap in, not where they end
			e.W = share(s.bw+k*dx, 20)
		}
		if s.h && dy != 0 {
			want := s.r[3] + dy
			l := c.look(e, time.Now())
			w := pw
			if e.W > 0 {
				w = pw * e.W / 100
			}
			e.Size = 1
			for n := codexSizes; n >= 1; n-- {
				l.size = n
				rows, got, _ := codexTextRows(l, w, 0)
				if got == n && len(rows) <= want {
					e.Size = n
					break
				}
			}
		}
	case ceSymbol:
		want := s.r[3] + dy
		if abs(dx) > abs(dy) { // sideways: by its width
			want = -1
		}
		sym := e.Symbol
		e.Size = 1
		for n := codexSizes; n >= 1; n-- {
			rows := codexSymbolBig(sym, 0, n)
			wd := 0
			for _, r := range rows {
				wd = max(wd, codexWidth(r))
			}
			if (want >= 0 && len(rows) <= want) || (want < 0 && wd <= s.r[2]+k*dx) {
				e.Size = n
				break
			}
		}
	}
	m.say(codexSizeSays(e))
	return nil
}

// codexSizeEnd: the mouse let go of the corner: the size is kept.
func (m *Model) codexSizeEnd() tea.Cmd {
	c := m.cx()
	c.sizing = nil
	m.codexSave()
	if e := c.el(c.sel); e != nil {
		m.say(codexSizeSays(e) + " Ctrl+Z undoes it.")
	}
	return m.codexTickCmd()
}

// codexDrawHandle draws the chosen thing's corner handle ◢, which the mouse drags.
func (m *Model) codexDrawHandle(cv *boardCanvas, r [4]int, col, paper string) {
	c := m.cx()
	x, y := codexHandle(r)
	if x < 0 || y < 0 || x >= cv.Width() || y >= cv.Height() || !c.insideInner(x, y) {
		return
	}
	cvSet(cv, x, y, "◢", col, cvBG(cv, x, y, paper), uv.AttrBold)
}
