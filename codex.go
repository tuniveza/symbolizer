package main

// ---- Codex mode: books you can play ---------------------------------------------------
//
// A codex is like HyperCard: pages of text, pictures, drawings, animated
// symbols and buttons, wrapped up behind a cover. Each page has one of 16
// layouts (codexLayouts); each thing on it sits in one of the layout's
// slots, several in a slot one under another. Pages lie on their side
// (wide) unless the codex is turned upright (O in edit mode).
//
// Every thing can be activated: a click on it, Enter on it (Tab moves
// between them), or a button aimed at it. What activating does is its row
// in the activation table (T in edit mode): hide, show, start its
// animation, change its colour, font, size or animation, swap its words,
// flash. Activating again puts it back. A button does its action instead:
// next page, page before, go to a page, or activate a chosen thing.
//
// Each codex is a folder: ~/.local/share/symbolizer/codex/<id>/codex.json,
// with its pictures in images/ next to it (copied in, so it stays whole).
// On the Design board a codex shows as its cover (kCodex, codex_design.go):
// Enter opens it here, E opens it to edit, and Esc goes back to the board.
//
// The keys are in codex_keys.go, the drawing in codex_draw.go, the fonts,
// sizes, animations and symbols in codex_fx.go.

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

const codexMode = 8 // its place in modes

func (m *Model) inCodex() bool { return m.mode == codexMode }

// The kinds of thing on a page.
const (
	ceText    = "text"
	ceButton  = "button"
	ceImage   = "image"
	ceSymbol  = "symbol"
	ceDrawing = "drawing"
)

var codexKindNames = map[string]string{ceText: "text", ceButton: "button", ceImage: "picture", ceSymbol: "symbol", ceDrawing: "drawing"}
var codexKindIcons = map[string]string{ceText: "T", ceButton: "▣", ceImage: "▨", ceSymbol: "✦", ceDrawing: "✎"}

// What a button does.
const (
	caNext     = "next"
	caPrev     = "prev"
	caGoto     = "goto"
	caActivate = "activate"
)

var codexActions = []string{caNext, caPrev, caGoto, caActivate}
var codexActionNames = map[string]string{caNext: "next page", caPrev: "page before", caGoto: "go to page", caActivate: "activate"}

// codexDo is one choice in the activation table: what activating a thing does.
type codexDo struct {
	key, name string
	value     string // what its value column holds: "", "colour", "anim", "font", "size", "words"
}

var codexDos = []codexDo{
	{"", "nothing", ""},
	{"flash", "flashes", ""},
	{"hide", "hides (again: shows)", ""},
	{"show", "shows (hidden at first)", ""},
	{"animate", "animates (again: stops)", "anim"},
	{"colour", "changes colour", "colour"},
	{"font", "changes font", "font"},
	{"grow", "changes size", "size"},
	{"swap", "swaps its words", "words"},
	{"invert", "turns inside out", ""},
}

func codexDoOf(key string) (int, codexDo) {
	for i, d := range codexDos {
		if d.key == key {
			return i, d
		}
	}
	return 0, codexDos[0]
}

// codexOn is a thing's row in the activation table.
type codexOn struct {
	Do    string `json:"do,omitempty"`
	Color string `json:"color,omitempty"`
	Anim  int    `json:"anim,omitempty"`
	Font  int    `json:"font,omitempty"`
	Size  int    `json:"size,omitempty"`
	Alt   string `json:"alt,omitempty"` // swap: the other words (a picture: another file; a symbol: its number)
}

// codexInk is one dot of a drawing, on its own grid (codexInkW × codexInkH).
type codexInk struct {
	X int  `json:"x"`
	Y int  `json:"y"`
	C int8 `json:"c"` // codexInkColors
}

// A drawing is kept on a grid of dots this big, and stretched to its place.
const codexInkW, codexInkH = 96, 48

var codexInkColors = []string{"#ffd400", "#ff8a00", "#ff3b3b", "#ff4fa3", "#a259ff", "#3b82ff", "#22d3ee", "#4ade80", "#fffaf0", "#2b2118"}

// codexEl is one thing on a page.
type codexEl struct {
	ID     int        `json:"id"`
	Kind   string     `json:"kind"`
	Slot   int        `json:"slot"`
	Text   string     `json:"text,omitempty"` // a text's words, a button's label
	Font   int        `json:"font,omitempty"`
	Size   int        `json:"size,omitempty"` // 0 or 1 normal … codexSizes
	Color  string     `json:"color,omitempty"`
	Anim   int        `json:"anim,omitempty"`
	Align  int        `json:"align,omitempty"`  // 0 middle, 1 left, 2 right
	VAlign int        `json:"valign,omitempty"` // up and down in its slot: 0 middle, 1 top, 2 bottom (the user asked, 19 Nov 2026)
	Image  string     `json:"image,omitempty"`  // a file in the codex's images folder
	Symbol int        `json:"symbol,omitempty"`
	Ink    []codexInk `json:"ink,omitempty"`
	Action string     `json:"action,omitempty"` // a button: codexActions
	Target int        `json:"target,omitempty"` // go to page: its number (1 the first); activate: the thing's ID
	On     codexOn    `json:"on,omitempty"`
	Border int        `json:"border,omitempty"` // 0 none, else codexBorders[Border-1] (codex_frames.go)
	Style  int        `json:"style,omitempty"`  // a button's look: codexButtonStyles
	Whole  bool       `json:"whole,omitempty"`  // a picture: shown whole (else it fills its box, cut at the edges)
	Head   int        `json:"head,omitempty"`   // a text: 0 body text, 1–3 a heading (codex_look.go)
	Fill   string     `json:"fill,omitempty"`   // the fill's colour ("" the theme's)
	FillBy int        `json:"fillBy,omitempty"` // the fill's look: 0 none, else codexFills (codex_look.go)
	W      int        `json:"w,omitempty"`      // its width: 0 as it comes, else this % of its place (codex_size.go)
	H      int        `json:"h,omitempty"`      // its height: 0 as it comes; a picture or drawing: % of its place; a button: rows
}

