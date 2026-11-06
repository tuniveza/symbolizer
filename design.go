package main

import (
	"encoding/json"
	"fmt"
	"image/color"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Design mode: a board made of text. It zooms, it moves, and it holds
// octagonal structures (a header, text and a footer) that can be dragged,
// resized and linked to each other. A highlighter pen with 8 colours
// colours cells under the text (the text stays as it is) or over it (the
// text shows through, tinted).
//
//	~/.local/share/symbolizer/design/board.json
//
// Mouse: double-click places a structure (or writes in one), left-drag moves
// it, its ◢ corner resizes it, middle-drag (or left-drag on the empty board)
// moves the board, the wheel zooms, right-drag rubs out the pen.
// Keys: AltGr+L link mode, 1–8 pen, E rubber, O over/under, + − 0 zoom.
//
// More kinds of structure (design_paper.go): pages (text of any length),
// notes (one page of text), and cabinets that keep stacks of pages, books,
// slides, notes and drawings. Every structure has a drawer, a small cabinet
// of its own, opened and closed by the ▾ on its bottom edge. Octagons can
// hold an image of any format and have layouts, chosen by the icon on their
// top edge (design_image.go).

const designMode = 1 // its place in modes

const (
	octMinW, octMinH = 12, 7
	octNewW, octNewH = 26, 9
)

var designZooms = []float64{0.25, 1.0 / 3, 0.5, 2.0 / 3, 0.8, 1, 1.25, 1.5, 2, 3}

const designZoom100 = 5

// The highlighter pens. The rubber is pen 8.
var penColors = [8]string{"#ffd400", "#ff8a00", "#ff3b3b", "#ff4fa3", "#a259ff", "#3b82ff", "#22d3ee", "#4ade80"}
var penNames = [8]string{"yellow", "orange", "red", "pink", "purple", "blue", "cyan", "green"}

const rubber = 8

type dOct struct {
	ID     int    `json:"id"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	W      int    `json:"w"`
	H      int    `json:"h"`
	Header string `json:"header"`
	Body   string `json:"body"`
	Footer string `json:"footer"`
	Color  string `json:"color"`

	Kind       string  `json:"kind,omitempty"`       // "" an octagon, or cabinet, page, note, book, slide, drawing
	Layout     int     `json:"layout,omitempty"`     // an octagon's layout (octLayouts)
	Image      string  `json:"image,omitempty"`      // an octagon's picture: a file in design/images
	Items      []*dOct `json:"items,omitempty"`      // what its drawer (or the cabinet) holds
	DrawerOpen bool    `json:"drawerOpen,omitempty"` // its drawer is pulled out
	OpenStack  string  `json:"openStack,omitempty"`  // the stack shown open in the drawer or cabinet
	Scroll     int     `json:"scroll,omitempty"`     // a page: the first line shown
	From       string  `json:"from,omitempty"`       // where it came from (a diary page, a file)
}

type dLink struct {
	A int `json:"a"`
	B int `json:"b"`
}

type dMark struct {
	Pen  int8 `json:"pen"`
	Over bool `json:"over"`
}

type dMarkAt struct {
	X int `json:"x"`
	Y int `json:"y"`
	dMark
}

type designFile struct {
	Octs   []*dOct   `json:"structures"`
	Links  []dLink   `json:"links"`
	Marks  []dMarkAt `json:"marks"`
	CamX   float64   `json:"camX"`
	CamY   float64   `json:"camY"`
	Zoom   int       `json:"zoom"`
	NextID int       `json:"nextID"`
}

type dragKind int

const (
	dragNone dragKind = iota
	dragPan
	dragMove
	dragResize
	dragPaint
	dragRub
)

// Parts of a structure that can be written in.
const (
	partNone = iota
	partHeader
	partBody
	partFooter
)

type designState struct {
	octs   []*dOct // the last one is on top
	links  []dLink
	marks  map[[2]int]dMark
	camX   float64 // the board cell at the top-left of the screen
	camY   float64
	zoom   int // index into designZooms
	nextID int

	sel      int // the chosen structure's ID, or -1
	editing  int // the part being written in (partNone: none)
	linkMode bool
	linkFrom int  // in link mode: the structure the next click links to, or -1
	pen      int  // the pen in hand (0–7, rubber), or -1
	over     bool // the pen colours over the text instead of under it

	drag           dragKind
	pressX, pressY int // where the drag started, on the screen
	origX, origY   int
	origW, origH   int
	origCX, origCY float64
	lastPX, lastPY int
	lastClick      time.Time
	lastCX, lastCY int
	toolbar        []toolSpot // what the toolbar row has where (for clicks)
	place          string     // the kind a double-click on the empty board places
	dropOn         int        // while a page or note is dragged: the structure it would go into, or -1
	dirty          bool
	saveErr        string
	loadedFrom     string // the board's file
}

type toolSpot struct {
	x0, x1 int
	act    string
}

func designPath() string {
	return filepath.Join(filepath.Dir(sessionDir()), "design", "board.json")
}

func (m *Model) inDesign() bool { return m.mode == designMode }

// ds is the board, loaded the first time it is needed.
func (m *Model) ds() *designState {
	if m.design == nil {
		m.design = loadDesign(designPath())
	}
	return m.design
}

func loadDesign(path string) *designState {
	d := &designState{marks: map[[2]int]dMark{}, zoom: designZoom100, sel: -1, linkFrom: -1, pen: -1, nextID: 1, dropOn: -1, loadedFrom: path}
	data, err := os.ReadFile(path)
	if err != nil {
		return d
	}
	var f designFile
	if json.Unmarshal(data, &f) != nil {
		return d
	}
	for _, o := range f.Octs {
		if o != nil {
			mw, mh := minSize(o.Kind)
			o.W, o.H = max(o.W, mw), max(o.H, mh)
			if o.Color == "" {
				o.Color = randomOctColor()
			}
			d.octs = append(d.octs, o)
			d.nextID = max(d.nextID, highestID(o)+1)
		}
	}
	d.links = f.Links
	for _, mk := range f.Marks {
		if mk.Pen >= 0 && mk.Pen < 8 {
			d.marks[[2]int{mk.X, mk.Y}] = mk.dMark
		}
	}
	d.camX, d.camY = f.CamX, f.CamY
	if f.Zoom >= 0 && f.Zoom < len(designZooms) {
		d.zoom = f.Zoom
	}
	d.nextID = max(d.nextID, f.NextID)
	return d
}

// save writes the board when something changed.
func (d *designState) save() {
	if !d.dirty {
		return
	}
	d.dirty = false
	f := designFile{Octs: d.octs, Links: d.links, CamX: d.camX, CamY: d.camY, Zoom: d.zoom, NextID: d.nextID}
	for at, mk := range d.marks {
		f.Marks = append(f.Marks, dMarkAt{at[0], at[1], mk})
	}
	data, err := json.MarshalIndent(f, "", " ")
	if err != nil {
		d.saveErr = err.Error()
		return
	}
	path := d.loadedFrom
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		d.saveErr = err.Error()
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		d.saveErr = err.Error()
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		d.saveErr = err.Error()
		return
	}
	d.saveErr = ""
}

// ---- colours ---------------------------------------------------------------------

// randomOctColor is a bright colour of any hue, so no two structures need
// look the same.
func randomOctColor() string {
	return hslHex(rand.Float64()*360, 0.62+rand.Float64()*0.25, 0.58+rand.Float64()*0.1)
}

func hslHex(h, s, l float64) string {
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	mm := l - c/2
	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	to := func(v float64) int { return int(math.Round((v + mm) * 255)) }
	return fmt.Sprintf("#%02x%02x%02x", to(r), to(g), to(b))
}

// mixHex is a mixed with b: t=0 is a, t=1 is b.
func mixHex(a, b string, t float64) string {
	ar, ag, ab := rgb(a)
	br, bg, bb := rgb(b)
	f := func(x, y int) int { return int(float64(x) + (float64(y)-float64(x))*t + 0.5) }
	return fmt.Sprintf("#%02x%02x%02x", f(ar, br), f(ag, bg), f(ab, bb))
}

// ---- the board on the screen --------------------------------------------------------

func (d *designState) z() float64 { return designZooms[d.zoom] }

// rect is where a structure is on the screen (canvas cells).
func (d *designState) rect(o *dOct) (x, y, w, h int) {
	z := d.z()
	ow, oh := d.size(o)
	x = int(math.Floor((float64(o.X) - d.camX) * z))
	y = int(math.Floor((float64(o.Y) - d.camY) * z))
	x1 := int(math.Floor((float64(o.X+ow) - d.camX) * z))
	y1 := int(math.Floor((float64(o.Y+oh) - d.camY) * z))
	return x, y, max(x1-x, 1), max(y1-y, 1)
}

// at is the top structure under a canvas cell.
func (d *designState) at(sx, sy int) *dOct {
	for i := len(d.octs) - 1; i >= 0; i-- {
		x, y, w, h := d.rect(d.octs[i])
		if sx >= x && sx < x+w && sy >= y && sy < y+h {
			return d.octs[i]
		}
	}
	return nil
}

func (d *designState) byID(id int) *dOct {
	for _, o := range d.octs {
		if o.ID == id {
			return o
		}
	}
	return nil
}

func (d *designState) toFront(o *dOct) {
	for i, p := range d.octs {
		if p == o {
			d.octs = append(append(d.octs[:i:i], d.octs[i+1:]...), o)
			return
		}
	}
}

// world is the board point at a canvas cell (its middle).
func (d *designState) world(sx, sy int) (float64, float64) {
	z := d.z()
	return d.camX + (float64(sx)+0.5)/z, d.camY + (float64(sy)+0.5)/z
}

// zoomTo changes the zoom keeping the board point under (sx, sy) still.
func (d *designState) zoomTo(i, sx, sy int) {
	i = max(0, min(i, len(designZooms)-1))
	if i == d.zoom {
		return
	}
	wx, wy := d.world(sx, sy)
	d.zoom = i
	z := d.z()
	d.camX, d.camY = wx-(float64(sx)+0.5)/z, wy-(float64(sy)+0.5)/z
	d.dirty = true
}

func (d *designState) newOct(cx, cy int) *dOct {
	o := &dOct{ID: d.nextID, X: cx - octNewW/2, Y: cy - octNewH/2, W: octNewW, H: octNewH, Color: randomOctColor()}
	d.nextID++
	d.octs = append(d.octs, o)
	d.sel = o.ID
	d.dirty = true
	return o
}

func (d *designState) remove(id int) {
	for i, o := range d.octs {
		if o.ID == id {
			d.octs = append(d.octs[:i:i], d.octs[i+1:]...)
			break
		}
	}
	links := d.links[:0]
	for _, l := range d.links {
		if l.A != id && l.B != id {
			links = append(links, l)
		}
	}
	d.links = links
	if d.sel == id {
		d.sel, d.editing = -1, partNone
	}
	if d.linkFrom == id {
		d.linkFrom = -1
	}
	d.dirty = true
}

// toggleLink links a and b, or unlinks them when they already are.
// It says whether they are linked now.
func (d *designState) toggleLink(a, b int) bool {
	d.dirty = true
	for i, l := range d.links {
		if l.A == a && l.B == b || l.A == b && l.B == a {
			d.links = append(d.links[:i:i], d.links[i+1:]...)
			return false
		}
	}
	d.links = append(d.links, dLink{a, b})
	return true
}

// mark colours (or rubs out) the board cells under one canvas cell.
func (d *designState) mark(sx, sy int, rub bool) {
	z := d.z()
	x0 := int(math.Floor(d.camX + float64(sx)/z))
	x1 := max(x0, int(math.Ceil(d.camX+float64(sx+1)/z))-1)
	y0 := int(math.Floor(d.camY + float64(sy)/z))
	y1 := max(y0, int(math.Ceil(d.camY+float64(sy+1)/z))-1)
	for wy := y0; wy <= y1; wy++ {
		for wx := x0; wx <= x1; wx++ {
			if rub || d.pen == rubber {
				delete(d.marks, [2]int{wx, wy})
			} else {
				d.marks[[2]int{wx, wy}] = dMark{int8(d.pen), d.over}
			}
		}
	}
	d.dirty = true
}

// stroke marks every cell on the line from the last point to (sx, sy), so
// a quick drag leaves no gaps.
func (d *designState) stroke(sx, sy int, rub bool) {
	x0, y0 := d.lastPX, d.lastPY
	dx, dy := abs(sx-x0), -abs(sy-y0)
	stepX, stepY := 1, 1
	if sx < x0 {
		stepX = -1
	}
	if sy < y0 {
		stepY = -1
	}
	e := dx + dy
	for n := 0; n < 10000; n++ {
		d.mark(x0, y0, rub)
		if x0 == sx && y0 == sy {
			break
		}
		if e2 := 2 * e; e2 >= dy {
			e += dy
			x0 += stepX
		} else {
			e += dx
			y0 += stepY
		}
	}
	d.lastPX, d.lastPY = sx, sy
}

// ---- keys ------------------------------------------------------------------------

func (m *Model) designLinkToggle() {
	d := m.ds()
	d.linkMode = !d.linkMode
	d.linkFrom = -1
	d.editing = partNone
	if d.linkMode {
		d.pen = -1
		if o := d.byID(d.sel); o != nil {
			d.linkFrom = o.ID
		}
		m.say("⛓  Link mode: click a structure, then each one to attach to it. AltGr+L or Esc leaves.")
	} else {
		m.say("Link mode off.")
	}
}

func (m *Model) designKey(k tea.Key) tea.Cmd {
	d := m.ds()
	defer d.save()
	name := keyName(k)
	if d.editing != partNone {
		if o := d.byID(d.sel); o != nil {
			m.designWrite(d, o, k, name)
			return nil
		}
		d.editing = partNone
	}
	m.msg = ""
	cx, cy := m.width/2, m.textHeight()/2
	switch name {
	case "f1":
		m.help = true
	case "esc":
		switch {
		case d.linkMode:
			m.designLinkToggle()
		case d.pen >= 0:
			d.pen = -1
			m.say("Pen put down.")
		default:
			d.sel = -1
		}
	case "1", "2", "3", "4", "5", "6", "7", "8":
		m.designPen(int(name[0] - '1'))
	case "e":
		m.designPen(rubber)
	case "o":
		d.over = !d.over
		m.say("Pen colours " + map[bool]string{true: "over the text: it shows through, tinted.", false: "under the text: the letters stay as they are."}[d.over])
	case "+", "=", "ctrl+=", "shift+=":
		d.zoomTo(d.zoom+1, cx, cy)
	case "-", "ctrl+-":
		d.zoomTo(d.zoom-1, cx, cy)
	case "0":
		d.zoomTo(designZoom100, cx, cy)
	case "n", "p", "s", "k":
		wx, wy := d.world(cx, cy)
		kind := map[string]string{"n": kOct, "p": kPage, "s": kNote, "k": kCabinet}[name]
		d.newKind(kind, int(wx), int(wy))
		d.editing = partHeader
		m.say(newHint(kind))
	case "l", "shift+l", "L":
		if o := d.byID(d.sel); o != nil && o.Kind == kOct {
			step := 1
			if name != "l" {
				step = -1
			}
			m.designLayout(o, step)
		}
	case "d":
		if o := d.byID(d.sel); o != nil {
			m.designDrawer(o)
		}
	case "i":
		return m.designAskFile()
	case "pgup", "pgdown":
		if o := d.byID(d.sel); o != nil && o.Kind != kOct && o.Kind != kCabinet {
			o.Scroll = max(0, o.Scroll+map[string]int{"pgup": -1, "pgdown": 1}[name]*max(1, o.H-5))
			d.dirty = true
		}
	case "c":
		if o := d.byID(d.sel); o != nil {
			o.Color = randomOctColor()
			d.dirty = true
		}
	case "enter":
		if d.byID(d.sel) != nil {
			d.editing = partHeader
		}
	case "delete", "backspace":
		if o := d.byID(d.sel); o != nil {
			return m.designRemove(o)
		}
	case "left", "right", "up", "down", "shift+left", "shift+right", "shift+up", "shift+down":
		step := 1
		if strings.HasPrefix(name, "shift+") {
			step = 4
		}
		v := map[string][2]int{"left": {-1, 0}, "right": {1, 0}, "up": {0, -1}, "down": {0, 1}}[strings.TrimPrefix(name, "shift+")]
		dx, dy := v[0], v[1]
		if o := d.byID(d.sel); o != nil {
			o.X += dx * step
			o.Y += dy * step
		} else {
			z := d.z()
			d.camX += float64(dx*step*4) / z
			d.camY += float64(dy*step*2) / z
		}
		d.dirty = true
	}
	return nil
}

func (m *Model) designPen(p int) {
	d := m.ds()
	if d.pen == p {
		d.pen = -1
		m.say("Pen put down.")
		return
	}
	d.pen = p
	d.linkMode, d.linkFrom, d.editing = false, -1, partNone
	if p == rubber {
		m.say("Rubber: drag over colour to take it off. Esc puts it down.")
		return
	}
	where := "under"
	if d.over {
		where = "over"
	}
	m.say(fmt.Sprintf("%s pen, %s the text: drag to colour. O switches over/under · Esc puts it down.", strings.ToUpper(penNames[p][:1])+penNames[p][1:], where))
}

// designWrite types into the part of the structure being written in.
func (m *Model) designWrite(d *designState, o *dOct, k tea.Key, name string) {
	field := map[int]*string{partHeader: &o.Header, partBody: &o.Body, partFooter: &o.Footer}[d.editing]
	if field == nil || !hasPart(o, d.editing) {
		d.editing = partNone
		return
	}
	before := *field
	switch name {
	case "esc":
		d.editing = partNone
		return
	case "tab":
		d.editing = nextPart(o, d.editing, 1)
		return
	case "shift+tab":
		d.editing = nextPart(o, d.editing, -1)
		return
	case "enter":
		if d.editing == partBody {
			*field += "\n"
		} else {
			d.editing = nextPart(o, d.editing, 1)
			return
		}
	case "backspace":
		r := []rune(*field)
		if len(r) > 0 {
			*field = string(r[:len(r)-1])
		}
	case "ctrl+u", "ctrl+backspace", "alt+backspace":
		*field = ""
	default:
		if k.Text == "" || k.Mod.Contains(tea.ModCtrl) || k.Mod.Contains(tea.ModAlt) {
			return
		}
		*field += textLike(k.Text)
	}
	if o.Kind == kNote && d.editing == partBody && !noteFits(o.Body) {
		*field = before
		m.say("This note is full: a note holds one page (" + fmt.Sprint(noteRows) + " lines). Use a page (P) for more.")
		return
	}
	d.dirty = true
}

// ---- mouse -----------------------------------------------------------------------

// designMouseHere: does the board take this mouse event?
func (m *Model) designMouseHere(y int) bool {
	return m.inDesign() && !m.help && !m.modesOpen && !m.themeOpen && m.learn == nil && !m.tabMode &&
		m.picker == nil && !(m.showTree && m.tree != nil) && y >= textTop && y < m.height-2
}

func (m *Model) designPress(ms tea.Mouse) tea.Cmd {
	d := m.ds()
	defer d.save()
	if ms.Y == m.height-3 {
		m.designToolClick(ms.X)
		return nil
	}
	sx, sy := ms.X, ms.Y-textTop
	d.pressX, d.pressY, d.lastPX, d.lastPY = sx, sy, sx, sy
	d.origCX, d.origCY = d.camX, d.camY
	switch ms.Button {
	case tea.MouseMiddle:
		d.drag = dragPan
		return nil
	case tea.MouseRight:
		d.drag = dragRub
		d.mark(sx, sy, true)
		return nil
	case tea.MouseLeft:
	default:
		return nil
	}
	now := time.Now()
	double := now.Sub(d.lastClick) < 400*time.Millisecond && abs(sx-d.lastCX) <= 1 && sy == d.lastCY
	d.lastClick, d.lastCX, d.lastCY = now, sx, sy
	if double {
		d.lastClick = time.Time{} // a third click starts again
	}
	if d.pen >= 0 {
		d.drag = dragPaint
		d.mark(sx, sy, false)
		return nil
	}
	hit := d.hit(sx, sy)
	o := hit.o
	if d.linkMode {
		switch {
		case o == nil:
			d.linkFrom = -1
		case d.linkFrom < 0 || d.byID(d.linkFrom) == nil || d.linkFrom == o.ID:
			if d.linkFrom == o.ID {
				d.linkFrom = -1
			} else {
				d.linkFrom = o.ID
				m.say("⛓  " + octName(o) + " chosen: click the structures to attach to it.")
			}
		default:
			if d.toggleLink(d.linkFrom, o.ID) {
				m.say("⛓  Linked " + octName(d.byID(d.linkFrom)) + " → " + octName(o) + ".")
			} else {
				m.say("Unlinked " + octName(d.byID(d.linkFrom)) + " and " + octName(o) + ".")
			}
		}
		return nil
	}
	if o == nil {
		d.sel, d.editing = -1, partNone
		if double {
			wx, wy := d.world(sx, sy)
			d.newKind(d.place, int(math.Floor(wx)), int(math.Floor(wy)))
			d.editing = partHeader
			m.say(newHint(d.place))
			return nil
		}
		d.drag = dragPan // the empty board moves with the left button too
		return nil
	}
	x, y, w, h := d.rect(o)
	d.toFront(o)
	if d.sel != o.ID {
		d.editing = partNone
	}
	d.sel = o.ID
	d.origX, d.origY, d.origW, d.origH = o.X, o.Y, o.W, o.H
	switch hit.where {
	case hitHandle:
		m.designDrawer(o)
		d.drag = dragNone
		return nil
	case hitToggle:
		m.designLayout(o, 1)
		d.drag = dragNone
		return nil
	case hitRow:
		return m.designRow(o, hit.act, sx, sy)
	case hitDrawer:
		d.drag = dragNone
		return nil
	}
	if double {
		d.editing = partAt(o, sy-y, h)
		d.drag = dragNone
		return nil
	}
	if resizable(o) && sx >= x+w-2 && sy >= y+h-2 {
		d.drag = dragResize
	} else {
		d.drag = dragMove
	}
	d.dirty = true // brought to the front
	return nil
}

// octPartAt is the part of a structure h rows high at row r.
func octPartAt(r, h int) int {
	switch {
	case r <= 1:
		return partHeader
	case h >= octMinH && r >= h-2:
		return partFooter
	}
	return partBody
}

func octName(o *dOct) string {
	if o == nil {
		return "?"
	}
	if s := strings.TrimSpace(o.Header); s != "" {
		return "“" + s + "”"
	}
	return fmt.Sprintf("structure %d", o.ID)
}

func (m *Model) designMotion(ms tea.Mouse) tea.Cmd {
	d := m.ds()
	sx, sy := ms.X, ms.Y-textTop
	z := d.z()
	dx, dy := float64(sx-d.pressX)/z, float64(sy-d.pressY)/z
	switch d.drag {
	case dragPan:
		d.camX, d.camY = d.origCX-dx, d.origCY-dy
	case dragMove:
		if o := d.byID(d.sel); o != nil {
			o.X, o.Y = d.origX+int(math.Round(dx)), d.origY+int(math.Round(dy))
			d.dropOn = -1
			if textual(o.Kind) {
				if t := d.dropTarget(o, sx, sy); t != nil {
					d.dropOn = t.ID
				}
			}
		}
	case dragResize:
		if o := d.byID(d.sel); o != nil {
			mw, mh := minSize(o.Kind)
			o.W, o.H = max(mw, d.origW+int(math.Round(dx))), max(mh, d.origH+int(math.Round(dy)))
		}
	case dragPaint:
		d.stroke(sx, sy, false)
	case dragRub:
		d.stroke(sx, sy, true)
	default:
		return nil
	}
	d.dirty = true
	return nil
}

func (m *Model) designRelease() tea.Cmd {
	d := m.ds()
	if d.drag == dragMove && d.dropOn >= 0 {
		m.designFileInto(d.byID(d.sel), d.byID(d.dropOn))
	}
	d.drag, d.dropOn = dragNone, -1
	d.save()
	return nil
}

func (m *Model) designWheel(ms tea.Mouse) tea.Cmd {
	d := m.ds()
	sx, sy := ms.X, ms.Y-textTop
	z := d.z()
	if o := d.hit(sx, sy).o; o != nil && o.ID == d.sel && o.Kind != kOct && o.Kind != kCabinet &&
		(ms.Button == tea.MouseWheelUp || ms.Button == tea.MouseWheelDown) {
		// The wheel over the chosen page scrolls its text.
		if ms.Button == tea.MouseWheelUp {
			o.Scroll = max(0, o.Scroll-3)
		} else {
			o.Scroll += 3
		}
		d.dirty = true
		d.save()
		return nil
	}
	switch ms.Button {
	case tea.MouseWheelUp:
		d.zoomTo(d.zoom+1, sx, sy)
	case tea.MouseWheelDown:
		d.zoomTo(d.zoom-1, sx, sy)
	case tea.MouseWheelLeft:
		d.camX -= 4 / z
	case tea.MouseWheelRight:
		d.camX += 4 / z
	}
	d.dirty = true
	d.save()
	return nil
}

func (m *Model) designToolClick(x int) {
	d := m.ds()
	for _, t := range d.toolbar {
		if x < t.x0 || x >= t.x1 {
			continue
		}
		switch {
		case strings.HasPrefix(t.act, "pen"):
			m.designPen(int(t.act[3] - '0'))
		case t.act == "over":
			d.over = !d.over
		case t.act == "link":
			m.designLinkToggle()
		case t.act == "zoomin":
			d.zoomTo(d.zoom+1, m.width/2, m.textHeight()/2)
		case t.act == "zoomout":
			d.zoomTo(d.zoom-1, m.width/2, m.textHeight()/2)
		case strings.HasPrefix(t.act, "place:"):
			d.place = strings.TrimPrefix(t.act, "place:")
			m.say("Double-click the empty board to place a " + kindNames[d.place] + ".")
		}
		return
	}
}

// ---- drawing (Lip Gloss) -----------------------------------------------------------
//
// The board is drawn with Lip Gloss: a canvas, one layer for the board (its
// dots and the links), and layers for each structure, made of Lip Gloss
// styles and borders. The pen then tints the canvas cells.

func st(fg, bg string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(hex(fg)).Background(hex(bg))
}

// octMiddle is the border of a structure's middle: its sides, with the
// dotted lines that part the text from the header and the footer.
var octMiddle = lipgloss.Border{
	Left: "│", Right: "│", Top: "┄", Bottom: "┄",
	TopLeft: "├", TopRight: "┤", BottomLeft: "├", BottomRight: "┤",
}

// octSides is the border of a small structure: only its sides.
var octSides = lipgloss.Border{Left: "│", Right: "│"}

// designCanvas is the whole board, drawn.
func (m *Model) designCanvas() *lipgloss.Canvas {
	d := m.ds()
	c := lipgloss.NewCanvas(m.width, m.textHeight())
	c.Compose(lipgloss.NewLayer(d.boardLayer(m.width, m.textHeight())))
	var layers []*lipgloss.Layer
	for i, o := range d.octs {
		for _, l := range d.structLayers(o) {
			layers = append(layers, l.Z(i+1))
		}
	}
	if len(layers) > 0 {
		c.Compose(lipgloss.NewCompositor(layers...))
	}
	d.tint(c)
	return c
}

// designRows draws the board in the text area.
func (m *Model) designRows(sb *strings.Builder) {
	rows := strings.Split(m.designCanvas().Render(), "\n")
	for y := range m.textHeight() {
		if y < len(rows) {
			sb.WriteString(rows[y])
		}
		sb.WriteByte('\n')
	}
}

// The four ways a line can leave a cell, mixed into one box letter.
const (
	goUp = 1 << iota
	goDown
	goLeft
	goRight
)

var boxLetters = map[int]string{
	goLeft | goRight: "─", goUp | goDown: "│", goLeft: "─", goRight: "─", goUp: "│", goDown: "│",
	goDown | goRight: "╭", goDown | goLeft: "╮", goUp | goRight: "╰", goUp | goLeft: "╯",
	goUp | goDown | goRight: "├", goUp | goDown | goLeft: "┤", goLeft | goRight | goDown: "┬",
	goLeft | goRight | goUp: "┴", goUp | goDown | goLeft | goRight: "┼",
}

// boardLayer is the empty board: a dot where its grid lines cross (they
// spread apart as it zooms in), a + at its middle, and the links.
func (d *designState) boardLayer(w, h int) string {
	type mark struct {
		s, fg string
	}
	at := map[[2]int]mark{}
	z := d.z()
	stepX, stepY := 8, 4
	for float64(stepX)*z < 6 {
		stepX, stepY = stepX*2, stepY*2
	}
	crosses := func(s int, cam float64, step int) (bool, int) {
		a := math.Floor((cam + float64(s)/z) / float64(step))
		b := math.Floor((cam + float64(s+1)/z) / float64(step))
		return b > a, int(b) * step
	}
	dot, centre := mixHex(cDim, cBG, 0.45), mixHex(cDim, cBG, 0.1)
	var xs [][2]int
	for x := range w {
		if ok, wx := crosses(x, d.camX, stepX); ok {
			xs = append(xs, [2]int{x, wx})
		}
	}
	for y := range h {
		ok, wy := crosses(y, d.camY, stepY)
		if !ok {
			continue
		}
		for _, x := range xs {
			if x[1] == 0 && wy == 0 {
				at[[2]int{x[0], y}] = mark{"+", centre}
			} else {
				at[[2]int{x[0], y}] = mark{"·", dot}
			}
		}
	}
	for cell, l := range d.linkCells(w, h) {
		at[cell] = l
	}
	bg := st(cText, cBG)
	var sb strings.Builder
	for y := range h {
		run := 0
		for x := range w {
			mk, ok := at[[2]int{x, y}]
			if !ok {
				run++
				continue
			}
			if run > 0 {
				sb.WriteString(bg.Render(strings.Repeat(" ", run)))
				run = 0
			}
			sb.WriteString(st(mk.fg, cBG).Render(mk.s))
		}
		if run > 0 {
			sb.WriteString(bg.Render(strings.Repeat(" ", run)))
		}
		if y < h-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// linkCells works out the links' lines: each goes with right-angle bends
// from the middle of one structure to the middle of the other. The
// structures are drawn over the ends, so a line seems to leave from the edge.
func (d *designState) linkCells(w, h int) map[[2]int]struct{ s, fg string } {
	ways := map[[2]int]int{}
	colour := map[[2]int]string{}
	add := func(x, y, way int, c string) {
		if x < 0 || y < 0 || x >= w || y >= h {
			return
		}
		ways[[2]int{x, y}] |= way
		colour[[2]int{x, y}] = c
	}
	hline := func(x0, x1, y int, c string) {
		if y < 0 || y >= h {
			return
		}
		if x0 > x1 {
			x0, x1 = x1, x0
		}
		for x := max(x0, -1); x < min(x1, w); x++ {
			add(x, y, goRight, c)
			add(x+1, y, goLeft, c)
		}
	}
	vline := func(x, y0, y1 int, c string) {
		if x < 0 || x >= w {
			return
		}
		if y0 > y1 {
			y0, y1 = y1, y0
		}
		for y := max(y0, -1); y < min(y1, h); y++ {
			add(x, y, goDown, c)
			add(x, y+1, goUp, c)
		}
	}
	for _, l := range d.links {
		a, b := d.byID(l.A), d.byID(l.B)
		if a == nil || b == nil {
			continue
		}
		c := mixHex(mixHex(a.Color, b.Color, 0.5), cBG, 0.2)
		ax, ay, aw, ah := d.rect(a)
		bx, by, bw, bh := d.rect(b)
		x0, y0, x1, y1 := ax+aw/2, ay+ah/2, bx+bw/2, by+bh/2
		if abs(x1-x0) >= 2*abs(y1-y0) { // side by side: across, down, across
			mx := (x0 + x1) / 2
			hline(x0, mx, y0, c)
			vline(mx, y0, y1, c)
			hline(mx, x1, y1, c)
		} else { // one above the other: down, across, down
			my := (y0 + y1) / 2
			vline(x0, y0, my, c)
			hline(x0, x1, my, c)
			vline(x1, my, y1, c)
		}
	}
	out := map[[2]int]struct{ s, fg string }{}
	for at, way := range ways {
		if s, ok := boxLetters[way]; ok {
			out[at] = struct{ s, fg string }{s, colour[at]}
		}
	}
	return out
}

// octLayers draws one structure as Lip Gloss layers: an octagon with its
// corners cut, the header at the top, the text in the middle and the footer
// at the bottom. The cut corners are separate, narrower layers, so the
// board shows through beside them.
func (d *designState) octLayers(o *dOct) []*lipgloss.Layer {
	x, y, w, h := d.rect(o)
	selected := d.sel == o.ID
	source := d.linkMode && d.linkFrom == o.ID
	editing := 0
	if selected {
		editing = d.editing
	}
	edge, fill := o.Color, mixHex(cBG, o.Color, 0.16)
	if source {
		fill = mixHex(cBG, o.Color, 0.32)
	}
	if selected {
		edge = mixHex(o.Color, cText, 0.35)
	}
	headFG := mixHex(o.Color, cText, 0.35)
	dim := mixHex(cDim, fill, 0.25)

	// The parts' words, with a placeholder when one is empty and the caret
	// where the typing goes.
	part := func(which int, s, placeholder string, style lipgloss.Style, room int) string {
		caret := st(cAccent, fill).Render("▏")
		switch {
		case editing == which:
			s = ansi.TruncateLeft(s, max(0, lipgloss.Width(s)-(room-1)), "…")
			return style.Render(s) + caret
		case s == "":
			return st(dim, fill).Italic(true).Render(placeholder)
		}
		return style.Render(ansi.Truncate(s, room, "…"))
	}

	if h < 3 || w < 6 { // too small for the shape: one coloured row
		pill := mixHex(cBG, o.Color, 0.55)
		return []*lipgloss.Layer{lipgloss.NewLayer(st(cBG, pill).Bold(true).Width(w).MaxWidth(w).
			Render(ansi.Truncate("◆"+o.Header, w, ""))).X(x).Y(y)}
	}
	diag := func(s string) string { return st(edge, cBG).Bold(selected).Render(s) }
	line := func(n int) string { return st(edge, fill).Render(strings.Repeat("─", max(n, 0))) }
	centred := func(s string, n int) string {
		return lipgloss.PlaceHorizontal(n, lipgloss.Center, s, lipgloss.WithWhitespaceStyle(st(cText, fill)))
	}
	head := part(partHeader, o.Header, "header", st(headFG, fill).Bold(true), w-6)
	var layers []*lipgloss.Layer
	add := func(s string, lx, ly int) { layers = append(layers, lipgloss.NewLayer(s).X(lx).Y(ly)) }

	if h < octMinH || w < octMinW { // small: corners cut by one
		add(diag("╱")+line(w-2)+diag("╲"), x, y)
		inner := head + "\n" + d.octContent(o, editing, w-4, h-3, fill, dim)
		add(lipgloss.NewStyle().Border(octSides, false, true, false, true).BorderForeground(hex(edge)).BorderBackground(hex(fill)).
			Background(hex(fill)).Foreground(hex(cText)).Padding(0, 1).Width(w).Height(h-2).
			Render(centredFirst(inner, w-4, fill)), x, y+1)
		add(diag("╲")+line(w-2)+diag("╱"), x, y+h-1)
		return d.octExtras(layers, o, x, y, w, h, edge, fill, selected, source)
	}

	foot := part(partFooter, o.Footer, "footer", st(mixHex(cText, fill, 0.3), fill).Italic(true), w-6)
	add(diag("╱")+line(w-6)+diag("╲"), x+2, y)
	add(diag("╱")+centred(head, w-4)+diag("╲"), x+1, y+1)
	add(lipgloss.NewStyle().Border(octMiddle).BorderForeground(hex(edge)).BorderBackground(hex(fill)).
		Background(hex(fill)).Foreground(hex(cText)).Padding(0, 1).Width(w).Height(h-4).
		Render(d.octContent(o, editing, w-4, h-6, fill, dim)), x, y+2)
	add(diag("╲")+centred(foot, w-4)+diag("╱"), x+1, y+h-2)
	add(diag("╲")+line(w-6)+diag("╱"), x+2, y+h-1)
	if tx, ty, ok := layoutToggleSpot(x, y, w, h); ok {
		// The layout toggle: its icon is the layout in use; a click shifts it to the next.
		tog := st(edge, fill).Render("[") + st(cAccent, fill).Bold(true).Render(octLayouts[o.Layout%len(octLayouts)].icon) + st(edge, fill).Render("]")
		add(tog, tx, ty)
	}
	return d.octExtras(layers, o, x, y, w, h, edge, fill, selected, source)
}

// octBody is the structure's text, wrapped to its width; what does not fit
// ends in "…" (while typing, the end shows instead).
func (d *designState) octBody(o *dOct, editing, w, rows int, fill, dim string) string {
	if rows <= 0 || w <= 0 {
		return ""
	}
	if o.Body == "" && editing != partBody {
		return st(dim, fill).Italic(true).Render(ansi.Truncate("double-click to write", w, "…"))
	}
	lines := strings.Split(ansi.Wrap(o.Body, w, ""), "\n")
	switch {
	case editing == partBody:
		if last := lines[len(lines)-1]; lipgloss.Width(last) >= w {
			lines = append(lines, "")
		}
		if len(lines) > rows {
			lines = lines[len(lines)-rows:] // keep the end, where the typing is
		}
	case len(lines) > rows:
		lines = lines[:rows]
		lines[rows-1] = ansi.Truncate(lines[rows-1], w-1, "") + "…"
	}
	text := st(cText, fill)
	for i, l := range lines {
		lines[i] = text.Render(l)
	}
	if editing == partBody {
		lines[len(lines)-1] += st(cAccent, fill).Render("▏")
	}
	return strings.Join(lines, "\n")
}

// centredFirst centres the first line (the header of a small structure).
func centredFirst(s string, w int, fill string) string {
	first, rest, _ := strings.Cut(s, "\n")
	return lipgloss.PlaceHorizontal(w, lipgloss.Center, first, lipgloss.WithWhitespaceStyle(st(cText, fill))) + "\n" + rest
}

// octExtras adds the ⛓ of the structure links start from and the ◢ that
// resizes the chosen one.
func (d *designState) octExtras(layers []*lipgloss.Layer, o *dOct, x, y, w, h int, edge, fill string, selected, source bool) []*lipgloss.Layer {
	if source && w > 8 {
		layers = append(layers, lipgloss.NewLayer(st(cAccent, fill).Bold(true).Render(" ⛓ ")).X(x+3).Y(y))
	}
	if selected && !d.linkMode {
		layers = append(layers, lipgloss.NewLayer(st(edge, cBG).Render("◢")).X(x+w-1).Y(y+h-1))
	}
	return layers
}

// tint lays the pen's colour on the drawn canvas. Under the text: the
// cell's background takes the colour and the letter stays exactly as it
// was. Over the text: the colour is laid over everything; letters show
// through, tinted, and empty cells show the ink's grain.
func (d *designState) tint(c *lipgloss.Canvas) {
	if len(d.marks) == 0 {
		return
	}
	z := d.z()
	for y := range c.Height() {
		wy := int(math.Floor(d.camY + (float64(y)+0.5)/z))
		for x := range c.Width() {
			mk, ok := d.marks[[2]int{int(math.Floor(d.camX + (float64(x)+0.5)/z)), wy}]
			if !ok {
				continue
			}
			cell := c.CellAt(x, y)
			if cell == nil || cell.Width == 0 {
				continue
			}
			pen := penColors[mk.Pen]
			bg, fg := colorHex(cell.Style.Bg, cBG), colorHex(cell.Style.Fg, cText)
			if !mk.Over {
				cell.Style.Bg = hex(mixHex(bg, pen, 0.5))
				continue
			}
			nbg := mixHex(bg, pen, 0.38)
			cell.Style.Bg = hex(nbg)
			if strings.TrimSpace(cell.Content) == "" {
				cell.Content, cell.Width = "░", 1
				cell.Style.Fg = hex(mixHex(nbg, pen, 0.55))
			} else {
				cell.Style.Fg = hex(mixHex(fg, pen, 0.45))
			}
		}
	}
}

// colorHex is c as #rrggbb, or or when there is none.
func colorHex(c color.Color, or string) string {
	if c == nil {
		return or
	}
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
}

// designToolbar is the row under the board: the pens, over/under, link
// mode and the zoom. Each can be clicked.
func (m *Model) designToolbar() string {
	d := m.ds()
	d.toolbar = d.toolbar[:0]
	var sb strings.Builder
	x := 0
	put := func(s string, style lipgloss.Style, act string) {
		out := style.Render(s)
		w := lipgloss.Width(out)
		if act != "" {
			d.toolbar = append(d.toolbar, toolSpot{x, x + w, act})
		}
		sb.WriteString(out)
		x += w
	}
	bar := st(cDim, cBarBG)
	put(" "+modes[designMode].Icon()+" ", st(cEmber, cBarBG).Bold(true), "")
	put("pen", bar, "")
	for i, c := range penColors {
		label := fmt.Sprintf(" %d ", i+1)
		if d.pen == i {
			label = fmt.Sprintf("[%d]", i+1)
		}
		put(label, st("#141010", mixHex(cBarBG, c, 0.85)).Bold(d.pen == i), fmt.Sprintf("pen%d", i))
	}
	rub := " ⌫ "
	if d.pen == rubber {
		rub = "[⌫]"
	}
	put(rub, st(cStatusText, mixHex(cBarBG, cText, 0.25)).Bold(d.pen == rubber), "pen8")
	put(" ", bar, "")
	if d.over {
		put("░ over ", st(cAccent, cBarBG).Bold(true), "over")
	} else {
		put("▁ under", st(cAccent, cBarBG).Bold(true), "over")
	}
	put(" ", bar, "")
	if d.linkMode {
		put(" ⛓ LINK MODE ", st(cBG, cAccent).Bold(true), "link")
	} else {
		put("⛓ link", bar, "link")
	}
	put(" new ", bar, "")
	for _, k := range []string{kOct, kPage, kNote, kCabinet} {
		if d.place == k {
			put("["+kindIcons[k]+"]", st(cBG, cAccent).Bold(true), "place:"+k)
		} else {
			put(" "+kindIcons[k]+" ", st(cStatusText, cBarBG), "place:"+k)
		}
	}
	put(" ", bar, "")
	put(" − ", st(cStatusText, cBarBG).Bold(true), "zoomout")
	put(fmt.Sprintf("%3.0f%%", d.z()*100), st(cStatusText, cBarBG), "")
	put(" + ", st(cStatusText, cBarBG).Bold(true), "zoomin")
	right := bar.Render(fmt.Sprintf("%d structures · %d links ", len(d.octs), len(d.links)))
	if gap := m.width - x - lipgloss.Width(right); gap > 0 {
		sb.WriteString(bar.Render(strings.Repeat(" ", gap)) + right)
	} else {
		return fit(sb.String(), m.width, bar)
	}
	return sb.String()
}

// designStatus is the status bar: what the mouse and keys do just now.
func (m *Model) designStatus() string {
	d := m.ds()
	var hint string
	switch {
	case d.saveErr != "":
		hint = "✗ board not saved: " + d.saveErr
	case d.drag == dragMove && d.dropOn >= 0:
		hint = "Let go to put it in " + octName(d.byID(d.dropOn)) + (map[bool]string{true: "", false: "'s drawer"}[d.byID(d.dropOn) != nil && d.byID(d.dropOn).Kind == kCabinet])
	case d.editing != partNone:
		part := map[int]string{partHeader: "header", partBody: "text", partFooter: "footer"}[d.editing]
		if o := d.byID(d.sel); o != nil && o.Kind != kOct {
			part = map[int]string{partHeader: "title", partBody: "text"}[d.editing]
		}
		hint = "Writing the " + part + " · Tab next part · Enter " + map[bool]string{true: "new line", false: "next part"}[d.editing == partBody] + " · Esc done"
	case d.linkMode && d.linkFrom >= 0:
		hint = "Link mode · click structures to attach (click again to detach) · empty board: choose another · AltGr+L leaves"
	case d.linkMode:
		hint = "Link mode · click the structure to link from · AltGr+L or Esc leaves"
	case d.pen == rubber:
		hint = "Rubber · drag to take colour off · Esc puts it down"
	case d.pen >= 0:
		hint = "Pen " + penNames[d.pen] + " · drag to colour · right-drag rubs out · O over/under · Esc puts it down"
	case d.sel >= 0 && d.byID(d.sel) != nil:
		hint = selHint(d.byID(d.sel))
	default:
		hint = "Double-click: new " + kindNames[d.place] + " · P page · S note · K cabinet · I image · wheel zooms · 1–8 pen · AltGr+L link · F1 keys"
	}
	style := stStatus
	if d.saveErr != "" {
		style = st(cError, cStatusBG)
	}
	return fit(style.Render(" "+hint), m.width, stStatus)
}
