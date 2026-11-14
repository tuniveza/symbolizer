//go:build linux && cgo

package main

import (
	"testing"
	"time"
)

// worldGPU is the world room ready for the card, or skips.
func worldGPU(t *testing.T, cw, ch int) *Model {
	m := worldRoom(t)
	withPixels(t, m, cw, ch)
	room3DGPU = true
	ok := gpuReady()
	room3DGPU = false
	if !ok {
		t.Skip("no graphics card: " + gpuRoom.why)
	}
	return m
}

// freePicture is gpuPicture with the first-person camera at pos, looking (yaw, pitch).
func freePicture(t *testing.T, m *Model, pos r3vec, yaw, pitch float64, gpu bool) ([]byte, int, int, time.Duration) {
	r := &m.ds().room
	r.pos, r.look = pos, [2]float64{yaw, pitch}
	return gpuPicture(t, m, gpu)
}

// The windows, the town through them (day, dusk and night, far and close)
// and the extra lights are the same on the card as on the processor.
func TestDesign3DWorldGPUMatches(t *testing.T) {
	m := worldGPU(t, 10, 20)
	start := m.ds().room.pos
	for _, v := range []struct {
		name       string
		hh, mm     int
		pos        r3vec
		yaw, pitch float64
	}{
		{"day", 12, 30, start.add(r3vec{60, 0, -30}), -90, 0},
		{"dusk", 16, 50, start.add(r3vec{60, 0, -30}), -90, 0},
		{"night", 22, 0, start.add(r3vec{60, 0, -30}), -90, 0},
		{"sky", 13, 0, start.add(r3vec{60, -20, -30}), -90, -25},
		{"stars", 23, 0, start.add(r3vec{-30, -25, -40}), -95, -30},
		{"close", 13, 0, start.add(r3vec{-30, -5, -40}), -95, 5},
		{"behind", 9, 0, start, 180, 0},
		{"lights", 22, 0, start.add(r3vec{0, 0, 60}), 0, 8},
	} {
		atClock(t, v.hh, v.mm)
		cpu, pw, ph, _ := freePicture(t, m, v.pos, v.yaw, v.pitch, false)
		gpu, gw, gh, _ := freePicture(t, m, v.pos, v.yaw, v.pitch, true)
		if pw != gw || ph != gh || len(cpu) != len(gpu) || len(cpu) == 0 {
			t.Fatalf("%s: sizes cpu %dx%d gpu %dx%d", v.name, pw, ph, gw, gh)
		}
		saveWorldPNG(t, "gpu-"+v.name, gpu, gw, gh)
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
		t.Logf("%s: mean difference %.2f, %.3f%% far apart", v.name, mean, 100*float64(far)/float64(n))
		if mean > 2 || float64(far) > 0.005*float64(n) {
			t.Errorf("%s: the card's picture differs: mean %.2f, %d of %d pixels far apart", v.name, mean, far, n)
		}
	}
}

// How long a frame takes on the owner's screen (a board of 100×27 cells
// of 25×52 pixels) looking at the back wall and looking out of a window.
func TestDesign3DWorldGPUSpeed(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	m := worldGPU(t, 25, 52)
	atClock(t, 22, 0)
	start := m.ds().room.pos
	for _, v := range []struct {
		name string
		pos  r3vec
		yaw  float64
	}{{"back wall", start, 0}, {"window", start.add(r3vec{60, 0, -30}), -90}} {
		for _, gpu := range []bool{false, true} {
			var total time.Duration
			const n = 30
			for i := range n {
				_, _, _, d := freePicture(t, m, v.pos, v.yaw+float64(i%3), 0, gpu)
				total += d
			}
			r := &m.ds().room
			t.Logf("%s, gpu %v: %dx%d, %.1f ms a frame", v.name, gpu, r.pw, r.ph, float64(total)/n/1e6)
		}
	}
}