type codexPage struct {
	Layout int        `json:"layout"`
	Paper  string     `json:"paper,omitempty"` // "" the codex's paper
	Els    []*codexEl `json:"els"`
	Turn   int        `json:"turn,omitempty"`   // how the page comes in: codexTurns16 (codex_turns.go)
	Border int        `json:"border,omitempty"` // 0 the codex's page border, 1…16 its own, -1 none
	Plain  bool       `json:"plain,omitempty"`  // no header or footer on this page
}

type codexFile struct {
	V        int          `json:"v,omitempty"` // the file's version (codexFileV): old files are brought up to date once
	Title    string       `json:"title"`
	Subtitle string       `json:"subtitle,omitempty"`
	Color    string       `json:"color"`  // the cover
	Symbol   int          `json:"symbol"` // the cover's animated symbol
	Paper    string       `json:"paper"`  // the pages
	Tall     bool         `json:"tall,omitempty"`
	Pages    []*codexPage `json:"pages"`
	NextID   int          `json:"nextID"`
	Room     int          `json:"room,omitempty"` // made in Codex mode: its own room on Design (-1: one is still to be made)

	CoverTurn   int    `json:"coverTurn,omitempty"`   // how the cover opens: 0 the page turn, else codexTurns16[CoverTurn-1]
	PageBorder  int    `json:"pageBorder,omitempty"`  // every page's border: 0 none, else codexPageBorders[PageBorder-1]
	Header      string `json:"header,omitempty"`      // words at the top of every page ({page} {pages} {title})
	Footer      string `json:"footer,omitempty"`      // words at the bottom of every page
	TitleFont   int    `json:"titleFont,omitempty"`   // the cover title's font (0: big letters when they fit)
	HFFont      int    `json:"hfFont,omitempty"`      // the header's and footer's font
	CoverBorder int    `json:"coverBorder,omitempty"` // the cover's frame: 0 gilt, 1…16 codexPageBorders, 17 none (codex_size.go)

	// The cover's own size and its parts (codex_cover.go, the user asked 19 Nov 2026).
	CoverW     int              `json:"coverW,omitempty"`     // its width in Codex mode: % of the page (0: all of it)
	CoverH     int              `json:"coverH,omitempty"`     // its height: % of the page (0: all of it)
	CoverV     int              `json:"coverV,omitempty"`     // where the words sit: 0 the middle, 1 the top, 2 the bottom
	CoverOrder []int            `json:"coverOrder,omitempty"` // the order of symbol, title and subtitle (cpSymbol…)
	CoverParts []codexCoverPart `json:"coverParts,omitempty"` // the symbol, title, subtitle and page count
}

// codexPalette: the colours C steps through (text, buttons, covers, symbols).
// The first 16 are Emberistal's warm colours (the user asked, 16 Nov 2026); 16 more warm and 16 cool ones follow (19 Nov). "" is
// the theme's own colour for that thing (codexTheme…).
var codexPalette = []string{
	"#ff6600", "#ff3704", "#ff4518", "#ff8a1a", "#ffb400", "#ffc14d", "#ffa66b", "#ffb870",
	"#fff0d6", "#d84017", "#d74a00", "#983818", "#8e3c1a", "#5a2214", "#2a1610", "#161616",
	// More shades (19 Nov 2026, the user asked for more colour variations):
	// still warm, from pale sand and rose to deep wine and plum.
	"#ffe08a", "#ffd6a5", "#f6c28b", "#e8a87c", "#ff8f8f", "#ff6b6b", "#e85d75", "#c9184a",
	"#a4133c", "#800f2f", "#9d4edd", "#6a3d9a", "#b08968", "#7f5539", "#ddb892", "#ede0d4",
	// Cool colours (19 Nov 2026, the user asked): sky to navy, teal, greens, lilac and slate.
	"#bde0fe", "#7cc6fe", "#3a86ff", "#1d4ed8", "#1e3a8a", "#0b1d3a", "#a5f3fc", "#22d3ee",
	"#0e9f9a", "#115e59", "#b7e4c7", "#52b788", "#2d6a4f", "#c8b6ff", "#7b8cde", "#64748b",
}

