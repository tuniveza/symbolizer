package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// room3DModel is a w×h Design mode with a few of each kind of structure.
func room3DModel(t testing.TB, w, h int) *Model {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	m, _ := NewModel("", 0)
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	m.setMode(designMode)
	d := m.ds()
	d.camX, d.camY = 0, 0
	kinds := []string{kOct, kPage, kNote, kCabinet, kDrawer, kRack, kBoard, kOct}
	for i, k := range kinds {
		o := d.newKind(k, 0, 0)
		o.X, o.Y, o.Header = 3+(i%4)*36, 2+(i/4)*20, "Thing "+fmt.Sprint(i)
		o.Body = "Some words on it."
	}
	d.sel = -1
	return m
}

// boardRows is the board area as drawn (designRows), split into lines.
func boardRows(m *Model) []string {
	var sb strings.Builder
	m.designRows(&sb)
	return strings.Split(strings.TrimSuffix(sb.String(), "\n"), "\n")
}

// checkRows fails unless the board area is exactly boardH lines of width cells.
func checkRows(t *testing.T, m *Model, what string) {
	t.Helper()
	rows := boardRows(m)
	if len(rows) != m.boardH() {
		t.Fatalf("%s: %d lines, want %d", what, len(rows), m.boardH())
	}
	for i, r := range rows {
		if w := ansi.StringWidth(r); w != m.width {
			t.Fatalf("%s: line %d is %d cells wide, want %d", what, i, w, m.width)
		}
	}
}

// Ctrl + F goes into 3D, first person: the mouse and arrows look round,
// W A S D move, and Esc (or Ctrl + F again) is the flat board as before.
// Ctrl + arrows no longer turn anything.
func TestDesign3DFirstPerson(t *testing.T) {
	m := room3DModel(t, 120, 40)
	designFrames = false
	flat := boardRows(m)
	d := m.ds()
	for _, k := range []string{"ctrl+right", "ctrl+up", "=", "p"} {
		press(m, k)
		if d.room.on() {
			t.Fatalf("%s on the flat board: no 3D", k)
		}
	}
	flat = boardRows(m)
	press(m, "ctrl+f")
	if !d.room.on() || !d.room.free {
		t.Fatal("Ctrl+F: the first-person camera")
	}
	checkRows(t, m, "3D")
	if !strings.Contains(m.designStatus(), "3D") {
		t.Fatalf("the status bar should say it is 3D: %q", m.designStatus())
	}
	press(m, "right")
	if d.room.look[0] != turnStep {
		t.Fatalf("Right: looks %v° right, want %d", d.room.look[0], turnStep)
	}
	pos := d.room.pos
	press(m, "w")
	if d.room.pos.sub(pos).len() < 1e-6 {
		t.Fatal("W: the camera should move")
	}
	for _, k := range []string{"=", "-", "0", "p", "f", "q", "e"} {
		look, pos := d.room.look, d.room.pos
		press(m, k)
		if d.room.look != look || d.room.pos != pos || !d.room.on() {
			t.Fatalf("%s: the camera should not change (looks %v, at %v)", k, d.room.look, d.room.pos)
		}
	}
	press(m, "esc")
	if d.room.on() {
		t.Fatal("Esc in 3D: the flat board")
	}
	if strings.Join(boardRows(m), "\n") != strings.Join(flat, "\n") {
		t.Fatal("out of 3D, the flat board should be exactly as before")
	}
	press(m, "ctrl+f")
	press(m, "ctrl+f")
	if d.room.on() {
		t.Fatal("Ctrl+F twice: the flat board")
	}
}

// Every angle, camera distance and screen size: whole lines, no panics,
// with an empty board too.
func TestDesign3DEveryAngle(t *testing.T) {
	for _, sz := range [][2]int{{20, 8}, {12, 6}, {41, 13}, {200, 60}} {
		for _, empty := range []bool{false, true} {
			m := room3DModel(t, sz[0], sz[1])
			if empty {
				m.ds().octs = nil
				m.ds().idIndex = nil
			}
			r := &m.ds().room
			for yaw := -80; yaw <= 80; yaw += 20 {
				for pitch := -75; pitch <= 75; pitch += 15 {
					for _, near := range []int{-6, 0, 10} {
						r.yaw, r.pitch, r.near = yaw, pitch, near
						if !r.on() {
							continue
						}
						checkRows(t, m, fmt.Sprintf("%dx%d empty %v yaw %d pitch %d near %d", sz[0], sz[1], empty, yaw, pitch, near))
					}
				}
			}
		}
	}
}

// kitty says when keys are let go: held, an arrow in 3D keeps looking
// round; let go, it stops.
func TestDesign3DHold(t *testing.T) {
	m := room3DModel(t, 80, 24)
	m.releases = true
	d := m.ds()
	press(m, "ctrl+f")
	press(m, "right")
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight, IsRepeat: true}))
	if d.room.look[0] != turnStep {
		t.Fatalf("a press and a repeat: looks %v°, want one step (the glide does the rest)", d.room.look[0])
	}
	for i := 0; i < 3; i++ {
		m.Update(designHoldMsg{})
	}
	held := float64(turnStep + 3*turnGlide)
	if d.room.look[0] != held {
		t.Fatalf("held for three glide steps: looks %v°, want %v", d.room.look[0], held)
	}
	m.Update(tea.KeyReleaseMsg(tea.Key{Code: tea.KeyRight}))
	_, cmd := m.Update(designHoldMsg{})
	if d.room.look[0] != held || d.holdTick || cmd != nil {
		t.Fatalf("let go: looks %v° (want %v), glide still going %v", d.room.look[0], held, d.holdTick)
	}
}

