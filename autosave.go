package main

import (
	"os"
	"sync"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
)

// Saving happens by itself, a moment after each change. Unnamed text is
// saved as a session (see sessions.go). If another program changes the same
// file, saving pauses so its changes are not overwritten.

type saveTick struct{}

type saveDone struct {
	path    string
	version int
	mod     time.Time
	wrote   bool
	err     error
}

// Writes go one at a time and never put older text over newer text, even
// when a background save and Ctrl+S overlap.
var (
	writeMu  sync.Mutex
	written  = map[string]uint64{}
	writeSeq atomic.Uint64
)

func writeOrdered(path string, data []byte, seq uint64) (mod time.Time, wrote bool, err error) {
	writeMu.Lock()
	defer writeMu.Unlock()
	if seq <= written[path] {
		return time.Time{}, false, nil // newer text is already on disk
	}
	mod, err = writeAtomic(path, data)
	if err == nil {
		written[path] = seq
	}
	return mod, err == nil, err
}

// wantSave asks for a save soon if the text changed.
func (m *Model) wantSave() tea.Cmd {
	b := m.buf
	if !b.Modified() || m.saving || m.saveWaiting || m.savePaused {
		return nil
	}
	if b.Path == "" {
		if !b.Draft {
			return nil
		}
		if len(b.Lines) == 1 && len(b.Lines[0]) == 0 {
			return nil // nothing typed yet: no session file
		}
		path, err := newSessionPath()
		if err != nil {
			m.saveErr = err.Error()
			return nil
		}
		b.Path = path
	}
	m.saveWaiting = true
	wait := 100 * time.Millisecond
	if len(b.Lines) > 100000 {
		wait = time.Second
	}
	return tea.Tick(wait, func(time.Time) tea.Msg { return saveTick{} })
}

func (m *Model) startSave() tea.Cmd {
	m.saveWaiting = false
	b := m.buf
	if b.Draft {
		m.followLanguage()
	}
	if !b.Modified() || b.Path == "" || m.savePaused {
		return nil
	}
	if b.ChangedOnDisk() {
		m.pauseSaving()
		return nil
	}
	path, snap, version, seq := b.Path, b.Snapshot(), b.Version(), writeSeq.Add(1)
	m.saving = true
	return func() tea.Msg {
		mod, wrote, err := writeOrdered(path, snap(), seq)
		return saveDone{path: path, version: version, mod: mod, wrote: wrote, err: err}
	}
}

func (m *Model) finishSave(msg saveDone) {
	m.saving = false
	if msg.path != m.buf.Path {
		return // a different file is open now
	}
	if msg.err != nil {
		m.saveErr = msg.err.Error()
		m.sayErr("Could not save: " + msg.err.Error())
		return
	}
	m.saveErr = ""
	if msg.wrote && msg.version > m.buf.savedVersion {
		m.buf.MarkSaved(msg.version, msg.mod)
	}
}

func (m *Model) pauseSaving() {
	m.savePaused = true
	m.sayErr("Another program changed this file. Saving is paused. Alt+L → reload it, or Ctrl+S → keep yours.")
}

// saveNow saves at once (Ctrl+S, quitting, switching files). It returns
// false if the text could not be saved.
func (m *Model) saveNow() bool {
	b := m.buf
	if !b.Modified() {
		return true
	}
	if b.Path == "" {
		if !b.Draft || (len(b.Lines) == 1 && len(b.Lines[0]) == 0) {
			return !b.Draft || true
		}
		path, err := newSessionPath()
		if err != nil {
			m.saveErr = err.Error()
			return false
		}
		b.Path = path
	}
	if m.savePaused {
		return false
	}
	if b.ChangedOnDisk() {
		m.pauseSaving()
		return false
	}
	version := b.Version()
	mod, wrote, err := writeOrdered(b.Path, b.Encode(), writeSeq.Add(1))
	if err != nil {
		m.saveErr = err.Error()
		m.sayErr("Could not save: " + err.Error())
		return false
	}
	m.saveErr = ""
	if wrote {
		b.MarkSaved(version, mod)
	}
	return true
}

// followLanguage re-guesses a session's language as it is typed, so the
// colours match what is being written.
func (m *Model) followLanguage() {
	name, _ := guessLanguage(m.sample(32 << 10))
	if name != m.hl.Name {
		m.resetHighlighter()
	}
}

// sample is the start of the text, up to n bytes.
func (m *Model) sample(n int) string {
	buf := make([]rune, 0, 1024)
	size := 0
	for _, l := range m.buf.Lines {
		buf = append(append(buf, l...), '\n')
		size += len(l) + 1
		if size > n {
			break
		}
	}
	return string(buf)
}

// dropEmptySession removes a session file that holds no text.
func dropEmptySession(b *Buffer) {
	if b.Draft && b.Path != "" && len(b.Lines) == 1 && len(b.Lines[0]) == 0 {
		os.Remove(b.Path)
	}
}