// codexPapers: the paper colours W steps through.
// "" is the theme's paper: warm charcoal in Emberistal.
var codexPapers = []string{"", "#1b1614", "#24120b", "#2a1610", "#161616", "#0f0b0a", "#3a1c10", "#fff0d6", "#f4ead5"}

// ---- the 16 layouts ---------------------------------------------------------------------

type codexLayout struct {
	name  string
	slots [][4]float64 // each slot: x, y, w, h as parts of the page (0 … 1)
}

var codexLayouts = [16]codexLayout{
	{"one area", [][4]float64{{0, 0, 1, 1}}},
	{"title and text", [][4]float64{{0, 0, 1, .22}, {0, .26, 1, .74}}},
	{"two columns", [][4]float64{{0, 0, .48, 1}, {.52, 0, .48, 1}}},
	{"three columns", [][4]float64{{0, 0, .31, 1}, {.345, 0, .31, 1}, {.69, 0, .31, 1}}},
	{"title, two columns", [][4]float64{{0, 0, 1, .22}, {0, .26, .48, .74}, {.52, .26, .48, .74}}},
	{"picture left, text right", [][4]float64{{0, 0, .45, 1}, {.49, 0, .51, 1}}},
	{"text left, picture right", [][4]float64{{0, 0, .51, 1}, {.55, 0, .45, 1}}},
	{"picture over a caption", [][4]float64{{0, 0, 1, .7}, {0, .74, 1, .26}}},
	{"grid of four", [][4]float64{{0, 0, .48, .46}, {.52, 0, .48, .46}, {0, .54, .48, .46}, {.52, .54, .48, .46}}},
	{"grid of six", [][4]float64{{0, 0, .31, .46}, {.345, 0, .31, .46}, {.69, 0, .31, .46}, {0, .54, .31, .46}, {.345, .54, .31, .46}, {.69, .54, .31, .46}}},
	{"sidebar left", [][4]float64{{0, 0, .26, 1}, {.3, 0, .7, 1}}},
	{"sidebar right", [][4]float64{{0, 0, .7, 1}, {.74, 0, .26, 1}}},
	{"header, body, button bar", [][4]float64{{0, 0, 1, .2}, {0, .24, 1, .54}, {0, .82, 1, .18}}},
	{"one big, two small", [][4]float64{{0, 0, .6, 1}, {.64, 0, .36, .46}, {.64, .54, .36, .46}}},
	{"comic strip", [][4]float64{{0, 0, .31, .72}, {.345, 0, .31, .72}, {.69, 0, .31, .72}, {0, .78, 1, .22}}},
	{"a card in the middle", [][4]float64{{.18, .12, .64, .76}}},
}

// codexSlotRects are the slots of layout li in a w×h area, in cells.
func codexSlotRects(li, w, h int) [][4]int {
	lay := codexLayouts[(li%16+16)%16]
	out := make([][4]int, len(lay.slots))
	for i, s := range lay.slots {
		x0 := int(math.Round(s[0] * float64(w)))
		y0 := int(math.Round(s[1] * float64(h)))
		x1 := int(math.Round((s[0] + s[2]) * float64(w)))
		y1 := int(math.Round((s[1] + s[3]) * float64(h)))
		out[i] = [4]int{x0, y0, max(x1-x0, 1), max(y1-y0, 1)}
	}
	return out
}

// codexSlotOf is the slot e sits in on a page of layout li (one past the last: the last).
func codexSlotOf(e *codexEl, li int) int {
	n := len(codexLayouts[(li%16+16)%16].slots)
	return min(max(e.Slot, 0), n-1)
}

// ---- files ------------------------------------------------------------------------------

func codexRoot() string { return filepath.Join(filepath.Dir(sessionDir()), "codex") }

func codexDir(id string) string { return filepath.Join(codexRoot(), id) }

func codexFilePath(id string) string { return filepath.Join(codexDir(id), "codex.json") }

// codexFileV is the codex file's version. 2: the old colours are moved to
// the theme's (once; a colour chosen after that is kept). 3: every text and
// button is centred (once; the user asked, 17 Nov 2026 — J can still line
// one up left or right after that).
const codexFileV = 3

func newCodexFile(title string) *codexFile {
	return &codexFile{
		V: codexFileV, Title: title, Symbol: 3,
		Pages: []*codexPage{{Layout: 1}}, NextID: 1,
	}
}

func loadCodex(id string) (*codexFile, error) {
	b, err := os.ReadFile(codexFilePath(id))
	if err != nil {
		return nil, err
	}
	var cf codexFile
	if err := json.Unmarshal(b, &cf); err != nil {
		return nil, err
	}
	cf.tidy()
	return &cf, nil
}

