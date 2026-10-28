package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

// Pos is a place in the text: a line and a letter on that line (both from 0).
type Pos struct{ Row, Col int }

func (a Pos) Less(b Pos) bool { return a.Row < b.Row || (a.Row == b.Row && a.Col < b.Col) }

// edit is one change, kept so it can be undone: at At, Old was replaced by New.
type edit struct {
	At       Pos
	Old, New string
}

// step is everything one undo press takes back.
type step struct {
	edits        []edit
	cursorBefore Pos
	anchorBefore *Pos
	cursorAfter  Pos
	kind         string
	when         time.Time
}

// Buffer is the text being edited.
type Buffer struct {
	Path         string
	Lines        [][]rune
	Cursor       Pos
	Anchor       *Pos // where the selection started, nil when nothing is selected
	WantCol      int  // screen column kept while moving up and down (-1 = none)
	Indent       string
	CRLF         bool
	FinalNewline bool
	ModTime      time.Time
	Draft        bool  // no name yet: saved to the drafts folder until Ctrl+S names it
	Centre       *bool // lines centred in the zone (nil: decided by the language)

	// the file-like name shown for a session (names.go)
	nameCache, nameExt string
	nameVersion        int
	nameText           string    // the text the name was guessed from
	nameAt             time.Time // when it was last guessed
	nameSeed           uint64

	undo, redo   []*step
	version      int
	savedVersion int

	// OnSplice is told when lines are added or removed, so the colour cache
	// can shift with the text.
	OnSplice func(row, removed, added int)
}

func NewBuffer() *Buffer {
	return &Buffer{Lines: [][]rune{{}}, WantCol: -1, Indent: "    ", FinalNewline: true}
}

// ---- files ----------------------------------------------------------------

func (b *Buffer) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	info, _ := os.Stat(path)
	b.CRLF = bytes.Contains(data[:min(len(data), 10000)], []byte("\r\n"))
	if b.CRLF {
		data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	}
	b.FinalNewline = len(data) == 0 || data[len(data)-1] == '\n'
	if len(data) > 0 && data[len(data)-1] == '\n' {
		data = data[:len(data)-1]
	}
	parts := bytes.Split(data, []byte("\n"))
	b.Lines = make([][]rune, len(parts))
	for i, p := range parts {
		b.Lines[i] = []rune(string(p))
	}
	b.Path = path
	if info != nil {
		b.ModTime = info.ModTime()
	}
	b.Cursor, b.Anchor, b.WantCol = Pos{}, nil, -1
	b.undo, b.redo = nil, nil
	b.version, b.savedVersion = 0, 0
	b.guessIndent()
	return nil
}

func (b *Buffer) guessIndent() {
	tabs, spaces := 0, 0
	for i, l := range b.Lines {
		if i > 2000 {
			break
		}
		if len(l) > 0 && l[0] == '\t' {
			tabs++
		} else if len(l) > 1 && l[0] == ' ' && l[1] == ' ' {
			spaces++
		}
	}
	if tabs > spaces {
		b.Indent = "\t"
	} else {
		b.Indent = "    "
	}
}

// Encode is the file as it goes to disk: line endings as the file had them,
// and a final newline if it had one.
func (b *Buffer) Encode() []byte { return b.Snapshot()() }

// Snapshot copies the list of lines (fast: lines are never changed in
// place, only replaced) and returns a function that builds the file from
// that copy. The slow part can then run in the background.
func (b *Buffer) Snapshot() func() []byte {
	lines := append([][]rune(nil), b.Lines...)
	crlf, final := b.CRLF, b.FinalNewline
	return func() []byte {
		nl := "\n"
		if crlf {
			nl = "\r\n"
		}
		var sb strings.Builder
		for i, l := range lines {
			if i > 0 {
				sb.WriteString(nl)
			}
			sb.WriteString(string(l))
		}
		if final && sb.Len() > 0 {
			sb.WriteString(nl)
		}
		return []byte(sb.String())
	}
}

// Save writes the file now and remembers that this version is saved.
func (b *Buffer) Save(path string) error {
	if path == "" {
		path = b.Path
	}
	mod, err := writeAtomic(path, b.Encode())
	if err != nil {
		return err
	}
	b.Path = path
	b.MarkSaved(b.version, mod)
	return nil
}

// MarkSaved records that version is on disk, written at time mod.
func (b *Buffer) MarkSaved(version int, mod time.Time) {
	b.savedVersion = version
	b.ModTime = mod
}

// ChangedOnDisk says whether another program changed the file since
// Symbolizer last read or wrote it.
func (b *Buffer) ChangedOnDisk() bool {
	if b.Path == "" || b.ModTime.IsZero() {
		return false
	}
	info, err := os.Stat(b.Path)
	return err == nil && !info.ModTime().Equal(b.ModTime)
}

