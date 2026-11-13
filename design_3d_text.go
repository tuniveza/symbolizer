package main

// ---- the room drawn in text ------------------------------------------------------------
//
// The flat board is made of text, and so is the room in 3D: over the
// picture (or the quarter blocks) goes a layer of the terminal's own
// letters. The edges of every block, the room's corners and the box lines
// of the flat drawings are lines of box drawing letters (─ │ ┌ ┐ ╱ ╲ …),
// whichever way they run on the screen, each at its own height in the
// cell (⎺ ⎻ ─ ⎼ ⎽), so a sloping one rises smoothly; the floor's planks are
// lines along their joins, the walls' rails and skirting board lines along
// the walls and their paper's stripes dotted ones (┊), the ceiling has a dot
// (·) every few units, and a block's sides are shaded with ░ or ▒ by the
// light on them. Wherever the
// board or a block's front is seen about as big as on the flat board (or
// bigger), the board's own dots, shades and box lines show as they do flat,
// so a front seen straight on is its flat drawing, letter for letter.
//
// Each letter's colour comes from the surface it is on, lit, and its
// cell's background is the mean of the pixels under it, so from a distance
// it is still the lit room. The words go over all of it (see words). P
// turns the layer off, for the plain picture, and on again.
//
// A line is worked out in the room and drawn cell by cell where it shows
// on the screen: only where a pixel by it shows one of its own faces, so a
// nearer block hides it as it hides the surface. Lines too close together
// to tell apart (far planks, a small front's box lines) are left to the
// picture.

import "math"

// What a mark of the text layer is. A higher one in a cell takes the place
// of a lower one; two lines of the same kind in a cell join (─ and │ make ┼).
const (
	inkShade   = iota + 1 // a block's side: ░ or ▒
	inkDot                // a dot or a sign of the flat board (· ░ ⣿ …), the ceiling's dots
	inkTexture            // the planks' joins, the walls' rails and stripes
	inkLine               // the edges of the blocks, the room's corners
	inkBoard              // the flat drawings' own box lines, which a block's outline gives way to
)

// How a line is drawn (from the flat board's own letters, see boxStyle);
// a cell with lines of two styles takes both.
const (
	styleHeavy  = 1 // ━ ┃ ┏
	styleDouble = 2 // ═ ║ ╔
	styleDashed = 4 // ┄ ┊
	styleRound  = 8 // ╭ ╮ ╰ ╯
)

// The room's faces, which scene puts first, in this order.
const (
	faceBoard = iota
	faceBack
	faceFloor
	faceLeft
	faceRight
	faceFront // the wall behind the camera
	faceCeiling
)

// noFace is an unused place in r3seg.faces (r.face holds −1 for none).
const noFace = -2

// r3ink is what the text layer puts in a screen cell.
type r3ink struct {
	stamp int32 // the frame it was put there (r.stamp): older ones are not there
	level int8  // inkShade … inkLine
	arms  uint8 // a line's arms: 1 up, 2 down, 4 left, 8 right (as boxArms)
	diag  uint8 // a slanting line: 1 ╱, 2 ╲
	lone  uint8 // the arms of a line only one point of which is in the cell
	style uint8
	// Where in the cell a line across (or down) runs: the sum of the
	// points seen in it, 0 its top (left) to 1 its bottom (right), and how
	// many; a line across or down only is drawn there (⎺ ⎻ ─ ⎼ ⎽, ▏ │ ▕).
	pos    float32
	posN   uint16
	need   uint8      // how far its colour's lightness is kept from the background's (see clearInkBy)
	depth  float32    // how far in front of the camera it is: the nearest mark gives the colour
	col    uint32     // its colour, lit
	face   int32      // the face it is on (the nearest mark's): the cell's background is that face's colour
	glyph  string     // a dot's or a shade's letter (a line's comes from its arms)
	seg    [4]float32 // the nearest line's point on the screen and way, for its shape letter (design_3d_shapes.go)
	hasSeg bool
	// The cell as written (see finishInk): its letter and colours.
	s      string
	fg, bg uint32
}

// r3mark is what a line or a dot puts in the cells it is seen in.
type r3mark struct {
	level int8
	style uint8
	need  uint8
	col   uint32 // before the light
	face  int32  // the face it is on
	glyph string
}

// r3seg is a line of the text layer, in the room, from a to b. It shows
// where a pixel by it shows one of its faces, and where a line next to it
// (off away) would be at least sep cells away on the screen (sep 0: always).
type r3seg struct {
	a, b   r3vec
	off    r3vec
	sep    float64
	minLen float64 // shorter than this on the screen (cells): not drawn (a knob's edges)
	faces  [3]int32
	plain  bool // on the back wall: only where the board is empty
	m      r3mark
}

// textLayer works out the text layer for the drawing just made (its
// pixels, faces and light), before the words go over it.
func (r *room3D) textLayer(c *r3shade) {
	w, h := r.rm.w, r.rm.h
	if len(r.cells) != w*h {
		r.cells = make([]r3ink, w*h)
	}
	r.sideShades(c)
	r.roomInk(c)
	r.boardInk(c)
	r.edgeInk(c)
}

