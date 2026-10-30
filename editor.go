package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"symbolizer/emmet"
)

const tabWidth = 4

type promptKind int

const (
	pNone promptKind = iota
	pFind
	pReplaceFind
	pReplaceWith
	pReplaceAsk
	pGoto
	pOpen
	pSaveAs
	pConfirm
	pName      // Ctrl+S on a session: what to call it
	pRename    // file manager
	pNewFile   // file manager
	pNewFolder // file manager
	pWrap      // Emmet: wrap with an abbreviation (web.go)
	pUpdateTag // Emmet: update the tag at the cursor
)

// Model is the whole editor, as Bubble Tea wants it.
type Model struct {
	buf    *Buffer
	hl     *Highlighter
	width  int
	height int
	top    int // first line on screen
	left   int // first screen column on screen (for long lines)

	msg    string
	msgErr bool

	clip     string // last copied text
	clipLine bool   // it was a whole line copied with nothing selected

	prompt      promptKind
	input       textinput.Model
	confirmText string
	confirmKeys string
	onConfirm   func(m *Model, key rune) tea.Cmd

	query       string // what Find looks for
	showMatches bool
	findFrom    Pos
	replaceWith string

	help    bool
	helpTop int

	chord    chordState
	releases bool    // the terminal tells us when keys are let go (kitty does)
	keys     keyMode // which of Shift / Ctrl / Alt is held (keymode.go)

	dragging   bool
	lastClick  time.Time
	clickCount int
	lastClickP Pos

	quitClean  bool
	afterSpace bool // the last letter typed was a space

	// saving (autosave.go)
	saving      bool   // a save is being written now
	saveWaiting bool   // a save will start in a moment
	savePaused  bool   // another program changed the file; not saving over it
	saveErr     string // why the last save failed
	nameExt     string // file ending for Ctrl+S naming (.cpp, .c, ...)
	nameSuggest string // the name Ctrl+S offers for a session

	// open files (tabs.go)
	tabs    []*tab
	cur     int
	tabHits [][3]int

	// file manager (tree.go)
	tree       *fileTree
	showTree   bool
	treeFocus  bool
	treeTarget string

	picker *picker // the Alt+L list

	// tab switch mode (tabmode.go)
	tabMode     bool
	tabModeFrom int

	// what was last written to the last-open file (lastopen.go)
	lastOpen struct {
		text, files string
		at          time.Time
	}

	// Modes (modes.go)
	mode      int
	modesOpen bool
	modeSel   int
	weather   *weatherData // the info line's weather (statusinfo.go)

	// Colour switcher (themes.go)
	themeOpen   bool
	themeSel    int
	themeBefore int

	// Central zone (zone.go)
	zoneWidth  int    // letters across; 0 = the default
	dragMargin string // "left" or "right" while a margin is being dragged
	fmBox      [4]int // the floating file manager: x, y, width, height (fileview.go)
	fmListTop  int
	fmListH    int
	altGr      altGrState   // AltGr tapped twice opens the file manager (altgr.go)
	emblem     emblem       // made fresh each start (emblem.go)
	lamps      string       // lock-key state (lamps.go)
	plate      *lockPlate   // the lock plate while it shows (lockplate.go)
	blurred    bool         // the terminal says another window has the keyboard
	diary      *diaryState  // diary mode: chapters, pages, the page in focus (diary.go)
	startDir   string       // folder sy was started in
	snip       snippetStops // Tab's stops after an Emmet expansion (web.go)
}

func NewModel(path string, line int) (*Model, error) {
	m := &Model{width: 80, height: 24, lamps: readLampsFile()}
	m.chord.held = map[string]bool{}
	settings := loadSettings()
	applyTheme(themeIndex(settings["theme"]))
	m.zoneWidth, _ = strconv.Atoi(settings["zone-width"])
	m.input = newInput()
	m.startDir, _ = os.Getwd()
	m.emblem = newEmblem()
	buf := newDraft()
	switch {
	case path == "":
		// No file named: carry on with today's session, or start a new one.
		if s, ok := currentSession(); ok {
			if err := buf.Load(s.Path); err == nil {
				buf.Draft = true
				buf.Cursor = Pos{len(buf.Lines) - 1, len(buf.Lines[len(buf.Lines)-1])}
				m.say("Carrying on with the " + sessionLabel(s.Path) + ". Ctrl+N starts a new one. Alt+L lists them all.")
			}
		} else {
			m.say("New session. It saves by itself. Ctrl+S gives it a name.")
		}
	default:
		abs, _ := filepath.Abs(path)
		info, err := os.Stat(abs)
		switch {
		case err == nil && info.IsDir():
			m.setRoot(abs)
			m.showTree, m.treeFocus = true, true
			m.say("Project " + filepath.Base(abs) + ". Choose a file on the left, or type to find one.")
		case err == nil:
			buf = NewBuffer()
			if err := buf.Load(abs); err != nil {
				return nil, err
			}
			buf.Draft = isSession(abs)
		case os.IsNotExist(err):
			buf = NewBuffer()
			buf.Path = abs
			m.say("New file. It is saved as you type.")
		default:
			return nil, err
		}
	}
	m.addTab(buf)
	if line > 0 {
		m.buf.Cursor = m.buf.Clamp(Pos{line - 1, 0})
	}
	return m, nil
}

func newInput() textinput.Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.SetVirtualCursor(false)
	st := inputStyles(ti.Styles())
	st.Cursor.Shape = tea.CursorBar
	ti.SetStyles(st)
	return ti
}

func (m *Model) resetHighlighter() {
	sample := ""
	if len(m.buf.Lines) > 0 {
		var sb strings.Builder
		for i := 0; i < len(m.buf.Lines) && sb.Len() < 4096; i++ {
			sb.WriteString(string(m.buf.Lines[i]))
			sb.WriteByte('\n')
		}
		sample = sb.String()
	}
	path := m.buf.Path
	if m.buf.Draft {
		path = "" // sessions are coloured by what they look like
	}
	m.hl = NewHighlighter(path, sample)
	m.buf.OnSplice = m.hl.Splice
	if m.cur < len(m.tabs) {
		m.tabs[m.cur].hl = m.hl
	}
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.hl.Want(m.buf.Version(), m.top, m.textHeight()), nextMinute(), loadWeather(), weatherEvery(), m.startLamps())
}

func (m *Model) say(s string)    { m.msg, m.msgErr = s, false }
func (m *Model) sayErr(s string) { m.msg, m.msgErr = s, true }
func (m *Model) textHeight() int { return max(m.height-5, 1) } // top panel, tab bar, info line, status bar, message line

// textTop is the screen row where the text starts (under the top panel and
// the tab bar).
const textTop = 2

