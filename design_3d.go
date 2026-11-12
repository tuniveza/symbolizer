package main

// ---- the board as a room in 3D ------------------------------------------------------
//
// Ctrl + Left / Right turn the board's room, Ctrl + Up / Down tilt it, 5° a
// press (held: it keeps turning); a drag with the left button on the empty
// room turns it too. Turned away from straight on, the board area shows the
// board as a room: the board is the back wall, with a floor and two side
// walls, and every structure on the screen comes out of the wall as a
// block. Paper (pages, notes, slides, drawings) is thin, a book a little
// thicker, a board shallow, a shape deeper, and cabinets, drawer units and
// racks deepest. The front of each block is its own drawing from the flat
// board; its sides are its colour. A lamp hangs at the front left and
// lights it all, and the blocks cast soft shadows on the back wall. The
// walls and the floor stand just beyond the outermost blocks, so no block
// goes through them, or further out to take in the camera: the room is
// closed, with a ceiling and a wall behind the camera, so the whole board
// area shows room at any turn, tilt and zoom. The walls are papered, with
// rails and a skirting board, and the board hangs on the back wall like a
// panel in a frame (see design_3d_walls.go).
//
// The room works as the flat board does: a click on a block's front is a
// click on that cell of its flat drawing (a drawer pulls out, a lid opens,
// a title is written in), a drag moves it along the wall under the mouse,
// and keys other than the 3D view's own do what they do on the flat board.
// The middle or right button moves the camera sideways, the wheel closer.
//
// Ctrl + F is free roam: the camera walks (W A S D, Q and E down and up,
// Shift faster) and looks round (the arrows, or a drag), anywhere, into
// the room and through the blocks. In the turned room the arrows turn it
// as Ctrl + arrows do, and W A S D start free roam from where the camera is.
//
// Straight on again (or Esc), it is the flat board, exactly as before. The
// turn is only a way of looking: it is not an undo step and is not saved.
// The drawing itself is in design_3d_draw.go.

import (
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
)

const (
	turnStep     = 5                      // degrees a press
	turnYawMax   = 80                     // the room has no front wall: never seen from behind
	turnPitchMax = 75                     // tilted down: seen from above
	turnPitchUp  = 45                     // tilted up: seen from below, the ceiling over it
	turnGlide    = 2                      // degrees a glide step, held
	turnStart    = 160 * time.Millisecond // held: the first glide step (a quick tap is one step)
	turnEvery    = 30 * time.Millisecond  // then 33 steps a second: about 65° a second
	roomNearMin  = -6                     // + and - (and the wheel): the camera's steps closer (or further)
	roomNearMax  = 10
	turnPerCol   = 1.5 // degrees the room turns for a cell the mouse is dragged across
	turnPerRow   = 3.0 // and for a row it is dragged down (a row is twice as tall)
	walkSpeed    = 0.4 // free roam: the screen's bigger side a second, walking
	walkFast     = 3   // with Shift: this many times faster
	walkTap      = 0.06 * float64(time.Second)
	walkEvery    = 20 * time.Millisecond // held: a step every 20 ms, as far as the time since the last one
	roomHint     = "3D view  ·  arrows/drag: turn  ·  W A S D: walk  ·  right-drag: move  ·  wheel: zoom  ·  Esc: flat"
	roomFreeHint = "Free roam  ·  W A S D: walk  ·  Q E: down, up  ·  Shift: fast  ·  arrows or drag: look  ·  Esc: back"
)

// What a drag with the mouse does to the camera.
const (
	camNone = iota
	camTurn // the left button on the empty room: it turns (free roam: the camera looks round)
	camPan  // the middle or right button: the camera moves sideways
)

