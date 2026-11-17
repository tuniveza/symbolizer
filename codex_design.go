package main

// ---- a codex on the Design board ----------------------------------------------------------
//
// A codex structure (kCodex) shows its codex's cover, as the codex is now.
// Its From is "codex:<id>"; its Header keeps the title, for containers'
// rows. It goes into drawers, racks, boards and boxes like anything else.
//
//	Alt+N, then K   a new codex, here on the board
//	Enter           open it in Codex mode (Esc there comes back)
//	E               open it in Codex mode, to edit
//	double-click    open it
//
// In Codex mode's library, P puts the chosen codex on the board.
//
// A codex made in Codex mode (not on a board) gets its own room on Design
// when it is kept (the user asked, 17 Nov 2026): the room holds the codex,
// and a lexicon door leads to it from the room last used on Design.
// Lexicon doors are only ever on Design boards; Codex mode edits them by
// editing the codex: its title names the door and the room, its cover
// colour is the door's leather.

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const kCodex = "codex"

const codexBoardW, codexBoardH = 30, 13

func init() {
	kindIcons[kCodex] = "▥"
	kindNames[kCodex] = "codex"
	for _, k := range []string{"k", "K", "shift+k"} {
		placeKeys[k] = kCodex
	}
}

// codexIDOf is the codex structure o shows ("" none).
func codexIDOf(o *dOct) string {
	if o == nil || o.Kind != kCodex {
		return ""
	}
	return strings.TrimPrefix(o.From, "codex:")
}

// codexLayers draws codex structure o: its codex's cover.
func (d *designState) codexLayers(o *dOct) []*lipgloss.Layer {
	x, y, w, h := d.rect(o)
	cf, err := codexPeek(codexIDOf(o))
	if err != nil {
		cf = &codexFile{Title: itemTitle(o) + " (not found)", Color: "#555555"}
	}
	cv := codexCover(cf, w, h, 0, d.sel == o.ID)
	l := lipgloss.NewLayer("").X(x).Y(y)
	d.preCells(l, cv)
	return []*lipgloss.Layer{l}
}

// codexNewOnBoard makes a new codex for structure o, just placed.
func (m *Model) codexNewOnBoard(o *dOct) {
	id, cf, err := createCodex("")
	if err != nil {
		m.sayErr("Could not make the codex: " + err.Error())
		return
	}
	o.From = "codex:" + id
	o.Header = codexName(cf.Title)
	o.Color = codexOr(cf.Color, codexThemeCover())
	m.say("New codex on the board. Enter opens it · E edits it (its title first).")
}

// codexFromDesign opens codex structure o in Codex mode (edit: to edit it).
func (m *Model) codexFromDesign(o *dOct, edit bool) tea.Cmd {
	id := codexIDOf(o)
	if id == "" {
		return nil
	}
	if _, err := codexPeek(id); err != nil {
		m.sayErr("Its codex is not there any more (" + id + ").")
		return nil
	}
	d := m.ds()
	d.save()
	cmd := m.setMode(codexMode)
	c := m.cx()
	if !m.codexOpen(id, edit) {
		return cmd
	}
	c.fromDesign, c.designID = true, o.ID
	if edit && strings.TrimSpace(c.cf.Title) == "" {
		m.codexStartWrite(codexWriteTitle)
		m.say("Type the codex's title, then Enter (Tab: a subtitle) · Esc twice: back to the board.")
	}
	return tea.Batch(cmd, m.codexTickCmd())
}

