package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// What was open when Symbolizer last closed, so plain `sy` opens it all
// again: the project folder, every file tab at its place, and the file that
// was on screen. It lives in ~/.local/share/symbolizer/last-open:
//
//	project /home/me/web
//	file 12 4 0 /home/me/web/index.html   (row, column, top row, path)
//	current /home/me/web/index.html

func lastOpenPath() string { return filepath.Join(filepath.Dir(sessionDir()), "last-open") }

// lastOpenText is what is open now, as the file says it; files is the
// same without the places, to tell a new file from a cursor that moved.
func (m *Model) lastOpenText() (text, files string) {
	m.storeTab()
	var sb, fb strings.Builder
	if m.tree != nil {
		sb.WriteString("project " + m.tree.root + "\n")
		fb.WriteString(m.tree.root + "\n")
	}
	for _, t := range m.tabs {
		if b := t.buf; b.Path != "" && fileExists(b.Path) {
			fmt.Fprintf(&sb, "file %d %d %d %s\n", b.Cursor.Row, b.Cursor.Col, t.top, b.Path)
			fb.WriteString(b.Path + "\n")
		}
	}
	if m.buf.Path != "" {
		sb.WriteString("current " + m.buf.Path + "\n")
		fb.WriteString(m.buf.Path + "\n")
	}
	return sb.String(), fb.String()
}

// noteLastOpen keeps the file up to date while sy runs, so it is right
// even when the window is closed without Alt+Q. Opening, closing or
// switching files is written at once; a cursor that only moved, at most
// every two seconds.
func (m *Model) noteLastOpen() {
	if len(m.tabs) == 0 {
		return
	}
	text, files := m.lastOpenText()
	if text == m.lastOpen.text || files == m.lastOpen.files && time.Since(m.lastOpen.at) < 2*time.Second {
		return
	}
	m.writeLastOpen(text, files)
}

// saveLastOpen writes down what is open now (sy calls it as it closes).
func (m *Model) saveLastOpen() {
	m.writeLastOpen(m.lastOpenText())
}

func (m *Model) writeLastOpen(text, files string) {
	os.MkdirAll(filepath.Dir(lastOpenPath()), 0o755)
	os.WriteFile(lastOpenPath(), []byte(text), 0o644)
	m.lastOpen.text, m.lastOpen.files, m.lastOpen.at = text, files, time.Now()
}

// reopenLast opens again what was open when sy last closed. Files that are
// gone, and sessions that have been concluded, are left out. It says
// whether any file opened; if none did, sy starts as it always has.
func (m *Model) reopenLast() bool {
	data, err := os.ReadFile(lastOpenPath())
	if err != nil {
		return false
	}
	type entry struct {
		pos  Pos
		top  int
		path string
	}
	var files []entry
	project, current := "", ""
	live, _ := currentSession()
	for _, line := range strings.Split(string(data), "\n") {
		key, rest, _ := strings.Cut(line, " ")
		switch key {
		case "project":
			project = rest
		case "current":
			current = rest
		case "file":
			f := strings.SplitN(rest, " ", 4)
			if len(f) < 4 || !fileExists(f[3]) || isSession(f[3]) && f[3] != live.Path {
				continue
			}
			row, _ := strconv.Atoi(f[0])
			col, _ := strconv.Atoi(f[1])
			top, _ := strconv.Atoi(f[2])
			files = append(files, entry{Pos{row, col}, top, f[3]})
		}
	}
	if len(files) == 0 {
		return false
	}
	if info, err := os.Stat(project); err == nil && info.IsDir() {
		m.setRoot(project)
	}
	first := m.buf // today's session, or a blank one: it goes unless it was open last time
	keepFirst := false
	for _, f := range files {
		m.openPath(f.path)
		m.buf.Cursor = m.buf.Clamp(f.pos)
		m.top = max(0, min(f.top, len(m.buf.Lines)-1))
		m.storeTab()
		keepFirst = keepFirst || f.path == first.Path
	}
	if !keepFirst {
		for i := range m.tabs {
			if m.tabBuf(i) == first {
				m.storeTab()
				m.tabs = append(m.tabs[:i], m.tabs[i+1:]...)
				m.showTab(min(m.cur, len(m.tabs)-1))
				break
			}
		}
	}
	if i := m.findTab(current); i >= 0 {
		m.storeTab()
		m.showTab(i)
	}
	msg := fmt.Sprintf("Opened again what was open last time: %d files.", len(m.tabs))
	if len(m.tabs) == 1 {
		msg = "Opened again what was open last time: " + m.tabName(m.cur) + "."
	}
	if m.tree != nil {
		msg += " Project " + filepath.Base(m.tree.root) + "."
	}
	m.say(msg + " sy --new starts fresh.")
	return true
}
