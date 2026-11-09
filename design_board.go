package main

// ---- the board: things pinned on it, small, turned and stretched -----------------------
//
// A board (Alt+N, then B) is a pinboard. Drop a page, note or shape on it
// and it is pinned where you let go: drawn small (a miniature: its outline,
// icon and one label) with a pin through its top. Each pin can be turned,
// scaled, stretched, skewed, flipped and moved, and the miniature is drawn
// so, cell by cell:
//
//	click a pin          opens it full size where it is pinned (Esc, its
//	                     row in Tab, or a click on the empty board closes it)
//	drag a pin           moves it on the board
//	Tab / Shift+Tab      chooses the board's next (last) pin
//	R / Shift+R          turns it 15° (back)
//	S / Shift+S          bigger / smaller
//	W / Shift+W          wider / narrower;  H / Shift+H  taller / shorter
//	K / Shift+K          skews it sideways;  J / Shift+J  up and down
//	F · V                flips it across · up and down
//	Alt + arrows         moves it (Alt + Shift: further)
//	T                    its transform as words, to change by typing:
//	                     rotate 30 scale 1.5 skew 0.2 flip move 2 -1 reset
//	Enter                opens it;  X X  takes it off the board (deletes it)
//	Esc                  lets go of it
//
// A pin's place is from the board's inside corner, in board cells; its
// transform is turned about its middle, with a cell twice as tall as wide,
// so a turned miniature keeps its shape.

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// dTf is a pin's transform. Zero scales mean 1; a negative one flips.
type dTf struct {
	Rot float64 `json:"rot,omitempty"` // degrees, clockwise
	SX  float64 `json:"sx,omitempty"`
	SY  float64 `json:"sy,omitempty"`
	KX  float64 `json:"kx,omitempty"` // skew: x moves by KX × y
	KY  float64 `json:"ky,omitempty"` // and y by KY × x
}

func (t *dTf) get() (rot, sx, sy, kx, ky float64) {
	if t == nil {
		return 0, 1, 1, 0, 0
	}
	sx, sy = t.SX, t.SY
	if sx == 0 {
		sx = 1
	}
	if sy == 0 {
		sy = 1
	}
	return t.Rot, sx, sy, t.KX, t.KY
}

func sameTf(a, b *dTf) bool {
	ar, asx, asy, akx, aky := a.get()
	br, bsx, bsy, bkx, bky := b.get()
	return ar == br && asx == bsx && asy == bsy && akx == bkx && aky == bky
}

// setTf stores a transform (nil when it is none).
func (o *dOct) setTf(rot, sx, sy, kx, ky float64) {
	rot = math.Mod(rot, 360)
	if rot < 0 {
		rot += 360
	}
	clampS := func(s float64) float64 {
		if math.Abs(s) < 0.25 {
			return math.Copysign(0.25, s)
		}
		return math.Max(-6, math.Min(6, s))
	}
	sx, sy = clampS(sx), clampS(sy)
	kx, ky = math.Max(-3, math.Min(3, kx)), math.Max(-3, math.Min(3, ky))
	round := func(v float64) float64 { return math.Round(v*1000) / 1000 }
	t := dTf{Rot: round(rot), SX: round(sx), SY: round(sy), KX: round(kx), KY: round(ky)}
	if t.SX == 1 {
		t.SX = 0
	}
	if t.SY == 1 {
		t.SY = 0
	}
	if t == (dTf{}) {
		o.Tf = nil
		return
	}
	o.Tf = &t
}

// tfText is a transform as words, as T shows and takes them.
func tfText(t *dTf) string {
	rot, sx, sy, kx, ky := t.get()
	num := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	var parts []string
	if rot != 0 {
		parts = append(parts, "rotate "+num(rot))
	}
	if sx < 0 && sy < 0 {
		parts = append(parts, "flip")
	} else if sx < 0 {
		parts = append(parts, "flipx")
	} else if sy < 0 {
		parts = append(parts, "flipy")
	}
	ax, ay := math.Abs(sx), math.Abs(sy)
	switch {
	case ax == ay && ax != 1:
		parts = append(parts, "scale "+num(ax))
	case ax != ay:
		parts = append(parts, "scale "+num(ax)+" "+num(ay))
	}
	switch {
	case kx != 0 && ky != 0:
		parts = append(parts, "skew "+num(kx)+" "+num(ky))
	case kx != 0:
		parts = append(parts, "skew "+num(kx))
	case ky != 0:
		parts = append(parts, "skewy "+num(ky))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, " ")
}

