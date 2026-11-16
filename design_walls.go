package main

// ---- a room's walls and floor: turning round, looking down ---------------------------
//
// Every room has 4 walls (more if added: AltGr+N, then W) and a floor; each
// is a board of its own (design_rooms.go).
//
//	AltGr+R          turn to the next wall (AltGr+Shift+R: the one before)
//	AltGr+F          look down at the floor; again, look up at the wall
//
// The turn is shown as a spin: the wall you leave folds away to one side
// as the next one swings in from the other, as the faces of a turning box
// do; looking down, the wall folds up as the floor comes up from below.
// The floor is the place for what is not finished: anything you do not
// want on a wall yet.
//
// Also here, choosing several to give one name plate:
//
//	Alt, then Space  (Alt tapped, then Space) the chosen structure is
//	                 added to those chosen together, or taken out
//	Alt+A            the structures chosen together (two or more) get one
//	                 name plate, written on the board (design_tags.go)

import (
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// spinFor is how long a turn to the next wall (or down to the floor) takes.
const spinFor = 480 * time.Millisecond

// altSpaceFor is how soon after a tap of Alt a Space chooses one more.
const altSpaceFor = 1500 * time.Millisecond

// wallSpin is a turn being shown: the board left and the one come to, as
// drawn when it started.
type wallSpin struct {
	from, to *boardCanvas
	start    time.Time
	dir      int  // 1: to the next wall (it comes from the right), -1: the one before
	vertical bool // looking down (dir 1) or up (dir -1)
}

// isSpinKey is AltGr+R (¶ on a UK keyboard; kitty with AltGr held: r):
// 1 the next wall, -1 (with Shift: ®) the one before, 0 not it.
func (m *Model) isSpinKey(k tea.Key, name string) int {
	switch {
	case k.Text == "¶" || name == "¶" || name == "altgr+r":
		return 1
	case k.Text == "®" || name == "®" || name == "altgr+shift+r":
		return -1
	case m.altGr.down && k.Code == 'r' && k.Mod == 0:
		return 1
	case m.altGr.down && (k.Code == 'r' || k.Code == 'R') && k.Mod == tea.ModShift:
		return -1
	}
	return 0
}

// isFloorKey is AltGr+F (đ on a UK keyboard; kitty with AltGr held: f).
func (m *Model) isFloorKey(k tea.Key, name string) bool {
	return k.Text == "đ" || name == "đ" || name == "altgr+f" || (m.altGr.down && k.Code == 'f' && k.Mod == 0)
}

// spinWall turns to the next wall (dir 1) or the one before (-1).
func (m *Model) spinWall(dir int) {
	h := m.hs()
	r := h.room(h.Current)
	if r.Down {
		m.say("You are looking at the floor: AltGr+F looks up, then AltGr+R turns.")
		return
	}
	n := r.walls()
	to := ((r.at()+dir)%n + n) % n
	m.turnTo(to, dir, false)
	m.say("Wall " + itoa(to+1) + " of " + itoa(n) + " · " + h.room(h.Current).Name + "  ·  AltGr+R: next wall · AltGr+F: floor")
}

// floorToggle looks down at the floor, or up at the wall again.
func (m *Model) floorToggle() {
	h := m.hs()
	r := h.room(h.Current)
	if r.Down {
		m.turnTo(r.Wall, -1, true)
		m.say("Wall " + itoa(r.at()+1) + " of " + itoa(r.walls()) + "  ·  AltGr+F: the floor again")
		return
	}
	m.turnTo(wallFloor, 1, true)
	m.say("The floor: for what is not finished yet  ·  AltGr+F: look up")
}

// turnTo goes to wall (or wallFloor) of the room you are in, with a spin.
func (m *Model) turnTo(wall, dir int, vertical bool) {
	d := m.ds()
	if d.through != nil {
		m.finishThrough()
	}
	if d.arrive != nil {
		m.finishArrive()
	}
	m.finishSpin()
	var from *boardCanvas
	if designAnims && m.width > 0 && m.boardH() > 0 {
		from = cloneCanvas(m.designCanvas())
	}
	nd := m.switchBoard(m.hs().Current, wall)
	nd.frame, nd.frameAt = "", time.Time{}
	if from == nil {
		return
	}
	to := cloneCanvas(m.designCanvas())
	m.spin = &wallSpin{from: from, to: to, start: animNow(), dir: dir, vertical: vertical}
}

// addWall is AltGr+N, then W: a new wall in this room, after the one you
// face, turned to at once.
func (m *Model) addWall() {
	h := m.hs()
	r := h.room(h.Current)
	if r.Down {
		m.say("Look up first (AltGr+F): a new wall goes after the one you face.")
		return
	}
	at, n := r.at(), r.walls()
	// The walls after this one move up a place, last first, so the new one
	// (empty) is next.
	d := m.ds()
	d.save()
	d.flushSave()
	for w := n - 1; w > at; w-- {
		m.moveWall(h.Current, w, w+1)
	}
	r.Walls = n + 1
	if err := h.save(); err != nil {
		r.Walls = n
		m.say("The new wall could not be saved: " + err.Error())
		return
	}
	m.turnTo(at+1, 1, false)
	m.say("New wall: wall " + itoa(at+2) + " of " + itoa(n+1) + ".  AltGr+R turns round the room.")
}

// moveWall moves room id's wall from one place to another (its board's
// folder, and the copy kept in m.rooms). Wall 0 never moves.
func (m *Model) moveWall(id, from, to int) {
	if from == 0 || to == 0 {
		return
	}
	src, dst := boardPath(id, from), boardPath(id, to)
	if b := m.rooms[boardKey(id, from)]; b != nil {
		b.flushSave()
	}
	moveBoardDir(src, dst)
	delete(m.rooms, boardKey(id, from))
	delete(m.rooms, boardKey(id, to))
}

// finishSpin ends a spin at once (a key, or the next one starting).
func (m *Model) finishSpin() {
	if m.spin != nil {
		m.spin = nil
		if m.design != nil {
			m.design.frame, m.design.frameAt = "", time.Time{}
		}
	}
}

// spinStep ends the spin once its time is up.
func (m *Model) spinStep() {
	if m.spin != nil && animNow().Sub(m.spin.start) >= spinFor {
		m.finishSpin()
	}
}

// spinRows writes the spin's frame now (true), or nothing (false: no spin).
func (m *Model) spinRows(sb *strings.Builder) bool {
	s := m.spin
	if s == nil {
		return false
	}
	w, h := m.width, m.boardH()
	if s.from.Width() != w || s.from.Height() != h || s.to.Width() != w || s.to.Height() != h {
		m.finishSpin() // the window changed size: no spin
		return false
	}
	k := openEase(float64(animNow().Sub(s.start)) / float64(spinFor))
	spinFrame(s, k).renderTo(sb)
	return true
}

// spinFrame is the spin at k (0 start, 1 done): the two boards as two faces
// of a box turning, each squeezed to the width (or height) it shows at,
// the one turning away darker.
func spinFrame(s *wallSpin, k float64) *boardCanvas {
	w, h := s.from.Width(), s.from.Height()
	out := newBoardCanvas(w, h)
	th := k * math.Pi / 2
	ca, sa := math.Cos(th), math.Sin(th)
	partA := ca / (ca + sa) // the share the board left still shows
	size := w
	if s.vertical {
		size = h
	}
	cut := int(math.Round(partA * float64(size)))
	if s.dir < 0 {
		cut = size - cut // the board left is on the far side
	}
	shadeA, shadeB := 1-0.55*k, 1-0.55*(1-k)
	// at gives the board and the place in it for screen place i (a column,
	// or a row looking down).
	at := func(i int) (*boardCanvas, int, float64) {
		first, second := s.from, s.to
		f1, f2 := shadeA, shadeB
		if s.dir < 0 {
			first, second, f1, f2 = s.to, s.from, shadeB, shadeA
		}
		if i < cut {
			return first, min(size-1, int(float64(i)*float64(size)/float64(max(cut, 1)))), f1
		}
		return second, min(size-1, int(float64(i-cut)*float64(size)/float64(max(size-cut, 1)))), f2
	}
	for y := range h {
		line := out.Lines[y]
		for x := range w {
			var src *boardCanvas
			var u int
			var f float64
			sx, sy := x, y
			if s.vertical {
				src, u, f = at(y)
				sy = u
			} else {
				src, u, f = at(x)
				sx = u
			}
			c := src.Lines[sy][sx]
			if c.Width != 1 || c.Content == "" {
				c = uv.Cell{Content: " ", Width: 1, Style: c.Style}
			}
			c.Style.Fg, c.Style.Bg = shadeColor(c.Style.Fg, f), shadeColor(c.Style.Bg, f)
			line[x] = c
		}
	}
	// The corner between them: a line of a little light.
	if !s.vertical && cut > 0 && cut < w {
		for y := range h {
			c := &out.Lines[y][cut]
			c.Style.Bg = shadeColor(c.Style.Bg, 1.35)
		}
	}
	return out
}

// shadeColor is c made darker (f < 1) or lighter (f > 1).
func shadeColor(c color.Color, f float64) color.Color {
	rc, ok := c.(color.RGBA)
	if !ok {
		return c
	}
	sc := func(v uint8) uint8 { return uint8(min(255, math.Round(float64(v)*f))) }
	return color.RGBA{sc(rc.R), sc(rc.G), sc(rc.B), rc.A}
}

// cloneCanvas is a copy of c's cells.
func cloneCanvas(c *boardCanvas) *boardCanvas {
	out := newBoardCanvas(c.Width(), c.Height())
	for y := range c.Lines {
		copy(out.Lines[y], c.Lines[y])
	}
	return out
}

// ---- Alt, then Space: one more chosen ---------------------------------------------------

// altSpace is Space just after a tap of Alt: the chosen structure (or the
// one under the mouse) joins those chosen together, or leaves them. It
// says whether it was that.
func (m *Model) altSpace() bool {
	d := m.ds()
	if d.altTapAt.IsZero() || time.Since(d.altTapAt) > altSpaceFor || d.editing != partNone {
		return false
	}
	d.altTapAt = time.Time{}
	o := d.byID(d.sel)
	if h := d.byID(d.hover - 1); d.hover > 0 && h != nil && o == nil {
		o = h
	}
	if o == nil {
		m.say("Choose a structure first (the arrows), then tap Alt and press Space.")
		return true
	}
	if d.multi == nil {
		d.multi = map[int]bool{}
	}
	if d.multi[o.ID] {
		delete(d.multi, o.ID)
		m.sayMulti(octName(o) + " left out · ")
	} else {
		d.multi[o.ID] = true
		if len(d.multi) >= 2 {
			m.sayMulti(octName(o) + " added · Alt+A: one name plate for all · ")
		} else {
			m.sayMulti(octName(o) + " added · tap Alt, then Space, on the next · ")
		}
	}
	d.spaceTap, d.spaceSwallow = false, true // the Space let go is not a tap of its own
	d.dirty = true
	return true
}

// moveBoardDir moves a board's folder (its board.json and what is kept
// next to it) from the folder of src to that of dst.
func moveBoardDir(src, dst string) {
	from, to := filepath.Dir(src), filepath.Dir(dst)
	if _, err := os.Stat(from); err != nil {
		return // never written: nothing to move
	}
	_ = os.RemoveAll(to) // (empty: the walls after move up from the last)
	_ = os.MkdirAll(filepath.Dir(to), 0o755)
	_ = os.Rename(from, to)
}

// floorBoards calls plank for the lines of a floor's boards on a w×h
// screen: a faint line between each row of planks and a seam where one
// plank ends, staggered from row to row, all fixed to the floor as the
// view moves.
func (d *designState) floorBoards(w, h int, plank func(x, y int, mk boardMark)) {
	z := d.z()
	rowH, plankW := 4.0, 28.0
	line := mixHex(cDim, boardBG(), 0.7)
	seam := mixHex(cDim, boardBG(), 0.55)
	cross := func(s int, cam, step float64) (bool, int) {
		a := math.Floor((cam + float64(s)/z) / step)
		b := math.Floor((cam + float64(s+1)/z) / step)
		return b > a, int(b)
	}
	for y := range h {
		edge, row := cross(y, d.camY, rowH)
		if edge {
			for x := range w {
				plank(x, y, boardMark{"─", line})
			}
			continue
		}
		wy := d.camY + (float64(y)+0.5)/z
		r := int(math.Floor(wy / rowH))
		_ = row
		off := float64((r*11)%4) * plankW / 4 // each row's seams fall elsewhere
		for x := range w {
			if ok, _ := cross(x, d.camX+off, plankW); ok {
				plank(x, y, boardMark{"│", seam})
			}
		}
	}
}
