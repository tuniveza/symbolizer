package main

// ---- the board as a room in 3D ------------------------------------------------------
//
// Ctrl + F (or Esc) goes into 3D and out again. 3D is a first-person
// camera and nothing else: moving the mouse looks round (no button
// needed), the arrows look round too (the mouse stops at the window's
// edge, they do not), W A S D move (W and S the way the camera looks, A
// and D sideways), Q and E move straight up and down, and Shift moves
// faster. The mouse pointer is hidden (design_3d_pointer.go). There is no turning, tilting,
// zooming or moving the room: the camera is the only way to look at it.
// Whatever the middle of the camera's view is on is the chosen structure
// (a small cross marks it), so keys act on it as on the flat board, and
// the mouse's buttons, wheel and drags act there too, not where the
// pointer is: the cross is the pointer.
//
// The board is the back wall of a room, with a floor and two side walls,
// and every structure on the screen comes out of the wall as a block.
// Paper (pages, notes, slides, drawings) is thin, a book a little thicker,
// a board shallow, a shape deeper, and cabinets, drawer units and racks
// deepest. The front of each block is its own drawing from the flat board;
// its sides are its colour. A lamp hangs at the front left and lights it
// all, and the blocks cast soft shadows on the back wall. The walls are
// papered, with rails and a skirting board, and the board hangs on the
// back wall like a panel in a frame (see design_3d_walls.go); T and F
// change what the walls and the floor are made of (design_3d_finish.go). The camera
// starts square to the wall, exactly where the flat board was, and the
// blocks come out of the wall as it looks away or moves (outK), so going
// into 3D has no jump.
//
// The room works as the flat board does: a click with the cross on a
// block's front is a click on that cell of its flat drawing (a drawer
// pulls out, a lid opens, a title is written in), a drag moves it along
// the wall under the cross as the mouse turns the camera,
// and keys other than the 3D view's own do what they do on the flat board.
//
// The room is drawn in letters over its picture (design_3d_text.go), or
// in kitty as a picture with the words painted on the cards
// (design_3d_glyphs.go). yaw, pitch, near and pan are a fixed camera round
// the middle of the wall that only tests set: no key turns it.
//
// Out of 3D it is the flat board, exactly as before. 3D is only a way of
// looking: it is not an undo step and is not saved. The drawing itself is
// in design_3d_draw.go.

import (
	"math"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
)

const (
	turnStep   = 5                      // degrees an arrow press looks round
	turnRate   = 65.0                   // degrees a second an arrow held looks round
	turnStart  = 160 * time.Millisecond // held: the glide starts (a quick tap is one step)
	moveEvery  = 4 * time.Millisecond   // held arrows and W A S D Q E move this often, as far as the time since: every frame
	turnPerCol = 1.5                    // degrees the camera turns for a cell the mouse moves across
	turnPerRow = 3.0                    // and for a row it moves down (a row is twice as tall)
	walkSpeed  = 0.4                    // the screen's bigger side a second, moving
	walkFast   = 3                      // with Shift: this many times faster
	walkTap    = 0.06 * float64(time.Second)
	outWalk    = 0.3 // the blocks are all the way out of the wall once the camera has moved this much of the screen's bigger side (see outK)
	roomHint   = "3D view · mouse: look · W A S D: move · Q E: up, down · Shift: faster · M: music · Esc: flat board"
)

// What a drag with the mouse does to the camera.
const (
	camNone = iota
	camTurn // the left button on the empty room: the camera looks round, as the mouse with no button does
)

