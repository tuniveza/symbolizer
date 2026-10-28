package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Opening, naming and closing files. Saving itself happens on its own
// (autosave.go); Ctrl+S only names a session or saves at once.

// save is Ctrl+S.
func (m *Model) save() tea.Cmd {
	b := m.buf
	if b.Draft {
		return m.askName()
	}
	if m.savePaused {
		return m.confirm("Another program changed this file. Save yours over it?  y = yes   n = no", "yn",
			func(m *Model, k rune) tea.Cmd {
				if k != 'y' {
					m.say("Not saved. Alt+L can load their version.")
					return nil
				}
				m.savePaused = false
				return m.saveAs(m.buf.Path)
			})
	}
	if m.saveNow() {
		m.say(fmt.Sprintf("Saved %s (%d lines). It also saves by itself after every change.",
			filepath.Base(b.Path), len(b.Lines)))
	}
	return nil
}

// askName asks what to call a session. The ending (.cpp, .c, .py ...) comes
// from what the text looks like, and is added if the name has none.
func (m *Model) askName() tea.Cmd {
	lang, ext := guessLanguage(m.sample(32 << 10))
	m.nameExt = ext
	label := fmt.Sprintf("Name it (%s, %s is added): ", lang, ext)
	if lang == "Text" {
		label = "Name it (.txt is added): "
	}
	cmd := m.ask(pName, label, "")
	m.nameSuggest, _ = m.buf.suggestName()
	m.input.Placeholder = m.nameSuggest + "   (Enter takes this name)"
	return cmd
}

// nameEntered turns what was typed into a path in the project or start folder.
func (m *Model) nameEntered(value string) tea.Cmd {
	value = strings.TrimSpace(value)
	if value == "" {
		value = m.nameSuggest // Enter on its own takes the suggested name
	}
	if value == "" {
		m.say("Not named. It is still kept as a session.")
		return nil
	}
	if filepath.Ext(filepath.Base(value)) == "" {
		value += m.nameExt
	}
	path := value
	if !filepath.IsAbs(path) && !strings.HasPrefix(path, "~") {
		path = filepath.Join(m.baseDir(), path)
	}
	path = expandPath(path)
	if fileExists(path) {
		return m.confirm(filepath.Base(path)+" already exists. Replace it?  y = yes   n = no", "yn",
			func(m *Model, k rune) tea.Cmd {
				if k == 'y' {
					return m.saveAs(path)
				}
				return m.askName()
			})
	}
	return m.saveAs(path)
}

// baseDir is where new names go: the project folder, else where sy started.
func (m *Model) baseDir() string {
	if m.tree != nil {
		return m.tree.root
	}
	if m.buf.Path != "" && !m.buf.Draft {
		return filepath.Dir(m.buf.Path)
	}
	return m.startDir
}

// saveAs writes the text to path and keeps editing it there. A session that
// gets a name moves into the named file.
func (m *Model) saveAs(path string) tea.Cmd {
	b := m.buf
	old, wasDraft := b.Path, b.Draft
	if dir := filepath.Dir(path); !fileExists(dir) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			m.sayErr("Could not make the folder: " + err.Error())
			return nil
		}
	}
	if err := b.Save(path); err != nil {
		m.sayErr("Could not save: " + err.Error())
		return nil
	}
	b.Draft = isSession(path)
	if wasDraft && old != "" && old != path {
		os.Remove(old)
	}
	m.savePaused, m.saveErr = false, ""
	m.resetHighlighter()
	m.refreshTree()
	m.say(fmt.Sprintf("Saved as %s in %s. It keeps saving by itself.", filepath.Base(path), shortPath(filepath.Dir(path))))
	return nil
}

