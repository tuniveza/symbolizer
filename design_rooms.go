package main

// ---- rooms, their walls and floor, and levels ----------------------------------------
//
// A house has rooms. Each room has 4 walls (more if added) and a floor,
// and each wall and the floor is a board of its own (its own structures,
// ink and pictures). The rooms stand on levels, one above another.
//
//	AltGr+R           turn to the next wall (AltGr+Shift+R: the one before), with a spin
//	AltGr+F           look down at the floor, and up again: the place for what is not finished
//	AltGr+N, then R   a new room on this level, and a door to it here
//	AltGr+N, then W   a new wall in this room, after this one
//	AltGr+N, then L   a new level above the top one, its first room, and stairs up to it here
//	Enter or Space    on a chosen door or stairs: go through
//
// A door is a structure like a page: it can be moved, written on, put on a
// layer in 3D and deleted. Its title is the name of the room it leads to,
// so writing a new title renames that room (as it is gone through). Each
// new room gets a door back (stairs down, on a new floor), chosen as it is
// arrived in, so Enter again goes back.
//
//	~/.local/share/symbolizer/design/house.json                  the rooms, their levels, where you are
//	~/.local/share/symbolizer/design/board.json                  room 1's first wall (the board from before rooms)
//	~/.local/share/symbolizer/design/rooms/2/board.json          every other room's first wall, a folder each
//	~/.local/share/symbolizer/design/rooms/N/walls/K/board.json  room N's wall K (2, 3, 4 …)
//	~/.local/share/symbolizer/design/rooms/N/floor/board.json    room N's floor
//
// A wall is written to disk when it is left, so only the one you are on
// waits to be saved when sy quits.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	kDoor   = "door"   // a door to another room on this floor
	kStairs = "stairs" // stairs to a room on another floor
)

const (
	doorW, doorH     = 22, 18
	roomLink         = "room:" // a door's From: the room it leads to ("room:2")
	houseFirstFloor  = 1       // floors are counted from 1, the ground floor
	houseFirstRoomID = 1
)

type houseRoom struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Floor int    `json:"floor"`           // its level (the JSON name is from before floors were floors)
	Walls int    `json:"walls,omitempty"` // how many walls: 4 when less
	Wall  int    `json:"wall,omitempty"`  // the wall you face, from 0
	Down  bool   `json:"down,omitempty"`  // looking at the floor
}

// minWalls is how many walls every room has at least.
const minWalls = 4

// walls is how many walls room r has.
func (r *houseRoom) walls() int { return max(r.Walls, minWalls) }

// wallFloor stands for the floor where a wall's number goes.
const wallFloor = -1

// at is the wall (or wallFloor) you look at in room r.
func (r *houseRoom) at() int {
	if r.Down {
		return wallFloor
	}
	return ((r.Wall % r.walls()) + r.walls()) % r.walls()
}

type houseFile struct {
	Rooms   []houseRoom `json:"rooms"`
	Current int         `json:"current"`         // the room you are in
	Music   bool        `json:"music,omitempty"` // the chiptune on the board (design_music.go)
}

func housePath() string { return filepath.Join(filepath.Dir(designPath()), "house.json") }

// boardPath is the board file of room id's wall (from 0), or its floor
// (wallFloor). Wall 0 is the room's board from before walls.
func boardPath(id, wall int) string {
	switch {
	case wall == 0:
		return roomPath(id)
	case wall == wallFloor:
		return filepath.Join(filepath.Dir(designPath()), "rooms", strconv.Itoa(id), "floor", "board.json")
	}
	return filepath.Join(filepath.Dir(designPath()), "rooms", strconv.Itoa(id), "walls", strconv.Itoa(wall+1), "board.json")
}

// boardKey is a wall's (or the floor's) key in m.rooms.
func boardKey(id, wall int) int {
	if wall == wallFloor {
		return id*1000 + 999
	}
	return id*1000 + wall
}

// curBoard is the room you are in and the wall (or floor) you look at.
func (m *Model) curBoard() (id, wall int) {
	h := m.hs()
	return h.Current, h.room(h.Current).at()
}

// roomPath is room id's board file: room 1 is the board from before rooms.
func roomPath(id int) string {
	if id == houseFirstRoomID {
		return designPath()
	}
	return filepath.Join(filepath.Dir(designPath()), "rooms", strconv.Itoa(id), "board.json")
}

// hs is the house, read the first time it is needed. With no house file
// it is the one room there always was.
func (m *Model) hs() *houseFile {
	if m.house != nil {
		return m.house
	}
	h := &houseFile{}
	if data, err := os.ReadFile(housePath()); err == nil {
		_ = json.Unmarshal(data, h)
	}
	if h.room(houseFirstRoomID) == nil {
		h.Rooms = append([]houseRoom{{ID: houseFirstRoomID, Name: "Room 1", Floor: houseFirstFloor}}, h.Rooms...)
	}
	if h.room(h.Current) == nil {
		h.Current = houseFirstRoomID
	}
	m.house = h
	return h
}