// room3D is the turned view and what its drawing keeps from frame to frame.
type room3D struct {
	// A fixed camera round the middle of the wall, for tests: no key sets
	// it. Turned right (+) or left (−), its front tilted down (+) or up
	// (−), in degrees; steps closer (+) or further (−); moved sideways.
	yaw   int
	pitch int
	near  int
	pan   r3vec

	// The first-person camera (Ctrl + F): where it is and which way it
	// looks (degrees right of straight at the wall, and down), its focal
	// length, and where it started.
	free      bool
	pos       r3vec
	look      [2]float64
	ff        float64
	freeK     float64 // how far out of the wall the blocks are (see outK)
	startPos  r3vec
	startLook [2]float64
	held      map[rune]bool // W A S D Q E held down (kitty says when they are let go)
	heldAt    time.Time
	stepAt    time.Time // the last walking step
	walking   bool      // the walking ticks are going
	turnAt    time.Time // held arrows: the last glide step (or when the glide starts)

	// The mouse.
	camDrag        int // camNone or camTurn
	pressX, pressY int // where it was pressed (screen cells of the board area)
	lastX, lastY   int
	click          r3hit // the empty wall pressed: let go without a drag, a click there
	clickOn        bool
	hoverX, hoverY float64     // mouse look with no button: where the mouse was last seen (cells, to a fraction)
	hoverOn        bool        // hoverX and hoverY are set (false: the next move only sets them)
	edge           [2]int      // the mouse rests at the window's edge: the way it turns on (-1, 0, 1 across and down)
	edgeOn         bool        // the edge ticks are going
	edgeStep       time.Time   // the last edge step
	lens           float64     // the lens: 1 normal, more zooms in (= and − in 3D, design_3d_layers.go)
	ff0            float64     // the focal length 3D started with
	anchor         [2]float64  // the flat board's camX, camY at the last drawing (keepCamera)
	flatZoom       int         // the flat board's zoom before 3D set it to 100% (design3DAt100)
	flatCam        [2]float64  // and its camX, camY
	flatSaved      bool        // flatZoom and flatCam are kept
	pend           r3hit       // a container pressed: dragged it moves, let go it is a click (design3DDefers)
	pendOn         bool        //
	pendMS         tea.Mouse   // the press
	warpOn         bool        // the pointer is being put back in the window's middle (design_3d_pointer.go)
	warpAt         time.Time   // when it was asked for
	passing        bool        // a mouse event handed on to the flat board (designPress and the rest)
	plane          float64     // a drag handed on: the mouse is followed on the plane this far out of the wall
	mapX, mapY     int         // the flat board cell the mouse was last followed to
	pw, ph         int         // the pixels: twice the board area's width and height (quarter blocks), or a picture's (kitty)
	kx, ky         float64     // the pixels a unit across and a unit down (2 and 1 in quarter blocks)
	want           [2]int      // the picture's size for the next drawing (see room3DDensity), or 0: quarter blocks
	pix            bool        // the last drawing is a picture (kitty, see design_3d_kitty.go)
	fdraw          []r3draw    // per face: how it is drawn this frame
	pinCards       []r3pinCard // boards' turned pins, cards of their own (design_3d_pins.go)
	polys          [][2]float64
	drawnAt        time.Time // when the last drawing started
	drawnTill      time.Time // and when it ended (see drawNow)
	frameTick      bool      // a room3DFrameMsg is on its way
	chosenCam      r3camKey  // the camera when the middle of its view last chose (see design3DChooseMiddle)
	drawnCam       r3camKey  // the camera at the last drawing
	movedAt        time.Time // when the camera last moved (see room3DMovingPixels)
	light          bool      // the last picture has fewer pixels than the screen: the camera was moving
	sharpTick      bool      // a room3DSharpMsg is on its way
	rgbOut         []byte    // this drawing's picture for kitty, made as it is drawn (see draw)
	rgbDone        bool      // and it was
	gpuDrawn       bool      // the last drawing's picture was drawn on the graphics card (design_3d_gpu.go): no pixel buffers
	base           []uint32  // each pixel's colour before the light
	mul            []float32 // the light on it: base times mul, plus add of the lamp's colour
	add            []float32
	zb             []float32  // the depth of the nearest surface
	plas           []float32  // the walls' plaster at each pixel (see shadePixel)
	face           []int32    // which face it shows (-1: none)
	colDX, rowDY   []float64  // each pixel's ray: fwd + right·colDX[column] + up·rowDY[row] (see dirAt)
	tex            []r3texel  // the back wall's cells (the flat board, widened: see r3room)
	mips           [][]uint32 // the back wall as pictures, each half as big as the one before (see mipmap)
	mipW, mipH     []int
	shadow         []float32 // the blocks' shadows on the back wall, per unit of it
	chars          []r3char  // per screen cell: the letter of a word drawn there (see words)
	cover          []int32   // per back wall cell: the frame (stamp) a block stood on it
	order          []r3near  // the faces with words, nearest first
	marks          []r3run   // a face's marks, put after its words
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
	ds             *designState  // the board it shows (a box's inside: design_3d_box.go)
	plain          bool          // the plain picture, without the text layer (design_3d_text.go): in kitty, unless looked
	looked         bool          // tests: plain is set by hand, not the default (the picture where kitty can show it)
	flat0          []uint32      // the back wall's finest picture with painted letters' cells as their background (design_3d_glyphs.go)
	paint          bool          // this drawing: the plain picture, its words painted on the faces (design_3d_glyphs.go)
	noShapes       bool          // tests: the text layer over the picture, not shape letters (design_3d_shapes.go)
	cells          []r3ink       // per screen cell: the text layer's letter
	seg            [4]float32    // the line being marked: a point on the screen (cells, rows) and its way (see line)
	segOn          bool
	edgeAt         map[r3edgeKey]int32
	edges          []r3edge // the blocks' edges this frame, and the faces either side (see edgeInk)
	world          r3world  // the windows, the town outside and the other lights (design_3d_world.go)
}

// on is whether the board area shows the room in 3D: the first-person camera (or the tests' fixed one).
func (r *room3D) on() bool { return r.free || r.yaw != 0 || r.pitch != 0 }

// hint is the status bar's line for the 3D view.
func (r *room3D) hint() string { return roomHint }

// ---- keys --------------------------------------------------------------------------------

// design3DKey handles the keys of the 3D view: Ctrl + F at any time (into
// 3D and out), and while it shows the arrows (look round), W A S D Q E
// (move), T and F (the walls' and the floor's finish, design_3d_finish.go)
// and Esc (the flat board). While writing in a structure letters type as
// usual (Esc stops writing). Every other key does what it does on the flat
// board, to the chosen structure (the one in the middle of the view). It
// says whether it took the key.
func (m *Model) design3DKey(k tea.Key, name string) (bool, tea.Cmd) {
	d := m.ds()
	r := &d.room
	if name == "ctrl+f" {
		d.editing = partNone
		if r.free {
			m.design3DEsc()
		} else {
			m.design3DFree() // (from the tests' fixed camera too: first person from where it is)
		}
		return true, nil
	}
	if !r.on() {
		return false, nil
	}
	switch name {
	case "left", "right", "up", "down", "shift+left", "shift+right", "shift+up", "shift+down":
		// The arrows look round, as the mouse does, and stop writing as
		// any arrow does. In 3D the board under the room never scrolls,
		// so the blocks stay where they are on the wall.
		d.editing = partNone
		return true, m.design3DTurnKey(k, arrowDir(name))
	case "tab", "shift+tab":
		m.design3DFront() // the camera square in front of the chosen structure, to write on it (design_3d_layers.go)
		if o := d.byID(d.sel); o != nil && isContainer(o.Kind) && d.editing == partNone {
			m.designEdit() // a container: write its name (on the flat board Tab steps through its stacks)
			return true, nil
		}
		return false, nil // then Tab writes in it, as on the flat board
	}
	if d.editing != partNone || d.placing || d.pin.active {
		return false, nil // writing; Alt+N, then a letter; a pin's own letters: as on the flat board
	}
	if name == "esc" {
		if m.design3DModeEsc() {
			return false, nil // a mode to leave first, as on the flat board (designEscape)
		}
		m.design3DEsc()
		return true, nil
	}
	if key := walkKey(name); key != 0 {
		return true, m.design3DWalkKey(k, key)
	}
	if ok := m.design3DLayerKey(name); ok { // = − 0: the lens; Shift + = and Shift + −: the chosen one's layer (design_3d_layers.go)
		return true, nil
	}
	if m.design3DFinishKey(name) { // T, F: the walls' and the floor's finish (design_3d_finish.go)
		return true, nil
	}
	if name == "m" || name == "shift+m" {
		m.soundMute() // the music off or on (design_3d_sound.go)
		return true, nil
	}
	return false, nil
}