// parseTf reads a transform typed as words onto o (CSS's way too:
// "rotate(30deg) scale(1.5)"). Words: rotate, scale (one or two numbers),
// scalex, scaley, skew (one or two), skewx, skewy, flip, flipx, flipy,
// move (two numbers: cells across and down), reset, none. It says what it
// did not understand.
func parseTf(o *dOct, s string) (moveX, moveY int, bad string) {
	rot, sx, sy, kx, ky := o.Tf.get()
	fx, fy := math.Copysign(1, sx), math.Copysign(1, sy)
	ax, ay := math.Abs(sx), math.Abs(sy)
	r := strings.NewReplacer("(", " ", ")", " ", ",", " ", "deg", " ", "°", " ", ";", " ", "·", " ")
	toks := strings.Fields(strings.ToLower(r.Replace(s)))
	nums := func(i int) []float64 {
		var out []float64
		for j := i + 1; j < len(toks); j++ {
			v, err := strconv.ParseFloat(toks[j], 64)
			if err != nil {
				break
			}
			out = append(out, v)
		}
		return out
	}
	for i := 0; i < len(toks); i++ {
		n := nums(i)
		switch toks[i] {
		case "rotate", "rot", "turn":
			if len(n) > 0 {
				rot = n[0]
			}
		case "scale":
			if len(n) == 1 {
				ax, ay = n[0], n[0]
			} else if len(n) >= 2 {
				ax, ay = n[0], n[1]
			}
		case "scalex":
			if len(n) > 0 {
				ax = n[0]
			}
		case "scaley":
			if len(n) > 0 {
				ay = n[0]
			}
		case "skew":
			if len(n) == 1 {
				kx = n[0]
			} else if len(n) >= 2 {
				kx, ky = n[0], n[1]
			}
		case "skewx":
			if len(n) > 0 {
				kx = n[0]
			}
		case "skewy":
			if len(n) > 0 {
				ky = n[0]
			}
		case "flip":
			fx, fy = -1, -1
		case "flipx":
			fx = -1
		case "flipy":
			fy = -1
		case "move", "translate":
			if len(n) >= 2 {
				moveX, moveY = int(math.Round(n[0])), int(math.Round(n[1]))
			}
		case "reset", "none":
			rot, ax, ay, kx, ky, fx, fy = 0, 1, 1, 0, 0, 1, 1
		default:
			if _, err := strconv.ParseFloat(toks[i], 64); err != nil && bad == "" {
				bad = toks[i]
			}
		}
		i += len(n)
	}
	o.setTf(rot, ax*fx, ay*fy, kx, ky)
	return moveX, moveY, bad
}

// ---- where a pin's miniature is on the screen ---------------------------------------------

// pinMap maps between a pin's miniature (cells u, v from its corner) and
// the screen (cells from the board's corner on the screen). The miniature
// is first made at its size on the screen (gw × gh cells: miniW × miniH,
// scaled and zoomed), so its edges and words are drawn once, never
// stretched; then only turned, skewed and flipped.
type pinMap struct {
	gw, gh         int     // the miniature's size, made at its size on the screen
	cx, cy         float64 // its middle on the screen, from the board's corner
	a, b, c, dd    float64 // the turn, skew and flip, in square units (a cell is 1 across, 2 down)
	ia, ib, ic, id float64 // its inverse
	ok             bool
}

func (d *designState) pinMapOf(board, it *dOct) pinMap {
	z := d.z()
	rot, sx, sy, kx, ky := it.Tf.get()
	th := rot * math.Pi / 180
	cs, sn := math.Cos(th), math.Sin(th)
	fx, fy := math.Copysign(1, sx), math.Copysign(1, sy)
	// flip, then skew, then turn
	s00, s01, s10, s11 := fx, kx*fy, ky*fx, fy
	m00 := cs*s00 - sn*s10
	m01 := cs*s01 - sn*s11
	m10 := sn*s00 + cs*s10
	m11 := sn*s01 + cs*s11
	det := m00*m11 - m01*m10
	p := pinMap{
		gw: max(3, int(math.Round(miniW*math.Abs(sx)*z))),
		gh: max(1, int(math.Round(miniH*math.Abs(sy)*z))),
		cx: (float64(1+it.X) + miniW/2.0) * z,
		cy: (float64(1+it.Y) + miniH/2.0) * z,
		a:  m00, b: m01, c: m10, dd: m11,
	}
	if math.Abs(det) > 1e-9 {
		p.ia, p.ib, p.ic, p.id, p.ok = m11/det, -m01/det, -m10/det, m00/det, true
	}
	return p
}

