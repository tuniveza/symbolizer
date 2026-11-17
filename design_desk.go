package main

// ---- the desk and its pens (the user asked, 17 Nov 2026) ----------------------------------------
//
// A desk (Alt+N, then T) is a table-top seen from the front, made of text:
//
//	the back panel   a METAL BOARD on the left (brushed steel, ░▒): sticky
//	                 notes and other sheets are held on it by round magnets ●;
//	                 RACKS on the right: notepads stand upright in them, each
//	                 showing its own spine (notepadStyle: its cover colour,
//	                 binding and pattern)
//	the HOLDER       a cup on the table top, right of the back panel: the
//	                 pens stand in it, sticking up in their colours
//	the table top    a thick slab with a lip, a modesty panel and legs under it
//
// It is a container: drop a notepad on it and it goes in a rack, a note (or
// a page, slide, drawing) and it goes on the metal board, a pen and it goes
// in the holder. Anything else is refused with a message. Drag a thing out
// of it to take it back to the board.
//
// Keys, with a desk chosen: Enter chooses the first thing in it, the arrows
// the next or last, Enter uses the one chosen (a pen: pick it up; a
// notepad: open it), P adds a new pen to the holder, C and D give the
// chosen pen its next colour and its next kind, Esc stops choosing.
//
// A pen (kPen) is a drawing device: pen, fineliner, pencil, marker,
// highlighter, glitter pen or brush (its Layout), in its colour (its
// Color). Picked up (Enter on it, a click on it in a holder, a double-click
// on a loose one) it is what the mouse draws with on the board, over
// everything, notepads and sticky notes too: each kind leaves its own
// stroke (design.go's tint calls inkCell). Esc, Enter on its desk, or a
// click on the holder puts it back.

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

const (
	kDesk = "desk"
	kPen  = "pen"
)

// A desk's size at 100%, and a loose pen's.
const (
	deskW, deskH = 64, 22
	penW, penH   = 16, 2
)

func init() {
	kindIcons[kDesk], kindNames[kDesk] = "⊓", "desk"
	kindIcons[kPen], kindNames[kPen] = "✒", "pen"
	for _, k := range []string{"t", "T", "shift+t"} { // T for table-top (no other kind uses it)
		placeKeys[k] = kDesk
	}
}

// The parts of a desk, kept in each thing's Slot.
const (
	deskBoard  = iota // the metal board: notes and other sheets
	deskRack          // the racks: notepads
	deskHolder        // the holder: pens
)

var deskPartNames = []string{"metal board", "rack", "holder"}

// deskPartFor is the part of a desk that takes a thing of kind k (-1: none).
func deskPartFor(k string) int {
	switch k {
	case kNotepad:
		return deskRack
	case kPen:
		return deskHolder
	case kNote, kPage, kDrawing, kSlide:
		return deskBoard
	}
	return -1
}

// ---- the drawing devices ----------------------------------------------------------------

// Devices. 0 is the highlighter, so ink saved before there were devices
// (dMark.Dev 0) stays a highlighter's wash.
const (
	devHighlighter = iota
	devPen
	devFineliner
	devPencil
	devMarker
	devGlitter
	devBrush
	devCount
)

var devNames = [devCount]string{"highlighter", "pen", "fineliner", "pencil", "marker", "glitter pen", "brush"}

// penInks are the colours a pen can have: warm Emberistal ones, then a few
// bright ones (glitter looks best in those).
var penInks = []struct{ name, hex string }{
	{"ember", "#e8743b"}, {"amber", "#f2b56b"}, {"red", "#d64541"}, {"ink blue", "#5b8fd6"},
	{"graphite", "#9a9aa0"}, {"cream", "#efe4c4"}, {"yellow", "#ffd400"}, {"pink", "#ff4fa3"},
	{"cyan", "#22d3ee"}, {"violet", "#a259ff"}, {"lime", "#4ade80"}, {"gold", "#ffcf40"},
}

// inkName is the name of colour hex among penInks (else the hex itself).
func inkName(hex string) string {
	for _, k := range penInks {
		if strings.EqualFold(k.hex, hex) {
			return k.name
		}
	}
	return hex
}

// penDev is pen p's device.
func penDev(p *dOct) int { return ((p.Layout % devCount) + devCount) % devCount }

// newPen makes a pen (not on the board yet) of device dev in colour ink.
func (d *designState) newPen(dev int, ink string) *dOct {
	p := &dOct{ID: d.nextID, Kind: kPen, W: penW, H: penH, Layout: dev, Color: ink, Header: devNames[dev]}
	d.nextID++
	return p
}

// penTitle is how a pen is called: "pink glitter pen".
func penTitle(p *dOct) string {
	name := strings.TrimSpace(p.Header)
	if name == "" {
		name = devNames[penDev(p)]
	}
	return inkName(p.Color) + " " + name
}

// deskNew fills a new desk: its wood and a few pens in its holder, so it
// is ready to draw with at once.
func (d *designState) deskNew(o *dOct) {
	o.W, o.H = deskW, deskH
	o.Color = hslHex(22+float64(o.ID%5)*4, 0.42, 0.42) // warm wood
	for _, p := range []struct {
		dev int
		ink string
	}{{devPen, "#e8743b"}, {devPencil, "#9a9aa0"}, {devHighlighter, "#ffd400"}, {devGlitter, "#ff4fa3"}} {
		pen := d.newPen(p.dev, p.ink)
		pen.Slot = deskHolder
		o.Items = append(o.Items, pen)
	}
}

// deskState is what the desks and pens need beyond the board itself (none
// of it is saved).
type deskState struct {
	held        int     // the pen in hand (its ID); 0: none, or a toolbar highlighter
	dev         int     // its device
	ink         string  // its colour
	from        int     // the desk it was taken from (0: a loose pen)
	pick        deskSel // the thing chosen inside a desk
	brushN      int     // a brush's dabs so far: its width changes along a stroke
	twinkle     int     // the glitter's sparkle frame
	glitterOn   bool    // glitter ink showed in the last drawing: its sparkles twinkle
	glitterTick bool    // a twinkle is on its way
	refuse      int     // a thing dragged over a desk that does not take it: the desk's ID
}

