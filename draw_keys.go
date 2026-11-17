package main

// ---- Draw mode's keys and mouse (draw.go) ----------------------------------------------------
//
// The start screen:
//
//	↑ ↓ · Home · End · PgUp · PgDn   choose      ← →   the drawings · the notes
//	Enter · double-click             open it     N     a new drawing
//	X X                              delete the chosen drawing
//
// Drawing:
//
//	the mouse      drag draws, a click makes a dot (a click on the tray picks a pen)
//	1 – 7          pen · fineliner · pencil · marker · highlighter · glitter pen · brush
//	C · Shift+C    the pen's next · last colour      [ · ]   thinner · thicker
//	E or R         the rubber, on and off
//	arrows         move the brush (Shift: one dot)    Space   pen down · up
//	Ctrl+Z · Y     undo · redo a stroke               PgUp · PgDn · wheel   scroll a note
//	Esc            done: back to the start screen (or the notepad it came from)

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

func (m *Model) drawKey(k tea.Key) tea.Cmd {
	name := keyName(k)
	if name == "f1" {
		m.help, m.helpTop = true, helpLineOf("Draw mode") // help.go: at Draw mode's keys
		return nil
	}
	dw := m.dw()
	if dw.f == nil {
		return m.drawStartKey(name)
	}
	return m.drawCanvasKey(name)
}

// ---- the start screen ------------------------------------------------------------------------

func (m *Model) drawStartKey(name string) tea.Cmd {
	dw := m.dw()
	if name != "x" && name != "X" && name != "shift+x" {
		dw.xAt = time.Time{}
	}
	switch name {
	case "up":
		m.drawStartMove(-1)
	case "down":
		m.drawStartMove(1)
	case "pgup":
		m.drawStartMove(-8)
	case "pgdown":
		m.drawStartMove(8)
	case "home":
		m.drawStartMove(-1 << 20)
	case "end":
		m.drawStartMove(1 << 20)
	case "left":
		dw.col = 0
	case "right", "tab":
		if len(dw.notes) > 0 {
			dw.col = 1
			dw.fixNoteSel(1)
		}
	case "enter":
		return m.drawStartOpen()
	case "n", "N", "shift+n", "alt+n":
		m.drawOpenNew()
	case "x", "X", "shift+x":
		if dw.col != 0 || dw.sel[0] == 0 || dw.sel[0] > len(dw.drawings) {
			m.say("X X deletes the chosen drawing. A note's ink: open the note and Ctrl+Z it away.")
			return nil
		}
		e := dw.drawings[dw.sel[0]-1]
		if !dw.xAt.IsZero() && time.Since(dw.xAt) < 2*time.Second {
			dw.xAt = time.Time{}
			m.drawDelete(e)
			return nil
		}
		dw.xAt = time.Now()
		m.say("X again deletes “" + e.f.Name + "”.")
	case "esc":
		m.say("Alt+M switches modes · Alt+Q quits · N makes a drawing.")
	}
	return nil
}

// drawStartMove moves the choice by n rows in the column chosen.
func (m *Model) drawStartMove(n int) {
	dw := m.dw()
	if dw.col == 0 {
		dw.sel[0] = min(max(dw.sel[0]+n, 0), len(dw.drawings))
		return
	}
	if len(dw.notes) == 0 {
		return
	}
	dir := 1
	if n < 0 {
		dir = -1
	}
	at := dw.sel[1]
	for step := 0; step < abs(n); step++ {
		next := at + dir
		for next >= 0 && next < len(dw.notes) && dw.notes[next].note == nil {
			next += dir
		}
		if next < 0 || next >= len(dw.notes) {
			break
		}
		at = next
	}
	dw.sel[1] = at
}

// drawStartOpen opens what is chosen on the start screen.
func (m *Model) drawStartOpen() tea.Cmd {
	dw := m.dw()
	if dw.col == 1 {
		if dw.sel[1] < len(dw.notes) && dw.notes[dw.sel[1]].note != nil {
			dw.fromDesign = false
			m.drawOpenNote(dw.notes[dw.sel[1]].note)
		}
		return nil
	}
	if dw.sel[0] == 0 {
		m.drawOpenNew()
		return nil
	}
	if dw.sel[0] <= len(dw.drawings) {
		m.drawOpenDrawing(dw.drawings[dw.sel[0]-1])
	}
	return nil
}