func (m *Model) line(r int) []rune { return m.buf.Lines[r] }

// ---- Update ------------------------------------------------------------------

func (m *Model) Update(msg tea.Msg) (model tea.Model, cmd tea.Cmd) {
	defer func() {
		if r := recover(); r != nil {
			rescue(m.buf)
			panic(r)
		}
	}()
	version := m.buf.Version()
	cmd = m.update(msg)
	if m.buf.Version() != version {
		m.showMatches = m.prompt == pFind || m.prompt == pReplaceAsk
	}
	if want := m.hl.Want(m.buf.Version(), m.top, m.textHeight()); want != nil {
		cmd = tea.Batch(cmd, want)
	}
	if save := m.wantSave(); save != nil {
		cmd = tea.Batch(cmd, save)
	}
	m.noteLastOpen()
	return m, cmd
}

func (m *Model) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.input.SetWidth(m.inputWidth())
		m.ensureVisible()
		return nil
	case tea.KeyboardEnhancementsMsg:
		m.releases = msg.SupportsEventTypes()
		return nil
	case highlightTick:
		return m.hl.Start(m.buf, m.top, m.textHeight())
	case highlightMsg:
		m.hl.Finish(msg)
		return nil
	case saveTick:
		return m.startSave()
	case lampsMsg:
		return m.gotLamps(string(msg))
	case plateMsg:
		return m.showPlate(string(msg))
	case plateFrame:
		return m.plateStep(int(msg))
	case tea.FocusMsg:
		m.blurred = false
		return nil
	case tea.BlurMsg:
		m.blurred = true
		m.keys = keyMode{} // keys let go in another window are never reported
		return nil
	case minuteTick:
		return nextMinute() // draw again: the time in words moves on
	case weatherTick:
		return tea.Batch(loadWeather(), weatherEvery())
	case weatherMsg:
		if msg.w != nil {
			m.weather = msg.w
		}
		return nil
	case saveDone:
		m.finishSave(msg)
		return nil
	case pasteMsg:
		return m.pasteText(string(msg))
	case clipErrMsg:
		m.sayErr(string(msg))
		return nil
	case tea.PasteMsg:
		if m.prompt != pNone && m.prompt != pConfirm && m.prompt != pReplaceAsk {
			var c tea.Cmd
			m.input, c = m.input.Update(msg)
			return tea.Batch(c, m.promptChanged())
		}
		return m.pasteText(msg.Content)
	case tea.KeyReleaseMsg:
		if cmd, done := m.altGrRelease(msg.Key()); done {
			return cmd
		}
		m.arrowRelease(msg.Key())
		m.keys.release(msg.Key(), m.altGr.down)
		return nil
	case tea.KeyPressMsg:
		m.keys.press(msg.Key(), m.altGr.down)
		if m.altGrPress(msg.Key()) || isModifierOnly(msg.Key()) {
			return nil // a modifier on its own does nothing (and keeps the message line)
		}
		if m.tabMode {
			if cmd, done := m.tabModeKey(msg.Key()); done {
				return cmd
			}
		}
		if keyName(msg.Key()) == "alt+q" {
			m.help = false
			m.closePrompt()
			return m.quit()
		}
		if keyName(msg.Key()) == "alt+m" {
			m.themeOpen = false
			return m.openModes()
		}
		if n := keyName(msg.Key()); n == "ŧ" || n == "alt+t" {
			return m.openThemes()
		}
		if m.themeOpen {
			return m.themeKey(keyName(msg.Key()))
		}
		if m.modesOpen {
			return m.modesKey(keyName(msg.Key()), msg.Key().Text)
		}
		if m.inDiary() && !(m.help && m.diaryViewer()) {
			if cmd, done := m.diaryKey(keyName(msg.Key())); done {
				return cmd
			}
		}
		if m.help && m.diaryViewer() {
			if keyName(msg.Key()) == "f1" || keyName(msg.Key()) == "esc" {
				m.help = false
				return nil
			}
			return m.helpKey(keyName(msg.Key()))
		}
		if !modes[m.mode].ready {
			if keyName(msg.Key()) == "f1" {
				m.help = !m.help
			}
			return nil
		}
		if m.help {
			return m.helpKey(keyName(msg.Key()))
		}
		if m.prompt != pNone {
			return m.promptKey(msg)
		}
		m.msg = ""
		if m.picker != nil {
			return m.pickerKey(keyName(msg.Key()))
		}
		if m.treeFocus && m.showTree && m.tree != nil {
			return m.treeKey(msg.Key())
		}
		return m.editKey(msg.Key())
	case tea.MouseClickMsg:
		if m.diaryViewer() && msg.Mouse().Y != 0 && !m.help {
			return nil // the page viewer has nothing to click but the top panel
		}
		return m.mouseClick(msg.Mouse())
	case tea.MouseMotionMsg:
		if m.dragMargin != "" {
			m.marginDrag(msg.Mouse().X)
			return nil
		}
		return m.mouseDrag(msg.Mouse())
	case tea.MouseReleaseMsg:
		if m.dragMargin != "" {
			return m.marginRelease()
		}
		m.dragging = false
		if m.buf.Anchor != nil && *m.buf.Anchor == m.buf.Cursor {
			m.buf.ClearSelect()
		}
		return nil
	case tea.MouseWheelMsg:
		n := 3
		if msg.Mouse().Button == tea.MouseWheelUp {
			n = -3
		}
		if m.help {
			m.helpTop = max(0, m.helpTop+n)
			return nil
		}
		if m.diaryViewer() && m.diary != nil {
			m.diary.turn(n / 3) // the wheel turns pages
			return nil
		}
		if m.showTree && m.tree != nil {
			t := m.tree
			t.scroll = max(0, t.scroll+n)
			t.sel = max(t.scroll, min(t.sel, t.scroll+max(m.fmListH-1, 0)))
			return nil
		}
		m.top = max(0, min(m.top+n, len(m.buf.Lines)-1))
		return nil
	}
	if m.prompt != pNone {
		var c tea.Cmd
		m.input, c = m.input.Update(msg)
		return c
	}
	return nil
}

// keyName gives one stable name per key: "ctrl+alt+shift+x".
func keyName(k tea.Key) string {
	if (k.Text == "ß" || k.Text == "ø" || k.Text == "”" || k.Text == "æ") && !k.Mod.Contains(tea.ModCtrl) {
		return k.Text // Right Alt+S / O / N / A on a UK keyboard
	}
	s := k.Keystroke()
	// Some terminals report Alt+Shift+L as "alt+L".
	if i := strings.LastIndexByte(s, '+'); i >= 0 && i < len(s)-1 {
		last := []rune(s[i+1:])
		if len(last) == 1 && unicode.IsUpper(last[0]) {
			pre := s[:i+1]
			if !strings.Contains(pre, "shift+") {
				pre += "shift+"
			}
			s = pre + string(unicode.ToLower(last[0]))
		}
	}
	switch s {
	case "ctrl+_", "ctrl+7":
		return "ctrl+/"
	case "ctrl+h":
		return "backspace"
	}
	return s
}

