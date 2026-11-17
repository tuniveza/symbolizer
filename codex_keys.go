package main

// ---- Codex mode's keys, clicks and bars -----------------------------------------------------
//
// The library (no codex open):
//
//	arrows        choose a codex          Enter   open it (to read)
//	E             open it to edit         Alt+N   make a new one
//	P             put it on the Design board (as its cover)
//	X X           delete it (asks first)
//
// Reading:
//
//	→ PgDn Space  next page (the cover opens)      ← PgUp   the page before
//	Home · End    the cover · the last page        Tab      the next button or active thing
//	Enter         press it (activate it)           click    press what is clicked
//	E             edit mode                        Esc      close (back to the board, if from there)
//
// Editing (the slots show as dotted lines):
//
//	arrows        choose a thing           PgUp · PgDn   page before · next page (cover first)
//	Alt+N, then T text · B button · I picture · S symbol · D drawing
//	Enter         write its words · choose its picture · draw in it · its next symbol;
//	              on the cover: write the title (Tab: the subtitle)
//	L · Shift+L   the page's next · last layout (16)
//	Alt+← →       move it to the slot before · after      Alt+↑ ↓   up · down in its slot
//	F C A + − S J font · colour · animation · size · symbol · left / middle / right (Shift: back)
//	K · G         a button's action · what it aims at (page, or the thing to activate)
//	T             the activation table: what activating each thing does
//	W · O         paper colour · pages wide or upright
//	Alt+P         a new page after this one          X X   delete the thing (none chosen: the page)
//	Space         try activating it                  Ctrl+Z · Ctrl+Y   undo · redo
//	E             back to reading          Esc   close it (back to the board, if from there)

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

func (m *Model) codexKey(k tea.Key) tea.Cmd {
	cmd := m.codexKeyIn(k)
	return tea.Batch(cmd, m.codexTickCmd())
}

func (m *Model) codexKeyIn(k tea.Key) tea.Cmd {
	c := m.cx()
	name := keyName(k)
	if name == "f1" && c.writing == 0 {
		m.help = true
		return nil
	}
	if c.turn != nil {
		c.turn = nil // a key during a turn: it is there at once
	}
	c.newPic = nil // a key here: the picture prompt is gone (chosen, or cancelled)
	if c.cf == nil {
		return m.codexLibKey(name)
	}
	if c.writing != 0 {
		m.codexWriteKey(k, name)
		return nil
	}
	if c.drawing {
		m.codexDrawKey(name)
		return nil
	}
	if c.table {
		return m.codexTableKey(name)
	}
	if c.placing {
		c.placing = false
		return m.codexPlace(name)
	}
	if name != "x" && name != "X" && name != "shift+x" {
		c.xAt = time.Time{}
	}
	if c.edit {
		return m.codexEditKey(name)
	}
	return m.codexReadKey(name)
}

// ---- the library ----------------------------------------------------------------------------

func (m *Model) codexLibCols() int { return max(1, (m.width-2)/(codexTileW+3)) }

func (m *Model) codexLibKey(name string) tea.Cmd {
	c := m.cx()
	n := len(c.lib)
	cols := m.codexLibCols()
	if name != "x" && name != "X" && name != "shift+x" {
		c.xAt = time.Time{}
	}
	switch name {
	case "left":
		c.libSel = max(c.libSel-1, 0)
	case "right":
		c.libSel = min(c.libSel+1, max(n-1, 0))
	case "up":
		if c.libSel-cols >= 0 {
			c.libSel -= cols
		}
	case "down":
		if c.libSel+cols < n {
			c.libSel += cols
		}
	case "home":
		c.libSel = 0
	case "end":
		c.libSel = max(n-1, 0)
	case "alt+n":
		id, cf, err := createCodex("")
		if err != nil {
			m.sayErr("Could not make a codex: " + err.Error())
			return nil
		}
		cf.Room = -1 // made here, not on a Design board: it gets its own room when kept
		saveCodex(id, cf)
		c.lib = codexList()
		if m.codexOpen(id, true) {
			m.codexStartWrite(codexWriteTitle)
			m.say("New codex. Type its title, then Enter (Tab: a subtitle).")
		}
	case "enter", "e", "E", "shift+e":
		if c.libSel < n {
			m.codexOpen(c.lib[c.libSel].id, name != "enter")
		}
	case "p", "P", "shift+p":
		if c.libSel < n {
			return m.codexToBoard(c.lib[c.libSel].id)
		}
	case "x", "X", "shift+x":
		if c.libSel >= n {
			return nil
		}
		e := c.lib[c.libSel]
		if c.xAt.IsZero() || time.Since(c.xAt) > xTwice {
			c.xAt = time.Now()
			m.say("Press X again to delete “" + codexName(e.title) + "”.")
			return nil
		}
		c.xAt = time.Time{}
		return m.confirm("Delete the codex “"+codexName(e.title)+"” and its pictures for good? y = yes, n = no", "yn", func(m *Model, k rune) tea.Cmd {
			if k != 'y' {
				m.say("Kept.")
				return nil
			}
			if err := os.RemoveAll(codexDir(e.id)); err != nil {
				m.sayErr("Could not delete it: " + err.Error())
				return nil
			}
			codexPeekDrop(e.id)
			c := m.cx()
			c.lib = codexList()
			c.libSel = min(c.libSel, max(len(c.lib)-1, 0))
			m.say("Deleted.")
			return nil
		})
	case "esc":
		m.say("Alt+M: other modes.")
	}
	return nil
}

func codexName(t string) string {
	if strings.TrimSpace(t) == "" {
		return "Untitled"
	}
	return t
}

// ---- reading ------------------------------------------------------------------------------

func (m *Model) codexReadKey(name string) tea.Cmd {
	c := m.cx()
	switch name {
	case "right", "pgdown", "space", "l":
		if c.page+1 < len(c.cf.Pages) {
			m.codexTurnTo(c.page + 1)
		} else {
			m.say("The last page.")
		}
	case "left", "pgup", "h":
		if c.page >= 0 {
			m.codexTurnTo(c.page - 1)
		}
	case "home":
		m.codexTurnTo(-1)
	case "end":
		m.codexTurnTo(len(c.cf.Pages) - 1)
	case "tab", "shift+tab", "down", "up":
		m.codexFocusStep(name == "shift+tab" || name == "up")
	case "enter":
		if c.page < 0 {
			m.codexTurnTo(0)
			return nil
		}
		e := c.el(c.focus)
		if e == nil {
			if list := c.interactive(); len(list) > 0 {
				e = list[0]
				for _, x := range list { // a button first: Enter is for going on
					if x.Kind == ceButton {
						e = x
						break
					}
				}
				c.focus = e.ID
			}
		}
		if e == nil {
			m.say("Nothing to press on this page.")
			return nil
		}
		m.codexActivate(e, 0)
	case "e", "E", "shift+e":
		c.edit = true
		if c.el(c.sel) == nil {
			c.sel = 0
			if p := c.pageOf(); p != nil && len(p.Els) > 0 {
				c.sel = p.Els[0].ID
			}
		}
		m.say("Edit mode. Alt+N adds · arrows choose · T: activation table · E: read again · Esc: close · F1: all keys.")
	case "esc":
		return m.codexClose()
	}
	return nil
}