// toScreen is where mini point (u, v) is, from the board's screen corner.
func (p pinMap) toScreen(u, v float64) (float64, float64) {
	x, y := u-float64(p.gw)/2, (v-float64(p.gh)/2)*2
	return p.cx + p.a*x + p.b*y, p.cy + (p.c*x+p.dd*y)/2
}

// toMini is the mini point at screen point (sx, sy), from the board's corner.
func (p pinMap) toMini(sx, sy float64) (float64, float64) {
	x, y := sx-p.cx, (sy-p.cy)*2
	return p.ia*x + p.ib*y + float64(p.gw)/2, (p.ic*x+p.id*y)/2 + float64(p.gh)/2
}

// inside is whether mini point (u, v) is on the miniature.
func (p pinMap) inside(u, v float64) bool {
	return u >= 0 && v >= 0 && u < float64(p.gw) && v < float64(p.gh)
}

// pinAt is the pin of board o at screen cell (bx, by) from its corner, the
// top one, or -1.
func (d *designState) pinAt(o *dOct, bx, by int) int {
	for j := len(o.Items) - 1; j >= 0; j-- {
		it := o.Items[j]
		if it == nil {
			continue
		}
		p := d.pinMapOf(o, it)
		if !p.ok {
			continue
		}
		if u, v := p.toMini(float64(bx)+0.5, float64(by)+0.5); p.inside(u, v) {
			return j
		}
	}
	return -1
}

// ---- drawing --------------------------------------------------------------------------------

// boardLayers draws a board: its frame and name, a cork texture, and each
// pin's miniature, transformed, with its pin.
func (d *designState) boardLayers(o *dOct) []*lipgloss.Layer {
	x, y, w, h := d.rect(o)
	selected := d.sel == o.ID
	edge := mixHex(o.Color, cText, 0.1)
	if selected {
		edge = mixHex(o.Color, cText, 0.45)
	}
	if d.dropOn == o.ID {
		edge = cAccent
	}
	cork := mixHex(cBG, o.Color, 0.2)
	if w < 6 || h < 3 {
		pill := mixHex(cBG, o.Color, 0.55)
		return []*lipgloss.Layer{pillLayer(x, y, w, h, kindIcons[kBoard]+o.Header, pill)}
	}
	c := newBoardCanvas(w, h)
	put := func(cx, cy int, s, fg, bg string, bold bool) {
		if cx < 0 || cy < 0 || cx >= w || cy >= h {
			return
		}
		if bg == "" { // clear: what is there shows through
			bg = colorHex(c.Lines[cy][cx].Style.Bg, cork)
		}
		stl := uv.Style{Fg: rgbaOf(fg), Bg: rgbaOf(bg)}
		if bold {
			stl.Attrs = uv.AttrBold
		}
		c.Lines[cy][cx] = uv.Cell{Content: s, Width: 1, Style: stl}
	}
	speck := mixHex(cork, o.Color, 0.45)
	for cy := 0; cy < h; cy++ {
		for cx := 0; cx < w; cx++ {
			s, fg := " ", cText
			switch {
			case cy == 0 && cx == 0:
				s, fg = "╭", edge
			case cy == 0 && cx == w-1:
				s, fg = "╮", edge
			case cy == h-1 && cx == 0:
				s, fg = "╰", edge
			case cy == h-1 && cx == w-1:
				s, fg = "╯", edge
			case cy == 0 || cy == h-1:
				s, fg = "─", edge
			case cx == 0 || cx == w-1:
				s, fg = "│", edge
			case (cx*3+cy*7)%11 == 0: // the cork's grain
				s, fg = "·", speck
			}
			put(cx, cy, s, fg, cork, false)
		}
	}
	// Its name on the top edge.
	name := o.Header
	if name == "" {
		name = kindNames[kBoard]
	}
	if n := len(drawerThings(o)); n > 0 {
		name += " · " + plural(n, "pin")
	}
	title := []rune(" " + kindIcons[kBoard] + " " + name + " ")
	if selected && d.editing == partHeader {
		title = []rune(" " + kindIcons[kBoard] + " " + o.Header + "▏")
	}
	for i, r := range title {
		if 2+i >= w-2 {
			break
		}
		put(2+i, 0, string(r), mixHex(o.Color, cText, 0.5), cork, true)
	}
	// The pins, first to last (the last on top), inside the frame.
	in := func(cx, cy int, r, fg, bg string, bold bool) {
		if cx >= 1 && cy >= 1 && cx < w-1 && cy < h-1 {
			put(cx, cy, r, fg, bg, bold)
		}
	}
	for j, it := range o.Items {
		if it == nil {
			continue
		}
		sel := d.pin.active && d.pin.board == o.ID && d.pin.at == j
		if p := d.pinMapOf(o, it); p.ok {
			drawPin(it, p, sel, in)
		}
	}
	l := lipgloss.NewLayer("").X(x).Y(y) // its cells are made already
	d.preCells(l, c)
	return d.octExtras([]*lipgloss.Layer{l}, o, x, y, w, h, edge, cork, selected, d.linkMode && d.linkFrom == o.ID)
}