// ---- drawing -----------------------------------------------------------------------------------

func (m *Model) drawCanvasKey(name string) tea.Cmd {
	dw := m.dw()
	switch name {
	case "esc":
		if dw.penDown {
			dw.penDown = false
			m.drawEnd()
			m.say("Pen up.")
			return nil
		}
		return m.drawClose()
	case "1", "2", "3", "4", "5", "6", "7":
		m.drawPick(trayDevs[int(name[0]-'1')])
	case "c", "C", "shift+c":
		m.drawColour(name == "c")
	case "[", "]":
		m.drawSize(map[bool]int{true: 1, false: -1}[name == "]"])
	case "e", "E", "shift+e", "r", "R", "shift+r":
		m.drawPenChange(func(p *drawPen) { p.Rubber = !p.Rubber })
		if dw.pen.Rubber {
			m.say("The rubber: drag over ink to take it away · E again: back to the " + devNames[dw.pen.Dev] + ".")
		} else {
			m.say("Back to the " + devNames[dw.pen.Dev] + ".")
		}
	case "ctrl+z":
		m.drawUndo()
	case "ctrl+y", "ctrl+shift+z":
		m.drawRedo()
	case "space":
		m.drawSpace()
	case "up", "down", "left", "right", "shift+up", "shift+down", "shift+left", "shift+right":
		m.drawBrushMove(name)
	case "pgup":
		m.drawScroll(-max(m.boardH()-drawTrayH-2, 1))
	case "pgdown":
		m.drawScroll(max(m.boardH()-drawTrayH-2, 1))
	}
	return nil
}

// drawPenChange changes the pen in hand and remembers it. A stroke being
// drawn by the keys goes on with the new pen.
func (m *Model) drawPenChange(change func(p *drawPen)) {
	dw := m.dw()
	if dw.drawing {
		m.drawEnd()
	}
	change(&dw.pen)
	dw.pen.save()
	if dw.penDown {
		m.drawBegin(dw.bx, dw.by)
	}
}

// drawPick takes device dev in hand (the rubber is put down).
func (m *Model) drawPick(dev int) {
	m.drawPenChange(func(p *drawPen) { p.Dev, p.Rubber = dev, false })
	m.say("In hand: the " + inkName(m.dw().pen.ink(dev)) + " " + devNames[dev] + ". C: its colour · [ ]: its size.")
}

// drawColour gives the pen in hand its next (or last) colour of the pens' colours.
func (m *Model) drawColour(next bool) {
	dw := m.dw()
	dev := dw.pen.Dev
	now, at := dw.pen.ink(dev), -1
	for i, k := range penInks {
		if k.hex == now {
			at = i
		}
	}
	step := 1
	if !next {
		step = len(penInks) - 1
	}
	at = (at + step + len(penInks)) % len(penInks)
	m.drawPenChange(func(p *drawPen) {
		p.Inks[devNames[dev]] = penInks[at].hex
		p.Rubber = false
	})
	m.say("The " + devNames[dev] + " is " + penInks[at].name + " now.")
}

// drawSize makes the pen thicker (1) or thinner (-1).
func (m *Model) drawSize(by int) {
	m.drawPenChange(func(p *drawPen) { p.Size = min(max(p.Size+by, 1), drawMaxSize) })
	m.say("Size " + itoa(m.dw().pen.Size) + " of " + itoa(drawMaxSize) + ".")
}

// drawSpace puts the keyboard's pen down, or lifts it.
func (m *Model) drawSpace() {
	dw := m.dw()
	dw.brushShown = true
	if dw.penDown {
		dw.penDown = false
		m.drawEnd()
		m.say("Pen up. Space puts it down again.")
		return
	}
	dw.penDown = true
	m.drawBegin(dw.bx, dw.by)
	m.say("Pen down: the arrows draw. Space lifts it.")
}

