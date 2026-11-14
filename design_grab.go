package main

// Alt+Space: grab a structure with the keyboard, or show a thing that is
// in a container big.
//
//   - In 3D, Alt+Space takes up the structure the cross is on (or, with
//     the cross on nothing, the one nearest the middle of the screen). It
//     then rides under the cross as the camera looks round and moves (the
//     mouse, the arrows, W A S D Q E), exactly as a mouse drag carries it.
//     Alt+Space again (or a click) puts it down where it is, as a drag let
//     go does: on the lowest layer where it covers nothing, or into the
//     container it is over. Esc (or Alt tapped twice) puts it back where
//     it was. The whole carry is one undo step.
//   - On the flat board, Alt+Space takes up the chosen structure; the
//     arrows move it (as Alt + arrows do), Alt+Space puts it down, Esc
//     puts it back.
//   - On a thing inside a container (in a drawer, on a cabinet's shelf, in
//     a rack's row, in a box, a pin on a board), and nothing being written,
//     Alt+Space shows it big instead: out of its container, as big as most
//     of the screen, in the middle (in 3D in front of everything, the
//     camera square in front of it). Alt+Space again, Esc or Alt tapped
//     twice puts it back in its place in the container (in 3D the camera
//     goes back too). It uses the rack's "open" (d.opened, max set), so
//     Tab shows the container's next thing big, and dragging it takes it
//     out for good, as with a rack.

