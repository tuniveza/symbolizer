package main

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type textinputStyles = textinput.Styles

// The colour switcher: ŧ (Right Alt+T on a UK keyboard) or Alt+T. Each theme
// is a tile drawn in its own colours. Moving to a tile shows the whole
// editor in it at once; Enter keeps it (remembered next time), Esc puts the
// old one back. Only Symbolizer changes.

func (m *Model) openThemes() tea.Cmd {
	if m.themeOpen {
		return m.themeKey("esc")
	}
	m.help, m.picker, m.modesOpen = false, nil, false
	m.closePrompt()
	m.themeOpen = true
	m.themeBefore = currentTheme
	m.themeSel = currentTheme
	return nil
}

func (m *Model) themeKey(name string) tea.Cmd {
	cols := m.modeCols()
	sel := m.themeSel
	switch name {
	case "esc", "ŧ", "alt+t":
		m.useTheme(m.themeBefore)
		m.themeOpen = false
		return nil
	case "enter":
		m.themeOpen = false
		saveSetting("theme", themes[m.themeSel].Name)
		m.say("Colours: " + themes[m.themeSel].Name + ". Kept for next time.")
		return nil
	case "left", "shift+tab":
		sel = (sel + len(themes) - 1) % len(themes)
	case "right", "tab":
		sel = (sel + 1) % len(themes)
	case "up":
		if sel-cols >= 0 {
			sel -= cols
		}
	case "down":
		if sel+cols < len(themes) {
			sel += cols
		}
	default:
		if len(name) == 1 && name[0] >= '1' && int(name[0]-'1') < len(themes) {
			sel = int(name[0] - '1')
		}
	}
	if sel != m.themeSel {
		m.themeSel = sel
		m.useTheme(sel) // show it straight away
	}
	return nil
}

// useTheme switches colours and refreshes what keeps its own copy of them.
func (m *Model) useTheme(i int) {
	applyTheme(i)
	m.input.SetStyles(inputStyles(m.input.Styles()))
}

func inputStyles(st textinputStyles) textinputStyles {
	st.Focused.Text = stPromptTxt
	st.Focused.Placeholder = stMessage
	st.Focused.Prompt = stPrompt
	st.Cursor.Color = hex(cEmber)
	return st
}

// themeIndex finds a theme by name (from the settings file).
func themeIndex(name string) int {
	for i, t := range themes {
		if strings.EqualFold(t.Name, name) {
			return i
		}
	}
	return 0
}

// ---- drawing ------------------------------------------------------------------------

func (m *Model) themesBox() string {
	var box string
	for tight := 0; tight <= 1; tight++ {
		box = m.themesBoxAt(tight)
		if lipgloss.Height(box) <= m.height-2 && lipgloss.Width(box) <= m.width {
			break
		}
	}
	return box
}

func (m *Model) themesBoxAt(tight int) string {
	bg := cBarBG
	cols := m.modeCols()
	var rows []string
	for start := 0; start < len(themes); start += cols {
		var tiles []string
		for i := start; i < min(start+cols, len(themes)); i++ {
			tiles = append(tiles, m.themeTile(i))
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, tiles...))
	}
	grid := lipgloss.JoinVertical(lipgloss.Center, rows...)
	width := lipgloss.Width(grid)
	center := func(fg string, bold, italic bool, text string) string {
		return lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Background(hex(bg)).
			Foreground(hex(fg)).Bold(bold).Italic(italic).Render(text)
	}
	title := lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Background(hex(bg)).
		Render(glowText(icon("\U000f03d8  ", "◆  ")+"C O L O U R S", bg))
	t := themes[m.themeSel]
	hint := fmt.Sprintf("arrows or 1–%d show it · Enter keeps it · Esc puts it back", len(themes))
	if lipgloss.Width(hint) > width {
		hint = "arrows · Enter keeps · Esc back"
	}
	parts := []string{title}
	if tight == 0 {
		parts = append(parts, center(cDim, false, false, "ŧ  (Right Alt+T)  ·  only Symbolizer changes"), "")
	}
	parts = append(parts, grid)
	if tight == 0 {
		parts = append(parts, "")
	}
	parts = append(parts, center(cEmber, true, false, t.Name), center(cStatusText, false, false, t.What))
	if tight == 0 {
		parts = append(parts, "")
	}
	parts = append(parts, center(cDim, false, true, hint))
	pad := []int{1, 3}
	if tight > 0 {
		pad = []int{0, 1}
	}
	return stHelpBox.Background(hex(bg)).BorderBackground(hex(bg)).Padding(pad...).
		Render(lipgloss.JoinVertical(lipgloss.Center, parts...))
}

// themeTile is a theme drawn in its own colours: its name, colour swatches,
// and a scrap of coloured code.
func (m *Model) themeTile(i int) string {
	p := themes[i]
	selected := i == m.themeSel
	on := func(fg string, bold bool) lipgloss.Style {
		return lipgloss.NewStyle().Foreground(hex(fg)).Background(hex(p.BG)).Bold(bold)
	}
	centerLine := func(s string) string {
		return lipgloss.NewStyle().Background(hex(p.BG)).Width(tileW).Align(lipgloss.Center).Render(s)
	}
	name := p.Name
	if i == m.themeBefore {
		name += " ●"
	}
	var sw strings.Builder
	for _, c := range []string{p.Text, p.Keyword, p.Function, p.String, p.Number, p.Comment} {
		sw.WriteString(on(c, false).Render("██"))
	}
	code := on(p.Keyword, true).Render("if") + on(p.Text, false).Render(" x ") +
		on(p.Operator, false).Render("=") + on(p.Number, false).Render(" 1") +
		on(p.Comment, false).Italic(true).Render(" // hi")
	inside := lipgloss.JoinVertical(lipgloss.Center,
		centerLine(on(p.Ember, true).Render(name)),
		centerLine(sw.String()),
		centerLine(code),
	)
	border := cGutterLine
	if selected {
		border = cEmber
	}
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(hex(border)).
		BorderBackground(hex(cBarBG)).Background(hex(p.BG)).Render(inside)
	lines := strings.Split(box, "\n")
	label := fmt.Sprintf(" %d ", i+1)
	numStyle := lipgloss.NewStyle().Foreground(hex(cBG)).Background(hex(border)).Bold(true)
	if !selected {
		numStyle = lipgloss.NewStyle().Foreground(hex(cDim)).Background(hex(cBarBG))
	}
	edge := lipgloss.NewStyle().Foreground(hex(border)).Background(hex(cBarBG))
	lines[0] = edge.Render("╭─") + numStyle.Render(label) + edge.Render(strings.Repeat("─", tileW-1-len(label))+"╮")
	pad := lipgloss.NewStyle().Background(hex(cBarBG)).Render(" ")
	for k := range lines {
		lines[k] = pad + lines[k] + pad
	}
	return strings.Join(lines, "\n")
}
