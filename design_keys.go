package main

// ---- the board by keyboard ---------------------------------------------------------
//
// The arrows move the view (Shift: further) and choose the structure nearest
// the middle of the screen as they go; Alt + the
// arrows move it; + and - zoom (the keypad's too).

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// arrowStep is the direction of an arrow key name, and how far: Shift goes four times as far.
func arrowStep(name string) (dx, dy, step int) {
	step = 1
	if strings.HasPrefix(name, "shift+") {
		step = 4
	}
	v := map[string][2]int{"left": {-1, 0}, "right": {1, 0}, "up": {0, -1}, "down": {0, 1}}[strings.TrimPrefix(name, "shift+")]
	return v[0], v[1], step
}

// middleStructure is the structure nearest the middle of a w×h screen (a
// cell's height counts as two widths, as on screen), or nil when none is on it.
func (d *designState) middleStructure(w, h int) *dOct {
	cx, cy := w/2, h/2
	var best *dOct
	bestD := -1
	for _, o := range d.octs { // later ones are on top: they win a tie
		if o == nil || !d.onScreen(o, w, h) {
			continue
		}
		x, y, ow, oh := d.rect(o)
		dx := max(x-cx, 0, cx-(x+ow-1))
		dy := max(y-cy, 0, cy-(y+oh-1)) * 2
		if dist := dx*dx + dy*dy; bestD < 0 || dist <= bestD {
			best, bestD = o, dist
		}
	}
	return best
}

// designChooseMiddle chooses the structure nearest the middle of the screen.
func (m *Model) designChooseMiddle() {
	d := m.ds()
	o := d.middleStructure(m.width, m.boardH())
	if o == nil {
		if d.sel >= 0 {
			d.sel, d.editing = -1, partNone
		}
		return
	}
	if o.ID != d.sel {
		d.sel, d.editing, d.selLink = o.ID, partNone, -1
		m.msg = "" // the top bar names it; the status bar says what keys do
	}
}

// designEdit starts writing in the chosen structure: its text (a page,
// note, book, slide or drawing), or its header (an octagon, a cabinet).
func (m *Model) designEdit() {
	d := m.ds()
	o := d.byID(d.sel)
	if o == nil {
		m.say("Nothing chosen. Use the arrows, or click one.")
		return
	}
	d.editing = partHeader
	if hasPart(o, partBody) && o.Kind != kOct {
		d.editing = partBody
	}
	m.say("Writing in " + octName(o) + ": Tab goes to the next part · Esc stops.")
}

// ---- diagonals ---------------------------------------------------------------------
//
// Two arrows held together (Up and Right, say) move the view, or with Alt the
// chosen structure, diagonally. kitty says when a key is let go, so sy knows
// which arrows are held; in other terminals two arrows pressed within
// chordWindow of each other count as held together.

// arrowDiagonal adds to (dx, dy), the step of arrow dir, the arrow held
// with it across, if one is.
func (m *Model) arrowDiagonal(dir string, dx, dy int) (int, int) {
	d := m.ds()
	now := time.Now()
	partner := ""
	if m.releases {
		for a, down := range m.chord.held {
			if down && a != dir && vertical(a) != vertical(dir) {
				partner = a
			}
		}
		m.chord.held[dir] = true
	} else if d.lastArrow != "" && d.lastArrow != dir && vertical(d.lastArrow) != vertical(dir) &&
		now.Sub(d.lastArrowAt) < chordWindow {
		partner = d.lastArrow
	}
	d.lastArrow, d.lastArrowAt = dir, now
	if partner != "" {
		px, py, _ := arrowStep(partner)
		dx, dy = dx+px, dy+py
	}
	return dx, dy
}

// arrowDir is the bare direction of an arrow key name: "alt+shift+up" is "up".
func arrowDir(name string) string {
	return name[strings.LastIndexByte(name, '+')+1:]
}

// ---- Tab: from part to part ---------------------------------------------------------

// designTab, with a structure chosen and nothing being written, starts
// writing in its first part (Shift+Tab: its last); in a cabinet it opens the
// next stack. While writing, Tab goes on to the next part (designWrite).
func (m *Model) designTab(back bool) {
	d := m.ds()
	o := d.byID(d.sel)
	if o == nil {
		m.say("Nothing chosen. Use the arrows, or click one.")
		return
	}
	if o.Kind == kCabinet {
		i, step := -1, 1
		if back {
			step = -1
		}
		for j, k := range stackKinds {
			if k == o.OpenStack {
				i = j
			}
		}
		if i < 0 && back {
			i = 0
		}
		o.OpenStack = stackKinds[(i+step+len(stackKinds))%len(stackKinds)]
		d.dirty = true
		m.viewToPart(o, partHeader)
		m.say(stackNames[o.OpenStack] + " pulled out.")
		return
	}
	ps := partsOf(o)
	d.editing = ps[0]
	if back {
		d.editing = ps[len(ps)-1]
	}
	m.viewToPart(o, d.editing)
	m.msg = ""
}