// writeAtomic writes data safely: first to a hidden file next to it, then it
// swaps that in, so a crash or full disk never leaves half a file. It keeps
// the file's permissions and follows symlinks. Safe to run in the background.
func writeAtomic(path string, data []byte) (time.Time, error) {
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	tmp := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".sy-saving")
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return time.Time{}, err
	}
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp, mode)
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		os.Remove(tmp)
		return time.Time{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, nil
	}
	return info.ModTime(), nil
}

func (b *Buffer) Modified() bool { return b.version != b.savedVersion }
func (b *Buffer) Version() int   { return b.version }

func (b *Buffer) Text() string {
	var sb strings.Builder
	for i, l := range b.Lines {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(string(l))
	}
	return sb.String()
}

// ---- positions ----------------------------------------------------------------

func (b *Buffer) Clamp(p Pos) Pos {
	p.Row = max(0, min(p.Row, len(b.Lines)-1))
	p.Col = max(0, min(p.Col, len(b.Lines[p.Row])))
	return p
}

// Selection returns the selected range in order, or ok=false.
func (b *Buffer) Selection() (start, end Pos, ok bool) {
	if b.Anchor == nil || *b.Anchor == b.Cursor {
		return Pos{}, Pos{}, false
	}
	a, c := *b.Anchor, b.Cursor
	if c.Less(a) {
		return c, a, true
	}
	return a, c, true
}

func (b *Buffer) StartSelect() {
	if b.Anchor == nil {
		a := b.Cursor
		b.Anchor = &a
	}
}

func (b *Buffer) ClearSelect() { b.Anchor = nil }

func (b *Buffer) Get(start, end Pos) string {
	if start.Row == end.Row {
		return string(b.Lines[start.Row][start.Col:end.Col])
	}
	var sb strings.Builder
	sb.WriteString(string(b.Lines[start.Row][start.Col:]))
	for r := start.Row + 1; r < end.Row; r++ {
		sb.WriteByte('\n')
		sb.WriteString(string(b.Lines[r]))
	}
	sb.WriteByte('\n')
	sb.WriteString(string(b.Lines[end.Row][:end.Col]))
	return sb.String()
}

func (b *Buffer) SelectedText() string {
	s, e, ok := b.Selection()
	if !ok {
		return ""
	}
	return b.Get(s, e)
}

// endOf is where text ends if it is put at p.
func endOf(p Pos, text string) Pos {
	n := strings.Count(text, "\n")
	if n == 0 {
		return Pos{p.Row, p.Col + len([]rune(text))}
	}
	return Pos{p.Row + n, len([]rune(text[strings.LastIndexByte(text, '\n')+1:]))}
}

// ---- the one place text changes ---------------------------------------------

// replace swaps the text between start and end for text, with no undo record.
func (b *Buffer) replace(start, end Pos, text string) Pos {
	head := b.Lines[start.Row][:start.Col]
	tail := b.Lines[end.Row][end.Col:]
	parts := strings.Split(text, "\n")
	newLines := make([][]rune, len(parts))
	for i, p := range parts {
		newLines[i] = []rune(p)
	}
	last := len(newLines) - 1
	endCol := len(newLines[last])
	first := make([]rune, 0, len(head)+len(newLines[0]))
	first = append(append(first, head...), newLines[0]...)
	newLines[0] = first
	if last == 0 {
		endCol = len(first)
	}
	newLines[last] = append(append(make([]rune, 0, len(newLines[last])+len(tail)), newLines[last]...), tail...)

	removed := end.Row - start.Row + 1
	if removed == len(newLines) {
		copy(b.Lines[start.Row:], newLines)
	} else {
		rest := b.Lines[end.Row+1:]
		lines := make([][]rune, 0, len(b.Lines)-removed+len(newLines))
		lines = append(lines, b.Lines[:start.Row]...)
		lines = append(lines, newLines...)
		lines = append(lines, rest...)
		b.Lines = lines
	}
	if b.OnSplice != nil {
		b.OnSplice(start.Row, removed, len(newLines))
	}
	b.version++
	return Pos{start.Row + last, endCol}
}

// Change replaces start..end with text and records it for undo. Changes of
// the same kind made quickly one after another (typing) undo together.
func (b *Buffer) Change(start, end Pos, text, kind string) Pos {
	old := b.Get(start, end)
	now := time.Now()
	var st *step
	if top := b.top(); top != nil && kind != "" && top.kind == kind &&
		now.Sub(top.when) < 1500*time.Millisecond && top.cursorAfter == b.Cursor {
		st = top
	}
	if st == nil {
		st = &step{cursorBefore: b.Cursor, kind: kind}
		if b.Anchor != nil {
			a := *b.Anchor
			st.anchorBefore = &a
		}
		b.undo = append(b.undo, st)
		if len(b.undo) > 5000 {
			b.undo = b.undo[1:]
		}
	}
	st.edits = append(st.edits, edit{At: start, Old: old, New: text})
	st.when = now
	b.redo = nil
	end = b.replace(start, end, text)
	b.Cursor, b.Anchor, b.WantCol = end, nil, -1
	st.cursorAfter = end
	return end
}

