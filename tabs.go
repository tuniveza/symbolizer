package main

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// tab is one open file. The Model's buf, hl, top, left, savePaused and
// saveErr are the tab on screen; the others wait in m.tabs.
type tab struct {
	buf        *Buffer
	hl         *Highlighter
	top, left  int
	savePaused bool
	saveErr    string
}

func (m *Model) storeTab() {
	if m.cur < len(m.tabs) {
		t := m.tabs[m.cur]
		t.buf, t.hl, t.top, t.left, t.savePaused, t.saveErr = m.buf, m.hl, m.top, m.left, m.savePaused, m.saveErr
	}
}

func (m *Model) showTab(i int) {
	t := m.tabs[i]
	m.cur = i
	m.buf, m.hl, m.top, m.left, m.savePaused, m.saveErr = t.buf, t.hl, t.top, t.left, t.savePaused, t.saveErr
	m.buf.OnSplice = m.hl.Splice
	m.showMatches = false
	m.ensureVisible()
}

// addTab opens b in a new tab after the current one and shows it.
func (m *Model) addTab(b *Buffer) {
	m.storeTab()
	at := min(m.cur+1, len(m.tabs))
	m.tabs = append(m.tabs[:at], append([]*tab{{buf: b}}, m.tabs[at:]...)...)
	m.cur = at
	m.buf, m.top, m.left, m.savePaused, m.saveErr = b, 0, 0, false, ""
	m.resetHighlighter()
	m.tabs[at].hl = m.hl
}

// switchTab saves the tab on screen, then shows tab i.
func (m *Model) switchTab(i int) tea.Cmd {
	if i < 0 || i >= len(m.tabs) || i == m.cur {
		return nil
	}
	m.saveNow()
	m.storeTab()
	m.showTab(i)
	return nil
}

func (m *Model) nextTab(d int) tea.Cmd {
	if len(m.tabs) < 2 {
		m.say("Only one file is open. Alt+O opens more.")
		return nil
	}
	return m.switchTab((m.cur + d + len(m.tabs)) % len(m.tabs))
}

// closeTab closes the file on screen (it is already saved).
func (m *Model) closeTab() tea.Cmd {
	return m.ifUnsaved("Close it?", func(m *Model) tea.Cmd {
		dropEmptySession(m.buf)
		name := m.tabName(m.cur)
		m.tabs = append(m.tabs[:m.cur], m.tabs[m.cur+1:]...)
		if len(m.tabs) == 0 {
			m.tabs = nil
			m.cur = 0
			m.addTab(newDraft())
		} else {
			m.showTab(min(m.cur, len(m.tabs)-1))
		}
		m.say("Closed " + name + ".")
		return nil
	})
}

func (m *Model) findTab(path string) int {
	for i, t := range m.tabs {
		b := t.buf
		if i == m.cur {
			b = m.buf
		}
		if b.Path == path {
			return i
		}
	}
	return -1
}

// blankDraft is an untouched new session: opening a file can reuse its tab.
func (m *Model) blankDraft() bool {
	b := m.buf
	return b.Draft && b.Path == "" && len(b.Lines) == 1 && len(b.Lines[0]) == 0
}

func newDraft() *Buffer {
	b := NewBuffer()
	b.Draft = true
	return b
}

func (m *Model) tabBuf(i int) *Buffer {
	if i == m.cur {
		return m.buf
	}
	return m.tabs[i].buf
}

// sessionText is what the status bar says about a session.
func sessionText(b *Buffer) string {
	if b.Path == "" {
		return "new session"
	}
	return strings.Replace(sessionLabel(b.Path), "session from ", "session ", 1)
}

func (m *Model) tabName(i int) string {
	b := m.tabBuf(i)
	switch {
	case b.Draft:
		name, ext := b.suggestName() // a file-like name from the text, or a random one
		return name + ext
	case b.Path == "":
		return "new file"
	}
	// Two open files with one name (a README in each folder) show their folder.
	name := filepath.Base(b.Path)
	for j := range m.tabs {
		if o := m.tabBuf(j); j != i && !o.Draft && filepath.Base(o.Path) == name {
			return filepath.Join(filepath.Base(filepath.Dir(b.Path)), name)
		}
	}
	return name
}

// tabBar draws the open files along the top, centred. tabHits remembers where each
// tab is, so a mouse click can choose it.
func (m *Model) tabBar() string {
	left := stBar.Render(" ")
	right := stBar.Render(" F1 help ")
	room := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	type part struct {
		text string
		i    int
	}
	var parts []part
	for i := range m.tabs {
		name := m.tabName(i)
		if len(m.tabs) > 1 && i < 9 {
			name = fmt.Sprintf("%d %s", i+1, name)
		}
		st := stBar
		if i == m.cur {
			st = lipgloss.NewStyle().Background(hex(cStatusBG)).Foreground(hex(cGlow)).Bold(true)
		}
		parts = append(parts, part{st.Render(" " + name + " "), i})
	}
	// Show tabs around the current one when they do not all fit.
	from, to := 0, len(parts)
	width := func() int {
		w := 0
		for _, p := range parts[from:to] {
			w += lipgloss.Width(p.text) + 1
		}
		return w
	}
	for width() > room && to-from > 1 {
		if m.cur-from > to-1-m.cur {
			from++
		} else {
			to--
		}
	}
	// Centre the tabs on the whole width, keeping clear of "F1 help".
	x := lipgloss.Width(left)
	if w := width() - 1; w < room {
		x = max(x, min((m.width-w)/2, m.width-lipgloss.Width(right)-w))
	}
	m.tabHits = m.tabHits[:0]
	var sb strings.Builder
	sb.WriteString(left)
	sb.WriteString(stBar.Render(strings.Repeat(" ", x-lipgloss.Width(left))))
	for _, p := range parts[from:to] {
		w := lipgloss.Width(p.text)
		m.tabHits = append(m.tabHits, [3]int{x, x + w, p.i})
		sb.WriteString(p.text)
		sb.WriteString(stBar.Render(" "))
		x += w + 1
	}
	return fit(sb.String(), m.width-lipgloss.Width(right), stBar) + right
}

func (m *Model) clickTab(x int) tea.Cmd {
	for _, h := range m.tabHits {
		if x >= h[0] && x < h[1] {
			return m.switchTab(h[2])
		}
	}
	return nil
}