// drawBrushMove moves the keyboard's brush a cell (Shift: a dot); with the pen down it draws.
func (m *Model) drawBrushMove(name string) {
	dw := m.dw()
	dw.brushShown = true
	sx, sy := 2, 4
	if len(name) > 6 && name[:6] == "shift+" {
		sx, sy, name = 1, 1, name[6:]
	}
	switch name {
	case "up":
		dw.by -= sy
	case "down":
		dw.by += sy
	case "left":
		dw.bx -= sx
	case "right":
		dw.bx += sx
	}
	// The brush stays on the canvas.
	w, h := m.drawCanvasSize()
	ox, oy := m.drawOrigin()
	dw.bx = min(max(dw.bx, -ox*2), (w-ox)*2-1)
	if dw.f.Note == "" {
		dw.by = min(max(dw.by, -oy*4), (h-oy)*4-1)
	} else {
		dw.by = max(dw.by, -4)
		if cy := oy + floorDiv(dw.by, 4); cy >= h { // a note scrolls to keep it in sight
			dw.scroll += cy - h + 1
		} else if cy < 0 {
			dw.scroll = max(dw.scroll+cy, 0)
		}
	}
	if dw.penDown {
		m.drawTo(dw.bx, dw.by)
	}
}

// drawScroll scrolls a note by n rows.
func (m *Model) drawScroll(n int) {
	dw := m.dw()
	if dw.f == nil || dw.f.Note == "" {
		return
	}
	_, h := m.drawCanvasSize()
	dw.scroll = min(max(dw.scroll+n, 0), max(len(dw.text)-h/2, 0))
}

// ---- the mouse ---------------------------------------------------------------------------------

// drawDot is the dot under screen cell (x, y), in the ink's dots, and
// whether that cell is on the canvas.
func (m *Model) drawDot(x, y int) (int, int, bool) {
	w, h := m.drawCanvasSize()
	cy := y - m.boardTop()
	ox, oy := m.drawOrigin()
	return (x - ox) * 2, (cy-oy)*4 + 1, x >= 0 && x < w && cy >= 0 && cy < h
}

func (m *Model) drawPress(ms tea.Mouse) tea.Cmd {
	dw := m.dw()
	if dw.f == nil {
		return m.drawStartClick(ms.X, ms.Y-m.boardTop())
	}
	if ms.Button != tea.MouseLeft {
		return nil
	}
	_, ch := m.drawCanvasSize()
	if cy := ms.Y - m.boardTop(); cy >= ch && cy < ch+drawTrayH { // the tray
		switch dev := m.drawTrayAt(ms.X); {
		case dev >= 0:
			m.drawPick(dev)
		case dev == -2:
			m.drawCanvasKey("e")
		}
		return nil
	}
	x, y, ok := m.drawDot(ms.X, ms.Y)
	if !ok {
		return nil
	}
	dw.brushShown, dw.penDown = false, false
	m.drawBegin(x, y)
	return nil
}

func (m *Model) drawMotion(ms tea.Mouse) tea.Cmd {
	dw := m.dw()
	if dw.f == nil || !dw.drawing || dw.penDown {
		return nil
	}
	x, y, _ := m.drawDot(ms.X, ms.Y)
	m.drawTo(x, y)
	return nil
}

func (m *Model) drawRelease(ms tea.Mouse) tea.Cmd {
	dw := m.dw()
	if dw.f == nil || !dw.drawing || dw.penDown {
		return nil
	}
	x, y, _ := m.drawDot(ms.X, ms.Y)
	m.drawTo(x, y)
	m.drawEnd()
	return nil
}

func (m *Model) drawWheel(up bool) tea.Cmd {
	dw := m.dw()
	n := 3
	if up {
		n = -3
	}
	if dw.f == nil {
		m.drawStartMove(n / 3)
		return nil
	}
	m.drawScroll(n)
	return nil
}

// drawStartClick chooses the row clicked on the start screen; twice opens it.
func (m *Model) drawStartClick(x, y int) tea.Cmd {
	dw := m.dw()
	for _, h := range drawHits {
		if x < h.x0 || x >= h.x1 || y < h.y0 || y >= h.y1 {
			continue
		}
		again := dw.col == h.col && dw.sel[h.col] == h.row && time.Since(dw.lastPick) < 400*time.Millisecond
		dw.col, dw.sel[h.col], dw.lastPick = h.col, h.row, time.Now()
		if again {
			dw.lastPick = time.Time{}
			return m.drawStartOpen()
		}
		return nil
	}
	return nil
}