// ---- keys in the text ------------------------------------------------------------

type command func(m *Model) tea.Cmd

var bindings map[string]command

func init() {
	bindings = map[string]command{
		// moving the cursor
		"up":    func(m *Model) tea.Cmd { m.moveVert(-1, false); return nil },
		"down":  func(m *Model) tea.Cmd { m.moveVert(1, false); return nil },
		"left":  func(m *Model) tea.Cmd { m.moveLeftRight(-1); return nil },
		"right": func(m *Model) tea.Cmd { m.moveLeftRight(1); return nil },
		"home":  func(m *Model) tea.Cmd { m.smartHome(false); return nil },
		"end": func(m *Model) tea.Cmd {
			m.moveTo(Pos{m.buf.Cursor.Row, len(m.line(m.buf.Cursor.Row))}, false)
			return nil
		},
		"pgup":      func(m *Model) tea.Cmd { m.page(-1, false); return nil },
		"pgdown":    func(m *Model) tea.Cmd { m.page(1, false); return nil },
		"ctrl+home": func(m *Model) tea.Cmd { m.moveTo(Pos{}, false); return nil },
		"ctrl+end":  func(m *Model) tea.Cmd { m.moveTo(m.endOfFile(), false); return nil },
		"alt+l":     func(m *Model) tea.Cmd { return m.openLoadList() },
		"alt+o":     func(m *Model) tea.Cmd { return m.openTree(false) },
		"alt+j":     func(m *Model) tea.Cmd { return m.toggleCentre() },
		// On a UK keyboard Right Alt+S types ß and Right Alt+O types ø, so
		// these letters are commands, not text.
		"ß":      func(m *Model) tea.Cmd { return m.save() },
		"ø":      func(m *Model) tea.Cmd { return m.openTree(false) },
		"alt+b":  func(m *Model) tea.Cmd { m.moveTo(Pos{m.buf.Cursor.Row, 0}, false); return nil },
		"alt+e":  func(m *Model) tea.Cmd { m.moveTo(m.endOfFile(), false); return nil },
		"alt+s":  func(m *Model) tea.Cmd { m.moveTo(Pos{}, false); return nil },
		"ctrl+s": func(m *Model) tea.Cmd { return m.save() },

		// highlighting: shift = by word
		"shift+left":  func(m *Model) tea.Cmd { m.moveTo(m.buf.WordLeft(m.buf.Cursor), true); return nil },
		"shift+right": func(m *Model) tea.Cmd { m.moveTo(m.buf.WordRight(m.buf.Cursor), true); return nil },
		"shift+up":    func(m *Model) tea.Cmd { m.moveVert(-1, true); return nil },
		"shift+down":  func(m *Model) tea.Cmd { m.moveVert(1, true); return nil },
		// Ctrl + arrows (cursor one character) are ctrlArrow, below.
		// highlight one character (Up / Down: one line)
		"ctrl+shift+left":  func(m *Model) tea.Cmd { m.moveTo(m.buf.LetterLeft(m.buf.Cursor), true); return nil },
		"ctrl+shift+right": func(m *Model) tea.Cmd { m.moveTo(m.buf.LetterRight(m.buf.Cursor), true); return nil },
		"ctrl+shift+up":    func(m *Model) tea.Cmd { m.moveVert(-1, true); return nil },
		"ctrl+shift+down":  func(m *Model) tea.Cmd { m.moveVert(1, true); return nil },
		"shift+home":       func(m *Model) tea.Cmd { m.smartHome(true); return nil },
		"shift+end": func(m *Model) tea.Cmd {
			m.moveTo(Pos{m.buf.Cursor.Row, len(m.line(m.buf.Cursor.Row))}, true)
			return nil
		},
		"shift+pgup":   func(m *Model) tea.Cmd { m.page(-1, true); return nil },
		"shift+pgdown": func(m *Model) tea.Cmd { m.page(1, true); return nil },
		"ctrl+a":       func(m *Model) tea.Cmd { m.selectAll(); return nil },

		// moving text: the highlighted text, else the word at the cursor
		"alt+left":        func(m *Model) tea.Cmd { m.moveSideways(-1); return nil },
		"alt+right":       func(m *Model) tea.Cmd { m.moveSideways(1); return nil },
		"alt+up":          func(m *Model) tea.Cmd { m.moveUpDown(-1); return nil },
		"alt+down":        func(m *Model) tea.Cmd { m.moveUpDown(1); return nil },
		"alt+shift+left":  func(m *Model) tea.Cmd { m.moveTextSideways(-1); return nil },
		"alt+shift+right": func(m *Model) tea.Cmd { m.moveTextSideways(1); return nil },

		// copy and paste
		"ctrl+c": func(m *Model) tea.Cmd { return m.copy(false) },
		"ctrl+x": func(m *Model) tea.Cmd { return m.copy(true) },
		"ctrl+v": func(m *Model) tea.Cmd { return readClipboard(m.clip) },
		"ctrl+p": func(m *Model) tea.Cmd { return readClipboard(m.clip) },

		// find
		"ctrl+f":   func(m *Model) tea.Cmd { return m.openFind(pFind) },
		"ctrl+r":   func(m *Model) tea.Cmd { return m.openFind(pReplaceFind) },
		"f3":       func(m *Model) tea.Cmd { m.findNext(1, false); return nil },
		"shift+f3": func(m *Model) tea.Cmd { m.findNext(-1, false); return nil },
		"ctrl+g":   func(m *Model) tea.Cmd { return m.ask(pGoto, "Go to line: ", "") },

		// files
		"alt+a": func(m *Model) tea.Cmd {
			if m.buf.Draft {
				return m.askName()
			}
			return m.ask(pSaveAs, "Save as: ", m.displayPath())
		},
		"ctrl+l": func(m *Model) tea.Cmd { return m.openLoadList() },
		"ctrl+o": func(m *Model) tea.Cmd { return m.openTree(false) },
		"ctrl+b": func(m *Model) tea.Cmd { return m.openTree(true) },

		// tabs
		"ctrl+w":      func(m *Model) tea.Cmd { return m.closeTab() },
		"alt+c":       func(m *Model) tea.Cmd { return m.closeTab() },
		"alt+n":       func(m *Model) tea.Cmd { return m.newFile() },
		"altgr+tab":   func(m *Model) tea.Cmd { return m.openTabMode() },
		"hyper+tab":   func(m *Model) tea.Cmd { return m.openTabMode() }, // AltGr+Tab sent on by Hyprland (tabmode.go)
		"ctrl+pgup":   func(m *Model) tea.Cmd { return m.nextTab(-1) },
		"ctrl+pgdown": func(m *Model) tea.Cmd { return m.nextTab(1) },
		"ctrl+n":      func(m *Model) tea.Cmd { return m.newFile() },
		"alt+q":       func(m *Model) tea.Cmd { return m.quit() },

		// editing
		"enter":         func(m *Model) tea.Cmd { m.newline(); return nil },
		"ctrl+j":        func(m *Model) tea.Cmd { m.newline(); return nil }, // some terminals send Enter as this
		"tab":           func(m *Model) tea.Cmd { m.tabKey(); return nil },  // Emmet, else indent (web.go)
		"alt+tab":       func(m *Model) tea.Cmd { m.tab(); return nil },     // the same
		"alt+shift+tab": func(m *Model) tea.Cmd { m.outdent(); return nil },
		"shift+tab":     func(m *Model) tea.Cmd { m.outdent(); return nil },
		"backspace": func(m *Model) tea.Cmd {
			tp := m.pairBefore()
			m.buf.Backspace()
			m.mirrorTag(tp)
			m.ensureVisible()
			return nil
		},
		"delete": func(m *Model) tea.Cmd {
			tp := m.pairBefore()
			m.buf.DeleteForward()
			m.mirrorTag(tp)
			m.ensureVisible()
			return nil
		},
		"alt+backspace":  func(m *Model) tea.Cmd { m.deleteWord(-1); return nil },
		"ctrl+backspace": func(m *Model) tea.Cmd { m.deleteWord(-1); return nil },
		"ctrl+delete":    func(m *Model) tea.Cmd { m.deleteWord(1); return nil },
		"alt+delete":     func(m *Model) tea.Cmd { m.deleteWord(1); return nil },
		"ctrl+z":         func(m *Model) tea.Cmd { m.undo(); return nil },
		"ctrl+y":         func(m *Model) tea.Cmd { m.redo(); return nil },
		"ctrl+shift+z":   func(m *Model) tea.Cmd { m.redo(); return nil },
		"ctrl+d":         func(m *Model) tea.Cmd { m.duplicate(); return nil },
		"ctrl+k":         func(m *Model) tea.Cmd { m.deleteLines(); return nil },
		"ctrl+/": func(m *Model) tea.Cmd {
			if !m.blockComment() {
				m.toggleComment()
			}
			return nil
		},
		"esc": func(m *Model) tea.Cmd {
			m.buf.ClearSelect()
			m.showMatches = false
			return nil
		},

		// web files: Emmet (web.go)
		"ctrl+e":      func(m *Model) tea.Cmd { return m.ctrlE() },
		"alt+w":       func(m *Model) tea.Cmd { return m.webMenu() },
		"alt+u":       func(m *Model) tea.Cmd { m.balance(true); return nil },
		"alt+d":       func(m *Model) tea.Cmd { m.balance(false); return nil },
		"alt+g":       func(m *Model) tea.Cmd { m.matchingPair(); return nil },
		"alt+r":       func(m *Model) tea.Cmd { m.tagChange("Remove tag", emmet.RemoveTag); return nil },
		"alt+k":       func(m *Model) tea.Cmd { m.tagChange("Split / join tag", emmet.SplitJoinTag); return nil },
		"alt+y":       func(m *Model) tea.Cmd { return m.askUpdateTag() },
		"alt+.":       func(m *Model) tea.Cmd { m.editPoint(1); return nil },
		"alt+,":       func(m *Model) tea.Cmd { m.editPoint(-1); return nil },
		"alt+]":       func(m *Model) tea.Cmd { m.selectItem(1); return nil },
		"alt+[":       func(m *Model) tea.Cmd { m.selectItem(-1); return nil },
		"alt+x":       func(m *Model) tea.Cmd { m.evalMath(); return nil },
		"alt+=":       func(m *Model) tea.Cmd { m.incNumber(1); return nil },
		"alt+-":       func(m *Model) tea.Cmd { m.incNumber(-1); return nil },
		"alt++":       func(m *Model) tea.Cmd { m.incNumber(10); return nil },
		"alt+shift+=": func(m *Model) tea.Cmd { m.incNumber(10); return nil },
		"alt+_":       func(m *Model) tea.Cmd { m.incNumber(-10); return nil },
		"alt+shift+-": func(m *Model) tea.Cmd { m.incNumber(-10); return nil },
		"alt+i":       func(m *Model) tea.Cmd { m.imageSize(); return nil },
		"alt+p":       func(m *Model) tea.Cmd { return m.pathList() },

		// help
		"f1":    func(m *Model) tea.Cmd { m.help, m.helpTop = true, 0; return nil },
		"alt+h": func(m *Model) tea.Cmd { m.help, m.helpTop = true, 0; return nil },
	}
	for i := 1; i <= 9; i++ {
		n := i - 1
		bindings[fmt.Sprintf("alt+%d", i)] = func(m *Model) tea.Cmd { return m.switchTab(n) }
	}
}