// finishInk works out each cell's letter and colours, after the words
// (a cell with a word's letter keeps it).
func (r *room3D) finishInk() {
	w := r.rm.w
	for i := range r.cells {
		k := &r.cells[i]
		if k.stamp != r.stamp {
			continue
		}
		k.s = ""
		if r.chars[i].stamp == r.stamp {
			continue
		}
		s := k.glyph
		if s == "" {
			s = lineGlyph(k, r.pix)
			if r.shapes() && k.hasSeg && (k.arms&3 == 0 || k.arms&12 == 0) {
				// A line at its own angle and place in the cell
				// (design_3d_shapes.go); a corner or a crossing keeps its box letter.
				g := k.seg
				if l := geoLineFor(float64(g[0]), float64(g[1]), float64(g[2]), float64(g[3]), i%w, i/w); l != "" {
					s = l
				} else if k.diag != 0 {
					s = "" // a cell the sloping line only steps through: the picture's own
				}
			}
		}
		if s == "" {
			continue
		}
		if r.shapes() && k.level <= inkLine && r.cellSpread(i%w, i/w) > splitCell {
			// In shape letters a cell an edge of the picture runs through
			// is that edge (design_3d_shapes.go), not one flat colour.
			continue
		}
		if r.pix && k.level < inkLine && r.cellSpread(i%w, i/w) > evenCell {
			// In the picture a letter's cell is one flat colour: a texture's
			// letter goes only where the picture under it is about even
			// already, so the walls and the floor show no patches.
			continue
		}
		bg := r.cellMean(i%w, i/w, k.face)
		fg := k.col
		switch {
		case k.level == inkShade:
			fg = mix32(bg, 0, 0.3) // a shade a little darker than the side under it
		case k.need > 0:
			fg = clearInkBy(fg, bg, float64(k.need))
		}
		k.s, k.fg, k.bg = s, fg, bg
	}
}

// evenCell is how far apart (in lightness, 0 to 255) the lightest and
// darkest pixels of a cell may be for a texture's letter to go there.
const evenCell = 14

// splitCell is how far apart they may be for a line's letter to go there
// when the room is drawn in shape letters: more, and the cell shows the
// edge itself.
const splitCell = 40

// cellSpread is how far apart the lightest and darkest pixels of screen
// cell (col, row) are, in lightness (0 to 255).
func (r *room3D) cellSpread(col, row int) float64 {
	lo, hi := math.Inf(1), math.Inf(-1)
	r.cellSamples(col, row, func(i int) {
		cr, cg, cb := chans(r.pixel(i))
		l := 0.299*cr + 0.587*cg + 0.114*cb
		lo, hi = math.Min(lo, l), math.Max(hi, l)
	})
	return hi - lo
}

// inkAt is the text layer's letter in screen cell i of the last drawing,
// if it has one.
func (r *room3D) inkAt(i int) *r3ink {
	if r.plain || i >= len(r.cells) {
		return nil
	}
	if k := &r.cells[i]; k.stamp == r.stamp && k.s != "" {
		return k
	}
	return nil
}

// cellMean is the mean colour of the pixels of screen cell (col, row) that
// show face fi, lit (none do: of all its pixels). A line's cell is the
// colour of the face it is on, as a frame's cell on the flat board is its
// structure's: an outline is a crisp edge of cells, not a smudge of the
// colours either side.
func (r *room3D) cellMean(col, row int, fi int32) uint32 {
	var sr, sg, sb, n, ar, ag, ab, an float64
	r.cellSamples(col, row, func(i int) {
		cr, cg, cb := chans(r.pixel(i))
		ar, ag, ab, an = ar+cr, ag+cg, ab+cb, an+1
		if r.face[i] == fi {
			sr, sg, sb, n = sr+cr, sg+cg, sb+cb, n+1
		}
	})
	if n == 0 {
		sr, sg, sb, n = ar, ag, ab, an
	}
	return rgb32(sr/n+0.5, sg/n+0.5, sb/n+0.5)
}

// The box line letters by their arms (1 up, 2 down, 4 left, 8 right): a
// line with one arm is drawn whole.
var (
	boxLight  = [16]string{"", "│", "│", "│", "─", "┘", "┐", "┤", "─", "└", "┌", "├", "─", "┴", "┬", "┼"}
	boxHeavy  = [16]string{"", "┃", "┃", "┃", "━", "┛", "┓", "┫", "━", "┗", "┏", "┣", "━", "┻", "┳", "╋"}
	boxDouble = [16]string{"", "║", "║", "║", "═", "╝", "╗", "╣", "═", "╚", "╔", "╠", "═", "╩", "╦", "╬"}
	boxRound  = [16]string{5: "╯", 6: "╮", 9: "╰", 10: "╭"}
	diagGlyph = [4]string{"", "╱", "╲", "╳"}
	// A line across at five heights in the cell (kitty draws these thin
	// lines itself), or three (other terminals: ▔ and ▁ are in every
	// font); a line down at three places across.
	scanLines  = [5]string{"⎺", "⎻", "─", "⎼", "⎽"}
	blockLines = [3]string{"▔", "─", "▁"}
	sideLines  = [3]string{"▏", "│", "▕"}
)

// lineGlyph is the letter for the lines in a cell.
func lineGlyph(k *r3ink, fine bool) string {
	if k.diag != 0 {
		return diagGlyph[k.diag&3]
	}
	a := k.arms
	if a == 0 {
		a = k.lone
	}
	a &= 15
	switch st := k.style; {
	case a == 0:
		return ""
	case st&(styleDouble|styleHeavy|styleDashed) == 0 && k.posN > 0 && (a&3 == 0 || a&12 == 0):
		// A light line across or down only: at its height (or place across)
		// in the cell, so a slanting one rises smoothly, not in steps.
		at := k.pos / float32(k.posN)
		if a&3 == 0 {
			if fine {
				return scanLines[min(int(at*5), 4)]
			}
			return blockLines[min(int(at*3), 2)]
		}
		return sideLines[min(int(at*3), 2)]
	case st&styleDouble != 0:
		return boxDouble[a]
	case st&styleHeavy != 0:
		return boxHeavy[a]
	case st&styleDashed != 0 && a&3 == 0:
		return "┄"
	case st&styleDashed != 0 && a&12 == 0:
		return "┊"
	case st&styleRound != 0 && boxRound[a] != "":
		return boxRound[a]
	}
	return boxLight[a]
}

// boxStyle is the style of a box line letter (U+2500 to U+257F).
func boxStyle(r rune) uint8 { return boxStyleTable[r-0x2500] }

