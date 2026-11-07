package main

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Tab switch mode (Alt+Tab). The tab row opens out into as many rows as
// the open files need, over the top of the text:
//
//	Left / Right, A / D  the file tab before / after (round the ends)
//	Up / Down, W / S     the tab in the row above / below
//	Home / End           the first / last tab
//	Enter (once or twice), Down twice quickly,
//	Alt+Tab, or Alt on its own   stay on this file
//	Esc                back to the file it started from
//
// The file on screen changes as the arrows move. Any other key leaves the
// mode and does what it always does. Clicking a tab chooses it.
//
// Hyprland takes Alt+Tab to switch windows; its bind sends Alt+Tab on to a
// window whose title ends "— Symbolizer" (keybinds.lua).

// tabTwice is how quick a second Down (or Enter) must be to count as twice.
const tabTwice = 500 * time.Millisecond

type tabCell struct {
	i, x, w int
	text    string
}

func (m *Model) openTabMode() tea.Cmd {
	if len(m.tabs) < 2 {
		m.say("Only one file is open. Alt+N opens a new tab, Alt+O opens a file.")
		return nil
	}
	m.closeTree()
	m.tabMode, m.tabModeFrom = true, m.cur
	m.tabDownAt = time.Time{}
	m.say("Tab mode: arrows choose a file · Enter, or Down twice, keeps it · Esc goes back")
	return nil
}

// tabLayout puts every tab in rows that fit the window, in order, each row
// centred like the tab bar.
func (m *Model) tabLayout() [][]tabCell {
	var rows [][]tabCell
	var row []tabCell
	x := 1
	for i := range m.tabs {
		text := fmt.Sprintf(" %d %s ", i+1, m.tabName(i))
		w := lipgloss.Width(text)
		if len(row) > 0 && x+w > m.width-1 {
			rows, row, x = append(rows, row), nil, 1
		}
		row = append(row, tabCell{i, x, w, text})
		x += w + 1
	}
	rows = append(rows, row)
	for _, row := range rows {
		last := row[len(row)-1]
		shift := (m.width-(last.x+last.w-row[0].x))/2 - row[0].x
		for c := range row {
			row[c].x += shift
		}
	}
	return rows
}

// tabCellAt finds tab i in the layout.
func tabCellAt(rows [][]tabCell, i int) (int, int) {
	for r, row := range rows {
		for c, cell := range row {
			if cell.i == i {
				return r, c
			}
		}
	}
	return 0, 0
}

// tabModeKey handles a key in tab mode. It returns false for a key that
// leaves the mode and should then do its usual job.
func (m *Model) tabModeKey(k tea.Key) (tea.Cmd, bool) {
	n := len(m.tabs)
	name := m.altGrName(k)
	if name == "down" {
		// Down twice quickly keeps the tab chosen before the first Down.
		if !m.tabDownAt.IsZero() && time.Since(m.tabDownAt) < tabTwice && !k.IsRepeat {
			m.tabDownAt = time.Time{}
			m.switchTab(m.tabDownFrom)
			m.endTabMode()
			return nil, true
		}
		m.tabDownAt, m.tabDownFrom = time.Now(), m.cur
	} else {
		m.tabDownAt = time.Time{}
	}
	if dir, ok := map[string]string{"w": "up", "a": "left", "s": "down", "d": "right"}[name]; ok {
		name = dir
	}
	switch name {
	case "left":
		return m.switchTab((m.cur - 1 + n) % n), true
	case "right":
		return m.switchTab((m.cur + 1) % n), true
	case "home":
		return m.switchTab(0), true
	case "end":
		return m.switchTab(n - 1), true
	case "up", "down":
		rows := m.tabLayout()
		r, c := tabCellAt(rows, m.cur)
		to := r - 1
		if name == "down" {
			to = r + 1
		}
		if to < 0 || to >= len(rows) {
			return nil, true
		}
		// The tab in that row nearest above / below this one.
		mid := rows[r][c].x + rows[r][c].w/2
		best := rows[to][0]
		for _, cell := range rows[to] {
			if abs(cell.x+cell.w/2-mid) < abs(best.x+best.w/2-mid) {
				best = cell
			}
		}
		return m.switchTab(best.i), true
	case "enter", "alt+tab":
		m.endTabMode()
		if name == "enter" {
			m.eatEnterAt = time.Now() // Enter twice chooses it too, with no new line
		}
		return nil, true
	case "esc":
		m.tabMode = false
		m.switchTab(m.tabModeFrom)
		m.say("Back on " + m.tabName(m.cur) + ".")
		return nil, true
	}
	m.tabMode = false
	return nil, false
}

// altTapRelease ends tab mode when Alt (the left one) is pressed and let go
// with no other key in between. Only kitty reports Alt on its own.
// In Design mode, Alt tapped twice quickly is Esc (design_keys.go); it
// says whether it was.
func (m *Model) altTapRelease(k tea.Key) bool {
	tapped := k.Code == tea.KeyLeftAlt && m.altTap
	m.altTap = false
	if tapped && m.tabMode {
		m.endTabMode()
		return false
	}
	if tapped && m.inDesign() && !m.help && m.learn == nil && !m.modesOpen && !m.themeOpen && m.prompt == pNone {
		return m.designAltTap()
	}
	return false
}

// endTabMode leaves tab mode on the file now chosen.
func (m *Model) endTabMode() {
	m.tabMode = false
	m.say("On " + m.tabName(m.cur) + ".")
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// tabModeClick chooses the tab under the mouse, if any, and leaves the mode.
func (m *Model) tabModeClick(x, y int) tea.Cmd {
	m.tabMode = false
	rows := m.tabLayout()
	if r := y - 1; r >= 0 && r < len(rows) {
		for _, cell := range rows[r] {
			if x >= cell.x && x < cell.x+cell.w {
				return m.switchTab(cell.i)
			}
		}
	}
	return nil
}

// overlayTabRows draws the opened-out tab rows over the screen, from the
// tab row down, with a line under them.
func (m *Model) overlayTabRows(content string) string {
	lines := strings.Split(content, "\n")
	rows := m.tabLayout()
	glow := lipgloss.NewStyle().Background(hex(cEmber)).Foreground(hex(cBG)).Bold(true)
	for r, row := range rows {
		y := 1 + r
		if y >= len(lines)-3 {
			break
		}
		var sb strings.Builder
		sb.WriteString(stBar.Render(strings.Repeat(" ", row[0].x)))
		for _, cell := range row {
			st := stBar
			if cell.i == m.cur {
				st = glow
			}
			sb.WriteString(st.Render(cell.text))
			sb.WriteString(stBar.Render(" "))
		}
		lines[y] = fit(sb.String(), m.width, stBar)
	}
	if y := 1 + len(rows); y < len(lines)-3 {
		rule := lipgloss.NewStyle().Background(hex(cStatusBG)).Foreground(hex(cEmber))
		lines[y] = rule.Render(strings.Repeat("─", m.width))
	}
	return strings.Join(lines, "\n")
}
