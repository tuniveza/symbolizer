package main

import (
	"encoding/json"
	"fmt"
	"image/color"
	"math"
	"math/rand/v2"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
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
// Keys (only these): arrows move the view (and choose), Alt + arrows move the
// chosen structure (design_keys.go), + − 0 zoom, 1–8 pen, Tab writes in the
// chosen structure's parts, Alt+N then P/N/C/S places a page, note, cabinet
// or shape, Alt+L changes a shape's layout, X twice deletes, Ctrl+Z/Ctrl+Y
// undo and redo. The rest is on the toolbar and the mouse.
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

	// The body wrapped at wrapW columns, kept so a long page is not wrapped
	// again on every frame while it scrolls. wrapParas keeps each paragraph's
	// wrap, so typing re-wraps only the paragraph that changed.
	wrapBody  string
	wrapW     int
	wrapLines []string
	wrapParas map[string][]string
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

	// The ink as a map, and its version (designState.marksVer): a save
	// whose ink has not changed leaves marks.json alone. Not in the JSON.
	marksMap map[[2]int]dMark
	marksVer int
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
	drawn map[int][]drawnStruct // each structure's last drawings, newest first, by ID (cachedLayers)
	// Pictures met while drawing a structure (pictureArea), and the layers
	// standing for them (octLayers), whose cells come from pictureCanvas.
	picSpots    []picSpot
	picLayers   map[*lipgloss.Layer]picSpot
	picsAsText  bool                             // draw pictures as styled text for now (octLayers)
	selLink     int                              // the chosen link, by index in links, or -1
	undo, redo  []*boardState                    // the board before each step, and after each undone one (design_undo.go)
	undoBase    *boardState                      // the board after the last step
	undoWriting int                              // the structure being written in during the last step, or -1
	picBox      map[int][4]int                   // where each structure's picture is, from its corner: x, y, w, h
	holesLeft   bool                             // pictureAlone left empty lines this time
	emptyHoles  bool                             // pictures alone on their rows leave empty lines (pictureAlone)
	preMade     map[*lipgloss.Layer]*boardCanvas // layers whose cells are made already (preCells)

	// The last drawing of the board (designRows), and whether a tick to draw
	// it again is on its way (designFrameCmd).
	frame          string
	frameAt        time.Time
	frameW, frameH int
	frameTick      bool
	frameCost      time.Duration // how long the last drawing took
	canvas         *boardCanvas  // the board drawn (designCanvas)
	octs           []*dOct       // the last one is on top
	links          []dLink
	marks          map[[2]int]dMark
	camX           float64 // the board cell at the top-left of the screen
	camY           float64
	zoom           int // index into designZooms
	nextID         int

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
	savePending    bool        // a change waits to be written (designSaveLater)
	saveTick       bool        // the wait for it has started (designSaveCmd)
	snapGen        int         // snapshots taken
	lastSnap       *designFile // the newest of them, for flushSave
	written        int         // the newest snapshot on disk (under designDisk)
	disk           diskState   // what is on disk, part by part (under designDisk; design_store.go)
	drop           dropState   // a drag over the board, or a drop being read (design_dnd.go)
	idIndex        map[int]int // where each structure is in octs, by ID (byID)
	marksVer       int         // goes up each time the ink changes
	lastArrow      string      // the last arrow pressed, for two at once (arrowDiagonal)
	holdTick       bool        // a glide is going (designHold)
	holdMods       string      // the keys held with the arrows: "", "shift+", "alt+", "alt+shift+"
	holding        bool        // a structure is gliding: one undo step when it stops
	heldAt         time.Time
	lastArrowAt    time.Time
	placing        bool         // Alt+N was pressed: the next key says what to place
	xAt            time.Time    // when X was pressed: X again soon deletes
	snapAt         time.Time    // when Alt + arrows last snapped: a held glide stays a moment (design_snap.go)
	jumpAt         time.Time    // when an arrow last jumped the view to a structure (design_keys.go)
	maxH           int          // board rows no structure is taller than: the screen at 100% (0: not known yet)
	inked          map[int]bool // the board rows that have ink (tint), for inkedVer
	inkedVer       int
	inkedLen       int
	idFor          **dOct // the octs it was made for: its first element and length
	idLen          int
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
	if m.height > 0 {
		// No structure is taller than the screen shows at 100%, less a row
		// above and below: the whole of it (a picture too) fits on screen.
		m.design.maxH = max(octMinH, m.boardH()-2)
	}
	return m.design
}

// startUndo makes the board as loaded the place undo goes back to.
func (d *designState) startUndo() *designState {
	d.undoBase = d.state()
	return d
}