func (m *Model) editKey(k tea.Key) tea.Cmd {
	name := m.altGrName(k)
	if dir := ctrlArrow(name); dir != "" {
		m.ctrlArrow(dir, k)
		return nil
	}
	if fn, ok := bindings[name]; ok {
		if name != "ctrl+z" && name != "ctrl+y" {
			m.buf.BreakUndo()
		}
		return fn(m)
	}
	// Type the letter when no Ctrl or Alt is held, or both are (AltGr on
	// some keyboards sends Ctrl+Alt with a letter).
	ctrl, alt := k.Mod.Contains(tea.ModCtrl), k.Mod.Contains(tea.ModAlt)
	if k.Text != "" && ctrl == alt {
		m.typeText(k.Text)
		return nil
	}
	if k.Text == "" && k.Code == tea.KeySpace && !ctrl && !alt {
		m.typeText(" ")
	}
	return nil
}

// ---- cursor moves ------------------------------------------------------------------

func (m *Model) moveTo(p Pos, sel bool) {
	if sel {
		m.buf.StartSelect()
	} else {
		m.buf.ClearSelect()
	}
	m.buf.Cursor = m.buf.Clamp(p)
	m.buf.WantCol = -1
	m.ensureVisible()
}

func (m *Model) moveLeftRight(d int) {
	if s, e, ok := m.buf.Selection(); ok {
		if d < 0 {
			m.moveTo(s, false)
		} else {
			m.moveTo(e, false)
		}
		return
	}
	// One word, one run of symbols such as += or ->, or one character.
	if d < 0 {
		m.moveTo(m.buf.WordLeft(m.buf.Cursor), false)
	} else {
		m.moveTo(m.buf.WordRight(m.buf.Cursor), false)
	}
}