// room3D is the turned view and what its drawing keeps from frame to frame.
type room3D struct {
	yaw   int   // degrees: the room turned right (+) or left (−)
	pitch int   // degrees: its front tilted down (+, the floor shows) or up (−)
	near  int   // camera steps closer (+) or further (−)
	pan   r3vec // the camera moved sideways (middle or right drag)

	// Free roam (Ctrl + F): where the camera is and which way it looks
	// (degrees right of straight at the wall, and down), its focal length,
	// and where it started (0 goes back there).
	free      bool
	pos       r3vec
	look      [2]float64
	ff        float64
	startPos  r3vec
	startLook [2]float64
	held      map[rune]bool // W A S D Q E held down (kitty says when they are let go)
	heldAt    time.Time
	stepAt    time.Time // the last walking step
	walking   bool      // the walking ticks are going

	// The mouse.
	camDrag        int // camNone, camTurn or camPan
	pressX, pressY int // where it was pressed (screen cells of the board area)
	lastX, lastY   int
	yaw0, pitch0   int
	look0          [2]float64
	dragDist       float64 // how far in front of the camera the press was: a pan moves that much under the mouse
	click          r3hit   // the empty wall pressed: let go without a drag, a click there
	clickOn        bool
	passing        bool     // a mouse event handed on to the flat board (designPress and the rest)
	plane          float64  // a drag handed on: the mouse is followed on the plane this far out of the wall
	mapX, mapY     int      // the flat board cell the mouse was last followed to
	pw, ph         int      // the pixels: twice the board area's width and height (quarter blocks), or a picture's (kitty)
	kx, ky         float64  // the pixels a unit across and a unit down (2 and 1 in quarter blocks)
	want           [2]int   // the picture's size for the next drawing (see room3DDensity), or 0: quarter blocks
	pix            bool     // the last drawing is a picture (kitty, see design_3d_kitty.go)
	fdraw          []r3draw // per face: how it is drawn this frame
	polys          [][2]float64
	camWas         r3camKey  // the camera of the last drawing
	movedAt        time.Time // when the camera last moved: until room3DSettle after, fewer pixels
	lowRes         bool      // the last picture was drawn with fewer pixels, the camera moving
	sharpTick      bool      // a room3DSharpMsg is on its way
	base           []uint32  // each pixel's colour before the light
	mul            []float32 // the light on it: base times mul, plus add of the lamp's colour
	add            []float32
	zb             []float32  // the depth of the nearest surface
	face           []int32    // which face it shows (-1: none)
	dirs           []r3vec    // each pixel's ray
	tex            []r3texel  // the back wall's cells (the flat board, widened: see r3room)
	mips           [][]uint32 // the back wall as pictures, each half as big as the one before (see mipmap)
	mipW, mipH     []int
	shadow         []float32 // the blocks' shadows on the back wall, per unit of it
	chars          []r3char  // per screen cell: the letter of a word drawn there (see words)
	cover          []int32   // per back wall cell: the frame (stamp) a block stood on it
	order          []r3near  // the faces with words, nearest first
	marks          []r3run   // a face's marks, put after its words
	firstAt        []int32   // per face: the first screen cell that shows it (see firstSeen), or -1
	firstStamp     int32     // the frame firstAt is for
	stamp          int32     // the frame: counts up with each drawing
	faces          []r3face
	pts            []r3vec
	blocks         []r3block
	inkBox         [4]int // where the pen's ink is on the board (see ink), kept until it changes
	inkVer, inkLen int
	hexes          map[string]uint32 // colours read from their "#rrggbb"
	cab            map[int]*r3cab    // the cabinets this frame: their doors and shelves (design_3d_cabinet.go)
	view           r3view            // the camera of the last drawing, for the mouse
	rm             r3room            // the room of the last drawing
	frame          string            // the last drawing, for frames where only the mouse moved
	frameW, frameH int
	cost           time.Duration // how long the last drawing took
}

// on is whether the board area shows the room in 3D: turned, or in free roam.
func (r *room3D) on() bool { return r.free || r.yaw != 0 || r.pitch != 0 }

