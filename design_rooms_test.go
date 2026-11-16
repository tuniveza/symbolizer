package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// roomKey presses AltGr+N (” on a UK keyboard), then key: r a room, w a wall, l a level.
func roomKey(m *Model, key string) {
	m.Update(tea.KeyPressMsg(tea.Key{Code: '”', Text: "”"}))
	press(m, key)
}

// chosen is the chosen structure on the board you are in.
func chosen(m *Model) *dOct { d := m.ds(); return d.byID(d.sel) }

func TestRoomsNewRoomDoorAndBack(t *testing.T) {
	m := designTest(t)
	roomKey(m, "r")
	h := m.hs()
	if len(h.Rooms) != 2 || h.Current != 1 {
		t.Fatalf("rooms %+v, in %d: want 2 rooms, still in room 1", h.Rooms, h.Current)
	}
	door := chosen(m)
	if door == nil || door.Kind != kDoor || doorTo(door) != 2 || door.Header != "Room 2" {
		t.Fatalf("the new door: %+v", door)
	}
	if _, err := os.Stat(roomPath(2)); err != nil {
		t.Fatalf("room 2's board is not on disk: %v", err)
	}

	press(m, "enter")
	if m.hs().Current != 2 {
		t.Fatalf("Enter on the door: in room %d, want 2", m.hs().Current)
	}
	back := chosen(m)
	if back == nil || doorTo(back) != 1 {
		t.Fatalf("in room 2 the door back is not chosen: %+v", back)
	}
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 30}) // (narrower, the toolbar has no room for it: arriving says it)
	if bar := m.designToolbar(); !strings.Contains(ansi.Strip(bar), "Room 2 · wall 1/4") {
		t.Fatalf("the toolbar does not say where you are: %q", ansi.Strip(bar))
	}

	// Something placed in room 2 stays in room 2.
	placeKey(m, "p")
	typeKeys(m, "In room two")
	press(m, "esc")
	if len(m.ds().octs) != 2 {
		t.Fatalf("room 2 has %d structures, want 2 (the door back and the page)", len(m.ds().octs))
	}
	m.ds().sel = back.ID
	press(m, "enter")
	if m.hs().Current != 1 || len(m.ds().octs) != 1 {
		t.Fatalf("back: in room %d with %d structures, want room 1 with just its door", m.hs().Current, len(m.ds().octs))
	}

	// sy started again: the house, and each room's things, are as they were.
	m.ds().sel = door.ID
	press(m, "enter")
	m.ds().flushSave()
	m2, _ := NewModel("", 0)
	m2.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m2.setMode(designMode)
	if m2.hs().Current != 2 || len(m2.hs().Rooms) != 2 {
		t.Fatalf("after a restart: in room %d of %d, want room 2 of 2", m2.hs().Current, len(m2.hs().Rooms))
	}
	found := false
	for _, o := range m2.ds().octs {
		if strings.Contains(o.Header, "In room two") {
			found = true
		}
	}
	if !found {
		t.Fatal("after a restart room 2's page is gone")
	}
}

func TestRoomsNewFloorStairs(t *testing.T) {
	m := designTest(t)
	roomKey(m, "l")
	st := chosen(m)
	if st == nil || st.Kind != kStairs || !strings.Contains(st.Body, "up to level 2") {
		t.Fatalf("the new stairs: %+v", st)
	}
	press(m, "enter")
	h := m.hs()
	if h.room(h.Current).Floor != 2 {
		t.Fatalf("up the stairs: on floor %d, want 2", h.room(h.Current).Floor)
	}
	if down := chosen(m); down == nil || down.Kind != kStairs || !strings.Contains(down.Body, "down to level 1") {
		t.Fatalf("the stairs down: %+v", down)
	}
	// A room on floor 2 stays on floor 2; a floor more goes to floor 3.
	roomKey(m, "r")
	if r := h.room(doorTo(chosen(m))); r == nil || r.Floor != 2 {
		t.Fatalf("a room made on floor 2: %+v", r)
	}
	roomKey(m, "l")
	if r := h.room(doorTo(chosen(m))); r == nil || r.Floor != 3 {
		t.Fatalf("a floor made from floor 2: %+v", r)
	}
}

