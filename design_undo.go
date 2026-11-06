package main

import "fmt"

// ---- undo and redo ----------------------------------------------------------------
//
// Every finished change to the board is one step: a key, a click, a drag, a
// deletion, an ink stroke. Writing in one part of a structure is one step
// until you stop writing in it. Looking around (moving the board, zooming,
// scrolling a page, pulling a drawer out) is not a step. Ctrl+Z undoes a
// step, Ctrl+Y (or Ctrl+Shift+Z) does it again; ↶ and ↷ on the toolbar too.
// The steps last until sy closes.

// undoKept is how many steps can be undone.
const undoKept = 100

// boardState is what a step changes: the structures (with what their
// drawers hold), the links and the ink. Words are shared, never copied:
// Go strings do not change.
type boardState struct {
	octs   []*dOct
	links  []dLink
	marks  map[[2]int]dMark
	nextID int
}

// state copies the board as it is now.
func (d *designState) state() *boardState {
	marks := make(map[[2]int]dMark, len(d.marks))
	for k, v := range d.marks {
		marks[k] = v
	}
	return &boardState{octs: copyOcts(d.octs), links: append([]dLink(nil), d.links...), marks: marks, nextID: d.nextID}
}

// sameAs is whether the board is as s has it (looking around aside).
func (d *designState) sameAs(s *boardState) bool {
	if len(d.links) != len(s.links) || len(d.marks) != len(s.marks) || !sameBoard(d.octs, s.octs) {
		return false
	}
	for i := range d.links {
		if d.links[i] != s.links[i] {
			return false
		}
	}
	for k, v := range d.marks {
		if w, ok := s.marks[k]; !ok || w != v {
			return false
		}
	}
	return true
}

// sameBoard compares the structures on the board. Which is in front is
// not a change: clicking one brings it forward.
func sameBoard(a, b []*dOct) bool {
	if len(a) != len(b) {
		return false
	}
	byID := make(map[int]*dOct, len(b))
	for _, o := range b {
		if o != nil {
			byID[o.ID] = o
		}
	}
	for _, o := range a {
		if o == nil {
			continue
		}
		p, ok := byID[o.ID]
		if !ok || !sameOct(o, p) || !sameOcts(o.Items, p.Items) {
			return false
		}
	}
	return true
}

func sameOcts(a, b []*dOct) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if (a[i] == nil) != (b[i] == nil) {
			return false
		}
		if a[i] != nil && (!sameOct(a[i], b[i]) || !sameOcts(a[i].Items, b[i].Items)) {
			return false
		}
	}
	return true
}

// sameOct compares what a step can change. Scroll, DrawerOpen and OpenStack
// are looking around, not changes.
func sameOct(a, b *dOct) bool {
	return a.ID == b.ID && a.X == b.X && a.Y == b.Y && a.W == b.W && a.H == b.H &&
		a.Header == b.Header && a.Body == b.Body && a.Footer == b.Footer && a.Color == b.Color &&
		a.Kind == b.Kind && a.Layout == b.Layout && a.Image == b.Image && a.From == b.From
}

// recordUndo makes a step of what changed since the last one. It runs each
// time the board is saved, which is after every change.
func (d *designState) recordUndo() {
	if d.undoBase == nil {
		d.undoBase = d.state()
		return
	}
	if d.drag != dragNone {
		return // a drag is one step: it is made when the button is let go
	}
	if d.sameAs(d.undoBase) {
		return
	}
	writing := d.editing != partNone && d.sel >= 0
	if !(writing && d.undoWriting == d.sel) { // more words in the same part: the same step
		d.undo = append(d.undo, d.undoBase)
		if len(d.undo) > undoKept {
			d.undo = d.undo[len(d.undo)-undoKept:]
		}
		d.redo = nil
	}
	d.undoBase = d.state()
	d.undoWriting = -1
	if writing {
		d.undoWriting = d.sel
	}
}

// restore puts the board as s has it. s stays as it was, for redo.
func (d *designState) restore(s *boardState) {
	d.octs = copyOcts(s.octs)
	d.links = append([]dLink(nil), s.links...)
	d.marks = make(map[[2]int]dMark, len(s.marks))
	for k, v := range s.marks {
		d.marks[k] = v
	}
	d.nextID = max(d.nextID, s.nextID)
	if d.byID(d.sel) == nil {
		d.sel = -1
	}
	if d.byID(d.linkFrom) == nil {
		d.linkFrom = -1
	}
	d.editing, d.selLink, d.undoWriting = partNone, -1, -1
	d.undoBase = s
	d.dirty = true
}

// designUndo takes back the last step.
func (m *Model) designUndo() {
	d := m.ds()
	d.recordUndo() // what was just written becomes a step first
	if len(d.undo) == 0 {
		m.say("Nothing to undo.")
		return
	}
	s := d.undo[len(d.undo)-1]
	d.undo = d.undo[:len(d.undo)-1]
	d.redo = append(d.redo, d.undoBase)
	d.restore(s)
	m.say(fmt.Sprintf("Undone. %d more to undo · Ctrl+Y does it again.", len(d.undo)))
}

// designRedo does again the last step undone.
func (m *Model) designRedo() {
	d := m.ds()
	d.recordUndo()
	if len(d.redo) == 0 {
		m.say("Nothing to redo.")
		return
	}
	s := d.redo[len(d.redo)-1]
	d.redo = d.redo[:len(d.redo)-1]
	d.undo = append(d.undo, d.undoBase)
	d.restore(s)
	m.say(fmt.Sprintf("Done again. %d more to redo.", len(d.redo)))
}
