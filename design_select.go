package main

// ---- choosing several at once ----------------------------------------------------------
//
// Several structures can be chosen together; each has a bright frame:
//
//	Space              on (or over, with the mouse) a structure with a name
//	                   plate: everything with that plate is chosen
//	Ctrl + drag        on the empty board: a shaded box; everything it
//	                   touches is chosen (with what was chosen already)
//	Ctrl + click       on a structure: chosen too, or no longer
//
// Then dragging one of them, or Alt + arrows, moves them all; X X deletes
// them all; Shift+G picks them for a name plate; Esc (or a click on the
// empty board, or on a structure not chosen) lets go of them.

import (
	"sort"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// multiIDs are the chosen structures still on the board, in board order.
func (d *designState) multiIDs() []int {
	var ids []int
	for _, o := range d.octs {
		if o != nil && d.multi[o.ID] {
			ids = append(ids, o.ID)
		}
	}
	return ids
}

// multiOn is whether several are chosen and o is one of them.
func (d *designState) multiOn(o *dOct) bool {
	return o != nil && len(d.multi) > 1 && d.multi[o.ID]
}

// clearMulti lets go of everything chosen together.
func (d *designState) clearMulti() {
	if len(d.multi) > 0 {
		d.multi = nil
		d.dirty = true
	}
}

// sayMulti says how many are chosen and what they can do.
func (m *Model) sayMulti(what string) {
	n := len(m.ds().multiIDs())
	m.say(what + plural(n, "structure") + " chosen.")
}

// ---- Space: a whole group ---------------------------------------------------------------

// designHover notes the structure under the mouse (the mouse moved, no
// button held). Nothing on the board changes, so the last drawing is used.
func (m *Model) designHover(ms tea.Mouse) {
	d := m.ds()
	d.hover = 0
	if sy := ms.Y - m.boardTop(); sy >= 0 && sy < m.boardH() {
		if o := d.at(ms.X, sy); o != nil {
			d.hover = o.ID + 1
		}
	}
	d.hoverOnly = true
}

// designSpaceTap is a tap of Space: on a container that opens and shuts
// (under the mouse, or else the chosen one) it opens it or shuts it: a
// cabinet's doors, a drawer, the thing open in a rack or on a board (a rack
// with nothing open opens its first; a board, its chosen pin). On anything
// else it chooses the whole group (its name plate's).
func (m *Model) designSpaceTap() {
	if !m.designSpaceOpen() {
		m.designSpaceGroup()
	}
}

// designSpaceOpen opens or shuts the container Space is on; it says
// whether there was one.
func (m *Model) designSpaceOpen() bool {
	d := m.ds()
	o := d.byID(d.hover - 1)
	if d.hover == 0 || o == nil || !opens(o.Kind) {
		o = d.byID(d.sel)
	}
	if d.opened.active && (o == nil || o.ID == d.opened.id || o.ID == d.opened.rack) {
		m.designRackClose() // what is open in a rack or on a board goes back
		d.dirty = true
		return true
	}
	if o == nil || !opens(o.Kind) {
		return false
	}
	switch o.Kind {
	case kCabinet:
		m.designDoors(o)
	case kDrawer:
		m.designPull(o)
	case kRack:
		if len(o.Items) == 0 {
			return false
		}
		m.designRackOpen(o, 0)
	case kBoard:
		if !d.pin.active || d.pin.board != o.ID {
			return false
		}
		m.designRackOpen(o, d.pin.at)
		d.pin.active = false
	}
	d.dirty = true
	return true
}

// opens is whether a structure of kind k opens and shuts.
func opens(k string) bool {
	return k == kCabinet || k == kDrawer || k == kRack || k == kBoard
}

// designSpaceGroup chooses every structure with the name plate of the one
// under the mouse, or else of the chosen one. It says whether it did.
func (m *Model) designSpaceGroup() bool {
	d := m.ds()
	o := d.byID(d.hover - 1)
	if d.hover == 0 || o == nil || o.Plate == "" {
		o = d.byID(d.sel)
	}
	if o == nil || o.Plate == "" {
		return false
	}
	d.multi = map[int]bool{}
	for _, p := range d.octs {
		if p != nil && p.Plate == o.Plate {
			d.multi[p.ID] = true
		}
	}
	inside := len(d.plates()[o.Plate]) - len(d.multi) // in containers: not on the board to choose
	d.sel, d.editing, d.selLink = o.ID, partNone, -1
	d.dirty = true
	what := "Group “" + o.Plate + "”: "
	if inside > 0 {
		what = "Group “" + o.Plate + "” (" + plural(inside, "more") + " in containers): "
	}
	m.sayMulti(what)
	return true
}

// spaceTapGap: in terminals that do not say when Space is let go, a Space
// press this long after the last one is a new tap (not the key repeating).
const spaceTapGap = spaceRepeat

// designSpacePress is Space going down: in kitty, a tap (let go with no
// arrow pressed) chooses the group when it is let go; elsewhere at once.
func (m *Model) designSpacePress(repeat bool, last time.Time) {
	d := m.ds()
	if repeat {
		return
	}
	if m.releases {
		d.spaceTap = true
		return
	}
	if time.Since(last) >= spaceTapGap {
		m.designSpaceTap()
	}
}

// designSpaceRelease is Space let go (kitty): a tap chooses the group.
func (m *Model) designSpaceRelease() {
	d := m.design
	if d == nil {
		return
	}
	tap := d.spaceTap && !d.spacePanned
	d.spaceTap = false
	if tap && m.inDesign() {
		m.designSpaceTap()
	}
}

// ---- Ctrl + drag: a box, Ctrl + click: one more ------------------------------------------

// marqueePress is a press with Ctrl (or Shift) held: on a structure it is
// chosen too (or no longer); on the empty board a box starts.
func (m *Model) marqueePress(o *dOct, sx, sy int) {
	d := m.ds()
	if d.multi == nil {
		d.multi = map[int]bool{}
	}
	if len(d.multi) == 0 && d.byID(d.sel) != nil {
		d.multi[d.sel] = true // what was chosen is one of them
	}
	d.drag = dragNone
	d.dirty = true
	if o != nil {
		if d.multi[o.ID] {
			delete(d.multi, o.ID)
		} else {
			d.multi[o.ID] = true
			d.sel = o.ID
		}
		d.editing = partNone
		m.sayMulti("")
		return
	}
	d.band = &groupBand{sx, sy, sx, sy}
	d.drag = dragMarquee
}

// marqueePick chooses everything the box touches, with what was chosen.
func (m *Model) marqueePick() {
	d := m.ds()
	b := d.band
	d.band = nil
	if b == nil {
		return
	}
	x0, x1 := min(b.x0, b.x1), max(b.x0, b.x1)
	y0, y1 := min(b.y0, b.y1), max(b.y0, b.y1)
	if d.multi == nil {
		d.multi = map[int]bool{}
	}
	for _, o := range d.octs {
		if o == nil {
			continue
		}
		x, y, w, h := d.rect(o)
		if x <= x1 && x+w-1 >= x0 && y <= y1 && y+h-1 >= y0 {
			d.multi[o.ID] = true
		}
	}
	if ids := d.multiIDs(); len(ids) > 0 {
		d.sel, d.editing = ids[len(ids)-1], partNone
	}
	d.dirty = true
	m.sayMulti("")
}

// startMultiDrag notes where each chosen structure was, as one is pressed
// to be dragged: they all move with it.
func (d *designState) startMultiDrag() {
	d.multiOrig = map[int][2]int{}
	for _, id := range d.multiIDs() {
		o := d.byID(id)
		d.multiOrig[id] = [2]int{o.X, o.Y}
	}
}

// followMulti moves the other chosen structures by as much as o moved from
// where it was (from: its place before).
func (d *designState) followMulti(o *dOct, fromX, fromY int) {
	if !d.multiOn(o) {
		return
	}
	dx, dy := o.X-fromX, o.Y-fromY
	if dx == 0 && dy == 0 {
		return
	}
	for _, id := range d.multiIDs() {
		if p := d.byID(id); p != nil && p != o {
			p.X, p.Y = p.X+dx, p.Y+dy
		}
	}
}

// dragMulti puts the other chosen structures where they were when the drag
// started, moved by as much as o has moved since.
func (d *designState) dragMulti(o *dOct) {
	if !d.multiOn(o) || d.multiOrig == nil {
		return
	}
	start, ok := d.multiOrig[o.ID]
	if !ok {
		return
	}
	dx, dy := o.X-start[0], o.Y-start[1]
	for id, at := range d.multiOrig {
		if p := d.byID(id); p != nil && p != o {
			p.X, p.Y = at[0]+dx, at[1]+dy
		}
	}
}

// designDeleteMulti deletes everything chosen; with things in containers,
// it asks first.
func (m *Model) designDeleteMulti() tea.Cmd {
	d := m.ds()
	ids := d.multiIDs()
	inside := 0
	for _, id := range ids {
		inside += countItems(d.byID(id))
	}
	del := func(m *Model) {
		d := m.ds()
		for _, id := range ids {
			if o := d.byID(id); o != nil {
				d.deleteStruct(o)
			}
		}
		d.multi = nil
		d.save()
		m.say("Deleted " + plural(len(ids), "structure") + ". Ctrl+Z brings them back.")
	}
	if inside == 0 {
		del(m)
		return nil
	}
	return m.confirm("Delete "+plural(len(ids), "structure")+" and the "+plural(inside, "thing")+" in them? y = yes, n = no", "yn", func(m *Model, k rune) tea.Cmd {
		if k == 'y' {
			del(m)
		} else {
			m.say("Kept.")
		}
		return nil
	})
}

// ---- drawing -----------------------------------------------------------------------------

// paintChosen draws a bright frame round each structure chosen together,
// and the shaded box being dragged out (Ctrl + drag).
func (m *Model) paintChosen(c *boardCanvas) {
	d := m.ds()
	cw, ch := c.Width(), c.Height()
	if ids := d.multiIDs(); len(ids) > 1 || (len(ids) == 1 && d.drag == dragMarquee) {
		sort.Ints(ids)
		for _, id := range ids {
			o := d.byID(id)
			if !d.onScreen(o, cw, ch) {
				continue
			}
			x, y, w, h := d.rect(o)
			boxFrame(c, x-1, y-1, x+w, y+h, cAccent, "┏┓┗┛━┃")
		}
	}
	if d.drag == dragMarquee && d.band != nil {
		paintBand(c, d.band)
	}
}

// paintBand draws a box being dragged out: shaded inside (what is under it
// shows through, tinted), a frame round it.
func paintBand(c *boardCanvas, b *groupBand) {
	x0, x1 := min(b.x0, b.x1), max(b.x0, b.x1)
	y0, y1 := min(b.y0, b.y1), max(b.y0, b.y1)
	cw, ch := c.Width(), c.Height()
	for y := max(y0, 0); y <= min(y1, ch-1); y++ {
		for x := max(x0, 0); x <= min(x1, cw-1); x++ {
			cell := c.Lines[y][x]
			bg := colorHex(cell.Style.Bg, cBG)
			tint := mixHex(bg, cAccent, 0.22)
			st := cell.Style
			st.Bg = rgbaOf(tint)
			if cell.Content == " " || cell.Content == "" || cell.Content == "·" {
				// The empty board: a texture.
				c.Lines[y][x] = uv.Cell{Content: "░", Width: 1, Style: uv.Style{Fg: rgbaOf(mixHex(tint, cAccent, 0.45)), Bg: rgbaOf(tint)}}
				continue
			}
			cell.Style = st
			c.Lines[y][x] = cell
		}
	}
	boxFrame(c, x0, y0, x1, y1, mixHex(cAccent, cText, 0.3), "┏┓┗┛┅┇")
}

// boxFrame draws a frame from (x0, y0) to (x1, y1) in colour fg, with the
// letters g: four corners (top left, top right, bottom left, bottom
// right), then the across and down lines.
func boxFrame(c *boardCanvas, x0, y0, x1, y1 int, fg string, g string) {
	r := []rune(g)
	cw, ch := c.Width(), c.Height()
	put := func(x, y int, s rune) {
		if x < 0 || y < 0 || x >= cw || y >= ch {
			return
		}
		cell := c.Lines[y][x]
		st := cell.Style
		st.Fg, st.Attrs = rgbaOf(fg), uv.AttrBold
		if st.Bg == nil {
			st.Bg = rgbaOf(cBG)
		}
		c.Lines[y][x] = uv.Cell{Content: string(s), Width: 1, Style: st}
	}
	for x := x0 + 1; x < x1; x++ {
		put(x, y0, r[4])
		put(x, y1, r[4])
	}
	for y := y0 + 1; y < y1; y++ {
		put(x0, y, r[5])
		put(x1, y, r[5])
	}
	put(x0, y0, r[0])
	put(x1, y0, r[1])
	put(x0, y1, r[2])
	put(x1, y1, r[3])
}
