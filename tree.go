package main

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
)

// The file manager: a folder tree down the left side. sy FOLDER opens a
// folder as a project with the tree showing; Alt+O or Ctrl+B shows it at
// any time. Typing finds files anywhere in the project.

type node struct {
	name     string
	path     string
	dir      bool
	depth    int
	open     bool
	loaded   bool
	children []*node
}

type fileTree struct {
	root       string
	top        *node
	rows       []*node // what is on screen, in order
	sel        int
	scroll     int
	showHidden bool
	query      string   // typed letters: find files by name
	found      []string // paths that match query
	all        []string // every file in the project, for finding (read once)
}

const findLimit = 60000 // files read for finding; enough for big projects

var skipDirs = map[string]bool{".git": true, "node_modules": true, "__pycache__": true,
	".cache": true, "target": true, ".venv": true, "venv": true, "dist": true, ".idea": true}

func newTree(root string) *fileTree {
	t := &fileTree{root: root}
	t.top = &node{name: filepath.Base(root), path: root, dir: true, open: true}
	t.load(t.top)
	t.flatten()
	return t
}

func (t *fileTree) load(n *node) {
	entries, err := os.ReadDir(n.path)
	n.loaded = true
	old := map[string]*node{}
	for _, c := range n.children {
		old[c.name] = c
	}
	n.children = nil
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if !t.showHidden && strings.HasPrefix(name, ".") {
			continue
		}
		dir := e.IsDir()
		if e.Type()&fs.ModeSymlink != 0 {
			if info, err := os.Stat(filepath.Join(n.path, name)); err == nil {
				dir = info.IsDir()
			}
		}
		c := &node{name: name, path: filepath.Join(n.path, name), dir: dir, depth: n.depth + 1}
		if was, ok := old[name]; ok && was.dir == dir {
			c.open, c.loaded, c.children = was.open, was.loaded, was.children
			if c.open {
				t.load(c)
			}
		}
		n.children = append(n.children, c)
	}
	sort.Slice(n.children, func(i, j int) bool {
		a, b := n.children[i], n.children[j]
		if a.dir != b.dir {
			return a.dir
		}
		return strings.ToLower(a.name) < strings.ToLower(b.name)
	})
}

func (t *fileTree) flatten() {
	t.rows = t.rows[:0]
	var walk func(n *node)
	walk = func(n *node) {
		t.rows = append(t.rows, n)
		if n.dir && n.open {
			for _, c := range n.children {
				walk(c)
			}
		}
	}
	walk(t.top)
	t.sel = max(0, min(t.sel, len(t.rows)-1))
}

func (t *fileTree) refresh() {
	t.load(t.top)
	t.all = nil
	t.flatten()
	if t.query != "" {
		t.find()
	}
}

// reveal opens the folders down to path and selects it.
func (t *fileTree) reveal(path string) {
	rel, err := filepath.Rel(t.root, path)
	if err != nil || strings.HasPrefix(rel, "..") || t.query != "" {
		return
	}
	n := t.top
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if !n.loaded {
			t.load(n)
		}
		n.open = true
		var next *node
		for _, c := range n.children {
			if c.name == part {
				next = c
			}
		}
		if next == nil {
			break
		}
		n = next
	}
	t.flatten()
	for i, r := range t.rows {
		if r == n {
			t.sel = i
		}
	}
}

// ---- finding ------------------------------------------------------------------------

func (t *fileTree) find() {
	if t.all == nil {
		t.all = []string{}
		filepath.WalkDir(t.root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			name := d.Name()
			if d.IsDir() && p != t.root && (skipDirs[name] || (!t.showHidden && strings.HasPrefix(name, "."))) {
				return filepath.SkipDir
			}
			if !d.IsDir() && (t.showHidden || !strings.HasPrefix(name, ".")) {
				t.all = append(t.all, p)
			}
			if len(t.all) >= findLimit {
				return filepath.SkipAll
			}
			return nil
		})
	}
	type hit struct {
		path  string
		score int
	}
	var hits []hit
	q := strings.ToLower(t.query)
	for _, p := range t.all {
		rel, _ := filepath.Rel(t.root, p)
		if s := fuzzy(strings.ToLower(rel), strings.ToLower(filepath.Base(p)), q); s > 0 {
			hits = append(hits, hit{p, s})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return len(hits[i].path) < len(hits[j].path)
	})
	t.found = t.found[:0]
	for i := 0; i < len(hits) && i < 300; i++ {
		t.found = append(t.found, hits[i].path)
	}
	t.sel = 0
	t.scroll = 0
}

// fuzzy scores how well q matches: letters in order, best when they are in
// the file name and next to each other. 0 = no match.
func fuzzy(rel, base, q string) int {
	if strings.Contains(base, q) {
		s := 1000 - len(base)
		if strings.HasPrefix(base, q) {
			s += 500
		}
		return s
	}
	if strings.Contains(rel, q) {
		return 500 - len(rel)
	}
	score, j, run := 1, 0, 0
	qr := []rune(q)
	for _, r := range rel {
		if j < len(qr) && unicode.ToLower(r) == qr[j] {
			j++
			run++
			score += run
		} else {
			run = 0
		}
	}
	if j < len(qr) {
		return 0
	}
	return score
}