// deskSel is the thing chosen inside a desk (by Enter and the arrows, or a click).
type deskSel struct {
	desk, at int
	active   bool
}

// device is the drawing device in hand: ok false for the toolbar's
// highlighters (and the rubber, or no pen at all).
func (d *designState) device() (dev int, ink string, ok bool) {
	if d.pen < 0 || d.pen == rubber || d.desk.held == 0 {
		return devHighlighter, "", false
	}
	return d.desk.dev, d.desk.ink, true
}

// inkMark is the mark the pen in hand leaves.
func (d *designState) inkMark() dMark {
	if dev, ink, ok := d.device(); ok {
		return dMark{Pen: 0, Over: d.over, Dev: int8(dev), Ink: ink}
	}
	return dMark{Pen: int8(d.pen), Over: d.over}
}

// brushWidths: how far a brush spreads round each dab, along a stroke.
var brushWidths = []int{0, 1, 1, 2, 2, 1, 0, 1, 2, 1}

// penSpread widens the cells one dab colours: a marker is thick, a brush
// swells and thins along its stroke; the others colour the cell itself.
func (d *designState) penSpread(x0, x1, y0, y1 int) (int, int, int, int) {
	dev, _, ok := d.device()
	if !ok {
		return x0, x1, y0, y1
	}
	switch dev {
	case devMarker:
		return x0 - 1, x1 + 1, y0, y1 + 1
	case devBrush:
		d.desk.brushN++
		r := brushWidths[(d.desk.brushN/2)%len(brushWidths)]
		return x0 - r, x1 + r, y0 - r/2, y1 + (r+1)/2
	}
	return x0, x1, y0, y1
}

// ---- picking up and putting back ------------------------------------------------------------

// penPickUp takes pen p (in desk, or loose when desk is nil) in hand.
func (m *Model) penPickUp(p, desk *dOct) {
	d := m.ds()
	d.pen = 0
	d.desk.held, d.desk.dev, d.desk.ink, d.desk.from = p.ID, penDev(p), p.Color, 0
	if desk != nil {
		d.desk.from = desk.ID
	}
	d.desk.brushN = 0
	d.linkMode, d.linkFrom, d.editing = false, -1, partNone
	d.dirty = true
	back := "Esc puts it down"
	if desk != nil {
		back = "Esc or Enter on the desk puts it back"
	}
	m.say("✒ " + penTitle(p) + " in hand: drag on the board to draw · " + back + ".")
}

// penDown puts the pen in hand down (back in its holder). It says whether
// there was one.
func (m *Model) penDown() bool {
	d := m.ds()
	if d.pen < 0 {
		return false
	}
	if _, _, ok := d.device(); ok {
		name := devNames[d.desk.dev]
		if desk := d.byID(d.desk.from); desk != nil && desk.Kind == kDesk {
			m.say("The " + inkName(d.desk.ink) + " " + name + " is back in " + deskName(desk) + "'s holder.")
		} else {
			m.say("The " + inkName(d.desk.ink) + " " + name + " is put down.")
		}
	} else {
		m.say("Pen put down.")
	}
	d.pen = -1
	d.desk.held, d.desk.from = 0, 0
	d.dirty = true
	return true
}

// deskName is how messages call desk o: its name, else "the desk".
func deskName(o *dOct) string {
	if strings.TrimSpace(o.Header) == "" {
		return "the desk"
	}
	return octName(o)
}

// heldIn is whether the pen in hand came from desk o.
func (d *designState) heldIn(o *dOct) bool {
	_, _, ok := d.device()
	return ok && o != nil && d.desk.from == o.ID
}

// openNotepadHook opens notepad pad, kept in desk, the way Enter on a
// notepad on the board opens it. The notepad's own code (design_notepad.go)
// sets it; until it does, Enter on a notepad in a desk only chooses it.
var openNotepadHook func(m *Model, desk, pad *dOct) tea.Cmd

// deskUse is Enter (or a click) on thing at in desk o.
func (m *Model) deskUse(o *dOct, at int) tea.Cmd {
	d := m.ds()
	if at < 0 || at >= len(o.Items) || o.Items[at] == nil {
		return nil
	}
	it := o.Items[at]
	d.desk.pick = deskSel{o.ID, at, true}
	d.dirty = true
	switch it.Kind {
	case kPen:
		if d.desk.held == it.ID && d.pen >= 0 {
			m.penDown()
			return nil
		}
		m.penPickUp(it, o)
	case kNotepad:
		if openNotepadHook != nil {
			return openNotepadHook(m, o, it)
		}
		m.say("▤ " + itemTitle(it) + " chosen in the rack. Drag it out to open it on the board.")
	default:
		m.say(kindIcons[it.Kind] + " " + itemTitle(it) + " is on the metal board: drag it off to write in it.")
	}
	return nil
}

// deskClick is a click (not a drag) on thing at in desk o: a pen is picked
// up; a notepad chosen already opens; anything else is chosen.
func (m *Model) deskClick(o *dOct, at int) tea.Cmd {
	d := m.ds()
	if at < 0 || at >= len(o.Items) || o.Items[at] == nil {
		return nil
	}
	it := o.Items[at]
	again := d.desk.pick.active && d.desk.pick.desk == o.ID && d.desk.pick.at == at
	d.sel = o.ID
	if it.Kind == kPen || (it.Kind == kNotepad && again) {
		return m.deskUse(o, at)
	}
	d.desk.pick = deskSel{o.ID, at, true}
	d.dirty = true
	m.sayDeskPick(o)
	return nil
}

// deskOrder is the order the arrows go through a desk's things: the metal
// board's, the racks', then the holder's.
func deskOrder(o *dOct) []int {
	var out []int
	for part := range len(deskPartNames) {
		for j, it := range o.Items {
			if it != nil && deskPartFor(it.Kind) == part {
				out = append(out, j)
			}
		}
	}
	return out
}

