package main

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Modes: Symbolizer does more than one kind of work. Alt+M shows them all in
// a box over the screen. Text is the editor; the others are empty for now.

type mode struct {
	icon, plainIcon string
	name            string
	what            string
	ready           bool
}

var modes = []mode{
	{"", "✎", "Text", "write and edit files: colours, tabs, the file manager", true},
	{"\U000f03d8", "◧", "Design", "draw with the terminal as the canvas: a palette of colours and characters, and resources to reuse", false},
	{"\U000f0428", "▤", "Slides", "make and show slides made of text, one screen at a time", false},
	{"\uf02d", "▣", "Diary", "a diary: writing kept by date, a chapter a day, one page in focus", true},
	{"\U000f05da", "▥", "Book", "write books: chapters, pages and so on", false},
	{"\U000f0f49", "✐", "Draw", "draw with the terminal itself as the drawing tool", false},
}

// modeWords are the names sy takes on the command line: sy design, sy slide ...
var modeWords = map[string]int{"text": 0, "design": 1, "slide": 2, "slides": 2, "diary": 3, "book": 4, "draw": 5}

// Icons come from the Nerd Font symbols kitty (and some other terminals)
// carry. Elsewhere plain symbols are used so nothing shows as an empty box.
// SY_ICONS=plain or SY_ICONS=nerd chooses by hand.
var nerdIcons = func() bool {
	switch os.Getenv("SY_ICONS") {
	case "plain":
		return false
	case "nerd":
		return true
	}
	term := strings.ToLower(os.Getenv("TERM") + " " + os.Getenv("TERM_PROGRAM"))
	return strings.Contains(term, "kitty") || strings.Contains(term, "wezterm") || strings.Contains(term, "ghostty")
}()

func icon(nerd, plain string) string {
	if nerdIcons {
		return nerd
	}
	return plain
}

func (md mode) Icon() string { return icon(md.icon, md.plainIcon) }

// LeftIcon is the icon flipped left to right, for the mode block on the left
// of the top panel, so the two icons face each other across the title. The
// flipped icons are in the "Symbolizer Mirror" font (tools/mirror_font.py);
// without it both sides show the icon as it is.
func (md mode) LeftIcon() string {
	if !nerdIcons || !mirrorFont {
		return md.Icon()
	}
	var sb strings.Builder
	for _, r := range md.icon {
		sb.WriteRune(mirrored(r))
	}
	return sb.String()
}

// mirrored is where tools/mirror_font.py puts the flipped copy of icon r.
func mirrored(r rune) rune {
	if r < 0x10000 {
		return 0x100000 + r
	}
	return 0x100000 + (r - 0xF0000)
}