// ---- keys -----------------------------------------------------------------------------

func (m *Model) treeKey(k tea.Key) tea.Cmd {
	t := m.tree
	name := m.altGrName(k)
	if m.altGr.down && (k.Code == 'e' || k.Code == 'o') {
		// On a UK keyboard AltGr+E types a plain "e"; kitty says AltGr is held.
		name = map[rune]string{'e': "altgr+e", 'o': "ø"}[k.Code]
	}
	if k.Text == "þ" || m.altGr.down && k.Code == 'p' {
		name = "altgr+p" // AltGr+P types þ on a UK keyboard
	}
	finding := t.query != ""
	count := len(t.rows)
	if finding {
		count = len(t.found)
	}
	switch name {
	case "alt+n", "alt+c", "tab", "altgr+tab", "ctrl+tab", "ctrl+w", "ctrl+pgup", "ctrl+pgdown":
		return bindings[name](m)
	case "esc":
		if finding {
			t.query = ""
			t.flatten()
			return nil
		}
		m.closeTree()
		return nil
	case "alt+l":
		m.closeTree()
		return m.openLoadList()
	case "ctrl+b", "alt+e", "altgr+e":
		m.closeTree() // Alt+E / AltGr+E: exit the file manager
		return nil
	case "ctrl+o", "alt+o", "ø":
		return nil // Alt+O / AltGr+O open it, and it is open already
	case "up":
		t.sel = max(0, t.sel-1)
	case "down":
		t.sel = min(max(count-1, 0), t.sel+1)
	case "pgup":
		t.sel = max(0, t.sel-10)
	case "pgdown":
		t.sel = min(max(count-1, 0), t.sel+10)
	case "home":
		t.sel = 0
	case "end":
		t.sel = max(count-1, 0)
	case "enter", "right":
		if finding {
			if t.sel < len(t.found) {
				return m.treeOpen(t.found[t.sel])
			}
			if p := expandPath(t.query); strings.ContainsAny(t.query, "/~") && fileExists(p) {
				return m.treeGo(p)
			}
			return nil
		}
		n := t.rows[t.sel]
		if n == t.top {
			return m.treeKey(tea.Key{Code: tea.KeyLeft}) // the "up a folder" row
		}
		if n.dir {
			if name == "right" && n.open && len(n.children) > 0 {
				t.sel++
				return nil
			}
			n.open = !n.open || name == "right"
			if n.open && !n.loaded {
				t.load(n)
			}
			t.flatten()
			return nil
		}
		return m.treeOpen(n.path)
	case "shift+enter", "ctrl+enter":
		// Every text file in the chosen folder (or the chosen file's folder) as tabs.
		if m.openFolder(m.treeFolder()) {
			m.closeTree()
		}
		return nil
	case "left":
		if finding {
			return nil
		}
		n := t.rows[t.sel]
		if n.dir && n.open && n != t.top {
			n.open = false
			t.flatten()
			return nil
		}
		if n == t.top {
			// Go up a folder: the parent becomes the top of the tree.
			parent := filepath.Dir(t.root)
			if parent != t.root {
				m.setRoot(parent)
				m.tree.reveal(n.path)
			}
			return nil
		}
		for i := t.sel - 1; i >= 0; i-- {
			if t.rows[i].depth < n.depth {
				t.sel = i
				break
			}
		}
	case "ctrl+s", "ß":
		return m.save()
	case "altgr+p", "alt+p":
		if p := m.treeSelected(); p != "" {
			if info, err := os.Stat(p); err == nil && !info.IsDir() {
				p = filepath.Dir(p)
			}
			m.setRoot(p)
			m.say("Project: " + shortPath(p))
		}
	case "alt+.":
		t.showHidden = !t.showHidden
		t.refresh()
		if t.showHidden {
			m.say("Showing hidden files.")
		} else {
			m.say("Hiding hidden files.")
		}
	case "f5":
		t.refresh()
		m.say("Folder read again.")
	case "f2":
		if p := m.treeSelected(); p != "" && p != t.root {
			m.treeTarget = p
			cmd := m.ask(pRename, "New name: ", filepath.Base(p))
			return cmd
		}
	case "ctrl+n":
		m.treeTarget = m.treeFolder()
		return m.ask(pNewFile, "New file in "+shortPath(m.treeTarget)+"/: ", "")
	case "f7":
		m.treeTarget = m.treeFolder()
		return m.ask(pNewFolder, "New folder in "+shortPath(m.treeTarget)+"/: ", "")
	case "delete":
		if p := m.treeSelected(); p != "" && p != t.root {
			return m.confirm("Move "+filepath.Base(p)+" to the Trash?  y = yes   n = no", "yn",
				func(m *Model, k rune) tea.Cmd {
					if k == 'y' {
						m.trash(p)
					}
					return nil
				})
		}
	case "backspace":
		if finding {
			r := []rune(t.query)
			t.query = string(r[:len(r)-1])
			if t.query == "" {
				t.flatten()
			} else {
				t.find()
			}
		}
	default:
		if k.Text != "" && !k.Mod.Contains(tea.ModCtrl) && !k.Mod.Contains(tea.ModAlt) {
			t.query += k.Text
			t.find()
		}
	}
	return nil
}