func loadDesign(path string) *designState {
	d := &designState{marks: map[[2]int]dMark{}, zoom: designZoom100, sel: -1, linkFrom: -1, pen: -1, nextID: 1, dropOn: -1, selLink: -1, undoWriting: -1, loadedFrom: path}
	f, inParts, err := readDesignFile(path)
	if err != nil && f.Octs == nil && f.Links == nil {
		return d.startUndo()
	}
	if err != nil {
		d.saveErr = err.Error() // some parts could not be read: the rest is shown
	}
	if inParts {
		index, _ := json.MarshalIndent(indexOf(f), "", " ")
		d.disk.remember(f, index)
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
	return d.startUndo()
}

// designSaveLater: a change reaches the disk a moment later, written from
// another goroutine (on in the real program, see main.go; tests save at
// once and look at the file straight away). A large project is megabytes
// of text, and writing it all at every key made typing stutter.
var designSaveLater = false

// designSaveDelay is how long after a change the board is written.
const designSaveDelay = 400 * time.Millisecond

// designSaveMsg: time to write the board. designSavedMsg: it was written.
type (
	designSaveMsg  struct{}
	designSavedMsg struct{ err string }
)

// designDisk keeps writes in order: a slower, older one never lands on top
// of a newer one.
var designDisk sync.Mutex // guards writing and designState.written

// save writes the board when something changed (or, with designSaveLater,
// notes that it must be written soon: see designSaveCmd).
func (d *designState) save() {
	if !d.dirty {
		return
	}
	d.recordUndo()
	d.dirty = false
	if designSaveLater {
		d.savePending = true
		return
	}
	gen, f := d.snapshot()
	d.saveErr = d.writeSnapshot(f, gen)
}

// snapshot copies what is saved, so it can be written while the board
// changes. The words are shared (Go strings never change); the structures
// are copied.
func (d *designState) snapshot() (int, designFile) {
	// The copy undo keeps (state) shares what has not changed since the
	// last one, so taking it costs little even on a large board.
	st := d.state()
	f := designFile{Octs: st.octs, Links: st.links, CamX: d.camX, CamY: d.camY, Zoom: d.zoom, NextID: d.nextID,
		marksMap: st.marks, marksVer: st.marksVer}
	d.snapGen++
	d.lastSnap = &f
	return d.snapGen, f
}

func copyOcts(os []*dOct) []*dOct {
	if os == nil {
		return nil
	}
	out := make([]*dOct, len(os))
	for i, o := range os {
		if o == nil {
			continue
		}
		c := dOct{X: o.X, Y: o.Y, ID: o.ID, W: o.W, H: o.H, Header: o.Header, Body: o.Body, Footer: o.Footer, Color: o.Color,
			Kind: o.Kind, Layout: o.Layout, Image: o.Image, DrawerOpen: o.DrawerOpen, OpenStack: o.OpenStack, Scroll: o.Scroll, From: o.From}
		c.Items = copyOcts(o.Items)
		out[i] = &c
	}
	return out
}

// writeSnapshot writes snapshot gen unless a newer one is on disk already.
// It returns the error, or "".
func (d *designState) writeSnapshot(f designFile, gen int) string {
	designDisk.Lock()
	defer designDisk.Unlock()
	if gen <= d.written {
		return ""
	}
	if err := d.writeParts(f); err != "" {
		return err // d.written stays: the next save tries again
	}
	d.written = gen
	return ""
}

// designSaveCmd starts the wait before a pending save.
func (m *Model) designSaveCmd() tea.Cmd {
	d := m.design
	if d == nil || !d.savePending || d.saveTick {
		return nil
	}
	d.saveTick = true
	return tea.Tick(designSaveDelay, func(time.Time) tea.Msg { return designSaveMsg{} })
}

// designSaveNow takes a snapshot and writes it from another goroutine.
func (m *Model) designSaveNow() tea.Cmd {
	d := m.design
	if d == nil {
		return nil
	}
	d.saveTick = false
	if !d.savePending {
		return nil
	}
	d.savePending = false
	gen, f := d.snapshot()
	return func() tea.Msg { return designSavedMsg{d.writeSnapshot(f, gen)} }
}

// flushSave writes anything not yet on disk, and waits for it: for quitting.
func (d *designState) flushSave() {
	if d.savePending {
		d.savePending = false
		d.snapshot()
	}
	if d.lastSnap != nil {
		d.saveErr = d.writeSnapshot(*d.lastSnap, d.snapGen)
	}
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
// mixHex is drawn with hundreds of times a frame, and reading the hex is
// slow, so each mix is worked out once and kept.
var (
	mixed  sync.Map     // mixKey → string
	mixedN atomic.Int64 // how many are kept: pen over a picture makes new ones
)

type mixKey struct {
	a, b string
	t    float64
}

func mixHex(a, b string, t float64) string {
	k := mixKey{a, b, t}
	if s, ok := mixed.Load(k); ok {
		return s.(string)
	}
	s := mixHexNow(a, b, t)
	if mixedN.Add(1) > 50000 {
		mixed.Clear()
		mixedN.Store(0)
	}
	mixed.Store(k, s)
	return s
}

func mixHexNow(a, b string, t float64) string {
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
	// The size comes from the zoom alone, not from where the edges fall,
	// so it stays the same while the board pans.
	return x, y, max(int(math.Round(float64(ow)*z)), 1), max(int(math.Round(float64(oh)*z)), 1)
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
	if id < 0 || len(d.octs) == 0 {
		return nil
	}
	// The index says where each ID was; it is checked against the board
	// itself, so it is never wrong, only sometimes out of date, and then
	// made again (once for each change to the board's list).
	i, ok := d.idIndex[id]
	if ok && i < len(d.octs) && d.octs[i].ID == id {
		return d.octs[i]
	}
	if !ok && d.idFor == &d.octs[0] && d.idLen == len(d.octs) {
		// Not in an index made for this very list: most likely there is no
		// such structure, but a list changed in place could still hold it.
		for _, o := range d.octs {
			if o.ID == id {
				return o
			}
		}
		return nil
	}
	d.idIndex = make(map[int]int, len(d.octs))
	for i, o := range d.octs {
		d.idIndex[o.ID] = i
	}
	d.idFor, d.idLen = &d.octs[0], len(d.octs)
	if i, ok := d.idIndex[id]; ok {
		return d.octs[i]
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
	d.selLink = -1
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
	d.dirty, d.selLink = true, -1
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
	d.marksVer++ // the ink changed (undo and saving look at this)
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
		m.say("⛓  Link mode: click a structure, then each one to attach to it. Esc leaves.")
	} else {
		m.say("Link mode off.")
	}
}

// xTwice is how quickly X must come again to delete.
const xTwice = 600 * time.Millisecond

// placeKeys are the keys after Alt+N, and what each places.
var placeKeys = map[string]string{
	"p": kPage, "n": kNote, "c": kCabinet, "s": kOct,
	"P": kPage, "N": kNote, "C": kCabinet, "S": kOct,
	"shift+p": kPage, "shift+n": kNote, "shift+c": kCabinet, "shift+s": kOct,
}

func (m *Model) designKey(k tea.Key) (cmd tea.Cmd) {
	d := m.ds()
	defer d.save()
	name := keyName(k)
	switch name {
	case "ctrl+z":
		m.designUndo()
		return nil
	case "ctrl+y", "ctrl+shift+z", "ctrl+Z":
		m.designRedo()
		return nil
	}
	if d.editing != partNone {
		if o := d.byID(d.sel); o != nil {
			m.designWrite(d, o, k, name)
			return nil
		}
		d.editing = partNone
	}
	m.msg = ""
	cx, cy := m.width/2, m.boardH()/2
	if d.placing { // Alt+N was pressed: this key says what to place
		d.placing = false
		if kind, ok := placeKeys[name]; ok {
			wx, wy := d.world(cx, cy)
			d.newKind(kind, int(wx), int(wy))
			d.editing = partHeader
			m.say(newHint(kind))
		} else {
			m.say("Nothing placed. Alt+N, then P page · N note · C cabinet · S shape.")
		}
		return nil
	}
	if name == "X" || name == "shift+x" {
		name = "x"
	}
	if name != "x" {
		d.xAt = time.Time{}
	}
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
	case "+", "=", "ctrl+=", "shift+=", "plus", "kp+":
		d.zoomTo(d.zoom+1, cx, cy)
	case "-", "ctrl+-", "minus", "kp-":
		d.zoomTo(d.zoom-1, cx, cy)
	case "0":
		d.zoomTo(designZoom100, cx, cy)
	case "alt+n":
		d.placing = true
		m.say("Place: P page · N note · C cabinet · S shape (any other key: nothing).")
	case "alt+l", "alt+shift+l", "alt+L":
		if o := d.byID(d.sel); o != nil && o.Kind == kOct {
			step := 1
			if name != "alt+l" {
				step = -1
			}
			m.designLayout(o, step)
		} else {
			m.say("Alt+L changes the layout of a chosen shape.")
		}
	case "x":
		// X twice, quickly, deletes what is chosen.
		o := d.byID(d.sel)
		hasLink := d.selLink >= 0 && d.selLink < len(d.links)
		if o == nil && !hasLink {
			m.say("Nothing chosen to delete.")
			return nil
		}
		if d.xAt.IsZero() || time.Since(d.xAt) > xTwice {
			d.xAt = time.Now()
			m.say("Press X again to delete it.")
			return nil
		}
		d.xAt = time.Time{}
		if hasLink {
			m.designDeleteLink(d.selLink)
		} else {
			return m.designRemove(o)
		}

	case "left", "right", "up", "down", "shift+left", "shift+right", "shift+up", "shift+down":
		// The arrows move the view; the structure nearest the middle of
		// the screen is chosen as it goes by. Held, it glides (designHold).
		if m.releases && k.IsRepeat {
			return nil // the terminal's own repeats: the glide moves instead
		}
		defer func() { cmd = m.startHold(name) }()
		dx, dy, step := arrowStep(name)
		dx, dy = m.arrowDiagonal(arrowDir(name), dx, dy) // two arrows held: diagonally
		if step > 1 {                                    // Shift: the view moves freely
			m.designPan(dx, dy, step)
			return nil
		}
		if k.IsRepeat && time.Since(d.jumpAt) < jumpEvery {
			return nil // the terminal's own repeats: on to the next one only every jumpEvery
		}
		if !m.designJump(dx, dy) { // to the next structure that way, or a step
			m.designPan(dx, dy, 1)
		}
	case "alt+left", "alt+right", "alt+up", "alt+down", "alt+shift+left", "alt+shift+right", "alt+shift+up", "alt+shift+down":
		// Alt + arrows move the chosen structure (Alt + Shift: further).
		if m.releases && k.IsRepeat {
			return nil
		}
		if o := d.byID(d.sel); o != nil {
			defer func() { cmd = m.startHold(name) }()
			dx, dy, step := arrowStep(strings.TrimPrefix(name, "alt+"))
			dx, dy = m.arrowDiagonal(arrowDir(name), dx, dy)
			d.moveByKeys(o, dx*step, dy*step) // it snaps to structures near it (design_snap.go)
			d.dirty = true
		}
	case "tab", "shift+tab":
		m.designTab(name == "shift+tab")
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
		m.viewToPart(o, d.editing)
		return
	case "shift+tab":
		d.editing = nextPart(o, d.editing, -1)
		m.viewToPart(o, d.editing)
		return
	case "enter":
		if d.editing == partBody {
			*field += "\n"
		} else {
			d.editing = nextPart(o, d.editing, 1)
			m.viewToPart(o, d.editing)
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
		m.say("This note is full: a note holds one page (" + fmt.Sprint(noteRows) + " lines). Use a page for more.")
		return
	}
	d.dirty = true
	if m.growToFit(o) { // all of it shows, without resizing by hand
		m.keepInView(o, d.editing == partBody)
	}
}

// ---- mouse -----------------------------------------------------------------------

// designMouseHere: does the board take this mouse event?
func (m *Model) designMouseHere(y int) bool {
	return m.inDesign() && !m.help && !m.modesOpen && !m.themeOpen && m.learn == nil && !m.tabMode &&
		m.picker == nil && !(m.showTree && m.tree != nil) && y >= m.boardTop() && y < m.height-2
}

func (m *Model) designPress(ms tea.Mouse) tea.Cmd {
	d := m.ds()
	defer d.save()
	if ms.Y == m.height-3 {
		return m.designToolClick(ms.X)
	}
	sx, sy := ms.X, ms.Y-m.boardTop()
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
	d.selLink = -1
	if o == nil {
		d.sel, d.editing = -1, partNone
		if i := d.linkAt(sx, sy, m.width, m.boardH()); i >= 0 && !double {
			d.selLink = i
			a, b := d.byID(d.links[i].A), d.byID(d.links[i].B)
			m.say("Link " + octName(a) + " – " + octName(b) + " chosen: X X deletes it (Ctrl+Z brings it back).")
			d.drag = dragNone
			return nil
		}
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
	case hitDelete:
		d.drag = dragNone
		return m.designRemove(o)
	case hitDelPicture:
		d.drag = dragNone
		m.designDeletePicture(o)
		return nil
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
	sx, sy := ms.X, ms.Y-m.boardTop()
	z := d.z()
	dx, dy := float64(sx-d.pressX)/z, float64(sy-d.pressY)/z
	switch d.drag {
	case dragPan:
		d.camX, d.camY = d.origCX-dx, d.origCY-dy
	case dragMove:
		if o := d.byID(d.sel); o != nil {
			o.X, o.Y = d.snapNearest(o, d.origX+int(math.Round(dx)), d.origY+int(math.Round(dy)))
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
			if d.maxH > 0 {
				o.H = min(o.H, max(mh, d.maxH)) // no taller than the screen
			}
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
	sx, sy := ms.X, ms.Y-m.boardTop()
	z := d.z()
	if o := d.hit(sx, sy).o; o != nil && o.ID == d.sel && o.Kind != kOct && o.Kind != kCabinet &&
		ms.Mod.Contains(tea.ModShift) && (ms.Button == tea.MouseWheelUp || ms.Button == tea.MouseWheelDown) {
		// Shift + the wheel over the chosen page scrolls its text; the
		// wheel on its own always zooms, toward the pointer.
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

func (m *Model) designToolClick(x int) tea.Cmd {
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
		case t.act == "file":
			return m.designAskFile() // a picture for the chosen shape, a text file for a page
		case t.act == "undo":
			m.designUndo()
		case t.act == "redo":
			m.designRedo()
		case t.act == "zoomin":
			d.zoomTo(d.zoom+1, m.width/2, m.boardH()/2)
		case t.act == "zoomout":
			d.zoomTo(d.zoom-1, m.width/2, m.boardH()/2)
		case strings.HasPrefix(t.act, "place:"):
			d.place = strings.TrimPrefix(t.act, "place:")
			m.say("Double-click the empty board to place a " + kindNames[d.place] + ".")
		}
		return nil
	}
	return nil
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
func (m *Model) designCanvas() *boardCanvas {
	d := m.ds()
	// One canvas, kept from frame to frame: the board covers all of it.
	if d.canvas == nil || d.canvas.Width() != m.width || d.canvas.Height() != m.boardH() {
		d.canvas = newBoardCanvas(m.width, m.boardH())
	}
	c := d.canvas
	d.paintBoard(c)
	seen := make(map[int][]drawnStruct, len(d.drawn))
	for _, o := range d.octs { // in order: the last one is on top
		if !d.onScreen(o, m.width, m.boardH()) {
			continue // nothing of it shows: drawing it would only cost time
		}
		for _, p := range d.cachedLayers(o, seen) {
			p.paint(c)
		}
	}
	// Structures off the screen keep their drawings too (up to a limit),
	// so zooming back out or panning back does not draw them again.
	for id, v := range d.drawn {
		if len(seen) >= drawnKeptMax {
			break
		}
		if _, ok := seen[id]; !ok && d.byID(id) != nil {
			seen[id] = v
		}
	}
	d.drawn = seen
	d.tint(c)
	// Something dragged over the empty board: a ✚ where it would land.
	if dr := d.drop; dr.over && d.dropOn < 0 {
		if x, y := dr.x, dr.y-m.boardTop(); x >= 0 && y >= 0 && x < c.Width() && y < c.Height() {
			c.Lines[y][x] = uv.Cell{Content: "✚", Width: 1, Style: uv.Style{Fg: rgbaOf(cAccent), Bg: rgbaOf(cBG), Attrs: uv.AttrBold}}
		}
	}
	return c
}

// drawKey is everything a structure's drawing depends on, except where it
// is on the screen. When it has not changed, the last drawing is moved
// instead of drawn again, so panning and dragging stay smooth.
type drawKey struct {
	w, h                                    int
	kind, header, body, footer, color       string
	image, openStack, items                 string
	layout, scroll, editing                 int
	drawerOpen, sel, source, drop           bool
	linkMode                                bool
	bg, text, dim, accent, err, bar, stText string
	stBG, ember                             string
}

type drawnStruct struct {
	key    drawKey
	parts  []*drawnPart
	picBox [4]int // where its picture is, from its corner (picBox)
	hasPic bool
}

// drawingsKept is how many sizes of each structure are kept: zooming in
// and out goes back to the same few sizes, and those are then not drawn
// again. Only small drawings are kept more than once.
const (
	drawingsKept   = 4
	drawingKeepMax = 6000  // cells
	drawnKeptMax   = 10000 // structures with drawings kept
)

// drawnPart is one layer of a structure's drawing.
type drawnPart struct {
	l      *lipgloss.Layer
	dx, dy int          // its place, from the structure's corner
	cells  *boardCanvas // its letters and colours, read once from the layer
}

// paint copies the part's cells that fall on the canvas. A structure zoomed
// in can be many screens big; reading its styled text only once, and then
// copying just the cells that show, keeps every frame about one screen's work.
func (p *drawnPart) paint(c *boardCanvas) {
	x, y := p.l.GetX(), p.l.GetY()
	if p.cells == nil {
		s := p.l.GetContent()
		p.cells = newBoardCanvas(lipgloss.Width(s), lipgloss.Height(s))
		uv.NewStyledString(s).Draw(p.cells, p.cells.Bounds())
	}
	w, h := p.cells.Width(), p.cells.Height()
	c0, c1 := max(0, -x), min(w, c.Width()-x)
	if c0 >= c1 {
		return
	}
	for cy := max(0, -y); cy < min(h, c.Height()-y); cy++ {
		c.copyCells(x+c0, y+cy, p.cells.Lines[cy][c0:c1])
	}
}

func (d *designState) drawKeyOf(o *dOct) (drawKey, bool) {
	if o.Image != "" && !picLoaded(filepath.Join(d.imagesDir(), o.Image)) {
		return drawKey{}, false // a missing picture may turn up: look again each time
	}
	_, _, w, h := d.rect(o)
	k := drawKey{
		w: w, h: h, kind: o.Kind, header: o.Header, body: o.Body, footer: o.Footer, color: o.Color,
		image: o.Image, openStack: o.OpenStack, layout: o.Layout, scroll: o.Scroll, drawerOpen: o.DrawerOpen,
		sel: d.sel == o.ID, source: d.linkMode && d.linkFrom == o.ID, drop: d.dropOn == o.ID, linkMode: d.linkMode,
		bg: cBG, text: cText, dim: cDim, accent: cAccent, err: cError, bar: cBarBG, stText: cStatusText,
		stBG: cStatusBG, ember: cEmber,
	}
	if k.sel {
		k.editing = d.editing
	}
	if len(o.Items) > 0 {
		var sb strings.Builder
		for _, it := range o.Items {
			if it != nil {
				sb.WriteString(it.Kind + "\x00" + itemTitle(it))
			}
			sb.WriteByte('\n')
		}
		k.items = sb.String()
	}
	return k, true
}

// cachedLayers is structLayers, reusing the last drawing when nothing in it changed.
func (d *designState) cachedLayers(o *dOct, seen map[int][]drawnStruct) []*drawnPart {
	x, y, _, _ := d.rect(o)
	k, ok := d.drawKeyOf(o)
	kept := d.drawn[o.ID]
	if ok {
		for i, c := range kept {
			if c.key != k {
				continue
			}
			for _, p := range c.parts {
				p.l.X(x + p.dx).Y(y + p.dy)
			}
			if c.hasPic {
				if d.picBox == nil {
					d.picBox = map[int][4]int{}
				}
				d.picBox[o.ID] = c.picBox
			} else {
				delete(d.picBox, o.ID)
			}
			seen[o.ID] = append([]drawnStruct{c}, append(kept[:i:i], kept[i+1:]...)...)
			return c.parts
		}
	}
	layers := d.structLayers(o)
	parts := make([]*drawnPart, len(layers))
	for i, l := range layers {
		parts[i] = &drawnPart{l: l, dx: l.GetX() - x, dy: l.GetY() - y}
		if sp, ok := d.picLayers[l]; ok {
			parts[i].cells = pictureCanvas(sp.path, sp.w, sp.h, sp.fill)
			delete(d.picLayers, l)
		}
		if c, ok := d.preMade[l]; ok {
			parts[i].cells = c
			delete(d.preMade, l)
		}
	}
	if k, ok = d.drawKeyOf(o); ok { // drawing a page can change its scroll
		c := drawnStruct{key: k, parts: parts}
		c.picBox, c.hasPic = d.picBox[o.ID]
		keep := []drawnStruct{c}
		if k.w*k.h <= drawingKeepMax {
			for _, old := range kept {
				if len(keep) < drawingsKept && old.key.w*old.key.h <= drawingKeepMax {
					keep = append(keep, old)
				}
			}
		}
		seen[o.ID] = keep
	}
	return parts
}

// onScreen is whether any part of o (with its open drawer, handle and
// corner marks, which sit just outside it) falls on a w×h screen.
func (d *designState) onScreen(o *dOct, w, h int) bool {
	x, y, ow, oh := d.rect(o)
	x0, y0, x1, y1 := x, y, x+ow, y+oh
	if o.DrawerOpen && o.Kind != kCabinet {
		dx, dy, dw, dh := d.drawerRect(o)
		x0, y0, x1, y1 = min(x0, dx), min(y0, dy), max(x1, dx+dw), max(y1, dy+dh)
	}
	const margin = 3
	return x1+margin > 0 && y1+margin > 0 && x0-margin < w && y0-margin < h
}

// designFrameGap is the shortest time between two drawings of the board,
// when designFrames is on: the screen is sent to the terminal at most 120
// times a second, so drawing more often only falls behind. A fast drag
// brings a mouse move every few milliseconds; drawing the whole board for
// each of them, full screen and zoomed in, left it trailing behind the hand.
const designFrameGap = time.Second / 120

// frameGap is the wait before the next drawing: three times what the last
// one took, so drawing never takes more than a third of the time, but at
// most designFrameGap and at least 2 ms. A quick board waits less, and what
// reaches the screen is fresher.
func (d *designState) frameGap() time.Duration {
	return min(designFrameGap, max(2*time.Millisecond, 3*d.frameCost))
}

// designFrames: the board may reuse its last drawing for a moment (on in
// the real program, see main.go; off in tests, which look straight away).
var designFrames = false

// designFrameMsg asks for the board to be drawn again: the last View reused
// an older drawing.
type designFrameMsg struct{}

// designRows draws the board in the text area.
func (m *Model) designRows(sb *strings.Builder) {
	d := m.ds()
	if designFrames && d.frame != "" && d.frameW == m.width && d.frameH == m.boardH() &&
		time.Since(d.frameAt) < d.frameGap() {
		sb.WriteString(d.frame) // designFrameCmd has a fresh drawing coming
		return
	}
	start := time.Now()
	var fb strings.Builder
	fb.Grow(len(d.frame) + len(d.frame)/8)
	m.designCanvas().renderTo(&fb)
	d.frame, d.frameAt, d.frameW, d.frameH = fb.String(), time.Now(), m.width, m.boardH()
	d.frameCost = d.frameAt.Sub(start)
	sb.WriteString(d.frame)
}

// designFrameCmd: when the next View would reuse the last drawing, a tick
// draws the latest board as soon as the gap is over.
func (m *Model) designFrameCmd() tea.Cmd {
	if !designFrames || !m.inDesign() {
		return nil
	}
	d := m.ds()
	wait := d.frameGap() - time.Since(d.frameAt)
	if d.frameTick || wait <= 0 {
		return nil
	}
	d.frameTick = true
	return tea.Tick(wait, func(time.Time) tea.Msg { return designFrameMsg{} })
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
// boardMark is a letter on the empty board: a grid dot or a piece of a link.
type boardMark struct {
	s, fg string
}

// boardLayer is the empty board as styled text (paintBoard lays the same
// cells straight onto the canvas; the tests check they match).
func (d *designState) boardLayer(w, h int) string {
	type mark = boardMark
	at := d.boardMarks(w, h)
	// Styling is slow next to the rest, and the same few dots and gaps come
	// again and again, so each is styled once per frame.
	bg := st(cText, cBG)
	gaps := map[int]string{}
	gap := func(n int) string {
		s, ok := gaps[n]
		if !ok {
			s = bg.Render(strings.Repeat(" ", n))
			gaps[n] = s
		}
		return s
	}
	styled := map[mark]string{}
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
				sb.WriteString(gap(run))
				run = 0
			}
			s, ok := styled[mk]
			if !ok {
				s = st(mk.fg, cBG).Render(mk.s)
				styled[mk] = s
			}
			sb.WriteString(s)
		}
		if run > 0 {
			sb.WriteString(gap(run))
		}
		if y < h-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// boardMarks is where the empty board's dots and links are on a w×h screen.
func (d *designState) boardMarks(w, h int) map[[2]int]boardMark {
	at := map[[2]int]boardMark{}
	d.boardDots(w, h, func(x, y int, mk boardMark) { at[[2]int{x, y}] = mk })
	for cell, l := range d.linkCells(w, h) {
		at[cell] = l
	}
	return at
}

// boardDots calls dot for each grid dot (and the + at the middle) on a
// w×h screen, row by row.
func (d *designState) boardDots(w, h int, dot func(x, y int, mk boardMark)) {
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
	plain, centre := boardMark{"·", mixHex(cDim, cBG, 0.45)}, boardMark{"+", mixHex(cDim, cBG, 0.1)}
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
				dot(x[0], y, centre)
			} else {
				dot(x[0], y, plain)
			}
		}
	}
}

// paintBoard lays the empty board straight onto the canvas as cells: no
// styled text to build and read back, which was a fifth of a frame.
func (d *designState) paintBoard(c *boardCanvas) {
	colours := map[string]color.Color{}
	rgba := func(hex string) color.Color {
		col, ok := colours[hex]
		if !ok {
			r, g, b := rgb(hex)
			col = color.RGBA{uint8(r), uint8(g), uint8(b), 255}
			colours[hex] = col
		}
		return col
	}
	blank := make(uv.Line, c.Width())
	for x := range blank {
		blank[x] = uv.Cell{Content: " ", Width: 1, Style: uv.Style{Fg: rgba(cText), Bg: rgba(cBG)}}
	}
	for _, line := range c.Lines {
		copy(line, blank)
	}
	bg := rgba(cBG)
	put := func(x, y int, mk boardMark) {
		c.Lines[y][x] = uv.Cell{Content: mk.s, Width: 1, Style: uv.Style{Fg: rgba(mk.fg), Bg: bg}}
	}
	d.boardDots(c.Width(), c.Height(), put)
	d.linkWalk(c.Width(), c.Height(), func(x, y, _ int, s, fg string) { put(x, y, boardMark{s, fg}) })
}

// boardCanvas is the board's grid of cells. It is uv's screen buffer, used
// directly: whole runs of cells are copied at once, not set one by one.
type boardCanvas struct {
	uv.ScreenBuffer
}

func newBoardCanvas(w, h int) *boardCanvas {
	s := uv.NewScreenBuffer(w, h)
	s.Method = ansi.GraphemeWidth // as Lip Gloss measures
	return &boardCanvas{s}
}

// Render is the cells as styled text, as Lip Gloss's canvas renders them.
func (c *boardCanvas) Render() string { return uv.TrimSpace(c.Buffer.Render()) }

// renderTo writes the cells as styled text, one row per line, each line
// ending in "\n". It gives the same cells as uv's own rendering when read
// back, without its allocations: that was half of a full-screen frame.
// Each change of style is written whole (reset, then everything).
func (c *boardCanvas) renderTo(sb *strings.Builder) {
	var buf []byte
	for _, line := range c.Lines {
		buf = buf[:0]
		var pen uv.Style
		for i := range line {
			cell := &line[i]
			if cell.IsZero() {
				continue // the right half of a wide letter
			}
			if cell.Link.URL != "" {
				buf = append(buf[:0], uv.Lines{line}.Render()...) // links: uv's way
				pen = uv.Style{}
				break
			}
			if !cell.Style.Equal(&pen) {
				buf = appendStyle(buf, &cell.Style)
				pen = cell.Style
			}
			if cell.Content == "" {
				buf = append(buf, ' ')
			} else {
				buf = append(buf, cell.Content...)
			}
		}
		if !pen.IsZero() {
			buf = append(buf, "\x1b[m"...)
		}
		sb.Write(buf)
		sb.WriteByte('\n')
	}
}

// appendStyle appends the escape that sets style s from scratch.
func appendStyle(buf []byte, s *uv.Style) []byte {
	if s.IsZero() {
		return append(buf, "\x1b[m"...)
	}
	fg, fgOK := s.Fg.(color.RGBA)
	bg, bgOK := s.Bg.(color.RGBA)
	if s.Underline != 0 || s.UnderlineColor != nil || (s.Fg != nil && !fgOK) || (s.Bg != nil && !bgOK) {
		return append(append(buf, "\x1b[m"...), s.String()...) // rarer styles: uv's way
	}
	buf = append(buf, "\x1b[0"...)
	for bit, code := range [...]byte{'1', '2', '3', '5', '6', '7', '8', '9'} {
		if s.Attrs&(1<<bit) != 0 {
			buf = append(buf, ';', code)
		}
	}
	rgb := func(buf []byte, lead string, c color.RGBA) []byte {
		buf = append(buf, lead...)
		buf = strconv.AppendUint(buf, uint64(c.R), 10)
		buf = append(buf, ';')
		buf = strconv.AppendUint(buf, uint64(c.G), 10)
		buf = append(buf, ';')
		return strconv.AppendUint(buf, uint64(c.B), 10)
	}
	if s.Fg != nil {
		buf = rgb(buf, ";38;2;", fg)
	}
	if s.Bg != nil {
		buf = rgb(buf, ";48;2;", bg)
	}
	return append(buf, 'm')
}

// copyCells puts src's cells at (x, y). A wide letter cut in two at either
// end becomes a space, as setting the cells one by one would leave it.
func (c *boardCanvas) copyCells(x, y int, src uv.Line) {
	line := c.Lines[y]
	n := copy(line[x:], src)
	if n == 0 {
		return
	}
	blank := func(i int) { line[i].Content, line[i].Width = " ", 1 }
	if x > 0 && line[x-1].Width > 1 {
		blank(x - 1) // its right half was just covered
	}
	if line[x].Width == 0 {
		blank(x) // the right half of a letter whose left half is cut off
	}
	if last := x + n - 1; line[last].Width > 1 {
		blank(last) // its right half is cut off
	}
	if end := x + n; end < len(line) && line[end].Width == 0 {
		blank(end) // the right half of a letter just covered
	}
}

// linkCells works out the links' lines: each goes with right-angle bends
// from the middle of one structure to the middle of the other. The
// structures are drawn over the ends, so a line seems to leave from the edge.
func (d *designState) linkCells(w, h int) map[[2]int]struct{ s, fg string } {
	return d.linkGrid(w, h, nil)
}

// linkAt is the link drawn at screen cell (sx, sy) of a w×h board, or -1.
func (d *designState) linkAt(sx, sy, w, h int) int {
	owner := map[[2]int]int{}
	if _, ok := d.linkGrid(w, h, owner)[[2]int{sx, sy}]; !ok {
		return -1
	}
	return owner[[2]int{sx, sy}]
}

// linkGrid is linkCells, noting in owner (when not nil) the link of each cell.
func (d *designState) linkGrid(w, h int, owner map[[2]int]int) map[[2]int]struct{ s, fg string } {
	out := map[[2]int]struct{ s, fg string }{}
	d.linkWalk(w, h, func(x, y, link int, s, fg string) {
		out[[2]int{x, y}] = struct{ s, fg string }{s, fg}
		if owner != nil {
			owner[[2]int{x, y}] = link
		}
	})
	return out
}

// linkWalk calls cell for each cell of a w×h screen a link line passes
// through, row by row, with the link's index, letter and colour. The grid
// is flat, and a link whose line is wholly off the screen is passed over.
func (d *designState) linkWalk(w, h int, cell func(x, y, link int, s, fg string)) {
	if len(d.links) == 0 || w <= 0 || h <= 0 {
		return
	}
	ways := make([]uint8, w*h)
	colour := make([]string, w*h)
	owner := make([]int32, w*h)
	current := 0
	add := func(x, y, way int, c string) {
		if x < 0 || y < 0 || x >= w || y >= h {
			return
		}
		k := y*w + x
		ways[k] |= uint8(way)
		colour[k] = c
		owner[k] = int32(current)
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
	for i, l := range d.links {
		a, b := d.byID(l.A), d.byID(l.B)
		if a == nil || b == nil {
			continue
		}
		ax, ay, aw, ah := d.rect(a)
		bx, by, bw, bh := d.rect(b)
		x0, y0, x1, y1 := ax+aw/2, ay+ah/2, bx+bw/2, by+bh/2
		if max(x0, x1) < -1 || min(x0, x1) > w || max(y0, y1) < -1 || min(y0, y1) > h {
			continue // the whole line is off the screen
		}
		current = i
		c := mixHex(mixHex(a.Color, b.Color, 0.5), cBG, 0.2)
		if i == d.selLink {
			c = cAccent // the chosen link
		}
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
	for k, way := range ways {
		if way == 0 {
			continue
		}
		if s, ok := boxLetters[int(way)]; ok {
			cell(k%w, k/w, int(owner[k]), s, colour[k])
		}
	}
}

// octLayers draws one structure as Lip Gloss layers: an octagon with its
// corners cut, the header at the top, the text in the middle and the footer
// at the bottom. The cut corners are separate, narrower layers, so the
// board shows through beside them.
func (d *designState) octLayers(o *dOct) []*lipgloss.Layer {
	delete(d.picBox, o.ID) // drawing it says where its picture is now
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
	if d.dropOn == o.ID {
		edge = cAccent // something dragged over it would go in
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
		spots := len(d.picSpots)
		// A header wider than the box wraps onto more rows and moves the
		// picture down by an amount only the text knows: draw it as text.
		d.picsAsText = lipgloss.Width(head) > w-4
		inner := head + "\n" + d.octContent(o, editing, w-4, h-3, fill, dim)
		d.picsAsText = false
		add(lipgloss.NewStyle().Border(octSides, false, true, false, true).BorderForeground(hex(edge)).BorderBackground(hex(fill)).
			Background(hex(fill)).Foreground(hex(cText)).Padding(0, 1).Width(w).Height(h-2).
			Render(centredFirst(inner, w-4, fill)), x, y+1)
		layers = d.pictureLayers(layers, spots, x+2, y+2, o, x, y)
		add(diag("╲")+line(w-2)+diag("╱"), x, y+h-1)
		return d.octExtras(layers, o, x, y, w, h, edge, fill, selected, source)
	}

	foot := part(partFooter, o.Footer, "footer", st(mixHex(cText, fill, 0.3), fill).Italic(true), w-6)
	add(diag("╱")+line(w-6)+diag("╲"), x+2, y)
	add(diag("╱")+centred(head, w-4)+diag("╲"), x+1, y+1)
	spots := len(d.picSpots)
	d.emptyHoles, d.holesLeft = middlesDirect, false
	content := d.octContent(o, editing, w-4, h-6, fill, dim)
	d.emptyHoles = false
	if c := d.middleCells(content, w, h-4, edge, fill); c != nil {
		l := lipgloss.NewLayer("").X(x).Y(y + 2) // its cells are made already
		d.preCells(l, c)
		layers = append(layers, l)
	} else {
		if d.holesLeft {
			// Built for middleCells, but it could not take it: start again.
			d.picSpots = d.picSpots[:spots]
			content = d.octContent(o, editing, w-4, h-6, fill, dim)
		}
		add(lipgloss.NewStyle().Border(octMiddle).BorderForeground(hex(edge)).BorderBackground(hex(fill)).
			Background(hex(fill)).Foreground(hex(cText)).Padding(0, 1).Width(w).Height(h-4).
			Render(content), x, y+2)
	}
	layers = d.pictureLayers(layers, spots, x+2, y+3, o, x, y)
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
		return centreLine(st(dim, fill).Italic(true).Render(ansi.Truncate("double-click to write", w, "…")), w, fill)
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
		lines[i] = text.Render(strings.TrimRight(l, " "))
	}
	if editing == partBody {
		lines[len(lines)-1] += st(cAccent, fill).Render("▏")
	}
	for i := range lines {
		lines[i] = centreLine(lines[i], w, fill) // all text on the board is centred
	}
	return strings.Join(lines, "\n")
}

// centreLine puts styled line s in the middle of w cells (the space on its
// left in the fill colour; what is on its right is left to the caller).
func centreLine(s string, w int, fill string) string {
	n := lipgloss.Width(s)
	if n >= w {
		return s
	}
	return st(cText, fill).Render(strings.Repeat(" ", (w-n)/2)) + s
}

// centredFirst centres the first line (the header of a small structure).
// middlesDirect: a structure's middle (its sides, dotted lines and text) is
// built as cells, not as text Lip Gloss lays out and uv reads back. Zoomed
// in, the middle is hundreds of cells across and that was most of the work
// of a zoom step. (Off only in the tests that check both ways match.)
var middlesDirect = true

// middleCells builds the middle of a structure w wide and h high: the
// dotted lines at top and bottom, a side at each edge, and content inside,
// one cell of room on either side, exactly as Lip Gloss lays it out. It
// returns nil when content needs Lip Gloss (a line too wide, too many lines).
func (d *designState) middleCells(content string, w, h int, edge, fill string) *boardCanvas {
	if !middlesDirect || w < 4 || h < 2 {
		return nil
	}
	lines := strings.Split(content, "\n")
	cw := w - 4
	if len(lines) > h-2 {
		return nil
	}
	text, bg, edgeFG := rgbaOf(cText), rgbaOf(fill), rgbaOf(edge)
	c := newBoardCanvas(w, h)
	blank := uv.Cell{Content: " ", Width: 1, Style: uv.Style{Bg: bg}} // Lip Gloss leaves room with no letter colour
	for _, line := range c.Lines {
		for x := range line {
			line[x] = blank
		}
	}
	put := func(x, y int, s string) {
		c.Lines[y][x] = uv.Cell{Content: s, Width: 1, Style: uv.Style{Fg: edgeFG, Bg: bg}}
	}
	for x := 1; x < w-1; x++ {
		put(x, 0, octMiddle.Top)
		put(x, h-1, octMiddle.Bottom)
	}
	put(0, 0, octMiddle.TopLeft)
	put(w-1, 0, octMiddle.TopRight)
	put(0, h-1, octMiddle.BottomLeft)
	put(w-1, h-1, octMiddle.BottomRight)
	for y := 1; y < h-1; y++ {
		put(0, y, octMiddle.Left)
		put(w-1, y, octMiddle.Right)
	}
	var row *boardCanvas
	for i, s := range lines {
		if s == "" {
			continue
		}
		if row == nil || row.Width() < len(s)+1 {
			row = newBoardCanvas(len(s)+1, 1)
		}
		uv.NewStyledString(s).Draw(row, row.Bounds())
		cells := row.Lines[0]
		n := len(cells)
		for n > 0 && cells[n-1] == uv.EmptyCell {
			n--
		}
		if n > cw {
			return nil // Lip Gloss would wrap it
		}
		for x := range n {
			cell := cells[x]
			if cell.IsZero() { // the right half of a wide letter: left as it is
				c.Lines[1+i][2+x] = cell
				continue
			}
			if cell.Style.Fg == nil {
				cell.Style.Fg = text
			}
			if cell.Style.Bg == nil {
				cell.Style.Bg = bg
			}
			c.Lines[1+i][2+x] = cell
		}
	}
	return c
}

// rgbaOf is a #rrggbb colour as uv reads it from text.
func rgbaOf(hex string) color.Color {
	r, g, b := rgb(hex)
	return color.RGBA{uint8(r), uint8(g), uint8(b), 255}
}

// preCells notes that layer l stands for cells made already: cachedLayers
// takes them instead of reading the layer's (empty) text.
func (d *designState) preCells(l *lipgloss.Layer, c *boardCanvas) {
	if d.preMade == nil {
		d.preMade = map[*lipgloss.Layer]*boardCanvas{}
	}
	d.preMade[l] = c
}

// pictureLayers turns the pictures met since picSpots[from] into layers
// whose text corner is at (cx, cy): empty stand-ins that cachedLayers
// fills with the picture's cells.
func (d *designState) pictureLayers(layers []*lipgloss.Layer, from, cx, cy int, o *dOct, x, y int) []*lipgloss.Layer {
	for _, sp := range d.picSpots[from:] {
		px, py := cx+sp.dx, cy+sp.dy
		l := lipgloss.NewLayer("").X(px).Y(py)
		if d.picLayers == nil {
			d.picLayers = map[*lipgloss.Layer]picSpot{}
		}
		d.picLayers[l] = sp
		layers = append(layers, l)
		if d.picBox == nil {
			d.picBox = map[int][4]int{}
		}
		d.picBox[o.ID] = [4]int{px - x, py - y, sp.w, sp.h}
		if dx, dy, ok := d.pictureDeleteSpot(o, x, y); ok {
			layers = append(layers, lipgloss.NewLayer(st(cBG, cError).Bold(true).Render("✕")).X(dx).Y(dy))
		}
	}
	d.picSpots = d.picSpots[:from]
	return layers
}

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
	if dx, dy, ok := d.deleteSpot(o, x, y, w, h); ok {
		layers = append(layers, lipgloss.NewLayer(st(cError, fill).Bold(true).Render("✕")).X(dx).Y(dy))
	}
	return layers
}

// tint lays the pen's colour on the drawn canvas. Under the text: the
// cell's background takes the colour and the letter stays exactly as it
// was. Over the text: the colour is laid over everything; letters show
// through, tinted, and empty cells show the ink's grain.
func (d *designState) tint(c *boardCanvas) {
	if len(d.marks) == 0 {
		return
	}
	z := d.z()
	// The world column under each screen column, and the world rows that
	// have ink at all: most rows have none and are passed over.
	wxs := make([]int, c.Width())
	for x := range wxs {
		wxs[x] = int(math.Floor(d.camX + (float64(x)+0.5)/z))
	}
	// Kept until the ink changes (its version, and its size as a check).
	if d.inked == nil || d.inkedVer != d.marksVer || d.inkedLen != len(d.marks) {
		d.inked = make(map[int]bool, len(d.marks))
		for at := range d.marks {
			d.inked[at[1]] = true
		}
		d.inkedVer, d.inkedLen = d.marksVer, len(d.marks)
	}
	inked := d.inked
	// The same cell colours under the same pen come again and again: each
	// mix is worked out once a frame.
	type look struct {
		bg, fg     color.Color
		pen        int8
		over, void bool
	}
	type result struct {
		bg, fg color.Color
		shade  bool // a blank cell gets the ░ shading
	}
	done := map[look]result{}
	for y := range c.Height() {
		wy := int(math.Floor(d.camY + (float64(y)+0.5)/z))
		if !inked[wy] {
			continue
		}
		line := c.Lines[y]
		for x, wx := range wxs {
			mk, ok := d.marks[[2]int{wx, wy}]
			if !ok {
				continue
			}
			cell := &line[x]
			if cell.Width == 0 {
				continue
			}
			k := look{cell.Style.Bg, cell.Style.Fg, mk.Pen, mk.Over, strings.TrimSpace(cell.Content) == ""}
			r, ok := done[k]
			if !ok {
				pen := penColors[mk.Pen]
				bg, fg := colorHex(cell.Style.Bg, cBG), colorHex(cell.Style.Fg, cText)
				switch {
				case !mk.Over:
					r = result{bg: hex(mixHex(bg, pen, 0.5)), fg: cell.Style.Fg}
				case k.void:
					nbg := mixHex(bg, pen, 0.38)
					r = result{bg: hex(nbg), fg: hex(mixHex(nbg, pen, 0.55)), shade: true}
				default:
					r = result{bg: hex(mixHex(bg, pen, 0.38)), fg: hex(mixHex(fg, pen, 0.45))}
				}
				done[k] = r
			}
			cell.Style.Bg, cell.Style.Fg = r.bg, r.fg
			if r.shade {
				cell.Content, cell.Width = "░", 1
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
	put(" ", bar, "")
	put("img", st(cStatusText, cBarBG), "file")
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
	put(" ", bar, "")
	canUndo, canRedo := len(d.undo) > 0 || (d.undoBase != nil && d.dirty), len(d.redo) > 0
	put(" ↶ ", st(map[bool]string{true: cStatusText, false: mixHex(cDim, cBarBG, 0.4)}[canUndo], cBarBG).Bold(canUndo), "undo")
	put(" ↷ ", st(map[bool]string{true: cStatusText, false: mixHex(cDim, cBarBG, 0.4)}[canRedo], cBarBG).Bold(canRedo), "redo")
	right := bar.Render(fmt.Sprintf("%d structures · %d links ", len(d.octs), len(d.links)))
	if m.width-x-lipgloss.Width(right) <= 0 { // short, when the long one does not fit
		right = bar.Render(fmt.Sprintf("%d %s · %d ⛓ ", len(d.octs), kindIcons[kOct], len(d.links)))
	}
	if gap := m.width - x - lipgloss.Width(right); gap > 0 {
		sb.WriteString(bar.Render(strings.Repeat(" ", gap)) + right)
	} else {
		return fit(sb.String(), m.width, bar)
	}
	return sb.String()
}

// boardTop is the screen row where the board starts: under the top panel,
// the tab bar and Design mode's own top bar.
func (m *Model) boardTop() int { return textTop + 1 }

// boardH is how many screen rows the board has (the top bar takes one).
func (m *Model) boardH() int { return max(m.textHeight()-1, 1) }

// designTopBar is the bar over the board. For now it names the board and
// what is chosen; what goes on it is still to be decided.
func (m *Model) designTopBar() string {
	d := m.ds()
	bar := st(cStatusText, cBarBG)
	left := " " + kindIcons[kOct] + " Design board"
	right := ""
	switch {
	case d.placing:
		right = "Place: P page · N note · C cabinet · S shape "
	case d.byID(d.sel) != nil:
		right = "Chosen: " + octName(d.byID(d.sel)) + " "
	}
	l, r := bar.Bold(true).Render(left), bar.Render(right)
	if gap := m.width - lipgloss.Width(l) - lipgloss.Width(r); gap > 0 {
		return l + bar.Render(strings.Repeat(" ", gap)) + r
	}
	return fit(l+bar.Render(" ")+r, m.width, bar)
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
			part = map[int]string{partHeader: "title", partBody: "text", partFooter: "footer"}[d.editing]
		}
		hint = "Writing the " + part + "  ·  Tab: next part  ·  Esc: done"
	case d.linkMode && d.linkFrom >= 0:
		hint = "Link mode  ·  click structures to link them  ·  Esc: leave"
	case d.linkMode:
		hint = "Link mode  ·  click the first structure  ·  Esc: leave"
	case d.pen == rubber:
		hint = "Rubber  ·  drag to rub out  ·  Esc: put down"
	case d.pen >= 0:
		hint = "Pen " + penNames[d.pen] + "  ·  drag to colour  ·  Esc: put down"
	case d.sel >= 0 && d.byID(d.sel) != nil:
		hint = selHint(d.byID(d.sel))
	default:
		hint = "Double-click: new " + kindNames[d.place] + "  ·  Alt+N: place  ·  1–8: pen  ·  F1: all keys"
	}
	style := stStatus
	if d.saveErr != "" {
		style = st(cError, cStatusBG)
	}
	return fit(style.Render(" "+hint), m.width, stStatus)
}