// ---- choosing and changing a pin ------------------------------------------------------------

// pinSel is the chosen pin: board's thing at.
type pinSel struct {
	board, at int
	active    bool
}

// pinPress is a press on a pin, not yet a click (it opens it) or a drag
// (it moves it).
type pinPress struct {
	board, at, sx, sy int
	origX, origY      int
	moved             bool
}

// chosenPin is the chosen pin's board and thing, if one is chosen.
func (d *designState) chosenPin() (*dOct, *dOct) {
	if !d.pin.active {
		return nil, nil
	}
	b := d.byID(d.pin.board)
	if b == nil || d.pin.at < 0 || d.pin.at >= len(b.Items) || b.Items[d.pin.at] == nil {
		d.pin = pinSel{}
		return nil, nil
	}
	return b, b.Items[d.pin.at]
}

// sayPin says the chosen pin's transform.
func (m *Model) sayPin() {
	if _, it := m.ds().chosenPin(); it != nil {
		m.say("Pin “" + itemTitle(it) + "”: " + tfText(it.Tf))
	}
}

// designPinStep chooses the board's next pin (step 1) or last (-1).
func (m *Model) designPinStep(b *dOct, step int) {
	d := m.ds()
	idx := drawerThings(b)
	if len(idx) == 0 {
		m.say(octName(b) + " has no pins yet: drop a page, note or shape on it.")
		return
	}
	at := -1
	for i, j := range idx {
		if d.pin.active && d.pin.board == b.ID && d.pin.at == j {
			at = i
		}
	}
	next := 0
	switch {
	case at < 0 && step < 0:
		next = len(idx) - 1
	case at >= 0:
		next = ((at+step)%len(idx) + len(idx)) % len(idx)
	}
	d.pin = pinSel{board: b.ID, at: idx[next], active: true}
	d.sel, d.editing = b.ID, partNone
	d.dirty = true
	m.sayPin()
}

