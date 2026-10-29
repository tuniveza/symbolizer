package main

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// Switching file tabs with Tab.
//
//	Tab                      next file tab in the common direction (right at first)
//	Tab, then Left / Right   switch file tabs in the arrow's direction
//	Ctrl+Tab, then Ctrl+Left / Ctrl+Right
//	                         set the common direction (remembered)
//	Alt+Tab                  a real Tab (indent)
//
// Terminals do not say when Tab is let go, so an arrow counts as part of the
// chord when it comes soon after the Tab press. Plain arrows get a shorter
// time (tabArrowGap) because they are also used to move the cursor.

const chordGap = 900 * time.Millisecond
const tabArrowGap = 500 * time.Millisecond

// tabSwitch is Tab: one file tab in the common direction. Arrows right after
// it choose the direction instead.
func (m *Model) tabSwitch() tea.Cmd {
	if len(m.tabs) < 2 {
		m.say("Only one file is open. Alt+N opens a new tab, Alt+O opens a file. Alt+Tab types a real Tab.")
		return nil
	}
	m.tabChord = "tab"
	m.tabChordFrom = m.cur
	m.tabChordArrowed = false
	m.tabChordUntil = time.Now().Add(tabArrowGap)
	cmd := m.nextTab(m.tabDir)
	m.say("Tab goes " + dirWord(m.tabDir) + ". Hold Tab and press Left / Right to choose. Alt+Tab types a real Tab.")
	return cmd
}

// tabSwitchBack is Shift+Tab: one file tab against the common direction.
func (m *Model) tabSwitchBack() tea.Cmd {
	if len(m.tabs) < 2 {
		m.say("Only one file is open. Alt+N opens a new tab, Alt+O opens a file.")
		return nil
	}
	m.tabChord = ""
	cmd := m.nextTab(-m.tabDir)
	m.say("Shift+Tab goes " + dirWord(-m.tabDir) + ", Tab goes " + dirWord(m.tabDir) + ".")
	return cmd
}

// ctrlTab is Ctrl+Tab: the next Ctrl+Left / Ctrl+Right sets the direction.
func (m *Model) ctrlTab() tea.Cmd {
	m.tabChord = "ctrl"
	m.tabChordUntil = time.Now().Add(chordGap)
	m.say("Tab goes " + dirWord(m.tabDir) + ". Keep Ctrl held and press Left or Right to change it.")
	return m.flashTabSign()
}

// tabChordKey handles an arrow that belongs to a Tab chord. It returns
// false when the key is not part of one.
func (m *Model) tabChordKey(name string) (tea.Cmd, bool) {
	if m.tabChord == "" || time.Now().After(m.tabChordUntil) {
		m.tabChord = ""
		return nil, false
	}
	d := 0
	switch {
	case m.tabChord == "tab" && name == "left", m.tabChord == "ctrl" && name == "ctrl+left":
		d = -1
	case m.tabChord == "tab" && name == "right", m.tabChord == "ctrl" && name == "ctrl+right":
		d = 1
	default:
		m.tabChord = ""
		return nil, false
	}
	m.tabChordUntil = time.Now().Add(tabArrowGap)
	if m.tabChord == "ctrl" {
		m.tabDir = d
		saveSetting("tab-direction", dirWord(d))
		m.tabChord = ""
		m.say("Tab now goes " + dirWord(d) + ".")
		return m.flashTabSign(), true
	}
	if !m.tabChordArrowed {
		// The Tab press already moved one file tab; the arrow decides instead.
		m.tabChordArrowed = true
		m.switchTab(m.tabChordFrom)
	}
	return m.nextTab(d), true
}

func dirWord(d int) string {
	if d < 0 {
		return "left"
	}
	return "right"
}

// ---- remembered settings (~/.config/symbolizer/settings) --------------------------

func settingsPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "symbolizer", "settings")
}

func loadSettings() map[string]string {
	out := map[string]string{}
	data, err := os.ReadFile(settingsPath())
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(data), "\n") {
		if k, v, ok := strings.Cut(line, "="); ok {
			out[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return out
}

func saveSetting(key, value string) {
	s := loadSettings()
	s[key] = value
	var sb strings.Builder
	sb.WriteString("# Symbolizer settings (written by sy)\n")
	for k, v := range s {
		sb.WriteString(k + " = " + v + "\n")
	}
	os.MkdirAll(filepath.Dir(settingsPath()), 0o755)
	os.WriteFile(settingsPath(), []byte(sb.String()), 0o644)
}