// codexFocusStep moves Tab's focus to the next (or last) thing that can be pressed.
func (m *Model) codexFocusStep(back bool) {
	c := m.cx()
	list := c.interactive()
	if len(list) == 0 {
		c.focus = 0
		m.say("Nothing to press on this page.")
		return
	}
	at := -1
	for i, e := range list {
		if e.ID == c.focus {
			at = i
		}
	}
	switch {
	case at < 0 && back:
		at = len(list) - 1
	case at < 0:
		at = 0
	case back:
		at = (at - 1 + len(list)) % len(list)
	default:
		at = (at + 1) % len(list)
	}
	c.focus = list[at].ID
	m.say(codexElName(list[at]) + " · Enter presses it")
}

// ---- editing ---------------------------------------------------------------------------

func (m *Model) codexEditKey(name string) tea.Cmd {
	c := m.cx()
	e := c.el(c.sel)
	cover := c.page < 0
	switch name {
	case "ctrl+z":
		m.codexUndo(false)
		return nil
	case "ctrl+y", "ctrl+shift+z":
		m.codexUndo(true)
		return nil
	case "e", "E", "shift+e":
		c.edit = false
		c.states = map[int]*codexElState{}
		m.codexSave()
		m.say("Reading. E edits again · Esc closes.")
		return nil
	case "esc":
		return m.codexClose()
	case "pgdown":
		if c.page+1 >= len(c.cf.Pages) {
			m.say("The last page. Alt+P adds a page.")
			return nil
		}
		m.codexShowPage(c.page + 1)
		return nil
	case "pgup":
		m.codexShowPage(c.page - 1)
		return nil
	case "home":
		m.codexShowPage(-1)
		return nil
	case "end":
		m.codexShowPage(len(c.cf.Pages) - 1)
		return nil
	case "alt+n":
		if cover {
			m.codexShowPage(0)
		}
		c.placing = true
		m.say("Add: T text · B button · I picture · S symbol · D drawing (any other key: nothing).")
		return nil
	case "alt+p":
		m.codexStep()
		lay := 1
		if p := c.pageOf(); p != nil {
			lay = p.Layout
		}
		at := c.page + 1
		c.cf.Pages = append(c.cf.Pages[:at], append([]*codexPage{{Layout: lay}}, c.cf.Pages[at:]...)...)
		c.cf.codexPagesMoved(at, 1)
		m.codexShowPage(at)
		m.codexSave()
		m.say("New page " + itoa(at+1) + " of " + itoa(len(c.cf.Pages)) + ". Alt+N adds things · L changes its layout.")
		return nil
	case "o", "O", "shift+o":
		m.codexStep()
		c.cf.Tall = !c.cf.Tall
		m.codexSave()
		m.say(map[bool]string{false: "Pages lie wide.", true: "Pages stand upright."}[c.cf.Tall])
		return nil
	case "t", "T", "shift+t":
		if cover {
			m.say("The cover has nothing to activate. PgDn: the pages.")
			return nil
		}
		if p := c.pageOf(); p == nil || len(p.Els) == 0 {
			m.say("Nothing on this page yet. Alt+N adds things.")
			return nil
		}
		c.table, c.tableCol = true, 0
		c.tableRow = 0
		for i, el := range c.pageOf().Els {
			if el.ID == c.sel {
				c.tableRow = i
			}
		}
		m.say("Activation table: ↑↓ things · ←→ change · Tab: next column · Enter: write words · Esc: close.")
		return nil
	}
	if cover {
		return m.codexCoverKey(name)
	}
	p := c.pageOf()
	switch name {
	case "left", "right", "up", "down":
		m.codexChoose(name)
		return nil
	case "l", "L", "shift+l":
		m.codexStep()
		step := 1
		if name != "l" {
			step = -1
		}
		p.Layout = ((p.Layout+step)%16 + 16) % 16
		m.codexSave()
		m.say("Layout " + itoa(p.Layout+1) + " of 16: " + codexLayouts[p.Layout].name + " (" + plural(len(codexLayouts[p.Layout].slots), "slot") + ").")
		return nil
	case "w", "W", "shift+w":
		m.codexStep()
		p.Paper = codexNext(codexPapers, p.Paper, name == "w")
		m.codexSave()
		return nil
	case "x", "X", "shift+x":
		whole := name != "x" // Shift+X X: the whole page
		if whole {
			e = nil
		}
		if c.xAt.IsZero() || time.Since(c.xAt) > xTwice || c.xWhole != whole {
			c.xAt, c.xWhole = time.Now(), whole
			if e != nil {
				m.say("Press X again to delete " + codexElName(e) + ".")
			} else {
				m.say("Press X again to delete page " + itoa(c.page+1) + ".")
			}
			return nil
		}
		c.xAt = time.Time{}
		m.codexStep()
		if e != nil {
			m.codexDeleteEl(e)
			m.say("Deleted. Ctrl+Z brings it back.")
		} else if len(c.cf.Pages) > 1 {
			c.cf.Pages = append(c.cf.Pages[:c.page], c.cf.Pages[c.page+1:]...)
			c.cf.codexPagesMoved(c.page, -1)
			c.sel = 0
			m.codexShowPage(min(c.page, len(c.cf.Pages)-1))
			m.say("Page deleted. Ctrl+Z brings it back.")
		} else {
			m.say("A codex keeps at least one page.")
		}
		m.codexSave()
		return nil
	}
	if e == nil {
		if name == "enter" {
			m.say("Nothing chosen. Alt+N adds a thing; the arrows choose one.")
		}
		return nil
	}
	switch name {
	case "enter":
		switch e.Kind {
		case ceText, ceButton:
			m.codexStartWrite(codexWriteEl)
		case ceImage:
			return m.ask(pCodexImage, "Picture (any format) for "+codexElName(e)+": ", "")
		case ceDrawing:
			m.codexStartDraw()
		case ceSymbol:
			m.codexStep()
			e.Symbol = (e.Symbol + 1) % len(codexSymbols)
			m.codexSave()
			m.say("Symbol: " + codexSymbols[e.Symbol].Name + ".")
		}
		return nil
	case "space":
		if e.Kind == ceButton { // editing: show the press, don't leave the page
			c.state(e.ID).at = time.Now()
			if e.On.Do != "" {
				c.state(e.ID).on, c.state(e.ID).at = true, time.Now()
			}
			if (e.Action == caNext || e.Action == "") && c.page+1 >= len(c.cf.Pages) {
				m.say("Pressed while reading, it goes to the next page, but this is the last page. Alt+P adds a page.")
				return nil
			}
			m.say("Pressed while reading, it will " + codexButtonSays(c, e) + ".")
			return nil
		}
		m.codexActivate(e, 0)
		return nil
	case "alt+left", "alt+right", "alt+up", "alt+down":
		m.codexMove(e, name[len("alt+"):]) // codex_frames.go
		return nil
	case "b", "B", "shift+b":
		m.codexStep()
		e.Border = codexCycle(e.Border, len(codexBorders)+1, name == "b")
		m.codexSave()
		m.say("Border " + itoa(e.Border) + " of " + itoa(len(codexBorders)) + ": " + codexBorderName(e.Border) + ". B next · Shift+B back.")
		return nil
	case "y", "Y", "shift+y":
		if e.Kind != ceButton {
			m.say("Y changes a button's look. B gives any thing a border.")
			return nil
		}
		m.codexStep()
		e.Style = codexCycle(e.Style, len(codexButtonStyles), name == "y")
		m.codexSave()
		m.say("Button look " + itoa(e.Style+1) + " of " + itoa(len(codexButtonStyles)) + ": " + codexButtonStyles[e.Style].name + ".")
		return nil
	case "f", "F", "shift+f":
		if e.Kind != ceText && e.Kind != ceButton {
			m.say("Fonts are for texts and buttons.")
			return nil
		}
		m.codexStep()
		e.Font = codexCycle(e.Font, 16, name == "f")
		m.codexSave()
		m.say("Font: " + codexFonts[e.Font] + ".")
	case "c", "C", "shift+c":
		if (e.Kind == ceImage || e.Kind == ceDrawing) && e.Border == 0 {
			m.say("Colour is for texts, buttons, symbols and borders (B gives this one a border). A drawing's colours are its ink (Enter draws).")
			return nil
		}
		m.codexStep()
		e.Color = codexNext(codexPalette, e.Color, name == "c")
		m.codexSave()
	case "a", "A", "shift+a":
		m.codexStep()
		e.Anim = codexCycle(e.Anim, 16, name == "a")
		c.pageAt = time.Now() // it starts from the beginning
		m.codexSave()
		m.say("Animation: " + codexAnims[e.Anim] + ".")
	case "+", "=", "plus", "shift+=", "kp+", "-", "minus", "kp-":
		if e.Kind == ceImage {
			if whole := name == "-" || name == "minus" || name == "kp-"; whole != e.Whole {
				m.codexStep()
				e.Whole = whole
				m.codexSave()
			}
			m.say(map[bool]string{false: "The picture fills its box (cut at the edges). − shows it whole.", true: "The picture shows whole. + makes it fill its box."}[e.Whole])
			return nil
		}
	}
	switch name {
	case "+", "=", "plus", "shift+=", "kp+":
		if e.Kind != ceText && e.Kind != ceSymbol {
			m.say("Size is for texts and symbols.")
			return nil
		}
		m.codexStep()
		e.Size = min(max(e.Size, 1)+1, codexSizes)
		m.codexSave()
		m.say("Size " + itoa(e.Size) + " of " + itoa(codexSizes) + ".")
	case "-", "minus", "kp-":
		if e.Kind != ceText && e.Kind != ceSymbol {
			m.say("Size is for texts and symbols.")
			return nil
		}
		m.codexStep()
		e.Size = max(max(e.Size, 1)-1, 1)
		m.codexSave()
		m.say("Size " + itoa(e.Size) + " of " + itoa(codexSizes) + ".")
	case "s", "S", "shift+s":
		if e.Kind != ceSymbol {
			m.say("S changes a symbol. Alt+N, then S adds one.")
			return nil
		}
		m.codexStep()
		e.Symbol = codexCycle(e.Symbol, len(codexSymbols), name == "s")
		m.codexSave()
		m.say("Symbol: " + codexSymbols[e.Symbol].Name + ".")
	case "j", "J", "shift+j":
		if !codexSlides(e) {
			m.say(map[bool]string{true: "A picture that fills its box can't line up. − shows it whole first.", false: "A drawing fills its slot, so it can't line up."}[e.Kind == ceImage])
			return nil
		}
		m.codexStep()
		e.Align = codexCycle(e.Align, 3, name == "j")
		m.codexSave()
		m.say("Lined up: " + []string{"in the middle", "on the left", "on the right"}[e.Align] + ".")
	case "k", "K", "shift+k":
		if e.Kind != ceButton {
			m.say("K is a button's action. Alt+N, then B adds a button.")
			return nil
		}
		m.codexStep()
		e.Action = codexNext(codexActions, e.Action, name == "k")
		c.codexActionTarget(e)
		m.codexSave()
		m.say("The button: " + codexButtonSays(c, e) + ".")
	case "g", "G", "shift+g":
		if e.Kind != ceButton || (e.Action != caGoto && e.Action != caActivate) {
			m.say("G aims a button that goes to a page or activates a thing (K chooses that first).")
			return nil
		}
		m.codexStep()
		m.codexAim(e, name == "g")
		m.codexSave()
		m.say("The button: " + codexButtonSays(c, e) + ".")
	}
	return nil
}