func (h *houseFile) room(id int) *houseRoom {
	for i := range h.Rooms {
		if h.Rooms[i].ID == id {
			return &h.Rooms[i]
		}
	}
	return nil
}

func (h *houseFile) nextID() int {
	n := 0
	for _, r := range h.Rooms {
		n = max(n, r.ID)
	}
	return n + 1
}

func (h *houseFile) topFloor() int {
	n := houseFirstFloor
	for _, r := range h.Rooms {
		n = max(n, r.Floor)
	}
	return n
}

func (h *houseFile) save() error {
	data, err := json.MarshalIndent(h, "", " ")
	if err != nil {
		return err
	}
	return writeBoardFile(housePath(), data)
}

// where is the room's name and floor, as the toolbar and messages say it.
func (h *houseFile) where(id int) string {
	r := h.room(id)
	if r == nil {
		return ""
	}
	s := r.Name
	if r.Down {
		s += " · floor"
	} else {
		s += fmt.Sprintf(" · wall %d/%d", r.at()+1, r.walls())
	}
	if h.topFloor() > houseFirstFloor {
		s += fmt.Sprintf(" · level %d", r.Floor)
	}
	return s
}

// levelWords renames "Floor N" to "Level N" on doors and stairs made
// before a room's floor was its floor (16 Nov 2026).
func levelWords(os []*dOct) {
	for _, o := range os {
		if o == nil || (o.Kind != kDoor && o.Kind != kStairs) {
			continue
		}
		if strings.HasPrefix(o.Footer, "Floor ") {
			o.Footer = "Level " + strings.TrimPrefix(o.Footer, "Floor ")
		}
		o.Body = strings.Replace(o.Body, " to Floor ", " to level ", 1)
	}
}

// whereShort is the wall (or floor) you look at in room id, short.
func (h *houseFile) whereShort(id int) string {
	r := h.room(id)
	if r == nil {
		return ""
	}
	if r.Down {
		return "floor"
	}
	return fmt.Sprintf("wall %d/%d", r.at()+1, r.walls())
}

// doorTo is the room a door or stairs leads to (0: none).
func doorTo(o *dOct) int {
	if o == nil || (o.Kind != kDoor && o.Kind != kStairs) || !strings.HasPrefix(o.From, roomLink) {
		return 0
	}
	id, _ := strconv.Atoi(strings.TrimPrefix(o.From, roomLink))
	return id
}

// makeDoor turns o (just placed) into a door or stairs to room to.
func (h *houseFile) makeDoor(o *dOct, kind string, to int, from int) {
	o.Kind, o.From = kind, roomLink+strconv.Itoa(to)
	o.W, o.H = doorW, doorH
	o.Header = h.room(to).Name
	o.Footer = fmt.Sprintf("Level %d", h.room(to).Floor)
	switch {
	case kind == kDoor:
		o.Color = "#8a5a35" // wood
		o.Body = "Enter or Space: go through."
	case h.room(to).Floor > h.room(from).Floor:
		o.Color = "#8f8a80" // stone
		o.Body = fmt.Sprintf("Stairs up to level %d.\nEnter or Space: go up.", h.room(to).Floor)
	default:
		o.Color = "#8f8a80"
		o.Body = fmt.Sprintf("Stairs down to level %d.\nEnter or Space: go down.", h.room(to).Floor)
	}
}