var boxStyleTable = func() (t [128]uint8) {
	for _, s := range []struct {
		s     string
		style uint8
	}{
		{"━┃┏┓┗┛┣┫┳┻╋┍┎┑┒┕┖┙┚┝┞┟┠┡┢┥┦┧┨┩┪┭┮┯┰┱┲┵┶┷┸┹┺┽┾┿╀╁╂╃╄╅╆╇╈╉╊╸╹╺╻╼╽╾╿", styleHeavy},
		{"═║╔╗╚╝╠╣╦╩╬╒╓╕╖╘╙╛╜╞╟╡╢╤╥╧╨╪╫", styleDouble},
		{"╌┄┈╎┆┊", styleDashed},
		{"╍┅┉╏┇┋", styleDashed | styleHeavy},
		{"╭╮╯╰", styleRound},
	} {
		for _, r := range s.s {
			t[r-0x2500] |= s.style
		}
	}
	return t
}()

// clearInkBy is clearInk with the least difference in lightness need (out
// of 255): a faint pattern needs little, a line more.
func clearInkBy(c, bg uint32, need float64) uint32 {
	lum := func(c uint32) float64 { r, g, b := chans(c); return 0.299*r + 0.587*g + 0.114*b }
	lc, lb := lum(c), lum(bg)
	if math.Abs(lc-lb) >= need {
		return c
	}
	up, down := lb+need <= 255, lb-need >= 0
	toward := func(to uint32, target float64) uint32 { // c mixed toward to until its lightness is target
		lt := lum(to)
		if math.Abs(lt-lc) < 1 {
			return c
		}
		return mix32(c, to, clampF((target-lc)/(lt-lc), 0, 1))
	}
	if lc >= lb && up || !down {
		return toward(0xffffff, lb+need)
	}
	return toward(0, lb-need)
}

// ---- putting marks in cells ------------------------------------------------------------

// mark puts m in screen cell (col, row): arms, diag and lone as for r3ink,
// pos where in the cell the line is (see r3ink, below 0: not known), depth
// how far in front of the camera it is, lit as pixel pix is.
func (r *room3D) mark(col, row int, arms, diag, lone uint8, pos float32, m *r3mark, depth float32, pix int) {
	w := r.rm.w
	if col < 0 || row < 0 || col >= w || row >= r.rm.h {
		return
	}
	k := &r.cells[row*w+col]
	switch {
	case k.stamp != r.stamp || m.level > k.level:
		*k = r3ink{stamp: r.stamp, level: m.level, arms: arms, diag: diag, lone: lone, style: m.style,
			need: m.need, depth: depth, col: r.lit(m.col, pix), face: m.face, glyph: m.glyph}
		k.seg, k.hasSeg = r.seg, r.segOn
		if pos >= 0 {
			k.pos, k.posN = pos, 1
		}
	case m.level == k.level:
		k.arms, k.diag, k.lone, k.style = k.arms|arms, k.diag|diag, k.lone|lone, k.style|m.style
		if pos >= 0 {
			k.pos, k.posN = k.pos+pos, k.posN+1
		}
		if depth < k.depth { // the nearer one's colour
			k.depth, k.col, k.need, k.face = depth, r.lit(m.col, pix), m.need, m.face
			if r.segOn {
				k.seg, k.hasSeg = r.seg, true
			}
			if m.glyph != "" {
				k.glyph = m.glyph
			}
		}
	}
}

// shows is whether a pixel at or next to (i, j) shows one of faces.
func (r *room3D) shows(i, j int, faces *[3]int32) bool {
	pw, ph := r.pw, r.ph
	if i >= 0 && j >= 0 && i < pw && j < ph { // most often the pixel itself does
		if f := r.face[j*pw+i]; f == faces[0] || f == faces[1] || f == faces[2] {
			return true
		}
	}
	for y := max(j-1, 0); y <= min(j+1, ph-1); y++ {
		row := r.face[y*pw:]
		for x := max(i-1, 0); x <= min(i+1, pw-1); x++ {
			if f := row[x]; f == faces[0] || f == faces[1] || f == faces[2] {
				return true
			}
		}
	}
	return false
}

// toCam is point p as the camera sees it: across, up and ahead.
func (v *r3view) toCam(p r3vec) r3vec {
	q := p.sub(v.cam)
	return r3vec{q.dot(v.right), q.dot(v.up), q.dot(v.fwd)}
}

// screenAt is where point p (as the camera sees it, ahead at least near)
// is on the screen: columns across, units down (a row is two).
func (v *r3view) screenAt(c r3vec) (x, y float64) {
	return v.px + v.f*c.x/c.z, v.py - v.f*c.y/c.z
}

// apart is whether point p, seen at screen (x, y), and p+off are at least
// sep cells apart on the screen (or p+off is behind the camera).
func (r *room3D) apart(p, off r3vec, x, y, sep float64) bool {
	c := r.view.toCam(p.add(off))
	if c.z < 0.25 {
		return true
	}
	x2, y2 := r.view.screenAt(c)
	return math.Max(math.Abs(x2-x), math.Abs(y2-y)/2) >= sep
}

// plainAt is whether point p of the back wall is on empty board (or off
// the board, plain wall).
func (r *room3D) plainAt(p r3vec, plain uint32) bool {
	rm := &r.rm
	x, y := int(math.Floor(p.x+rm.hw))-rm.tx0, int(math.Floor((rm.hh-p.y)/2))-rm.ty0
	if x < 0 || y < 0 || x >= rm.tw || y >= rm.th {
		return true
	}
	t := &r.tex[y*rm.tw+x]
	return t.blank && t.bg == plain
}

// clipSpan is the part t0 ≤ t ≤ t1 of the line from (x0, y0) to (x1, y1)
// inside the box xmin ≤ x ≤ xmax, ymin ≤ y ≤ ymax (Liang and Barsky's way).
func clipSpan(x0, y0, x1, y1, xmin, ymin, xmax, ymax float64) (t0, t1 float64, ok bool) {
	t0, t1 = 0, 1
	dx, dy := x1-x0, y1-y0
	for _, e := range [4][2]float64{{-dx, x0 - xmin}, {dx, xmax - x0}, {-dy, y0 - ymin}, {dy, ymax - y0}} {
		p, q := e[0], e[1]
		if p == 0 {
			if q < 0 {
				return 0, 0, false
			}
			continue
		}
		t := q / p
		if p < 0 {
			if t > t1 {
				return 0, 0, false
			}
			t0 = math.Max(t0, t)
		} else {
			if t < t0 {
				return 0, 0, false
			}
			t1 = math.Min(t1, t)
		}
	}
	return t0, t1, t0 <= t1
}