// codexCoverKey: editing the cover.
func (m *Model) codexCoverKey(name string) tea.Cmd {
	c := m.cx()
	switch name {
	case "enter":
		m.codexStartWrite(codexWriteTitle)
	case "tab":
		m.codexStartWrite(codexWriteSubtitle)
	case "c", "C", "shift+c":
		m.codexStep()
		c.cf.Color = codexNext(codexPalette, c.cf.Color, name == "c")
		m.codexSave()
	case "s", "S", "shift+s":
		m.codexStep()
		c.cf.Symbol = codexCycle(c.cf.Symbol, len(codexSymbols), name == "s")
		m.codexSave()
		m.say("Cover symbol: " + codexSymbols[c.cf.Symbol].Name + ".")
	case "w", "W", "shift+w":
		m.codexStep()
		c.cf.Paper = codexNext(codexPapers, c.cf.Paper, name == "w")
		m.codexSave()
		m.say("The pages' paper changed.")
	default:
		m.say("The cover: Enter title · Tab subtitle · C colour · S symbol · W paper · PgDn: the pages.")
	}
	return nil
}

// codexCycle steps i through 0 … n-1 (fwd: up, else down), round.
func codexCycle(i, n int, fwd bool) int {
	if n <= 0 {
		return 0
	}
	if fwd {
		return ((i+1)%n + n) % n
	}
	return ((i-1)%n + n) % n
}

// codexNext is the item after (or before) cur in list.
func codexNext(list []string, cur string, fwd bool) string {
	at := -1
	for i, s := range list {
		if strings.EqualFold(s, cur) {
			at = i
		}
	}
	if at < 0 {
		if fwd {
			return list[0]
		}
		return list[len(list)-1]
	}
	return list[codexCycle(at, len(list), fwd)]
}

// codexChoose moves the choice to the nearest thing that way on the page.
func (m *Model) codexChoose(dir string) {
	c := m.cx()
	p := c.pageOf()
	if p == nil || len(p.Els) == 0 {
		m.say("Nothing on this page yet. Alt+N adds things.")
		return
	}
	cur, ok := c.rects[c.sel]
	if !ok {
		c.sel = p.Els[0].ID
		return
	}
	cx, cy := cur[0]+cur[2]/2, cur[1]+cur[3]/2
	best, bestD := 0, 1<<30
	for _, e := range p.Els {
		r, ok := c.rects[e.ID]
		if !ok || e.ID == c.sel {
			continue
		}
		ex, ey := r[0]+r[2]/2, r[1]+r[3]/2
		dx, dy := ex-cx, ey-cy
		var along, across int
		switch dir {
		case "left":
			along, across = -dx, abs(dy)
		case "right":
			along, across = dx, abs(dy)
		case "up":
			along, across = -dy*3, abs(dx)
		case "down":
			along, across = dy*3, abs(dx)
		}
		if along <= 0 {
			continue
		}
		if d := along + 2*across; d < bestD {
			best, bestD = e.ID, d
		}
	}
	if best != 0 {
		c.sel = best
		m.say(codexElName(c.el(best)) + " · " + codexElHint(c.el(best)))
	}
}

