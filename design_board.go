package main

// ---- the board: things pinned on it, small, turned and stretched -----------------------
//
// A board (Alt+N, then B) is a pinboard. Drop a page, note or shape on it
// and it is pinned where you let go: drawn as a card (its outline, title
// and first lines of text) with a pin through its top. Each pin can be turned,
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
	"image/color"
	"math"
	"path/filepath"
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
// is first made at its size on the screen (gw × gh cells: pinW × pinH,
// scaled and zoomed), so its edges and words are drawn once, never
// stretched; then only turned, skewed and flipped.
type pinMap struct {
	gw, gh         int     // the miniature's size, made at its size on the screen
	cx, cy         float64 // its middle on the screen, from the board's corner
	a, b, c, dd    float64 // the turn, skew and flip, in square units (a cell is 1 across, 2 down)
	ia, ib, ic, id float64 // its inverse
	ok             bool
}

// boardFit is how much a board's pins are drawn smaller (1: not at all) so
// that every one of them, turned and scaled as it is, is inside the board:
// a board smaller than what is pinned on it shows it all, smaller, its
// pins' places shrunk with them. mnX, mnY: where the pins start, when one
// is pinned up or left of the board's corner.
func (d *designState) boardFit(board *dOct) (fit, mnX, mnY float64) {
	bw, bh := d.size(board)
	iw, ih := float64(max(bw-2, 1)), float64(max(bh-2, 1))
	mxX, mxY := iw, ih
	for _, it := range board.Items {
		if it == nil {
			continue
		}
		rot, sx, sy, kx, ky := it.Tf.get()
		th := rot * math.Pi / 180
		w, h2 := pinW*math.Abs(sx), 2*pinH*math.Abs(sy) // in square units: a cell is 1 across, 2 down
		w, h2 = w+math.Abs(kx)*h2, h2+math.Abs(ky)*w
		bx := math.Abs(math.Cos(th))*w + math.Abs(math.Sin(th))*h2
		by := (math.Abs(math.Sin(th))*w + math.Abs(math.Cos(th))*h2) / 2
		cx, cy := float64(it.X)+pinW/2.0, float64(it.Y)+pinH/2.0
		mnX, mnY = math.Min(mnX, cx-bx/2), math.Min(mnY, cy-by/2)
		mxX, mxY = math.Max(mxX, cx+bx/2), math.Max(mxY, cy+by/2)
	}
	fit = math.Min(1, math.Min(iw/(mxX-mnX), ih/(mxY-mnY)))
	return fit, mnX, mnY
}