// Which way a line runs on the screen: across, up and down, or slanting.
const (
	runAcross = iota
	runDown
	runSlant
)

// line draws segment s cell by cell where it shows: points along it half
// a cell apart on the screen, and between two seen points, the
// cells from one to the other. A line across (or down) is drawn at its
// height (or place across) in each cell, so a slight slope rises smoothly
// (⎽⎼─⎻⎺); a slanting line is ╱ or ╲ all along. Where its end meets
// another line, their arms make a corner (┌ ┐ └ ┘).
func (r *room3D) line(s *r3seg, plain uint32) {
	v := &r.view
	w, h := r.rm.w, r.rm.h
	wa, wb := s.a, s.b
	ca, cb := v.toCam(wa), v.toCam(wb)
	if s.m.face == 0 {
		s.m.face = s.faces[0] // on its first face, unless said
	}
	const near = 0.25
	if ca.z < near && cb.z < near {
		return
	}
	if ca.z < near {
		t := (near - ca.z) / (cb.z - ca.z)
		ca, wa = ca.add(cb.sub(ca).mul(t)), wa.add(wb.sub(wa).mul(t))
	} else if cb.z < near {
		t := (near - cb.z) / (ca.z - cb.z)
		cb, wb = cb.add(ca.sub(cb).mul(t)), wb.add(wa.sub(wb).mul(t))
	}
	ax, ay := v.screenAt(ca)
	bx, by := v.screenAt(cb)
	dcol, drow := bx-ax, (by-ay)/2
	long := math.Max(math.Abs(dcol), math.Abs(drow))
	if long < s.minLen {
		return
	}
	s0, s1, ok := clipSpan(ax, ay/2, bx, by/2, -1, -1, float64(w)+1, float64(h)+1)
	if !ok {
		return
	}
	if s.sep > 0 && !r.apart(wa, s.off, ax, ay, s.sep) && !r.apart(wb, s.off, bx, by, s.sep) {
		return // the lines next to it too close at both ends, so all along (it changes steadily)
	}
	way := runSlant
	switch m := math.Abs(drow) / math.Max(math.Abs(dcol), 1e-9); {
	case m < 0.4:
		way = runAcross
	case m > 2.2:
		way = runDown
	}
	var diag, lone uint8
	switch {
	case way == runSlant && (dcol > 0) == (drow < 0):
		diag = 1 // ╱: right and up
	case way == runSlant:
		diag = 2
	case way == runAcross:
		lone = 4 | 8
	default:
		lone = 1 | 2
	}
	r.seg, r.segOn = [4]float32{float32(ax), float32(ay / 2), float32(bx - ax), float32((by - ay) / 2)}, true
	defer func() { r.segOn = false }()
	n := min(int(math.Ceil(long*(s1-s0)*2))+1, 4000)
	iza, izb := 1/ca.z, 1/cb.z
	pc, pr, have := 0, 0, false
	wide := true // the lines next to it far enough apart: looked at every fourth point (it changes steadily)
	for i := 0; i <= n; i++ {
		sp := s0 + (s1-s0)*float64(i)/float64(n)
		x, y := ax+(bx-ax)*sp, ay+(by-ay)*sp
		iz := iza + (izb-iza)*sp
		p := wa.add(wb.sub(wa).mul(sp * izb / iz)) // the point of the room seen there
		pi, pj := int(math.Floor(x*r.kx)), int(math.Floor(y*r.ky))
		if pi < -1 || pj < -1 || pi > r.pw || pj > r.ph || !r.shows(pi, pj, &s.faces) ||
			s.plain && r.face[max(0, min(pj, r.ph-1))*r.pw+max(0, min(pi, r.pw-1))] == faceBoard && !r.plainAt(p, plain) ||
			s.sep > 0 && !func() bool {
				if i%4 == 0 || !have {
					wide = r.apart(p, s.off, x, y, s.sep)
				}
				return wide
			}() {
			have = false
			continue
		}
		col, row := int(math.Floor(x)), int(math.Floor(y/2))
		pix := max(0, min(pj, r.ph-1))*r.pw + max(0, min(pi, r.pw-1))
		depth := float32(1 / iz)
		if have && (col != pc || row != pr) {
			r.steps(pc, pr, col, row, way, diag, lone, &s.m, depth, pix)
		}
		pos := float32(-1) // a level line (or upright) stays in the middle, joining its corners
		switch {
		case way == runAcross && math.Abs(drow) >= 1:
			pos = float32(y/2 - float64(row))
		case way == runDown && math.Abs(dcol) >= 1:
			pos = float32(x - float64(col))
		}
		r.mark(col, row, 0, diag, lone, pos, &s.m, depth, pix)
		pc, pr, have = col, row, true
	}
}

// steps marks the cells of a line from (c0, r0) to (c1, r1): each joined
// to the next by its arms. A line across joins cells side by side (one a
// row higher or lower too: it rises there), a line down cells one over
// the other; a slanting line goes diagonally, ╱ or ╲ in every cell.
func (r *room3D) steps(c0, r0, c1, r1, way int, diag, lone uint8, m *r3mark, depth float32, pix int) {
	sign := func(v int) int {
		switch {
		case v > 0:
			return 1
		case v < 0:
			return -1
		}
		return 0
	}
	for guard := 0; (c0 != c1 || r0 != r1) && guard < 4*(r.rm.w+r.rm.h); guard++ {
		dc, dr := sign(c1-c0), sign(r1-r0)
		switch {
		case way == runSlant:
			c0, r0 = c0+dc, r0+dr
			r.mark(c0, r0, 0, diag, 0, -1, m, depth, pix)
		case way == runAcross && dc != 0:
			out, in := uint8(8), uint8(4) // right, and in from the left
			if dc < 0 {
				out, in = 4, 8
			}
			r.mark(c0, r0, out, 0, 0, -1, m, depth, pix)
			c0, r0 = c0+dc, r0+dr
			r.mark(c0, r0, in, 0, 0, -1, m, depth, pix)
		case way == runDown && dr != 0:
			out, in := uint8(2), uint8(1) // down, and in from above
			if dr < 0 {
				out, in = 1, 2
			}
			r.mark(c0, r0, out, 0, 0, -1, m, depth, pix)
			c0, r0 = c0+dc, r0+dr
			r.mark(c0, r0, in, 0, 0, -1, m, depth, pix)
		default: // a line across going on in the next row (or one down in the next column)
			c0, r0 = c0+dc, r0+dr
			r.mark(c0, r0, 0, 0, lone, -1, m, depth, pix)
		}
	}
}