// deskThings are the indexes of the things in part of desk o.
func deskThings(o *dOct, part int) []int {
	var out []int
	for j, it := range o.Items {
		if it != nil && deskPartFor(it.Kind) == part {
			out = append(out, j)
		}
	}
	return out
}

func (m *Model) sayDeskPick(o *dOct) {
	d := m.ds()
	at := d.desk.pick.at
	if at < 0 || at >= len(o.Items) || o.Items[at] == nil {
		return
	}
	it := o.Items[at]
	part := deskPartNames[max(0, deskPartFor(it.Kind))]
	switch it.Kind {
	case kPen:
		m.say("✒ " + penTitle(it) + " in the holder · Enter: draw with it · C colour · D kind")
	case kNotepad:
		m.say("▤ " + itemTitle(it) + " in the " + part + " · Enter: open it")
	default:
		m.say(kindIcons[it.Kind] + " " + itemTitle(it) + " on the " + part + " · drag it off to write in it")
	}
}

// designDeskKey handles a key with a desk (or a loose pen) chosen. It says
// whether it did.
func (m *Model) designDeskKey(name string) (bool, tea.Cmd) {
	d := m.ds()
	o := d.byID(d.sel)
	if o == nil {
		d.desk.pick = deskSel{}
		return false, nil
	}
	if o.Kind == kPen {
		switch name {
		case "enter":
			if d.desk.held == o.ID && d.pen >= 0 {
				m.penDown()
			} else {
				m.penPickUp(o, nil)
			}
			return true, nil
		case "c", "C", "shift+c":
			m.penNext(o, true)
			return true, nil
		case "d", "D", "shift+d":
			m.penNext(o, false)
			return true, nil
		}
		return false, nil
	}
	if o.Kind != kDesk {
		d.desk.pick = deskSel{}
		return false, nil
	}
	pk := &d.desk.pick
	if pk.desk != o.ID || pk.at >= len(o.Items) || (pk.active && o.Items[pk.at] == nil) {
		*pk = deskSel{}
	}
	order := deskOrder(o)
	switch name {
	case "enter":
		if pk.active {
			return true, m.deskUse(o, pk.at)
		}
		if d.heldIn(o) {
			m.penDown()
			return true, nil
		}
		if len(order) == 0 {
			m.say("The desk is empty: drop notepads, notes or pens on it (P: a new pen).")
			return true, nil
		}
		*pk = deskSel{o.ID, order[0], true}
		d.dirty = true
		m.sayDeskPick(o)
		return true, nil
	case "left", "right", "up", "down":
		if !pk.active || len(order) == 0 {
			return false, nil // the arrows move the view
		}
		i := 0
		for k, j := range order {
			if j == pk.at {
				i = k
			}
		}
		step := 1
		if name == "left" || name == "up" {
			step = -1
		}
		pk.at = order[((i+step)%len(order)+len(order))%len(order)]
		d.dirty = true
		m.sayDeskPick(o)
		return true, nil
	case "p", "P", "shift+p":
		m.deskAddPen(o)
		return true, nil
	case "c", "C", "shift+c", "d", "D", "shift+d":
		if pk.active && o.Items[pk.at].Kind == kPen {
			m.penNext(o.Items[pk.at], strings.ContainsAny(name, "cC"))
			return true, nil
		}
	}
	return false, nil
}

// deskAddPen puts a new pen in desk o's holder: the first kind it has not
// got yet (else the next after its last), in its own colour.
func (m *Model) deskAddPen(o *dOct) {
	d := m.ds()
	have := map[int]bool{}
	last := 0
	for _, j := range deskThings(o, deskHolder) {
		have[penDev(o.Items[j])] = true
		last = penDev(o.Items[j])
	}
	dev := (last + 1) % devCount
	for _, k := range []int{devPen, devFineliner, devPencil, devMarker, devHighlighter, devGlitter, devBrush} {
		if !have[k] {
			dev = k
			break
		}
	}
	ink := penInks[(dev*5+len(o.Items))%len(penInks)].hex
	p := d.newPen(dev, ink)
	p.Slot = deskHolder
	o.Items = append(o.Items, p)
	d.desk.pick = deskSel{o.ID, len(o.Items) - 1, true}
	d.dirty = true
	m.say("New " + penTitle(p) + " in " + deskName(o) + "'s holder · Enter: draw with it · C colour · D kind")
}

// penNext gives pen p its next colour (colour) or its next kind.
func (m *Model) penNext(p *dOct, colour bool) {
	d := m.ds()
	if colour {
		i := -1
		for k, c := range penInks {
			if strings.EqualFold(c.hex, p.Color) {
				i = k
			}
		}
		p.Color = penInks[(i+1)%len(penInks)].hex
	} else {
		named := strings.TrimSpace(p.Header) == "" || p.Header == devNames[penDev(p)]
		p.Layout = (penDev(p) + 1) % devCount
		if named {
			p.Header = devNames[p.Layout]
		}
	}
	if d.desk.held == p.ID && d.pen >= 0 { // in hand: it draws so from now on
		d.desk.dev, d.desk.ink = penDev(p), p.Color
	}
	d.dirty = true
	m.say("✒ " + penTitle(p) + ".")
}

// deskPenHint is the status bar while a pen is in hand.
func (d *designState) deskPenHint() string {
	back := "Esc: put down"
	if desk := d.byID(d.desk.from); desk != nil && desk.Kind == kDesk {
		back = "Esc or Enter on the desk: put back"
	}
	return "✒ " + strings.ToUpper(devNames[d.desk.dev][:1]) + devNames[d.desk.dev][1:] + " · " + inkName(d.desk.ink) +
		"  ·  drag to draw  ·  " + back
}

// deskUnder is the desk at canvas cell (sx, sy) under the dragged o, or nil.
func (d *designState) deskUnder(o *dOct, sx, sy int) *dOct {
	for i := len(d.octs) - 1; i >= 0; i-- {
		p := d.octs[i]
		if p == o {
			continue
		}
		x, y, w, h := d.rect(p)
		if sx >= x && sx < x+w && sy >= y && sy < y+h {
			if p.Kind == kDesk {
				return p
			}
			return nil
		}
	}
	return nil
}