func (m *Model) moveVert(d int, sel bool) {
	b := m.buf
	if sel {
		b.StartSelect()
	} else {
		b.ClearSelect()
	}
	if b.WantCol < 0 {
		b.WantCol = colToX(m.line(b.Cursor.Row), b.Cursor.Col)
	}
	want := b.WantCol
	r := b.Cursor.Row + d
	switch {
	case r < 0:
		b.Cursor = Pos{0, 0}
		b.WantCol = -1
	case r >= len(b.Lines):
		b.Cursor = Pos{len(b.Lines) - 1, len(m.line(len(b.Lines) - 1))}
		b.WantCol = -1
	default:
		b.Cursor = Pos{r, xToCol(m.line(r), want)}
		b.WantCol = want
	}
	m.ensureVisible()
}

func (m *Model) page(d int, sel bool) {
	h := m.textHeight() - 1
	keep := m.buf.Cursor.Row - m.top
	m.moveVert(d*h, sel)
	m.top = max(0, min(m.buf.Cursor.Row-keep, len(m.buf.Lines)-1))
	m.ensureVisible()
}

func (m *Model) smartHome(sel bool) {
	l := m.line(m.buf.Cursor.Row)
	first := 0
	for first < len(l) && (l[first] == ' ' || l[first] == '\t') {
		first++
	}
	col := first
	if m.buf.Cursor.Col == first {
		col = 0
	}
	m.moveTo(Pos{m.buf.Cursor.Row, col}, sel)
}

func (m *Model) endOfFile() Pos {
	last := len(m.buf.Lines) - 1
	return Pos{last, len(m.line(last))}
}

func (m *Model) selectAll() {
	a := Pos{}
	m.buf.Anchor = &a
	m.buf.Cursor = m.endOfFile()
	m.ensureVisible()
}

// ---- Ctrl + arrows, and two of them held together ------------------------------
//
// Ctrl + arrows move the cursor one character (Up / Down: one line). Holding
// two at once (Ctrl+Left+Up) moves it one line AND one letter, again and
// again while both are held. In kitty the terminal says when a key is let
// go, so Symbolizer knows exactly which arrows are held. Other terminals do
// not say this, so there two arrows pressed within 0.15 seconds of each
// other count as held together.

type chordState struct {
	held    map[string]bool
	last    string
	lastAt  time.Time
	single  bool // the last ctrl-arrow did its one-key action
	before  Pos  // cursor before that action
	after   Pos  // cursor after it
	version int  // buffer version after it
	pair    [2]string
	pairAt  time.Time
}

const chordWindow = 150 * time.Millisecond
const repeatWindow = 700 * time.Millisecond

func ctrlArrow(name string) string {
	switch name {
	case "ctrl+left":
		return "left"
	case "ctrl+right":
		return "right"
	case "ctrl+up":
		return "up"
	case "ctrl+down":
		return "down"
	}
	return ""
}

func vertical(d string) bool { return d == "up" || d == "down" }

func (m *Model) arrowRelease(k tea.Key) {
	switch k.Code {
	case tea.KeyLeft:
		m.chord.held["left"] = false
	case tea.KeyRight:
		m.chord.held["right"] = false
	case tea.KeyUp:
		m.chord.held["up"] = false
	case tea.KeyDown:
		m.chord.held["down"] = false
	}
}

func (m *Model) ctrlArrow(dir string, k tea.Key) {
	now := time.Now()
	a := &m.chord
	partner := ""
	if m.releases {
		for d, down := range a.held {
			if down && d != dir && vertical(d) != vertical(dir) {
				partner = d
			}
		}
		a.held[dir] = true
	} else {
		if a.last != "" && a.last != dir && vertical(a.last) != vertical(dir) &&
			now.Sub(a.lastAt) < chordWindow {
			partner = a.last
		} else if (a.pair[0] == dir || a.pair[1] == dir) && now.Sub(a.pairAt) < repeatWindow {
			if a.pair[0] == dir {
				partner = a.pair[1]
			} else {
				partner = a.pair[0]
			}
		}
	}
	if partner != "" {
		// The first arrow already moved the cursor on its own; take that
		// back, so the pair makes exactly one diagonal step.
		if a.single && a.last == partner && m.buf.Cursor == a.after && m.buf.Version() == a.version {
			m.buf.Cursor = a.before
		}
		m.diagonal(dir, partner)
		a.single = false
		a.pair, a.pairAt = [2]string{dir, partner}, now
	} else {
		a.before = m.buf.Cursor
		switch dir {
		case "left":
			m.moveTo(m.buf.LetterLeft(m.buf.Cursor), false)
		case "right":
			m.moveTo(m.buf.LetterRight(m.buf.Cursor), false)
		case "up":
			m.moveVert(-1, false)
		case "down":
			m.moveVert(1, false)
		}
		a.after, a.version = m.buf.Cursor, m.buf.Version()
		// Only a single first press can be taken back. Once the key has been
		// held long enough to repeat, its moves were meant.
		a.single = !k.IsRepeat
	}
	a.last, a.lastAt = dir, now
}

func (m *Model) diagonal(d1, d2 string) {
	dr, dc := 0, 0
	for _, d := range []string{d1, d2} {
		switch d {
		case "up":
			dr = -1
		case "down":
			dr = 1
		case "left":
			dc = -1
		case "right":
			dc = 1
		}
	}
	b := m.buf
	b.ClearSelect()
	r := b.Cursor.Row + dr
	if r < 0 || r >= len(b.Lines) {
		r = b.Cursor.Row
	}
	c := min(b.Cursor.Col, len(m.line(r))) + dc
	c = max(0, min(c, len(m.line(r))))
	b.Cursor = Pos{r, c}
	b.WantCol = -1
	m.ensureVisible()
}

// ---- typing and editing --------------------------------------------------------------

// typeText types letters. Undo takes back one word (with its space) at a time.
func (m *Model) typeText(s string) {
	if s != " " && m.afterSpace {
		m.buf.BreakUndo()
	}
	m.afterSpace = s == " "
	tp := tagPair{}
	if len([]rune(s)) == 1 && (isWord([]rune(s)[0]) || s == "-" || s == ":" || s == ".") {
		tp = m.pairBefore()
	}
	m.buf.Insert(s, "type")
	m.mirrorTag(tp)
	if len([]rune(s)) == 1 {
		m.afterTyped(s) // Emmet: close tags, fix CSS and paths (web.go)
	}
	m.ensureVisible()
}