// dot puts m's letter where point p shows on face fi, when the points p+off
// for each of offs are at least sep cells from it on the screen.
func (r *room3D) dot(p r3vec, fi int32, m *r3mark, sep float64, offs ...r3vec) {
	v := &r.view
	c := v.toCam(p)
	if c.z < 0.25 {
		return
	}
	x, y := v.screenAt(c)
	col, row := int(math.Floor(x)), int(math.Floor(y/2))
	if col < 0 || row < 0 || col >= r.rm.w || row >= r.rm.h {
		return
	}
	pi, pj := int(math.Floor(x*r.kx)), int(math.Floor(y*r.ky))
	faces := [3]int32{fi, noFace, noFace}
	if !r.shows(pi, pj, &faces) {
		return
	}
	m.face = fi
	for _, off := range offs {
		if !r.apart(p, off, x, y, sep) {
			return
		}
	}
	r.mark(col, row, 0, 0, 0, -1, m, float32(c.z), max(0, min(pj, r.ph-1))*r.pw+max(0, min(pi, r.pw-1)))
}

// ---- what the layer shows ----------------------------------------------------------------

// sideShades shades the cells of a block's sides (the faces round its
// front, not those facing the camera as a door does): ░ where the lamp
// lights them, ▒ where it does not, in a darker shade of the side's own
// colour. Only cells wholly on one side, so its edges stay lines.
func (r *room3D) sideShades(c *r3shade) {
	w, h := r.rm.w, r.rm.h
	l := c.light
	lit := l.ambient + 0.5*l.fill + 0.2*l.lamp
	light := r3mark{level: inkShade, glyph: "░"}
	dark := r3mark{level: inkShade, glyph: "▒"}
	for row := range h {
		for col := range w {
			four := r.cellFour(col, row)
			f := r.face[four[0]]
			if f < 0 || r.face[four[1]] != f || r.face[four[2]] != f || r.face[four[3]] != f {
				continue // not all one face
			}
			fc := &r.faces[f]
			if fc.own < 0 || fc.mat != matSolid || !fc.edge || math.Abs(fc.n.z) > 0.5 || r.faceCells(f) < 12 {
				continue // only a block's sides, big enough for a shade to read as one
			}
			// Wholly on the side, and much the same colour all over (not a
			// bright patch of the lamp's light): a shaded cell is one colour
			// under its letter.
			whole, lo, hi := true, [3]float64{255, 255, 255}, [3]float64{}
			r.cellSamples(col, row, func(i int) {
				whole = whole && r.face[i] == f
				cr, cg, cb := chans(r.pixel(i))
				lo = [3]float64{min(lo[0], cr), min(lo[1], cg), min(lo[2], cb)}
				hi = [3]float64{max(hi[0], cr), max(hi[1], cg), max(hi[2], cb)}
			})
			if !whole || max(hi[0]-lo[0], hi[1]-lo[1], hi[2]-lo[2]) > 16 {
				continue
			}
			mul := 0.0
			for _, i := range four {
				mul += float64(r.mul[i])
			}
			m := &dark
			if mul/4 > lit {
				m = &light
			}
			m.col, m.face = fc.col, f
			r.mark(col, row, 0, 0, 0, -1, m, r.zb[four[0]], four[0])
		}
	}
}

// wallHeights is how high over the floor a papered wall's skirting board,
// dado rail and picture rail are (pic +Inf: a low room has none).
func wallHeights(rm *r3room) (skirt, dado, pic float64) {
	skirt, dado = 3, max(9, 0.14*rm.big)
	pic = rm.cy - rm.fy - max(3, 0.05*rm.big)
	if pic < dado+8 {
		pic = math.Inf(1)
	}
	return skirt, dado, pic
}

// railH is how tall a rail is (see wallAt).
const railH = 1.2

