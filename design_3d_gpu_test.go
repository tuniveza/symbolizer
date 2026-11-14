//go:build linux && cgo

package main

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// gpuPicture draws model m's room once, on the graphics card or on the
// processor, and is its picture and how long the drawing took.
func gpuPicture(t testing.TB, m *Model, gpu bool) ([]byte, int, int, time.Duration) {
	was := room3DGPU
	room3DGPU = gpu
	defer func() { room3DGPU = was }()
	var pic []byte
	var pw, ph int
	room3DSeeRGB = func(rgb []byte, w, h int) { pic, pw, ph = append([]byte(nil), rgb...), w, h }
	defer func() { room3DSeeRGB = nil }()
	r := &m.ds().room
	r.frame = "" // drawn afresh
	var sb strings.Builder
	m.designRows(&sb)
	if gpu != r.gpuDrawn {
		t.Fatalf("gpu %v: drawn on the card %v (%s)", gpu, r.gpuDrawn, gpuRoom.frameWhy)
	}
	return pic, pw, ph, r.cost
}

// The room drawn on the graphics card is the room drawn on the processor:
// the same faces, colours, letters and light, pixel for pixel but for a
// little rounding (the card works the light out at every pixel, the
// processor over 2×2, and in float32).
func TestDesign3DGPUMatches(t *testing.T) {
	atClock(t, 12, 0) // the people outside stand still between the two drawings (design_3d_world.go)
	m := wordsScene(t, 100, 33)
	for m.boardH() < 27 {
		m.Update(tea.WindowSizeMsg{Width: 100, Height: m.height + 1})
	}
	withPixels(t, m, 10, 20)
	room3DGPU = true
	ok := gpuReady()
	room3DGPU = false
	if !ok {
		t.Skip("no graphics card: " + gpuRoom.why)
	}
	t.Logf("card: %s", gpuRendererName())
	r := &m.ds().room
	for _, cam := range []struct{ yaw, pitch, near int }{{1, 0, 0}, {35, 20, 0}, {-50, -15, 3}, {20, 40, 6}} {
		r.yaw, r.pitch, r.near = cam.yaw, cam.pitch, cam.near
		cpu, pw, ph, cpuT := gpuPicture(t, m, false)
		gpu, gw, gh, gpuT := gpuPicture(t, m, true)
		if pw != gw || ph != gh || len(cpu) != len(gpu) || len(cpu) == 0 {
			t.Fatalf("%v: sizes cpu %dx%d gpu %dx%d", cam, pw, ph, gw, gh)
		}
		far, sum := 0, 0
		for i := 0; i < len(cpu); i += 3 {
			d := 0
			for c := range 3 {
				d = max(d, abs(int(cpu[i+c])-int(gpu[i+c])))
			}
			sum += d
			if d > 40 {
				far++
			}
		}
		n := len(cpu) / 3
		mean := float64(sum) / float64(n)
		t.Logf("%+v: %dx%d, mean difference %.2f, %.3f%% far apart; cpu %v gpu %v", cam, pw, ph, mean, 100*float64(far)/float64(n), cpuT, gpuT)
		if mean > 2 || float64(far) > 0.005*float64(n) {
			t.Errorf("%+v: the card's picture differs: mean %.2f, %d of %d pixels far apart", cam, mean, far, n)
		}
	}
}

// Every finish of the walls and the floor (design_3d_finish.go) is the
// same on the graphics card as on the processor, near and far, from the
// side and from above.
func TestDesign3DGPUFinishes(t *testing.T) {
	atClock(t, 12, 0) // the people outside stand still between the two drawings (design_3d_world.go)
	m := wordsScene(t, 100, 33)
	for m.boardH() < 27 {
		m.Update(tea.WindowSizeMsg{Width: 100, Height: m.height + 1})
	}
	withPixels(t, m, 10, 20)
	room3DGPU = true
	ok := gpuReady()
	room3DGPU = false
	if !ok {
		t.Skip("no graphics card: " + gpuRoom.why)
	}
	d := m.ds()
	r := &d.room
	defer func() { d.walls, d.floor = 0, 0 }()
	cams := []struct{ yaw, pitch, near int }{{35, 20, 0}, {-50, -15, 3}, {-70, 30, 8}}
	first := make([][]byte, len(cams)) // paper and planks, from each camera
	for i := range max(len(wallNames), len(floorNames)) {
		d.walls, d.floor = min(i, len(wallNames)-1), min(i, len(floorNames)-1)
		for ci, cam := range cams {
			r.yaw, r.pitch, r.near = cam.yaw, cam.pitch, cam.near
			cpu, _, _, _ := gpuPicture(t, m, false)
			gpu, _, _, _ := gpuPicture(t, m, true)
			if i == 0 {
				first[ci] = cpu
			} else if len(cpu) == len(first[ci]) {
				// The finish shows: the room is not paper and planks.
				changed := 0
				for k := range cpu {
					if abs(int(cpu[k])-int(first[ci][k])) > 6 {
						changed++
					}
				}
				if changed < len(cpu)/100 {
					t.Errorf("walls %s, floor %s, %+v: only %d of %d bytes differ from paper and planks", wallNames[d.walls], floorNames[d.floor], cam, changed, len(cpu))
				}
				t.Logf("  %.1f%% of the picture changed from paper and planks", 100*float64(changed)/float64(len(cpu)))
			}
			if len(cpu) != len(gpu) || len(cpu) == 0 {
				t.Fatalf("%s, %s: sizes cpu %d gpu %d", wallNames[d.walls], floorNames[d.floor], len(cpu), len(gpu))
			}
			far, sum := 0, 0
			for k := 0; k < len(cpu); k += 3 {
				dd := 0
				for c := range 3 {
					dd = max(dd, abs(int(cpu[k+c])-int(gpu[k+c])))
				}
				sum += dd
				if dd > 40 {
					far++
				}
			}
			n := len(cpu) / 3
			mean := float64(sum) / float64(n)
			t.Logf("walls %s, floor %s, %+v: mean difference %.2f, %.3f%% far apart", wallNames[d.walls], floorNames[d.floor], cam, mean, 100*float64(far)/float64(n))
			if mean > 2 || float64(far) > 0.005*float64(n) {
				t.Errorf("walls %s, floor %s, %+v: the card's picture differs: mean %.2f, %d of %d pixels far apart",
					wallNames[d.walls], floorNames[d.floor], cam, mean, far, n)
			}
		}
	}
}

// The owner's screen: a board of 100×27 cells of 25×52 pixels (about
// 2500×1400), turned, on the card and on the processor.
func TestDesign3DGPUSpeed(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	m := wordsScene(t, 100, 33)
	for m.boardH() < 27 {
		m.Update(tea.WindowSizeMsg{Width: 100, Height: m.height + 1})
	}
	withPixels(t, m, 25, 52)
	room3DGPU = true
	ok := gpuReady()
	room3DGPU = false
	if !ok {
		t.Skip("no graphics card: " + gpuRoom.why)
	}
	r := &m.ds().room
	for _, gpu := range []bool{false, true} {
		var total time.Duration
		const n = 30
		for i := range n {
			r.yaw, r.pitch = 20+i, 15
			_, _, _, d := gpuPicture(t, m, gpu)
			total += d
		}
		t.Logf("gpu %v: %dx%d, %.1f ms a frame", gpu, r.pw, r.ph, float64(total)/n/1e6)
	}
}