// The lamp lights the room: surfaces change brightness as it turns.
func TestDesign3DLight(t *testing.T) {
	m := room3DModel(t, 120, 40)
	r := &m.ds().room
	bright := func(yaw, pitch int) float64 {
		r.yaw, r.pitch = yaw, pitch
		boardRows(m)
		sum, n := 0.0, 0
		for i := range r.face {
			if r.face[i] == 0 { // the back wall
				cr, cg, cb := chans(r.lit(r.base[i], i))
				sum += cr + cg + cb
				n++
			}
		}
		return sum / float64(max(n, 1))
	}
	a, b := bright(-60, 0), bright(60, 40)
	if diff := a - b; diff < 3 && diff > -3 {
		t.Fatalf("the back wall should look brighter or darker as the room turns: %.1f and %.1f", a, b)
	}
}

// Thousands of structures: still whole lines.
func TestDesign3DThousands(t *testing.T) {
	m := room3DModel(t, 160, 50)
	d := m.ds()
	for i := 0; i < 3000; i++ {
		o := d.newKind([]string{kOct, kPage, kCabinet, kNote}[i%4], 0, 0)
		o.X, o.Y = (i%60)*28-800, (i/60)*12-300
	}
	d.zoom = 0
	d.room.yaw, d.room.pitch = 30, 20
	checkRows(t, m, "3000 structures")
}

// SY_3D_PNG=dir go test -run TestDesign3DPictures writes the room's pixels
// at a few angles as pictures, to look at while working on it.
func TestDesign3DPictures(t *testing.T) {
	dir := os.Getenv("SY_3D_PNG")
	if dir == "" {
		t.Skip("set SY_3D_PNG to a folder")
	}
	m := room3DModel(t, 200, 60)
	r := &m.ds().room
	for _, a := range [][2]int{{5, 0}, {-25, 10}, {25, 10}, {0, -45}, {0, 75}, {35, 25}, {80, 0}, {-80, 0}, {80, 75}, {-80, -45}} {
		r.yaw, r.pitch = a[0], a[1]
		boardRows(m)
		img := image.NewRGBA(image.Rect(0, 0, r.pw*2, r.ph*4)) // a pixel is half a cell: twice as tall as wide
		for y := 0; y < r.ph; y++ {
			for x := 0; x < r.pw; x++ {
				c := r.pixel(y*r.pw + x)
				col := color.RGBA{uint8(c >> 16), uint8(c >> 8), uint8(c), 255}
				for k := 0; k < 8; k++ {
					img.Set(2*x+k%2, 4*y+k/2, col)
				}
			}
		}
		f, _ := os.Create(filepath.Join(dir, fmt.Sprintf("room_%d_%d.png", a[0], a[1])))
		png.Encode(f, img)
		f.Close()
	}
}

func BenchmarkDesign3D(b *testing.B) {
	m, _ := designBenchModel(b)
	r := &m.ds().room
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.yaw, r.pitch = 20+i%10, 15
		var sb strings.Builder
		m.designRows(&sb)
	}
}

// Free roam, walking into the room at 200×60: frames with the camera close
// to the blocks and then among them.
func BenchmarkDesign3DFree(b *testing.B) {
	m, _ := designBenchModel(b)
	press(m, "ctrl+f")
	r := &m.ds().room
	start := r.pos
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.pos = start.add(r3vec{0, 0, -float64(i%40) * 7})
		var sb strings.Builder
		m.designRows(&sb)
	}
	b.ReportMetric(float64(r.cost)/float64(time.Millisecond), "ms/last")
}

func BenchmarkDesign3DHuge(b *testing.B) {
	m := hugeBoard(b)
	d := m.ds()
	d.zoom, d.camX, d.camY = 0, -400, -150
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.room.yaw, d.room.pitch = 20+i%10, 15
		var sb strings.Builder
		m.designRows(&sb)
	}
	b.ReportMetric(float64(d.room.cost)/float64(time.Millisecond), "ms/last")
	b.ReportMetric(float64(len(d.room.faces)), "faces")
}

// The owner's view (turned 35° right, tilted 30° down) of a board of
// pages with lines of words, a box, drawers and a cabinet, at 200×60:
// most of a frame's words drawn as letters.
func BenchmarkDesign3DWords(b *testing.B) {
	m := wordsScene(b, 200, 60)
	r := &m.ds().room
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.yaw, r.pitch = 35+i%2, 30
		var sb strings.Builder
		m.designRows(&sb)
	}
	b.ReportMetric(float64(r.cost)/float64(time.Millisecond), "ms/last")
}

// In 3D the arrows look round and W A S D move; on the flat board they
// do not.
func TestDesign3DArrowsLookWASDWalk(t *testing.T) {
	m := room3DModel(t, 120, 40)
	designFrames = false
	d := m.ds()
	press(m, "left")
	press(m, "w")
	if d.room.on() {
		t.Fatalf("flat board: the arrows and W must not go into 3D")
	}
	press(m, "ctrl+f")
	press(m, "right")
	if d.room.look[0] != turnStep {
		t.Fatalf("an arrow in 3D: look %v, want %d right", d.room.look, turnStep)
	}
	press(m, "down")
	if d.room.look[1] != turnStep {
		t.Fatalf("down in 3D: look %v, want %d down", d.room.look, turnStep)
	}
	boardRows(m)
	at := d.room.pos
	press(m, "w")
	if d.room.pos == at {
		t.Fatalf("W in free roam: the camera did not move")
	}
	look := d.room.look
	press(m, "left")
	if d.room.look[0] >= look[0] {
		t.Fatalf("left in free roam: look %v, want it turned left of %v", d.room.look, look)
	}
	press(m, "esc")
	if d.room.on() {
		t.Fatalf("Esc: want the flat board")
	}
}