// roomInk puts the room's own lines: its corners, the floor's planks, the
// walls' rails and stripes, and the ceiling's dots.
func (r *room3D) roomInk(c *r3shade) {
	rm := c.rm
	text, plainBG := c.textCol, c.bgPlain
	seg := func(a, b r3vec, m r3mark, faces ...int32) r3seg {
		s := r3seg{a: a, b: b, m: m, faces: [3]int32{noFace, noFace, noFace}}
		copy(s.faces[:], faces)
		return s
	}
	// The corners: where the walls, the floor and the ceiling meet.
	corner := r3mark{level: inkLine, col: mix32(c.wallCol, text, 0.35), need: 40}
	X, Y, Z := [2]float64{rm.wx0, rm.wx1}, [2]float64{rm.fy, rm.cy}, [2]float64{0, rm.fz}
	sides, flats, ends := [2]int32{faceLeft, faceRight}, [2]int32{faceFloor, faceCeiling}, [2]int32{faceBack, faceFront}
	board := [2]int32{faceBoard, noFace} // the back wall is the board too
	for i := range 2 {
		for j := range 2 {
			s := seg(r3vec{X[i], Y[j], 0}, r3vec{X[i], Y[j], rm.fz}, corner, sides[i], flats[j])
			r.line(&s, plainBG)
			s = seg(r3vec{X[i], rm.fy, Z[j]}, r3vec{X[i], rm.cy, Z[j]}, corner, sides[i], ends[j], board[j])
			r.line(&s, plainBG)
			s = seg(r3vec{rm.wx0, Y[i], Z[j]}, r3vec{rm.wx1, Y[i], Z[j]}, corner, flats[i], ends[j], board[j])
			r.line(&s, plainBG)
		}
	}

	// The floor's planks: the joins between them running out from the
	// wall, and the ends of each length of plank (see floorAt).
	const plankW, plankL = 7.0, 34.0
	mod := func(a, b float64) float64 { return a - b*math.Floor(a/b) }
	join := r3mark{level: inkTexture, col: mix32(r.faces[faceFloor].col, 0, 0.4), need: 20}
	for p := math.Ceil((rm.wx0 + rm.hw) / plankW); rm.floor == floorPlanks && p*plankW-rm.hw <= rm.wx1; p++ { // other floors: no lines (design_3d_finish.go)
		x := p*plankW - rm.hw
		s := seg(r3vec{x, rm.fy, 0}, r3vec{x, rm.fy, rm.fz}, join, faceFloor)
		s.off, s.sep = r3vec{plankW, 0, 0}, 2.5
		r.line(&s, plainBG)
		for z := plankL - mod(p*13, plankL); z < rm.fz; z += plankL {
			e := seg(r3vec{x, rm.fy, z}, r3vec{math.Min(x+plankW, rm.wx1), rm.fy, z}, join, faceFloor)
			e.off, e.sep = r3vec{plankW, 0, 0}, 5
			r.line(&e, plainBG)
		}
	}

	// The walls: the skirting board, the dado and picture rails,
	// and the paper's stripes between them (dotted).
	skirt, dado, pic := wallHeights(rm)
	top := math.Min(rm.fy+pic, rm.cy)
	type wall struct {
		face   int32
		at     func(u, y float64) r3vec
		u0, u1 float64
		plain  bool
	}
	walls := []wall{
		{faceLeft, func(u, y float64) r3vec { return r3vec{rm.wx0, y, u} }, 0, rm.fz, false},
		{faceRight, func(u, y float64) r3vec { return r3vec{rm.wx1, y, u} }, 0, rm.fz, false},
		{faceBack, func(u, y float64) r3vec { return r3vec{u, y, 0} }, rm.wx0, rm.wx1, true},
		{faceFront, func(u, y float64) r3vec { return r3vec{u, y, rm.fz} }, rm.wx0, rm.wx1, false},
	}
	wallCol := r.faces[faceLeft].col
	trim := r3mark{level: inkTexture, col: mix32(wallCol, text, 0.25), need: 26}
	rail := trim
	stripe := r3mark{level: inkTexture, col: mix32(wallCol, 0, 0.3), need: 12, style: styleDashed}
	for _, wl := range walls {
		faces := []int32{wl.face}
		if wl.plain {
			faces = append(faces, faceBoard)
		}
		for _, l := range []struct {
			y float64
			m r3mark
		}{{rm.fy + skirt/2, trim}, {rm.fy + dado + railH/2, rail}, {rm.fy + pic + railH/2, rail}} {
			if l.y >= rm.cy || (!wallHasRails(rm.walls) && (l.y > rm.fy+skirt || rm.walls != wallPanels)) {
				continue // a finish without rails (and but for wood panels, without a skirting board)
			}
			s := seg(wl.at(wl.u0, l.y), wl.at(wl.u1, l.y), l.m, faces...)
			s.plain = wl.plain
			r.line(&s, plainBG)
		}
		const period = 6.0
		for u := math.Ceil(wl.u0/period) * period; rm.walls == wallPaper && u <= wl.u1; u += period { // stripes on paper only
			s := seg(wl.at(u, rm.fy+dado+railH), wl.at(u, top), stripe, faces...)
			s.plain = wl.plain
			s.off, s.sep = wl.at(u+period, 0).sub(wl.at(u, 0)), 2.5
			r.line(&s, plainBG)
		}
	}

	// The ceiling: a dot every 8 units, where they are well apart.
	dotM := r3mark{level: inkDot, col: mix32(r.faces[faceCeiling].col, text, 0.3), need: 22, glyph: "·"}
	const grid = 8.0
	for x := math.Ceil(rm.wx0/grid) * grid; x <= rm.wx1; x += grid {
		for z := grid / 2; z < rm.fz; z += grid {
			r.dot(r3vec{x, rm.cy, z}, faceCeiling, &dotM, 2.2, r3vec{grid, 0, 0}, r3vec{0, 0, grid})
		}
	}
}