// design3DTurnKey turns the camera's look for an arrow pressed. Held in
// kitty, which says when the key is let go, it glides on while held.
func (m *Model) design3DTurnKey(k tea.Key, dir string) tea.Cmd {
	d := m.ds()
	if !m.releases {
		m.design3DTurn(dir, turnStep) // other terminals: each press and repeat turns a step
		return nil
	}
	// The terminal's own repeats only say it is still held.
	m.chord.held[dir] = true
	d.heldAt = time.Now()
	if k.IsRepeat {
		return nil
	}
	m.design3DTurn(dir, turnStep)
	d.holdMods = hold3D // designHold sends the glide steps to design3DHold
	if d.holdTick {
		return nil
	}
	d.holdTick = true
	d.room.turnAt = time.Now().Add(turnStart) // the glide turns from then on
	return tea.Tick(turnStart, func(time.Time) tea.Msg { return designHoldMsg{} })
}

// hold3D is designState.holdMods while arrows held look round in 3D.
const hold3D = "3d"

// walkKeys are the keys that move the camera in 3D.
const walkKeys = "wasdqe"

// walkKey is the moving key that name is (w a s d q e, with or without
// Shift), or 0.
func walkKey(name string) rune {
	name = strings.TrimPrefix(name, "shift+")
	if len(name) == 1 && strings.Contains(walkKeys, name) {
		return rune(name[0])
	}
	return 0
}