// deskRefused says why desk did not take o, dropped on it, and moves o
// off it.
func (m *Model) deskRefused(o, desk *dOct) {
	d := m.ds()
	if o == nil || desk == nil {
		return
	}
	d.clearSpot(o)
	m.say("A desk takes notepads (its racks), notes and sheets (its metal board) and pens (its holder): not a " + kindNames[o.Kind] + ".")
}

// ---- glitter twinkles ------------------------------------------------------------------------

// glitterEvery is how often glitter ink twinkles, while it shows.
const glitterEvery = 420 * time.Millisecond

type glitterTickMsg struct{}

// glitterCmd asks for the next twinkle while glitter ink is on the screen.
func (m *Model) glitterCmd() tea.Cmd {
	d := m.design
	if !designAnims || d == nil || !m.inDesign() || m.help || d.desk.glitterTick || !d.desk.glitterOn {
		return nil
	}
	d.desk.glitterTick = true
	return tea.Tick(glitterEvery, func(time.Time) tea.Msg { return glitterTickMsg{} })
}

// glitterTicked: the sparkles move on, and the board is drawn afresh.
func (m *Model) glitterTicked() {
	if d := m.design; d != nil {
		d.desk.glitterTick = false
		d.desk.twinkle++
		d.frameAt = time.Time{}
	}
}

// ---- ink of each device ----------------------------------------------------------------------

type devLook struct {
	bg, fg color.Color
	dev    int8
	ink    string
	pen    int8
	over   bool
	void   bool
}

type devResult struct {
	bg, fg color.Color
}

// inkHash is a fixed scatter for board cell (x, y).
func inkHash(x, y int) uint32 {
	h := uint32(x)*73856093 ^ uint32(y)*19349663
	h ^= h >> 13
	h *= 0x5bd1e995
	return h ^ h>>15
}

// inkOf is mark mk's colour.
func inkOf(mk dMark) string {
	if mk.Ink != "" {
		return mk.Ink
	}
	return penColors[min(max(int(mk.Pen), 0), 7)]
}

// lineDev: a device that draws a line of letters on empty cells.
func lineDev(dev int8) bool { return dev == devPen || dev == devFineliner || dev == devPencil }

// inkCell lays mark mk (not a highlighter's) on canvas cell c at board
// cell (wx, wy): each device its own way.
func (d *designState) inkCell(c *uv.Cell, mk dMark, wx, wy int, done map[devLook]devResult) {
	void := strings.TrimSpace(c.Content) == ""
	k := devLook{c.Style.Bg, c.Style.Fg, mk.Dev, mk.Ink, mk.Pen, mk.Over, void}
	r, ok := done[k]
	if !ok {
		nbg, nfg := inkColours(mk, colorHex(c.Style.Bg, cBG), colorHex(c.Style.Fg, cText), void)
		r = devResult{hex(nbg), hex(nfg)}
		done[k] = r
	}
	c.Style.Bg, c.Style.Fg = r.bg, r.fg
	if mk.Dev == devGlitter {
		d.desk.glitterOn = true
	}
	if !void {
		return
	}
	if g := d.inkGlyph(mk, wx, wy); g != "" {
		c.Content, c.Width = g, 1
	}
}

// inkColours are the colours mark mk (not a highlighter's) gives a cell of
// colours bg and fg, empty (void) or with a letter on it. Design's board
// and Draw mode (draw.go) both use it, so a device looks the same in both.
func inkColours(mk dMark, bg, fg string, void bool) (nbg, nfg string) {
	ink := inkOf(mk)
	switch mk.Dev {
	case devPen:
		nbg, nfg = bg, ink
		if !void {
			nbg, nfg = mixHex(bg, ink, 0.16), mixHex(fg, ink, 0.6)
			if mk.Over {
				nfg = ink
			}
		}
	case devFineliner:
		nbg, nfg = bg, mixHex(ink, bg, 0.1)
		if !void {
			nfg = mixHex(fg, ink, 0.7)
		}
	case devPencil:
		lead := mixHex(ink, "#a0a0a0", 0.5)
		nbg, nfg = bg, mixHex(bg, lead, 0.75)
		if !void {
			nbg, nfg = mixHex(bg, lead, 0.12), fg
		}
	case devMarker:
		nbg = mixHex(bg, ink, 0.82)
		nfg = nbg
		if !void {
			nbg = mixHex(bg, ink, 0.7)
			nfg = "#f5efe0"
			if lum(nbg) > 0.5 {
				nfg = "#1a1210"
			}
			if mk.Over {
				nbg, nfg = mixHex(bg, ink, 0.75), mixHex(fg, ink, 0.5)
			}
		}
	case devBrush:
		nbg, nfg = mixHex(bg, ink, 0.62), mixHex(bg, ink, 0.9)
		if !void {
			nbg, nfg = mixHex(bg, ink, 0.5), fg
		}
	case devGlitter:
		nbg, nfg = mixHex(bg, ink, 0.42), mixHex(ink, "#ffffff", 0.55)
		if !void {
			nfg = fg
		}
	default:
		nbg, nfg = mixHex(bg, ink, 0.5), fg
	}
	return nbg, nfg
}

// highlightColours are the colours a highlighter's mark mk gives a cell of
// colours bg and fg: under the words, a wash; over them, the words take
// its colour too, and an empty cell is shaded (░). Shared with Draw mode.
func highlightColours(mk dMark, bg, fg string, void bool) (nbg, nfg string, shade bool) {
	pen := inkOf(mk)
	switch {
	case !mk.Over:
		return mixHex(bg, pen, 0.5), fg, false
	case void:
		nbg = mixHex(bg, pen, 0.38)
		return nbg, mixHex(nbg, pen, 0.55), true
	}
	return mixHex(bg, pen, 0.38), mixHex(fg, pen, 0.45), false
}

