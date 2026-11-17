package main

// ---- a door, with a window into its room --------------------------------------------------
//
// A door (or stairs) to another room (design_rooms.go) is drawn as a door:
// a frame, a name plate with the room's name, a glass window, a handle and
// a panel below. Through the window you see the room behind it, small:
// every structure in it as a block of its own colour where it stands,
// against the room's wall, with its floor along the bottom. Stairs have
// steps in the panel and an arrow on the plate.
//
// The window is drawn in half blocks (▀): each cell is two dots, one above
// the other, so the room shows at twice the rows. In 3D the door's front is
// this drawing. While its name is being written it
// shows as a plain card, with the cursor (paperLayers).

import (
	"fmt"
	"math"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

const (
	doorGlass = "#9fc8e8" // the window's tint
	doorBrass = "#c9a227" // the plate and the handle
)

// peekRoom is the board of room id, for a door's window (nil: none).
// The Model sets it (m.ds): it knows the rooms.
func (d *designState) peekRoom(id int) *designState {
	if d.peek == nil || id == 0 {
		return nil
	}
	return d.peek(id)
}

// roomPeek is m's way of looking into room id: the room you are in is the
// board itself, a room been in or made is kept, any other is read from disk
// (once).
func (m *Model) roomPeek(id int) *designState {
	h := m.hs()
	if h.room(id) == nil {
		return nil
	}
	if id == h.Current {
		return m.design
	}
	return m.boardOf(id, 0) // its first wall
}

// doorKey is what a door's drawing depends on beyond what every
// structure's does: what stands where in the room behind it.
func (d *designState) doorKey(o *dOct) string {
	r := d.peekRoom(doorTo(o))
	if r == nil {
		return "none"
	}
	var sb strings.Builder
	for _, it := range r.octs {
		if it != nil {
			fmt.Fprintf(&sb, "%d,%d,%d,%d,%s;", it.X, it.Y, it.W, it.H, it.Color)
		}
	}
	return sb.String()
}

// doorLayers draws door (or stairs) o.
func (d *designState) doorLayers(o *dOct) []*lipgloss.Layer {
	x, y, w, h := d.rect(o)
	selected := d.sel == o.ID
	if w < 10 || h < 8 { // too small (zoomed far out): a block of its colour with its name
		return []*lipgloss.Layer{pillLayer(x, y, w, h, ansi.Truncate(kindIcons[o.Kind]+o.Header, w, ""), o.Color)}
	}
	wood := o.Color
	frame := mixHex(wood, "#000000", 0.45)
	if o.Lexicon != "" { // a lexicon door: leather in its codex's colour, gilt (codex_design.go)
		frame = codexGold()
	}
	if selected {
		frame = cAccent
	}
	panel := wood
	groove := mixHex(wood, "#000000", 0.25)
	c := newBoardCanvas(w, h)
	set := func(cx, cy int, r, fg, bg string, bold bool) {
		if cx < 0 || cy < 0 || cx >= w || cy >= h {
			return
		}
		stl := uv.Style{Fg: rgbaOf(fg), Bg: rgbaOf(bg)}
		if bold {
			stl.Attrs = uv.AttrBold
		}
		c.Lines[cy][cx] = uv.Cell{Content: r, Width: 1, Style: stl}
	}
	text := func(cy int, s, fg, bg string, bold bool) { // s in the middle of row cy, inside the frame
		s = ansi.Truncate(s, w-4, "…")
		cx := (w - ansi.StringWidth(s)) / 2
		for _, r := range s {
			set(cx, cy, string(r), fg, bg, bold)
			cx++
		}
	}

	// The panel, and the frame round it.
	for cy := 0; cy < h; cy++ {
		for cx := 0; cx < w; cx++ {
			set(cx, cy, " ", cText, panel, false)
		}
	}
	for cx := 1; cx < w-1; cx++ {
		set(cx, 0, "━", frame, panel, false)
		set(cx, h-1, "━", frame, panel, false)
	}
	for cy := 1; cy < h-1; cy++ {
		set(0, cy, "┃", frame, panel, false)
		set(w-1, cy, "┃", frame, panel, false)
	}
	set(0, 0, "┏", frame, panel, false)
	set(w-1, 0, "┓", frame, panel, false)
	set(0, h-1, "┗", frame, panel, false)
	set(w-1, h-1, "┛", frame, panel, false)

	if p := d.doorOpenness(o); p > 0 { // swinging open: the room in the doorway (going through)
		doorway(set, d.peekRoom(doorTo(o)), w, h, p, wood, frame)
		l := lipgloss.NewLayer("").X(x).Y(y)
		d.preCells(l, c)
		return []*lipgloss.Layer{l}
	}

	// The name plate: brass, the room's name.
	name := strings.TrimSpace(o.Header)
	if name == "" {
		name = kindNames[o.Kind]
	}
	to := d.peekRoom(doorTo(o))
	if o.Kind == kStairs {
		if strings.Contains(o.Body, "down") {
			name = "▼ " + name
		} else {
			name = "▲ " + name
		}
	}
	plate := " " + ansi.Truncate(name, w-6, "…") + " "
	text(1, plate, "#2a1d10", doorBrass, true)

	// The window: glass in a thin frame, the room seen through it.
	wx0, wx1 := 2, w-3 // inside columns of its frame
	wy0 := 3
	winRows := max(2, (h-4)*58/100)
	wy1 := min(h-4, wy0+winRows) // the frame's bottom row
	for cx := wx0; cx <= wx1; cx++ {
		set(cx, wy0, "▁", groove, panel, false)
		set(cx, wy1, "▔", groove, panel, false)
	}
	for cy := wy0 + 1; cy < wy1; cy++ {
		set(wx0, cy, "▕", groove, panel, false)
		set(wx1, cy, "▏", groove, panel, false)
	}
	gw, gh := wx1-wx0-1, wy1-wy0-1 // the glass, in cells
	if gw > 0 && gh > 0 {
		dots := roomDots(to, gw, gh*2)
		for gy := 0; gy < gh; gy++ {
			for gx := 0; gx < gw; gx++ {
				top, bot := dots[2*gy][gx], dots[2*gy+1][gx]
				set(wx0+1+gx, wy0+1+gy, "▀", mixHex(top, doorGlass, 0.12), mixHex(bot, doorGlass, 0.12), false)
			}
		}
	}

	// The handle, or for stairs a few steps.
	hy := min(h-3, wy1+2)
	if o.Kind == kStairs {
		steps := []string{"▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"}
		down := strings.Contains(o.Body, "down")
		n := min(len(steps), w-4)
		for i := 0; i < n; i++ {
			s := steps[i]
			if down {
				s = steps[n-1-i]
			}
			set((w-n)/2+i, h-3, s, groove, panel, false)
		}
	} else if o.Lexicon != "" {
		set(w-3, hy, "▥", codexGold(), panel, true) // a book for a handle
	} else {
		set(w-3, hy, "●", doorBrass, panel, true)
		set(w-4, hy, "━", doorBrass, panel, false)
	}

	// A panel under the window.
	py0, py1 := hy+1, h-2
	if o.Kind == kDoor && py1-py0 >= 2 && w >= 12 {
		for cx := 3; cx < w-3; cx++ {
			set(cx, py0, "─", groove, panel, false)
			set(cx, py1, "─", groove, panel, false)
		}
		for cy := py0; cy <= py1; cy++ {
			set(2, cy, "│", groove, panel, false)
			set(w-3, cy, "│", groove, panel, false)
		}
		set(2, py0, "┌", groove, panel, false)
		set(w-3, py0, "┐", groove, panel, false)
		set(2, py1, "└", groove, panel, false)
		set(w-3, py1, "┘", groove, panel, false)
	}

	// Its floor, on the bottom of the frame.
	if f := strings.TrimSpace(o.Footer); f != "" {
		text(h-1, " "+f+" ", mixHex(wood, cText, 0.7), frame, false)
	}

	l := lipgloss.NewLayer("").X(x).Y(y) // its cells are made already
	d.preCells(l, c)
	return d.octExtras([]*lipgloss.Layer{l}, o, x, y, w, h, frame, panel, selected, d.linkMode && d.linkFrom == o.ID)
}

// roomDots is room r seen small, w dots across and h down (a board cell is
// one dot across and two down): its wall, its floor along the bottom, and
// every structure as a block of its colour with a darker edge, the whole
// room fitted in and in the middle.
func roomDots(r *designState, w, h int) [][]string {
	wall := mixHex(boardBG(), cText, 0.1)
	floor := mixHex("#6b4a2e", boardBG(), 0.35)
	g := make([][]string, h)
	floorAt := h - max(1, h/6)
	for y := range g {
		g[y] = make([]string, w)
		for x := range g[y] {
			if y >= floorAt {
				g[y][x] = floor
			} else {
				g[y][x] = wall
			}
		}
	}
	if r == nil {
		return g
	}
	// The room's extent, in board cells.
	x0, y0, x1, y1, any := 0, 0, 0, 0, false
	for _, o := range r.octs {
		if o == nil {
			continue
		}
		if !any {
			x0, y0, x1, y1, any = o.X, o.Y, o.X+o.W, o.Y+o.H, true
			continue
		}
		x0, y0, x1, y1 = min(x0, o.X), min(y0, o.Y), max(x1, o.X+o.W), max(y1, o.Y+o.H)
	}
	if !any {
		return g
	}
	// Fit it on the wall above the floor, a dot clear all round.
	aw, ah := float64(max(1, w-2)), float64(max(1, floorAt-2))
	bw, bh := float64(max(1, x1-x0)), float64(max(1, y1-y0))*2 // a board cell is two dots tall
	s := min(aw/bw, ah/bh)
	ox := 1 + (aw-bw*s)/2
	oy := 1 + (ah-bh*s)/2
	for _, o := range r.octs {
		if o == nil {
			continue
		}
		fx0 := ox + float64(o.X-x0)*s
		fy0 := oy + float64(o.Y-y0)*2*s
		fx1 := fx0 + max(1, float64(o.W)*s)
		fy1 := fy0 + max(1, float64(o.H)*2*s)
		ix0, iy0 := int(fx0), int(fy0)
		ix1, iy1 := max(ix0+1, int(fx1+0.5)), max(iy0+1, int(fy1+0.5))
		fill := mixHex(wall, o.Color, 0.85)
		edge := mixHex(o.Color, "#000000", 0.35)
		for y := iy0; y < iy1 && y < floorAt; y++ {
			for x := ix0; x < ix1 && x < w; x++ {
				if x < 0 || y < 0 {
					continue
				}
				if (x == ix0 || x == ix1-1 || y == iy0 || y == iy1-1) && ix1-ix0 > 2 && iy1-iy0 > 2 {
					g[y][x] = edge
				} else {
					g[y][x] = fill
				}
			}
		}
	}
	return g
}

// ---- going through: the door swings open, and in you go ------------------------------------
//
// Enter or Space on a door (goThrough) does not jump: the door swings open
// on its left hinge (doorSwingFor), showing the room behind it in the whole
// doorway, and then you are in that room. On the flat board the view then
// zooms into the doorway (doorZoomFor), and in the next room it zooms back
// out from the door you came through (doorArriveFor). In 3D the camera walks
// up to the door as it opens, until the doorway fills the view
// (doorWalkFor). Any key meanwhile goes straight through. With designAnims
// off (tests) it is at once.

const (
	doorSwingFor  = 420 * time.Millisecond
	doorZoomFor   = 420 * time.Millisecond                           // flat: then zooming into the doorway
	doorFlatFor   = doorSwingFor + doorZoomFor + 80*time.Millisecond // and a moment there
	doorArriveFor = 380 * time.Millisecond                           // flat: zooming out from the door in the next room
	doorSwingDeg  = 78                                               // how far it swings open
)

// doorThrough is a door being gone through.
type doorThrough struct {
	id, to int
	start  time.Time
	zoom0  int // the zoom and view before (put back as you leave, so the room is kept as it was)
	cam0   [2]float64
}

// doorArrive is the view zooming out from the door you came in by.
type doorArrive struct {
	id, zoom int // the door, and the zoom it ends at
	start    time.Time
}

// doorOpenness is how far door o has swung open: 0 shut, 1 open.
func (d *designState) doorOpenness(o *dOct) float64 {
	t := d.through
	if t == nil || t.id != o.ID {
		return 0
	}
	return openEase(float64(animNow().Sub(t.start)) / float64(doorSwingFor))
}

// startThrough starts going through door o to room to: true when it
// moves, false when it should be at once.
func (m *Model) startThrough(o *dOct, to int) bool {
	d := m.ds()
	if !designAnims {
		return false
	}
	t := &doorThrough{id: o.ID, to: to, start: animNow(), zoom0: d.zoom, cam0: [2]float64{d.camX, d.camY}}
	d.through = t
	d.editing = partNone
	d.frameAt = time.Time{}
	return true
}

// throughStep moves a door being gone through on a frame; at the end it is
// the next room.
func (m *Model) throughStep() {
	d := m.design
	if d == nil {
		return
	}
	if d.arrive != nil {
		m.arriveStep()
	}
	if d.through == nil {
		return
	}
	t := d.through
	el := animNow().Sub(t.start)
	end := doorFlatFor
	if el > doorSwingFor { // flat: into the doorway, a zoom at a time, the door kept in the middle
		k := min(1, float64(el-doorSwingFor)/float64(doorZoomFor))
		top := len(designZooms) - 1
		if i := t.zoom0 + int(math.Round(k*float64(max(0, top-t.zoom0)))); i != d.zoom {
			d.zoomTo(i, m.width/2, m.boardH()/2)
		}
		if o := d.byID(t.id); o != nil {
			m.centreOn(o)
		}
	}
	if el >= end {
		m.finishThrough()
	}
}

// finishThrough is in the next room now.
func (m *Model) finishThrough() {
	d := m.design
	if d == nil || d.through == nil {
		return
	}
	t := d.through
	d.through = nil
	d.zoom, d.camX, d.camY, d.dirty = t.zoom0, t.cam0[0], t.cam0[1], true // the room's own view back, as it is left
	m.goRoom(t.to)
	if designAnims {
		m.startArrive()
	}
}

// startArrive starts the view in the room just come into close on the door
// you came in by, zooming out to the room's own zoom.
func (m *Model) startArrive() {
	d := m.ds()
	o := d.byID(d.sel)
	if o == nil || d.zoom >= len(designZooms)-1 {
		return
	}
	d.arrive = &doorArrive{id: o.ID, zoom: d.zoom, start: animNow()}
	d.zoomTo(len(designZooms)-1, m.width/2, m.boardH()/2)
	m.centreOn(o)
}

// arriveStep zooms out a step on a frame; finishArrive is the end of it.
func (m *Model) arriveStep() {
	d := m.ds()
	a := d.arrive
	k := float64(animNow().Sub(a.start)) / float64(doorArriveFor)
	if k >= 1 {
		m.finishArrive()
		return
	}
	top := len(designZooms) - 1
	if i := top - int(math.Round(k*float64(top-a.zoom))); i != d.zoom {
		d.zoomTo(i, m.width/2, m.boardH()/2)
	}
	if o := d.byID(a.id); o != nil {
		m.centreOn(o)
	}
}

func (m *Model) finishArrive() {
	d := m.ds()
	a := d.arrive
	if a == nil {
		return
	}
	d.arrive = nil
	d.zoomTo(a.zoom, m.width/2, m.boardH()/2)
	if o := d.byID(a.id); o != nil {
		m.centreOn(o)
	}
}

// doorway draws a door swung open p (0 … 1): the room behind it in the
// whole doorway, and the door itself, turned on its left hinge, narrower.
func doorway(set func(cx, cy int, r, fg, bg string, bold bool), to *designState, w, h int, p float64, wood, frame string) {
	iw, ih := w-2, h-2
	dots := roomDots(to, iw, ih*2)
	for gy := 0; gy < ih; gy++ {
		for gx := 0; gx < iw; gx++ {
			set(1+gx, 1+gy, "▀", dots[2*gy][gx], dots[2*gy+1][gx], false)
		}
	}
	pw := int(math.Round(float64(iw) * math.Cos(p*doorSwingDeg*math.Pi/180)))
	if pw <= 0 {
		return
	}
	lit := mixHex(wood, "#000000", 0.15*p) // turning away from the light
	edge := mixHex(wood, "#000000", 0.4)
	for cy := 1; cy <= ih; cy++ {
		for cx := 1; cx <= pw; cx++ {
			set(cx, cy, " ", cText, lit, false)
		}
		set(pw, cy, "▐", edge, lit, false) // its edge, facing you
	}
	if pw >= 3 {
		set(pw-1, 1+ih/2, "●", doorBrass, lit, true)
	}
	_ = frame
}