// designPinKey handles a key with a pin chosen. It says whether it did.
func (m *Model) designPinKey(name string) (bool, tea.Cmd) {
	d := m.ds()
	b, it := d.chosenPin()
	if it == nil {
		return false, nil
	}
	rot, sx, sy, kx, ky := it.Tf.get()
	moved := func(dx, dy int) {
		maxX, maxY := max(0, b.W-2-miniW), max(0, b.H-2-miniH)
		it.X, it.Y = max(-miniW/2, min(maxX+miniW/2, it.X+dx)), max(-miniH/2, min(maxY+miniH/2, it.Y+dy))
	}
	switch name {
	case "r", "R", "shift+r":
		step := 15.0
		if name != "r" {
			step = -15
		}
		it.setTf(rot+step, sx, sy, kx, ky)
	case "s", "S", "shift+s":
		f := 1.1
		if name != "s" {
			f = 1 / 1.1
		}
		it.setTf(rot, sx*f, sy*f, kx, ky)
	case "w", "W", "shift+w":
		f := 1.1
		if name != "w" {
			f = 1 / 1.1
		}
		it.setTf(rot, sx*f, sy, kx, ky)
	case "h", "H", "shift+h":
		f := 1.1
		if name != "h" {
			f = 1 / 1.1
		}
		it.setTf(rot, sx, sy*f, kx, ky)
	case "k", "K", "shift+k":
		step := 0.15
		if name != "k" {
			step = -0.15
		}
		it.setTf(rot, sx, sy, kx+step, ky)
	case "j", "J", "shift+j":
		step := 0.15
		if name != "j" {
			step = -0.15
		}
		it.setTf(rot, sx, sy, kx, ky+step)
	case "f", "F", "shift+f":
		it.setTf(rot, -sx, sy, kx, ky)
	case "v", "V", "shift+v":
		it.setTf(rot, sx, -sy, kx, ky)
	case "alt+left", "alt+right", "alt+up", "alt+down", "alt+shift+left", "alt+shift+right", "alt+shift+up", "alt+shift+down":
		dx, dy, step := arrowStep(strings.TrimPrefix(name, "alt+"))
		moved(dx*step, dy*step)
	case "t", "T", "shift+t":
		return true, m.ask(pDesignTransform, "Transform for “"+itemTitle(it)+"” (rotate 30 scale 1.5 skew 0.2 flip move 2 -1 reset): ", tfText(it.Tf))
	case "enter":
		d.pin.active = false
		m.designRackOpen(b, d.pin.at)
		return true, nil
	case "tab", "shift+tab":
		m.designPinStep(b, map[bool]int{true: -1, false: 1}[name == "shift+tab"])
		return true, nil
	case "esc":
		d.pin = pinSel{}
		d.dirty = true
		m.say("Let go of the pin.")
		return true, nil
	default:
		return false, nil
	}
	d.dirty = true
	m.sayPin()
	return true, nil
}

// designSetTransform takes a transform typed at the prompt.
func (m *Model) designSetTransform(s string) {
	d := m.ds()
	b, it := d.chosenPin()
	if it == nil {
		return
	}
	dx, dy, bad := parseTf(it, s)
	if dx != 0 || dy != 0 {
		it.X, it.Y = it.X+dx, it.Y+dy
	}
	_ = b
	d.dirty = true
	d.save()
	if bad != "" {
		m.say(fmt.Sprintf("Did not understand “%s” · pin “%s”: %s", bad, itemTitle(it), tfText(it.Tf)))
		return
	}
	m.sayPin()
}

// designDeletePin takes the chosen pin off its board (Ctrl+Z brings it back).
func (m *Model) designDeletePin() {
	d := m.ds()
	b, it := d.chosenPin()
	if it == nil {
		return
	}
	b.Items = append(b.Items[:d.pin.at:d.pin.at], b.Items[d.pin.at+1:]...)
	d.pin = pinSel{}
	d.dirty = true
	d.save()
	m.say("Took " + itemTitle(it) + " off " + octName(b) + ". Ctrl+Z brings it back.")
}

// pinMotion drags a pressed pin: it moves on its board.
func (m *Model) pinMotion(sx, sy int) {
	d := m.ds()
	pp := d.pinPress
	b := d.byID(pp.board)
	if b == nil || pp.at >= len(b.Items) || b.Items[pp.at] == nil {
		d.pinPress = nil
		return
	}
	if !pp.moved && abs(sx-pp.sx) < 2 && abs(sy-pp.sy) < 1 {
		return
	}
	pp.moved = true
	it := b.Items[pp.at]
	z := d.z()
	it.X = pp.origX + int(math.Round(float64(sx-pp.sx)/z))
	it.Y = pp.origY + int(math.Round(float64(sy-pp.sy)/z))
	it.X = max(-miniW/2, min(b.W-2-miniW/2, it.X))
	it.Y = max(-miniH/2, min(b.H-2-miniH/2, it.Y))
	d.dirty = true
}

// pinRelease ends a press on a pin: not moved, it opens.
func (m *Model) pinRelease() {
	d := m.ds()
	pp := d.pinPress
	d.pinPress = nil
	if pp == nil {
		return
	}
	b := d.byID(pp.board)
	if b == nil {
		return
	}
	if pp.moved {
		m.sayPin()
		return
	}
	d.pin.active = false
	m.designRackOpen(b, pp.at)
}

// ---- drawing a pin ------------------------------------------------------------------------

// pinPut puts letter r at screen cell (cx, cy) from the board's corner.
type pinPut func(cx, cy int, r, fg, bg string, bold bool)