func (m *Model) newline() {
	m.beforeNewline()
	if m.splitPair() {
		return
	}
	l := m.line(m.buf.Cursor.Row)
	indent := leadingSpace(l)
	if s, _, ok := m.buf.Selection(); ok {
		indent = leadingSpace(m.line(s.Row))
	}
	col := m.buf.Cursor.Col
	if col < len(indent) {
		indent = indent[:col]
	}
	trimmed := strings.TrimRight(string(l[:col]), " \t")
	if m.hl.Name != "Text" && trimmed != "" && strings.ContainsAny(trimmed[len(trimmed)-1:], ":{[(") {
		indent += m.buf.Indent
	}
	m.buf.Insert("\n"+indent, "")
	m.ensureVisible()
}

func leadingSpace(l []rune) string {
	i := 0
	for i < len(l) && (l[i] == ' ' || l[i] == '\t') {
		i++
	}
	return string(l[:i])
}

func (m *Model) selectedRows() (int, int, bool) {
	if s, e, ok := m.buf.Selection(); ok {
		last := e.Row
		if e.Col == 0 && e.Row > s.Row {
			last--
		}
		return s.Row, last, true
	}
	return m.buf.Cursor.Row, m.buf.Cursor.Row, false
}

func (m *Model) tab() {
	if r1, r2, ok := m.selectedRows(); ok && r2 > r1 {
		m.indentRows(r1, r2, 1)
		return
	}
	if m.buf.Indent == "\t" {
		m.buf.Insert("\t", "type")
	} else {
		x := colToX(m.line(m.buf.Cursor.Row), m.buf.Cursor.Col)
		m.buf.Insert(strings.Repeat(" ", tabWidth-x%tabWidth), "type")
	}
	m.ensureVisible()
}

func (m *Model) outdent() {
	r1, r2, _ := m.selectedRows()
	m.indentRows(r1, r2, -1)
}

// indentRows adds or removes one indent on each line, keeping the selection.
func (m *Model) indentRows(r1, r2, d int) {
	b := m.buf
	anchor, cursor := b.Anchor, b.Cursor
	var shift = map[int]int{}
	b.Group(func() {
		for r := r1; r <= r2; r++ {
			l := m.line(r)
			if d > 0 {
				if len(l) == 0 {
					continue
				}
				b.Change(Pos{r, 0}, Pos{r, 0}, b.Indent, "")
				shift[r] = len([]rune(b.Indent))
			} else {
				n := 0
				if len(l) > 0 && l[0] == '\t' {
					n = 1
				}
				for n < len(l) && n < tabWidth && l[n] == ' ' {
					n++
				}
				if n > 0 {
					b.Change(Pos{r, 0}, Pos{r, n}, "", "")
					shift[r] = -n
				}
			}
		}
	})
	fix := func(p Pos) Pos { p.Col = max(0, p.Col+shift[p.Row]); return b.Clamp(p) }
	if anchor != nil {
		a := fix(*anchor)
		b.Anchor = &a
	}
	b.Cursor = fix(cursor)
	m.ensureVisible()
}

func (m *Model) deleteWord(d int) {
	b := m.buf
	if b.DeleteSelection() {
		return
	}
	if d < 0 {
		b.Change(b.WordLeft(b.Cursor), b.Cursor, "", "")
	} else {
		b.Change(b.Cursor, b.WordRight(b.Cursor), "", "")
	}
	m.ensureVisible()
}

func (m *Model) undo() {
	if !m.buf.Undo() {
		m.say("Nothing to undo.")
	}
	m.ensureVisible()
}

func (m *Model) redo() {
	if !m.buf.Redo() {
		m.say("Nothing to redo.")
	}
	m.ensureVisible()
}

func (m *Model) duplicate() {
	b := m.buf
	if s, e, ok := b.Selection(); ok {
		text := b.Get(s, e)
		b.Change(e, e, text, "")
		end := endOf(e, text)
		b.Anchor, b.Cursor = &e, end
		m.ensureVisible()
		return
	}
	r := b.Cursor.Row
	col := b.Cursor.Col
	l := string(m.line(r))
	b.Change(Pos{r, len(m.line(r))}, Pos{r, len(m.line(r))}, "\n"+l, "")
	b.Cursor = Pos{r + 1, col}
	m.ensureVisible()
}

func (m *Model) deleteLines() {
	b := m.buf
	r1, r2, _ := m.selectedRows()
	last := len(b.Lines) - 1
	switch {
	case r2 < last:
		b.Change(Pos{r1, 0}, Pos{r2 + 1, 0}, "", "")
	case r1 > 0:
		b.Change(Pos{r1 - 1, len(m.line(r1 - 1))}, Pos{r2, len(m.line(r2))}, "", "")
		b.Cursor = Pos{r1 - 1, 0}
	default:
		b.Change(Pos{0, 0}, Pos{r2, len(m.line(r2))}, "", "")
	}
	b.Cursor = b.Clamp(Pos{min(r1, len(b.Lines)-1), 0})
	m.ensureVisible()
}

// moveUpDown is Alt+Up / Alt+Down. Whole lines highlighted (or nothing
// highlighted) move as lines. Part of a line highlighted: only that text
// moves, to the same column one line up or down.
func (m *Model) moveUpDown(d int) {
	s, e, ok := m.buf.Selection()
	if !ok || m.wholeLines(s, e) {
		m.moveLines(d)
		return
	}
	m.moveTextVert(s, e, d)
}

// wholeLines: the highlight runs from the start of a line to the end of a
// line. Over several lines the indent counts as the start; on one line it
// does not, so text moved onto a line padded with spaces can move back.
func (m *Model) wholeLines(s, e Pos) bool {
	if s.Col > 0 && (e.Row == s.Row || s.Col > len(leadingSpace(m.line(s.Row)))) {
		return false
	}
	return (e.Col == 0 && e.Row > s.Row) || e.Col == len(m.line(e.Row))
}