// partRows is where part p of o is, in board rows from o's top: first and
// last row. A cabinet is all one part.
func (d *designState) partRows(o *dOct, p int) (int, int) {
	_, h := d.size(o)
	switch {
	case o.Kind == kCabinet:
		return 0, h - 1
	case p == partHeader:
		return 0, min(1, h-1)
	case p == partFooter && o.Kind == kOct && h >= octMinH:
		return h - 2, h - 1
	case p == partBody && o.Kind == kOct && h >= octMinH:
		return min(2, h-1), max(2, h-3)
	case p == partFooter && o.Kind != kOct:
		return max(0, h-3), h - 1
	case p == partBody && o.Kind != kOct:
		return min(2, h-1), max(2, h-4)
	case p == partBody:
		return min(2, h-1), h - 1
	}
	return 0, h - 1
}

// viewToPart moves the view to part p of o (Tab picks a part): the
// structure in the middle across, the part in the middle up and down.
// A part taller than the screen shows from its top.
func (m *Model) viewToPart(o *dOct, p int) {
	d := m.ds()
	if o == nil {
		return
	}
	z := d.z()
	w, _ := d.size(o)
	top, bottom := d.partRows(o, p)
	screenW, screenH := float64(m.width)/z, float64(m.boardH())/z
	cx := float64(o.X) + float64(w)/2
	cy := float64(o.Y) + float64(top+bottom+1)/2
	d.camX = cx - screenW/2
	if float64(bottom-top+1) > screenH-2 {
		d.camY = float64(o.Y+top) - 1
	} else {
		d.camY = cy - screenH/2
	}
	d.dirty = true
}

// ---- fitting a structure to what it holds -------------------------------------------
//
// While you write, the structure grows (wider up to fitMaxW, then taller) so
// all of it shows, and the view follows; F fits the chosen one exactly,
// smaller too. Pictures keep their shape. Notes are one page and cabinets
// size themselves, so they are left as they are.

const (
	fitMaxW     = 60 // columns a structure grows to before its text wraps
	fitMaxPageH = 40 // rows a page grows to; beyond, it scrolls
	fitMaxOctH  = 60
)

// fitSize is the size (in board cells) that shows all of o, at most maxH
// rows high (the screen's height: more and it scrolls, or is cut).
func (d *designState) fitSize(o *dOct, maxH int) (int, int, bool) {
	widest := func(s string) int {
		n := 0
		for _, l := range strings.Split(s, "\n") {
			n = max(n, lipgloss.Width(l))
		}
		return n
	}
	lines := func(s string, w int) int {
		if s == "" || w <= 0 {
			return 0
		}
		return len(strings.Split(ansi.Wrap(s, w, ""), "\n"))
	}
	switch o.Kind {
	case kNote, kCabinet:
		return o.W, o.H, false
	case kOct:
		w := max(octMinW, min(fitMaxW, max(widest(o.Body), lipgloss.Width(o.Header)+2, lipgloss.Width(o.Footer)+2)+6))
		cw := w - 4
		rows := lines(o.Body, cw)
		pic := func(pw int) int { // rows that show the picture pw cells wide, keeping its shape
			if o.Image == "" {
				return max(3, pw/3)
			}
			if img, ok := loadPicture(filepath.Join(d.imagesDir(), o.Image)); ok {
				b := img.Bounds()
				if b.Dx() > 0 {
					return max(2, (pw*b.Dy()+b.Dx())/(2*b.Dx()))
				}
			}
			return max(3, pw/3)
		}
		switch o.Layout % len(octLayouts) {
		case layoutPicture:
			w, cw = max(w, 34), max(w, 34)-4
			rows = pic(cw)
		case layoutAbove:
			w, cw = max(w, 34), max(w, 34)-4
			rows = 2 * max(pic(cw), rows) // the picture takes half the rows
		case layoutLeft, layoutRight:
			w = max(w, 40)
			cw = w - 4
			iw := cw / 2
			rows = max(pic(iw), lines(o.Body, cw-iw-1))
		case layoutColumns:
			rows = (lines(o.Body, (cw-3)/2) + 1) / 2
		}
		return w, max(octMinH, min(fitMaxOctH, maxH, rows+6)), true
	}
	// Pages and the other sheets.
	mw, mh := minSize(o.Kind)
	w := max(mw, min(fitMaxW+10, max(widest(o.Body), lipgloss.Width(o.Header)+2, lipgloss.Width(o.Footer)+2)+4))
	return w, max(mh, min(fitMaxPageH, maxH, lines(o.Body, w-4)+6)), true
}