// pinQuarter is how many quarter turns (clockwise) a pin is turned by, and
// whether it is only that: turned by a whole quarter, not skewed.
func pinQuarter(t *dTf) (int, bool) {
	rot, _, _, kx, ky := t.get()
	if kx != 0 || ky != 0 {
		return 0, false
	}
	if r := math.Mod(math.Mod(rot, 90)+90, 90); r > 0.5 && r < 89.5 {
		return 0, false
	}
	return (int(math.Round(rot/90))%4 + 4) % 4, true
}

// drawPin draws pin it of a board: its miniature, its one label and its
// pin. Turned by quarters (or not at all), it is its miniature, made at its
// size on the screen, so its outline is whole and sharp. Turned by any
// other angle, or skewed, it is drawn as a shape: its cells filled, its
// four edges as joined lines, its label upright inside it.
func drawPin(it *dOct, p pinMap, sel bool, put pinPut) {
	fill, edge := miniColours(it, sel)
	if q, ok := pinQuarter(it.Tf); ok {
		drawPinBox(it, p, q, sel, put)
		return
	}
	// The cells it covers: the box round its corners.
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	gw, gh := float64(p.gw), float64(p.gh)
	for _, cr := range [][2]float64{{0, 0}, {gw, 0}, {0, gh}, {gw, gh}} {
		sx, sy := p.toScreen(cr[0], cr[1])
		minX, minY, maxX, maxY = math.Min(minX, sx), math.Min(minY, sy), math.Max(maxX, sx), math.Max(maxY, sy)
	}
	x0, y0 := int(math.Floor(minX)), int(math.Floor(minY))
	cw, ch := int(math.Ceil(maxX))-x0+1, int(math.Ceil(maxY))-y0+1
	inside := make([][]bool, ch)
	edgeAt := make([][]string, ch)
	for yy := range inside {
		inside[yy] = make([]bool, cw)
		edgeAt[yy] = make([]string, cw)
		for xx := range inside[yy] {
			u, v := p.toMini(float64(x0+xx)+0.5, float64(y0+yy)+0.5)
			inside[yy][xx] = p.inside(u, v)
		}
	}
	// Its edges: lines through the middles of its outer cells, each cell
	// the line letter nearest its slant (a cell is twice as tall as wide).
	cs := [4][2]float64{{0.5, 0.5}, {gw - 0.5, 0.5}, {gw - 0.5, gh - 0.5}, {0.5, gh - 0.5}}
	for i := range cs {
		ax, ay := p.toScreen(cs[i][0], cs[i][1])
		bx, by := p.toScreen(cs[(i+1)%4][0], cs[(i+1)%4][1])
		// One cell per step along its longer way; where it steps the
		// other way too, a slant letter joins the two.
		x1, y1 := int(math.Floor(ax)), int(math.Floor(ay))
		x2, y2 := int(math.Floor(bx)), int(math.Floor(by))
		dx, dy := x2-x1, y2-y1
		n := max(abs(dx), abs(dy))
		cellAt := func(k int) (int, int) {
			if n == 0 {
				return x1, y1
			}
			t := float64(k) / float64(n)
			return int(math.Round(float64(x1) + float64(dx)*t)), int(math.Round(float64(y1) + float64(dy)*t))
		}
		for k := 0; k <= n; k++ {
			cx, cy := cellAt(k)
			r := "─"
			if abs(dy) > abs(dx) {
				r = "│"
			}
			if k < n {
				nx, ny := cellAt(k + 1)
				if nx != cx && ny != cy {
					r = "╲"
					if (nx-cx)*(ny-cy) < 0 {
						r = "╱"
					}
				}
			}
			if xx, yy := cx-x0, cy-y0; xx >= 0 && yy >= 0 && xx < cw && yy < ch {
				if edgeAt[yy][xx] == "" || r != "─" && r != "│" {
					edgeAt[yy][xx] = r
				}
				inside[yy][xx] = true
			}
		}
	}
	// Its inside: on each row, the cells between its edges (it is a
	// parallelogram, so never hollow), and none past them.
	for yy := range inside {
		lo, hi := -1, -1
		for xx := range cw {
			if edgeAt[yy][xx] != "" {
				if lo < 0 {
					lo = xx
				}
				hi = xx
			}
		}
		for xx := range cw {
			inside[yy][xx] = lo >= 0 && xx >= lo && xx <= hi
		}
	}
	for yy := range inside {
		for xx := range inside[yy] {
			switch {
			case edgeAt[yy][xx] != "":
				put(x0+xx, y0+yy, edgeAt[yy][xx], edge, fill, false)
			case inside[yy][xx]:
				put(x0+xx, y0+yy, " ", cText, fill, false)
			}
		}
	}
	// Its icon and label, upright (turned letters cannot be read), in the
	// middle, each cut to the room inside its edges on that row.
	mxs, _ := p.toScreen(gw/2, gh/2)
	mid := int(math.Floor(mxs)) - x0
	room := func(yy int) (int, int) { // the inside cells of row yy round its middle, between its edges
		if yy < 0 || yy >= ch || mid < 0 || mid >= cw || !inside[yy][mid] || edgeAt[yy][mid] != "" {
			return 0, -1
		}
		lo, hi := mid, mid
		for lo > 0 && inside[yy][lo-1] && edgeAt[yy][lo-1] == "" {
			lo--
		}
		for hi < cw-1 && inside[yy][hi+1] && edgeAt[yy][hi+1] == "" {
			hi++
		}
		return lo, hi
	}
	// words puts text on row yy, centred on its middle, kept inside it.
	full := func(yy int) (int, int) { // all its cells on row yy, its edges too
		if yy < 0 || yy >= ch {
			return 0, -1
		}
		lo, hi := -1, -2
		for xx := range cw {
			if inside[yy][xx] {
				if lo < 0 {
					lo = xx
				}
				hi = xx
			}
		}
		return lo, hi
	}
	words := func(text string, yy int, fg string, bold bool) {
		lo, hi := room(yy)
		if hi-lo+1 < lipgloss.Width(text) { // too long between its edges: over them
			lo, hi = full(yy)
		}
		if hi < lo {
			return
		}
		rs := []rune(ansi.Truncate(text, hi-lo+1, "…"))
		start := min(max(mid-len(rs)/2, lo), hi-len(rs)+1)
		for i, r := range rs {
			put(x0+start+i, y0+yy, string(r), fg, fill, bold)
		}
	}
	icon, label := kindIcons[it.Kind], itemTitle(it)
	_, pinY := p.toScreen(gw/2, 0.5)
	pinRow := int(math.Floor(pinY)) - y0
	width := func(yy int) int {
		if yy == pinRow {
			return 0 // the pin is there
		}
		lo, hi := room(yy)
		if lo2, hi2 := full(yy); hi-lo+1 < lipgloss.Width(label) {
			lo, hi = lo2, hi2
		}
		return hi - lo + 1
	}
	_, my := p.toScreen(gw/2, gh/2)
	mrow := int(math.Floor(my)) - y0
	textFG := cText
	if sel {
		textFG = cAccent
	}
	// The label's row: the middle one, or the next one that holds it all,
	// or else the widest of them.
	row := mrow
	for _, r := range []int{mrow, mrow + 1, mrow - 1} {
		if width(r) >= lipgloss.Width(label) {
			row = r
			break
		}
		if width(r) > width(row) {
			row = r
		}
	}
	if width(row-1) >= 1 && p.gh >= 4 {
		words(icon, row-1, mixHex(it.Color, cText, 0.3), false)
		words(label, row, textFG, sel)
	} else if width(row) >= lipgloss.Width(icon+" "+label) {
		words(icon+" "+label, row, textFG, sel)
	} else {
		words(label, row, textFG, sel) // no room for its icon too
	}
	// The pin, through the top of it.
	px, py := p.toScreen(gw/2, 0.5)
	put(int(math.Floor(px)), int(math.Floor(py)), "●", "#e06c75", fill, true)
}