func TestRoomsDoorTitleNamesRoom(t *testing.T) {
	m := designTest(t)
	roomKey(m, "r")
	press(m, "tab") // write its title
	for range len("Room 2") {
		press(m, "backspace")
	}
	typeKeys(m, "Kitchen")
	press(m, "esc")
	press(m, "enter")
	h := m.hs()
	if h.Current != 2 || h.room(2).Name != "Kitchen" {
		t.Fatalf("in room %d named %q, want room 2 named Kitchen", h.Current, h.room(2).Name)
	}
}

func TestRoomsKeys(t *testing.T) {
	m := designTest(t)
	placeKey(m, "r") // Alt+N, then R is still a rack
	if o := chosen(m); o == nil || o.Kind != kRack || len(m.hs().Rooms) != 1 {
		t.Fatalf("Alt+N then R: %+v, %d rooms; want a rack and still 1 room", o, len(m.hs().Rooms))
	}
	m.Update(tea.KeyPressMsg(tea.Key{Code: '”', Text: "”"}))
	m.Update(tea.KeyPressMsg(tea.Key{Code: '¶', Text: "¶"})) // AltGr still held: AltGr+R
	if len(m.hs().Rooms) != 2 {
		t.Fatalf("AltGr+N then AltGr+R: %d rooms, want 2", len(m.hs().Rooms))
	}
	m.Update(tea.KeyPressMsg(tea.Key{Code: '”', Text: "”"}))
	m.Update(tea.KeyPressMsg(tea.Key{Code: 'l'})) // then L: a level
	if h := m.hs(); len(h.Rooms) != 3 || h.Rooms[2].Floor != 2 {
		t.Fatalf("AltGr+N then L: %+v, want a third room on level 2", h.Rooms)
	}
	roomKey(m, "x") // anything else: nothing
	if len(m.hs().Rooms) != 3 {
		t.Fatal("AltGr+N then X made a room")
	}
}

// doorCells is the door's drawing: its cells' letters and background colours.
func doorCells(t *testing.T, m *Model, o *dOct) (string, map[string]bool) {
	t.Helper()
	d := m.ds()
	ls := d.doorLayers(o)
	c := d.preMade[ls[0]]
	if c == nil {
		t.Fatal("the door has no drawing")
	}
	var sb strings.Builder
	cols := map[string]bool{}
	for _, row := range c.Lines {
		for _, cell := range row {
			sb.WriteString(cell.Content)
			if cell.Style.Fg != nil {
				r, g, b, _ := cell.Style.Fg.RGBA()
				cols[fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)] = true
			}
		}
		sb.WriteByte('\n')
	}
	return sb.String(), cols
}

func TestRoomsDoorWindowShowsRoom(t *testing.T) {
	m := designTest(t)
	roomKey(m, "r")
	door := chosen(m)
	before, _ := doorCells(t, m, door)
	for _, want := range []string{"Room 2", "▀", "●", "Level 1"} {
		if !strings.Contains(before, want) {
			t.Fatalf("the door has no %q:\n%s", want, before)
		}
	}
	// Something bright placed in room 2 shows through the window, in its colour.
	press(m, "enter")
	placeKey(m, "n")
	press(m, "esc")
	note := chosen(m)
	note.Color = "#ff00ff"
	m.ds().dirty = true
	m.ds().save()
	m.ds().sel = doorTo(note) // nothing
	for _, o := range m.ds().octs {
		if doorTo(o) == 1 {
			m.ds().sel = o.ID
		}
	}
	press(m, "enter")
	k1, _ := m.ds().drawKeyOf(door)
	_, cols := doorCells(t, m, door)
	pink := false
	for c := range cols {
		var r, g, b int
		fmt.Sscanf(c, "#%02x%02x%02x", &r, &g, &b)
		if r > 150 && b > 150 && g < 140 {
			pink = true
		}
	}
	if !pink {
		t.Fatalf("room 2's pink note does not show through the door's window: %v", cols)
	}
	// Moved in room 2, the door's drawing is made again.
	note.X += 10
	if k2, _ := m.ds().drawKeyOf(door); k2 == k1 {
		t.Fatal("the door's drawing would not change when its room does")
	}
	m.View()
}

func TestRoomsSpaceGoesThrough(t *testing.T) {
	m := designTest(t)
	roomKey(m, "r")
	press(m, "space")
	if m.hs().Current != 2 {
		t.Fatalf("Space on the door: in room %d, want 2", m.hs().Current)
	}
	time.Sleep(spaceTapGap) // (a second Space this soon is Space held)
	press(m, "space")
	if m.hs().Current != 1 {
		t.Fatalf("Space on the door back: in room %d, want 1", m.hs().Current)
	}
	// Space on anything else still does what it did: no room changes.
	m.ds().sel = -1
	time.Sleep(spaceTapGap)
	press(m, "space")
	if m.hs().Current != 1 {
		t.Fatal("Space with no door chosen changed room")
	}
}