// inkGlyph is the letter an empty cell under mark mk shows: a line's
// piece, joined to its neighbours (pen, fineliner, pencil), a brush's
// bristle streaks, or glitter's sparkles; "" keeps the cell as it is.
func (d *designState) inkGlyph(mk dMark, wx, wy int) string {
	return inkGlyphFor(mk, wx, wy, d.desk.twinkle, func(x, y int) bool {
		n, ok := d.marks[[2]int{x, y}]
		return ok && lineDev(n.Dev)
	})
}

// inkGlyphFor is inkGlyph for any drawing: twinkle is glitter's sparkle
// frame, and on says whether cell (x, y) has a line device's ink (nil:
// none does).
func inkGlyphFor(mk dMark, wx, wy, twinkle int, on func(x, y int) bool) string {
	switch mk.Dev {
	case devBrush:
		return []string{"▓", "▒", " ", "▓", "▒"}[inkHash(wx, wy)%5]
	case devGlitter:
		h := inkHash(wx, wy)
		if h%3 != 0 {
			return ""
		}
		return glitterSparks[(int(h/3)+twinkle)%len(glitterSparks)]
	}
	if !lineDev(mk.Dev) {
		return ""
	}
	if on == nil {
		on = func(int, int) bool { return false }
	}
	conn := 0
	if on(wx, wy-1) {
		conn |= goUp
	}
	if on(wx, wy+1) {
		conn |= goDown
	}
	if on(wx-1, wy) {
		conn |= goLeft
	}
	if on(wx+1, wy) {
		conn |= goRight
	}
	if mk.Dev == devPencil { // dotted
		switch conn {
		case goLeft, goRight, goLeft | goRight:
			return "┄"
		case goUp, goDown, goUp | goDown:
			return "┆"
		}
	}
	if conn != 0 {
		if g, ok := boxLetters[conn]; ok {
			return g
		}
		return "┼"
	}
	switch {
	case on(wx+1, wy-1) || on(wx-1, wy+1):
		return "╱"
	case on(wx+1, wy+1) || on(wx-1, wy-1):
		return "╲"
	case mk.Dev == devPen:
		return "•"
	}
	return "·"
}

// glitterSparks are glitter ink's sparkles, in the order they twinkle.
var glitterSparks = []string{"✦", "✧", "·", "⋆"}

// ---- drawing a desk --------------------------------------------------------------------------

// deskPlan is where a desk's parts are, drawn w × h (cells from its corner,
// first and last inclusive).
type deskPlan struct {
	hutchW, hutchH int    // the back panel: columns 0 … hutchW-1, rows 0 … hutchH-1
	board, rack    [4]int // x0, y0, x1, y1
	cup            [4]int
	penTop         int // the highest a pen in the holder reaches
	top            int // the table top's row (the lip is under it)
}

func deskPlanOf(w, h int) deskPlan {
	fx, fy := float64(w)/deskW, float64(h)/deskH
	r := func(v float64) int { return int(v + 0.5) }
	p := deskPlan{hutchW: max(12, r(46*fx)), hutchH: max(5, r(13*fy))}
	p.top = p.hutchH
	by0 := max(1, r(2*fy))
	split := max(6, r(27*fx))
	p.board = [4]int{max(1, r(2*fx)), by0, split, p.hutchH - 2}
	p.rack = [4]int{split + 2, by0, p.hutchW - 3, p.hutchH - 2}
	cw := max(5, r(10*fx))
	ch := max(2, r(3*fy))
	cx0 := p.hutchW + (w-p.hutchW-cw)/2
	p.cup = [4]int{cx0, p.top - ch, cx0 + cw - 1, p.top - 1}
	p.penTop = max(1, p.cup[1]-max(2, r(6*fy)))
	return p
}

// deskWood is a desk's wood: its colour, darker and lighter.
func deskWood(o *dOct) (wood, dark, light string) {
	wood = o.Color
	if wood == "" {
		wood = "#7a4a2a"
	}
	return wood, mixHex(wood, "#140a06", 0.38), mixHex(wood, "#f3e0c0", 0.3)
}

// deskGridOf draws desk o, w × h on the screen, cell by cell (clicks read it too).
func (d *designState) deskGridOf(o *dOct, w, h int) *boxGrid {
	g := &boxGrid{w: w, h: h, c: make([][]boxCell, h)}
	bb := boardBG()
	for y := range g.c {
		g.c[y] = make([]boxCell, w)
		for x := range g.c[y] {
			g.c[y][x] = boxCell{r: " ", fg: cText, bg: bb}
		}
	}
	p := deskPlanOf(w, h)
	wood, dark, light := deskWood(o)
	if d.dropOn == o.ID {
		wood, light = mixHex(wood, cAccent, 0.45), mixHex(light, cAccent, 0.4)
	}
	panel := mixHex(wood, "#140a06", 0.22)
	pick := -1
	if d.desk.pick.active && d.desk.pick.desk == o.ID && d.sel == o.ID {
		pick = d.desk.pick.at
	}

	// The back panel, framed in wood, a little grain on it.
	for y := 0; y < p.hutchH; y++ {
		for x := 0; x < p.hutchW; x++ {
			if y == 0 || x == 0 || x == p.hutchW-1 {
				g.set(x, y, " ", light, wood)
			} else if (x+3*y)%11 == 0 {
				g.set(x, y, "╌", mixHex(panel, light, 0.18), panel)
			} else {
				g.set(x, y, " ", cText, panel)
			}
		}
	}
	d.deskTitle(g, o, p, wood, light)

	d.deskMetalBoard(g, o, p, pick)
	d.deskRacks(g, o, p, panel, pick)

	// The table top: a thick slab, lit along its top, with a lip under it.
	for x := range w {
		g.set(x, p.top, "▀", light, wood)
	}
	if p.top+1 < h {
		for x := 1; x < w-1; x++ {
			g.set(x, p.top+1, "▀", dark, bb)
		}
	}
	// Under it: the legs, and the modesty panel between them.
	lw := max(1, int(2*float64(w)/deskW+0.5))
	mp := max(1, int(3*float64(h)/deskH+0.5))
	for y := p.top + 2; y < h; y++ {
		for x := range w {
			leg := (x >= 2 && x < 2+lw) || (x <= w-3 && x > w-3-lw)
			switch {
			case leg && y == h-1:
				g.set(x, y, "▀", dark, bb) // its foot
			case leg:
				g.set(x, y, "▌", mixHex(dark, light, 0.25), dark)
			case x >= 2+lw && x <= w-3-lw && y < p.top+2+mp:
				r := " "
				if (x-2-lw)%9 == 8 {
					r = "│" // the panel's boards
				}
				g.set(x, y, r, mixHex(dark, "#000000", 0.3), mixHex(dark, panel, 0.5))
			}
		}
	}
	d.deskHolder(g, o, p, pick)
	return g
}