// codexOnBoard is whether any Design board (in any room, on any wall)
// shows codex id: the board open now, and the boards' files.
func (m *Model) codexOnBoard(id string) bool {
	if id == "" {
		return false
	}
	if m.design != nil {
		for _, o := range m.design.octs {
			if codexIDOf(o) == id {
				return true
			}
		}
	}
	want := []byte(`"codex:` + id + `"`)
	found := false
	filepath.WalkDir(filepath.Dir(designPath()), func(p string, de fs.DirEntry, err error) error {
		if err != nil || de.IsDir() || !strings.HasSuffix(p, ".json") {
			return nil
		}
		if b, err := os.ReadFile(p); err == nil && bytes.Contains(b, want) {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// codexToBoard puts codex id on the Design board, in the middle of the view.
func (m *Model) codexToBoard(id string) tea.Cmd {
	cf, err := codexPeek(id)
	if err != nil {
		m.sayErr("Could not read that codex: " + err.Error())
		return nil
	}
	cmd := m.setMode(designMode)
	d := m.ds()
	wx, wy := d.world(m.width/2, m.boardH()/2)
	o := d.newKind(kCodex, int(wx), int(wy))
	o.From, o.Header, o.Color = "codex:"+id, codexName(cf.Title), codexOr(cf.Color, codexThemeCover())
	d.clearSpot(o)
	d.sel, d.editing = o.ID, partNone
	m.centreOn(o)
	d.save()
	m.say("“" + codexName(cf.Title) + "” is on the board. Enter opens it.")
	return cmd
}

// codexHome makes or updates codex id's own room, on closing it: a new one
// for a codex made in Codex mode (cf.Room -1); for one that has a room, the
// room's name and its lexicon doors follow the codex's title and colour.
func (m *Model) codexHome(id string, cf *codexFile) {
	if cf.Room < 0 && !m.codexOnBoard(id) {
		if r := m.codexMakeRoom(id, cf); r > 0 {
			cf.Room = r
			saveCodex(id, cf)
			m.cx().saved = ""
		}
		return
	}
	if cf.Room > 0 {
		m.codexSyncRoom(id, cf)
	}
}

// lexiconLook dresses door o as codex cf's lexicon door.
func lexiconLook(o *dOct, id string, cf *codexFile) {
	o.Lexicon = id
	o.Header = codexName(cf.Title)
	o.Color = codexOr(cf.Color, codexThemeCover())
	o.Footer = "▥ Lexicon"
	o.Body = "A lexicon door to “" + codexName(cf.Title) + "”.\nEnter or Space: go in."
}

// codexMakeRoom makes codex id's room: the codex in its middle, a door
// back, and a lexicon door to it in the room last used on Design. It gives
// the room's number (0: it could not be made).
func (m *Model) codexMakeRoom(id string, cf *codexFile) int {
	h := m.hs()
	here := h.Current
	hr := h.room(here)
	nr := houseRoom{ID: h.nextID(), Floor: hr.Floor, Name: codexName(cf.Title)}
	h.Rooms = append(h.Rooms, nr)
	if err := h.save(); err != nil {
		h.Rooms = h.Rooms[:len(h.Rooms)-1]
		m.sayErr("Its room could not be made: " + err.Error())
		return 0
	}
	hr = h.room(here)

	// The lexicon door, where you were last on Design, on the wall you faced.
	d := m.boardOf(here, ((hr.Wall%hr.walls())+hr.walls())%hr.walls())
	wx, wy := d.world(m.width/2, m.boardH()/2)
	o := d.newKind(kDoor, int(wx), int(wy))
	if o != nil {
		ox, oy, ow, oh := o.X, o.Y, o.W, o.H
		h.makeDoor(o, kDoor, nr.ID, here)
		lexiconLook(o, id, cf)
		o.X, o.Y = ox+ow/2-o.W/2, oy+oh/2-o.H/2
		d.clearSpot(o)
		d.dirty = true
		d.save()
		d.flushSave()
	}

	// The room: the codex in the middle, a door back beside it.
	nd := loadDesign(roomPath(nr.ID))
	nd.walls, nd.floor = d.walls, d.floor
	bx, by := m.width/2, m.boardH()/2
	co := nd.newKind(kCodex, bx, by)
	co.From, co.Header, co.Color = "codex:"+id, codexName(cf.Title), codexOr(cf.Color, codexThemeCover())
	back := nd.newKind(kDoor, bx-codexBoardW-8, by)
	h.makeDoor(back, kDoor, here, nr.ID)
	back.X, back.Y = bx-codexBoardW/2-back.W-6, by-back.H/2
	nd.clearSpot(back)
	nd.sel = co.ID
	nd.dirty = true
	nd.save()
	nd.flushSave()
	if m.rooms == nil {
		m.rooms = map[int]*designState{}
	}
	m.rooms[boardKey(nr.ID, 0)] = nd
	return nr.ID
}

// codexSyncRoom: codex id's room is named after it, and its lexicon doors
// (on any wall of any room) and its covers there show its title and colour.
func (m *Model) codexSyncRoom(id string, cf *codexFile) {
	h := m.hs()
	if r := h.room(cf.Room); r != nil && r.Name != codexName(cf.Title) {
		r.Name = codexName(cf.Title)
		h.save()
	}
	for _, r := range h.Rooms {
		for w := 0; w < r.walls(); w++ {
			b := m.boardOf(r.ID, w)
			changed := false
			for _, o := range b.octs {
				switch {
				case o.Lexicon == id:
					if o.Header != codexName(cf.Title) || o.Color != codexOr(cf.Color, codexThemeCover()) {
						lexiconLook(o, id, cf)
						changed = true
					}
				case codexIDOf(o) == id && r.ID == cf.Room:
					if o.Header != codexName(cf.Title) {
						o.Header, changed = codexName(cf.Title), true
					}
				}
			}
			if changed {
				b.dirty = true
				b.save()
				b.flushSave()
			}
		}
	}
}