import (
	"math"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// grabState is a structure carried by Alt+Space, and the 3D camera from
// before a thing was shown big.
type grabState struct {
	active   bool
	in3D     bool     // carried in the room: it follows the cross (else the arrows move it)
	id       int      // what is carried
	x, y     int      // where it was taken up
	layer    int      // and its layer
	cam      r3camKey // the camera it last followed
	camSaved bool     // shown big in 3D: the camera to go back to
	camPos   r3vec
	camLook  [2]float64
	camK     float64
}

// maxFill is how much of the screen's width or height a thing shown big
// takes, at most.
const maxFill = 0.8

const (
	grabHint3D = "Carrying it  ·  look or move: it follows the cross  ·  Alt+Space or click: put down  ·  Esc: put back"
	grabHint2D = "Carrying it  ·  arrows: move  ·  Alt+Space: put down  ·  Esc: put back"
	maxHint    = "Shown big  ·  Alt+Space, Esc or Alt twice: put back  ·  Tab: the next one  ·  drag: take out"
)

// isGrabKey is whether name is Alt+Space (some terminals give it as "alt+ ").
func isGrabKey(name string) bool { return name == "alt+space" || name == "alt+ " }

// grabStatus is the status bar's line while something is carried or shown
// big, else "".
func (m *Model) grabStatus() string {
	d := m.ds()
	switch {
	case d.grab.active && d.drag == dragMove && d.dropOn >= 0:
		return "Alt+Space: put it in " + octName(d.byID(d.dropOn)) + "  ·  Esc: put back"
	case d.grab.active && d.grab.in3D:
		return grabHint3D
	case d.grab.active:
		return grabHint2D
	case d.opened.active && d.opened.max:
		return maxHint
	}
	return ""
}

// designGrabKey takes Alt+Space, and the keys while something is carried.
// It says whether it took the key.
func (m *Model) designGrabKey(k tea.Key, name string) (bool, tea.Cmd) {
	d := m.ds()
	g := &d.grab
	if g.active {
		return m.grabKey(k, name)
	}
	if !isGrabKey(name) || d.editing != partNone || d.placing {
		return false, nil
	}
	if d.opened.active && d.opened.max {
		m.designRackClose() // shown big: back in its container
		return true, nil
	}
	if d.room.on() {
		m.grab3DStart()
		return true, nil
	}
	if c, i := m.grab2DItem(); c != nil {
		m.designMaximize(c, i)
		return true, nil
	}
	m.grab2DStart()
	return true, nil
}

// grabKey is a key while something is carried: Alt+Space puts it down,
// Esc puts it back; in 3D the keys that look and move the camera work
// (it follows the cross), on the flat board the arrows move it. Other keys
// do nothing, so nothing else happens to the board mid-carry.
func (m *Model) grabKey(k tea.Key, name string) (bool, tea.Cmd) {
	d := m.ds()
	switch {
	case isGrabKey(name):
		return true, m.grabDrop()
	case name == "esc":
		m.grabCancel()
		return true, nil
	case name == "ctrl+f":
		m.grabCancel() // leaving 3D (or going in) mid-carry: put back first
		return false, nil
	}
	if d.grab.in3D {
		if walkKey(name) != 0 || name == "m" || name == "shift+m" || name == "=" || name == "-" || name == "0" || name == "minus" {
			return false, nil // the camera moves; what is carried follows (grabFollow)
		}
		switch name {
		case "left", "right", "up", "down", "shift+left", "shift+right", "shift+up", "shift+down":
			return false, nil
		}
		m.say(grabHint3D)
		return true, nil
	}
	switch name {
	case "left", "right", "up", "down", "shift+left", "shift+right", "shift+up", "shift+down":
		// As Alt + arrows: it moves (Shift: further), held it glides.
		k.Mod |= tea.ModAlt
		return true, m.designKey(k)
	case "alt+left", "alt+right", "alt+up", "alt+down", "alt+shift+left", "alt+shift+right", "alt+shift+up", "alt+shift+down":
		return false, nil
	}
	m.say(grabHint2D)
	return true, nil
}

// grabEsc is Esc (Alt tapped twice) for Alt+Space: what is carried goes
// back, what is shown big goes back in its container. It says whether
// there was either.
func (m *Model) grabEsc() bool {
	d := m.ds()
	switch {
	case d.grab.active:
		m.grabCancel()
		return true
	case d.opened.active && d.opened.max:
		m.designRackClose()
		d.dirty = true
		return true
	}
	return false
}

// ---- carrying --------------------------------------------------------------------------

// grab3DStart is Alt+Space in 3D: what the cross is on is taken up (a thing
// in a container is shown big instead), else the structure nearest the
// middle of the screen.
func (m *Model) grab3DStart() {
	d := m.ds()
	r := &d.room
	hit := r.pickAim()
	var o *dOct
	if hit.ok && hit.own >= 0 {
		o = d.byID(hit.own)
	}
	if o != nil && d.opened.active && !d.opened.max && o.ID == d.opened.id {
		m.grabOpenedBig() // open in a rack: a thing in a container
		return
	}
	if o != nil {
		if c, i := d.containerItemAt(o, hit.sx, hit.sy); c != nil {
			m.designMaximize(c, i)
			return
		}
	}
	pressX, pressY := hit.sx, hit.sy
	plane := hit.p.z
	near := false
	if o == nil {
		b := r.nearestBlock()
		if b == nil {
			m.say("Nothing to grab: no structure on the screen.")
			return
		}
		o, near = b.o, true
		plane = b.off + b.dep // its front
		x, y, w, h := d.rect(o)
		pressX, pressY = x+w/2, y+h/2 // its middle comes under the cross
	}
	if d.opened.active && o.ID == d.opened.rack {
		m.designRackClose() // a rack with something open: shut first, so it moves whole
	}
	d.save() // what came before is a step of its own
	r.plane = plane
	r.mapX, r.mapY = pressX, pressY
	if near {
		m.design3DThrough(tea.Mouse{}) // where the cross meets its front's plane
	}
	m.grabTakeUp(o, pressX, pressY)
	d.drag, d.dropOn, d.dropSlot = dragMove, -1, -1 // a drag, moved by the camera (designMotion)
	g := &d.grab
	g.in3D = true
	g.cam = r.camKey(m.width, m.boardH())
	if near {
		m.designMotion(tea.Mouse{}) // under the cross at once
	}
	r.chosenCam = g.cam
	m.say(kindIcons[o.Kind] + " " + itemTitle(o) + " taken up · it follows the cross · Alt+Space: put down · Esc: put back")
}

// grab2DStart is Alt+Space on the flat board: the chosen structure is
// taken up; the arrows move it.
func (m *Model) grab2DStart() {
	d := m.ds()
	o := d.byID(d.sel)
	if o == nil {
		m.say("Alt+Space: choose a structure first (an arrow, or a click), then Alt+Space takes it up.")
		return
	}
	if d.opened.active && o.ID == d.opened.rack {
		m.designRackClose()
	}
	d.save()
	m.grabTakeUp(o, 0, 0)
	m.say(kindIcons[o.Kind] + " " + itemTitle(o) + " taken up · arrows: move · Alt+Space: put down · Esc: put back")
}

// grabTakeUp starts carrying o, pressed at flat cell (sx, sy).
func (m *Model) grabTakeUp(o *dOct, sx, sy int) {
	d := m.ds()
	d.toFront(o)
	d.sel, d.selLink, d.editing = o.ID, -1, partNone
	d.clearMulti()
	d.pin, d.boxPick = pinSel{}, boxSel{}
	d.origX, d.origY, d.origW, d.origH = o.X, o.Y, o.W, o.H
	d.pressX, d.pressY, d.lastPX, d.lastPY = sx, sy, sx, sy
	d.origCX, d.origCY = d.camX, d.camY
	d.grab = grabState{active: true, id: o.ID, x: o.X, y: o.Y, layer: o.Layer,
		camSaved: d.grab.camSaved, camPos: d.grab.camPos, camLook: d.grab.camLook, camK: d.grab.camK}
	d.dirty = true
}

// grabFollow runs after every message in 3D (design3DAfter): what is
// carried follows the cross when the camera has moved, and stays the
// chosen one (the middle of the view does not choose meanwhile).
func (m *Model) grabFollow() {
	d := m.ds()
	g := &d.grab
	if !g.active || !g.in3D {
		return
	}
	r := &d.room
	o := d.byID(g.id)
	if o == nil || !r.on() {
		g.active = false // deleted, or the room is flat: nothing to carry
		d.drag, d.dropOn = dragNone, -1
		return
	}
	d.drag = dragMove
	key := r.camKey(m.width, m.boardH())
	if key != g.cam {
		g.cam = key
		m.designMotion(tea.Mouse{})
	}
	d.sel = g.id
	r.chosenCam = key
}

// grabDrop puts what is carried down where it is: in 3D as a drag let go
// (on the lowest layer where it covers nothing, or into the container it
// is over). One undo step.
func (m *Model) grabDrop() tea.Cmd {
	d := m.ds()
	g := d.grab
	d.grab.active = false
	o := d.byID(g.id)
	if o == nil {
		d.drag, d.dropOn = dragNone, -1
		return nil
	}
	d.sel = o.ID
	d.dirty = true
	if g.in3D {
		d.drag = dragMove
		into := d.byID(d.dropOn)
		if into == nil {
			m.design3DSettle(o.ID) // its layer now, so the step has it (designRelease settles after its save)
		}
		cmd := m.designRelease() // settles its layer, or files it, and saves the step
		if into != nil && d.byID(g.id) == nil {
			m.say(kindIcons[o.Kind] + " " + itemTitle(o) + " put in " + octName(into) + ".")
		} else if m.msg == "" || strings.HasPrefix(m.msg, kindIcons[o.Kind]+" "+itemTitle(o)+" taken up") {
			m.say(kindIcons[o.Kind] + " " + itemTitle(o) + " put down: " + layerName(o.Layer) + ".")
		}
		return cmd
	}
	d.drag, d.dropOn = dragNone, -1
	d.save()
	m.say(kindIcons[o.Kind] + " " + itemTitle(o) + " put down.")
	return nil
}

// grabCancel puts what is carried back where it was taken up. No undo step.
func (m *Model) grabCancel() {
	d := m.ds()
	g := d.grab
	d.grab.active = false
	d.drag, d.dropOn, d.dropSlot = dragNone, -1, -1
	if o := d.byID(g.id); o != nil {
		o.X, o.Y, o.Layer = g.x, g.y, g.layer
		d.sel = o.ID
		m.say(kindIcons[o.Kind] + " " + itemTitle(o) + " put back where it was.")
	}
	d.dirty = true
}

// nearestBlock is the block (as last drawn) whose front's middle is
// nearest the middle of the screen, in front of the camera; nil: none
// shows.
func (r *room3D) nearestBlock() *r3block {
	v, ok := r.nowView()
	if !ok {
		return nil
	}
	rm := &r.rm
	var best *r3block
	bestD := math.Inf(1)
	for i := range r.blocks {
		b := &r.blocks[i]
		mid := r3vec{float64(b.x0+b.x1)/2 - rm.hw, rm.hh - float64(b.y0+b.y1), b.off + b.dep}
		c := v.toCam(mid)
		if c.z <= 0.25 {
			continue // behind the camera
		}
		x, y := v.screenAt(c)
		if x < 0 || y < 0 || x > float64(rm.w) || y > float64(2*rm.h) {
			continue // off the screen
		}
		dx, dy := x-v.px, (y-v.py)/2 // in cells
		if dd := dx*dx + dy*dy; dd < bestD {
			best, bestD = b, dd
		}
	}
	return best
}

// ---- shown big ----------------------------------------------------------------------------

// actItem is the index in its container's Items of the thing a hit act
// stands for ("item:3", "open:3", "pin:3", "boxthing:3", "del:3"), or -1.
func actItem(act string) int {
	for _, p := range []string{"item:", "open:", "pin:", "boxthing:", "del:"} {
		if strings.HasPrefix(act, p) {
			if n, err := strconv.Atoi(act[len(p):]); err == nil {
				return n
			}
		}
	}
	return -1
}

// containerItemAt is the container o and the index of the thing in it at
// flat cell (sx, sy), or nil when there is none there.
func (d *designState) containerItemAt(o *dOct, sx, sy int) (*dOct, int) {
	if o == nil || !isContainer(o.Kind) {
		return nil, -1
	}
	hi := d.hit(sx, sy)
	if hi.o != o {
		return nil, -1
	}
	if i := actItem(hi.act); i >= 0 && i < len(o.Items) && o.Items[i] != nil {
		return o, i
	}
	return nil, -1
}

// grab2DItem is the chosen thing inside a container on the flat board: a
// box's chosen thing, a board's chosen pin, or the thing the mouse is on
// in the chosen container; nil: none.
func (m *Model) grab2DItem() (*dOct, int) {
	d := m.ds()
	if b, it := d.chosenBoxThing(); it != nil {
		return b, d.boxPick.at
	}
	if b, it := d.chosenPin(); it != nil {
		return b, d.pin.at
	}
	if d.opened.active && !d.opened.max && d.sel == d.opened.id {
		if r := d.byID(d.opened.rack); r != nil {
			at := d.opened.at
			m.designRackClose()
			if at >= 0 && at < len(r.Items) {
				return r, at
			}
		}
		return nil, -1
	}
	if o := d.byID(d.sel); o != nil && isContainer(o.Kind) && d.hover == o.ID+1 {
		c := m.pxm.cell
		return d.containerItemAt(o, c[0], c[1]-m.boardTop())
	}
	return nil, -1
}

// grabOpenedBig: Alt+Space on the thing open in a rack (or a board's pin
// opened): it goes back and is shown big instead.
func (m *Model) grabOpenedBig() {
	d := m.ds()
	r, at := d.byID(d.opened.rack), d.opened.at
	m.designRackClose()
	if r != nil && at >= 0 && at < len(r.Items) && r.Items[at] != nil {
		m.designMaximize(r, at)
	}
}

// designMaximize takes thing i out of container c and shows it big in the
// middle of the view (see the top of this file), until put back.
func (m *Model) designMaximize(c *dOct, i int) {
	d := m.ds()
	if c == nil || i < 0 || i >= len(c.Items) || c.Items[i] == nil {
		return
	}
	it := c.Items[i]
	if d.opened.active {
		m.designRackClose() // one at a time (it may go back into c: i is found again)
		i = slicesIndex(c.Items, it)
		if i < 0 {
			return
		}
	}
	c.Items = append(c.Items[:i:i], c.Items[i+1:]...)
	op := rackOpen{id: it.ID, rack: c.ID, at: i, origW: it.W, origH: it.H, active: true,
		max: true, origX: it.X, origY: it.Y, layer: it.Layer}
	z := d.z()
	if resizable(it) {
		// As big as most of the screen, keeping its shape.
		w0, h0 := it.W, it.H
		mw, mh := minSize(it.Kind)
		w0, h0 = max(w0, mw), max(h0, mh)
		fw, fh := maxFill*float64(m.width)/z, maxFill*float64(m.boardH())/z
		k := math.Min(fw/float64(w0), fh/float64(h0))
		if k > 1 {
			it.W, it.H = max(mw, int(float64(w0)*k)), max(mh, int(float64(h0)*k))
		}
	}
	cx, cy := m.screenMiddle()
	wx, wy := d.world(cx, cy)
	iw, ih := d.size(it)
	it.X, it.Y = int(math.Floor(wx))-iw/2, int(math.Floor(wy))-ih/2
	d.octs = append(d.octs, it)
	d.sel, d.editing, d.selLink = it.ID, partNone, -1
	d.pin, d.boxPick = pinSel{}, boxSel{}
	d.clearMulti()
	d.opened = op
	d.dirty = true
	if r := &d.room; r.on() {
		// In front of everything, the camera square in front of it.
		it.Layer = min(maxLayer, c.Layer+1)
		for it.Layer < maxLayer && d.coversOnLayer(it) {
			it.Layer++
		}
		g := &d.grab
		if !g.camSaved {
			g.camSaved, g.camPos, g.camLook, g.camK = true, r.pos, r.look, r.freeK
		}
		if r.rm.w > 0 {
			r.area(d, m.width, m.boardH(), d.z()) // its block, for the camera to face (drawn again before it shows)
			m.design3DFront()
		}
	}
	m.say(kindIcons[it.Kind] + " " + itemTitle(it) + " from " + octName(c) + ", shown big · Alt+Space, Esc or Alt twice: put back")
}

// grabMaxStep shows the container's next thing big (step 1, Tab) or the
// one before (-1), the one shown big going back first.
func (m *Model) grabMaxStep(c *dOct, op rackOpen, step int) {
	d := m.ds()
	m.designRackClose()
	n := len(c.Items)
	if n == 0 {
		return
	}
	next := ((op.at+step)%n + n) % n
	m.designMaximize(c, next)
	if d.opened.active {
		it := d.byID(d.opened.id)
		m.say(kindIcons[it.Kind] + " " + itemTitle(it) + " shown big (" + itoa(next+1) + " of " + itoa(n) + ") · Tab: next · Alt+Space: put back")
	}
}

// grabCameraBack puts the 3D camera back where it was before a thing was
// shown big.
func (m *Model) grabCameraBack() {
	d := m.ds()
	g := &d.grab
	if !g.camSaved {
		return
	}
	g.camSaved = false
	if r := &d.room; r.free {
		r.pos, r.look, r.freeK = g.camPos, g.camLook, g.camK
	}
}

// slicesIndex is the index of o in os, or -1.
func slicesIndex(os []*dOct, o *dOct) int {
	for i, p := range os {
		if p == o {
			return i
		}
	}
	return -1
}