// drawPinBox draws a pin turned by q quarter turns (and maybe flipped): its
// miniature, upright, made at the size the turn gives it, its corners
// turned and mirrored as the pin is.
func drawPinBox(it *dOct, p pinMap, q int, sel bool, put pinPut) {
	_, sx, sy, _, _ := it.Tf.get()
	W, H := p.gw, p.gh
	if q%2 == 1 { // on its side: as wide as it was tall (a cell is twice as tall as wide)
		W, H = max(3, int(math.Round(float64(p.gh)*2))), max(1, int(math.Round(float64(p.gw)/2)))
	}
	g := miniOfSize(it, W, H, sel, true)
	if H >= 3 && W >= 2 {
		// The corners, clockwise from the top left, mirrored, then turned.
		at := [4][2]int{{0, 0}, {W - 1, 0}, {W - 1, H - 1}, {0, H - 1}}
		var c [4]miniCell
		for i, a := range at {
			c[i] = g[a[1]][a[0]]
		}
		mapR := func(m map[string]string, mc miniCell) miniCell {
			if r, ok := m[mc.r]; ok {
				mc.r = r
			}
			return mc
		}
		if sx < 0 {
			c = [4]miniCell{mapR(glyphFlipX, c[1]), mapR(glyphFlipX, c[0]), mapR(glyphFlipX, c[3]), mapR(glyphFlipX, c[2])}
		}
		if sy < 0 {
			c = [4]miniCell{mapR(glyphFlipY, c[3]), mapR(glyphFlipY, c[2]), mapR(glyphFlipY, c[1]), mapR(glyphFlipY, c[0])}
		}
		for range q {
			c = [4]miniCell{mapR(glyphTurn, c[3]), mapR(glyphTurn, c[0]), mapR(glyphTurn, c[1]), mapR(glyphTurn, c[2])}
		}
		for i, a := range at {
			g[a[1]][a[0]] = c[i]
		}
	}
	x0 := int(math.Round(p.cx - float64(W)/2))
	y0 := int(math.Round(p.cy - float64(H)/2))
	fill, _ := miniColours(it, sel)
	for yy, row := range g {
		for xx, mc := range row {
			fg, bold := mc.fg, false // a clear cell (bg "") keeps the cork behind
			if sel && mc.fg == cText && mc.r != " " {
				fg, bold = cAccent, true // its label, picked out
			}
			put(x0+xx, y0+yy, mc.r, fg, mc.bg, bold)
		}
	}
	// The pin, through the top of it (wherever the turn put its top).
	px, py := p.toScreen(float64(p.gw)/2, 0.5)
	cx := min(max(int(math.Floor(px)), x0), x0+W-1)
	cy := min(max(int(math.Floor(py)), y0), y0+H-1)
	// on the nearest edge, never on its label
	switch dl, dr, dt, db := cx-x0, x0+W-1-cx, cy-y0, y0+H-1-cy; min(dl, dr, dt, db) {
	case dt:
		cy = y0
	case db:
		cy = y0 + H - 1
	case dl:
		cx = x0
	default:
		cx = x0 + W - 1
	}
	put(cx, cy, "●", "#e06c75", fill, true)
}

