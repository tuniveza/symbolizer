package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Sessions are drafts: text with no name yet. Each one is a file in
// ~/.local/share/symbolizer/sessions, saved after every change.
//
// A session stays open for 24 hours after its last change. Starting sy
// without a file name continues that session. After 24 hours without a
// change the session is concluded: sy starts a new one, but the old one is
// kept and can be opened (and written to) again from Alt+L.

const sessionLife = 24 * time.Hour

func sessionDir() string {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "symbolizer", "sessions")
}

func newSessionPath() (string, error) {
	dir := sessionDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	name := "session-" + time.Now().Format("2006-01-02-150405")
	path := filepath.Join(dir, name+".txt")
	for i := 2; fileExists(path); i++ {
		path = filepath.Join(dir, fmt.Sprintf("%s-%d.txt", name, i))
	}
	return path, nil
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

func isSession(path string) bool {
	return path != "" && filepath.Dir(path) == sessionDir()
}

type sessionInfo struct {
	Path    string
	Changed time.Time
	Lines   int
	Lang    string
	Preview string
}

func (s sessionInfo) Open() bool { return time.Since(s.Changed) < sessionLife }

// listSessions returns every session, newest change first.
func listSessions() []sessionInfo {
	entries, _ := os.ReadDir(sessionDir())
	var out []sessionInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "session-") || !strings.HasSuffix(e.Name(), ".txt") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		s := sessionInfo{Path: filepath.Join(sessionDir(), e.Name()), Changed: info.ModTime()}
		if data, err := os.ReadFile(s.Path); err == nil {
			text := string(data)
			s.Lines = strings.Count(strings.TrimSuffix(text, "\n"), "\n") + 1
			s.Lang, _ = guessLanguage(text)
			for _, l := range strings.Split(text, "\n") {
				if t := strings.TrimSpace(l); t != "" {
					s.Preview = t
					break
				}
			}
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Changed.After(out[j].Changed) })
	return out
}

// currentSession is the session changed in the last 24 hours, if any.
func currentSession() (sessionInfo, bool) {
	all := listSessions()
	if len(all) > 0 && all[0].Open() {
		return all[0], true
	}
	return sessionInfo{}, false
}

func when(t time.Time) string {
	now := time.Now()
	switch {
	case t.Format("2006-01-02") == now.Format("2006-01-02"):
		return "today " + t.Format("15:04")
	case t.Format("2006-01-02") == now.AddDate(0, 0, -1).Format("2006-01-02"):
		return "yesterday " + t.Format("15:04")
	}
	return t.Format("Mon 2 Jan 15:04")
}

// sessionLabel is how a session is named on screen.
func sessionLabel(path string) string {
	if path == "" {
		return "new session"
	}
	base := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(path), "session-"), ".txt")
	if t, err := time.ParseInLocation("2006-01-02-150405", base[:min(len(base), 17)], time.Local); err == nil {
		return "session from " + when(t)
	}
	return "session " + base
}

// ---- the Alt+L list ------------------------------------------------------------

type pickItem struct {
	label string
	open  func(m *Model) tea.Cmd
}

type picker struct {
	title string
	items []pickItem
	sel   int
	top   int
}

func (m *Model) openLoadList() tea.Cmd {
	p := &picker{title: "Load"}
	if m.buf.Path != "" && !m.buf.Draft && fileExists(m.buf.Path) {
		p.items = append(p.items, pickItem{
			label: stHelpKey.Render("↻ ") + stHelpText.Render("this file again from disk: "+filepath.Base(m.buf.Path)),
			open:  func(m *Model) tea.Cmd { return m.reload() },
		})
	}
	for _, s := range listSessions() {
		s := s
		state := stHelpNew.Render("concluded")
		if s.Open() {
			state = stModified.Background(hex(cBG)).Render("open")
		}
		if s.Path == m.buf.Path {
			state += stHelpNew.Render(" · on screen now")
		}
		preview := s.Preview
		if r := []rune(preview); len(r) > 40 {
			preview = string(r[:39]) + "…"
		}
		label := stHelpKey.Render(fmt.Sprintf("%-22s", when(s.Changed))) + " " + state +
			stHelpText.Render(fmt.Sprintf("  ·  %s  ·  %d lines  ·  ", s.Lang, s.Lines)) + stHelpNew.Render(preview)
		p.items = append(p.items, pickItem{label: label, open: func(m *Model) tea.Cmd {
			already := m.findTab(s.Path) >= 0
			cmd := m.openPath(s.Path)
			if !already {
				m.buf.Cursor = m.endOfFile()
				m.ensureVisible()
			}
			m.say("Opened the " + sessionLabel(s.Path) + ". It saves by itself; Ctrl+S gives it a name.")
			return cmd
		}})
	}
	if len(p.items) == 0 {
		m.say("No sessions yet. Type something and it is kept as a session.")
		return nil
	}
	m.picker = p
	return nil
}

func (m *Model) pickerKey(name string) tea.Cmd {
	p := m.picker
	switch name {
	case "esc", "ctrl+l", "alt+l":
		m.picker = nil
	case "up", "alt+up", "shift+tab":
		p.sel = max(0, p.sel-1)
	case "down", "alt+down", "tab":
		p.sel = min(len(p.items)-1, p.sel+1)
	case "pgup":
		p.sel = max(0, p.sel-10)
	case "pgdown":
		p.sel = min(len(p.items)-1, p.sel+10)
	case "enter":
		m.picker = nil
		return p.items[p.sel].open(m)
	}
	return nil
}

func (m *Model) drawPicker(sb *strings.Builder) {
	p := m.picker
	room := max(m.height-8, 3)
	if p.sel < p.top {
		p.top = p.sel
	}
	if p.sel >= p.top+room {
		p.top = p.sel - room + 1
	}
	lines := []string{
		glowText("◆ "+p.title, cBG),
		stHelpNew.Render("Sessions are your unnamed drafts. Open ones were changed in the last 24 hours."),
		"",
	}
	for i := p.top; i < min(len(p.items), p.top+room); i++ {
		mark := "  "
		if i == p.sel {
			mark = stHelpKey.Render("▶ ")
		}
		lines = append(lines, mark+p.items[i].label)
	}
	lines = append(lines, "", stHelpNew.Render("Up / Down chooses.  Enter opens.  Esc closes."))
	box := stHelpBox.Width(min(m.width-2, 110)).Render(strings.Join(lines, "\n"))
	sb.WriteString(lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box,
		lipgloss.WithWhitespaceStyle(lipgloss.NewStyle().Background(hex(cBG)))))
}