// Group runs several changes as one undo step.
func (b *Buffer) Group(fn func()) {
	before := len(b.undo)
	fn()
	if len(b.undo) > before+1 {
		merged := b.undo[before]
		for _, s := range b.undo[before+1:] {
			merged.edits = append(merged.edits, s.edits...)
			merged.cursorAfter = s.cursorAfter
		}
		merged.kind = ""
		b.undo = b.undo[:before+1]
	} else if len(b.undo) == before+1 {
		b.undo[before].kind = ""
	}
}

// BreakUndo stops the next change joining the last undo step.
func (b *Buffer) BreakUndo() {
	if top := b.top(); top != nil {
		top.kind = ""
	}
}

func (b *Buffer) top() *step {
	if len(b.undo) == 0 {
		return nil
	}
	return b.undo[len(b.undo)-1]
}

func (b *Buffer) Undo() bool {
	st := b.top()
	if st == nil {
		return false
	}
	b.undo = b.undo[:len(b.undo)-1]
	for i := len(st.edits) - 1; i >= 0; i-- {
		e := st.edits[i]
		b.replace(e.At, endOf(e.At, e.New), e.Old)
	}
	b.redo = append(b.redo, st)
	b.Cursor, b.Anchor, b.WantCol = b.Clamp(st.cursorBefore), st.anchorBefore, -1
	st.kind = ""
	return true
}

// TakeBack undoes the last change and forgets it, as if it never happened.
func (b *Buffer) TakeBack() {
	if b.Undo() {
		b.redo = b.redo[:len(b.redo)-1]
	}
}

func (b *Buffer) Redo() bool {
	if len(b.redo) == 0 {
		return false
	}
	st := b.redo[len(b.redo)-1]
	b.redo = b.redo[:len(b.redo)-1]
	for _, e := range st.edits {
		b.replace(e.At, endOf(e.At, e.Old), e.New)
	}
	b.undo = append(b.undo, st)
	b.Cursor, b.Anchor, b.WantCol = b.Clamp(st.cursorAfter), nil, -1
	return true
}

// ---- common edits --------------------------------------------------------------

// Insert types or pastes text at the cursor, replacing any selection.
func (b *Buffer) Insert(text, kind string) {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	if s, e, ok := b.Selection(); ok {
		b.Change(s, e, text, "")
		return
	}
	b.Change(b.Cursor, b.Cursor, text, kind)
}

func (b *Buffer) DeleteSelection() bool {
	s, e, ok := b.Selection()
	if !ok {
		return false
	}
	b.Change(s, e, "", "")
	return true
}

func (b *Buffer) Backspace() {
	if b.DeleteSelection() {
		return
	}
	c := b.Cursor
	if c.Row == 0 && c.Col == 0 {
		return
	}
	if c.Col == 0 {
		b.Change(Pos{c.Row - 1, len(b.Lines[c.Row-1])}, c, "", "delete")
		return
	}
	n := 1
	head := b.Lines[c.Row][:c.Col]
	if b.Indent != "\t" && len(head) >= 4 && strings.TrimSpace(string(head)) == "" {
		n = (len(head)-1)%4 + 1 // back to the last indent step
	}
	b.Change(Pos{c.Row, c.Col - n}, c, "", "delete")
}

func (b *Buffer) DeleteForward() {
	if b.DeleteSelection() {
		return
	}
	c := b.Cursor
	if c.Col < len(b.Lines[c.Row]) {
		b.Change(c, Pos{c.Row, c.Col + 1}, "", "deletefwd")
	} else if c.Row < len(b.Lines)-1 {
		b.Change(c, Pos{c.Row + 1, 0}, "", "deletefwd")
	}
}

// ---- word and letter steps ------------------------------------------------------

func isWord(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }

func (b *Buffer) LetterLeft(p Pos) Pos {
	if p.Col > 0 {
		return Pos{p.Row, p.Col - 1}
	}
	if p.Row > 0 {
		return Pos{p.Row - 1, len(b.Lines[p.Row-1])}
	}
	return p
}

func (b *Buffer) LetterRight(p Pos) Pos {
	if p.Col < len(b.Lines[p.Row]) {
		return Pos{p.Row, p.Col + 1}
	}
	if p.Row < len(b.Lines)-1 {
		return Pos{p.Row + 1, 0}
	}
	return p
}

// WordAt is the word around p, for double-click.
func (b *Buffer) WordAt(p Pos) (Pos, Pos) {
	l := b.Lines[p.Row]
	s, e := p.Col, p.Col
	for s > 0 && isWord(l[s-1]) {
		s--
	}
	for e < len(l) && isWord(l[e]) {
		e++
	}
	return Pos{p.Row, s}, Pos{p.Row, e}
}