func (m *Model) moveTextVert(s, e Pos, d int) {
	b := m.buf
	if (d < 0 && s.Row == 0) || (d > 0 && e.Row >= len(b.Lines)-1) {
		return
	}
	startAtCursor := b.Cursor == s
	text := b.Get(s, e)
	x := colToX(m.line(s.Row), s.Col)
	var at Pos
	b.Group(func() {
		b.Change(s, e, "", "")
		// Spaces left at the end of the line (padding from an earlier move)
		// go too.
		l := m.line(s.Row)
		if s.Col == len(l) {
			keep := len([]rune(strings.TrimRight(string(l), " \t")))
			if keep < len(l) {
				b.Change(Pos{s.Row, keep}, Pos{s.Row, len(l)}, "", "")
			}
		}
		row := s.Row + d
		t := m.line(row)
		at = Pos{row, xToCol(t, x)}
		if w := colToX(t, len(t)); w < x {
			// the line is too short: spaces keep the text in its column
			b.Change(Pos{row, len(t)}, Pos{row, len(t)}, strings.Repeat(" ", x-w), "")
			at = Pos{row, len(m.line(row))}
		}
		b.Change(at, at, text, "")
	})
	end := endOf(at, text)
	if startAtCursor {
		b.Anchor, b.Cursor = &end, at
	} else {
		b.Anchor, b.Cursor = &at, end
	}
	b.WantCol = -1
	m.ensureVisible()
}

// moveLines moves the current line (or the selected lines) up or down.
func (m *Model) moveLines(d int) {
	b := m.buf
	r1, r2, _ := m.selectedRows()
	if (d < 0 && r1 == 0) || (d > 0 && r2 >= len(b.Lines)-1) {
		return
	}
	anchor, cursor := b.Anchor, b.Cursor
	block := make([]string, 0, r2-r1+1)
	for r := r1; r <= r2; r++ {
		block = append(block, string(m.line(r)))
	}
	if d < 0 {
		above := string(m.line(r1 - 1))
		b.Change(Pos{r1 - 1, 0}, Pos{r2, len(m.line(r2))}, strings.Join(block, "\n")+"\n"+above, "")
	} else {
		below := string(m.line(r2 + 1))
		b.Change(Pos{r1, 0}, Pos{r2 + 1, len(m.line(r2 + 1))}, below+"\n"+strings.Join(block, "\n"), "")
	}
	if anchor != nil {
		a := Pos{anchor.Row + d, anchor.Col}
		b.Anchor = &a
	}
	b.Cursor = b.Clamp(Pos{cursor.Row + d, cursor.Col})
	m.ensureVisible()
}

// moveTextSideways moves the selected text (or the letter under the cursor)
// one letter left or right along its line.
func (m *Model) moveTextSideways(d int) {
	b := m.buf
	s, e, ok := b.Selection()
	if ok && s.Row != e.Row {
		m.say("Alt+Shift+Left/Right moves text within one line. Select less text.")
		return
	}
	if !ok {
		s, e = b.Cursor, Pos{b.Cursor.Row, b.Cursor.Col + 1}
		if e.Col > len(m.line(s.Row)) {
			return
		}
	}
	l := m.line(s.Row)
	if (d < 0 && s.Col == 0) || (d > 0 && e.Col >= len(l)) {
		return
	}
	text := string(l[s.Col:e.Col])
	if d < 0 {
		b.Change(Pos{s.Row, s.Col - 1}, e, text+string(l[s.Col-1]), "")
	} else {
		b.Change(s, Pos{e.Row, e.Col + 1}, string(l[e.Col])+text, "")
	}
	ns, ne := Pos{s.Row, s.Col + d}, Pos{e.Row, e.Col + d}
	if ok {
		b.Anchor, b.Cursor = &ns, ne
	} else {
		b.Cursor = ns
	}
	m.ensureVisible()
}

// moveSideways is Alt+Left / Alt+Right: the highlighted text moves one
// letter that way; with nothing highlighted, the word at the cursor swaps
// places with the word (or symbol) next to it.
func (m *Model) moveSideways(d int) {
	if _, _, ok := m.buf.Selection(); ok {
		m.moveTextSideways(d)
		return
	}
	m.moveWord(d)
}

// wordUnits lists the [start, end) of each thing Alt+Left / Right moves on a
// line: a whole word (MAX_PARTS, don't, 3.14) or one step of anything else
// (a bracket, an operator like += or ->).
func wordUnits(l []rune) [][2]int {
	var out [][2]int
	for i := 0; i < len(l); {
		if unicode.IsSpace(l[i]) {
			i++
			continue
		}
		e := tokenEnd(l, i)
		if isWord(l[i]) {
			for e < len(l) && (isWord(l[e]) || l[e] == '\'' && e+1 < len(l) && unicode.IsLetter(l[e+1]) && unicode.IsLetter(l[e-1])) {
				e++
			}
		}
		out = append(out, [2]int{i, e})
		i = e
	}
	return out
}

// moveWord swaps the word at the cursor with its neighbour on the line. The
// space between them stays where it is, and the cursor stays in the word.
func (m *Model) moveWord(d int) {
	b := m.buf
	row, col := b.Cursor.Row, b.Cursor.Col
	l := m.line(row)
	units := wordUnits(l)
	at := -1
	for i, u := range units {
		if u[0] <= col && col < u[1] {
			at = i
			break
		}
	}
	if at < 0 {
		for i, u := range units {
			if u[1] == col {
				at = i // just after a word, as after typing it
			}
		}
	}
	if at < 0 {
		for i, u := range units {
			if u[0] >= col {
				at = i // in the spaces: the next word
				break
			}
		}
	}
	other := at + d
	if at < 0 || other < 0 || other >= len(units) {
		return
	}
	a, z := units[min(at, other)], units[max(at, other)]
	text := string(l[z[0]:z[1]]) + string(l[a[1]:z[0]]) + string(l[a[0]:a[1]])
	moved := units[at]
	off := col - moved[0]
	b.Change(Pos{row, a[0]}, Pos{row, z[1]}, text, "")
	start := a[0]
	if d > 0 {
		start = z[1] - (moved[1] - moved[0])
	}
	b.Cursor = Pos{row, start + off}
	b.WantCol = -1
	m.ensureVisible()
}

func (m *Model) toggleComment() {
	tok := commentToken(m.hl.Name)
	if tok == "" {
		m.say("No line comment in " + m.hl.Name + ".")
		return
	}
	b := m.buf
	r1, r2, _ := m.selectedRows()
	all := true
	minIndent := -1
	for r := r1; r <= r2; r++ {
		l := strings.TrimLeft(string(m.line(r)), " \t")
		if l == "" {
			continue
		}
		if !strings.HasPrefix(l, tok) {
			all = false
		}
		if n := len(leadingSpace(m.line(r))); minIndent < 0 || n < minIndent {
			minIndent = n
		}
	}
	if minIndent < 0 {
		return
	}
	anchor, cursor := b.Anchor, b.Cursor
	b.Group(func() {
		for r := r1; r <= r2; r++ {
			l := string(m.line(r))
			if strings.TrimSpace(l) == "" {
				continue
			}
			if all {
				i := len([]rune(leadingSpace(m.line(r))))
				n := len([]rune(tok))
				rest := m.line(r)[i+n:]
				if len(rest) > 0 && rest[0] == ' ' {
					n++
				}
				b.Change(Pos{r, i}, Pos{r, i + n}, "", "")
			} else {
				b.Change(Pos{r, minIndent}, Pos{r, minIndent}, tok+" ", "")
			}
		}
	})
	if anchor != nil {
		a := b.Clamp(*anchor)
		b.Anchor = &a
	}
	b.Cursor = b.Clamp(cursor)
	m.ensureVisible()
}