// tidy makes a file read from disk safe to use.
func (cf *codexFile) tidy() {
	// The first look's colours (cream paper, a red cover, blue buttons) are
	// the theme's now: Emberistal (16 Nov 2026). Only once, for files from
	// before (version under 2): cream paper chosen later must stay.
	old := cf.V < 2
	if old && cf.Color == "#7a2e1d" {
		cf.Color = ""
	}
	if old && cf.Paper == "#f4ead5" {
		cf.Paper = ""
	}
	if len(cf.Pages) == 0 {
		cf.Pages = []*codexPage{{Layout: 1}}
	}
	if cf.V < 3 { // all words in the middle
		for _, p := range cf.Pages {
			for _, e := range p.Els {
				if e != nil && (e.Kind == ceText || e.Kind == ceButton) {
					e.Align = 0
				}
			}
		}
	}
	top := 0
	for i, p := range cf.Pages {
		if p == nil {
			cf.Pages[i] = &codexPage{Layout: 1}
			continue
		}
		keep := p.Els[:0]
		for _, e := range p.Els {
			if old && e != nil && e.Kind == ceButton && e.Color == "#2c7fb8" {
				e.Color = ""
			}
			if e == nil {
				continue
			}
			if _, ok := codexKindNames[e.Kind]; !ok {
				e.Kind = ceText
			}
			e.Font = ((e.Font % 16) + 16) % 16
			e.Anim = ((e.Anim % 16) + 16) % 16
			e.Align = ((e.Align % 3) + 3) % 3
			e.VAlign = ((e.VAlign % 3) + 3) % 3
			e.Size = max(0, min(e.Size, codexSizes))
			e.Head = max(0, min(e.Head, 3))
			e.W = max(0, min(e.W, 100))
			e.H = max(0, min(e.H, 100))
			e.FillBy = ((e.FillBy % len(codexFills)) + len(codexFills)) % len(codexFills)
			if n := len(codexSymbols); n > 0 {
				e.Symbol = ((e.Symbol % n) + n) % n
			}
			keep = append(keep, e)
			top = max(top, e.ID)
		}
		p.Els = keep
		p.Layout = ((p.Layout % len(codexLayouts)) + len(codexLayouts)) % len(codexLayouts)
		p.Turn = ((p.Turn % 16) + 16) % 16
		p.Border = max(-1, min(p.Border, len(codexPageBorders)))
	}
	cf.CoverTurn = max(0, min(cf.CoverTurn, 16))
	cf.PageBorder = max(0, min(cf.PageBorder, len(codexPageBorders)))
	cf.CoverBorder = max(0, min(cf.CoverBorder, codexCoverNone))
	cf.TitleFont = ((cf.TitleFont % 16) + 16) % 16
	cf.tidyCover() // codex_cover.go
	cf.HFFont = ((cf.HFFont % 16) + 16) % 16
	if n := len(codexSymbols); n > 0 {
		cf.Symbol = ((cf.Symbol % n) + n) % n
	}
	cf.NextID = max(cf.NextID, top+1)
	cf.V = max(cf.V, codexFileV)
}

func saveCodex(id string, cf *codexFile) error {
	if err := os.MkdirAll(codexDir(id), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cf, "", " ")
	if err != nil {
		return err
	}
	tmp := codexFilePath(id) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	codexPeekDrop(id)
	return os.Rename(tmp, codexFilePath(id))
}

// newCodexID is a fresh folder name: the time, and a number if taken.
func newCodexID() string {
	base := "codex-" + time.Now().Format("20060102-150405")
	id := base
	for n := 2; ; n++ {
		if _, err := os.Stat(codexDir(id)); os.IsNotExist(err) {
			return id
		}
		id = base + "-" + itoa(n)
	}
}

// createCodex makes a new codex on disk and gives its id.
func createCodex(title string) (string, *codexFile, error) {
	id := newCodexID()
	cf := newCodexFile(title)
	return id, cf, saveCodex(id, cf)
}

type codexEntry struct {
	id    string
	title string
	at    time.Time
	pages int
}

// codexList is every codex on disk, the newest change first.
func codexList() []codexEntry {
	ents, _ := os.ReadDir(codexRoot())
	var out []codexEntry
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		info, err := os.Stat(codexFilePath(e.Name()))
		if err != nil {
			continue
		}
		cf, err := codexPeek(e.Name())
		if err != nil {
			continue
		}
		out = append(out, codexEntry{e.Name(), cf.Title, info.ModTime(), len(cf.Pages)})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].at.After(out[j].at) })
	return out
}

// codexPeek reads a codex for showing its cover (the Design board, the
// library), kept (by its file's path) until the file changes.
var codexPeeks = map[string]codexPeekEntry{}

type codexPeekEntry struct {
	at time.Time
	cf *codexFile
}

func codexPeek(id string) (*codexFile, error) {
	info, err := os.Stat(codexFilePath(id))
	if err != nil {
		return nil, err
	}
	if p, ok := codexPeeks[codexFilePath(id)]; ok && p.at.Equal(info.ModTime()) {
		return p.cf, nil
	}
	cf, err := loadCodex(id)
	if err != nil {
		return nil, err
	}
	codexPeeks[codexFilePath(id)] = codexPeekEntry{info.ModTime(), cf}
	return cf, nil
}

func codexPeekDrop(id string) { delete(codexPeeks, codexFilePath(id)) }

