package main

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// The central zone: the text sits in a column in the middle of the screen.
// Code inside it stays left-aligned. Each side has a margin with a thin
// handle line; dragging either handle with the mouse makes the zone wider
// or narrower (both sides move, so it stays centred). The width is
// remembered. On a window too narrow for margins the text uses all of it.

const (
	defaultZone = 100 // letters across, not counting line numbers
	minZone     = 20
)

// zone gives the margins either side of the text, and the text's width.
func (m *Model) zone() (left, text, right int) {
	avail := m.width - m.treeWidth()
	gw := m.gutterWidth()
	want := m.zoneWidth
	if want <= 0 {
		want = defaultZone
	}
	if gw+want+6 > avail { // no room for margins worth having
		return 0, max(avail-gw, 1), 0
	}
	left = (avail - gw - want) / 2
	return left, want, avail - gw - want - left
}

// marginHandles are the screen columns of the two handle lines.
func (m *Model) marginHandles() (leftX, rightX int, ok bool) {
	left, text, right := m.zone()
	if left == 0 || right == 0 {
		return 0, 0, false
	}
	base := m.treeWidth()
	return base + left - 1, base + left + m.gutterWidth() + text, true
}

// marginRow draws one row of a margin, with its handle on the inner edge.
type marginKey struct {
	width, startX, y, h int
	right               bool
	drag                string
	bg                  string
}

var marginRows = map[marginKey]string{}

func (m *Model) marginRow(width, startX, y, h int, handleRight bool) string {
	if width <= 0 {
		return ""
	}
	k := marginKey{width, startX, y, h, handleRight, m.dragMargin, cBG + cGutterLine + cDim + cEmber}
	if s, ok := marginRows[k]; ok {
		return s
	}
	if len(marginRows) > 5000 {
		marginRows = map[marginKey]string{}
	}
	s := m.marginRowNew(width, startX, y, h, handleRight)
	marginRows[k] = s
	return s
}

func (m *Model) marginRowNew(width, startX, y, h int, handleRight bool) string {
	// The margins keep the plain text background: in kitty that colour is
	// see-through, so its picture shows; any grain here would be solid squares.
	cells := func(n int) string {
		return sgr(cText, cBG, false, false, false) + strings.Repeat(" ", n) + ansiReset
	}
	handle := "│"
	fg := cGutterLine
	if y >= h/2-1 && y <= h/2+1 { // a grip in the middle shows it can be dragged
		handle, fg = "┃", cDim
	}
	if m.dragMargin != "" {
		handle, fg = "┃", cEmber
	}
	line := lipgloss.NewStyle().Background(hex(cBG)).Foreground(hex(fg)).Render(handle)
	if handleRight {
		return cells(width-1) + line
	}
	return line + cells(width-1)
}

// withMargins puts the margins around each text row.
func (m *Model) withMargins(rows []string) []string {
	left, _, right := m.zone()
	if left == 0 && right == 0 {
		return rows
	}
	out := make([]string, len(rows))
	for y, r := range rows {
		out[y] = m.marginRow(left, 0, y+textTop, len(rows), true) + r +
			m.marginRow(right, m.width-right, y+textTop, len(rows), false)
	}
	return out
}

// marginPress starts a drag when a handle is pressed. It returns false when
// the press was somewhere else.
func (m *Model) marginPress(x, y int) bool {
	lx, rx, ok := m.marginHandles()
	if !ok || y < textTop || y >= textTop+m.textHeight() {
		return false
	}
	switch {
	case x >= lx-1 && x <= lx:
		m.dragMargin = "left"
	case x >= rx && x <= rx+1:
		m.dragMargin = "right"
	default:
		return false
	}
	return true
}

// marginDrag follows the mouse: the dragged handle goes where the mouse
// is, and the other margin becomes the same width so the zone stays centred.
func (m *Model) marginDrag(x int) {
	base := m.treeWidth()
	avail := m.width - base
	gw := m.gutterWidth()
	margin := x - base + 1 // left handle: the margin ends at the mouse
	if m.dragMargin == "right" {
		margin = avail - (x - base) // right handle: the margin starts at the mouse
	}
	m.zoneWidth = max(minZone, min(avail-gw-2*margin, avail-gw-6))
	m.ensureVisible()
}

func (m *Model) marginRelease() tea.Cmd {
	m.dragMargin = ""
	saveSetting("zone-width", strconv.Itoa(m.zoneWidth))
	left, text, _ := m.zone()
	if left > 0 {
		m.say("Central zone: " + strconv.Itoa(text) + " letters wide. Remembered for next time.")
	}
	return nil
}

// Centred lines: prose (plain text, Markdown, sessions) is centred line by
// line inside the zone, like a page; code stays left-aligned so indents line
// up. Alt+J switches it for the file on screen.

// centred says whether the lines of the file on screen are centred.
func (m *Model) centred() bool {
	if m.buf.Centre != nil {
		return *m.buf.Centre
	}
	switch m.hl.Name {
	case "Text", "Markdown", "plaintext", "markdown":
		return true
	}
	return false
}

// lineOffset is how far line r is moved right to sit in the middle.
func (m *Model) lineOffset(r int) int {
	if !m.centred() || m.left > 0 || r < 0 || r >= len(m.buf.Lines) {
		return 0
	}
	l := m.buf.Lines[r]
	w := colToX(l, len(l))
	if tw := m.textWidth(); w < tw-1 {
		return (tw - 1 - w) / 2 // keep one column free for the cursor at the end
	}
	return 0
}

func (m *Model) toggleCentre() tea.Cmd {
	on := !m.centred()
	m.buf.Centre = &on
	if on {
		m.say("Lines centred in this file. Alt+J puts them back on the left.")
	} else {
		m.say("Lines on the left in this file. Alt+J centres them.")
	}
	return nil
}