// design3DTurn looks step degrees that way (an arrow): turnStep a press,
// a glide step's share of turnRate while held.
func (m *Model) design3DTurn(dir string, step float64) {
	r := &m.ds().room
	if !r.free {
		m.design3DFree() // the tests' fixed camera: first person from where it is
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
}

// lookBy turns the camera: dx degrees right, dy down. It turns right
// round; looking up or down stops just short of straight.
func (r *room3D) lookBy(dx, dy float64) {
	r.look[0] = math.Mod(r.look[0]+dx+540, 360) - 180
	r.look[1] = clampF(r.look[1]+dy, -89, 89)
}

// design3DHold is a glide step while arrows are held in 3D (designHold
// sends it here), or its end when they are let go.
func (m *Model) design3DHold() tea.Cmd {
	d := m.design
	any := false
	r := &m.ds().room // (a box's inside has its own room)
	now := time.Now()
	step := turnRate * math.Max(0, math.Min(now.Sub(r.turnAt).Seconds(), 0.05))
	if now.After(r.turnAt) {
		r.turnAt = now
	}
	if m.inDesign() && !m.help && time.Since(d.heldAt) <= holdLost {
		for _, dir := range []string{"left", "right", "up", "down"} {
			if m.chord.held[dir] {
				m.design3DTurn(dir, step)
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
	return tea.Tick(moveEvery, func(time.Time) tea.Msg { return designHoldMsg{} })
}

// ---- the first-person camera ------------------------------------------------------------

// room3DWalkMsg is the next step while W A S D Q E are held.
type room3DWalkMsg struct{}

// design3DFree goes into 3D: the first-person camera, where the flat
// board's view is (square to the wall, the wall filling the board area
// cell for cell), or where the tests' fixed camera is: the view does not
// jump.
func (m *Model) design3DFree() {
	d := m.ds()
	r := &d.room
	was := m.design3DAt100() // 3D always shows the board at 100% (design_3d_layers.go)
	rm := r.area(d, m.width, m.boardH(), d.z())
	v := r.orbitView(&rm)
	c := v.ray(float64(rm.w)/2, float64(rm.h)).norm() // straight ahead on the screen
	r.look = [2]float64{math.Atan2(c.x, -c.z) * 180 / math.Pi, math.Asin(clampF(-c.y, -1, 1)) * 180 / math.Pi}
	r.look[1] = clampF(r.look[1], -89, 89)
	r.pos, r.ff, r.rm = v.cam, v.f, rm
	r.pullBack(v, was) // as far back as the flat board's zoom showed the wall: no jump
	r.ff0, r.lens, r.anchor = r.ff, 1, [2]float64{d.camX, d.camY}
	r.startPos, r.startLook = r.pos, r.look
	r.freeK = r.outK() // the blocks as far out as they were: they do not jump
	r.free = true
	m.say("3D: the mouse looks, W A S D Q E move, clicks act at the cross.  Esc: flat board.")
}

// design3DWalkKey takes W A S D Q E pressed in 3D.
func (m *Model) design3DWalkKey(k tea.Key, key rune) tea.Cmd {
	r := &m.ds().room
	if !r.free {
		m.design3DFree() // the tests' fixed camera: first person from where it is
	}
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
	return tea.Tick(moveEvery, func(time.Time) tea.Msg { return room3DWalkMsg{} })
}

// design3DKeyUp: a key let go (kitty): W A S D Q E stop moving.
func (m *Model) design3DKeyUp(k tea.Key) {
	if m.design == nil {
		return
	}
	r := &m.design.room
	if key := unicode.ToLower(rune(k.Code)); r.held != nil && strings.ContainsRune(walkKeys, key) {
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
	return tea.Tick(moveEvery, func(time.Time) tea.Msg { return room3DWalkMsg{} })
}

// walk moves the camera for secs seconds with keys held: W and S forward
// and back the way it looks (up and down too, when it looks up or down),
// A and D sideways, level, and Q and E straight up and down.
func (r *room3D) walk(keys map[rune]bool, secs float64, fast bool) {
	ly, lp := r.look[0]*math.Pi/180, r.look[1]*math.Pi/180
	ahead := r3vec{math.Sin(ly) * math.Cos(lp), -math.Sin(lp), -math.Cos(ly) * math.Cos(lp)}
	side := r3vec{math.Cos(ly), 0, math.Sin(ly)}
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
		case 'q':
			way = way.add(r3vec{0, 1, 0})
		case 'e':
			way = way.sub(r3vec{0, 1, 0})
		}
	}
	if way.dot(way) < 1e-9 {
		return
	}
	speed := walkSpeed * math.Max(r.rm.big, 20)
	if fast {
		speed *= walkFast
	}
	r.pos = r.inRoom(r.pos.add(way.norm().mul(speed * secs)))
}

// inRoom is point p kept inside the room's walls, floor and ceiling (as
// last drawn), a little off them: the camera cannot walk out of the room
// into the dark round it.
func (r *room3D) inRoom(p r3vec) r3vec {
	rm := &r.rm
	if rm.wx1 <= rm.wx0 || rm.cy <= rm.fy || rm.fz <= 0 {
		return p // not drawn yet
	}
	const off = 1.5
	return r3vec{clampF(p.x, rm.wx0+off, rm.wx1-off), clampF(p.y, rm.fy+off, rm.cy-off), clampF(p.z, off, rm.fz-off)}
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
	if pw == 0 || col < 0 || row < 0 || col >= r.rm.w || row >= r.rm.h || !r.gpuDrawn && len(r.face) != pw*r.ph {
		return r3hit{own: -1}
	}
	x, y := float64(col)+0.5, float64(2*row)+1
	best, bestF, bestZ := -1, int32(-1), float32(0)
	for _, i := range r.cellFour(col, row) {

		f, z := r.hitAt(i)
		if f >= 0 && int(f) < len(r.faces) && (best < 0 || z < bestZ) {
			best, bestF, bestZ = i, f, z
		}
	}
	if best < 0 {
		return r3hit{own: -1}
	}
	fc := &r.faces[bestF]
	v := &r.view
	p := v.cam.add(r.dirAt(best).mul(float64(bestZ)))
	// (x, y) on that face's plane: the point the mouse is on.
	dir := v.ray(x, y)
	if nd := fc.n.dot(dir); math.Abs(nd) > 1e-12 {
		if t := (fc.k - fc.n.dot(v.cam)) / nd; t > 0 {
			p = v.cam.add(dir.mul(t))
		}
	}
	return r.hitOn(int(bestF), p, v)
}

// aim is the board area cell the cross is on (the middle of the camera's
// view).
func (r *room3D) aim() (col, row int) {
	return r.rm.w / 2, r.rm.h / 2
}

// pickAim is what the cross is on (see pickAt): what the middle of the
// camera's view is on, the camera as it is now, the blocks as last drawn,
// exactly as the chosen structure is found (middleHit). In 3D the cross
// is the pointer.
func (r *room3D) pickAim() r3hit {
	fi, p, ok := r.middleFace()
	if !ok {
		return r3hit{own: -1}
	}
	v, _ := r.nowView()
	return r.hitOn(fi, p, &v)
}

// hitOn is the r3hit of point p on face fi, seen by camera v.
func (r *room3D) hitOn(fi int, p r3vec, v *r3view) r3hit {
	fc := &r.faces[fi]
	h := r3hit{ok: true, face: int32(fi), own: fc.own, p: p, dist: p.sub(v.cam).dot(v.fwd)}
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

// design3DPress: a press in the 3D view. It acts where the cross is, not
// where the pointer is (the pointer is hidden). The left button with the
// cross on a block (or on the wall where the flat board has something: a
// pulled out drawer, the pen) is a press on its flat board cell, and a drag
// after it turns the camera and drags it along under the cross; on the
// empty room a drag looks round, as the mouse with no button does, and let
// go without a drag it is a click on the empty board there. The right
// button rubs out with the pen out; else the other buttons do nothing. It says whether it took the press.
func (m *Model) design3DPress(ms tea.Mouse) (bool, tea.Cmd) {
	d := m.ds()
	r := &d.room
	if !r.on() || r.passing {
		return false, nil
	}
	col, row := ms.X, ms.Y-m.boardTop()
	hit := r.pickAim()
	r.camDrag, r.clickOn = camNone, false
	r.pressX, r.pressY, r.lastX, r.lastY = col, row, col, row
	fx, fy := m.mouseAt(ms)
	r.hoverX, r.hoverY, r.hoverOn = fx, fy-float64(m.boardTop()), true // a drag turns from here (design3DLook)
	r.edge = [2]int{}
	switch ms.Button {
	case tea.MouseRight:
		if m.ds().pen >= 0 && hit.flat {
			return true, m.design3DPass(ms, hit) // the pen out: the right button rubs out, as on the flat board
		}
		return true, nil
	case tea.MouseLeft:
	default:
		return true, nil
	}
	if hit.flat && hit.own >= 0 && m.design3DDefers(hit) {
		r.pend, r.pendOn, r.pendMS = hit, true, ms // a drag moves it, a click opens it (design_3d_layers.go)
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
	return d.hit(hit.sx, hit.sy).o != nil || d.pen >= 0 || d.groupMode || d.linkMode || d.labelMode ||
		ms.Mod.Contains(tea.ModCtrl) || ms.Mod.Contains(tea.ModShift)
}

// design3DPass hands a press on to the flat board, as if it were on the
// flat board cell under the cross. A drag after it follows the cross on
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

// design3DThrough is the flat board cell the cross is over in the 3D view
// (the camera as it is now), on the plane of the drag (see design3DPass),
// as a mouse position with ms's buttons.
func (m *Model) design3DThrough(ms tea.Mouse) tea.Mouse {
	r := &m.ds().room
	if v, ok := r.nowView(); ok && v.f > 0 {
		dir := v.ray(v.px, v.py)
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

// design3DMotion: the mouse moved with the left button down on the empty
// room: the camera looks round.
func (m *Model) design3DMotion(ms tea.Mouse) tea.Cmd {
	r := &m.ds().room
	col, row := ms.X, ms.Y-m.boardTop()
	var cmd tea.Cmd
	if r.camDrag == camTurn {
		dx, dy := col-r.pressX, row-r.pressY
		if dx == 0 && dy == 0 && r.clickOn {
			return nil // not moved yet: it may still be a click
		}
		r.clickOn = false
		_, cmd = m.design3DLook(ms) // as the mouse with no button: from where it last was
	}
	r.lastX, r.lastY = col, row
	return cmd
}

// hoverJump is how far (cells) the mouse may move between two reports and
// still turn the camera: further, it has been out of the window and come
// back in somewhere else, so the camera does not jump.
const hoverJump = 16

// design3DLook: the mouse moved in the 3D view (no button, or dragging
// from a press). That is mouse look: the camera stays where it is and turns the way the mouse
// goes. The first move after a press, a jump or the flat board only notes
// where the mouse is. Resting at the window's edge it turns on by itself
// (see design3DEdge). It says whether the camera turned, and the ticks of
// the edge turning to start.
func (m *Model) design3DLook(ms tea.Mouse) (bool, tea.Cmd) {
	r := &m.ds().room
	fx, fy := m.mouseAt(ms)
	fy -= float64(m.boardTop())
	dx, dy := fx-r.hoverX, fy-r.hoverY
	if r.lens > 1 { // zoomed in: the mouse turns as much less, so it aims as finely
		dx, dy = dx/r.lens, dy/r.lens
	}
	was := r.hoverOn
	r.hoverX, r.hoverY, r.hoverOn = fx, fy, r.on()
	if was && m.pointerWarped(fx, fy+float64(m.boardTop()), dx, dy) {
		r.edge = [2]int{}
		return false, nil // the pointer put back in the middle: it only notes where it is
	}
	if !was || !r.on() || r.passing {
		r.edge = [2]int{}
		return false, nil
	}
	if math.Abs(dx) > hoverJump || math.Abs(dy) > hoverJump {
		return false, m.design3DEdge(fx, fy+float64(m.boardTop()), 0, 0)
	}
	edge := m.design3DEdge(fx, fy+float64(m.boardTop()), dx, dy)
	if dx == 0 && dy == 0 {
		return false, edge
	}
	if !r.free {
		m.design3DFree()
	}
	m.pointerRecenter(fx, fy+float64(m.boardTop())) // far from the middle: put back there, so it never meets the edge
	return true, tea.Batch(r.turnBy(dx*turnPerCol, dy*turnPerRow), edge)
}

// design3DRelease: the button let go after a press on the empty room or a
// camera drag. Not dragged, a press with the cross on the empty wall is a
// click on the empty board there (a double click places a structure, as on the flat board).
func (m *Model) design3DRelease(ms tea.Mouse) tea.Cmd {
	r := &m.ds().room
	was, click := r.camDrag, r.clickOn
	r.camDrag, r.clickOn = camNone, false
	if was == camTurn && click && r.on() {
		cmd := m.design3DPass(ms, r.click)
		return tea.Batch(cmd, m.designRelease())
	}
	if was == camTurn && r.lastX == r.pressX && r.lastY == r.pressY && r.on() {
		// A click on the floor, the ceiling or a side wall: as a click on
		// the empty board, it lets go of what is chosen.
		d := m.ds()
		if d.sel >= 0 || len(d.multi) > 0 {
			d.sel, d.editing = -1, partNone
			d.clearMulti()
			d.dirty = true
		}
	}
	return nil
}

// design3DWheel: the wheel does nothing to the camera in 3D. With Shift
// and the cross on a block it does what it does on the flat board
// (scrolls a page's text). It says whether it took it.
func (m *Model) design3DWheel(ms tea.Mouse) (bool, tea.Cmd) {
	r := &m.ds().room
	if !r.on() || r.passing {
		return false, nil
	}
	if ms.Mod.Contains(tea.ModShift) {
		if hit := r.pickAim(); hit.flat && hit.own >= 0 {
			r.passing = true
			defer func() { r.passing = false }()
			ms.X, ms.Y = hit.sx, hit.sy+m.boardTop()
			return true, m.designWheel(ms)
		}
		return true, nil
	}
	switch ms.Button { // the wheel zooms the lens, round the middle of the screen (design_3d_layers.go)
	case tea.MouseWheelUp:
		m.design3DLayerKey("=")
	case tea.MouseWheelDown:
		m.design3DLayerKey("-")
	}
	return true, nil
}

// design3DHover is the flat board cell the cross is on in the 3D view
// (for Space, which opens what is under the pointer), and whether there is one.
func (m *Model) design3DHover(ms tea.Mouse) (tea.Mouse, bool) {
	hit := m.ds().room.pickAim()
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
	if r.frame != "" && r.frameW == w && r.frameH == h && !r.drawNow(start) {
		// Drawn a moment ago: the room is drawn again once room3DEvery has
		// gone by (design3DAfter asks for that), with every key and mouse
		// move up to then in it, rather than once for each of them.
		sb.WriteString(r.frame)
		return
	}
	r.frameTick = false
	// The flat board, which the wall and the fronts show, without what
	// floats over it (the mini map, the list of name plates).
	corner, plates := d.mapCorner, d.platesOpen
	d.mapCorner, d.platesOpen = 0, false
	r.cabinets(d, w, h) // how open each cabinet is this frame, for the flat drawing and the 3D doors alike
	pic := room3DPixels && w <= len(kittyMarks) && h <= len(kittyMarks)
	if !r.looked {
		r.plain = pic // where kitty shows pictures the room is a picture, its words painted on
	}
	r.paint = pic && r.plain
	clean3D = r.paint // the structures drawn without the flat board's counts and tabs
	all := d.octs
	if d.layered() {
		d.octs = d.onWall() // the wall without what stands out on layers (layerCopies, design_3d_layers.go)
	}
	c := m.designCanvas()
	d.octs = all
	clean3D = false
	d.mapCorner, d.platesOpen = corner, plates
	r.ds = d
	m.design3DZoomGuard() // the flat board's zoom changed in 3D: the lens takes it (design_3d_layers.go)
	r.keepCamera(d, w, h) // the window or the board under it changed: the camera stays on the same spot (design_3d_layers.go)
	rm := r.area(d, w, h, d.z())
	rm.walls, rm.floor = d.walls, d.floor // the finishes (design_3d_finish.go)
	r.takeRoom(d, c, &rm)
	r.mipmap(&rm)
	r.want, r.light = [2]int{}, false
	if r.shapes() {
		// Shape letters (design_3d_shapes.go): 8 × 16 pixels a cell to
		// fit each one to.
		r.want = [2]int{w * geoSX, h * geoSY}
		if kittyPic.isShown() {
			kittyPic.remove()
		}
	} else if room3DPixels && w <= len(kittyMarks) && h <= len(kittyMarks) {
		// A picture (kitty): one pixel for each of the screen's when still.
		// While the camera moves, a big window's picture has fewer
		// (room3DMovingPixels), which kitty stretches: kitty takes a
		// picture in a time that grows with its pixels, and the full
		// screen's would hold the frames to 60 a second.
		limit := room3DMaxPixels
		if room3DRemote {
			limit = room3DRemotePixels // over SSH every pixel goes down the line
		}
		if k := r.camKey(w, h); k != r.drawnCam {
			r.drawnCam, r.movedAt = k, start
		}
		cw, ch := termCell(m.width, m.height)
		full := w * cw * h * ch
		if start.Sub(r.movedAt) < room3DStill {
			limit = min(limit, room3DMovingPixels)
		}
		r.want[0], r.want[1] = room3DDensity(w, h, cw, ch, limit)
		r.light = r.want[0]*r.want[1] < full
	}
	var fb strings.Builder
	r.rgbOut, r.rgbDone = nil, false
	if r.want[0] > 0 && !r.shapes() {
		// The picture's bytes are made as it is drawn (see draw).
		buf := kittyPic.buffer()
		if n := 3 * r.want[0] * r.want[1]; cap(buf) >= n {
			r.rgbOut = buf[:n]
		} else {
			r.rgbOut = make([]byte, n)
		}
	}
	r.draw(&rm, d.z(), &fb)
	if traceOn() { // where the camera is, for SY_3D_TRACE (design_3d_trace.go)
		traceLine("cam pos %.1f,%.1f,%.1f look %.1f,%.1f ff %.1f lens %.2f board %dx%d cam %.1f,%.1f zoom %d pic %dx%d",
			r.pos.x, r.pos.y, r.pos.z, r.look[0], r.look[1], r.ff, r.lensK(), w, h, d.camX, d.camY, d.zoom, r.pw, r.ph)
	}
	if was := d.sel; r.chosenCam == (r3camKey{}) && m.design3DChooseMiddle() && d.sel != was {
		// The room's first drawing: only now can the middle of the camera
		// choose, and the chosen structure looks chosen, so draw it again
		// (once: it has chosen now).
		r.rgbOut, r.frame, r.drawnAt, r.drawnTill = nil, "", time.Time{}, time.Time{}
		m.design3DRows(sb)
		return
	}
	r.frame, r.frameW, r.frameH = fb.String(), w, h
	if r.pix && !r.shapes() {
		rgb := r.rgbOut
		if !r.rgbDone {
			rgb = r.picture(rgb)
		}
		r.crosshair(rgb)
		if room3DSeeRGB != nil {
			room3DSeeRGB(rgb, r.pw, r.ph)
		}
		kittyPic.show(&kittyFrame{rgb: rgb, pw: r.pw, ph: r.ph, cols: w, rows: h})
	}
	r.rgbOut = nil
	r.drawnAt, r.drawnTill = start, time.Now()
	r.cost = r.drawnTill.Sub(start)
	traceDraw(r.cost)
	sb.WriteString(r.frame)
}

// How often the room is drawn at most. A drawing takes some milliseconds,
// and keys and mouse moves come quicker than that: each is taken at once,
// but the room is drawn with all of them in it, not once for each, so the
// picture keeps up with the hand instead of falling behind it.
const (
	room3DEvery = 4 * time.Millisecond // from the start of one drawing to the next: at most 250 a second
	room3DGap   = 2 * time.Millisecond // and after one ends: time to take what came in while it was drawn
)

// While the camera moves, the picture has at most room3DMovingPixels
// (about a 1366×768 screen's), so kitty takes it in a few milliseconds and
// the room keeps up with the hand in a window of any size; room3DStill
// after the camera stops, it is drawn sharp, every pixel.
const (
	room3DMovingPixels = 1_000_000
	room3DStill        = 120 * time.Millisecond
)

// room3DSharpMsg: the camera may have stopped; draw the room sharp if so.
type room3DSharpMsg struct{}

// room3DSeeRGB, when set (tests), is shown each kitty picture as it goes.
var room3DSeeRGB func(rgb []byte, pw, ph int)

// room3DPace: the room is drawn at most every room3DEvery. Set in
// main.go; off in tests, which look at each drawing straight after a key.
var room3DPace = false

// drawNow is whether the room may be drawn again now (see room3DEvery).
func (r *room3D) drawNow(now time.Time) bool {
	return r.paced(now) && kittyPic.ready()
}

// paced is whether room3DEvery has gone by since the last drawing.
func (r *room3D) paced(now time.Time) bool {
	return !room3DPace || now.Sub(r.drawnAt) >= room3DEvery && now.Sub(r.drawnTill) >= room3DGap
}

// room3DFrameMsg: time to draw the room again, with what changed since
// the last drawing (see drawNow).
type room3DFrameMsg struct{}

// design3DAfter runs after every message (see Update): when the View
// after it will not draw the room again yet (drawn a moment ago), it asks
// for a room3DFrameMsg once it may; and it has kitty delete the picture
// once the board does not show the room.
func (m *Model) design3DAfter() tea.Cmd {
	d := m.design
	if d != nil && !d.room.on() {
		d.room.chosenCam = r3camKey{} // turned again later, the middle of the camera chooses afresh
	}
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
	m.design3DChooseMiddle() // the camera moved: what the middle of its view is on is chosen
	var sharp tea.Cmd
	if r.light && !r.sharpTick {
		// The last picture was drawn light, the camera moving: drawn
		// sharp once it has stopped.
		r.sharpTick = true
		wait := max(room3DStill-time.Since(r.movedAt), 0) + time.Millisecond
		sharp = tea.Tick(wait, func(time.Time) tea.Msg { return room3DSharpMsg{} })
	}
	if r.frameTick || d.hoverOnly || r.frame == "" {
		return sharp
	}
	now := time.Now()
	if r.paced(now) {
		if !kittyPic.ready() {
			// kitty still takes in a picture and the next is waiting to
			// go: the room is drawn again once that goes (kittyOut.wakeUp).
			r.frameTick = true
		}
		return sharp
	}
	// Drawn a moment ago: a tick once room3DEvery has gone by.
	wait := max(room3DEvery-now.Sub(r.drawnAt), room3DGap-now.Sub(r.drawnTill), time.Millisecond)
	r.frameTick = true
	return tea.Batch(sharp, tea.Tick(wait, func(time.Time) tea.Msg { return room3DFrameMsg{} }))
}

// ---- Esc -----------------------------------------------------------------------------------

// design3DModeEsc is whether Esc (or Alt twice) has something of the flat
// board's to leave first: Alt+N, several chosen, the name plates, group,
// rack, label or link mode, the pen, a pin, or a box's chosen thing or
// layer. Then it does that, as on the flat board; else it is the 3D
// view's own Esc (design3DEsc).
func (m *Model) design3DModeEsc() bool {
	d := m.ds()
	if d.placing || len(d.multi) > 0 || d.platesOpen || d.groupMode || d.opened.active ||
		d.labelMode || d.linkMode || d.pen >= 0 || d.pin.active {
		return true
	}
	if b := d.byID(d.sel); b != nil && b.Kind == kBox {
		_, it := d.chosenBoxThing()
		return it != nil || d.boxView(b) > 0
	}
	return false
}

// design3DEsc leaves 3D: the flat board, as it was.
func (m *Model) design3DEsc() {
	r := &m.ds().room
	r.free, r.walking, r.camDrag = false, false, camNone
	r.edge = [2]int{}
	clear(r.held)
	r.yaw, r.pitch, r.near, r.pan = 0, 0, 0, r3vec{}
	m.design3DFlatZoom() // the flat board's own zoom back (design_3d_layers.go)
	m.say("Flat board.")
}

// ---- the middle of the camera chooses --------------------------------------------------------
//
// In 3D the chosen structure is whatever the middle of the camera's view
// is on: when the camera moves (the mouse, the arrows, walking), the
// structure there is chosen, and on the empty room none is. A click still
// chooses what it is on, until the camera moves again.

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

// nowView is the camera as it is now (it may have moved since the last
// drawing), in the room of the last drawing; false before any drawing.
func (r *room3D) nowView() (r3view, bool) {
	if r.rm.w == 0 || len(r.faces) == 0 {
		return r3view{}, false
	}
	rm := r.rm
	if r.free {
		return r.freeView(&rm), true
	}
	return r.orbitView(&rm), true
}

// middleHit is what the middle of the camera's view is on: the structure
// (its ID, -1: the room), and the point. The camera is taken as it is now,
// the blocks as they were last drawn (moving the camera does not move
// them). False before any drawing.
func (r *room3D) middleHit() (own int, p r3vec, ok bool) {
	fi, p, ok := r.middleFace()
	if !ok {
		return -1, p, false
	}
	return r.faces[fi].own, p, true
}

// middleFace is the face the middle of the camera's view is on (see
// middleHit), and the point.
func (r *room3D) middleFace() (fi int, p r3vec, ok bool) {
	v, ok := r.nowView()
	if !ok {
		return -1, r3vec{}, false
	}
	dir := v.ray(v.px, v.py)
	best := math.Inf(1)
	fi = -1
	for i := range r.faces {
		fc := &r.faces[i]
		if fc.n.dot(v.cam)-fc.k <= 1e-9 && !fc.both {
			continue // it faces away, so it is not drawn
		}
		nd := fc.n.dot(dir)
		if math.Abs(nd) < 1e-12 {
			continue
		}
		t := (fc.k - fc.n.dot(v.cam)) / nd
		if t <= 0.25 || t >= best {
			continue
		}
		q := v.cam.add(dir.mul(t))
		if !inFace(r.pts[fc.p0:fc.p0+fc.pn], fc.n, q) {
			continue
		}
		best, fi, p = t, i, q
	}
	return fi, p, fi >= 0
}

// inFace is whether point q, on the plane of a flat convex face with
// corners pts and normal n, is inside it (or on its edge).
// hitAt is the face pixel i of the last drawing shows, and how far along
// its ray (-1: none): from the pixel buffers, or when the graphics card
// drew it (and there are none), cast as depthRows finds it.
func (r *room3D) hitAt(i int) (int32, float32) {
	if !r.gpuDrawn {
		return r.face[i], r.zb[i]
	}
	v := &r.view
	dir := r.dirAt(i)
	best, bf := float32(math.MaxFloat32), int32(-1)
	for fi := range r.faces {
		fc := &r.faces[fi]
		if !fc.seen {
			continue
		}
		fd := &r.fdraw[fi]
		nd := fd.n.dot(dir)
		if nd >= -1e-12 {
			continue
		}
		t := fd.num / nd
		if t <= 0 || float32(t) >= best {
			continue
		}
		q := v.cam.add(dir.mul(t))
		if q.sub(v.cam).dot(v.fwd) < 0.25 || !inFace(r.pts[fc.p0:fc.p0+fc.pn], fc.n, q) {
			continue // cut off in front of the camera, or not on the face
		}
		best, bf = float32(t), int32(fi)
	}
	return bf, best
}

func inFace(pts []r3vec, n, q r3vec) bool {
	sign := 0.0
	for i := range pts {
		a, b := pts[i], pts[(i+1)%len(pts)]
		s := b.sub(a).cross(q.sub(a)).dot(n)
		if s*sign < -1e-9 {
			return false
		}
		if math.Abs(s) > 1e-9 {
			sign = s
		}
	}
	return true
}

// design3DChooseMiddle chooses what the middle of the camera's view is on,
// once the camera has moved since it last chose (not while writing). It
// says whether it could look (there has been a drawing).
func (m *Model) design3DChooseMiddle() bool {
	d := m.ds()
	r := &d.room
	key := r.camKey(m.width, m.boardH())
	if key == r.chosenCam || d.editing != partNone {
		return true
	}
	own, _, ok := r.middleHit()
	if !ok && len(r.faces) == 0 {
		return false
	}
	r.chosenCam = key
	if own < 0 || d.byID(own) == nil {
		if d.sel >= 0 {
			d.sel, d.selLink = -1, -1
			d.dirty = true
		}
		return true
	}
	if own != d.sel {
		d.sel, d.selLink = own, -1
		d.dirty = true
		m.msg = "" // the top bar names it; the status bar says what keys do
	}
	return true
}

// design3DMiddle is the flat board cell the middle of the camera's view
// is on, on the back wall (for Alt+N, pasting: what is placed goes where
// the camera looks), and whether the room is in 3D and drawn.
func (m *Model) design3DMiddle() (sx, sy int, ok bool) {
	r := &m.ds().room
	if !r.on() {
		return 0, 0, false
	}
	v, ok := r.nowView()
	if !ok {
		return 0, 0, false
	}
	dir := v.ray(v.px, v.py)
	if math.Abs(dir.z) < 1e-9 {
		return 0, 0, false
	}
	t := -v.cam.z / dir.z // the back wall: z = 0
	if t <= 0 {
		return 0, 0, false
	}
	p := v.cam.add(dir.mul(t))
	sx = int(math.Floor(clampF(p.x+r.rm.hw, 0, float64(r.rm.w-1))))
	sy = int(math.Floor(clampF((r.rm.hh-p.y)/2, 0, float64(r.rm.h-1))))
	return sx, sy, true
}

// screenMiddle is the board area's middle as a flat board cell: in 3D,
// where the middle of the camera looks on the wall (see design3DMiddle).
func (m *Model) screenMiddle() (sx, sy int) {
	if sx, sy, ok := m.design3DMiddle(); ok {
		return sx, sy
	}
	return m.width / 2, m.boardH() / 2
}

// design3DCell is the flat board cell under board area cell (col, row):
// in 3D, the cell of the block or wall the cross is on (the cross is the
// pointer); else the same cell.
func (m *Model) design3DCell(col, row int) (sx, sy int) {
	if r := &m.ds().room; r.on() {
		if hit := r.pickAim(); hit.flat {
			return hit.sx, hit.sy
		}
	}
	return col, row
}

// crosshair marks the middle of the camera's view on the picture (rgb,
// r.pw × r.ph): a small light cross with a dark rim, with a gap in its
// middle, so what it is on (the chosen structure) shows through.
func (r *room3D) crosshair(rgb []byte) {
	pw, ph := r.pw, r.ph
	if len(rgb) != 3*pw*ph || pw < 40 || ph < 40 {
		return
	}
	cx, cy := pw/2, ph/2
	arm := max(8, pw/max(r.rm.w, 1)) // about a cell
	gap := max(2, arm/3)
	set := func(x, y int, c byte) {
		if x >= 0 && y >= 0 && x < pw && y < ph {
			i := 3 * (y*pw + x)
			rgb[i], rgb[i+1], rgb[i+2] = c, c, c
		}
	}
	// The rim first, then the light lines over it: each arm is 2 pixels
	// wide, its rim 1 pixel round it.
	for pass, c := range [2]byte{20, 235} {
		e := 1 - pass // the rim reaches a pixel further
		for d := gap - e; d <= arm+e; d++ {
			for w := -1 - e; w <= e; w++ {
				set(cx+d, cy+w, c)
				set(cx-d-1, cy+w, c)
				set(cx+w, cy+d, c)
				set(cx+w, cy-d-1, c)
			}
		}
	}
}