// codexStamp says when the codex last changed (for redrawing its cover).
func codexStamp(id string) string {
	info, err := os.Stat(codexFilePath(id))
	if err != nil {
		return "gone"
	}
	return fmt.Sprint(info.ModTime().UnixNano())
}

// ---- the mode's state ---------------------------------------------------------------------

type codexElState struct {
	on   bool
	at   time.Time // when it was last activated
	seen bool      // shown: a settling animation has started
}

type codexState struct {
	lib    []codexEntry
	libSel int

	id      string
	cf      *codexFile
	page    int // -1 the cover, else a page
	edit    bool
	sel     int // edit mode: the chosen thing's ID (0: none)
	focus   int // reading: the thing Tab reached (0: none)
	states  map[int]*codexElState
	pageAt  time.Time  // when this page came up: the animations' clock
	turn    *codexTurn // a page turning
	placing bool       // Alt+N: the next key says what to add

	writing   int    // what is being written: 0 nothing, codexWrite…
	words     []rune // the words being written
	cur       int    // the cursor in them
	drawing   bool   // drawing in the chosen drawing
	brushX    int    // the brush, in the drawing's dots
	brushY    int
	brushC    int8 // its colour (codexInkColors), -1 the rubber
	brushSize int
	penDown   bool

	table              bool // the activation table is open
	tableRow, tableCol int

	added     int          // the thing Alt+N just added, while it is first written or drawn (0: none)
	addedUndo int          // how long undo was just after adding it (nothing changed since: equal)
	addedFrom int          // what was chosen before it was added
	fresh     bool         // a new button's label: the first key typed replaces it
	newPic    *codexNewPic // Alt+N, I: where the picture goes, once one is chosen

	undo, redo []string // the codex as it was, as JSON
	saved      string   // the codex as last saved, as JSON
	xAt        time.Time
	xWhole     bool // the first X was Shift+X: the whole page
	err        string

	fromDesign bool // opened from the Design board: Esc goes back there
	designID   int  // the structure it was opened from

	shadeKey string // what the no-shader note says (codex_shade.go)
	shadeTop int    // 0 not known, 1 sy's pane is not the top of its window, 2 it is

	tick   bool              // a frame tick is coming
	rects  map[int][4]int    // where each thing was drawn last, on the screen: x, y, w, h
	inner  [4]int            // the page's writing area on the screen
	slots  map[int][][4]int  // the page's slots on the screen, by page
	box    map[string][4]int // other click spots: "cover", "prev", "next"
	place  map[int][4]int    // each thing's place (its slot, inside its border): what its size is a share of
	sizing *codexSizing      // the mouse is dragging a thing's corner (codex_size.go)

	coverPart int             // the cover's chosen part in edit mode: cpWhole … cpCount (codex_cover.go)
	coverInfo *codexCoverInfo // where the cover's parts were drawn last

	pageRect [4]int     // the page (or cover) on the screen, as last drawn
	pick     *codexPick // the font list (F): open while choosing (codex_look.go)
}

// codexNewPic is where a new picture goes: it is only added once a picture is chosen.
type codexNewPic struct {
	slot, at, from int
}

const (
	codexWriteEl = 1 + iota
	codexWriteTitle
	codexWriteSubtitle
	codexWriteAlt
	codexWriteHeader
	codexWriteFooter
)

type codexTurn struct {
	from  *boardCanvas // the view before
	at    time.Time
	ahead bool
	anim  int    // which of the 16 turns (codex_turns.go)
	rect  [4]int // the page on the screen: the turn happens there
}

func (m *Model) cx() *codexState {
	if m.codex == nil {
		m.codex = &codexState{page: -1, states: map[int]*codexElState{}, brushSize: 1}
	}
	return m.codex
}

// enterCodex: the mode starts (or comes back): the library, or the open codex.
func (m *Model) enterCodex() tea.Cmd {
	c := m.cx()
	c.lib = codexList()
	c.libSel = min(c.libSel, max(len(c.lib)-1, 0))
	if c.cf == nil {
		m.say(modes[codexMode].Icon() + "  Codex mode. Enter opens a codex · Alt+N makes one · F1: all keys.")
	}
	return m.codexTickCmd()
}

// codexOpen opens codex id: its cover first (edit: on to its first page, edit mode).
func (m *Model) codexOpen(id string, edit bool) bool {
	cf, err := loadCodex(id)
	if err != nil {
		m.sayErr("Could not open that codex: " + err.Error())
		return false
	}
	c := m.cx()
	c.id, c.cf, c.edit = id, cf, edit
	b, _ := json.Marshal(cf)
	c.saved = string(b)
	c.page, c.sel, c.focus = -1, 0, 0
	c.undo, c.redo = nil, nil
	c.writing, c.drawing, c.table, c.placing = 0, false, false, false
	c.added, c.fresh, c.newPic = 0, false, nil
	c.turn = nil
	m.codexShowPage(-1)
	if edit {
		m.say("Editing “" + cf.Title + "”. Enter writes the title · PgDn: the pages · E: read it · Esc: close · F1: all keys.")
	} else {
		m.say("“" + cf.Title + "”. → opens it · ← → turn pages · Tab · Enter: buttons · E: edit · Esc: close.")
	}
	return true
}

