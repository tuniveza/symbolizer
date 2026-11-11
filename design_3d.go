package main

// ---- the board as a room in 3D ------------------------------------------------------
//
// Ctrl + Left / Right turn the board's room, Ctrl + Up / Down tilt it, 5° a
// press (held: it keeps turning). Turned away from straight on, the board
// area shows the board as a room: the board is the back wall, with a floor
// and two side walls, and every structure on the screen comes out of the
// wall as a block. Paper (pages, notes, slides, drawings) is thin, a book a
// little thicker, a board shallow, a shape deeper, and cabinets, drawer
// units and racks deepest. The front of each block is its own drawing from
// the flat board; its sides are its colour. A lamp hangs at the front left
// and lights it all, and the blocks cast soft shadows on the back wall.
//
// It is drawn by a small renderer of its own: every pixel finds the nearest
// surface (a depth buffer), which is lit there; two pixels make one cell of
// the screen with "▀" (the upper pixel is the letter's colour, the lower
// one the background). Close to straight on, the letters of the flat board
// show on the wall and on the fronts of the blocks, so it stays readable.
//
// Straight on again (or Esc), it is the flat board, exactly as before. The
// turn is only a way of looking: it is not an undo step and is not saved.

import (
	"image/color"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

const (
	turnStep     = 5                      // degrees a press
	turnYawMax   = 80                     // the room has no front wall: never seen from behind
	turnPitchMax = 75                     // tilted down: seen from above
	turnPitchUp  = 45                     // tilted up: seen from below, where the room has no ceiling to look at
	turnGlide    = 2                      // degrees a glide step, held
	turnStart    = 160 * time.Millisecond // held: the first glide step (a quick tap is one step)
	turnEvery    = 30 * time.Millisecond  // then 33 steps a second: about 65° a second
	roomNearMin  = -6                     // + and -: the camera's steps closer (or further)
	roomNearMax  = 10
	roomHint     = "3D view  ·  Ctrl + arrows: turn  ·  + −: closer, further  ·  arrows: move  ·  Esc: flat board"
)

// room3D is the turned view and what its drawing keeps from frame to frame.
type room3D struct {
	yaw   int // degrees: the room turned right (+) or left (−)
	pitch int // degrees: its front tilted down (+, the floor shows) or up (−)
	near  int // camera steps closer (+) or further (−)

	pw, ph int       // the pixels: the board area's width, and twice its height
	base   []uint32  // each pixel's colour before the light
	mul    []float32 // the light on it: base times mul, plus add of the lamp's colour
	add    []float32
	zb     []float32 // the depth of the nearest surface
	face   []int32   // which face it shows (-1: none)
	src    []int32   // the flat board cell it shows (wall and fronts), or -1
	dirs   []r3vec   // each pixel's ray
	tex    []r3texel // the flat board's cells
	shadow []float32 // the blocks' shadows on the back wall, per wall pixel
	glyph  []int32   // per screen cell: the board cell whose letter shows there, or -1
	placed []int32   // per board cell: the frame (stamp) its letter was put on the screen
	claim  []int32   // per screen cell: the board cell that took it (letters), or -1
	stamp  int32
	faces  []r3face
	pts    []r3vec
	hexes  map[string]uint32 // colours read from their "#rrggbb"

	frame          string // the last drawing, for frames where only the mouse moved
	frameW, frameH int
	cost           time.Duration // how long the last drawing took
}

// on is whether the room is turned (the board area shows it in 3D).
func (r *room3D) on() bool { return r.yaw != 0 || r.pitch != 0 }

// ---- keys and mouse --------------------------------------------------------------------

// design3DKey handles the keys of the 3D view: Ctrl + arrows at any time,
// and while turned every other key (Esc: flat again; + −: closer, further;
// the arrows move the board; anything else only shows the hint). It says
// whether it took the key.
func (m *Model) design3DKey(k tea.Key, name string) (bool, tea.Cmd) {
	d := m.ds()
	r := &d.room
	switch name {
	case "ctrl+left", "ctrl+right", "ctrl+up", "ctrl+down":
		d.editing = partNone // writing stops, as with any arrow
		dir := arrowDir(name)
		if m.releases {
			// kitty says when the key is let go: a glide turns while it is
			// held, and the terminal's own repeats only say it still is.
			m.chord.held[dir] = true
			d.heldAt = time.Now()
			if k.IsRepeat {
				return true, nil
			}
			m.design3DTurn(dir, false)
			d.holdMods = "ctrl+"
			if d.holdTick {
				return true, nil
			}
			d.holdTick = true
			return true, tea.Tick(turnStart, func(time.Time) tea.Msg { return designHoldMsg{} })
		}
		m.design3DTurn(dir, false) // other terminals: each press and repeat turns a step
		return true, nil
	}
	if !r.on() {
		return false, nil
	}
	switch name {
	case "esc":
		r.yaw, r.pitch = 0, 0
		m.say("Flat board.")
	case "f1":
		m.help = true
	case "+", "=", "ctrl+=", "shift+=", "plus", "kp+":
		m.design3DNear(1)
	case "-", "ctrl+-", "minus", "kp-":
		m.design3DNear(-1)
	case "0":
		r.near = 0
		m.say("3D view: the camera where it starts.")
	case "left", "right", "up", "down", "shift+left", "shift+right", "shift+up", "shift+down":
		if d.holdTick && d.holdMods == "ctrl+" {
			return true, nil // the arrow of a turn being held (Ctrl let go first): not a move
		}
		dx, dy, step := arrowStep(name)
		m.designPan(dx, dy, step)
		m.msg = ""
	default:
		m.say("That key does nothing in the 3D view.  Esc: flat board.")
	}
	return true, nil
}

// design3DTurn turns the room that way (Ctrl + that arrow): a press to
// the next multiple of 5°, a glide step (held) by turnGlide°.
func (m *Model) design3DTurn(dir string, glide bool) {
	r := &m.ds().room
	by := func(a, sign int) int {
		if glide {
			return a + sign*turnGlide
		}
		f := int(math.Floor(float64(a) / turnStep))
		if sign > 0 {
			return (f + 1) * turnStep // the next multiple of 5 up
		}
		if f*turnStep == a {
			return a - turnStep
		}
		return f * turnStep // the next one down: 3° and Left is 0°, not −2°
	}
	switch dir {
	case "left":
		r.yaw = by(r.yaw, -1)
	case "right":
		r.yaw = by(r.yaw, 1)
	case "up":
		r.pitch = by(r.pitch, -1)
	case "down":
		r.pitch = by(r.pitch, 1)
	}
	r.yaw = max(-turnYawMax, min(turnYawMax, r.yaw))
	r.pitch = max(-turnPitchUp, min(turnPitchMax, r.pitch))
	if !r.on() {
		m.say("Flat board.")
		return
	}
	m.say(roomAngles(r.yaw, r.pitch))
}

// roomAngles says how the room is turned, in words.
func roomAngles(yaw, pitch int) string {
	way := func(n int, plus, minus string) string {
		if n < 0 {
			return strconv.Itoa(-n) + "° " + minus
		}
		return strconv.Itoa(n) + "° " + plus
	}
	return "3D view: the room turned " + way(yaw, "right", "left") + ", tilted " + way(pitch, "down", "up") + ".  Esc: flat board."
}

// design3DNear moves the camera closer (step 1) or further (-1).
func (m *Model) design3DNear(step int) {
	r := &m.ds().room
	r.near = max(roomNearMin, min(roomNearMax, r.near+step))
	switch {
	case r.near > 0:
		m.say("3D view: the camera " + plural(r.near, "step") + " closer.")
	case r.near < 0:
		m.say("3D view: the camera " + plural(-r.near, "step") + " further away.")
	default:
		m.say("3D view: the camera where it starts.")
	}
}

// design3DHold is a glide step while Ctrl + arrows are held (designHold
// sends it here), or its end when they are let go.
func (m *Model) design3DHold() tea.Cmd {
	d := m.design
	any := false
	if m.inDesign() && !m.help && time.Since(d.heldAt) <= holdLost {
		for _, dir := range []string{"left", "right", "up", "down"} {
			if m.chord.held[dir] {
				m.design3DTurn(dir, true)
				any = true
			}
		}
	}
	if !any {
		d.holdTick, d.holdMods = false, ""
		for dir := range m.chord.held {
			m.chord.held[dir] = false
		}
		return nil
	}
	return tea.Tick(turnEvery, func(time.Time) tea.Msg { return designHoldMsg{} })
}

// design3DMouse: in the 3D view a click does nothing to the board (it only
// shows the hint) and the wheel moves the camera. It says whether it took it.
func (m *Model) design3DMouse(ms tea.Mouse) (bool, tea.Cmd) {
	if !m.ds().room.on() {
		return false, nil
	}
	switch ms.Button {
	case tea.MouseWheelUp:
		m.design3DNear(1)
	case tea.MouseWheelDown:
		m.design3DNear(-1)
	default:
		m.say(roomHint)
	}
	return true, nil
}

// design3DRows draws the room in the board area: m.boardH() lines, each
// m.width cells wide and ending in "\n".
func (m *Model) design3DRows(sb *strings.Builder) {
	d := m.ds()
	r := &d.room
	w, h := m.width, m.boardH()
	if d.hoverOnly && r.frame != "" && r.frameW == w && r.frameH == h {
		d.hoverOnly = false // only the mouse moved: the room has not changed
		sb.WriteString(r.frame)
		return
	}
	d.hoverOnly = false
	d.frame = "" // the flat board's last drawing is old now: it is drawn afresh when flat again
	start := time.Now()
	// The flat board, which the wall and the fronts show, without what
	// floats over it (the mini map, the list of name plates).
	corner, plates := d.mapCorner, d.platesOpen
	d.mapCorner, d.platesOpen = 0, false
	c := m.designCanvas()
	d.mapCorner, d.platesOpen = corner, plates
	var fb strings.Builder
	r.draw(d, c, w, h, &fb)
	r.frame, r.frameW, r.frameH = fb.String(), w, h
	r.cost = time.Since(start)
	sb.WriteString(r.frame)
}

// ---- small sums ----------------------------------------------------------------------

type r3vec struct{ x, y, z float64 }

func (a r3vec) add(b r3vec) r3vec   { return r3vec{a.x + b.x, a.y + b.y, a.z + b.z} }
func (a r3vec) sub(b r3vec) r3vec   { return r3vec{a.x - b.x, a.y - b.y, a.z - b.z} }
func (a r3vec) mul(k float64) r3vec { return r3vec{a.x * k, a.y * k, a.z * k} }
func (a r3vec) dot(b r3vec) float64 { return a.x*b.x + a.y*b.y + a.z*b.z }
func (a r3vec) cross(b r3vec) r3vec {
	return r3vec{a.y*b.z - a.z*b.y, a.z*b.x - a.x*b.z, a.x*b.y - a.y*b.x}
}
func (a r3vec) norm() r3vec            { return a.mul(1 / math.Sqrt(a.dot(a))) }
func clampF(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }
func hex32(h string) uint32            { r, g, b := rgb(h); return uint32(r)<<16 | uint32(g)<<8 | uint32(b) }
func rgb32(r, g, b float64) uint32     { return uint32(r)<<16 | uint32(g)<<8 | uint32(b) }
func chans(c uint32) (r, g, b float64) {
	return float64(c >> 16 & 255), float64(c >> 8 & 255), float64(c & 255)
}

// mix32 is a colour t of the way from a to b.
func mix32(a, b uint32, t float64) uint32 {
	ar, ag, ab := chans(a)
	br, bg, bb := chans(b)
	return rgb32(ar+(br-ar)*t+0.5, ag+(bg-ag)*t+0.5, ab+(bb-ab)*t+0.5)
}

// colour32 is a cell's colour as 0xRRGGBB (or, when it has none, or).
func colour32(c color.Color, or uint32) uint32 {
	switch v := c.(type) {
	case nil:
		return or
	case color.RGBA:
		return uint32(v.R)<<16 | uint32(v.G)<<8 | uint32(v.B)
	}
	r, g, b, _ := c.RGBA()
	return (r>>8)<<16 | (g>>8)<<8 | b>>8
}

// hex is hex32, remembered: thousands of blocks have only a few colours.
func (r *room3D) hex(h string) uint32 {
	c, ok := r.hexes[h]
	if !ok {
		if r.hexes == nil || len(r.hexes) > 4096 {
			r.hexes = map[string]uint32{}
		}
		c = hex32(h)
		r.hexes[h] = c
	}
	return c
}

// ---- the flat board as a picture ----------------------------------------------------------

// r3texel is one cell of the flat board: its two pixels, its colours, and
// its letter when that can show as it is.
type r3texel struct {
	top, bot uint32
	fg, bg   uint32
	glyph    string
	blank    bool // a space
}

// takeBoard reads the flat board's cells.
func (r *room3D) takeBoard(c *boardCanvas) {
	w, h := c.Width(), c.Height()
	if len(r.tex) != w*h {
		r.tex = make([]r3texel, w*h)
		r.placed = make([]int32, w*h)
	}
	defFG, defBG := hex32(cText), hex32(cBG)
	for y := 0; y < h; y++ {
		line := c.Lines[y]
		for x := 0; x < w; x++ {
			t := r3texel{fg: defFG, bg: defBG, top: defBG, bot: defBG}
			if x < len(line) {
				cell := &line[x]
				t.fg, t.bg = colour32(cell.Style.Fg, defFG), colour32(cell.Style.Bg, defBG)
				if cell.Style.Attrs&uv.AttrReverse != 0 {
					t.fg, t.bg = t.bg, t.fg
				}
				switch s := cell.Content; s {
				case "", " ":
					t.top, t.bot, t.blank = t.bg, t.bg, true
				case "▀":
					t.top, t.bot = t.fg, t.bg
				case "▄":
					t.top, t.bot = t.bg, t.fg
				case "█":
					t.top, t.bot = t.fg, t.fg
				case "▌", "▐", "▒":
					t.top = mix32(t.bg, t.fg, 0.5)
					t.bot = t.top
				case "░":
					t.top = mix32(t.bg, t.fg, 0.25)
					t.bot = t.top
				case "▓":
					t.top = mix32(t.bg, t.fg, 0.75)
					t.bot = t.top
				default:
					// A letter or a line: seen from afar, a tint of its colour.
					k := 0.35
					if ru, _ := utf8.DecodeRuneInString(s); ru >= 0x2500 && ru <= 0x257f {
						k = 0.6 // box lines: the edges of things
					}
					t.top = mix32(t.bg, t.fg, k)
					t.bot = t.top
					if cell.Width == 1 && ansi.StringWidth(s) == 1 {
						t.glyph = s
					}
				}
			}
			r.tex[y*w+x] = t
		}
	}
}

// ---- the scene ------------------------------------------------------------------------

// What a face is made of.
const (
	matBoard = iota // the back wall: the flat board
	matFront        // a block's front: its part of the flat board
	matSolid        // one colour (a block's sides)
	matFloor        // planks
	matWall         // a side wall, with a skirting board
	matLamp         // the bulb: it shines, it is not lit
	matShade        // the lamp's shade, lit from inside
)

// r3face is one flat, convex face.
type r3face struct {
	p0, pn int // its corners: pts[p0 : p0+pn]
	n      r3vec
	k      float64 // its plane: n · p = k
	mat    int
	col    uint32
	gloss  float64
	o      r3vec // for the edge lines of a block's side: a corner, two edge directions and lengths
	eu, ev r3vec
	lu, lv float64
	edge   bool
	both   bool // seen from either side (the shade)
	// matFront: the board cells it shows (columns cx0 ≤ x < cx1, pixel rows py0 ≤ y < py1)
	cx0, cx1, py0, py1 int
	seen               bool // drawn this frame (it faced the camera)
}

// depthOf is how far a kind of structure comes out of the wall, at 100%.
func depthOf(kind string) float64 {
	switch kind {
	case kPage, kNote, kSlide, kDrawing:
		return 1
	case kBook:
		return 2.5
	case kBoard:
		return 2
	case kOct:
		return 5
	case kCabinet, kDrawer, kRack:
		return 12
	case kBox: // a box is deep: things lie in it
		return 10
	}
	return 3
}

func (r *room3D) addFace(f r3face, pts ...r3vec) {
	f.p0, f.pn = len(r.pts), len(pts)
	r.pts = append(r.pts, pts...)
	f.k = f.n.dot(pts[0])
	r.faces = append(r.faces, f)
}

// quadFace adds the rectangle with corner o and edges u and v, facing n.
func (r *room3D) quadFace(f r3face, o, u, v r3vec) {
	f.o, f.lu, f.lv = o, math.Sqrt(u.dot(u)), math.Sqrt(v.dot(v))
	if f.lu > 0 && f.lv > 0 {
		f.eu, f.ev = u.mul(1/f.lu), v.mul(1/f.lv)
	}
	r.addFace(f, o, o.add(u), o.add(u).add(v), o.add(v))
}

// scene puts the room, the blocks and the lamp in faces. The world's units
// are pixels: x right, y up, z out of the back wall toward the viewer; the
// back wall is the board area, centred on (0, 0, 0).
func (r *room3D) scene(d *designState, w, h int, depth float64, lamp r3vec) {
	r.faces, r.pts = r.faces[:0], r.pts[:0]
	hw, hh := float64(w)/2, float64(h) // the board is w cells by 2h pixels
	bg, text, accent := hex32(cBG), hex32(cText), hex32(cAccent)

	// The room.
	r.quadFace(r3face{n: r3vec{0, 0, 1}, mat: matBoard, cx0: 0, cx1: w, py0: 0, py1: 2 * h, gloss: 0.15},
		r3vec{-hw, -hh, 0}, r3vec{2 * hw, 0, 0}, r3vec{0, 2 * hh, 0})
	r.quadFace(r3face{n: r3vec{0, 1, 0}, mat: matFloor, col: mix32(mix32(bg, text, 0.16), accent, 0.06), gloss: 0.6},
		r3vec{-hw, -hh, 0}, r3vec{2 * hw, 0, 0}, r3vec{0, 0, depth})
	r.quadFace(r3face{n: r3vec{1, 0, 0}, mat: matWall, col: mix32(mix32(bg, text, 0.09), accent, 0.05), gloss: 0.1},
		r3vec{-hw, -hh, 0}, r3vec{0, 2 * hh, 0}, r3vec{0, 0, depth})
	r.quadFace(r3face{n: r3vec{-1, 0, 0}, mat: matWall, col: mix32(bg, text, 0.07), gloss: 0.1},
		r3vec{hw, -hh, 0}, r3vec{0, 2 * hh, 0}, r3vec{0, 0, depth})

	// Every structure on the screen, as a block.
	z := d.z()
	for _, o := range d.octs {
		x, y, ow, oh := d.rect(o)
		x0, y0, x1, y1 := max(x, 0), max(y, 0), min(x+ow, w), min(y+oh, h)
		if x0 >= x1 || y0 >= y1 {
			continue // off the screen: not in the room
		}
		dep := math.Max(0.6, depthOf(o.Kind)*z)
		dep = math.Min(dep, math.Min(0.3*depth, 0.8*float64(max(x1-x0, 2*(y1-y0)))))
		X0, X1 := float64(x0)-hw, float64(x1)-hw
		Yt, Yb := hh-float64(2*y0), hh-float64(2*y1)
		// The front: an octagon for a shape drawn whole as one, else a rectangle.
		front := []r3vec{{X0, Yt, dep}, {X1, Yt, dep}, {X1, Yb, dep}, {X0, Yb, dep}}
		if o.Kind == kOct && x0 == x && y0 == y && x1 == x+ow && y1 == y+oh && ow >= 6 && oh >= 3 {
			cx, cy := 2.0, 4.0 // its corners, cut as the flat drawing cuts them
			if oh < octMinH || ow < octMinW {
				cx, cy = 1, 2
			}
			front = []r3vec{{X0 + cx, Yt, dep}, {X1 - cx, Yt, dep}, {X1, Yt - cy, dep}, {X1, Yb + cy, dep},
				{X1 - cx, Yb, dep}, {X0 + cx, Yb, dep}, {X0, Yb + cy, dep}, {X0, Yt - cy, dep}}
		}
		r.addFace(r3face{n: r3vec{0, 0, 1}, mat: matFront, cx0: x0, cx1: x1, py0: 2 * y0, py1: 2 * y1, gloss: 0.3}, front...)
		col := text
		if o.Color != "" {
			col = r.hex(o.Color)
		}
		mid := r3vec{(X0 + X1) / 2, (Yt + Yb) / 2, 0}
		for i := range front {
			a, b := front[i], front[(i+1)%len(front)]
			e := b.sub(a)
			n := r3vec{e.y, -e.x, 0}.norm()
			if n.dot(r3vec{a.x - mid.x, a.y - mid.y, 0}) < 0 {
				n = n.mul(-1)
			}
			a0 := r3vec{a.x, a.y, 0}
			r.quadFace(r3face{n: n, mat: matSolid, col: col, edge: true, gloss: 0.35}, a0, e, r3vec{0, 0, dep})
		}
	}

	// The lamp: a bulb under a shade, hung on a cord from above the walls.
	rb := math.Max(1, 0.012*math.Max(2*hw, 2*hh))
	warm := hex32("#fff4d6")
	for _, sx := range []float64{-1, 1} {
		for _, sy := range []float64{-1, 1} {
			for _, sz := range []float64{-1, 1} {
				a, b, c := lamp.add(r3vec{sx * rb, 0, 0}), lamp.add(r3vec{0, sy * rb, 0}), lamp.add(r3vec{0, 0, sz * rb})
				r.addFace(r3face{n: r3vec{sx, sy, sz}.norm(), mat: matLamp, col: warm}, a, b, c)
			}
		}
	}
	shade := mix32(accent, text, 0.15)
	b0, b1, tp := 2.1*rb, 0.8*rb, lamp.y+0.3*rb
	top := lamp.y + 2*rb
	corners := [4][2]float64{{-1, -1}, {1, -1}, {1, 1}, {-1, 1}}
	for i := range corners {
		c0, c1 := corners[i], corners[(i+1)%4]
		p := []r3vec{
			{lamp.x + c0[0]*b0, tp, lamp.z + c0[1]*b0}, {lamp.x + c1[0]*b0, tp, lamp.z + c1[1]*b0},
			{lamp.x + c1[0]*b1, top, lamp.z + c1[1]*b1}, {lamp.x + c0[0]*b1, top, lamp.z + c0[1]*b1},
		}
		n := p[1].sub(p[0]).cross(p[3].sub(p[0])).norm()
		if n.dot(p[0].sub(lamp)) < 0 {
			n = n.mul(-1) // outward
		}
		r.addFace(r3face{n: n, mat: matShade, col: shade, both: true}, p...)
	}
	cw := math.Max(0.5, rb/4)
	cord := mix32(text, bg, 0.3)
	ceil := hh + 0.25*hh
	for _, s := range []r3vec{{1, 0, 0}, {-1, 0, 0}, {0, 0, 1}, {0, 0, -1}} {
		side := r3vec{s.z, 0, -s.x} // along the face
		o := lamp.add(s.mul(cw)).sub(side.mul(cw))
		o.y = top
		r.quadFace(r3face{n: s, mat: matSolid, col: cord}, o, side.mul(2*cw), r3vec{0, ceil - top, 0})
	}
}

// shadows works out where the blocks' shadows fall on the back wall, as
// seen from the lamp, softened at the edges.
func (r *room3D) shadows(lamp r3vec) {
	w, ph := r.pw, r.ph
	if len(r.shadow) != w*ph {
		r.shadow = make([]float32, w*ph)
	}
	clear(r.shadow)
	hw, hh := float64(w)/2, float64(ph)/2
	var pts [8][2]float64
	for i := range r.faces {
		f := &r.faces[i]
		if f.mat != matFront {
			continue
		}
		// The shadow is the outline of the block's front, seen from the
		// lamp on the wall, joined to the block's own outline on it.
		dep := r.pts[f.p0].z
		if lamp.z-dep < 0.5 {
			continue
		}
		X0, X1 := float64(f.cx0)-hw, float64(f.cx1)-hw
		Yt, Yb := hh-float64(f.py0), hh-float64(f.py1)
		n := 0
		for _, c := range [4][2]float64{{X0, Yt}, {X1, Yt}, {X1, Yb}, {X0, Yb}} {
			pts[n] = [2]float64{c[0] + hw, hh - c[1]}
			k := lamp.z / (lamp.z - dep)
			sx := lamp.x + (c[0]-lamp.x)*k
			sy := lamp.y + (c[1]-lamp.y)*k
			pts[n+4] = [2]float64{sx + hw, hh - sy}
			n++
		}
		fillHull(pts[:], r.shadow, w, ph)
	}
	// Soften: a box blur three pixels wide, across and then down.
	tmp := make([]float32, max(w, ph))
	for y := 0; y < ph; y++ {
		row := r.shadow[y*w : y*w+w]
		for x := 0; x < w; x++ {
			a, c := row[max(x-1, 0)], row[min(x+1, w-1)]
			tmp[x] = (a + row[x] + c) / 3
		}
		copy(row, tmp[:w])
	}
	for x := 0; x < w; x++ {
		for y := 0; y < ph; y++ {
			a, c := r.shadow[max(y-1, 0)*w+x], r.shadow[min(y+1, ph-1)*w+x]
			tmp[y] = (a + r.shadow[y*w+x] + c) / 3
		}
		for y := 0; y < ph; y++ {
			r.shadow[y*w+x] = tmp[y]
		}
	}
}

// fillHull fills the smallest convex shape round pts with 1 in a w×h mask.
func fillHull(pts [][2]float64, mask []float32, w, h int) {
	// The hull, by the monotone chain.
	ps := make([][2]float64, len(pts))
	copy(ps, pts)
	for i := 1; i < len(ps); i++ { // insertion sort: eight points
		for j := i; j > 0 && (ps[j][0] < ps[j-1][0] || ps[j][0] == ps[j-1][0] && ps[j][1] < ps[j-1][1]); j-- {
			ps[j], ps[j-1] = ps[j-1], ps[j]
		}
	}
	cross := func(o, a, b [2]float64) float64 {
		return (a[0]-o[0])*(b[1]-o[1]) - (a[1]-o[1])*(b[0]-o[0])
	}
	hull := make([][2]float64, 0, 2*len(ps))
	for _, p := range ps {
		for len(hull) >= 2 && cross(hull[len(hull)-2], hull[len(hull)-1], p) <= 0 {
			hull = hull[:len(hull)-1]
		}
		hull = append(hull, p)
	}
	lo := len(hull) + 1
	for i := len(ps) - 2; i >= 0; i-- {
		p := ps[i]
		for len(hull) >= lo && cross(hull[len(hull)-2], hull[len(hull)-1], p) <= 0 {
			hull = hull[:len(hull)-1]
		}
		hull = append(hull, p)
	}
	hull = hull[:len(hull)-1]
	if len(hull) < 3 {
		return
	}
	spans(hull, w, h, func(y, x0, x1 int) {
		row := mask[y*w:]
		for x := x0; x <= x1; x++ {
			row[x] = 1
		}
	})
}

// spans calls fill with each row of pixels (whose middles are) inside the
// convex polygon poly, cut to a w×h screen: row y, columns x0 to x1.
func spans(poly [][2]float64, w, h int, fill func(y, x0, x1 int)) {
	ymin, ymax := math.Inf(1), math.Inf(-1)
	for _, p := range poly {
		ymin, ymax = math.Min(ymin, p[1]), math.Max(ymax, p[1])
	}
	ymin, ymax = clampF(ymin, -1, float64(h)+1), clampF(ymax, -1, float64(h)+1)
	j0, j1 := max(0, int(math.Ceil(ymin-0.5))), min(h-1, int(math.Ceil(ymax-0.5))-1)
	for j := j0; j <= j1; j++ {
		yc := float64(j) + 0.5
		xl, xr := math.Inf(1), math.Inf(-1)
		for i := range poly {
			a, b := poly[i], poly[(i+1)%len(poly)]
			if (a[1] <= yc && yc < b[1]) || (b[1] <= yc && yc < a[1]) {
				x := a[0] + (yc-a[1])*(b[0]-a[0])/(b[1]-a[1])
				xl, xr = math.Min(xl, x), math.Max(xr, x)
			}
		}
		if xl > xr {
			continue
		}
		xl, xr = clampF(xl, -1, float64(w)+1), clampF(xr, -1, float64(w)+1)
		i0, i1 := max(0, int(math.Ceil(xl-0.5))), min(w-1, int(math.Ceil(xr-0.5))-1)
		if i0 <= i1 {
			fill(j, i0, i1)
		}
	}
}

// ---- drawing ----------------------------------------------------------------------------

// r3light is the light: what every surface gets, what facing the viewer
// adds, the lamp's share, the lamp's own colour added where it shines
// (0–255), its shine on glossy surfaces, and how light the board is (0–1).
type r3light struct{ ambient, fill, lamp, glow, spec, lightness float64 }

// lightFor is the light for a theme: on a dark board the lamp can be
// strong and glow; on a light one (Daybreak) it is softer, or the room
// would be all white.
func lightFor(bg string) r3light {
	r, g, b := rgb(bg)
	t := clampF((0.3*float64(r)+0.59*float64(g)+0.11*float64(b))/255, 0, 1) // how light the board is
	mixF := func(dark, light float64) float64 { return dark + (light-dark)*t }
	return r3light{mixF(0.30, 0.42), mixF(0.22, 0.14), mixF(1.05, 0.55), mixF(38, 0), mixF(150, 70), t}
}

var lampWarm = [3]float64{1, 0.9, 0.72}

// draw draws the room for a w×h board area into sb.
func (r *room3D) draw(d *designState, c *boardCanvas, w, h int, sb *strings.Builder) {
	if w < 1 || h < 1 {
		for range h {
			sb.WriteByte('\n')
		}
		return
	}
	pw, ph := w, 2*h
	if r.pw != pw || r.ph != ph || len(r.base) != pw*ph {
		r.pw, r.ph = pw, ph
		n := pw * ph
		r.base, r.mul, r.add = make([]uint32, n), make([]float32, n), make([]float32, n)
		r.zb, r.face, r.src, r.dirs = make([]float32, n), make([]int32, n), make([]int32, n), make([]r3vec, n)
		r.glyph = make([]int32, w*h)
	}
	r.takeBoard(c)
	r.stamp++

	fw, fh := float64(pw), float64(ph)
	big := math.Max(fw, fh)
	depth := math.Max(8, 0.45*big)
	dist0 := depth + 1.05*big // the camera's distance from the back wall, straight on
	f := dist0                // so straight on, the back wall is the board area cell for cell
	pivot := r3vec{0, 0, depth / 2}
	rad := dist0 - depth/2
	ya, pa := -float64(r.yaw)*math.Pi/180, float64(r.pitch)*math.Pi/180
	way := r3vec{math.Sin(ya) * math.Cos(pa), math.Sin(pa), math.Cos(ya) * math.Cos(pa)}
	cam := pivot.add(way.mul(rad))
	fwd := pivot.sub(cam).norm()
	right := fwd.cross(r3vec{0, 1, 0}).norm()
	up := right.cross(fwd)
	scale, ox, oy := r.fit(cam, right, up, fwd, f, fw/2, fh/2, depth, math.Max(math.Abs(float64(r.yaw)), math.Abs(float64(r.pitch))))
	f *= scale
	px, py := fw/2+ox, fh/2+oy                                      // the screen's middle, as the camera sees it
	cam = pivot.add(way.mul(rad * math.Pow(0.87, float64(r.near)))) // + and -: closer, further
	// The lamp: front left, near the top, out of sight close to straight on.
	lamp := r3vec{-fw/2 + 0.15*fw, fh/2 - math.Max(3, 0.06*fh), 0.6 * depth}
	invR2 := 1 / (1.15 * big * 1.15 * big)
	light := lightFor(cBG)

	r.scene(d, w, h, depth, lamp)
	r.shadows(lamp)

	// A ray for every pixel, and a clean screen.
	outside := mix32(hex32(cBG), 0, 0.55-0.37*light.lightness) // round the room: darker than the board
	for j := 0; j < ph; j++ {
		dy := -(float64(j) + 0.5 - py) / f
		row := j * pw
		for i := 0; i < pw; i++ {
			dx := (float64(i) + 0.5 - px) / f
			r.dirs[row+i] = r3vec{fwd.x + right.x*dx + up.x*dy, fwd.y + right.y*dx + up.y*dy, fwd.z + right.z*dx + up.z*dy}
		}
	}
	for i := range r.base {
		r.base[i], r.mul[i], r.add[i] = outside, 1, 0
		r.zb[i], r.face[i], r.src[i] = math.MaxFloat32, -1, -1
	}
	for i := range r.glyph {
		r.glyph[i] = -1
	}

	const near = 0.25
	hw, hh := fw/2, fh/2
	var cs, clip [20]r3vec
	var poly [20][2]float64
	for fi := range r.faces {
		fc := &r.faces[fi]
		fc.seen = false
		n, k := fc.n, fc.k
		inside := false // seen from behind (the shade's inside)
		if n.dot(cam)-k <= 1e-9 {
			if !fc.both {
				continue // it faces away
			}
			n, k, inside = n.mul(-1), -k, true
		}
		// Its corners as the camera sees them, cut off in front of the camera.
		pts := r.pts[fc.p0 : fc.p0+fc.pn]
		for i, p := range pts {
			q := p.sub(cam)
			cs[i] = r3vec{q.dot(right), q.dot(up), q.dot(fwd)}
		}
		m := 0
		for i := range pts {
			a, b := cs[i], cs[(i+1)%len(pts)]
			if a.z >= near {
				clip[m] = a
				m++
			}
			if (a.z >= near) != (b.z >= near) {
				t := (near - a.z) / (b.z - a.z)
				clip[m] = a.add(b.sub(a).mul(t))
				m++
			}
		}
		if m < 3 {
			continue
		}
		for i := 0; i < m; i++ {
			poly[i] = [2]float64{px + f*clip[i].x/clip[i].z, py - f*clip[i].y/clip[i].z}
		}
		fc.seen = true
		num := k - n.dot(cam)
		fid := int32(fi)
		spans(poly[:m], pw, ph, func(j, i0, i1 int) {
			row := j * pw
			for i := i0; i <= i1; i++ {
				idx := row + i
				dir := r.dirs[idx]
				nd := n.x*dir.x + n.y*dir.y + n.z*dir.z
				if nd >= -1e-12 {
					continue
				}
				t := num / nd
				if t <= 0 || float32(t) >= r.zb[idx] {
					continue
				}
				p := r3vec{cam.x + dir.x*t, cam.y + dir.y*t, cam.z + dir.z*t}
				src := int32(-1)
				var col uint32
				shade := float32(1)
				switch fc.mat {
				case matBoard, matFront:
					cx := max(fc.cx0, min(fc.cx1-1, int(math.Floor(p.x+hw))))
					py := max(fc.py0, min(fc.py1-1, int(math.Floor(hh-p.y))))
					src = int32((py>>1)*w + cx)
					if py&1 == 0 {
						col = r.tex[src].top
					} else {
						col = r.tex[src].bot
					}
					if fc.mat == matBoard {
						shade = 1 - 0.7*r.shadow[py*pw+cx]
					}
				case matSolid:
					col = fc.col
					if fc.edge {
						q := p.sub(fc.o)
						u, v := q.dot(fc.eu), q.dot(fc.ev)
						e := math.Min(0.45, 0.25*math.Min(fc.lu, fc.lv))
						if u < e || v < e || u > fc.lu-e || v > fc.lv-e {
							col = mix32(col, 0, 0.3) // its edges, a little darker
						}
					}
				case matFloor:
					col = floorAt(fc.col, p.x+hw, p.z)
				case matWall:
					col = fc.col
					if p.y < -hh+3 {
						col = mix32(col, hex32(cText), 0.12) // the skirting board
					}
				case matLamp, matShade:
					col = fc.col
				}
				r.zb[idx], r.face[idx], r.src[idx], r.base[idx] = float32(t), fid, src, col

				// The light here.
				dl := 1 / math.Sqrt(dir.x*dir.x+dir.y*dir.y+dir.z*dir.z)
				vx, vy, vz := -dir.x*dl, -dir.y*dl, -dir.z*dl
				fill := n.x*vx + n.y*vy + n.z*vz
				if fc.mat == matLamp {
					r.mul[idx], r.add[idx] = float32(0.85+0.15*fill), 0
					continue
				}
				lx, ly, lz := lamp.x-p.x, lamp.y-p.y, lamp.z-p.z
				d2 := lx*lx + ly*ly + lz*lz
				il := 1 / math.Sqrt(d2)
				lx, ly, lz = lx*il, ly*il, lz*il
				att := 1 / (1 + d2*invR2)
				ndl := n.x*lx + n.y*ly + n.z*lz
				if fc.mat == matShade && inside {
					ndl = 1 // the shade's inside is lit by the bulb right next to it
				}
				diff, spec := 0.0, 0.0
				if ndl > 0 {
					diff = ndl * att * float64(shade)
					hx, hy, hz := lx+vx, ly+vy, lz+vz
					hn := (n.x*hx + n.y*hy + n.z*hz) / math.Sqrt(hx*hx+hy*hy+hz*hz)
					if hn > 0 {
						s := hn * hn
						s *= s
						s *= s
						s *= s
						s *= s // to the 32nd
						spec = s * att * fc.gloss * float64(shade)
					}
				}
				r.mul[idx] = float32(light.ambient + light.fill*fill + light.lamp*diff)
				r.add[idx] = float32(light.glow*diff + light.spec*spec)
			}
		})
	}

	r.letters(cam, right, up, fwd, f, px, py, w, h)
	r.write(w, h, sb)
}

// fit is how much smaller than the board the view is drawn, and how far
// its middle is moved (ox, oy pixels), so the room fits on the screen:
// close to straight on, the back wall is the board cell for cell (as flat);
// the further it is turned, the more the whole room is fitted and centred,
// fully from 30° on.
func (r *room3D) fit(cam, right, up, fwd r3vec, f, hw, hh, depth, angle float64) (s, ox, oy float64) {
	x0, x1, y0, y1 := math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
	for _, p := range []r3vec{{-hw, -hh, 0}, {hw, -hh, 0}, {hw, hh, 0}, {-hw, hh, 0},
		{-hw, -hh, depth}, {hw, -hh, depth}, {hw, hh, depth}, {-hw, hh, depth}} {
		q := p.sub(cam)
		x, y, z := q.dot(right), q.dot(up), q.dot(fwd)
		if z < 1 {
			continue
		}
		x0, x1 = math.Min(x0, f*x/z), math.Max(x1, f*x/z)
		y0, y1 = math.Min(y0, -f*y/z), math.Max(y1, -f*y/z)
	}
	if x1 <= x0 || y1 <= y0 {
		return 1, 0, 0
	}
	room := math.Min(1, 0.96*math.Min(2*hw/(x1-x0), 2*hh/(y1-y0)))
	a := math.Min(1, angle/30)
	a *= a
	return (1 - a) + room*a, -a * room * (x0 + x1) / 2, -a * room * (y0 + y1) / 2
}

// floorAt is the floor's colour at (x, z): planks running out from the
// wall, each a slightly different shade, with dark joins.
func floorAt(base uint32, x, z float64) uint32 {
	const plankW, plankL = 7.0, 34.0
	p := math.Floor(x / plankW)
	along := z + math.Mod(p*13, plankL)
	hash := math.Mod(p*7.31+math.Floor(along/plankL)*3.17, 1)
	if hash < 0 {
		hash++
	}
	col := mix32(base, 0, 0.04+0.07*hash)
	if x-p*plankW < 0.45 || math.Mod(along, plankL) < 0.6 {
		col = mix32(col, 0, 0.2)
	}
	return col
}

// letters puts the flat board's letters on the screen where the wall and
// the blocks' fronts are seen nearly straight on, about one cell for one:
// there the text reads as on the flat board. Elsewhere the cells are only
// their colours.
//
// Each board cell goes to the screen cell its middle is seen in. Drawn a
// little bigger or smaller than the board, a word would get a gap or lose
// a letter, so each letter of a word goes straight after the one before,
// and a word shows whole or not at all. Spaces take their screen cells
// too, so where two rows would fall on one screen row, the second does not
// show.
func (r *room3D) letters(cam, right, up, fwd r3vec, f, px, py float64, w, h int) {
	pw := r.pw
	hw, hh := float64(w)/2, float64(h)
	if len(r.claim) != w*h {
		r.claim = make([]int32, w*h)
	}
	for i := range r.claim {
		r.claim[i] = -1
	}
	if -fwd.z < math.Cos(30*math.Pi/180) {
		return // turned more than 30°: only colours
	}
	// A step of one cell right, and one down, as the camera sees it.
	dR := r3vec{right.x, up.x, fwd.x}
	dD := r3vec{-2 * right.y, -2 * up.y, -2 * fwd.y}
	// target is the screen cell where the middle of board cell (cx, cy) on
	// a face at depth z is seen, when it is seen there about one for one.
	target := func(cx, cy int, z float64) (int, bool) {
		q := r3vec{float64(cx) + 0.5 - hw, hh - float64(2*cy+1), z}.sub(cam)
		c := r3vec{q.dot(right), q.dot(up), q.dot(fwd)}
		cr, cd := c.add(dR), c.add(dD)
		if c.z < 1 || cr.z < 1 || cd.z < 1 {
			return 0, false
		}
		sx, sy := px+f*c.x/c.z, py-f*c.y/c.z
		rx, ry := px+f*cr.x/cr.z-sx, py-f*cr.y/cr.z-sy
		dx, dy := px+f*cd.x/cd.z-sx, py-f*cd.y/cd.z-sy
		if rx < 0.85 || rx > 1.5 || math.Abs(ry) > 0.3 || dy < 1.8 || dy > 3 || math.Abs(dx) > 0.3 {
			return 0, false // turned too far, or too small or big, to read
		}
		col, row := int(math.Floor(sx)), int(math.Floor(sy/2))
		if col < 0 || col >= w || row < 0 || row >= h {
			return 0, false
		}
		return row*w + col, true
	}
	// free: screen cell at shows face fid in both its pixels, and nothing
	// has taken it yet.
	free := func(at int, fid int32) bool {
		top := at/w*2*pw + at%w
		return r.claim[at] < 0 && r.face[top] == fid && r.face[top+pw] == fid
	}
	var word []int
	for fi := range r.faces {
		fc := &r.faces[fi]
		if !fc.seen || (fc.mat != matBoard && fc.mat != matFront) {
			continue
		}
		z := r.pts[fc.p0].z
		fid := int32(fi)
		for cy := fc.py0 >> 1; cy < (fc.py1+1)>>1 && cy < h; cy++ {
			if fc.mat == matFront && !r.rowReads(target, fc, cy, z, w) {
				continue // a line of a structure shows whole or not at all
			}
			for cx := fc.cx0; cx < fc.cx1; {
				if r.tex[cy*w+cx].glyph == "" { // a space: it takes its cell
					at, ok := target(cx, cy, z)
					if ok && cx > fc.cx0 && r.claim[at] == int32(cy*w+cx-1) && (at+1)/w == at/w {
						at++ // drawn a little smaller: the cell before it took its cell
					}
					if ok && free(at, fid) {
						r.claim[at] = int32(cy*w + cx)
						if r.tex[cy*w+cx].blank {
							r.glyph[at] = int32(cy*w + cx) // a clean space between the words
						}
					}
					cx++
					continue
				}
				end := cx
				for end < fc.cx1 && r.tex[cy*w+end].glyph != "" {
					end++
				}
				word = word[:0]
				ok := true
				for x := cx; x < end && ok; x++ {
					at, seen := target(x, cy, z)
					if seen && x == cx && x > fc.cx0 && r.claim[at] == int32(cy*w+x-1) && (at+1)/w == at/w {
						at++ // drawn a little smaller: the cell before it took its cell
					}
					if n := len(word); n > 0 && seen {
						prev := word[n-1]
						if at < prev || at > prev+2 || (prev+1)/w != prev/w {
							seen = false // not about one cell for one here
						}
						at = prev + 1
					}
					ok = seen && free(at, fid)
					word = append(word, at)
				}
				if ok {
					for i, at := range word {
						s := int32(cy*w + cx + i)
						r.claim[at], r.glyph[at], r.placed[s] = s, s, r.stamp
					}
				}
				cx = end
			}
		}
	}
}

// rowReads is whether every letter of row cy of face fc is seen about one
// cell for one.
func (r *room3D) rowReads(target func(cx, cy int, z float64) (int, bool), fc *r3face, cy int, z float64, w int) bool {
	for cx := fc.cx0; cx < fc.cx1; cx++ {
		if r.tex[cy*w+cx].glyph != "" {
			if _, ok := target(cx, cy, z); !ok {
				return false
			}
		}
	}
	return true
}

// pixel is a pixel's colour, lit.
func (r *room3D) pixel(i int) uint32 {
	col := r.base[i]
	if s := r.src[i]; s >= 0 && r.placed[s] == r.stamp {
		col = r.tex[s].bg // its letter shows in a cell of its own: here only its background
	}
	return r.lit(col, i)
}

// lit is colour col in the light of pixel i.
func (r *room3D) lit(col uint32, i int) uint32 {
	m, a := float64(r.mul[i]), float64(r.add[i])
	cr, cg, cb := chans(col)
	return rgb32(clampF(cr*m+a*lampWarm[0], 0, 255), clampF(cg*m+a*lampWarm[1], 0, 255), clampF(cb*m+a*lampWarm[2], 0, 255))
}

// write turns the pixels into h lines of w cells, two pixels a cell.
func (r *room3D) write(w, h int, sb *strings.Builder) {
	pw := r.pw
	buf := make([]byte, 0, w*24)
	colour := func(buf []byte, lead string, c uint32) []byte {
		buf = append(buf, lead...)
		buf = strconv.AppendUint(buf, uint64(c>>16&255), 10)
		buf = append(buf, ';')
		buf = strconv.AppendUint(buf, uint64(c>>8&255), 10)
		buf = append(buf, ';')
		buf = strconv.AppendUint(buf, uint64(c&255), 10)
		return append(buf, 'm')
	}
	for row := 0; row < h; row++ {
		buf = buf[:0]
		fg, bg := int64(-1), int64(-1)
		for col := 0; col < w; col++ {
			ti := 2*row*pw + col
			bi := ti + pw
			var top, bot uint32
			glyph := "▀"
			if s := r.glyph[row*w+col]; s >= 0 {
				t := &r.tex[s]
				top, bot, glyph = r.lit(t.fg, ti), r.lit(t.bg, ti), t.glyph
				if glyph == "" {
					glyph = " " // a space of the board
				}
			} else {
				top, bot = r.pixel(ti), r.pixel(bi)
				if top == bot {
					glyph = " "
				}
			}
			if glyph != " " && int64(top) != fg {
				buf = colour(buf, "\x1b[38;2;", top)
				fg = int64(top)
			}
			if int64(bot) != bg {
				buf = colour(buf, "\x1b[48;2;", bot)
				bg = int64(bot)
			}
			buf = append(buf, glyph...)
		}
		buf = append(buf, "\x1b[m\n"...)
		sb.Write(buf)
	}
}