// boardInk puts the flat board's own box lines, dots and signs on the
// back wall and the blocks' fronts, where they are seen big enough to tell
// apart: a run of box line letters is one line in the room (a corner
// joins the lines it ends).
func (r *room3D) boardInk(c *r3shade) {
	rm := &r.rm
	hw, hh := rm.hw, rm.hh
	tx0, ty0, tw, th := rm.tx0, rm.ty0, rm.tw, rm.th
	sh := 0 // the face's block on a layer: its drawing is kept below the board (layerCopies)
	tex := func(x, y int) *r3texel { return &r.tex[(y+sh-ty0)*tw+x-tx0] }
	for fi := range r.faces {
		fc := &r.faces[fi]
		if !fc.seen || (fc.mat != matBoard && fc.mat != matFront) {
			continue
		}
		sh = fc.tdy / 2
		z := r.pts[fc.p0].z
		x0, x1 := max(fc.cx0, tx0), min(fc.cx1, tx0+tw)
		y0, y1 := max(fc.py0>>1, ty0), min(fc.py1>>1, ty0+th)
		if x0 >= x1 || y0 >= y1 {
			continue
		}
		faces := [3]int32{int32(fi), noFace, noFace}
		at := func(u, v float64) r3vec { return r3vec{u - hw, hh - 2*v, z} }
		if fc.mat == matFront && r.small(at(float64(x0+x1)/2, float64(y0+y1)/2)) {
			continue // seen too small for its lines to be told apart: the picture shows them
		}
		wall := fc.mat == matBoard
		covered := func(x, y int) bool { return wall && r.cover[(y-ty0)*tw+x-tx0] == r.stamp }
		line := func(u0, v0, u1, v1 float64, off r3vec, t *r3texel, style uint8) {
			if wall && r.small(at((u0+u1)/2, (v0+v1)/2)) {
				return // on the wall, far off or at a slant
			}
			s := r3seg{a: at(u0, v0), b: at(u1, v1), off: off, sep: 0.7, faces: faces,
				m: r3mark{level: inkBoard, style: style, need: 40, col: t.fg}}
			r.line(&s, c.bgPlain)
		}
		for y := y0; y < y1; y++ {
			v := float64(y) + 0.5
			for x := x0; x < x1; x++ {
				t := tex(x, y)
				if covered(x, y) {
					continue // a block stands on it
				}
				// A sign of the flat board as it is: a dot of its grid on the
				// wall, a shade, a braille picture.
				if g := t.mark; g != "" || fc.mat == matBoard && (t.glyph == "·" || t.glyph == "+") {
					if g == "" {
						g = t.glyph
					}
					m := r3mark{level: inkDot, col: t.fg, need: 30, glyph: g}
					r.dot(at(float64(x)+0.5, v), int32(fi), &m, 0.75, r3vec{1, 0, 0}, r3vec{0, 2, 0})
				}
				if t.arms&12 == 0 {
					continue
				}
				// A run of box lines across: from the first to the last
				// joined, to a cell's edge where an arm joins nothing.
				xs, style := x, t.style
				for x+1 < x1 && tex(x, y).arms&8 != 0 && tex(x+1, y).arms&4 != 0 && !covered(x+1, y) {
					x++
					style |= tex(x, y).style
				}
				u0, u1 := float64(xs)+0.5, float64(x)+0.5
				if tex(xs, y).arms&4 != 0 {
					u0 = float64(xs) + 0.01
				}
				if tex(x, y).arms&8 != 0 {
					u1 = float64(x) + 0.99
				}
				line(u0, v, u1, v, r3vec{0, 2, 0}, tex((xs+x)/2, y), style)
			}
		}
		for x := x0; x < x1; x++ {
			u := float64(x) + 0.5
			for y := y0; y < y1; y++ {
				t := tex(x, y)
				if t.arms&3 == 0 || covered(x, y) {
					continue
				}
				ys, style := y, t.style
				for y+1 < y1 && tex(x, y).arms&2 != 0 && tex(x, y+1).arms&1 != 0 && !covered(x, y+1) {
					y++
					style |= tex(x, y).style
				}
				v0, v1 := float64(ys)+0.5, float64(y)+0.5
				if tex(x, ys).arms&1 != 0 {
					v0 = float64(ys) + 0.01
				}
				if tex(x, y).arms&2 != 0 {
					v1 = float64(y) + 0.99
				}
				line(u, v0, u, v1, r3vec{1, 0, 0}, tex(x, (ys+y)/2), style)
			}
		}
	}
}

// small is whether the flat board's cells round point p (of the wall or a
// front) are seen smaller than 0.6 of a cell of the screen across or down:
// turned far or drawn small, a drawing's box lines would be squeezed
// together, so the picture shows them instead.
func (r *room3D) small(p r3vec) bool {
	v := &r.view
	c := v.toCam(p)
	if c.z < 0.25 {
		return false
	}
	x, y := v.screenAt(c)
	return !r.apart(p, r3vec{1, 0, 0}, x, y, 0.6) || !r.apart(p, r3vec{0, 2, 0}, x, y, 0.6)
}

// r3edgeKey is an edge of a face, its two ends rounded to 1/64 of a unit,
// the lesser first: the faces either side of an edge have the same key.
type r3edgeKey struct{ a, b [3]int32 }

// r3edge is an edge and the faces either side of it (f2 −1: only one).
type r3edge struct {
	a, b   r3vec
	f1, f2 int32
}

func edgeKeyOf(a, b r3vec) r3edgeKey {
	q := func(p r3vec) [3]int32 {
		return [3]int32{int32(math.Round(p.x * 64)), int32(math.Round(p.y * 64)), int32(math.Round(p.z * 64))}
	}
	ka, kb := q(a), q(b)
	if kb[0] < ka[0] || kb[0] == ka[0] && (kb[1] < ka[1] || kb[1] == ka[1] && kb[2] < ka[2]) {
		ka, kb = kb, ka
	}
	return r3edgeKey{ka, kb}
}

// edgeInk outlines the blocks: every edge of a block's faces where it
// folds (the faces either side meet at a corner, not a soft bend such as a
// round box's wall), where it is the block's outline against what is
// behind it, or where a face ends with none beyond it. Each line is set a
// little into the face that faces the camera most, so on a front seen
// straight on its edges fall in the front's own cells, as the flat
// drawing's frame does.
func (r *room3D) edgeInk(c *r3shade) {
	if r.edgeAt == nil {
		r.edgeAt = map[r3edgeKey]int32{}
	}
	clear(r.edgeAt)
	r.edges = r.edges[:0]
	w, h := float64(r.pw), float64(r.ph)
	onScreen := func(fi int) bool { // facing the camera, and its outline not all off one side of the screen
		fd := &r.fdraw[fi]
		if !r.faces[fi].seen || fd.bo < 0 || fd.top > h {
			return false
		}
		x0, x1 := math.Inf(1), math.Inf(-1)
		for _, p := range r.polys[fd.p0 : fd.p0+fd.pn] {
			x0, x1 = math.Min(x0, p[0]), math.Max(x1, p[0])
		}
		return x1 >= 0 && x0 <= w
	}
	for f0 := 0; f0 < len(r.faces); {
		// A block's faces come one after another: a block none of whose
		// faces shows on the screen has no edge to draw.
		own := r.faces[f0].own
		f1, any := f0, false
		for ; f1 < len(r.faces) && r.faces[f1].own == own; f1++ {
			any = any || own >= 0 && onScreen(f1)
		}
		if !any {
			f0 = f1
			continue // the room has its own lines (see roomInk)
		}
		for fi := f0; fi < f1; fi++ {
			r.faceEdges(fi)
		}
		f0 = f1
	}
	r.drawEdges(c)
}

