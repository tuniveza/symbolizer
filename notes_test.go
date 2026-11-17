package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A saved .txt is a note; 16 fill a notepad and the 17th starts a new one;
// sessions are not notes; a sticker is kept.
func TestNotesFillNotepads(t *testing.T) {
	m := newTest(t, "text stays")
	notesCache = nil
	dir := t.TempDir()
	if isNoteFile(filepath.Join(sessionDir(), "session-2026-11-17-120000.txt")) {
		t.Fatal("a session draft is not a note")
	}
	for i := 1; i <= 17; i++ {
		p := filepath.Join(dir, "n"+itoa(i)+".txt")
		os.WriteFile(p, []byte("x"), 0o644)
		m.noteTouched(p)
		time.Sleep(time.Millisecond)
	}
	nf := notesLoad()
	if len(nf.inPad(1)) != 16 || len(nf.inPad(2)) != 1 || nf.lastPad() != 2 {
		t.Fatalf("pads: %d + %d, last %d", len(nf.inPad(1)), len(nf.inPad(2)), nf.lastPad())
	}
	m.noteTouched(filepath.Join(dir, "n1.txt")) // saved again: still one note
	if len(nf.Notes) != 17 {
		t.Fatalf("notes: %d", len(nf.Notes))
	}
	for _, n := range []int{1, 2} {
		if nf.pad(n).Room == 0 {
			t.Fatalf("notepad %d should be on a board", n)
		}
	}
	m.buf.Path = filepath.Join(dir, "n3.txt")
	m.stickerEntered(" ideas ")
	notesCache = nil
	if r := notesLoad().note(filepath.Join(dir, "n3.txt")); r == nil || r.Sticker != "ideas" {
		t.Fatalf("the sticker: %+v", r)
	}
}

// A note saved by the background autosave is a note too; a sticker given to
// a note never saved before puts its new notepad on the board.
func TestNotesFromAutosaveAndSticker(t *testing.T) {
	m := newTest(t, "text stays")
	dir := t.TempDir()
	p := filepath.Join(dir, "auto.txt")
	os.WriteFile(p, []byte("x"), 0o644)
	m.finishSave(saveDone{path: p, wrote: true})
	if notesLoad().note(p) == nil {
		t.Fatal("an autosaved .txt should be a note")
	}
	notesCache = nil
	os.Remove(notesPath())
	m.buf.Path = filepath.Join(dir, "stick.txt")
	os.WriteFile(m.buf.Path, []byte("y"), 0o644)
	m.askSticker()
	m.stickerEntered("idea")
	nf := notesLoad()
	if r := nf.note(m.buf.Path); r == nil || r.Sticker != "idea" || nf.pad(r.Pad) == nil || nf.pad(r.Pad).Room == 0 {
		t.Fatalf("the sticker's note and its notepad on a board: %+v %+v", r, nf.Pads)
	}
}