// mirrorFont: is the Symbolizer Mirror font installed?
var mirrorFont = func() bool {
	home, _ := os.UserHomeDir()
	for _, p := range []string{home + "/.local/share/fonts/SymbolizerMirror.ttf", "/usr/share/fonts/TTF/SymbolizerMirror.ttf"} {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}()

// ---- the Alt+M box -----------------------------------------------------------------

func (m *Model) openModes() tea.Cmd {
	m.help, m.picker = false, nil
	m.closePrompt()
	m.modesOpen = !m.modesOpen
	m.modeSel = m.mode
	return nil
}

func (m *Model) modesKey(name, text string) tea.Cmd {
	switch name {
	case "esc", "alt+m":
		m.modesOpen = false
	case "left", "shift+tab":
		m.modeSel = (m.modeSel + len(modes) - 1) % len(modes)
	case "right", "tab":
		m.modeSel = (m.modeSel + 1) % len(modes)
	case "up":
		if m.modeSel-m.modeCols() >= 0 {
			m.modeSel -= m.modeCols()
		}
	case "down":
		if m.modeSel+m.modeCols() < len(modes) {
			m.modeSel += m.modeCols()
		}
	case "enter":
		return m.setMode(m.modeSel)
	default:
		if len(text) == 1 && text[0] >= '1' && int(text[0]-'1') < len(modes) {
			return m.setMode(int(text[0] - '1'))
		}
	}
	return nil
}

func (m *Model) setMode(i int) tea.Cmd {
	m.modesOpen = false
	if i == m.mode {
		return nil
	}
	m.saveNow()
	if m.diary != nil {
		m.diary.writing = false
	}
	m.mode = i
	if i == diaryMode {
		m.enterDiary()
	}
	md := modes[i]
	if md.ready {
		m.say(md.Icon() + "  " + md.name + " mode.")
	} else {
		m.say(md.Icon() + "  " + md.name + " mode is still empty. Alt+M switches back.")
	}
	return nil
}

// modesBox is the box drawn over the screen.
const (
	tileW = 18 // inside width of one mode tile
	tileH = 3  // inside height: icon, name, state
)

// modeCols is how many tiles fit side by side.
func (m *Model) modeCols() int {
	switch {
	case m.width >= 3*(tileW+2)+10:
		return 3
	case m.width >= 2*(tileW+2)+8:
		return 2
	}
	return 1
}

// modesBox is the Alt+M box: a title, the modes as tiles, and what the
// chosen one is for. Everything is centred. On a small window it shrinks:
// first less space around things, then one line per mode, then without the
// description.
func (m *Model) modesBox() string {
	var box string
	for tight := 0; tight <= 3; tight++ {
		box = m.modesBoxAt(tight)
		if lipgloss.Height(box) <= m.height-2 && lipgloss.Width(box) <= m.width {
			break
		}
	}
	return box
}

func (m *Model) modesBoxAt(tight int) string {
	bg := cBarBG
	cols := m.modeCols()
	var rows []string
	if tight < 2 {
		for start := 0; start < len(modes); start += cols {
			var tiles []string
			for i := start; i < min(start+cols, len(modes)); i++ {
				tiles = append(tiles, m.modeTile(i))
			}
			rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, tiles...))
		}
	} else {
		for i := range modes {
			rows = append(rows, m.modeLine(i))
		}
	}
	grid := lipgloss.JoinVertical(lipgloss.Center, rows...)
	width := lipgloss.Width(grid)
	center := func(fg string, bold, italic bool, text string) string {
		return lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Background(hex(bg)).
			Foreground(hex(fg)).Bold(bold).Italic(italic).Render(text)
	}
	title := lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Background(hex(bg)).
		Render(glowText(icon("\uf0ec  ", "◆  ")+"M O D E S", bg))
	md := modes[m.modeSel]
	what := wrapWords(md.what, max(width-4, 20))
	hint := fmt.Sprintf("arrows or 1–%d choose · Enter switches · Esc closes", len(modes))
	if lipgloss.Width(hint) > width {
		hint = fmt.Sprintf("1–%d · Enter · Esc", len(modes))
	}
	var parts []string
	if tight == 0 {
		parts = []string{title, center(cDim, false, false, "Alt+M"), "", grid, ""}
	} else {
		parts = []string{title, grid}
	}
	parts = append(parts, center(cEmber, true, false, md.Icon()+"  "+md.name))
	if tight < 3 {
		for _, w := range what {
			parts = append(parts, center(cStatusText, false, false, w))
		}
	}
	if tight == 0 {
		parts = append(parts, "")
	}
	parts = append(parts, center(cDim, false, true, hint))
	body := lipgloss.JoinVertical(lipgloss.Center, parts...)
	pad := []int{1, 3}
	if tight > 0 {
		pad = []int{0, 1}
	}
	return stHelpBox.Background(hex(bg)).BorderBackground(hex(bg)).Padding(pad...).Render(body)
}

// modeLine is a mode on one line, for small windows.
func (m *Model) modeLine(i int) string {
	md := modes[i]
	bg, mark := cBarBG, "  "
	if i == m.modeSel {
		bg, mark = cSelection, "▶ "
	}
	st := func(fg string, bold bool) lipgloss.Style {
		return lipgloss.NewStyle().Foreground(hex(fg)).Background(hex(bg)).Bold(bold)
	}
	state := st(cDim, false).Italic(true).Width(13).Render("empty for now")
	if i == m.mode {
		state = st(cAccent, true).Width(13).Render("● now")
	}
	return st(cAccent, true).Render(mark) + st(cDim, false).Render(fmt.Sprintf("%d  ", i+1)) +
		st(cEmber, true).Render(md.Icon()+"  "+fmt.Sprintf("%-8s", md.name)) + st(cDim, false).Render("  ") + state + st(cDim, false).Render(" ")
}