// hint is the status bar's line for the 3D view.
func (r *room3D) hint() string {
	if r.free {
		return roomFreeHint
	}
	return roomHint
}

// ---- keys --------------------------------------------------------------------------------

// design3DKey handles the keys of the 3D view: Ctrl + arrows and Ctrl + F
// at any time, and while it shows Esc, + − 0 and, in free roam, W A S D Q E.
// While writing in a structure those type as usual (Esc stops writing).
// Every other key does what it does on the flat board. It says whether it
// took the key.
func (m *Model) design3DKey(k tea.Key, name string) (bool, tea.Cmd) {
	d := m.ds()
	r := &d.room
	switch name {
	case "ctrl+left", "ctrl+right", "ctrl+up", "ctrl+down":
		d.editing = partNone // writing stops, as with any arrow
		return true, m.design3DTurnKey(k, arrowDir(name))
	case "ctrl+f":
		d.editing = partNone
		if r.free {
			m.design3DLeaveFree()
		} else {
			m.design3DFree()
		}
		return true, nil
	}
	if !r.on() || d.editing != partNone {
		return false, nil
	}
	switch name {
	case "esc":
		if r.free {
			m.design3DLeaveFree()
		} else {
			r.yaw, r.pitch, r.pan = 0, 0, r3vec{}
			m.say("Flat board.")
		}
		return true, nil
	case "+", "=", "ctrl+=", "shift+=", "plus", "kp+":
		m.design3DNear(1)
		return true, nil
	case "-", "ctrl+-", "minus", "kp-":
		m.design3DNear(-1)
		return true, nil
	case "0":
		if r.free {
			r.pos, r.look = r.startPos, r.startLook
			m.say("Free roam: back where it started.")
		} else {
			r.near, r.pan = 0, r3vec{}
			m.say("3D view: the camera where it starts.")
		}
		return true, nil
	case "left", "right", "up", "down":
		// The arrows look round, as the mouse does: they turn the room
		// (in free roam the camera looks that way).
		return true, m.design3DTurnKey(k, name)
	}
	if key := walkKey(name); key != 0 {
		if !r.free {
			if key == 'q' || key == 'e' {
				return false, nil // down and up only in free roam
			}
			m.design3DFree() // W A S D walk off from where the camera is
		}
		return true, m.design3DWalkKey(k, key)
	}
	return false, nil
}

// design3DTurnKey turns the room (or, in free roam, the camera's look)
// for an arrow pressed, with or without Ctrl. Held in kitty, which says
// when the key is let go, it glides on while held.
func (m *Model) design3DTurnKey(k tea.Key, dir string) tea.Cmd {
	d := m.ds()
	if !m.releases {
		m.design3DTurn(dir, false) // other terminals: each press and repeat turns a step
		return nil
	}
	// The terminal's own repeats only say it is still held.
	m.chord.held[dir] = true
	d.heldAt = time.Now()
	if k.IsRepeat {
		return nil
	}
	m.design3DTurn(dir, false)
	d.holdMods = "ctrl+" // designHold sends the glide steps to design3DHold
	if d.holdTick {
		return nil
	}
	d.holdTick = true
	return tea.Tick(turnStart, func(time.Time) tea.Msg { return designHoldMsg{} })
}

// walkKey is the free roam key that name is (w a s d q e, with or without
// Shift), or 0.
func walkKey(name string) rune {
	name = strings.TrimPrefix(name, "shift+")
	if len(name) == 1 && strings.Contains("wasdqe", name) {
		return rune(name[0])
	}
	return 0
}

