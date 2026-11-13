package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// In 3D, T and Shift+T go through the walls' finishes and F and Shift+F
// through the floor's, round and round, saying each; it is not an undo
// step. While writing, in label mode, and on the flat board, the letters
// are what they always were.
func TestDesign3DFinishKeys(t *testing.T) {
	m, a, _ := middleScene(t)
	d := m.ds()
	press(m, "t")
	if d.walls != 0 {
		t.Fatal("T on the flat board changed the walls")
	}
	press(m, "ctrl+f")
	boardRows(m)
	undos := len(d.undo)
	press(m, "t")
	if d.walls != wallBrick || m.msg != "Walls: brick.  T: next · Shift+T: back" {
		t.Fatalf("T: walls %d, said %q", d.walls, m.msg)
	}
	press(m, "shift+t")
	press(m, "shift+t")
	if d.walls != len(wallNames)-1 {
		t.Fatalf("Shift+T twice from brick: walls %d, want the last, %d", d.walls, len(wallNames)-1)
	}
	press(m, "f")
	press(m, "f")
	if d.floor != floorCarpet || !strings.HasPrefix(m.msg, "Floor: carpet tiles.") {
		t.Fatalf("F twice: floor %d, said %q", d.floor, m.msg)
	}
	press(m, "shift+f")
	if d.floor != floorParquet {
		t.Fatalf("Shift+F: floor %d, want parquet", d.floor)
	}
	if len(d.undo) != undos {
		t.Fatalf("finishes made %d undo steps", len(d.undo)-undos)
	}
	// Writing in a structure: T is a letter.
	d.sel, d.editing = a.ID, partBody
	press(m, "t")
	if d.walls != len(wallNames)-1 {
		t.Fatal("T while writing changed the walls")
	}
	d.editing = partNone
	// Label mode keeps T for the label's words.
	d.labelMode = true
	walls := d.walls
	m.design3DFinishKey("t")
	if d.walls != walls {
		t.Fatal("T in label mode changed the walls")
	}
	d.labelMode = false
}