// codexReorder moves e up (or down) past the thing before (after) it in its slot.
func (m *Model) codexReorder(e *codexEl, up bool) {
	c := m.cx()
	p := c.pageOf()
	same := elsIn(p, codexSlotOf(e, p.Layout))
	at := -1
	for i, x := range same {
		if x == e {
			at = i
		}
	}
	to := at + 1
	if up {
		to = at - 1
	}
	if at < 0 || to < 0 || to >= len(same) {
		return
	}
	other := same[to]
	var i, j int
	for k, x := range p.Els {
		if x == e {
			i = k
		}
		if x == other {
			j = k
		}
	}
	p.Els[i], p.Els[j] = p.Els[j], p.Els[i]
}

func (m *Model) codexDeleteEl(e *codexEl) {
	c := m.cx()
	p := c.pageOf()
	for i, x := range p.Els {
		if x == e {
			p.Els = append(p.Els[:i], p.Els[i+1:]...)
			break
		}
	}
	c.sel = 0
	if len(p.Els) > 0 {
		c.sel = p.Els[len(p.Els)-1].ID
	}
}

// codexPlace adds a thing of the kind the key after Alt+N says.
func (m *Model) codexPlace(name string) tea.Cmd {
	c := m.cx()
	kind := map[string]string{"t": ceText, "b": ceButton, "i": ceImage, "s": ceSymbol, "d": ceDrawing}[strings.ToLower(strings.TrimPrefix(name, "shift+"))]
	p := c.pageOf()
	if kind == "" || p == nil {
		m.say("Nothing added. Alt+N, then T text · B button · I picture · S symbol · D drawing.")
		return nil
	}
	// The first empty slot; all full: after the chosen thing, in its slot.
	// A button goes to the button bar, if the layout has one.
	slot, at := -1, len(p.Els)
	lay := codexLayouts[p.Layout]
	if kind == ceButton && strings.Contains(lay.name, "button bar") {
		slot = len(lay.slots) - 1
	}
	for s := range lay.slots {
		if slot >= 0 {
			break
		}
		if len(elsIn(p, s)) == 0 {
			slot = s
		}
	}
	if slot < 0 {
		slot = 0
		if e := c.el(c.sel); e != nil {
			slot = codexSlotOf(e, p.Layout)
			for i, x := range p.Els {
				if x == e {
					at = i + 1
				}
			}
		} else if len(p.Els) > 0 {
			slot = codexSlotOf(p.Els[len(p.Els)-1], p.Layout)
		}
	}
	if kind == ceImage { // added once a picture is chosen: cancelled, nothing is left behind
		c.newPic = &codexNewPic{slot: slot, at: at, from: c.sel}
		return m.ask(pCodexImage, "Picture (any format) for the new picture: ", "")
	}
	m.codexStep()
	e := c.newEl(kind, slot)
	if kind == ceButton && c.page+1 >= len(c.cf.Pages) && c.page > 0 {
		e.Text, e.Action = "Back", caPrev
	}
	if kind == ceText && slot == 0 && codexTitleSlot[p.Layout] {
		e.Size = 2 // a title
	}
	p.Els = append(p.Els[:at], append([]*codexEl{e}, p.Els[at:]...)...)
	c.added, c.addedUndo, c.addedFrom = 0, len(c.undo), c.sel
	c.sel = e.ID
	m.codexSave()
	switch kind {
	case ceText:
		c.added = e.ID
		m.codexStartWrite(codexWriteEl)
		m.say("New text. Type it (Enter: a new line) · Esc: done.")
	case ceButton:
		c.added = e.ID
		m.codexStartWrite(codexWriteEl)
		c.fresh = true
		m.say("New button. Type its label (it replaces “" + e.Text + "”) · Enter: done · then K: its action.")
	case ceDrawing:
		c.added = e.ID
		m.codexStartDraw()
	case ceSymbol:
		m.say("New symbol: " + codexSymbols[e.Symbol].Name + ". S / Shift+S: another · + −: size · C: colour.")
	}
	return nil
}

// codexTitleSlot: layouts whose first slot is a title or header (a new text there is big).
var codexTitleSlot = map[int]bool{1: true, 4: true, 12: true}

// codexDropAdded takes away thing id, just added by Alt+N, if it was left
// empty (a text with no words, a drawing with no ink, a picture with no
// file), with the undo step that added it if nothing changed since. It
// says whether it did.
func (m *Model) codexDropAdded(id int) bool {
	c := m.cx()
	e, p := c.el(id), c.pageOf()
	if e == nil || p == nil {
		return false
	}
	switch e.Kind {
	case ceText:
		if strings.TrimSpace(e.Text) != "" {
			return false
		}
	case ceDrawing:
		if len(e.Ink) > 0 {
			return false
		}
	case ceImage:
		if e.Image != "" {
			return false
		}
	default:
		return false
	}
	for i, x := range p.Els {
		if x == e {
			p.Els = append(p.Els[:i], p.Els[i+1:]...)
			break
		}
	}
	if len(c.undo) > 0 && len(c.undo) == c.addedUndo {
		c.undo = c.undo[:len(c.undo)-1] // adding it was no step after all
	}
	c.sel = 0
	if c.el(c.addedFrom) != nil {
		c.sel = c.addedFrom
	} else if len(p.Els) > 0 {
		c.sel = p.Els[len(p.Els)-1].ID
	}
	return true
}

// codexAim steps a button's target: the next page, or the next thing.
func (m *Model) codexAim(e *codexEl, fwd bool) {
	c := m.cx()
	if e.Action == caGoto {
		e.Target = codexCycle(max(e.Target, 0), len(c.cf.Pages)+1, fwd) // 0 the cover, then the pages
		return
	}
	var ids []int
	for _, p := range c.cf.Pages {
		for _, x := range p.Els {
			if x.ID != e.ID {
				ids = append(ids, x.ID)
			}
		}
	}
	if len(ids) == 0 {
		return
	}
	at := -1
	for i, id := range ids {
		if id == e.Target {
			at = i
		}
	}
	if at < 0 {
		at = map[bool]int{true: -1, false: 0}[fwd]
	}
	e.Target = ids[codexCycle(at, len(ids), fwd)]
}

// codexButtonSays says what a button does, in words.
func codexButtonSays(c *codexState, e *codexEl) string {
	switch e.Action {
	case caGoto:
		if e.Target < 1 {
			return "go to the cover"
		}
		return "go to page " + itoa(e.Target)
	case caActivate:
		if t, pi := c.find(e.Target); t != nil {
			return "activate " + codexElName(t) + " (page " + itoa(pi+1) + ")"
		}
		return "activate … (G chooses what)"
	case caPrev:
		return "the page before"
	}
	return "next page"
}

// codexElName names a thing: its kind and a little of what it is.
func codexElName(e *codexEl) string {
	if e == nil {
		return "nothing"
	}
	what := ""
	switch e.Kind {
	case ceText, ceButton:
		what = strings.TrimSpace(strings.SplitN(e.Text, "\n", 2)[0])
	case ceImage:
		what = e.Image
	case ceSymbol:
		if len(codexSymbols) > 0 {
			what = codexSymbols[(e.Symbol%len(codexSymbols)+len(codexSymbols))%len(codexSymbols)].Name
		}
	}
	name := codexKindNames[e.Kind]
	if what != "" {
		name += " “" + codexClip(what, 18) + "”"
	}
	return name
}