// treeSelected is the path of the chosen row.
func (m *Model) treeSelected() string {
	t := m.tree
	if t.query != "" {
		if t.sel < len(t.found) {
			return t.found[t.sel]
		}
		return ""
	}
	if t.sel < len(t.rows) {
		return t.rows[t.sel].path
	}
	return ""
}

// treeFolder is the folder of the chosen row (for new files).
func (m *Model) treeFolder() string {
	p := m.treeSelected()
	if p == "" {
		return m.tree.root
	}
	if info, err := os.Stat(p); err == nil && info.IsDir() {
		return p
	}
	return filepath.Dir(p)
}

func (m *Model) treeOpen(path string) tea.Cmd {
	cmd := m.openPath(path)
	m.closeTree()
	m.tree.query = ""
	m.tree.flatten()
	m.tree.reveal(path)
	return cmd
}

// treeGo goes to a typed path: a folder becomes the project, a file opens.
func (m *Model) treeGo(p string) tea.Cmd {
	if info, err := os.Stat(p); err == nil && info.IsDir() {
		m.setRoot(p)
		m.say("Project: " + shortPath(p))
		return nil
	}
	return m.treeOpen(p)
}

func (m *Model) setRoot(dir string) {
	hidden := m.tree != nil && m.tree.showHidden
	m.tree = newTree(dir)
	if hidden {
		m.tree.showHidden = true
		m.tree.refresh()
	}
}

func (m *Model) refreshTree() {
	if m.tree != nil {
		m.tree.refresh()
	}
}

// openTree is Alt+O / Ctrl+B: show the file manager and work in it.
func (m *Model) openTree(toggle bool) tea.Cmd {
	if toggle && m.showTree {
		m.closeTree()
		return nil
	}
	if m.tree == nil {
		m.setRoot(m.baseDir())
	}
	m.showTree, m.treeFocus = true, true
	if m.buf.Path != "" {
		m.tree.reveal(m.buf.Path)
	}
	return nil
}

func (m *Model) trash(p string) {
	var err error
	if have("gio") {
		err = exec.Command("gio", "trash", p).Run()
	} else if have("trash-put") {
		err = exec.Command("trash-put", p).Run()
	} else {
		m.sayErr("No Trash tool (gio or trash-put) found, so nothing was deleted.")
		return
	}
	if err != nil {
		m.sayErr("Could not move it to the Trash: " + err.Error())
		return
	}
	if i := m.findTab(p); i >= 0 {
		m.say(filepath.Base(p) + " is in the Trash. Its tab stays open; close it with Ctrl+W.")
	} else {
		m.say(filepath.Base(p) + " is in the Trash.")
	}
	m.tree.refresh()
}

// treeAnswer handles the rename / new file / new folder prompts.
func (m *Model) treeAnswer(kind promptKind, value string) tea.Cmd {
	value = strings.TrimSpace(value)
	if value == "" || strings.Contains(value, "/") && kind == pRename {
		m.say("Cancelled.")
		return nil
	}
	switch kind {
	case pRename:
		dst := filepath.Join(filepath.Dir(m.treeTarget), value)
		if fileExists(dst) {
			m.sayErr(value + " already exists.")
			return nil
		}
		if err := os.Rename(m.treeTarget, dst); err != nil {
			m.sayErr("Could not rename: " + err.Error())
			return nil
		}
		m.storeTab()
		for i, t := range m.tabs {
			if t.buf.Path == m.treeTarget {
				t.buf.Path = dst
				if i == m.cur {
					m.resetHighlighter()
				}
			}
		}
		m.tree.refresh()
		m.tree.reveal(dst)
		m.say("Renamed to " + value + ".")
	case pNewFolder:
		dst := filepath.Join(m.treeTarget, value)
		if err := os.MkdirAll(dst, 0o755); err != nil {
			m.sayErr("Could not make the folder: " + err.Error())
			return nil
		}
		m.tree.refresh()
		m.tree.reveal(dst)
		m.say("Made folder " + value + ".")
	case pNewFile:
		dst := filepath.Join(m.treeTarget, value)
		if !fileExists(dst) {
			os.MkdirAll(filepath.Dir(dst), 0o755)
			if err := os.WriteFile(dst, nil, 0o644); err != nil {
				m.sayErr("Could not make the file: " + err.Error())
				return nil
			}
		}
		m.tree.refresh()
		return m.treeOpen(dst)
	}
	return nil
}