// codexClose closes the open codex: back to the Design board if it came
// from there, else to the library.
func (m *Model) codexClose() tea.Cmd {
	c := m.cx()
	m.codexLeave()
	id, title := c.id, ""
	if c.cf != nil {
		title = c.cf.Title
	}
	// A codex made and left empty (no title, no subtitle, nothing on any
	// page) is not kept, unless the Design board shows it.
	dropped := c.cf != nil && codexEmpty(c.cf) && !c.fromDesign && !m.codexOnBoard(id)
	if dropped {
		if err := os.RemoveAll(codexDir(id)); err != nil {
			dropped = false
		}
		codexPeekDrop(id)
	}
	if !dropped && c.cf != nil {
		m.codexHome(id, c.cf) // its room and lexicon door on Design (codex_design.go)
	}
	c.cf, c.id = nil, ""
	c.lib = codexList()
	for i, e := range c.lib {
		if e.id == id {
			c.libSel = i
		}
	}
	if c.fromDesign {
		c.fromDesign = false
		cmd := m.setMode(designMode)
		d := m.ds()
		if o := d.byID(c.designID); o != nil && o.Kind == kCodex {
			o.Header = title
			d.sel = o.ID
			m.centreOn(o)
		}
		d.dirty = true
		d.save()
		m.say("Back on the board.")
		return cmd
	}
	if dropped {
		m.say("Closed. It was empty, so it was not kept. Alt+N makes a codex.")
		return nil
	}
	m.say("Closed. Enter opens a codex · Alt+N makes one.")
	return nil
}

// codexEmpty: no title, no subtitle and nothing on any page.
func codexEmpty(cf *codexFile) bool {
	if strings.TrimSpace(cf.Title) != "" || strings.TrimSpace(cf.Subtitle) != "" {
		return false
	}
	for _, p := range cf.Pages {
		if p != nil && len(p.Els) > 0 {
			return false
		}
	}
	return true
}

// codexLeave keeps everything as it is when the codex is left (closed,
// another mode, sy quits): words being written are kept, a drawing ends,
// and it is saved.
func (m *Model) codexLeave() {
	c := m.codex
	if c == nil || c.cf == nil {
		return
	}
	if c.writing != 0 {
		m.codexEndWrite()
	}
	if c.drawing {
		m.codexEndDraw()
	}
	c.placing, c.newPic = false, nil
	m.codexSave()
}

func (m *Model) codexSave() {
	c := m.cx()
	if c.cf == nil || c.id == "" {
		return
	}
	b, _ := json.Marshal(c.cf)
	if string(b) == c.saved { // nothing changed: reading must not make it "changed now"
		return
	}
	if err := saveCodex(c.id, c.cf); err != nil {
		c.err = err.Error()
		m.sayErr("Not saved: " + c.err)
		return
	}
	c.saved, c.err = string(b), ""
}

// codexStep notes the codex as it is, before a change, for Ctrl+Z.
func (m *Model) codexStep() {
	c := m.cx()
	if c.cf == nil {
		return
	}
	b, _ := json.Marshal(c.cf)
	c.codexPushUndo(string(b))
}

// codexPushUndo notes was (the codex as JSON, before a change) for Ctrl+Z.
func (c *codexState) codexPushUndo(was string) {
	c.undo = append(c.undo, was)
	if len(c.undo) > 200 {
		c.undo = c.undo[1:]
	}
	c.redo = nil
}

func (m *Model) codexUndo(redo bool) {
	c := m.cx()
	from, to := &c.undo, &c.redo
	if redo {
		from, to = &c.redo, &c.undo
	}
	if len(*from) == 0 || c.cf == nil {
		m.say(map[bool]string{false: "Nothing to undo.", true: "Nothing to redo."}[redo])
		return
	}
	now, _ := json.Marshal(c.cf)
	*to = append(*to, string(now))
	last := (*from)[len(*from)-1]
	*from = (*from)[:len(*from)-1]
	var cf codexFile
	if json.Unmarshal([]byte(last), &cf) == nil {
		cf.tidy()
		c.cf = &cf
	}
	c.page = min(c.page, len(c.cf.Pages)-1)
	if c.el(c.sel) == nil {
		c.sel = 0
		if p := c.pageOf(); p != nil && len(p.Els) > 0 {
			c.sel = p.Els[len(p.Els)-1].ID
		}
	}
	if e := c.el(c.sel); c.drawing && (e == nil || e.Kind != ceDrawing) {
		c.drawing, c.penDown = false, false // what was being drawn is gone
	}
	c.added, c.newPic = 0, nil // undo decides what is there now
	m.codexSave()
	m.say(map[bool]string{false: "Undone.", true: "Redone."}[redo])
}

// ---- pages and things -------------------------------------------------------------------

func (c *codexState) pageOf() *codexPage {
	if c.cf == nil || c.page < 0 || c.page >= len(c.cf.Pages) {
		return nil
	}
	return c.cf.Pages[c.page]
}