func codexElHint(e *codexEl) string {
	switch e.Kind {
	case ceText:
		return "Enter write · F font · C colour · A animation · + − size"
	case ceButton:
		return "Enter label · K action · G aim · C colour"
	case ceImage:
		return "Enter picture · A animation"
	case ceSymbol:
		return "S symbol · + − size · C colour · A animation"
	case ceDrawing:
		return "Enter draw · A animation"
	}
	return ""
}

// ---- writing words ------------------------------------------------------------------------

func (m *Model) codexStartWrite(what int) {
	c := m.cx()
	c.writing = what
	switch what {
	case codexWriteTitle:
		c.words = []rune(c.cf.Title)
	case codexWriteSubtitle:
		c.words = []rune(c.cf.Subtitle)
	case codexWriteEl:
		if e := c.el(c.sel); e != nil {
			c.words = []rune(e.Text)
		}
	case codexWriteAlt:
		if e := c.el(c.sel); e != nil {
			c.words = []rune(e.On.Alt)
		}
	}
	c.cur = len(c.words)
	if what == codexWriteEl || what == codexWriteAlt {
		m.say("Writing · Enter: " + map[bool]string{true: "a new line", false: "done"}[c.multiline()] + " · Esc: done.")
	}
}

// multiline is whether Enter makes a new line in what is being written.
func (c *codexState) multiline() bool {
	if c.writing == codexWriteAlt {
		return true
	}
	e := c.el(c.sel)
	return c.writing == codexWriteEl && e != nil && e.Kind == ceText
}

func (m *Model) codexWriteKey(k tea.Key, name string) {
	c := m.cx()
	fresh := c.fresh // a new button's label: typing replaces it; other keys keep it
	c.fresh = false
	switch name {
	case "esc":
		m.codexEndWrite()
		return
	case "tab":
		was := c.writing
		m.codexEndWrite()
		if was == codexWriteTitle {
			m.codexStartWrite(codexWriteSubtitle)
			m.say("The subtitle · Enter: done.")
		}
		return
	case "enter", "ctrl+j":
		if c.multiline() {
			m.codexInsert("\n")
			return
		}
		m.codexEndWrite()
		return
	case "shift+enter", "alt+enter":
		m.codexEndWrite()
		return
	case "left":
		c.cur = max(c.cur-1, 0)
	case "right":
		c.cur = min(c.cur+1, len(c.words))
	case "home":
		for c.cur > 0 && c.words[c.cur-1] != '\n' {
			c.cur--
		}
	case "end":
		for c.cur < len(c.words) && c.words[c.cur] != '\n' {
			c.cur++
		}
	case "up", "down":
		m.codexWriteLine(name == "up")
	case "backspace":
		if c.cur > 0 {
			c.words = append(c.words[:c.cur-1], c.words[c.cur:]...)
			c.cur--
		}
	case "ctrl+backspace", "alt+backspace", "ctrl+w":
		i := c.cur
		for i > 0 && c.words[i-1] == ' ' {
			i--
		}
		for i > 0 && c.words[i-1] != ' ' && c.words[i-1] != '\n' {
			i--
		}
		c.words = append(c.words[:i], c.words[c.cur:]...)
		c.cur = i
	case "delete":
		if c.cur < len(c.words) {
			c.words = append(c.words[:c.cur], c.words[c.cur+1:]...)
		}
	case "space":
		if fresh {
			c.words, c.cur = nil, 0
		}
		m.codexInsert(" ")
	default:
		if k.Text != "" && !k.Mod.Contains(tea.ModCtrl) && !k.Mod.Contains(tea.ModAlt) {
			if fresh {
				c.words, c.cur = nil, 0
			}
			m.codexInsert(k.Text)
		} else if fresh {
			c.fresh = true // a key that does nothing: the label is still new
		}
	}
}

// codexWriteLine moves the cursor to the line above (or below), same column.
func (m *Model) codexWriteLine(up bool) {
	c := m.cx()
	start := c.cur
	for start > 0 && c.words[start-1] != '\n' {
		start--
	}
	col := c.cur - start
	if up {
		if start == 0 {
			c.cur = 0
			return
		}
		prev := start - 1
		for prev > 0 && c.words[prev-1] != '\n' {
			prev--
		}
		c.cur = min(prev+col, start-1)
		return
	}
	end := c.cur
	for end < len(c.words) && c.words[end] != '\n' {
		end++
	}
	if end >= len(c.words) {
		c.cur = len(c.words)
		return
	}
	next := end + 1
	nextEnd := next
	for nextEnd < len(c.words) && c.words[nextEnd] != '\n' {
		nextEnd++
	}
	c.cur = min(next+col, nextEnd)
}

func (m *Model) codexInsert(s string) {
	c := m.cx()
	rs := []rune(s)
	c.words = append(c.words[:c.cur], append(rs, c.words[c.cur:]...)...)
	c.cur += len(rs)
}

// codexEndWrite puts the words where they belong.
// Ctrl+Z gets a step only if something changed; a thing just added and
// written is one step (the one adding it); a new text left empty is taken
// away again.
func (m *Model) codexEndWrite() {
	c := m.cx()
	what := c.writing
	c.writing, c.fresh = 0, false
	words := string(c.words)
	added := c.added
	c.added = 0
	before, _ := json.Marshal(c.cf)
	switch what {
	case codexWriteTitle:
		c.cf.Title = strings.TrimSpace(words)
	case codexWriteSubtitle:
		c.cf.Subtitle = strings.TrimSpace(words)
	case codexWriteEl:
		if e := c.el(c.sel); e != nil {
			e.Text = strings.TrimRight(words, " \n")
			if e.Kind == ceButton && e.Text == "" {
				e.Text = "Button"
			}
		}
	case codexWriteAlt:
		if e := c.el(c.sel); e != nil {
			e.On.Alt = strings.TrimRight(words, " \n")
		}
		c.table = true
	}
	c.words = nil
	if what == codexWriteEl && added != 0 && added == c.sel && m.codexDropAdded(added) {
		m.codexSave()
		m.say("Nothing written, so no text was added.")
		return
	}
	after, _ := json.Marshal(c.cf)
	changed := string(after) != string(before)
	if changed && added == 0 {
		c.codexPushUndo(string(before))
	}
	m.codexSave()
	if changed || added != 0 {
		m.say("Saved.")
	} else {
		m.say("No change.")
	}
}

// codexPaste: text pasted while writing goes in at the cursor.
func (m *Model) codexPaste(s string) tea.Cmd {
	c := m.cx()
	if c.writing == 0 {
		return nil
	}
	if !c.multiline() {
		s = strings.ReplaceAll(s, "\n", " ")
	}
	if c.fresh {
		c.words, c.cur, c.fresh = nil, 0, false
	}
	m.codexInsert(s)
	return nil
}

// ---- drawing ------------------------------------------------------------------------------