// boardRows is how many board rows the screen shows at this zoom.
func (m *Model) boardRows() int {
	return max(4, int(float64(m.boardH())/m.ds().z())-1)
}

// growToFit makes o bigger, never smaller, until all of it shows (or it
// is as high as the screen).
func (m *Model) growToFit(o *dOct) bool {
	d := m.ds()
	w, h, ok := d.fitSize(o, m.boardRows())
	if !ok || (w <= o.W && h <= o.H) {
		return false
	}
	o.W, o.H = max(o.W, w), max(o.H, h)
	d.dirty = true
	return true
}

// designFit fits the chosen structure to what it holds (F).
func (m *Model) designFit() {
	d := m.ds()
	o := d.byID(d.sel)
	if o == nil {
		m.say("Nothing chosen. Use the arrows, or click one.")
		return
	}
	w, h, ok := d.fitSize(o, m.boardRows())
	if !ok {
		m.say(octName(o) + " sizes itself.")
		return
	}
	o.W, o.H = w, h
	d.dirty = true
	m.keepInView(o, false)
	m.say(fmt.Sprintf("%s fitted to what it holds: %d × %d.", octName(o), w, h))
}

// keepInView moves the view the least it can so o is on the screen: all
// of it when it fits, else its top left (its bottom when bottom is true:
// where the typing is).
func (m *Model) keepInView(o *dOct, bottom bool) {
	d := m.ds()
	x, y, w, h := d.rect(o)
	sw, sh := m.width, m.boardH()
	z := d.z()
	dx, dy := 0, 0
	switch {
	case x < 0 || w > sw:
		dx = x
	case x+w > sw:
		dx = x + w - sw
	}
	switch {
	case h > sh && bottom:
		dy = y + h - sh
	case y < 0 || h > sh:
		dy = y
	case y+h > sh:
		dy = y + h - sh
	}
	if dx != 0 || dy != 0 {
		d.camX += float64(dx) / z
		d.camY += float64(dy) / z
		d.dirty = true
	}
}

// ---- gliding: an arrow held -----------------------------------------------------------
//
// A press moves one step at once, and while the arrow stays down the view
// (or, with Alt, the chosen structure) glides straight on, a step every
// holdEvery, until it is let go: no waiting for the keyboard's own pause
// before it repeats. A quick tap is one step (the glide's first step comes
// only if the arrow is still down a frame later). kitty says when keys are
// let go; other terminals repeat the key themselves, after their pause.

const (
	holdStart = 33 * time.Millisecond // the glide's first step, if the arrow is still down
	holdEvery = 33 * time.Millisecond // then 30 steps a second
	holdAlt   = 45 * time.Millisecond // Alt + arrows (moving a structure): about 22 a second, a little slower
	holdLost  = time.Second           // no word of the arrow for this long: let go
)

type designHoldMsg struct{}

// startHold starts the glide for arrow key name, when it is not going yet.
func (m *Model) startHold(name string) tea.Cmd {
	d := m.ds()
	if !m.releases {
		return nil
	}
	d.holdMods = name[:strings.LastIndexByte(name, '+')+1]
	d.heldAt = time.Now()
	if strings.HasPrefix(d.holdMods, "alt+") {
		d.holding = true
	}
	if d.holdTick {
		return nil
	}
	d.holdTick = true
	return tea.Tick(holdStart, func(time.Time) tea.Msg { return designHoldMsg{} })
}

// designHold takes one glide step while arrows are held, or ends it.
func (m *Model) designHold() tea.Cmd {
	d := m.design
	if d == nil {
		return nil
	}
	var dx, dy int
	for dir, down := range m.chord.held {
		if down {
			x, y, _ := arrowStep(dir)
			dx, dy = dx+x, dy+y
		}
	}
	if !m.inDesign() || m.help || (dx == 0 && dy == 0) || time.Since(d.heldAt) > holdLost {
		d.holdTick = false
		for dir := range m.chord.held {
			m.chord.held[dir] = false
		}
		if d.holding { // the glided structure: one undo step
			d.holding = false
			d.dirty = true
			d.save()
		}
		return nil
	}
	step := 1
	if strings.Contains(d.holdMods, "shift+") {
		step = 4
	}
	if strings.HasPrefix(d.holdMods, "alt+") {
		if o := d.byID(d.sel); o != nil {
			o.X += dx * step
			o.Y += dy * step
		}
	} else {
		z := d.z()
		d.camX += float64(dx*step*4) / z
		d.camY += float64(dy*step*2) / z
		m.designChooseMiddle()
	}
	d.dirty = true
	d.save()
	every := holdEvery
	if strings.HasPrefix(d.holdMods, "alt+") {
		every = holdAlt
	}
	return tea.Tick(every, func(time.Time) tea.Msg { return designHoldMsg{} })
}