// ifUnsaved runs fn once the text is safely saved. Only if saving fails
// (or is paused because another program changed the file) does it ask.
func (m *Model) ifUnsaved(what string, fn func(m *Model) tea.Cmd) tea.Cmd {
	if m.saveNow() {
		return fn(m)
	}
	if m.savePaused {
		return m.confirm("Another program changed this file. "+what+"  k = keep mine (save over theirs)   d = drop mine   Esc = cancel",
			"kd", func(m *Model, k rune) tea.Cmd {
				if k == 'k' {
					m.savePaused = false
					if err := m.buf.Save(""); err != nil {
						m.sayErr("Could not save: " + err.Error())
						return nil
					}
				}
				return fn(m)
			})
	}
	return m.confirm("Could not save ("+m.saveErr+"). "+what+"  d = drop the changes   Esc = cancel",
		"d", func(m *Model, k rune) tea.Cmd { return fn(m) })
}

// openPath opens a file in a tab (or shows its tab if already open).
func (m *Model) openPath(path string) tea.Cmd {
	if i := m.findTab(path); i >= 0 {
		return m.switchTab(i)
	}
	nb := NewBuffer()
	if fileExists(path) {
		if err := nb.Load(path); err != nil {
			m.sayErr("Could not open: " + err.Error())
			return nil
		}
		m.say(fmt.Sprintf("Opened %s (%d lines).", filepath.Base(path), len(nb.Lines)))
	} else {
		nb.Path = path
		m.say("New file " + filepath.Base(path) + ". It saves by itself as you type.")
	}
	nb.Draft = isSession(path)
	m.saveNow()
	if m.blankDraft() {
		m.buf = nb
		m.top, m.left, m.savePaused, m.saveErr = 0, 0, false, ""
		m.resetHighlighter()
		m.storeTab()
		return nil
	}
	m.addTab(nb)
	return nil
}

// reload loads the file on screen again from disk (Alt+L list).
func (m *Model) reload() tea.Cmd {
	path := m.buf.Path
	if path == "" || !fileExists(path) {
		m.sayErr("Not on disk yet, so there is nothing to load.")
		return nil
	}
	if m.savePaused {
		// Their version wins: that is what loading it again means.
		m.savePaused = false
		m.buf.MarkSaved(m.buf.Version(), m.buf.ModTime)
	}
	return m.ifUnsaved("Load it from disk?", func(m *Model) tea.Cmd {
		nb := NewBuffer()
		if err := nb.Load(path); err != nil {
			m.sayErr("Could not load: " + err.Error())
			return nil
		}
		nb.Draft = m.buf.Draft
		cur, top := m.buf.Cursor, m.top
		m.buf = nb
		m.resetHighlighter()
		m.buf.Cursor, m.top = nb.Clamp(cur), min(top, len(nb.Lines)-1)
		m.saveErr = ""
		m.storeTab()
		m.say("Loaded " + filepath.Base(path) + " from disk.")
		return nil
	})
}

// newFile is Ctrl+N: a new session in a new tab.
func (m *Model) newFile() tea.Cmd {
	m.saveNow()
	if m.blankDraft() {
		m.say("This is already a new session. Type, and it saves by itself.")
		return nil
	}
	m.addTab(newDraft())
	m.say("New session. It saves by itself. Ctrl+S gives it a name.")
	return nil
}

func (m *Model) quit() tea.Cmd {
	return m.ifUnsaved("Quit anyway?", func(m *Model) tea.Cmd {
		m.storeTab()
		for _, t := range m.tabs {
			dropEmptySession(t.buf)
		}
		m.quitClean = true
		return tea.Quit
	})
}

// shortPath writes the home folder as ~.
func shortPath(p string) string {
	if home, err := os.UserHomeDir(); err == nil && (p == home || strings.HasPrefix(p, home+"/")) {
		return "~" + p[len(home):]
	}
	return p
}

// rescue writes unsaved text somewhere safe if Symbolizer crashes.
func rescue(b *Buffer) string {
	if b == nil || !b.Modified() {
		return ""
	}
	path := b.Path + ".sy-rescue"
	if b.Path == "" {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, "symbolizer-rescue.txt")
	}
	if err := os.WriteFile(path, []byte(b.Text()+"\n"), 0o600); err != nil {
		return ""
	}
	return path
}