func (m *Model) codexStartDraw() {
	c := m.cx()
	c.drawing, c.penDown = true, false
	c.brushX, c.brushY = codexInkW/2, codexInkH/2
	if c.brushC < 0 || int(c.brushC) >= len(codexInkColors) {
		c.brushC = 0
	}
	m.say("Drawing: arrows move the brush · Space pen down / up · 1–8, 9 white, 0 dark · R rubber · B brush size · Esc: done (the mouse draws too).")
}

// codexBrushStep is how many dots one arrow press moves the brush (one cell's worth).
func (m *Model) codexBrushStep() (int, int) {
	c := m.cx()
	r, ok := c.rects[c.sel]
	if !ok || r[2] <= 0 || r[3] <= 0 {
		return 2, 2
	}
	return max(1, codexInkW/r[2]), max(1, codexInkH/(2*r[3]))
}

func (m *Model) codexDrawKey(name string) {
	c := m.cx()
	sx, sy := m.codexBrushStep()
	step := 1
	if strings.HasPrefix(name, "shift+") {
		step = 4
		name = strings.TrimPrefix(name, "shift+")
	}
	moved := false
	switch name {
	case "left":
		c.brushX, moved = max(c.brushX-sx*step, 0), true
	case "right":
		c.brushX, moved = min(c.brushX+sx*step, codexInkW-1), true
	case "up":
		c.brushY, moved = max(c.brushY-sy*step, 0), true
	case "down":
		c.brushY, moved = min(c.brushY+sy*step, codexInkH-1), true
	case "space":
		c.penDown = !c.penDown
		if c.penDown {
			m.codexStep()
			m.codexDab()
		}
	case "1", "2", "3", "4", "5", "6", "7", "8", "9", "0":
		i := int(name[0] - '1')
		if name == "0" {
			i = 9
		}
		c.brushC = int8(i)
		m.say("Colour " + name + ".")
	case "r", "R", "backspace":
		c.brushC = -1
		m.say("Rubber.")
	case "b", "B":
		c.brushSize = c.brushSize%3 + 1
		m.say("Brush size " + itoa(c.brushSize) + ".")
	case "ctrl+z":
		m.codexUndo(false)
	case "enter", "esc":
		m.codexEndDraw()
	}
	if moved && c.penDown {
		m.codexDab()
	}
}

// codexEndDraw stops drawing. A drawing just added and left with no ink is
// taken away again.
func (m *Model) codexEndDraw() {
	c := m.cx()
	c.drawing, c.penDown = false, false
	added := c.added
	c.added = 0
	if added != 0 && added == c.sel && m.codexDropAdded(added) {
		m.codexSave()
		m.say("Nothing drawn, so no drawing was added.")
		return
	}
	m.codexSave()
	m.say("Drawing kept.")
}

// codexDab puts the brush's colour (or the rubber) where the brush is.
func (m *Model) codexDab() {
	c := m.cx()
	e := c.el(c.sel)
	if e == nil {
		return
	}
	sx, sy := m.codexBrushStep()
	bw, bh := sx*c.brushSize, sy*c.brushSize
	x0, y0 := c.brushX-(bw-sx)/2, c.brushY-(bh-sy)/2
	have := map[[2]int]int{}
	for i, d := range e.Ink {
		have[[2]int{d.X, d.Y}] = i
	}
	var gone map[int]bool
	for y := max(y0, 0); y < min(y0+bh, codexInkH); y++ {
		for x := max(x0, 0); x < min(x0+bw, codexInkW); x++ {
			i, ok := have[[2]int{x, y}]
			switch {
			case c.brushC < 0 && ok:
				if gone == nil {
					gone = map[int]bool{}
				}
				gone[i] = true
			case c.brushC >= 0 && ok:
				e.Ink[i].C = c.brushC
			case c.brushC >= 0:
				e.Ink = append(e.Ink, codexInk{x, y, c.brushC})
				have[[2]int{x, y}] = len(e.Ink) - 1
			}
		}
	}
	if gone != nil {
		keep := e.Ink[:0]
		for i, d := range e.Ink {
			if !gone[i] {
				keep = append(keep, d)
			}
		}
		e.Ink = keep
	}
}

// ---- the activation table -------------------------------------------------------------------

func (m *Model) codexTableKey(name string) tea.Cmd {
	c := m.cx()
	p := c.pageOf()
	if p == nil || len(p.Els) == 0 {
		c.table = false
		return nil
	}
	c.tableRow = min(max(c.tableRow, 0), len(p.Els)-1)
	e := p.Els[c.tableRow]
	c.sel = e.ID
	switch name {
	case "esc", "t", "T", "shift+t":
		c.table = false
		m.codexSave()
		m.say("Table closed. Space tries the chosen thing; reading, a click or Enter activates.")
	case "up":
		c.tableRow = max(c.tableRow-1, 0)
	case "down":
		c.tableRow = min(c.tableRow+1, len(p.Els)-1)
	case "tab", "shift+tab":
		c.tableCol = 1 - c.tableCol
	case "left", "right":
		m.codexStep()
		m.codexTableChange(e, name == "right")
		m.codexSave()
	case "enter":
		_, do := codexDoOf(e.On.Do)
		if e.Kind != ceButton && do.value == "words" {
			if e.Kind == ceImage {
				c.table = false
				return m.ask(pCodexAltImage, "The other picture for "+codexElName(e)+": ", "")
			}
			if e.Kind == ceSymbol {
				m.codexStep()
				m.codexTableChange(e, true)
				m.codexSave()
				return nil
			}
			c.table = false
			m.codexStartWrite(codexWriteAlt)
			return nil
		}
		c.tableCol = 1 - c.tableCol
	}
	return nil
}

// codexTableChange steps the chosen column of e's row.
func (m *Model) codexTableChange(e *codexEl, fwd bool) {
	c := m.cx()
	if e.Kind == ceButton { // a button's row: its action, and what it aims at
		if c.tableCol == 0 {
			e.Action = codexNext(codexActions, e.Action, fwd)
			c.codexActionTarget(e)
		} else if e.Action == caGoto || e.Action == caActivate {
			m.codexAim(e, fwd)
		}
		return
	}
	i, do := codexDoOf(e.On.Do)
	if c.tableCol == 0 {
		for range codexDos { // only what does something for this kind of thing
			i = codexCycle(i, len(codexDos), fwd)
			if codexDoFits(e.Kind, codexDos[i].key) {
				break
			}
		}
		e.On.Do = codexDos[i].key
		_, do = codexDoOf(e.On.Do)
		// A sensible value to start with.
		switch do.value {
		case "colour":
			if e.On.Color == "" {
				e.On.Color = codexPalette[8]
			}
		case "anim":
			if e.On.Anim == 0 {
				e.On.Anim = 3
			}
		case "size":
			if e.On.Size == 0 {
				e.On.Size = min(max(e.Size, 1)+1, codexSizes)
			}
		case "font":
			if e.On.Font == 0 {
				e.On.Font = 1
			}
		}
		return
	}
	switch do.value {
	case "colour":
		e.On.Color = codexNext(codexPalette, e.On.Color, fwd)
	case "anim":
		e.On.Anim = codexCycle(e.On.Anim-1, 15, fwd) + 1
	case "font":
		e.On.Font = codexCycle(e.On.Font, 16, fwd)
	case "size":
		e.On.Size = codexCycle(max(e.On.Size, 1)-1, codexSizes, fwd) + 1
	case "words":
		if e.Kind == ceSymbol {
			n := 0
			fmt.Sscan(e.On.Alt, &n)
			e.On.Alt = itoa(codexCycle(n, len(codexSymbols), fwd))
		}
	}
}

