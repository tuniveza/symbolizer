package main

// ---- the board by keyboard ---------------------------------------------------------
//
// The arrows move the view (Shift: further) and choose the structure nearest
// the middle of the screen as they go; Alt + the
// arrows move it; + and - zoom (the keypad's too).

import (
	"fmt"
	"math"
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
	d.editing = partHeader // a cabinet has only its name
	if hasPart(o, partBody) {
		d.editing = partBody // its text
	}
	m.viewToPart(o, d.editing)
	m.msg = ""
}

// ---- Space held: the arrows pan --------------------------------------------------------
//
// While Space is held the arrows move the view freely, a step at a time
// (held, it glides), like Shift + arrows but in smaller steps. kitty says
// when Space is let go; other terminals repeat it while it is held, so
// there it counts as held for spaceRepeat after it was last seen.

const spaceRepeat = 600 * time.Millisecond

// designSpaceMsg comes spaceRepeat after a Space pan, to see whether Space
// was let go (in terminals that do not say so).
type designSpaceMsg struct{}

func (m *Model) spaceCheck() tea.Cmd {
	return tea.Tick(spaceRepeat+50*time.Millisecond, func(time.Time) tea.Msg { return designSpaceMsg{} })
}

// designSpaceUp: Space was let go. If the view moved while it was held,
// it snaps to the structure nearest the middle of the screen, centred and
// chosen.
func (m *Model) designSpaceUp() {
	d := m.design
	if d == nil || !d.spacePanned || m.spaceDown() || !m.inDesign() {
		return
	}
	d.spacePanned = false
	if o := d.middleStructure(m.width, m.boardH()); o != nil {
		m.centreOn(o)
		d.sel, d.editing, d.selLink = o.ID, partNone, -1
	}
	d.dirty = true
}

