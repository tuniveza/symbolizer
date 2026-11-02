package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func projectDir(t *testing.T) string {
	dir := t.TempDir()
	for _, rel := range []string{"one/a.go", "one/b.txt", "two/c.go", "two/d.md", "loose.txt"} {
		p := filepath.Join(dir, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("x\n"), 0o644)
	}
	return dir
}

func tabPaths(m *Model) string {
	var names []string
	for i := range m.tabs {
		names = append(names, filepath.Base(m.tabBuf(i).Path))
	}
	return strings.Join(names, " ")
}

func TestProjectRightTwiceLeftUndo(t *testing.T) {
	dir := projectDir(t)
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	m, _ := NewModel(dir, 0)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.openPath(filepath.Join(dir, "loose.txt")) // open before: not part of a project
	press(m, "alt+o")
	m.tree.reveal(filepath.Join(dir, "one"))
	sel := m.tree.sel

	// Right once: the folder opens in the file manager, but no tabs.
	press(m, "right")
	if row := m.tree.rows[m.tree.sel]; !row.open || row.name != "one" || len(m.tree.rows) != 6 {
		t.Fatalf("Right once should open the folder in the file manager (on %s, open %v, %d rows)", row.name, row.open, len(m.tree.rows))
	}
	if len(m.tabs) != 1 || !m.showTree || !strings.Contains(m.msg, "Right again opens one") {
		t.Fatalf("Right once: %d tabs, tree %v, message %q", len(m.tabs), m.showTree, m.msg)
	}
	// Right twice: the project opens.
	press(m, "right")
	if got := tabPaths(m); got != "loose.txt a.go b.txt" || m.showTree || m.project == nil || m.tree.root != filepath.Join(dir, "one") {
		t.Fatalf("Right twice: tabs %q, tree %v, root %s", got, m.showTree, m.tree.root)
	}
	// Left straight after: all gone, back in the file manager where it was.
	press(m, "left")
	if got := tabPaths(m); got != "loose.txt" || !m.showTree || !m.treeFocus || m.project != nil ||
		m.tree.root != dir || m.tree.sel != sel || m.buf.Cursor.Col != 0 {
		t.Fatalf("quick Left: tabs %q, tree %v, root %s, sel %d (want %d)", got, m.showTree, m.tree.root, m.tree.sel, sel)
	}

	// A Left that is not quick moves the cursor and keeps the project.
	press(m, "right")
	press(m, "right")
	m.projUndo.at = m.projUndo.at.Add(-3 * time.Second)
	press(m, "left")
	if m.project == nil || len(m.tabs) != 3 {
		t.Fatal("a slow Left should keep the project")
	}
	// Any other key first also keeps it.
	if m.projUndo != nil {
		t.Fatal("the undo should be gone after one key")
	}

	// One project at a time: Right on another folder says so and opens nothing.
	press(m, "alt+o")
	press(m, "home")
	press(m, "left") // top row: the project itself
	if m.project == nil || !strings.Contains(m.msg, "Left again closes project one") {
		t.Fatalf("Left once on the project: message %q", m.msg)
	}
	if screen := ansi.Strip(m.View().Content); !strings.Contains(screen, "project one") {
		t.Fatalf("the top row should say it is the project:\n%s", screen)
	}
	press(m, "esc")
	press(m, "alt+o")
	m.setRoot(dir) // look at the folder above, with the project still open
	m.tree.reveal(filepath.Join(dir, "two"))
	press(m, "right")
	if row := m.tree.rows[m.tree.sel]; !row.open || !strings.Contains(m.msg, "cannot open as a project") {
		t.Fatalf("Right once with a project open: folder open %v, message %q", row.open, m.msg)
	}
	press(m, "right")
	if len(m.tabs) != 3 || !strings.Contains(m.msg, "One at a time") {
		t.Fatalf("a second project: %d tabs, message %q", len(m.tabs), m.msg)
	}
	// Left twice on the project's folder closes it; then another opens.
	m.tree.reveal(filepath.Join(dir, "one"))
	press(m, "left")
	press(m, "left")
	if got := tabPaths(m); got != "loose.txt" || m.project != nil || !m.showTree || !strings.Contains(m.msg, "Closed project one (2 tabs)") {
		t.Fatalf("Left twice: tabs %q, message %q", got, m.msg)
	}
	m.tree.reveal(filepath.Join(dir, "two"))
	press(m, "right")
	press(m, "right")
	if got := tabPaths(m); got != "loose.txt c.go d.md" || m.project == nil {
		t.Fatalf("after closing, the next project: tabs %q, message %q", got, m.msg)
	}

	// The project is remembered for next time, and closes then too.
	m.saveLastOpen()
	data, _ := os.ReadFile(lastOpenPath())
	if !strings.Contains(string(data), "opened "+filepath.Join(dir, "two")+"\n") {
		t.Fatalf("last-open does not keep the project:\n%s", data)
	}
	m2, _ := NewModel("", 0)
	m2.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if !m2.reopenLast() || m2.project == nil || m2.project.dir != filepath.Join(dir, "two") {
		t.Fatal("the project did not come back")
	}
	press(m2, "alt+o")
	press(m2, "home")
	press(m2, "left")
	press(m2, "left")
	if got := tabPaths(m2); got != "loose.txt" || m2.project != nil {
		t.Fatalf("closing the project after a restart: tabs %q, message %q", got, m2.msg)
	}
}

// Closing back to the place it opened from, with the top row the project.
func TestProjectCloseGoesBack(t *testing.T) {
	dir := projectDir(t)
	m, _ := NewModel(dir, 0)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.tree.reveal(filepath.Join(dir, "two"))
	sel := m.tree.sel
	press(m, "shift+right")
	press(m, "shift+right")
	if got := tabPaths(m); got != "c.go d.md" {
		t.Fatalf("Shift+Right twice: tabs %q", got)
	}
	press(m, "down") // not Left: the project stays
	press(m, "alt+o")
	press(m, "home")
	press(m, "right") // top row: Right says how to close, and does not go up
	if m.tree.root != filepath.Join(dir, "two") || !strings.Contains(m.msg, "Left twice closes it") {
		t.Fatalf("Right on the project's top row: root %s, message %q", m.tree.root, m.msg)
	}
	press(m, "left")
	press(m, "down") // another key in between: not twice
	press(m, "home")
	press(m, "left")
	if m.project == nil {
		t.Fatal("Left, another key, Left should not close the project")
	}
	press(m, "left")
	if m.project != nil || m.tree.root != dir || m.tree.sel != sel || len(m.tabs) != 1 || !m.blankDraft() {
		t.Fatalf("closed: root %s, sel %d (want %d), tabs %q", m.tree.root, m.tree.sel, sel, tabPaths(m))
	}
}

// A tab with changes that could not be saved stays open when the project closes.
func TestProjectCloseKeepsUnsaved(t *testing.T) {
	dir := projectDir(t)
	m, _ := NewModel(dir, 0)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.tree.reveal(filepath.Join(dir, "one"))
	press(m, "right")
	press(m, "right")
	press(m, "down")
	typeStr(m, "more")
	os.Chmod(filepath.Join(dir, "one"), 0o500)
	os.Chmod(m.buf.Path, 0o400) // it cannot be saved
	defer os.Chmod(filepath.Join(dir, "one"), 0o755)
	m.closeProject()
	if len(m.tabs) != 1 || !strings.Contains(m.msg, "1 with changes not saved stay open") {
		t.Fatalf("tabs %q, message %q", tabPaths(m), m.msg)
	}
}