// codexDoFits is whether activation do changes a thing of this kind: a font
// is only for words, a size for words and symbols, a colour not for a
// picture, swapping not for a drawing.
func codexDoFits(kind, do string) bool {
	switch do {
	case "font":
		return kind == ceText || kind == ceButton
	case "grow":
		return kind == ceText || kind == ceSymbol
	case "colour":
		return kind != ceImage
	case "swap":
		return kind != ceDrawing
	}
	return true
}

// codexTableValue says the value column of e's row.
func codexTableValue(c *codexState, e *codexEl) string {
	if e.Kind == ceButton {
		switch e.Action {
		case caGoto, caActivate:
			return codexButtonSays(c, e)
		}
		return "—"
	}
	_, do := codexDoOf(e.On.Do)
	switch do.value {
	case "colour":
		return "● " + e.On.Color
	case "anim":
		return codexAnims[max(e.On.Anim, 1)%16]
	case "font":
		return codexFont(codexFonts[e.On.Font%16], e.On.Font)
	case "size":
		return "size " + itoa(max(e.On.Size, 1))
	case "words":
		switch e.Kind {
		case ceImage:
			if e.On.Alt == "" {
				return "Enter: choose the other picture"
			}
			return e.On.Alt
		case ceSymbol:
			n := 0
			fmt.Sscan(e.On.Alt, &n)
			return codexSymbols[(n%len(codexSymbols)+len(codexSymbols))%len(codexSymbols)].Name
		}
		if e.On.Alt == "" {
			return "Enter: write the other words"
		}
		return "“" + strings.ReplaceAll(e.On.Alt, "\n", " ") + "”"
	}
	return "—"
}

// codexTableDraw draws the activation table over the page.
func (m *Model) codexTableDraw(cv *boardCanvas) {
	c := m.cx()
	p := c.pageOf()
	if p == nil {
		return
	}
	w := min(cv.Width()-4, 96)
	h := min(len(p.Els)+5, cv.Height()-2)
	x, y := (cv.Width()-w)/2, (cv.Height()-h)/2
	bg := mixHex(cBarBG, "#000000", 0.2)
	edge := cAccent
	cvFill(cv, x, y, w, h, bg)
	for xx := x; xx < x+w; xx++ {
		cvSet(cv, xx, y, "━", edge, bg, 0)
		cvSet(cv, xx, y+h-1, "━", edge, bg, 0)
	}
	cvMiddle(cv, x, y, w, " Activation table · page "+itoa(c.page+1)+" ", edge, bg, 1)
	c1, c2, c3 := x+2, x+2+w*36/100, x+2+w*66/100
	cvText(cv, c1, y+1, "THING", mixHex(bg, cText, 0.55), bg, 0)
	cvText(cv, c2, y+1, "WHEN ACTIVATED IT…", mixHex(bg, cText, 0.55), bg, 0)
	cvText(cv, c3, y+1, "TO", mixHex(bg, cText, 0.55), bg, 0)
	first := max(c.tableRow-(h-6), 0)
	for i := first; i < len(p.Els) && y+2+i-first < y+h-2; i++ {
		e := p.Els[i]
		ry := y + 2 + i - first
		rowBG := bg
		if i == c.tableRow {
			rowBG = mixHex(bg, cAccent, 0.2)
			cvFill(cv, x+1, ry, w-2, 1, rowBG)
		}
		cvText(cv, c1, ry, codexClip(codexKindIcons[e.Kind]+" "+codexElName(e), c2-c1-1), cText, rowBG, 0)
		do := ""
		if e.Kind == ceButton {
			do = "does: " + codexActionNames[codexOr(e.Action, caNext)]
		} else {
			_, d := codexDoOf(e.On.Do)
			do = d.name
		}
		val := codexTableValue(c, e)
		f1, f2 := cText, cText
		var a1, a2 uint8
		if i == c.tableRow {
			if c.tableCol == 0 {
				f1, a1, do = cAccent, 1, "‹ "+do+" ›"
			} else {
				f2, a2, val = cAccent, 1, "‹ "+val+" ›"
			}
		}
		cvText(cv, c2, ry, codexClip(do, c3-c2-1), f1, rowBG, a1)
		cvText(cv, c3, ry, codexClip(val, x+w-2-c3), f2, rowBG, a2)
	}
	cvMiddle(cv, x, y+h-2, w, "↑↓ thing · ←→ change · Tab column · Enter: words / picture · Esc close", mixHex(bg, cText, 0.6), bg, 0)
}

func codexOr(s, or string) string {
	if s == "" {
		return or
	}
	return s
}

// ---- pictures ---------------------------------------------------------------------------------

// codexUseImage copies picture path into the codex and gives it to the
// chosen thing (alt: as the picture it swaps to).
func (m *Model) codexUseImage(path string, alt bool) tea.Cmd {
	c := m.cx()
	pic := c.newPic // Alt+N, I: the new picture is only added now
	c.newPic = nil
	if alt {
		pic = nil
	}
	e := c.el(c.sel)
	if c.cf == nil || (e == nil && pic == nil) || (pic != nil && c.pageOf() == nil) {
		return nil
	}
	if alt {
		c.table = true
	}
	if _, ok := loadPicture(path); !ok {
		m.sayErr("That is not a picture sy can show (PNG, JPEG, GIF, WebP…). Nothing changed.")
		return nil
	}
	name, err := codexCopyIn(c.id, path)
	if err != nil {
		m.sayErr("Could not take that picture: " + err.Error())
		return nil
	}
	m.codexStep()
	if pic != nil {
		p := c.pageOf()
		e = c.newEl(ceImage, pic.slot)
		at := min(max(pic.at, 0), len(p.Els))
		p.Els = append(p.Els[:at], append([]*codexEl{e}, p.Els[at:]...)...)
		c.sel = e.ID
	}
	if alt {
		e.On.Alt = name
	} else {
		e.Image = name
	}
	m.codexSave()
	m.say("Picture “" + name + "” kept in the codex.")
	return nil
}

// codexCopyIn copies a file into codex id's images folder and gives its name there.
func codexCopyIn(id, path string) (string, error) {
	src, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer src.Close()
	if info, err := src.Stat(); err != nil || info.IsDir() {
		return "", fmt.Errorf("not a file")
	}
	dir := filepath.Join(codexDir(id), "images")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	name := base
	for n := 2; ; n++ {
		if _, err := os.Stat(filepath.Join(dir, name)); os.IsNotExist(err) {
			break
		}
		name = stem + "-" + itoa(n) + ext
	}
	dst, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return "", err
	}
	return name, dst.Close()
}

// ---- the mouse ----------------------------------------------------------------------------------

func codexIn(r [4]int, x, y int) bool {
	return x >= r[0] && y >= r[1] && x < r[0]+r[2] && y < r[1]+r[3]
}

