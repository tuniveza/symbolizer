package main

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// A codex made in Codex mode gets its own room on Design when kept, with a
// lexicon door to it in the room last used; renaming the codex renames the
// room and the door. One made on a Design board gets no room.
func TestCodexOwnRoom(t *testing.T) {
	m := codexTest(t)
	m.Update(tea.WindowSizeMsg{Width: 180, Height: 50})
	press(m, "alt+n")
	typeStr(m, "Star Atlas")
	press(m, "enter")
	c := m.cx()
	id := c.id
	press(m, "esc") // edit mode: Esc closes
	if c.cf != nil {
		t.Fatal("Esc should close the codex")
	}
	cf, err := loadCodex(id)
	if err != nil || cf.Room <= 0 {
		t.Fatalf("the codex should know its room: %+v %v", cf, err)
	}
	h := m.hs()
	r := h.room(cf.Room)
	if r == nil || r.Name != "Star Atlas" {
		t.Fatalf("its room: %+v", r)
	}
	findDoor := func() *dOct {
		hr := h.room(houseFirstRoomID)
		for _, o := range m.boardOf(houseFirstRoomID, hr.at()).octs {
			if o.Lexicon == id {
				return o
			}
		}
		return nil
	}
	door := findDoor()
	if door == nil || door.Kind != kDoor || doorTo(door) != cf.Room || door.Header != "Star Atlas" {
		t.Fatalf("a lexicon door to its room: %+v", door)
	}
	var cover, back *dOct
	for _, o := range m.boardOf(cf.Room, 0).octs {
		if codexIDOf(o) == id {
			cover = o
		}
		if doorTo(o) == houseFirstRoomID {
			back = o
		}
	}
	if cover == nil || back == nil {
		t.Fatalf("the room holds the codex and a door back: %v %v", cover, back)
	}
	// Renamed in Codex mode: the room and the door follow.
	m.codexOpen(id, true)
	c.cf.Title = "Sky Atlas"
	press(m, "esc")
	if h.room(cf.Room).Name != "Sky Atlas" || findDoor().Header != "Sky Atlas" {
		t.Fatalf("renamed: room %q door %q", h.room(cf.Room).Name, findDoor().Header)
	}
	// Closing again makes no second room.
	n := len(h.Rooms)
	m.codexOpen(id, false)
	press(m, "esc")
	if len(h.Rooms) != n {
		t.Fatal("a second room was made")
	}

	// Made on a Design board: no room of its own.
	m.setMode(designMode)
	d := m.ds()
	o := d.newKind(kCodex, 10, 10)
	m.codexNewOnBoard(o)
	m.codexFromDesign(o, true)
	typeStr(m, "Board Book")
	press(m, "enter")
	press(m, "esc")
	bcf, _ := loadCodex(codexIDOf(o))
	if bcf.Room != 0 || len(h.Rooms) != n {
		t.Fatalf("a codex made on the board stays there: room %d, rooms %d→%d", bcf.Room, n, len(h.Rooms))
	}
}
