package main

// ---- notes and notepads (the user asked, 17 Nov 2026) -----------------------------------------
//
// Every .txt file saved under a name in Text mode is a note. The file stays
// a plain .txt on disk (other programs still open it); the note points at
// it. Notes live in notepads on the Design board, 16 to a notepad, in the
// order they were first saved; when one is full the next note starts a new
// notepad, placed beside the last one. Each notepad looks different
// (notepadStyle). In Text mode Alt+F gives the open note a sticker: a word
// or two to find it by or to group notes with.
//
// The notes and notepads are kept in symbolizer/notes.json; a notepad on a
// board (kNotepad) shows notepad number N (From "notepad:N").

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
)

const notesPerPad = 16

// noteRec is one note: a saved .txt file.
type noteRec struct {
	Path    string    `json:"path"`
	Added   time.Time `json:"added"`   // first saved: its place in the notepads
	Saved   time.Time `json:"saved"`   // last saved
	Sticker string    `json:"sticker,omitempty"`
	Pad     int       `json:"pad"` // the notepad it is in, from 1
}

// padRec is one notepad: where it was put on Design (0: not on a board yet).
type padRec struct {
	N    int `json:"n"`
	Room int `json:"room,omitempty"`
}

type notesFile struct {
	Notes []*noteRec `json:"notes"`
	Pads  []*padRec  `json:"pads"`
}

var (
	notesMu    sync.Mutex
	notesCache *notesFile
	notesFrom  string // the file notesCache was read from (tests change the data folder)
)

func notesPath() string { return filepath.Join(filepath.Dir(sessionDir()), "notes.json") }

// notesLoad reads the notes once; later calls give the same.
func notesLoad() *notesFile {
	notesMu.Lock()
	defer notesMu.Unlock()
	if notesCache != nil && notesFrom == notesPath() {
		return notesCache
	}
	nf := &notesFile{}
	if b, err := os.ReadFile(notesPath()); err == nil {
		_ = json.Unmarshal(b, nf)
	}
	notesCache, notesFrom = nf, notesPath()
	return nf
}

func (nf *notesFile) save() error {
	b, err := json.MarshalIndent(nf, "", " ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(notesPath()), 0o755); err != nil {
		return err
	}
	tmp := notesPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, notesPath())
}

// isNoteFile: a .txt file with a name of its own (not a session draft).
func isNoteFile(path string) bool {
	return path != "" && strings.EqualFold(filepath.Ext(path), ".txt") && !isSession(path)
}

// note is the note for path (nil: none).
func (nf *notesFile) note(path string) *noteRec {
	path = filepath.Clean(path)
	for _, n := range nf.Notes {
		if n.Path == path {
			return n
		}
	}
	return nil
}

func (nf *notesFile) pad(n int) *padRec {
	for _, p := range nf.Pads {
		if p.N == n {
			return p
		}
	}
	return nil
}

// lastPad is the highest notepad number (0: none yet).
func (nf *notesFile) lastPad() int {
	n := 0
	for _, p := range nf.Pads {
		n = max(n, p.N)
	}
	return n
}

// inPad are notepad n's notes, oldest first.
func (nf *notesFile) inPad(n int) []*noteRec {
	var out []*noteRec
	for _, r := range nf.Notes {
		if r.Pad == n {
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Added.Before(out[j].Added) })
	return out
}

// noteSaved notes that path was saved: a new note goes into the last
// notepad, or a new one when that has 16. It gives the note, and the
// notepad's number if that notepad is new (0 otherwise).
func (nf *notesFile) noteSaved(path string, now time.Time) (*noteRec, int) {
	path = filepath.Clean(path)
	if r := nf.note(path); r != nil {
		r.Saved = now
		return r, 0
	}
	pad, fresh := nf.lastPad(), 0
	if pad == 0 || len(nf.inPad(pad)) >= notesPerPad {
		pad++
		nf.Pads = append(nf.Pads, &padRec{N: pad})
		fresh = pad
	}
	r := &noteRec{Path: path, Added: now, Saved: now, Pad: pad}
	nf.Notes = append(nf.Notes, r)
	return r, fresh
}

// noteTouched is called after Text mode saves path: it keeps the notes up
// to date and puts a new notepad on the Design board.
func (m *Model) noteTouched(path string) {
	if !isNoteFile(path) {
		return
	}
	nf := notesLoad()
	_, fresh := nf.noteSaved(path, time.Now())
	if err := nf.save(); err != nil {
		return
	}
	if fresh > 0 {
		m.placeNotepad(fresh)
	}
}

// askSticker (Alt+F in Text mode) asks for the open note's sticker.
func (m *Model) askSticker() tea.Cmd {
	b := m.buf
	if !isNoteFile(b.Path) {
		m.say("Stickers are for notes: a .txt file saved with a name (Ctrl+S names it).")
		return nil
	}
	m.saveNow()
	nf := notesLoad()
	r := nf.note(b.Path)
	if r == nil {
		r, _ = nf.noteSaved(b.Path, time.Now())
	}
	cmd := m.ask(pNoteSticker, "Sticker for “"+filepath.Base(b.Path)+"” (empty: none): ", r.Sticker)
	return cmd
}

// stickerEntered gives the open note sticker s.
func (m *Model) stickerEntered(s string) tea.Cmd {
	s = strings.TrimSpace(s)
	nf := notesLoad()
	path := m.buf.Path
	r := nf.note(path)
	fresh := 0
	if r == nil {
		r, fresh = nf.noteSaved(path, time.Now())
	}
	r.Sticker = s
	if err := nf.save(); err != nil {
		m.sayErr("The sticker could not be kept: " + err.Error())
		return nil
	}
	if fresh > 0 {
		m.placeNotepad(fresh)
	}
	if s == "" {
		m.say("No sticker on " + filepath.Base(path) + " now.")
	} else {
		m.say("Sticker “" + s + "” on " + filepath.Base(path) + ". It shows in notepad " + itoa(r.Pad) + " on the Design board.")
	}
	return nil
}

// placeNotepad puts notepad n on the Design board: beside notepad n-1 if
// that is on a board, else in the room last used, on the wall faced.
func (m *Model) placeNotepad(n int) {
	nf := notesLoad()
	pr := nf.pad(n)
	if pr == nil || pr.Room != 0 {
		return
	}
	h := m.hs()
	room := h.Current
	var near *dOct
	if prev := nf.pad(n - 1); prev != nil && prev.Room != 0 && h.room(prev.Room) != nil {
		room = prev.Room
	}
	r := h.room(room)
	var d *designState
	for w := 0; w < r.walls() && near == nil; w++ { // the wall with the notepad before
		b := m.boardOf(room, w)
		for _, o := range b.octs {
			if o.Kind == kNotepad && o.From == notepadLink(n-1) {
				d, near = b, o
			}
		}
	}
	if d == nil {
		d = m.boardOf(room, ((r.Wall%r.walls())+r.walls())%r.walls())
	}
	wx, wy := d.world(m.width/2, m.boardH()/2)
	x, y := int(wx), int(wy)
	if near != nil {
		x, y = near.X+near.W+notepadW/2+3, near.Y+near.H/2
	}
	o := d.newKind(kNotepad, x, y)
	o.From = notepadLink(n)
	o.Header = "Notepad " + itoa(n)
	d.clearSpot(o)
	d.dirty = true
	d.save()
	d.flushSave()
	pr.Room = room
	nf.save()
}

func notepadLink(n int) string { return "notepad:" + itoa(n) }
