package main

// Files reload by themselves: when another program (git, a formatter, another
// editor) changes an open file, the new text comes in within a second. Only
// the lines that changed are replaced, so the cursor and the view stay put,
// and Ctrl+Z takes the outside change back.
//
// If the text in Symbolizer has changes not yet saved, the file is not
// reloaded: saving pauses and asks instead (autosave.go), so nothing is lost.

import (
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

type diskTick struct{}

const diskEvery = 700 * time.Millisecond

func watchDisk() tea.Cmd {
	return tea.Tick(diskEvery, func(time.Time) tea.Msg { return diskTick{} })
}

// checkDisk reloads every open file another program changed.
func (m *Model) checkDisk() {
	m.storeTab()
	for i, t := range m.tabs {
		if i == m.cur && (m.saving || m.saveWaiting) {
			continue // our own save is on its way: its time is not known yet
		}
		if !m.reloadBuffer(t.buf) {
			continue
		}
		if i == m.cur {
			m.ensureVisible()
			m.say("Another program changed this file. The new text is in. Ctrl+Z takes it back.")
		} else {
			m.say("Another program changed " + m.tabName(i) + ". The new text is in.")
		}
	}
}

// reloadBuffer puts the file's new text into b if it changed on disk and b
// has nothing unsaved. It says whether the text changed.
func (m *Model) reloadBuffer(b *Buffer) bool {
	if b == nil || b.Binary || b.Modified() || !b.ChangedOnDisk() {
		return false
	}
	info, err := os.Stat(b.Path)
	if err != nil || info.Size() > 64<<20 {
		return false
	}
	nb := NewBuffer()
	if err := nb.Load(b.Path); err != nil {
		return false
	}
	changed := b.replaceLines(nb.Lines)
	b.CRLF, b.FinalNewline = nb.CRLF, nb.FinalNewline
	b.MarkSaved(b.Version(), nb.ModTime)
	return changed
}

// replaceLines swaps in new text, changing only the lines between the first
// and last that differ, as one undo step. The cursor keeps its place.
func (b *Buffer) replaceLines(lines [][]rune) bool {
	old := b.Lines
	same := func(a, c []rune) bool { return string(a) == string(c) }
	p := 0
	for p < len(old) && p < len(lines) && same(old[p], lines[p]) {
		p++
	}
	s := 0
	for s < len(old)-p && s < len(lines)-p && same(old[len(old)-1-s], lines[len(lines)-1-s]) {
		s++
	}
	if p == len(old) && p == len(lines) {
		return false
	}
	mid := make([]string, 0, len(lines)-s-p)
	for _, l := range lines[p : len(lines)-s] {
		mid = append(mid, string(l))
	}
	text := strings.Join(mid, "\n")
	var start, end Pos
	switch {
	case s > 0: // whole lines p .. len(old)-s-1
		start, end = Pos{p, 0}, Pos{len(old) - s, 0}
		if len(mid) > 0 {
			text += "\n"
		}
	case p > 0: // from the end of line p-1 to the end of the file
		start = Pos{p - 1, len(old[p-1])}
		end = Pos{len(old) - 1, len(old[len(old)-1])}
		if len(mid) > 0 {
			text = "\n" + text
		}
	default:
		end = Pos{len(old) - 1, len(old[len(old)-1])}
	}
	cur := b.Cursor
	delta := len(lines) - len(old)
	b.BreakUndo()
	b.Change(start, end, text, "")
	b.BreakUndo()
	if cur.Row >= len(old)-s && s > 0 {
		cur.Row += delta // below the change: moves with its line
	}
	b.Cursor, b.Anchor, b.WantCol = b.Clamp(cur), nil, -1
	return true
}
