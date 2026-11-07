package main

// ---- the board by keyboard ---------------------------------------------------------
//
// The arrows move the view (Shift: further) and choose the structure nearest
// the middle of the screen as they go; E writes in the chosen one; Alt + the
// arrows move it; + and - zoom (the keypad's too).

import "strings"

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
	o := d.middleStructure(m.width, m.textHeight())
	if o == nil {
		if d.sel >= 0 {
			d.sel, d.editing = -1, partNone
		}
		return
	}
	if o.ID != d.sel {
		d.sel, d.editing, d.selLink = o.ID, partNone, -1
		m.say("Chosen: " + octName(o) + " · E writes in it · Alt + arrows move it · Delete deletes it")
	}
}

// designEdit starts writing in the chosen structure: its text (a page,
// note, book, slide or drawing), or its header (an octagon, a cabinet).
func (m *Model) designEdit() {
	d := m.ds()
	o := d.byID(d.sel)
	if o == nil {
		m.say("Nothing chosen: the arrows choose the structure in the middle of the screen, or click one.")
		return
	}
	d.editing = partHeader
	if hasPart(o, partBody) && o.Kind != kOct {
		d.editing = partBody
	}
	m.say("Writing in " + octName(o) + ": Tab goes to the next part · Esc stops.")
}