// deskTitle writes the desk's name in the top of its back panel (with the
// cursor while it is written).
func (d *designState) deskTitle(g *boxGrid, o *dOct, p deskPlan, wood, light string) {
	lo, hi := 2, p.hutchW-3
	if hi-lo < 3 {
		return
	}
	head := light
	if d.sel == o.ID {
		head = mixHex(light, cText, 0.5)
	}
	if d.sel == o.ID && d.editing == partHeader { // with the cursor and the highlight (design_field.go)
		x := lo + 1
		for _, c := range d.fieldLine(o, partHeader, kindIcons[kDesk]+" ", hi-lo-1) {
			fg, bg := head, ""
			switch c.mark {
			case markHL:
				fg, bg = cSelectedText, cSelection
			case markCur:
				fg, bg = boardBG(), cAccent
			case markCaret:
				fg = cAccent
			}
			x = g.text(x, 0, hi, c.g, fg, bg, true, "")
		}
		return
	}
	name := strings.TrimSpace(o.Header)
	if name == "" {
		name = "desk"
	}
	t := ansi.Truncate(" "+kindIcons[kDesk]+" "+name+" ", hi-lo+1, "…")
	g.text(lo+(hi-lo+1-lipgloss.Width(t))/2, 0, hi, t, head, wood, true, "")
}

// noteMagnets: the colours of the magnets that hold notes on the metal board.
var noteMagnets = []string{"#e8743b", "#5b8fd6", "#4ade80", "#e8e8e8", "#ff4fa3"}

// deskMetalBoard draws the brushed-steel board and the notes held on it.
func (d *designState) deskMetalBoard(g *boxGrid, o *dOct, p deskPlan, pick int) {
	x0, y0, x1, y1 := p.board[0], p.board[1], p.board[2], p.board[3]
	if x1-x0 < 3 || y1-y0 < 2 {
		return
	}
	steel := mixHex("#8f98a3", cBG, 0.3)
	rim := mixHex(steel, "#20242a", 0.45)
	sheen := mixHex(steel, "#e8edf2", 0.3)
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			switch {
			case y == y0 || y == y1:
				g.set(x, y, "─", sheen, rim)
			case x == x0 || x == x1:
				g.set(x, y, "│", sheen, rim)
			default:
				r := "░" // brushed: fine streaks along it
				if (x*5+y*17)%23 < 4 || (x+y*3)%13 == 0 {
					r = "▒"
				}
				g.set(x, y, r, sheen, steel)
			}
		}
	}
	for _, c := range [][2]int{{x0, y0}, {x1, y0}, {x0, y1}, {x1, y1}} {
		g.set(c[0], c[1], "•", sheen, rim) // its screws
	}
	idx := deskThings(o, deskBoard)
	iw, ih := x1-x0-1, y1-y0-1 // inside
	if len(idx) == 0 {
		if iw >= 12 {
			t := "notes here"
			g.text(x0+1+(iw-len(t))/2, y0+1+ih/2, x1-1, t, mixHex(steel, cText, 0.5), steel, false, "")
		}
		return
	}
	nw := max(5, int(11*float64(g.w)/deskW+0.5))
	nh := max(2, int(4*float64(g.h)/deskH+0.5))
	cols := max(1, (iw-1)/(nw+1))
	rows := max(1, (ih-1)/(nh+1))
	shown := min(len(idx), cols*rows)
	for n := 0; n < shown; n++ {
		j := idx[n]
		it := o.Items[j]
		cx := x0 + 2 + (n%cols)*(nw+1)
		cy := y0 + 1 + (n/cols)*(nh+1)
		if cx+nw-1 >= x1 || cy+nh-1 >= y1 {
			continue
		}
		paper := it.Color
		if it.Kind != kNote || paper == "" {
			paper = "#efe4c4"
		}
		if j == pick {
			paper = mixHex(paper, cAccent, 0.4)
		}
		ink := "#2b1608"
		act := "desk:" + itoa(j)
		lines := append([]string{ansi.Truncate(itemTitle(it), nw, "…")}, pinText(it, nw)...)
		for yy := 0; yy < nh; yy++ {
			for xx := 0; xx < nw; xx++ {
				g.c[cy+yy][cx+xx] = boxCell{r: " ", fg: ink, bg: paper, act: act}
			}
			if yy > 0 && yy-1 < len(lines) {
				g.text(cx, cy+yy, cx+nw-1, lines[yy-1], ink, paper, yy == 1, act)
			}
		}
		g.set(cx+nw, cy+1, " ", rim, mixHex(steel, "#000000", 0.3)) // its shadow
		magnet := "●"
		if j == pick {
			magnet = "◉"
		}
		g.text(cx+nw/2, cy, cx+nw-1, magnet, noteMagnets[it.ID%len(noteMagnets)], paper, true, act)
	}
	if more := len(idx) - shown; more > 0 {
		t := "+" + itoa(more)
		g.text(x1-len(t), y1, x1-1, t, cText, rim, true, "")
	}
}

// padNumber is notepad structure it's number (from "notepad:N"), or 0.
func padNumber(it *dOct) int {
	var n int
	if _, err := fmt.Sscanf(it.From, "notepad:%d", &n); err == nil {
		return n
	}
	return 0
}

// padTop is the letter across the top of a notepad's spine: its binding.
func padTop(s padStyle) string {
	switch s.Binding {
	case bindSpiral:
		return "◦"
	case bindGlued:
		return "▀"
	case bindStitched:
		return "┼"
	case bindClip:
		return "╤"
	}
	return "▀"
}