// el is the thing with this ID on the open page, or nil.
func (c *codexState) el(id int) *codexEl {
	p := c.pageOf()
	if p == nil || id == 0 {
		return nil
	}
	for _, e := range p.Els {
		if e.ID == id {
			return e
		}
	}
	return nil
}

// find is the thing with this ID anywhere in the codex, and its page.
func (c *codexState) find(id int) (*codexEl, int) {
	if c.cf == nil {
		return nil, -1
	}
	for pi, p := range c.cf.Pages {
		for _, e := range p.Els {
			if e.ID == id {
				return e, pi
			}
		}
	}
	return nil, -1
}

func (c *codexState) state(id int) *codexElState {
	s := c.states[id]
	if s == nil {
		s = &codexElState{}
		c.states[id] = s
	}
	return s
}

// codexShowPage turns to page i (-1 the cover): the things start as they
// were made, and the animations start again.
func (m *Model) codexShowPage(i int) {
	c := m.cx()
	if c.cf == nil {
		return
	}
	i = max(-1, min(i, len(c.cf.Pages)-1))
	c.page = i
	c.pageAt = time.Now()
	c.states = map[int]*codexElState{}
	c.focus = 0
	if c.edit && c.el(c.sel) == nil {
		c.sel = 0
		if p := c.pageOf(); p != nil && len(p.Els) > 0 {
			c.sel = p.Els[0].ID
		}
	}
}

// codexTurns: pages turn with a slide (tests turn it off, so a page is
// there at once).
var codexTurns = true

// codexTurnTo turns the page with a slide (ahead: to a later page).
func (m *Model) codexTurnTo(i int) {
	c := m.cx()
	if c.cf == nil {
		return
	}
	i = max(-1, min(i, len(c.cf.Pages)-1))
	if i == c.page {
		return
	}
	if !codexTurns {
		m.codexShowPage(i)
		return
	}
	from, rect := m.codexCanvas(), c.pageRect
	anim := c.cf.turnBetween(c.page, i)
	c.turn = &codexTurn{from: from, at: time.Now(), ahead: i > c.page, anim: anim, rect: rect}
	m.codexShowPage(i)
}

func (c *codexState) newEl(kind string, slot int) *codexEl {
	e := &codexEl{ID: c.cf.NextID, Kind: kind, Slot: slot}
	c.cf.NextID++
	switch kind {
	case ceText:
		e.Text = "" // in the middle, like everything in a codex (J lines it up left or right)
	case ceButton:
		e.Text = "Next"
		e.Action = caNext
	case ceSymbol:
		e.Symbol = int(e.ID) % max(len(codexSymbols), 1)
		e.Size = 2
	}
	return e
}

// elsIn are the things in slot s of page p, in order.
func elsIn(p *codexPage, s int) []*codexEl {
	var out []*codexEl
	for _, e := range p.Els {
		if codexSlotOf(e, p.Layout) == s {
			out = append(out, e)
		}
	}
	return out
}

// interactive are the things Tab steps through while reading: buttons, and
// anything whose activation does something.
func (c *codexState) interactive() []*codexEl {
	p := c.pageOf()
	if p == nil {
		return nil
	}
	var out []*codexEl
	now := time.Now()
	for _, e := range p.Els {
		if !c.edit && c.look(e, now).hidden { // a hidden thing must not give itself away
			continue
		}
		if e.Kind == ceButton || e.On.Do != "" {
			out = append(out, e)
		}
	}
	return out
}

// ---- activating ---------------------------------------------------------------------------

// codexActivate activates thing e: a button does its action; anything else
// changes as its activation table row says (again: back).
func (m *Model) codexActivate(e *codexEl, depth int) {
	c := m.cx()
	if e == nil || depth > 8 {
		return
	}
	s := c.state(e.ID)
	if e.Kind == ceButton && !(depth > 0 && e.On.Do != "") { // aimed at: a button with an effect only changes
		s.at = time.Now() // it lights up as pressed
		switch e.Action {
		case caNext, "":
			if c.page+1 < len(c.cf.Pages) {
				m.codexTurnTo(c.page + 1)
			} else if c.edit {
				m.say("The last page. Alt+P adds a page.")
			} else {
				m.say("The last page.")
			}
		case caPrev:
			m.codexTurnTo(c.page - 1)
		case caGoto:
			if e.Target >= 1 && e.Target <= len(c.cf.Pages) {
				m.codexTurnTo(e.Target - 1)
			} else {
				m.codexTurnTo(-1)
			}
		case caActivate:
			t, pi := c.find(e.Target)
			if t == nil {
				m.say("That button is not aimed at anything yet (edit mode: G).")
				return
			}
			if pi != c.page {
				m.codexTurnTo(pi)
			}
			m.codexActivate(t, depth+1)
		}
		if e.On.Do != "" { // a button can change too
			s.on = !s.on
		}
		return
	}
	if e.On.Do == "" {
		return
	}
	s.on = !s.on
	s.at = time.Now()
	if e.On.Do == "flash" || c.edit {
		s.on = true // a flash plays each time; editing, trying shows the change each time
	}
}