// ---- turning the letters of the outline ----------------------------------------------------

// glyphTurn turns a line letter a quarter turn clockwise; glyphFlipX and
// glyphFlipY mirror one across and up and down. Letters not in them stay.
var (
	glyphTurn = map[string]string{
		"─": "│", "│": "─", "━": "┃", "┃": "━", "═": "║", "║": "═", "┈": "┊", "┊": "┈",
		"┌": "┐", "┐": "┘", "┘": "└", "└": "┌", "┏": "┓", "┓": "┛", "┛": "┗", "┗": "┏",
		"╔": "╗", "╗": "╝", "╝": "╚", "╚": "╔", "╭": "╮", "╮": "╯", "╯": "╰", "╰": "╭",
		"╱": "╲", "╲": "╱", "◹": "◿", "◿": "◺", "◺": "◸", "◸": "◹",
	}
	glyphFlipX = map[string]string{
		"┌": "┐", "┐": "┌", "└": "┘", "┘": "└", "┏": "┓", "┓": "┏", "┗": "┛", "┛": "┗",
		"╔": "╗", "╗": "╔", "╚": "╝", "╝": "╚", "╭": "╮", "╮": "╭", "╰": "╯", "╯": "╰",
		"╱": "╲", "╲": "╱", "◹": "◸", "◸": "◹", "◿": "◺", "◺": "◿",
	}
	glyphFlipY = map[string]string{
		"┌": "└", "└": "┌", "┐": "┘", "┘": "┐", "┏": "┗", "┗": "┏", "┓": "┛", "┛": "┓",
		"╔": "╚", "╚": "╔", "╗": "╝", "╝": "╗", "╭": "╰", "╰": "╭", "╮": "╯", "╯": "╮",
		"╱": "╲", "╲": "╱", "◹": "◿", "◿": "◹", "◸": "◺", "◺": "◸",
	}
)