// deskPadPattern is the letter a notepad's cover is patterned with.
func deskPadPattern(s padStyle) string {
	return []string{" ", "─", "·", "▚", "≈", "┆", "◇"}[s.Pattern%7]
}

// deskRacks draws the racks and the notepads standing in them, each its own spine.
func (d *designState) deskRacks(g *boxGrid, o *dOct, p deskPlan, panel string, pick int) {
	x0, y0, x1, y1 := p.rack[0], p.rack[1], p.rack[2], p.rack[3]
	if x1-x0 < 3 || y1-y0 < 2 {
		return
	}
	steel := mixHex("#aeb4ba", panel, 0.25)
	spine := max(1, int(2*float64(g.w)/deskW+0.5))
	step := spine + 1
	slots := max(1, (x1-x0)/step)
	left := x0 + ((x1-x0)-slots*step)/2
	for i := 0; i <= slots; i++ { // the rods between the slots
		for y := y0 + 1; y < y1; y++ {
			g.set(left+i*step, y, "│", steel, panel)
		}
	}
	for x := x0; x <= x1; x++ { // the bar along the bottom
		r := "━"
		if (x-left)%step == 0 && x >= left && x <= left+slots*step {
			r = "┷"
		}
		g.set(x, y1, r, steel, panel)
	}
	idx := deskThings(o, deskRack)
	if len(idx) == 0 {
		if x1-x0 >= 8 {
			g.text(x0+(x1-x0+1-5)/2, y1, x1, " rack ", mixHex(steel, panel, 0.3), panel, false, "")
		}
		return
	}
	shown := min(len(idx), slots)
	for n := 0; n < shown; n++ {
		j := idx[n]
		it := o.Items[j]
		num := padNumber(it)
		s := notepadStyle(max(1, num))
		sx := left + n*step + 1
		top := y0 + 1 + (n % 2) // not all the same height
		act := "desk:" + itoa(j)
		cover := s.Cover
		if j == pick {
			g.text(sx, y0, sx+spine-1, strings.Repeat("▾", spine), cAccent, panel, true, act)
		}
		for y := top; y < y1; y++ {
			for x := sx; x < sx+spine; x++ {
				r, fg := deskPadPattern(s), mixHex(cover, s.Paper, 0.45)
				switch {
				case y == top:
					r, fg = padTop(s), mixHex(cover, "#e8edf2", 0.55)
				case y == y1-1 && x == sx+spine-1:
					r, fg = "▐", s.Paper // the pages' edge
				}
				g.c[y][x] = boxCell{r: r, fg: fg, bg: cover, act: act}
			}
		}
		if num > 0 && y1-top >= 4 { // its number down the spine
			label := itoa(num)
			if len(label) <= spine {
				g.text(sx, top+(y1-top)/2, sx+spine-1, label, s.Paper, cover, true, act)
			}
		}
	}
	if more := len(idx) - shown; more > 0 {
		t := "+" + itoa(more)
		g.text(x1-len(t)+1, y1, x1, t, cText, panel, true, "")
	}
}

// penUp is how a pen standing in a holder looks: the letter at its top and
// the letter of its body.
func penUp(dev int) (top, body string) {
	switch dev {
	case devPen:
		return "▴", "┃"
	case devFineliner:
		return "╿", "│"
	case devPencil:
		return "▲", "┃"
	case devMarker:
		return "▇", "█"
	case devHighlighter:
		return "◢", "█"
	case devGlitter:
		return "✦", "┃"
	case devBrush:
		return "Ψ", "│"
	}
	return "▴", "┃"
}

// deskHolder draws the cup on the table top and the pens standing in it.
func (d *designState) deskHolder(g *boxGrid, o *dOct, p deskPlan, pick int) {
	x0, y0, x1, y1 := p.cup[0], p.cup[1], p.cup[2], p.cup[3]
	if x1-x0 < 3 || y0 < 1 {
		return
	}
	bb := boardBG()
	clay := mixHex("#b5603a", cBG, 0.15)
	glaze := mixHex(clay, "#f3e0c0", 0.35)
	idx := deskThings(o, deskHolder)
	slots := max(1, (x1-x0)/2)
	shown := min(len(idx), slots)
	for n := 0; n < shown; n++ {
		j := idx[n]
		it := o.Items[j]
		x := x0 + 1 + n*2
		act := "desk:" + itoa(j)
		if d.desk.held == it.ID && d.pen >= 0 { // in hand: its place is empty
			g.c[y0-1][x] = boxCell{r: "·", fg: mixHex(bb, cText, 0.4), bg: bb, act: act}
			continue
		}
		dev := penDev(it)
		top, body := penUp(dev)
		ink := it.Color
		reach := p.penTop + int(inkHash(it.ID, 7)%3) // not all the same height
		reach = min(reach, y0-1)
		for y := reach; y < y0; y++ {
			r, fg, bg := body, ink, bb
			switch {
			case y == reach:
				r = top
				if dev == devPencil {
					fg = mixHex(ink, "#f3e0c0", 0.4) // sharpened wood
				}
			case dev == devGlitter && (y-reach)%2 == 0:
				r, fg = "✧", mixHex(ink, "#ffffff", 0.5)
			case dev == devHighlighter || dev == devMarker:
				fg = mixHex(ink, "#000000", 0.15)
			}
			if j == pick {
				bg = mixHex(bb, cAccent, 0.35)
			}
			g.c[y][x] = boxCell{r: r, fg: fg, bg: bg, bold: true, act: act}
		}
	}
	// The cup itself, over the pens' feet.
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			r, fg := " ", glaze
			switch {
			case y == y0:
				r = "▀" // its rim, lit
			case x == x0:
				r, fg = "▌", mixHex(glaze, clay, 0.5)
			case x == x1:
				r, fg = "▐", mixHex(clay, "#000000", 0.35)
			}
			g.c[y][x] = boxCell{r: r, fg: fg, bg: clay, act: "deskholder"}
		}
	}
	if y1 > y0 {
		label := ""
		if more := len(idx) - shown; more > 0 {
			label = "+" + itoa(more)
		} else if x1-x0 >= 6 && len(idx) == 0 {
			label = "pens"
		}
		if label != "" {
			g.text(x0+(x1-x0+1-len(label))/2, y0+1, x1-1, label, glaze, clay, true, "deskholder")
		}
	}
}