// codexClick is a click at (x, y) on the screen.
func (m *Model) codexClick(ms tea.Mouse) tea.Cmd {
	c := m.cx()
	c.newPic = nil
	x, y := ms.X, ms.Y-m.boardTop()
	if c.cf == nil {
		for i := range c.lib {
			if r, ok := c.box["lib:"+itoa(i)]; ok && codexIn(r, x, y) {
				if c.libSel == i {
					m.codexOpen(c.lib[i].id, false)
				} else {
					c.libSel = i
				}
				return m.codexTickCmd()
			}
		}
		return nil
	}
	if c.writing != 0 {
		m.codexEndWrite()
	}
	if c.table {
		return nil
	}
	if c.drawing {
		if r, ok := c.rects[c.sel]; ok && codexIn(r, x, y) {
			m.codexStep()
			m.codexMouseDab(r, x, y)
			return nil
		}
		m.codexEndDraw()
	}
	if c.page < 0 {
		if r, ok := c.box["cover"]; ok && codexIn(r, x, y) && !c.edit {
			m.codexTurnTo(0)
			return m.codexTickCmd()
		}
	}
	if p := c.pageOf(); p != nil {
		for i := len(p.Els) - 1; i >= 0; i-- {
			e := p.Els[i]
			if r, ok := c.rects[e.ID]; ok && codexIn(r, x, y) {
				if c.edit {
					c.sel = e.ID
					m.say(codexElName(e) + " · " + codexElHint(e))
				} else {
					c.focus = e.ID
					m.codexActivate(e, 0)
				}
				return m.codexTickCmd()
			}
		}
	}
	if r, ok := c.box["prev"]; ok && codexIn(r, x, y) {
		m.codexTurnTo(c.page - 1)
	} else if r, ok := c.box["next"]; ok && codexIn(r, x, y) {
		m.codexTurnTo(c.page + 1)
	}
	return m.codexTickCmd()
}

// codexDrag: the mouse moved with its button down: drawing, it draws.
func (m *Model) codexDrag(ms tea.Mouse) tea.Cmd {
	c := m.cx()
	if !c.drawing {
		return nil
	}
	x, y := ms.X, ms.Y-m.boardTop()
	if r, ok := c.rects[c.sel]; ok && codexIn(r, x, y) {
		m.codexMouseDab(r, x, y)
	}
	return nil
}

// codexMouseDab puts the brush where the mouse is in drawing r.
func (m *Model) codexMouseDab(r [4]int, x, y int) {
	c := m.cx()
	c.brushX = (x - r[0]) * codexInkW / r[2]
	c.brushY = (y - r[1]) * codexInkH / r[3]
	m.codexDab()
}

func (m *Model) codexWheel(up bool) tea.Cmd {
	c := m.cx()
	if c.cf == nil {
		if up {
			c.libSel = max(c.libSel-1, 0)
		} else {
			c.libSel = min(c.libSel+1, max(len(c.lib)-1, 0))
		}
		return nil
	}
	if c.writing != 0 || c.table || c.drawing {
		return nil
	}
	if up {
		m.codexTurnTo(c.page - 1)
	} else {
		m.codexTurnTo(c.page + 1)
	}
	return m.codexTickCmd()
}

// ---- the bars ---------------------------------------------------------------------------------

func (m *Model) codexTopBar() string {
	c := m.cx()
	bar := st(cStatusText, cBarBG)
	left := " " + modes[codexMode].Icon() + " Codex"
	right := itoa(len(c.lib)) + map[bool]string{true: " codex", false: " codexes"}[len(c.lib) == 1] + " in the library"
	if c.cf != nil {
		where := "the cover"
		if c.page >= 0 {
			where = "page " + itoa(c.page+1) + " of " + itoa(len(c.cf.Pages))
			if c.edit {
				where += " · layout " + itoa(c.cf.Pages[c.page].Layout+1) + ": " + codexLayouts[c.cf.Pages[c.page].Layout].name
			}
		}
		mode := "reading"
		if c.edit {
			mode = "editing"
		}
		right = "“" + codexName(c.cf.Title) + "” · " + where + " · " + mode
	}
	return centreBar(bar.Bold(true).Render(left)+bar.Render("  ·  "+right), m.width, bar)
}

// codexStrip is the row under the page: the pages as dots, and the chosen thing.
func (m *Model) codexStrip() string {
	c := m.cx()
	bar := st(cStatusText, cBarBG)
	if c.cf == nil {
		return centreBar(bar.Render("Enter open · E edit · Alt+N new · P put on the Design board · X X delete"), m.width, bar)
	}
	var sb strings.Builder
	dot := func(on bool, s string) {
		if on {
			sb.WriteString(st(cAccent, cBarBG).Bold(true).Render(s))
		} else {
			sb.WriteString(bar.Render(s))
		}
	}
	dot(c.page < 0, "▥ ")
	for i := range c.cf.Pages {
		if i > 40 {
			sb.WriteString(bar.Render("…"))
			break
		}
		g := "○ "
		if i == c.page {
			g = "● "
		}
		dot(i == c.page, g)
	}
	if e := c.el(c.sel); c.edit && e != nil {
		info := "  " + codexElName(e)
		switch e.Kind {
		case ceText, ceButton:
			info += " · " + codexFonts[e.Font%16]
		}
		if e.Kind != ceButton && e.Kind != ceImage && e.Kind != ceDrawing {
			info += " · size " + itoa(max(e.Size, 1))
		}
		info += " · " + codexAnims[e.Anim%16]
		if e.Kind == ceButton {
			info += " · " + codexButtonSays(c, e)
		} else {
			_, do := codexDoOf(e.On.Do)
			info += " · activated: " + do.name
		}
		sb.WriteString(bar.Render(info))
	}
	return centreBar(sb.String(), m.width, bar)
}

func (m *Model) codexStatus() string {
	c := m.cx()
	var hint string
	switch {
	case c.err != "":
		hint = "✗ not saved: " + c.err
	case c.cf == nil:
		hint = "←→↑↓ choose · Enter open · Alt+N new codex · F1: all keys"
	case c.writing != 0:
		hint = "Writing · Enter: " + map[bool]string{true: "new line", false: "done"}[c.multiline()] + " · Esc: done"
	case c.drawing:
		hint = "Drawing · arrows brush · Space pen " + map[bool]string{true: "up", false: "down"}[c.penDown] + " · 1–9 0 colour · R rubber · B size · Esc done"
	case c.table:
		hint = "Activation table · ↑↓ · ←→ change · Tab column · Esc close"
	case c.placing:
		hint = "Add: T text · B button · I picture · S symbol · D drawing"
	case c.edit && c.page < 0:
		hint = "Cover · Enter title · Tab subtitle · C colour · S symbol · W paper · PgDn pages · E read · Esc close"
	case c.edit:
		hint = "Alt+N add · arrows choose · Enter write · L layout · F C A + − font colour anim size · B border · Alt+arrows move · T table · Alt+P page · E read · Esc close"
	case c.page < 0:
		hint = "→ or Enter: open · E edit · Esc close"
	default:
		hint = "← → pages · Tab · Enter press · click a thing · E edit · Esc close"
	}
	style := stStatus
	if c.err != "" {
		style = st(cError, cStatusBG)
	}
	return centreBar(style.Render(hint), m.width, stStatus)
}

// codexActionTarget gives a button a target that fits its new action: the
// next page for "go to", nothing yet for "activate".
func (c *codexState) codexActionTarget(e *codexEl) {
	switch e.Action {
	case caGoto:
		e.Target = min(c.page+2, len(c.cf.Pages))
	default:
		e.Target = 0
	}
}