// houseKey is the key after AltGr+N: R a room, W a wall, L a level (held
// AltGr: ¶, ł). It says whether it was one.
func (m *Model) houseKey(name string, cx, cy int) bool {
	var floor bool
	switch name {
	case "r", "R", "shift+r", "¶", "altgr+r":
	case "w", "W", "shift+w", "ł", "altgr+w":
		m.addWall()
		return true
	case "l", "L", "shift+l", "Ł", "altgr+l":
		floor = true
	default:
		return false
	}
	h := m.hs()
	d := m.ds()
	here := h.Current
	nr := houseRoom{ID: h.nextID(), Floor: h.room(here).Floor}
	kind := kDoor
	if floor {
		nr.Floor, kind = h.topFloor()+1, kStairs
	}
	nr.Name = "Room " + strconv.Itoa(nr.ID)
	h.Rooms = append(h.Rooms, nr)
	if err := h.save(); err != nil {
		h.Rooms = h.Rooms[:len(h.Rooms)-1]
		m.say("The new room could not be saved: " + err.Error())
		return true
	}

	// The door here, in the middle of the view.
	wx, wy := d.world(cx, cy)
	o := d.newKind(kind, int(wx), int(wy))
	if o != nil {
		ox, oy, ow, oh := o.X, o.Y, o.W, o.H
		h.makeDoor(o, kind, nr.ID, here)
		o.X, o.Y = ox+ow/2-o.W/2, oy+oh/2-o.H/2 // the same middle, at a door's size
		if d.clearSpot(o) {
			m.centreOn(o)
		}
		d.dirty = true
	}

	// The new room's own board, with a door back, in the middle of its view.
	nd := loadDesign(roomPath(nr.ID))
	nd.walls, nd.floor = d.walls, d.floor // the same finishes to start with
	bx, by := m.width/2, m.boardH()/2
	back := nd.newKind(kind, bx, by)
	h.makeDoor(back, kind, here, nr.ID)
	back.X, back.Y = bx-back.W/2, by-back.H/2
	nd.save()
	nd.flushSave() // on disk now: only the room you are in waits to be saved
	if m.rooms == nil {
		m.rooms = map[int]*designState{}
	}
	m.rooms[boardKey(nr.ID, 0)] = nd

	d.editing = partNone
	if floor {
		m.say(fmt.Sprintf("New level: level %d, with %s. Enter or Space on the stairs goes up.", nr.Floor, nr.Name))
	} else {
		m.say("New room: " + nr.Name + ". Enter or Space on the door goes in. Tab: write its name.")
	}
	return true
}

// goThrough is Enter (or Space) on a chosen door or stairs: to the room it leads to.
// It says whether it was one.
func (m *Model) goThrough() bool {
	d := m.ds()
	o := d.byID(d.sel)
	to := doorTo(o)
	if to == 0 {
		return false
	}
	h := m.hs()
	if h.room(to) == nil {
		m.say("That room is not in the house any more.")
		return true
	}
	if t := strings.TrimSpace(o.Header); o.Kind == kDoor && t != "" && t != h.room(to).Name {
		h.room(to).Name = t // the door's title names the room
	}
	if d.through != nil {
		m.finishThrough() // going through already: straight in
		return true
	}
	if m.startThrough(o, to) { // it swings open first, then the view zooms in (design_door.go)
		return true
	}
	m.goRoom(to)
	return true
}

// goRoom leaves the room you are in (written to disk first) for room id,
// facing the wall with the door back (the first wall if none has one).
func (m *Model) goRoom(id int) {
	h := m.hs()
	from := h.Current
	r := h.room(id)
	r.Down = false
	wall := r.at()
	for w := range r.walls() { // the wall with the door back, the one you face now first
		w = (r.at() + w) % r.walls()
		if b := m.boardOf(id, w); b != nil && b.doorBackTo(from) != nil {
			wall = w
			break
		}
	}
	nd := m.switchBoard(id, wall)
	nd.multi = nil
	if o := nd.doorBackTo(from); o != nil { // the door back is chosen: Enter again goes back
		nd.sel = o.ID
		m.centreOn(o)
	}
	m.say("You are in " + h.where(id) + ".")
}

// doorBackTo is the door or stairs on this board to room id (nil: none).
func (d *designState) doorBackTo(id int) *dOct {
	for _, o := range d.octs {
		if doorTo(o) == id {
			return o
		}
	}
	return nil
}

// boardOf is room id's wall (or floor) board: the one on screen, one kept,
// or read from disk (once).
func (m *Model) boardOf(id, wall int) *designState {
	if cid, cw := m.curBoard(); m.design != nil && cid == id && cw == wall {
		return m.design
	}
	if m.rooms == nil {
		m.rooms = map[int]*designState{}
	}
	k := boardKey(id, wall)
	if b := m.rooms[k]; b != nil {
		return b
	}
	b := loadDesign(boardPath(id, wall))
	b.isFloor = wall == wallFloor
	m.rooms[k] = b
	return b
}

// switchBoard leaves the board on screen (written to disk first) for room
// id's wall (or floor), and gives it, sized for the screen.
func (m *Model) switchBoard(id, wall int) *designState {
	h := m.hs()
	if d := m.design; d != nil {
		d.editing, d.placing, d.placingRoom = partNone, false, false
		d.save()
		d.flushSave()
		cid, cw := m.curBoard()
		if m.rooms == nil {
			m.rooms = map[int]*designState{}
		}
		m.rooms[boardKey(cid, cw)] = d
	}
	nd := m.boardOf(id, wall)
	m.design = nd
	h.Current = id
	r := h.room(id)
	if wall == wallFloor {
		r.Down = true
	} else {
		r.Down, r.Wall = false, wall
	}
	if err := h.save(); err != nil {
		nd.saveErr = err.Error()
	}
	return m.ds() // its height for this screen
}