func (d *designState) pinMapOf(board, it *dOct) pinMap {
	z := d.z()
	fit, mnX, mnY := d.boardFit(board)
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
		gw: max(3, int(math.Round(pinW*math.Abs(sx)*z*fit))),
		gh: max(1, int(math.Round(pinH*math.Abs(sy)*z*fit))),
		cx: (1 + (float64(it.X)+pinW/2.0-mnX)*fit) * z,
		cy: (1 + (float64(it.Y)+pinH/2.0-mnY)*fit) * z,
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
			pic := ""
			if it.Image != "" {
				pic = filepath.Join(d.imagesDir(), it.Image) // its picture fills its frame
			}
			drawPin(it, p, sel, pic, in)
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
		maxX, maxY := max(0, b.W-2-pinW), max(0, b.H-2-pinH)
		it.X, it.Y = max(-pinW/2, min(maxX+pinW/2, it.X+dx)), max(-pinH/2, min(maxY+pinH/2, it.Y+dy))
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
	fit, _, _ := d.boardFit(b)
	z := d.z() * fit // its pins drawn smaller: the mouse moves it as far as it goes on screen
	it.X = pp.origX + int(math.Round(float64(sx-pp.sx)/z))
	it.Y = pp.origY + int(math.Round(float64(sy-pp.sy)/z))
	it.X = max(-pinW/2, min(b.W-2-pinW/2, it.X))
	it.Y = max(-pinH/2, min(b.H-2-pinH/2, it.Y))
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

// pinW, pinH are a pin's card size on the board at 100% (before its own
// scale): room for its title and the first lines of its text.
const pinW, pinH = 20, 8

// pinText is what a pin's card shows under its title: its picture (as a
// word) and its text, wrapped to w cells, each paragraph on its own lines.
func pinText(it *dOct, w int) []string {
	var out []string
	if it.Image != "" {
		out = append(out, "▣ picture")
	}
	for _, para := range strings.Split(it.Body, "\n") {
		if strings.TrimSpace(para) == "" {
			if len(out) > 0 && out[len(out)-1] != "" {
				out = append(out, "")
			}
			continue
		}
		for _, l := range wrapWords(para, max(w, 1)) {
			out = append(out, ansi.Truncate(l, max(w, 1), "…"))
		}
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}

// pinLines are a pin's title and text, to show w cells wide in rows rows:
// cut, the last row shown ends in "…".
func pinLines(it *dOct, w, rows int) (title string, body []string) {
	title = ansi.Truncate(kindIcons[it.Kind]+" "+itemTitle(it), max(w, 1), "…")
	body = pinText(it, w)
	if len(body) > rows {
		body = body[:max(rows, 0)]
		if rows > 0 {
			last := []rune(body[rows-1])
			if len(last) >= w {
				last = last[:max(w-1, 0)]
			}
			body[rows-1] = string(last) + "…"
		}
	}
	return title, body
}

// pinCard is a pin drawn upright, w × h: its outline (its kind's), its
// title on the first row inside, and under it as much of its text as fits.
func pinCard(it *dOct, w, h int, sel bool) [][]miniCell {
	if h < 3 || w < 4 { // tiny (zoomed far out): a block of its colour with its name
		return miniOfSize(it, w, h, sel, true)
	}
	g := miniOfSize(it, w, h, sel, false)
	fill, _ := miniColours(it, sel)
	titleFG, textFG := cText, mixHex(cText, fill, 0.3)
	if sel {
		titleFG = cAccent
	}
	set := func(x, y int, s, fg string) {
		for i, r := range []rune(s) {
			if x+i >= 1 && x+i < w-1 && y >= 1 && y < h-1 {
				g[y][x+i] = miniCell{string(r), fg, fill}
			}
		}
	}
	title, body := pinLines(it, w-2, h-3)
	set(1+max(0, (w-2-len([]rune(title)))/2), 1, title, titleFG)
	if len(body) == 0 && h > 3 {
		set(2, 2, "empty", mixHex(cDim, fill, 0.2))
	}
	for i, l := range body {
		set(1, 2+i, l, textFG)
	}
	return g
}

// drawPin draws pin it of a board. Turned by quarters (or not at all) it
// is its card, made at its size on the screen, so its outline is whole and
// sharp. Turned by any other angle, or skewed, its outline is a fine line
// of dots (braille: 2 × 4 dots a cell) round its turned shape, so it is
// smooth and never a run of repeated letters; inside, its title and text
// stay upright to be read.
func drawPin(it *dOct, p pinMap, sel bool, pic string, put pinPut) {
	if q, ok := pinQuarter(it.Tf); ok {
		drawPinBox(it, p, q, sel, pic, put)
		return
	}
	var picture *boardCanvas // its picture, the size of its frame, turned with it
	if pic != "" {
		picture = pinPicture(pic, p.gw, p.gh)
	}
	fill, edge := miniColours(it, sel)
	gw, gh := float64(p.gw), float64(p.gh)
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, cr := range [][2]float64{{0, 0}, {gw, 0}, {0, gh}, {gw, gh}} {
		sx, sy := p.toScreen(cr[0], cr[1])
		minX, minY, maxX, maxY = math.Min(minX, sx), math.Min(minY, sy), math.Max(maxX, sx), math.Max(maxY, sy)
	}
	x0, y0 := int(math.Floor(minX))-1, int(math.Floor(minY))-1
	cw, ch := int(math.Ceil(maxX))-x0+2, int(math.Ceil(maxY))-y0+2
	inside := make([][]bool, ch)
	dots := make([][]uint8, ch)
	for yy := range inside {
		inside[yy] = make([]bool, cw)
		dots[yy] = make([]uint8, cw)
		for xx := range inside[yy] {
			u, v := p.toMini(float64(x0+xx)+0.5, float64(y0+yy)+0.5)
			inside[yy][xx] = p.inside(u, v)
		}
	}
	// Its outline in dots: each edge a line from corner to corner, a dot
	// a step (a cell is 2 dots across and 4 down).
	bit := [2][4]uint8{{0x01, 0x02, 0x04, 0x40}, {0x08, 0x10, 0x20, 0x80}}
	const eu, ev = 0.25, 0.125 // just inside its edges
	cs := [4][2]float64{{eu, ev}, {gw - eu, ev}, {gw - eu, gh - ev}, {eu, gh - ev}}
	for i := range cs {
		ax, ay := p.toScreen(cs[i][0], cs[i][1])
		bx, by := p.toScreen(cs[(i+1)%4][0], cs[(i+1)%4][1])
		ax, ay, bx, by = ax*2, ay*4, bx*2, by*4
		n := int(math.Ceil(math.Max(math.Abs(bx-ax), math.Abs(by-ay))))
		for k := 0; k <= n; k++ {
			t := float64(k) / float64(max(n, 1))
			dx, dy := int(math.Floor(ax+(bx-ax)*t)), int(math.Floor(ay+(by-ay)*t))
			cx, cy := floorDiv(dx, 2)-x0, floorDiv(dy, 4)-y0
			if cx >= 0 && cy >= 0 && cx < cw && cy < ch {
				dots[cy][cx] |= bit[dx-floorDiv(dx, 2)*2][dy-floorDiv(dy, 4)*4]
			}
		}
	}
	for yy := range inside {
		for xx := range inside[yy] {
			switch {
			case dots[yy][xx] != 0:
				bg := ""
				if inside[yy][xx] {
					bg = fill
				}
				put(x0+xx, y0+yy, string(rune(0x2800+int(dots[yy][xx]))), edge, bg, false)
			case inside[yy][xx] && picture != nil:
				u, v := p.toMini(float64(x0+xx)+0.5, float64(y0+yy)+0.5)
				pc := picture.Lines[min(max(int(v), 0), p.gh-1)][min(max(int(u), 0), p.gw-1)]
				put(x0+xx, y0+yy, "▀", colorHex(pc.Style.Fg, fill), colorHex(pc.Style.Bg, fill), false)
			case inside[yy][xx]:
				put(x0+xx, y0+yy, " ", cText, fill, false)
			}
		}
	}
	// Its title and text, upright, each row cut to the room inside it
	// round its middle on that row.
	mxs, mys := p.toScreen(gw/2, gh/2)
	mid := int(math.Floor(mxs)) - x0
	room := func(yy int) (int, int) {
		if yy < 0 || yy >= ch || mid < 0 || mid >= cw || !inside[yy][mid] || dots[yy][mid] != 0 {
			return 0, -1
		}
		lo, hi := mid, mid
		for lo > 0 && inside[yy][lo-1] && dots[yy][lo-1] == 0 {
			lo--
		}
		for hi < cw-1 && inside[yy][hi+1] && dots[yy][hi+1] == 0 {
			hi++
		}
		return lo, hi
	}
	// The rows with room, from its middle up and down: they hold its text.
	mrow := int(math.Floor(mys)) - y0
	top, bottom := mrow, mrow
	for lo, hi := room(top - 1); hi-lo+1 >= 3; lo, hi = room(top - 1) {
		top--
	}
	for lo, hi := room(bottom + 1); hi-lo+1 >= 3; lo, hi = room(bottom + 1) {
		bottom++
	}
	titleFG, textFG := cText, mixHex(cText, fill, 0.3)
	if sel {
		titleFG = cAccent
	}
	// As many rows as it has words for, centred on its middle (where it
	// is widest), inside the rows with room.
	widest := 0
	for r := top; r <= bottom; r++ {
		lo, hi := room(r)
		widest = max(widest, hi-lo+1)
	}
	_, all := pinLines(it, max(widest-1, 1), bottom-top)
	n := min(1+max(len(all), 1), bottom-top+1)
	top = min(max(mrow-(n-1)/2, top), bottom-n+1)
	title, body := pinLines(it, max(widest-1, 1), n-1)
	if len(body) == 0 && n > 1 {
		body = []string{"empty"}
	}
	line := func(r int, text, fg string, centre, bold bool) {
		lo, hi := room(r)
		if hi < lo {
			return
		}
		if lo < hi { // a cell of margin, when there is room
			lo++
		}
		rs := []rune(ansi.Truncate(text, hi-lo+1, "…"))
		start := lo
		if centre {
			start = min(max(mid-len(rs)/2, lo), hi-len(rs)+1)
		}
		for i, r2 := range rs {
			put(x0+start+i, y0+r, string(r2), fg, fill, bold)
		}
	}
	line(top, title, titleFG, true, sel)
	if picture != nil { // a picture: its title on it, and the picture is the rest
		px, py := p.toScreen(gw/2, 0.5)
		put(int(math.Floor(px)), int(math.Floor(py)), "●", "#e06c75", fill, true)
		return
	}
	// Its text flows into the rows under the title, as many words on each
	// as its room there holds; what does not fit ends in "…".
	var words []string
	for _, l := range pinText(it, 1<<20) {
		words = append(words, strings.Fields(l)...)
	}
	if len(words) == 0 && len(body) > 0 {
		words = body // "empty"
	}
	for r := top + 1; r <= bottom && len(words) > 0; r++ {
		lo, hi := room(r)
		w := hi - lo // a cell of margin
		if w < 1 {
			continue
		}
		row := ""
		for len(words) > 0 {
			next := words[0]
			if row != "" {
				next = row + " " + words[0]
			}
			if lipgloss.Width(next) > w && row != "" {
				break
			}
			row, words = next, words[1:]
		}
		if len(words) > 0 && (r == bottom || lipgloss.Width(row) > w) {
			row = ansi.Truncate(row, max(w-1, 0), "") + "…"
		}
		line(r, row, textFG, false, false)
	}
	// The pin, through the top of it.
	px, py := p.toScreen(gw/2, 0.5)
	put(int(math.Floor(px)), int(math.Floor(py)), "●", "#e06c75", fill, true)
}

// floorDiv is a ÷ b rounded down (for negative a too).
func floorDiv(a, b int) int {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

// pinPicture is the picture at path as w × h cells filling them all (cut
// to fit, never squeezed: a cell is two pixels, one above the other).
func pinPicture(path string, w, h int) *boardCanvas {
	w, h = max(w, 1), max(h, 1)
	key := fmt.Sprintf("cover|%s|%d|%d", path, w, h)
	picMu.Lock()
	defer picMu.Unlock()
	if c, ok := picCells[key]; ok {
		return c
	}
	c := newBoardCanvas(w, h)
	if img, ok := loadPictureLocked(path); ok {
		b := img.Bounds()
		iw, ih := float64(b.Dx()), float64(b.Dy())
		pw, ph := float64(w), float64(h*2)
		scale := math.Max(pw/iw, ph/ih) // it covers the frame
		ox, oy := (iw-pw/scale)/2, (ih-ph/scale)/2
		step := max(1, int(1/scale/3))
		px := func(x, y int) color.RGBA { // the average of the source pixels under pixel (x, y)
			x0, y0 := ox+float64(x)/scale, oy+float64(y)/scale
			x1, y1 := x0+1/scale, y0+1/scale
			var r, g, bl, n int
			for sy := int(y0); sy < max(int(y1), int(y0)+1); sy += step {
				for sx := int(x0); sx < max(int(x1), int(x0)+1); sx += step {
					cr, cg, cb, _ := img.At(b.Min.X+min(sx, b.Dx()-1), b.Min.Y+min(sy, b.Dy()-1)).RGBA()
					r, g, bl, n = r+int(cr>>8), g+int(cg>>8), bl+int(cb>>8), n+1
				}
			}
			n = max(n, 1)
			return color.RGBA{uint8(r / n), uint8(g / n), uint8(bl / n), 255}
		}
		cell := uv.Cell{Content: "▀", Width: 1}
		for row := range h {
			for col := range w {
				cell.Style.Fg, cell.Style.Bg = px(col, row*2), px(col, row*2+1)
				c.SetCell(col, row, &cell)
			}
		}
	}
	if picCellsKept += w * h; picCellsKept > 1_000_000 {
		clear(picCells)
		picCellsKept = w * h
	}
	picCells[key] = c
	return c
}

// drawPinBox draws a pin turned by q quarter turns (and maybe flipped): its
// miniature, upright, made at the size the turn gives it, its corners
// turned and mirrored as the pin is.
func drawPinBox(it *dOct, p pinMap, q int, sel bool, pic string, put pinPut) {
	_, sx, sy, _, _ := it.Tf.get()
	W, H := p.gw, p.gh
	if q%2 == 1 { // on its side: as wide as it was tall (a cell is twice as tall as wide)
		W, H = max(3, int(math.Round(float64(p.gh)*2))), max(1, int(math.Round(float64(p.gw)/2)))
	}
	g := pinCard(it, W, H, sel)
	if pic != "" && W >= 4 && H >= 3 {
		// Its picture fills its frame; its title goes on its bottom edge
		// (the pin is through its top).
		fill, _ := miniColours(it, sel)
		pc := pinPicture(pic, W-2, H-2)
		for yy := 1; yy < H-1; yy++ {
			for xx := 1; xx < W-1; xx++ {
				c := pc.Lines[yy-1][xx-1]
				g[yy][xx] = miniCell{"▀", colorHex(c.Style.Fg, fill), colorHex(c.Style.Bg, fill)}
			}
		}
		titleFG := mixHex(it.Color, cText, 0.5)
		if sel {
			titleFG = cAccent
		}
		t := []rune(" " + ansi.Truncate(kindIcons[it.Kind]+" "+itemTitle(it), max(W-6, 1), "…") + " ")
		for i, r := range t {
			if x := 2 + i; x < W-2 {
				g[H-1][x] = miniCell{string(r), titleFG, g[H-1][x].bg}
			}
		}
	}
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