// design3DTurn turns the room that way (Ctrl + that arrow): a press to
// the next multiple of 5°, a glide step (held) by turnGlide°. In free roam
// the camera looks that way instead.
func (m *Model) design3DTurn(dir string, glide bool) {
	r := &m.ds().room
	if r.free {
		step := float64(turnStep)
		if glide {
			step = turnGlide
		}
		switch dir {
		case "left":
			r.lookBy(-step, 0)
		case "right":
			r.lookBy(step, 0)
		case "up":
			r.lookBy(0, -step)
		case "down":
			r.lookBy(0, step)
		}
		return
	}
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
	r.clampTurn()
	if !r.on() {
		m.say("Flat board.")
		return
	}
	m.say(roomAngles(r.yaw, r.pitch))
}

// clampTurn keeps the turn and tilt within what the room can show.
func (r *room3D) clampTurn() {
	r.yaw = max(-turnYawMax, min(turnYawMax, r.yaw))
	r.pitch = max(-turnPitchUp, min(turnPitchMax, r.pitch))
}

// lookBy turns the free roam camera: dx degrees right, dy down. It turns
// right round; looking up or down stops just short of straight.
func (r *room3D) lookBy(dx, dy float64) {
	r.look[0] = math.Mod(r.look[0]+dx+540, 360) - 180
	r.look[1] = clampF(r.look[1]+dy, -89, 89)
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

// design3DNear moves the camera closer (step 1) or further (-1). In free
// roam it walks that way, straight ahead.
func (m *Model) design3DNear(step int) {
	r := &m.ds().room
	if r.free {
		r.pos = r.pos.add(r.view.fwd.mul(0.06 * r.rm.big * float64(step)))
		return
	}
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

// ---- free roam -----------------------------------------------------------------------------

// room3DWalkMsg is the next walking step while W A S D Q E are held.
type room3DWalkMsg struct{}

// design3DFree starts free roam where the camera is now: the view does not
// jump.
func (m *Model) design3DFree() {
	d := m.ds()
	r := &d.room
	rm := r.area(d, m.width, m.boardH(), d.z())
	v := r.orbitView(&rm)
	c := v.ray(float64(rm.w)/2, float64(rm.h)).norm() // straight ahead on the screen
	r.look = [2]float64{math.Atan2(c.x, -c.z) * 180 / math.Pi, math.Asin(clampF(-c.y, -1, 1)) * 180 / math.Pi}
	r.look[1] = clampF(r.look[1], -89, 89)
	r.pos, r.ff, r.rm = v.cam, v.f, rm
	r.startPos, r.startLook = r.pos, r.look
	r.free = true
	m.say("Free roam: W A S D walk, Q E down and up, Shift faster; the arrows or a drag look round.  Esc: back.")
}

// design3DLeaveFree goes back from free roam to the turned room (or the
// flat board, when it was straight on).
func (m *Model) design3DLeaveFree() {
	r := &m.ds().room
	r.free, r.walking = false, false
	clear(r.held)
	if r.on() {
		m.say("Free roam over: back to the turned room.")
	} else {
		m.say("Free roam over: the flat board.")
	}
}

// design3DWalkKey takes W A S D Q E pressed in free roam.
func (m *Model) design3DWalkKey(k tea.Key, key rune) tea.Cmd {
	r := &m.ds().room
	fast := m.keys.shift || k.Mod.Contains(tea.ModShift)
	if !m.releases {
		// Other terminals: each press and repeat walks a step.
		r.walk(map[rune]bool{key: true}, 2*walkTap/float64(time.Second), fast)
		return nil
	}
	// kitty says when the key is let go: it walks smoothly while held.
	if r.held == nil {
		r.held = map[rune]bool{}
	}
	r.held[key] = true
	r.heldAt = time.Now()
	if k.IsRepeat {
		return nil
	}
	r.walk(map[rune]bool{key: true}, walkTap/float64(time.Second), fast) // a tap walks a little at once
	if r.walking {
		return nil
	}
	r.walking, r.stepAt = true, time.Now()
	return tea.Tick(walkEvery, func(time.Time) tea.Msg { return room3DWalkMsg{} })
}

// design3DKeyUp: a key let go (kitty): W A S D Q E stop walking.
func (m *Model) design3DKeyUp(k tea.Key) {
	if m.design == nil {
		return
	}
	r := &m.design.room
	if key := unicode.ToLower(rune(k.Code)); r.held != nil && strings.ContainsRune("wasdqe", key) {
		r.held[key] = false
	}
}

// design3DWalkStep walks on while keys are held, as far as the time since
// the last step, or stops when they are let go.
func (m *Model) design3DWalkStep() tea.Cmd {
	d := m.design
	if d == nil {
		return nil
	}
	r := &d.room
	any := false
	for _, down := range r.held {
		any = any || down
	}
	now := time.Now()
	if !m.inDesign() || m.help || !r.free || !any || now.Sub(r.heldAt) > holdLost {
		r.walking = false
		clear(r.held)
		return nil
	}
	dt := math.Min(now.Sub(r.stepAt).Seconds(), 0.1)
	r.stepAt = now
	r.walk(r.held, dt, m.keys.shift)
	return tea.Tick(walkEvery, func(time.Time) tea.Msg { return room3DWalkMsg{} })
}

// walk moves the free roam camera for secs seconds with keys held: W and
// S forward and back and A and D sideways, level whichever way it looks,
// and Q and E straight down and up.
func (r *room3D) walk(keys map[rune]bool, secs float64, fast bool) {
	ly := r.look[0] * math.Pi / 180
	ahead, side := r3vec{math.Sin(ly), 0, -math.Cos(ly)}, r3vec{math.Cos(ly), 0, math.Sin(ly)}
	var way r3vec
	for key, down := range keys {
		if !down {
			continue
		}
		switch key {
		case 'w':
			way = way.add(ahead)
		case 's':
			way = way.sub(ahead)
		case 'd':
			way = way.add(side)
		case 'a':
			way = way.sub(side)
		case 'e':
			way.y++
		case 'q':
			way.y--
		}
	}
	if way.dot(way) < 1e-9 {
		return
	}
	speed := walkSpeed * math.Max(r.rm.big, 20)
	if fast {
		speed *= walkFast
	}
	r.pos = r.pos.add(way.norm().mul(speed * secs))
}

// ---- the mouse ----------------------------------------------------------------------------

// r3hit is what is under a screen cell in the 3D view: the face, the
// structure it is part of (-1: the room), the point, how far in front of
// the camera it is, and the flat board cell it stands for (flat: there is
// one: a block's front or side, or the back wall where it is on the screen).
type r3hit struct {
	ok, flat, wall bool
	face           int32
	own            int
	p              r3vec
	sx, sy         int
	dist           float64
}

// pickAt is what the last drawing shows in screen cell (col, row) of the
// board area: of its four quarters (see cellFour), the nearest surface.
func (r *room3D) pickAt(col, row int) r3hit {
	pw := r.pw
	if pw == 0 || col < 0 || row < 0 || col >= r.rm.w || row >= r.rm.h || len(r.face) != pw*r.ph {
		return r3hit{own: -1}
	}
	best := -1
	for _, i := range r.cellFour(col, row) {
		if f := r.face[i]; f >= 0 && int(f) < len(r.faces) && (best < 0 || r.zb[i] < r.zb[best]) {
			best = i
		}
	}
	if best < 0 {
		return r3hit{own: -1}
	}
	fc := &r.faces[r.face[best]]
	v := &r.view
	p := v.cam.add(r.dirs[best].mul(float64(r.zb[best])))
	// The cell's middle on that face's plane: the point the mouse is on.
	dir := v.ray(float64(col)+0.5, float64(2*row)+1)
	if nd := fc.n.dot(dir); math.Abs(nd) > 1e-12 {
		if t := (fc.k - fc.n.dot(v.cam)) / nd; t > 0 {
			p = v.cam.add(dir.mul(t))
		}
	}
	h := r3hit{ok: true, face: r.face[best], own: fc.own, p: p, dist: p.sub(v.cam).dot(v.fwd)}
	rm := &r.rm
	h.sx, h.sy = int(math.Floor(p.x+rm.hw)), int(math.Floor((rm.hh-p.y)/2))
	switch {
	case fc.own >= 0: // a block: the cell of its front straight in front of the point
		h.sx = max(fc.cx0, min(fc.cx1-1, h.sx))
		h.sy = max(fc.py0>>1, min(fc.py1>>1-1, h.sy))
		h.flat = true
	case fc.mat == matBoard:
		h.wall = true
		h.flat = h.sx >= 0 && h.sy >= 0 && h.sx < rm.w && h.sy < rm.h
	}
	return h
}

// design3DPress: a press in the 3D view. The left button on a block (or
// on the wall where the flat board has something: a pulled out drawer, the
// pen) is a press on its flat board cell; on the empty room it turns the
// room (free roam: looks round), and let go without a drag it is a click
// on the empty board there. The middle and right buttons move the camera.
// It says whether it took the press.
func (m *Model) design3DPress(ms tea.Mouse) (bool, tea.Cmd) {
	d := m.ds()
	r := &d.room
	if !r.on() || r.passing {
		return false, nil
	}
	col, row := ms.X, ms.Y-m.boardTop()
	hit := r.pickAt(col, row)
	r.camDrag, r.clickOn = camNone, false
	r.pressX, r.pressY, r.lastX, r.lastY = col, row, col, row
	r.yaw0, r.pitch0, r.look0 = r.yaw, r.pitch, r.look
	r.dragDist = r3vec{0, 0, r.rm.depth / 2}.add(r.pan).sub(r.view.cam).len()
	if hit.ok {
		r.dragDist = math.Max(1, hit.dist)
	}
	switch ms.Button {
	case tea.MouseMiddle, tea.MouseRight:
		r.camDrag = camPan
		return true, nil
	case tea.MouseLeft:
	default:
		return true, nil
	}
	if hit.flat && (hit.own >= 0 || m.design3DTakes(hit, ms)) {
		return true, m.design3DPass(ms, hit)
	}
	r.camDrag = camTurn
	r.click, r.clickOn = hit, hit.flat
	return true, nil
}

// design3DTakes is whether a press on the back wall is for the board, not
// for turning: something of a structure is drawn there (a pulled out
// drawer, an open door), the pen is out, or a mode picks with the mouse.
func (m *Model) design3DTakes(hit r3hit, ms tea.Mouse) bool {
	d := m.ds()
	return d.hit(hit.sx, hit.sy).o != nil || d.pen >= 0 || d.groupMode ||
		ms.Mod.Contains(tea.ModCtrl) || ms.Mod.Contains(tea.ModShift)
}

// design3DPass hands a press on to the flat board, as if it were on the
// flat board cell under the mouse. A drag after it follows the mouse on
// the plane of the point pressed, so what is dragged stays under it.
func (m *Model) design3DPass(ms tea.Mouse, hit r3hit) tea.Cmd {
	r := &m.ds().room
	r.plane = 0
	if hit.own >= 0 {
		r.plane = hit.p.z
	}
	r.mapX, r.mapY = hit.sx, hit.sy
	r.passing = true
	defer func() { r.passing = false }()
	ms.X, ms.Y = hit.sx, hit.sy+m.boardTop()
	return m.designPress(ms)
}

// design3DThrough turns a mouse position in the 3D view into the flat
// board cell it is over, on the plane of the drag (see design3DPass).
func (m *Model) design3DThrough(ms tea.Mouse) tea.Mouse {
	r := &m.ds().room
	v := &r.view
	if v.f > 0 {
		dir := v.ray(float64(ms.X)+0.5, 2*float64(ms.Y-m.boardTop())+1)
		if math.Abs(dir.z) > 1e-9 {
			if t := (r.plane - v.cam.z) / dir.z; t > 0 {
				p := v.cam.add(dir.mul(t))
				w, h := float64(r.rm.w), float64(r.rm.h)
				r.mapX = int(math.Floor(clampF(p.x+r.rm.hw, -4*w, 5*w)))
				r.mapY = int(math.Floor(clampF((r.rm.hh-p.y)/2, -4*h, 5*h)))
			}
		}
	}
	ms.X, ms.Y = r.mapX, r.mapY+m.boardTop()
	return ms
}

// design3DMotion: the mouse moved with a button down, turning or moving
// the camera.
func (m *Model) design3DMotion(ms tea.Mouse) tea.Cmd {
	r := &m.ds().room
	col, row := ms.X, ms.Y-m.boardTop()
	switch r.camDrag {
	case camTurn:
		dx, dy := col-r.pressX, row-r.pressY
		if dx == 0 && dy == 0 && r.clickOn {
			return nil // not moved yet: it may still be a click
		}
		r.clickOn = false
		if r.free {
			r.look = r.look0
			r.lookBy(float64(dx)*turnPerCol, float64(dy)*turnPerRow)
		} else {
			r.yaw = r.yaw0 + int(math.Round(float64(dx)*turnPerCol))
			r.pitch = r.pitch0 + int(math.Round(float64(dy)*turnPerRow))
			r.clampTurn()
			if r.on() {
				m.say(roomAngles(r.yaw, r.pitch))
			} else {
				m.say("Flat board.")
			}
		}
	case camPan:
		// What is as far away as the point pressed moves with the mouse.
		v := &r.view
		if v.f > 0 {
			s := r.dragDist / v.f
			move := v.right.mul(-float64(col-r.lastX) * s).add(v.up.mul(2 * float64(row-r.lastY) * s))
			if r.free {
				r.pos = r.pos.add(move)
			} else {
				r.pan = r.pan.add(move)
			}
		}
	}
	r.lastX, r.lastY = col, row
	return nil
}

// design3DRelease: the button let go after a press on the empty room or a
// camera drag. Not dragged, a press on the empty wall is a click on the
// empty board there (a double click places a structure, as on the flat board).
func (m *Model) design3DRelease(ms tea.Mouse) tea.Cmd {
	r := &m.ds().room
	was, click := r.camDrag, r.clickOn
	r.camDrag, r.clickOn = camNone, false
	if was == camTurn && click && r.on() {
		cmd := m.design3DPass(ms, r.click)
		return tea.Batch(cmd, m.designRelease())
	}
	return nil
}

// design3DWheel: the wheel moves the camera closer or further (free roam:
// walks ahead or back). With Shift over a block it does what it does on
// the flat board (scrolls a page's text). It says whether it took it.
func (m *Model) design3DWheel(ms tea.Mouse) (bool, tea.Cmd) {
	r := &m.ds().room
	if !r.on() || r.passing {
		return false, nil
	}
	if ms.Mod.Contains(tea.ModShift) {
		if hit := r.pickAt(ms.X, ms.Y-m.boardTop()); hit.flat && hit.own >= 0 {
			r.passing = true
			defer func() { r.passing = false }()
			ms.X, ms.Y = hit.sx, hit.sy+m.boardTop()
			return true, m.designWheel(ms)
		}
	}
	switch ms.Button {
	case tea.MouseWheelUp:
		m.design3DNear(1)
	case tea.MouseWheelDown:
		m.design3DNear(-1)
	}
	return true, nil
}

// design3DHover is the flat board cell the mouse is over in the 3D view
// (for Space, which opens what is under the mouse), and whether there is one.
func (m *Model) design3DHover(ms tea.Mouse) (tea.Mouse, bool) {
	hit := m.ds().room.pickAt(ms.X, ms.Y-m.boardTop())
	if !hit.flat {
		return ms, false
	}
	ms.X, ms.Y = hit.sx, hit.sy+m.boardTop()
	return ms, true
}

// ---- drawing -------------------------------------------------------------------------------

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
	if w < 1 || h < 1 {
		for range h {
			sb.WriteByte('\n')
		}
		return
	}
	start := time.Now()
	// The flat board, which the wall and the fronts show, without what
	// floats over it (the mini map, the list of name plates).
	corner, plates := d.mapCorner, d.platesOpen
	d.mapCorner, d.platesOpen = 0, false
	r.cabinets(d, w, h) // how open each cabinet is this frame, for the flat drawing and the 3D doors alike
	c := m.designCanvas()
	d.mapCorner, d.platesOpen = corner, plates
	rm := r.area(d, w, h, d.z())
	r.takeRoom(d, c, &rm)
	r.mipmap(&rm)
	r.want, r.lowRes = [2]int{}, false
	if room3DPixels && w <= len(kittyMarks) && h <= len(kittyMarks) {
		// A picture (kitty): fewer pixels while the camera moves.
		moving := r.moving(w, h, start)
		limit := room3DStill
		if moving {
			limit = room3DMoving
		}
		if room3DRemote {
			limit /= 3 // over SSH every pixel goes down the line
		}
		cw, ch := termCell(m.width, m.height)
		pw, ph := room3DDensity(w, h, cw, ch, limit)
		r.want, r.lowRes = [2]int{pw, ph}, moving
	}
	var fb strings.Builder
	r.draw(&rm, d.z(), &fb)
	r.frame, r.frameW, r.frameH = fb.String(), w, h
	if r.pix {
		kittyPic.show(&kittyFrame{rgb: r.picture(kittyPic.buffer()), pw: r.pw, ph: r.ph, cols: w, rows: h})
	}
	r.cost = time.Since(start)
	sb.WriteString(r.frame)
}

// r3camKey is where the camera is: when it changes, the camera moved.
type r3camKey struct {
	yaw, pitch, near int
	pan, pos         r3vec
	look             [2]float64
	free             bool
	w, h             int
}

// camKey is the camera now, for a w×h board area.
func (r *room3D) camKey(w, h int) r3camKey {
	return r3camKey{r.yaw, r.pitch, r.near, r.pan, r.pos, r.look, r.free, w, h}
}

// moving is whether the camera moved within room3DSettle before now (for
// a w×h board area): a picture drawn now is drawn with fewer pixels.
func (r *room3D) moving(w, h int, now time.Time) bool {
	if k := r.camKey(w, h); k != r.camWas {
		r.camWas, r.movedAt = k, now
	}
	return now.Sub(r.movedAt) < room3DSettle
}

// room3DSharpMsg: the camera may have stopped, so the room is drawn sharp.
type room3DSharpMsg struct{}

// design3DAfter runs after every message (see Update): it asks for the
// room to be drawn sharp once the camera stops (room3DSharpMsg), and has
// kitty delete the picture once the board does not show the room.
func (m *Model) design3DAfter() tea.Cmd {
	d := m.design
	if d == nil || !m.inDesign() || !d.room.on() {
		if kittyPic.isShown() {
			kittyPic.remove()
			if d != nil {
				d.room.frame = "" // its placeholders, without the picture now
			}
		}
		return nil
	}
	r := &d.room
	if !room3DPixels || r.sharpTick {
		return nil
	}
	// Will the next drawing be one with fewer pixels? Then a sharp one
	// after it, once the camera has been still for room3DSettle.
	wait := room3DSettle - time.Since(r.movedAt)
	if r.camKey(m.width, m.boardH()) != r.camWas {
		wait = room3DSettle
	}
	if wait <= 0 {
		return nil // the next drawing is sharp anyway
	}
	r.sharpTick = true
	return tea.Tick(wait+5*time.Millisecond, func(time.Time) tea.Msg { return room3DSharpMsg{} })
}