// spaceDown is whether Space is held.
func (m *Model) spaceDown() bool {
	d := m.ds()
	if m.releases {
		return d.spaceHeld
	}
	return time.Since(d.spaceAt) < spaceRepeat
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
// writing in its first part (Shift+Tab: its last); on a cabinet it opens
// its doors, or shuts them. While writing, Tab goes on to the next part (designWrite).
func (m *Model) designTab(back bool) {
	d := m.ds()
	o := d.byID(d.sel)
	if o == nil {
		m.say("Nothing chosen. Use the arrows, or click one.")
		return
	}
	if o.Kind == kCabinet { // Tab opens its doors, or shuts them (design_cabinet.go)
		m.designDoors(o)
		return
	}
	if o.Kind == kDrawer { // Tab pulls it out, or pushes it in (design_drawer.go)
		m.designPull(o)
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
// While you write, the structure grows taller (never wider: the text wraps
// at the width it has) so all of it shows, and the view follows. It stops a
// gap above a structure under it, unless it was already made to reach past
// that by hand; beyond, a page scrolls. designFit fits the chosen one
// exactly, smaller too. Pictures keep their shape. Notes are one page and
// cabinets size themselves, so they are left as they are.

const (
	fitMaxW     = 60 // columns a structure grows to before its text wraps
	fitMaxPageH = 40 // rows a page grows to; beyond, it scrolls
	fitMaxOctH  = 60
)

// fitSize is the size (in board cells) that shows all of o, at most maxH
// rows high (the screen's height: more and it scrolls, or is cut).
func (d *designState) fitSize(o *dOct, maxH int) (int, int, bool) {
	return d.fitSizeAt(o, 0, maxH)
}

// fitSizeAt is fitSize at width fixedW (0: the width that suits it best).
func (d *designState) fitSizeAt(o *dOct, fixedW, maxH int) (int, int, bool) {
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
		wide := func(least int) int { // the width a layout needs, unless the width is fixed
			if fixedW > 0 {
				return fixedW
			}
			return max(w, least)
		}
		if fixedW > 0 {
			w, cw = fixedW, fixedW-4
			rows = lines(o.Body, cw)
		}
		switch o.Layout % len(octLayouts) {
		case layoutPicture:
			w = wide(34)
			cw = w - 4
			rows = pic(cw)
		case layoutAbove:
			w = wide(34)
			cw = w - 4
			rows = 2 * max(pic(cw), rows) // the picture takes half the rows
		case layoutLeft, layoutRight:
			w = wide(40)
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
	if fixedW > 0 {
		w = fixedW
	}
	return w, max(mh, min(fitMaxPageH, maxH, lines(o.Body, w-4)+6)), true
}

// boardRows is how tall a structure may be: the screen's rows at 100%,
// less a row above and below, whatever the zoom (see designState.maxH).
func (m *Model) boardRows() int {
	return max(4, m.ds().maxH)
}

// growToFit makes o taller, never wider or smaller, until all of it shows
// (or it is as high as the screen, or a gap above a structure under it).
func (m *Model) growToFit(o *dOct) bool {
	d := m.ds()
	_, h, ok := d.fitSizeAt(o, o.W, m.boardRows())
	h = min(h, d.roomBelow(o))
	if !ok || h <= o.H {
		return false
	}
	o.H = h
	d.dirty = true
	return true
}

// roomBelow is how tall o may grow: down to a gap (snapGapY) above the
// closest structure under it. One it already reaches (made so by hand, or
// placed on it) does not count.
func (d *designState) roomBelow(o *dOct) int {
	w, h := d.size(o)
	room := 1 << 30
	for _, b := range d.octs {
		if b == nil || b.ID == o.ID {
			continue
		}
		bw, _ := d.size(b)
		if b.X >= o.X+w || o.X >= b.X+bw || b.Y < o.Y+h {
			continue // not under it, or already reached
		}
		room = min(room, b.Y-snapGapY-o.Y)
	}
	return room
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
		if o := d.byID(d.sel); o != nil && time.Since(d.snapAt) >= snapStay {
			d.moveByKeys(o, dx*step, dy*step) // it snapped just now: it stays a moment
		}
	} else if m.spaceDown() { // Space held: the view glides
		m.designPan(dx, dy, 1)
		d.spacePanned = true
	} else if step == 1 { // an arrow held: on to the next structure, every jumpEvery
		if time.Since(d.jumpAt) >= jumpEvery && !m.designJump(dx, dy) {
			m.designPan(dx, dy, 1)
		}
	} else {
		m.designPan(dx, dy, step)
	}
	d.dirty = true
	d.save()
	every := holdEvery
	if strings.HasPrefix(d.holdMods, "alt+") {
		every = holdAlt
	}
	return tea.Tick(every, func(time.Time) tea.Msg { return designHoldMsg{} })
}

// designPan moves the view step steps that way and chooses the structure
// in the middle of the screen.
func (m *Model) designPan(dx, dy, step int) {
	d := m.ds()
	z := d.z()
	d.camX += float64(dx*step*4) / z
	d.camY += float64(dy*step*2) / z
	m.designChooseMiddle()
	d.dirty = true
}

// ---- the view jumps to the next structure ---------------------------------------------
//
// An arrow (no Shift) moves the view to the closest structure that way from
// the chosen one (or from the middle of the screen), centres it and chooses
// it. With none that way, the view moves a step, as Shift + arrows always
// do. Held, it jumps on to the next one every jumpEvery.

const jumpEvery = 300 * time.Millisecond

// nextThatWay is the closest structure in direction (dx, dy) from the board
// point (fx, fy), skipping from: within 45° of the direction, the nearest
// along it counting a step to the side twice. A row counts as two columns,
// as on screen.
func (d *designState) nextThatWay(from *dOct, fx, fy float64, dx, dy int) *dOct {
	n := math.Hypot(float64(dx), float64(dy))
	if n == 0 {
		return nil
	}
	ux, uy := float64(dx)/n, float64(dy)/n
	var best *dOct
	bestScore := math.Inf(1)
	for _, b := range d.octs {
		if b == nil || b == from {
			continue
		}
		bw, bh := d.size(b)
		vx := float64(b.X) + float64(bw)/2 - fx
		vy := (float64(b.Y) + float64(bh)/2 - fy) * 2
		along := vx*ux + vy*uy
		side := math.Abs(vx*uy - vy*ux)
		if along <= 0 || side > along {
			continue
		}
		if s := along + 2*side; s < bestScore {
			best, bestScore = b, s
		}
	}
	return best
}

// centreOn moves the view so o is in the middle of the screen (a structure
// taller than the screen shows from its top).
func (m *Model) centreOn(o *dOct) {
	d := m.ds()
	z := d.z()
	if isContainer(o.Kind) || (d.opened.active && o.ID == d.opened.id) {
		// Its size on the screen is not its board size times the zoom (a
		// container's rows are one screen row each): centre what shows.
		x, y, w, h := d.rect(o)
		d.camX += (float64(x) + float64(w)/2 - float64(m.width)/2) / z
		if h > m.boardH()-2 {
			d.camY += float64(y-1) / z
		} else {
			d.camY += (float64(y) + float64(h)/2 - float64(m.boardH())/2) / z
		}
		d.dirty = true
		return
	}
	w, h := d.size(o)
	ox, oy := o.X, o.Y
	if x, y, ok := d.openPlace(o); ok { // open inside a rack: where it shows
		ox, oy = x, y
	}
	screenW, screenH := float64(m.width)/z, float64(m.boardH())/z
	d.camX = float64(ox) + float64(w)/2 - screenW/2
	if float64(h) > screenH-2 {
		d.camY = float64(oy) - 1
	} else {
		d.camY = float64(oy) + float64(h)/2 - screenH/2
	}
	d.dirty = true
}

// designJump moves the view to the next structure in direction (dx, dy)
// and chooses it. It says whether there was one.
func (m *Model) designJump(dx, dy int) bool {
	d := m.ds()
	from := d.byID(d.sel)
	var fx, fy float64
	if from != nil {
		w, h := d.size(from)
		fx, fy = float64(from.X)+float64(w)/2, float64(from.Y)+float64(h)/2
	} else {
		fx, fy = d.world(m.width/2, m.boardH()/2)
	}
	o := d.nextThatWay(from, fx, fy, dx, dy)
	if o == nil {
		return false
	}
	m.centreOn(o)
	d.sel, d.editing, d.selLink = o.ID, partNone, -1
	d.jumpAt = time.Now()
	m.msg = ""
	return true
}

// ---- zooming by keys ----------------------------------------------------------------

// designZoomTo zooms to level i by keyboard (+ − 0, and the toolbar's + −):
// around the chosen structure, which stays in the middle of the screen and
// on it whole (at any zoom it is drawn no taller than the screen), or,
// with none chosen, around the middle of the screen. The wheel zooms
// toward the pointer instead.
func (m *Model) designZoomTo(i int) {
	d := m.ds()
	cx, cy := m.width/2, m.boardH()/2
	o := d.byID(d.sel)
	if o == nil {
		d.zoomTo(i, cx, cy)
		return
	}
	d.zoomTo(i, cx, cy)
	m.centreOn(o)
}

// ---- Alt tapped twice: Esc ------------------------------------------------------------
//
// Alt (the left one) pressed and let go on its own twice within altTwice
// does what Esc does: stops writing, cancels Alt+N, leaves link mode, puts
// the pen down, or else lets go of the chosen structure. (kitty says when
// Alt alone goes down and up; other terminals do not, so there it is Esc.)

const altTwice = 450 * time.Millisecond

// designAltTap counts an Alt tap; the second one soon after is Esc. It
// says whether it was.
func (m *Model) designAltTap() bool {
	d := m.ds()
	if d.altTapAt.IsZero() || time.Since(d.altTapAt) > altTwice {
		d.altTapAt = time.Now()
		return false
	}
	d.altTapAt = time.Time{}
	m.designEscape()
	return true
}

// designEscape is Esc in Design mode.
func (m *Model) designEscape() {
	d := m.ds()
	switch {
	case d.editing != partNone:
		d.editing = partNone
	case d.placing:
		d.placing = false
		m.say("Nothing placed.")
	case len(d.multi) > 0:
		d.clearMulti()
		m.say("Let go of them.")
	case d.platesOpen:
		d.platesOpen = false
	case d.groupMode:
		m.designGroupMode(false)
	case d.opened.active:
		m.designRackClose()
	case d.labelMode:
		m.designLabelMode()
	case d.linkMode:
		m.designLinkToggle()
	case d.pen >= 0:
		d.pen = -1
		m.say("Pen put down.")
	default:
		d.sel = -1
	}
	d.dirty = true
}

// designLayoutKey is AltGr+L: the chosen shape's next layout (its [¶] does
// the same with the mouse).
func (m *Model) designLayoutKey() {
	d := m.ds()
	o := d.byID(d.sel)
	if o == nil || o.Kind != kOct {
		m.say("AltGr+L changes the layout of a chosen shape.")
		return
	}
	m.designLayout(o, 1)
	d.save()
}
