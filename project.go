package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// A project is a folder opened from the file manager with its files as
// tabs. Only one is open at a time, so the tab row never fills up:
//
//	Right, Right       on a folder: open it as a project (Shift+Right, Shift+Right:
//	                   only text with something in it; Shift+Enter: text files)
//	Left straight after  undo it: its tabs close and the file manager is back
//	                   where it was, as if it never happened
//	Left, Left         on the project's folder: close the project, so another
//	                   can open
//
// While a project is open, the file manager's top row is its folder.

// undoWindow is how soon after opening a project Left undoes it.
const undoWindow = 2 * time.Second

type project struct {
	dir  string
	kept map[string]bool // files open before the project: closing it leaves them
	back *fileTree       // the file manager as it was before, to go back to
}

// projectUndo is everything as it was just before a project opened.
type projectUndo struct {
	at       time.Time
	tabs     []tab
	cur      int
	tree     *fileTree
	showTree bool
}

// inFolder says whether path is dir or inside it.
func inFolder(dir, path string) bool {
	return path == dir || strings.HasPrefix(path, dir+string(filepath.Separator))
}

// openProject opens dir as the project: its files (the ones pick chooses)
// as tabs, and the file manager on that folder. It says whether it opened.
func (m *Model) openProject(dir string, pick filePick) bool {
	if p := m.project; p != nil {
		if p.dir == dir {
			m.say("Project " + filepath.Base(dir) + " is open already.")
		} else {
			m.say("Project " + filepath.Base(p.dir) + " is open. One at a time: close it first with Left twice on its folder.")
		}
		return false
	}
	m.storeTab()
	u := &projectUndo{cur: m.cur, tree: m.tree, showTree: m.showTree}
	kept := map[string]bool{}
	for i, t := range m.tabs {
		u.tabs = append(u.tabs, *t)
		kept[m.tabBuf(i).Path] = true
	}
	if !m.openFolder(dir, pick) {
		return false
	}
	m.project = &project{dir: dir, kept: kept, back: m.tree}
	if m.tree == nil || m.tree.root != dir {
		m.setRoot(dir)
	} else {
		m.project.back = nil // it was the file manager's folder already
	}
	u.at = time.Now()
	m.projUndo = u
	m.say("Project " + filepath.Base(dir) + ": " + m.msg + " Left now undoes it.")
	return true
}

// undoProjectKey is the first key after a project opened: a quick Left
// undoes it. Any other key keeps the project. It says whether it undid.
func (m *Model) undoProjectKey(name string) bool {
	u := m.projUndo
	if u == nil {
		return false
	}
	m.projUndo = nil
	if name != "left" || time.Since(u.at) > undoWindow || m.prompt != pNone || m.help || m.tabMode || m.showTree {
		return false
	}
	dir := m.project.dir
	m.tabs = nil
	for i := range u.tabs {
		t := u.tabs[i]
		m.tabs = append(m.tabs, &t)
	}
	m.showTab(u.cur)
	m.resetHighlighter()
	m.tabs[m.cur].hl = m.hl
	m.tree, m.project = u.tree, nil
	m.showTree, m.treeFocus = true, true
	m.treeArm = ""
	m.say("Undone: project " + filepath.Base(dir) + " closed. Back where you were.")
	return true
}

// closeProject closes the project's tabs (not files that were open before
// it, nor any with changes that could not be saved) and goes back to where
// the file manager was before it opened.
func (m *Model) closeProject() {
	p := m.project
	if p == nil {
		return
	}
	m.saveNow()
	m.storeTab()
	shown := m.buf
	var keep []*tab
	closed, held := 0, 0
	for _, t := range m.tabs {
		b := t.buf
		switch {
		case !inFolder(p.dir, b.Path) || p.kept[b.Path]:
			keep = append(keep, t)
		case b.Modified() && !b.Binary:
			keep = append(keep, t) // its changes are not saved: it stays
			held++
		default:
			closed++
		}
	}
	m.tabs, m.cur = keep, 0
	if len(keep) == 0 {
		m.tabs = nil
		m.addTab(newDraft())
	} else {
		for i, t := range keep {
			if t.buf == shown {
				m.cur = i
			}
		}
		m.showTab(m.cur)
	}
	m.project, m.projUndo = nil, nil
	switch {
	case m.tree == nil:
		m.setRoot(filepath.Dir(p.dir))
	case m.tree.root == p.dir && p.back != nil:
		m.tree = p.back // where the file manager was before the project opened
		m.tree.refresh()
	}
	m.tree.reveal(p.dir)
	m.showTree, m.treeFocus = true, true
	msg := fmt.Sprintf("Closed project %s (%d tabs). Choose another folder: Right twice opens it.", filepath.Base(p.dir), closed)
	if held > 0 {
		msg += fmt.Sprintf(" %d with changes not saved stay open.", held)
	}
	m.say(msg)
}

// newProject is a project with nothing to go back to (opened last time).
func newProject(dir string) *project {
	return &project{dir: dir, kept: map[string]bool{}}
}