// look is how thing e shows just now: its own look, changed by its
// activation if it is on. hidden: not drawn at all.
type codexLook struct {
	text   string
	font   int
	size   int
	color  string
	anim   int
	animAt time.Time
	image  string
	symbol int
	invert bool
	hidden bool
	ghost  bool    // editing: hidden until activated, so drawn faint
	flash  float64 // 0 … 1: how bright a flash is just now
}

func (c *codexState) look(e *codexEl, now time.Time) codexLook {
	l := codexLook{text: e.Text, font: e.Font, size: max(e.Size, 1), color: e.Color, anim: e.Anim,
		animAt: c.pageAt, image: e.Image, symbol: e.Symbol}
	s := c.states[e.ID]
	on := s != nil && s.on
	if c.edit && c.writing == 0 && !c.table { // edit mode shows things as made (Space tries one)
		on = s != nil && s.on && now.Sub(s.at) < 3*time.Second
	}
	switch e.On.Do {
	case "show":
		l.hidden = !on
		if on && s != nil {
			l.animAt = s.at // its entrance animation plays as it appears
		}
	case "hide":
		l.hidden = on
	case "animate":
		l.anim = 0
		if on {
			l.anim, l.animAt = max(e.On.Anim, 1), s.at
		}
	case "colour":
		if on {
			l.color = e.On.Color
		}
	case "font":
		if on {
			l.font = e.On.Font
		}
	case "grow":
		if on {
			l.size = max(e.On.Size, 1)
		}
	case "swap":
		if on {
			switch e.Kind {
			case ceImage:
				l.image = e.On.Alt
			case ceSymbol:
				fmt.Sscan(e.On.Alt, &l.symbol)
			default:
				l.text = e.On.Alt
			}
		}
	case "invert":
		l.invert = on
	case "flash":
		if s != nil && s.on {
			if d := now.Sub(s.at).Seconds(); d < 0.9 {
				l.flash = 1 - d/0.9
			}
		}
	}
	if e.Kind == ceButton && s != nil && now.Sub(s.at) < 300*time.Millisecond {
		l.flash = max(l.flash, 1-now.Sub(s.at).Seconds()/0.3)
	}
	if c.edit && l.hidden {
		l.hidden, l.ghost = false, true // edit mode shows everything, the hidden faint
	}
	return l
}

// moving is whether anything on the page is moving just now (so frames keep coming).
func (c *codexState) moving(now time.Time) bool {
	if c.cf == nil {
		return false
	}
	if c.turn != nil || c.page == -1 || c.placing {
		return true // the cover's symbol moves; the add menu's symbol too
	}
	if e := c.el(c.sel); c.edit && e != nil && (e.Anim > 0 || e.Kind == ceSymbol) {
		return true // the inspector plays its animation
	}
	p := c.pageOf()
	if p == nil {
		return false
	}
	for _, e := range p.Els {
		l := c.look(e, now)
		if l.hidden {
			continue
		}
		if e.Kind == ceSymbol || l.flash > 0 {
			return true
		}
		if l.anim > 0 && (codexAnimLoops(l.anim) || !codexAnimDone(l.anim, 40, 10, now.Sub(l.animAt).Seconds())) {
			return true
		}
		if s := c.states[e.ID]; s != nil && now.Sub(s.at) < time.Second {
			return true
		}
		if s := c.states[e.ID]; c.edit && s != nil && s.on && now.Sub(s.at) < 3200*time.Millisecond {
			return true // a tried change goes back after 3 s
		}
	}
	return false
}

// ---- frames -------------------------------------------------------------------------------

type codexTickMsg struct{}

const codexFrame = 80 * time.Millisecond

func (m *Model) codexTickCmd() tea.Cmd {
	c := m.cx()
	if !m.inCodex() || c.tick {
		return nil
	}
	if !c.moving(time.Now()) && !(c.cf == nil && len(c.lib) > 0) {
		return nil
	}
	c.tick = true
	frame := codexFrame
	if c.turn != nil {
		frame = codexTurnFrame // a turn: smoother
	}
	return tea.Tick(frame, func(time.Time) tea.Msg { return codexTickMsg{} })
}

func (m *Model) codexTicked() tea.Cmd {
	c := m.cx()
	c.tick = false
	if c.turn != nil && time.Since(c.turn.at) > codexTurnDur(c.turn.anim) {
		c.turn = nil
	}
	return m.codexTickCmd()
}

// codexPagesMoved keeps "go to page" buttons on their page when a page is
// added at index at (delta 1) or the page at index at is deleted (delta -1).
// A button aimed at a deleted page aims at the page that took its place.
func (cf *codexFile) codexPagesMoved(at, delta int) {
	for _, p := range cf.Pages {
		for _, e := range p.Els {
			if e.Kind != ceButton || e.Action != caGoto || e.Target < 1 {
				continue
			}
			switch {
			case delta > 0 && e.Target >= at+1:
				e.Target++
			case delta < 0 && e.Target > at+1:
				e.Target--
			}
			e.Target = min(e.Target, len(cf.Pages))
		}
	}
}