// deskLayers draws desk o.
func (d *designState) deskLayers(o *dOct) []*lipgloss.Layer {
	x, y, w, h := d.rect(o)
	wood, _, light := deskWood(o)
	if w < 30 || h < 10 { // too small (zoomed far out): a block of wood with its name
		return []*lipgloss.Layer{pillLayer(x, y, w, h, ansi.Truncate(kindIcons[kDesk]+o.Header, w, ""), mixHex(boardBG(), wood, 0.8))}
	}
	l := d.gridLayer(d.deskGridOf(o, w, h), x, y)
	return d.octExtras([]*lipgloss.Layer{l}, o, x, y, w, h, light, wood, false, d.linkMode && d.linkFrom == o.ID)
}

// gridLayer makes a layer of grid g's cells at (x, y).
func (d *designState) gridLayer(g *boxGrid, x, y int) *lipgloss.Layer {
	c := newBoardCanvas(g.w, g.h)
	for cy, row := range g.c {
		for cx, bc := range row {
			stl := uv.Style{Fg: rgbaOf(bc.fg), Bg: rgbaOf(bc.bg)}
			if bc.bold {
				stl.Attrs = uv.AttrBold
			}
			c.Lines[cy][cx] = uv.Cell{Content: bc.r, Width: 1, Style: stl}
		}
	}
	l := lipgloss.NewLayer("").X(x).Y(y) // its cells are made already
	d.preCells(l, c)
	return l
}

// deskHit is what a click does at (cx, cy) from desk o's corner, drawn
// w × h: "desk:N" on thing N, "deskholder" on the cup, "" elsewhere.
func (d *designState) deskHit(o *dOct, cx, cy, w, h int) string {
	if w < 30 || h < 10 || cx < 0 || cy < 0 || cx >= w || cy >= h {
		return ""
	}
	return d.deskGridOf(o, w, h).c[cy][cx].act
}

// deskKey is what a desk's drawing depends on beyond what every
// structure's does: each thing's colour and kind, the chosen one, the pen in hand.
func (d *designState) deskKey(o *dOct) string {
	var sb strings.Builder
	for _, it := range o.Items {
		if it != nil {
			fmt.Fprintf(&sb, "%d,%s,%d,%s,%s;", it.ID, it.Color, it.Layout, it.From, it.Body)
		}
	}
	if d.desk.pick.active && d.desk.pick.desk == o.ID && d.sel == o.ID {
		fmt.Fprintf(&sb, "p%d", d.desk.pick.at)
	}
	if d.pen >= 0 {
		fmt.Fprintf(&sb, "h%d", d.desk.held)
	}
	return sb.String()
}

// ---- a loose pen on the board -----------------------------------------------------------------

// penAcross is a pen lying on its side, n cells long, tip to the right:
// each cell's letter and colour.
func penAcross(dev int, ink string, n int) []boxCell {
	n = max(n, 4)
	end, body, tip := "╾", "━", "▶"
	switch dev {
	case devFineliner:
		end, body, tip = "╶", "─", "╼"
	case devPencil:
		end, body, tip = "▕", "═", "▷"
	case devMarker:
		end, body, tip = "▐", "█", "▶"
	case devHighlighter:
		end, body, tip = "▐", "▆", "◤"
	case devBrush:
		end, body, tip = "╺", "━", "≫"
	}
	out := make([]boxCell, n)
	for i := range out {
		r, fg := body, ink
		switch {
		case i == 0:
			r, fg = end, mixHex(ink, "#000000", 0.25)
		case i == n-1:
			r = tip
			if dev == devPencil {
				fg = mixHex(ink, "#f3e0c0", 0.4)
			}
		case dev == devGlitter && i%2 == 0:
			r, fg = []string{"✦", "✧"}[i/2%2], mixHex(ink, "#ffffff", 0.5)
		case dev == devBrush && i == n-2:
			r, fg = "▌", "#c0c4c8" // its ferrule
		}
		out[i] = boxCell{r: r, fg: fg}
	}
	return out
}

// penLayers draws a loose pen: lying on its side, its name under it.
func (d *designState) penLayers(o *dOct) []*lipgloss.Layer {
	x, y, w, h := d.rect(o)
	if w < 6 || h < 1 {
		return []*lipgloss.Layer{pillLayer(x, y, max(w, 1), max(h, 1), kindIcons[kPen], mixHex(boardBG(), o.Color, 0.6))}
	}
	bg := boardBG()
	sel := d.sel == o.ID
	if sel {
		bg = mixHex(bg, cAccent, 0.18)
	}
	g := &boxGrid{w: w, h: h, c: make([][]boxCell, h)}
	for yy := range g.c {
		g.c[yy] = make([]boxCell, w)
		for xx := range g.c[yy] {
			g.c[yy][xx] = boxCell{r: " ", fg: cText, bg: bg}
		}
	}
	held := d.desk.held == o.ID && d.pen >= 0
	for i, c := range penAcross(penDev(o), o.Color, w-2) {
		if held {
			c.r, c.fg = "┄", mixHex(bg, o.Color, 0.5) // in hand: only its outline is left
		}
		g.set(1+i, 0, c.r, c.fg, bg)
	}
	if h >= 2 {
		name := penTitle(o)
		if held {
			name = "in hand"
		}
		if sel && d.editing == partHeader {
			name = o.Header + "▏"
		}
		name = ansi.Truncate(name, w, "…")
		g.text((w-lipgloss.Width(name))/2, 1, w-1, name, mixHex(cDim, cText, 0.4), bg, sel, "")
	}
	l := d.gridLayer(g, x, y)
	return d.octExtras([]*lipgloss.Layer{l}, o, x, y, w, h, o.Color, bg, false, d.linkMode && d.linkFrom == o.ID)
}