// withAnims turns the moving on, on a clock the test steps.
func withAnims(t *testing.T) *time.Time {
	t.Helper()
	now := time.Now()
	designAnims, animNow = true, func() time.Time { return now }
	t.Cleanup(func() { designAnims, animNow = false, time.Now })
	return &now
}

func TestRoomsDoorSwingsThenIn(t *testing.T) {
	m := designTest(t)
	now := withAnims(t)
	roomKey(m, "r")
	door := chosen(m)
	shut, _ := doorCells(t, m, door)
	press(m, "enter")
	if m.hs().Current != 1 || m.ds().through == nil {
		t.Fatalf("Enter: in room %d, going through %v; want still room 1, the door swinging", m.hs().Current, m.ds().through != nil)
	}
	*now = now.Add(doorSwingFor / 2)
	half, _ := doorCells(t, m, door)
	if half == shut || strings.Contains(half, "Room 2") {
		t.Fatalf("half open, the door looks shut:\n%s", half)
	}
	if k, ok := m.ds().drawKeyOf(door); ok {
		t.Fatalf("a swinging door's drawing would be kept: %v", k)
	}
	m.Update(designAnimMsg{})
	if m.hs().Current != 1 {
		t.Fatal("in the next room before the door is open")
	}
	*now = now.Add(doorFlatFor)
	m.Update(designAnimMsg{})
	if m.hs().Current != 2 || m.design.through != nil {
		t.Fatalf("after the swing: in room %d, want 2", m.hs().Current)
	}
	// A key while it swings: straight in.
	*now = now.Add(time.Second)
	press(m, "enter")
	if m.hs().Current != 2 {
		t.Fatal("went through at once")
	}
	press(m, "x")
	if m.hs().Current != 1 {
		t.Fatalf("a key while the door swings: in room %d, want 1", m.hs().Current)
	}
}

func TestRoomsFlatZoomsIntoDoor(t *testing.T) {
	m := designTest(t)
	now := withAnims(t)
	roomKey(m, "r")
	d1 := m.ds()
	z0, cam0 := d1.zoom, [2]float64{d1.camX, d1.camY}
	start := *now
	press(m, "enter")
	*now = start.Add(doorSwingFor + doorZoomFor/2)
	m.Update(designAnimMsg{})
	if d1.zoom <= z0 {
		t.Fatalf("half way into the doorway: zoom %d, want more than %d", d1.zoom, z0)
	}
	x, y, w, h := d1.rect(d1.byID(d1.through.id))
	if mx, my := x+w/2, y+h/2; abs(mx-m.width/2) > 2 || abs(my-m.boardH()/2) > 2 {
		t.Fatalf("zooming in, the door is not in the middle: its middle %d,%d", mx, my)
	}
	*now = start.Add(doorFlatFor)
	m.Update(designAnimMsg{})
	if m.hs().Current != 2 {
		t.Fatalf("after the zoom: in room %d, want 2", m.hs().Current)
	}
	if d1.zoom != z0 || d1.camX != cam0[0] || d1.camY != cam0[1] {
		t.Fatalf("room 1's view was not put back: zoom %d cam %v,%v; want %d %v", d1.zoom, d1.camX, d1.camY, z0, cam0)
	}
	// In room 2 the view starts close on the door back and zooms out.
	d2 := m.ds()
	if d2.arrive == nil || d2.zoom != len(designZooms)-1 {
		t.Fatalf("arriving: zoom %d, arrive %v; want the closest zoom, zooming out", d2.zoom, d2.arrive != nil)
	}
	*now = now.Add(doorArriveFor / 2)
	m.Update(designAnimMsg{})
	mid := d2.zoom
	*now = now.Add(doorArriveFor)
	m.Update(designAnimMsg{})
	if d2.arrive != nil || d2.zoom != designZoom100 || mid >= len(designZooms)-1 || mid <= designZoom100 {
		t.Fatalf("zooming out: half way %d, at the end %d (arrive %v); want between, then %d", mid, d2.zoom, d2.arrive != nil, designZoom100)
	}
}