// The finishes are kept with the board: written by name in board.json,
// left out when they are the first (so an old board's file is as it was),
// and read back; a name this sy does not know is the first.
func TestDesign3DFinishSaved(t *testing.T) {
	m, _, _ := middleScene(t)
	d := m.ds()
	path := designPath()
	press(m, "ctrl+f")
	boardRows(m)
	press(m, "x") // anything, so the board is written once
	d.dirty = true
	d.save()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"walls"`) || strings.Contains(string(data), `"floor"`) {
		t.Fatalf("the first finishes are written: %s", data)
	}
	press(m, "t")
	press(m, "t") // wood panels
	press(m, "shift+f")
	d.save()
	data, _ = os.ReadFile(path)
	var idx designIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		t.Fatal(err)
	}
	if idx.Walls != "wood panels" || idx.Floor != "slate" {
		t.Fatalf("board.json keeps walls %q floor %q, want wood panels and slate", idx.Walls, idx.Floor)
	}
	back := loadDesign(path)
	if back.walls != wallPanels || back.floor != floorSlate {
		t.Fatalf("read back: walls %d floor %d", back.walls, back.floor)
	}
	idx.Walls, idx.Floor = "gold leaf", ""
	data, _ = json.Marshal(idx)
	os.WriteFile(path, data, 0o600)
	if back := loadDesign(path); back.walls != 0 || back.floor != 0 {
		t.Fatalf("an unknown finish: walls %d floor %d, want the first", back.walls, back.floor)
	}
}

// The room in letters draws every finish, at its size; the paper's
// stripes and the planks' joins are drawn only for paper and planks.
func TestDesign3DFinishLetters(t *testing.T) {
	m := room3DModel(t, 120, 40)
	designFrames = false
	d := m.ds()
	r := &d.room
	r.yaw, r.pitch = -40, -20 // the left wall and the ceiling in sight
	for i := range max(len(wallNames), len(floorNames)) {
		d.walls, d.floor = min(i, len(wallNames)-1), min(i, len(floorNames)-1)
		r.frame = ""
		checkRows(t, m, wallNames[d.walls])
		stripes := 0
		inkCells(r, func(_ int, k *r3ink) {
			if (k.face == faceLeft || k.face == faceRight) && k.s == "┊" {
				stripes++
			}
		})
		if (stripes > 0) != (d.walls == wallPaper) {
			t.Errorf("walls %s: %d cells of the paper's stripes", wallNames[d.walls], stripes)
		}
	}
}

// No finish flickers: seen from far off, where a pixel spans several
// units, a pixel's colour hardly changes as the camera moves a quarter of
// a pixel, as with the paper and the planks; and seen close, the pattern
// shows (the colour does change along the surface).
func TestDesign3DFinishNoFlicker(t *testing.T) {
	rm := &r3room{fy: 0, cy: 80, big: 120}
	base, text := uint32(0x2a2a2a), uint32(0xff4518)
	gap := func(c, d uint32) int {
		r1, g1, b1 := chans(c)
		r2, g2, b2 := chans(d)
		return int(max(absF(r1-r2), absF(g1-g2), absF(b1-b2)))
	}
	// jump is the biggest change between two points a quarter of a pixel
	// apart, along a line across the surface; span is how much the colour
	// changes along it in all.
	jump := func(at func(u, foot float64) uint32, foot float64) (worst, span int) {
		lo, hi := 255, 0
		for k := range 4000 {
			u := 3.1 + 0.093*float64(k)
			c := at(u, foot)
			worst = max(worst, gap(c, at(u+foot/4, foot)))
			l := int(chansSum(c))
			lo, hi = min(lo, l), max(hi, l)
		}
		return worst, hi - lo
	}
	for fin := range wallNames {
		for _, along := range []bool{true, false} {
			at := func(u, foot float64) uint32 {
				x, y := u, 40.0+0.37*u/10
				if !along {
					x, y = 17.3+0.37*u/10, u/4
				}
				return wallFinish(fin, base, text, x, y, foot, 2, plaster(x, y, foot), rm)
			}
			if w, _ := jump(at, 6); w > 8 {
				t.Errorf("walls %s (along %v): far off, a quarter pixel's move changes a pixel by %d", wallNames[fin], along, w)
			}
			if _, s := jump(at, 0.05); fin != wallPaper && s < 20 {
				t.Errorf("walls %s (along %v): close, the colour spans only %d: no pattern", wallNames[fin], along, s)
			}
		}
	}
	for fin := range floorNames {
		for _, along := range []bool{true, false} {
			at := func(u, foot float64) uint32 {
				x, z := u, 30.0+0.41*u/10
				if !along {
					x, z = 11.9+0.41*u/10, u
				}
				return floorFinish(fin, 0x5a4a3a, x, z, foot, foot)
			}
			if w, _ := jump(at, 6); w > 8 {
				t.Errorf("floor %s (along %v): far off, a quarter pixel's move changes a pixel by %d", floorNames[fin], along, w)
			}
			if _, s := jump(at, 0.05); s < 20 {
				t.Errorf("floor %s (along %v): close, the colour spans only %d: no pattern", floorNames[fin], along, s)
			}
		}
	}
}

func chansSum(c uint32) float64 { r, g, b := chans(c); return r + g + b }

// The blurred checkerboard is exact in the middle of its squares when they
// are big, and a half when they are far smaller than a pixel.
func TestFilteredChecker(t *testing.T) {
	if v := filteredChecker(0.5, 0.5, 0.01); absF(v-0) > 1e-6 && absF(v-1) > 1e-6 {
		t.Fatalf("in a square's middle: %v, want 0 or 1", v)
	}
	if a, b := filteredChecker(0.5, 0.5, 0.01), filteredChecker(1.5, 0.5, 0.01); absF(a+b-1) > 1e-6 {
		t.Fatalf("neighbouring squares: %v and %v, want one of each", a, b)
	}
	for _, p := range [][2]float64{{0.3, 0.7}, {13.1, -4.2}, {-7.7, 2.2}} {
		if v := filteredChecker(p[0], p[1], 20); absF(v-0.5) > 0.06 {
			t.Fatalf("far off at %v: %v, want about a half", p, v)
		}
	}
}