// ---- copy and paste ------------------------------------------------------------------

func (m *Model) copy(cut bool) tea.Cmd {
	b := m.buf
	text, whole := b.SelectedText(), false
	if text == "" {
		// Nothing selected: copy (or cut) the whole line.
		text, whole = string(m.line(b.Cursor.Row))+"\n", true
	}
	m.clip, m.clipLine = text, whole
	if cut {
		if whole {
			m.deleteLines()
		} else {
			b.DeleteSelection()
		}
		m.ensureVisible()
	}
	what := "Copied"
	if cut {
		what = "Cut"
	}
	if whole {
		m.say(what + " the line.")
	} else {
		m.say(fmt.Sprintf("%s %d letters.", what, len([]rune(text))))
	}
	return writeClipboard(text)
}

func (m *Model) pasteText(text string) tea.Cmd {
	if text == "" {
		m.say("The clipboard is empty.")
		return nil
	}
	b := m.buf
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	if m.clipLine && text == m.clip && b.SelectedText() == "" {
		// A whole copied line goes in above the current line.
		col := b.Cursor.Col
		r := b.Cursor.Row
		b.Change(Pos{r, 0}, Pos{r, 0}, text, "")
		b.Cursor = b.Clamp(Pos{r + strings.Count(text, "\n"), col})
	} else {
		b.Insert(text, "")
	}
	b.BreakUndo()
	m.ensureVisible()
	return nil
}

// ---- scrolling ------------------------------------------------------------------------

func (m *Model) gutterWidth() int {
	return max(3, len(fmt.Sprint(len(m.buf.Lines)))) + 2
}

func (m *Model) textWidth() int { _, text, _ := m.zone(); return text }

func (m *Model) ensureVisible() {
	h := m.textHeight()
	margin := min(3, (h-1)/2)
	r := m.buf.Cursor.Row
	if r < m.top+margin {
		m.top = max(0, r-margin)
	}
	if r > m.top+h-1-margin {
		m.top = r - h + 1 + margin
	}
	m.top = max(0, min(m.top, len(m.buf.Lines)-1))
	w := m.textWidth()
	x := colToX(m.line(r), m.buf.Cursor.Col)
	if x < m.left {
		m.left = max(0, x-w/4)
	}
	if x >= m.left+w {
		m.left = x - w + w/4
	}
}

// colToX is the screen column of letter col on a line (tabs and wide letters
// count as more than one column).
func colToX(l []rune, col int) int {
	x := 0
	for i := 0; i < col && i < len(l); i++ {
		x += runeWidth(l[i], x)
	}
	return x
}

func xToCol(l []rune, want int) int {
	x := 0
	for i, r := range l {
		w := runeWidth(r, x)
		if x+w > want {
			if want-x > w/2 && r != '\t' {
				return i + 1
			}
			return i
		}
		x += w
	}
	return len(l)
}

// ---- mouse ------------------------------------------------------------------------------

func (m *Model) mouseToPos(x, y int) (Pos, bool) {
	if y < textTop || y >= textTop+m.textHeight() {
		return Pos{}, false
	}
	r := m.top + y - textTop
	if r >= len(m.buf.Lines) {
		return m.endOfFile(), true
	}
	left, _, _ := m.zone()
	col := xToCol(m.line(r), max(0, x-m.treeWidth()-left-m.gutterWidth()-m.lineOffset(r))+m.left)
	return Pos{r, col}, true
}

func (m *Model) mouseClick(ms tea.Mouse) tea.Cmd {
	if m.help {
		m.help = false
		return nil
	}
	if ms.Button != tea.MouseLeft || m.prompt != pNone {
		return nil
	}
	if m.tabMode {
		return m.tabModeClick(ms.X, ms.Y)
	}
	if m.picker != nil {
		m.picker = nil
		return nil
	}
	if m.showTree && m.tree != nil {
		return m.treeMouse(ms.X, ms.Y)
	}
	if ms.Y == 0 {
		return m.openModes() // the top panel shows the mode: click it to switch
	}
	if ms.Y == 1 {
		return m.clickTab(ms.X)
	}
	if m.marginPress(ms.X, ms.Y) {
		return nil
	}
	m.treeFocus = false
	p, ok := m.mouseToPos(ms.X, ms.Y)
	if !ok {
		return nil
	}
	now := time.Now()
	if now.Sub(m.lastClick) < 400*time.Millisecond && p == m.lastClickP {
		m.clickCount++
	} else {
		m.clickCount = 1
	}
	m.lastClick, m.lastClickP = now, p
	b := m.buf
	m.buf.BreakUndo()
	switch {
	case m.clickCount == 2:
		s, e := b.WordAt(p)
		b.Anchor, b.Cursor = &s, e
	case m.clickCount >= 3:
		s := Pos{p.Row, 0}
		b.Anchor, b.Cursor = &s, b.Clamp(Pos{p.Row + 1, 0})
		if p.Row == len(b.Lines)-1 {
			b.Cursor = Pos{p.Row, len(m.line(p.Row))}
		}
	case ms.Mod.Contains(tea.ModShift):
		b.StartSelect()
		b.Cursor = p
	default:
		a := p
		b.Anchor, b.Cursor = &a, p
		m.dragging = true
	}
	b.WantCol = -1
	return nil
}

func (m *Model) mouseDrag(ms tea.Mouse) tea.Cmd {
	if !m.dragging {
		return nil
	}
	y := ms.Y
	if y < textTop {
		m.top = max(0, m.top-1)
		y = textTop
	}
	if y >= textTop+m.textHeight() {
		m.top = min(len(m.buf.Lines)-1, m.top+1)
		y = textTop + m.textHeight() - 1
	}
	if p, ok := m.mouseToPos(ms.X, y); ok {
		m.buf.Cursor = p
	}
	return nil
}

// ---- files ---------------------------------------------------------------------------------

func (m *Model) displayPath() string {
	p := m.buf.Path
	if p == "" {
		return ""
	}
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home+"/") {
		return "~" + p[len(home):]
	}
	return p
}

func expandPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = home + p[1:]
		}
	}
	if p != "" {
		if abs, err := filepath.Abs(p); err == nil {
			return abs
		}
	}
	return p
}