// faceEdges adds face fi's edges to r.edges, or itself to an edge there
// already (the face on its other side).
func (r *room3D) faceEdges(fi int) {
	fc := &r.faces[fi]
	pts := r.pts[fc.p0 : fc.p0+fc.pn]
	for i := range pts {
		a, b := pts[i], pts[(i+1)%len(pts)]
		k := edgeKeyOf(a, b)
		if j, ok := r.edgeAt[k]; ok && r.edges[j].f2 < 0 {
			r.edges[j].f2 = int32(fi)
			continue
		}
		r.edgeAt[k] = int32(len(r.edges))
		r.edges = append(r.edges, r3edge{a: a, b: b, f1: int32(fi), f2: -1})
	}
}

// drawEdges draws the edges of r.edges that are a block's outline or a
// fold in it.
func (r *room3D) drawEdges(c *r3shade) {
	cam := r.view.cam
	facing := func(fc *r3face, p r3vec) float64 { // how much it faces the camera (less than 0: away)
		if fc.both {
			return math.Abs(fc.n.dot(cam.sub(p).norm()))
		}
		return fc.n.dot(cam.sub(p).norm())
	}
	for _, e := range r.edges {
		mid := e.a.add(e.b).mul(0.5)
		f1 := &r.faces[e.f1]
		into, see := e.f1, facing(f1, mid)
		if e.f2 >= 0 {
			f2 := &r.faces[e.f2]
			s2 := facing(f2, mid)
			if see <= 0 && s2 <= 0 {
				continue // both face away: hidden
			}
			if (see > 0) == (s2 > 0) && f1.n.dot(f2.n) > 0.8 {
				continue // a soft bend
			}
			if s2 > see {
				into, see = e.f2, s2
			}
			if r.faceCells(e.f1) < tinyFace && r.faceCells(e.f2) < tinyFace {
				continue // between two tiny faces (a knob, a thing deep in a box): the picture shows it
			}
		} else if see <= 0 || r.faceCells(e.f1) < tinyFace {
			continue
		}
		if r.framed(e) {
			continue // the front's own frame (its flat drawing's box lines) is its outline
		}
		// Set into that face, toward its middle.
		fc := &r.faces[into]
		var mc r3vec
		pts := r.pts[fc.p0 : fc.p0+fc.pn]
		for _, p := range pts {
			mc = mc.add(p)
		}
		mc = mc.mul(1 / float64(len(pts)))
		in := func(p r3vec) r3vec {
			d := mc.sub(p)
			l := d.len()
			if l < 1e-9 {
				return p
			}
			return p.add(d.mul(math.Min(0.3, 0.3*l) / l))
		}
		col := c.textCol
		switch {
		case f1.mat == matSolid && f1.col != 0:
			col = f1.col
		case e.f2 >= 0 && r.faces[e.f2].mat == matSolid && r.faces[e.f2].col != 0:
			col = r.faces[e.f2].col
		default:
			col = r.blockCol(f1.own, col)
		}
		s := r3seg{a: in(e.a), b: in(e.b), minLen: 1.5, faces: [3]int32{e.f1, e.f2, noFace},
			m: r3mark{level: inkLine, col: col, need: 55, face: into}}
		if e.f2 < 0 {
			s.faces[1] = noFace
		}
		r.line(&s, c.bgPlain)
	}
}

// tinyFace is how many cells of the screen a face takes at least for its
// edges to be lines of letters (see faceCells).
const tinyFace = 3

// faceCells is how many cells of the screen face fi's outline takes (0:
// not seen).
func (r *room3D) faceCells(fi int32) float64 {
	if !r.faces[fi].seen {
		return 0
	}
	fd := &r.fdraw[fi]
	ps := r.polys[fd.p0 : fd.p0+fd.pn]
	a := 0.0
	for i := range ps {
		p, q := ps[i], ps[(i+1)%len(ps)]
		a += p[0]*q[1] - q[0]*p[1]
	}
	return math.Abs(a) / 2 / (r.kx * 2 * r.ky) // a cell is a unit across and two down
}

// framed is whether edge e is an edge of a block's front along which the
// front's flat drawing has a box line (its frame), at a quarter, half or
// three quarters of the way along it: that line is drawn instead.
func (r *room3D) framed(e r3edge) bool {
	rm := &r.rm
	for _, fi := range [2]int32{e.f1, e.f2} {
		if fi < 0 || r.faces[fi].mat != matFront {
			continue
		}
		fc := &r.faces[fi]
		var mc r3vec
		pts := r.pts[fc.p0 : fc.p0+fc.pn]
		for _, p := range pts {
			mc = mc.add(p)
		}
		mc = mc.mul(1 / float64(len(pts)))
		for _, t := range [3]float64{0.25, 0.5, 0.75} {
			p := e.a.add(e.b.sub(e.a).mul(t))
			if d := mc.sub(p); d.len() > 1e-9 {
				p = p.add(d.mul(math.Min(0.3, 0.3*d.len()) / d.len())) // into the front
			}
			x, y := int(math.Floor(p.x+rm.hw)), int(math.Floor((rm.hh-p.y)/2))
			if x >= max(fc.cx0, rm.tx0) && x < min(fc.cx1, rm.tx0+rm.tw) && y >= max(fc.py0>>1, rm.ty0) && y < min(fc.py1>>1, rm.ty0+rm.th) &&
				r.tex[(y+fc.tdy/2-rm.ty0)*rm.tw+x-rm.tx0].arms != 0 {
				return true
			}
		}
	}
	return false
}

// blockCol is the colour of the structure whose ID is own (or, not one of
// the room's blocks, or), for its outline.
func (r *room3D) blockCol(own int, or uint32) uint32 {
	for i := range r.blocks {
		if o := r.blocks[i].o; o.ID == own {
			if o.Color != "" {
				return r.hex(o.Color)
			}
			return or
		}
	}
	return or
}