// modeTile draws one mode as a small block: number, icon, name, state.
func (m *Model) modeTile(i int) string {
	md := modes[i]
	selected := i == m.modeSel
	bg, border, nameFG := cBarBG, cGutterLine, cStatusText
	if selected {
		bg, border, nameFG = cSelection, cEmber, cGlow
	}
	st := func(fg string, bold, italic bool) lipgloss.Style {
		return lipgloss.NewStyle().Foreground(hex(fg)).Background(hex(bg)).Bold(bold).Italic(italic).
			Width(tileW).Align(lipgloss.Center)
	}
	state := st(cDim, false, true).Render("empty for now")
	switch {
	case i == m.mode:
		state = st(cAccent, true, false).Render("● now")
	case md.ready:
		state = st(cDim, false, false).Render("ready")
	}
	iconFG := cEmber
	if selected {
		iconFG = cAccent
	}
	inside := lipgloss.JoinVertical(lipgloss.Center,
		st(iconFG, true, false).Render(md.Icon()),
		st(nameFG, true, false).Render(md.name),
		state,
	)
	label := fmt.Sprintf(" %d ", i+1)
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(hex(border)).
		BorderBackground(hex(cBarBG)).Background(hex(bg)).Render(inside)
	// Put the number into the top border, like a tab on the block.
	lines := strings.Split(box, "\n")
	numStyle := lipgloss.NewStyle().Foreground(hex(cBG)).Background(hex(border)).Bold(true)
	if !selected {
		numStyle = lipgloss.NewStyle().Foreground(hex(cDim)).Background(hex(cBarBG))
	}
	edge := lipgloss.NewStyle().Foreground(hex(border)).Background(hex(cBarBG))
	lines[0] = edge.Render("╭─") + numStyle.Render(label) + edge.Render(strings.Repeat("─", tileW-1-len(label))+"╮")
	// a gap between tiles
	pad := lipgloss.NewStyle().Background(hex(cBarBG)).Render(" ")
	for k := range lines {
		lines[k] = pad + lines[k] + pad
	}
	return strings.Join(lines, "\n")
}

func wrapWords(s string, w int) []string {
	var out []string
	line := ""
	for _, word := range strings.Fields(s) {
		if line != "" && len([]rune(line))+1+len([]rune(word)) > w {
			out = append(out, line)
			line = word
			continue
		}
		if line != "" {
			line += " "
		}
		line += word
	}
	if line != "" {
		out = append(out, line)
	}
	return out
}

// overlay puts box in the middle of screen.
func (m *Model) overlay(screen, box string) string {
	bw, bh := lipgloss.Width(box), lipgloss.Height(box)
	x, y := max(0, (m.width-bw)/2), max(1, (m.height-bh)/2)
	return lipgloss.NewCompositor(
		lipgloss.NewLayer(screen),
		lipgloss.NewLayer(box).X(x).Y(y).Z(1),
	).Render()
}

// ---- empty modes ------------------------------------------------------------------------

// emptyModeRows fills the text area for a mode that is not built yet.
func (m *Model) emptyModeRows(sb *strings.Builder) {
	md := modes[m.mode]
	h := m.textHeight()
	body := []string{
		lipgloss.NewStyle().Foreground(hex(cEmber)).Bold(true).Render(md.Icon() + "  " + md.name + " mode"),
		"",
		lipgloss.NewStyle().Foreground(hex(cStatusText)).Render("Still empty. It will be for:"),
	}
	for _, w := range wrapWords(md.what, 50) {
		body = append(body, lipgloss.NewStyle().Foreground(hex(cAccent)).Render(w))
	}
	body = append(body, "", lipgloss.NewStyle().Foreground(hex(cDim)).Italic(true).Render("Alt+M switches modes · Alt+Q quits"))
	block := lipgloss.Place(m.width, h, lipgloss.Center, lipgloss.Center, strings.Join(body, "\n"),
		lipgloss.WithWhitespaceStyle(lipgloss.NewStyle().Background(hex(cBG))))
	sb.WriteString(lipgloss.NewStyle().Background(hex(cBG)).Render(block))
	sb.WriteByte('\n')
}

// topPanel is the first row: the title in the middle and the mode at both
// ends. Clicking it opens the modes box.
func (m *Model) topPanel() string {
	bg := cStatusBG
	md := modes[m.mode]
	fill := blockFill()
	modeText := lipgloss.NewStyle().Background(hex(fill)).Foreground(hex(cStatusText))
	modeIcon := lipgloss.NewStyle().Background(hex(fill)).Foreground(hex(cEmber))
	left := block(modeIcon.Render(" "+md.LeftIcon()+" ")+modeText.Render(md.name+" "), bg)
	right := block(modeText.Render(" "+md.name)+modeIcon.Render(" "+md.Icon()+" "), bg)
	sp := lipgloss.NewStyle().Background(hex(bg)).Render("   ")
	title := m.emblem.full(bg) + sp + glowText("S Y M B O L I Z E R", bg) + sp + m.emblem.full(bg)
	if m.width < lipgloss.Width(left)+lipgloss.Width(right)+lipgloss.Width(title)+4 {
		title = glowText("Symbolizer", bg)
	}
	barFill := lipgloss.NewStyle().Background(hex(bg))
	room := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if room < lipgloss.Width(title) {
		return fit(left+barFill.Render(" ")+title, m.width, barFill)
	}
	lw := (room - lipgloss.Width(title)) / 2
	rw := room - lipgloss.Width(title) - lw
	return left + barFill.Render(strings.Repeat(" ", lw)) + title + barFill.Render(strings.Repeat(" ", rw)) + right
}
