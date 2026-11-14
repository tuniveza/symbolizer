package main

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// atClock sets the clock the sky follows to hh:mm on 14 November 2026
// (the people outside stand still: the clock does not move).
func atClock(t testing.TB, hh, mm int) {
	was := room3DClock
	at := time.Date(2026, 11, 14, hh, mm, 0, 0, time.Local)
	room3DClock = func() time.Time { return at }
	t.Cleanup(func() { room3DClock = was })
}

// worldRoom is the words scene in a kitty picture, gone first person.
func worldRoom(t testing.TB) *Model {
	m := wordsScene(t, 100, 33)
	for m.boardH() < 27 {
		m.Update(tea.WindowSizeMsg{Width: 100, Height: m.height + 1})
	}
	withPixels(t, m, 10, 20)
	press(m, "ctrl+f")
	boardRows(m)
	return m
}

// worldShot draws the room with the camera at pos looking (yaw, pitch)
// and is its picture.
func worldShot(t testing.TB, m *Model, pos r3vec, yaw, pitch float64) ([]byte, int, int) {
	r := &m.ds().room
	r.pos, r.look = pos, [2]float64{yaw, pitch}
	var pic []byte
	var pw, ph int
	room3DSeeRGB = func(rgb []byte, w, h int) { pic, pw, ph = append([]byte(nil), rgb...), w, h }
	defer func() { room3DSeeRGB = nil }()
	r.frame = ""
	boardRows(m)
	if pic == nil {
		t.Fatal("no picture drawn")
	}
	return pic, pw, ph
}

// saveWorldPNG keeps a picture in $SY_3D_PNGS (when set), to look at.
func saveWorldPNG(t testing.TB, name string, rgb []byte, w, h int) {
	dir := os.Getenv("SY_3D_PNGS")
	if dir == "" {
		return
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range w * h {
		img.Pix[4*i], img.Pix[4*i+1], img.Pix[4*i+2], img.Pix[4*i+3] = rgb[3*i], rgb[3*i+1], rgb[3*i+2], 255
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// The views the owner looks at, kept as pictures when SY_3D_PNGS is set.
func TestDesign3DWorldPictures(t *testing.T) {
	m := worldRoom(t)
	r := &m.ds().room
	start := r.pos
	for _, s := range []struct {
		name   string
		hh, mm int
		pos    r3vec
		yaw    float64
		pitch  float64
	}{
		{"day-left", 12, 30, start.add(r3vec{60, 0, -30}), -90, 0},
		{"dusk-left", 16, 50, start.add(r3vec{60, 0, -30}), -90, 0},
		{"night-left", 22, 0, start.add(r3vec{60, 0, -30}), -90, 0},
		{"day-behind", 11, 0, start, 180, 0},
		{"close-window", 13, 0, start.add(r3vec{-30, -5, -40}), -95, 5},
		{"night-close", 21, 30, start.add(r3vec{-30, -5, -40}), -95, 5},
		{"lights", 22, 0, start, 0, 0},
		{"lights-turned", 22, 0, start.add(r3vec{0, 0, -30}), 30, 10},
	} {
		atClock(t, s.hh, s.mm)
		pic, w, h := worldShot(t, m, s.pos, s.yaw, s.pitch)
		saveWorldPNG(t, s.name, pic, w, h)
	}
}

// The extra lights each colour the room their way: the warm lamp on the
// right adds more red than blue, the magenta strip on the left more red
// and blue than green.
func TestDesign3DWorldLightColours(t *testing.T) {
	atClock(t, 22, 0)
	m := worldRoom(t)
	r := &m.ds().room
	pos := r.pos.add(r3vec{0, 0, 60})
	with, w, h := worldShot(t, m, pos, 0, 8)
	xlightsOff = true
	without, _, _ := worldShot(t, m, pos, 0, 8)
	xlightsOff = false
	gain := func(x0, x1 int) (dr, dg, db float64) {
		n := 0.0
		for y := h / 2; y < h; y++ {
			for x := x0; x < x1; x++ {
				i := 3 * (y*w + x)
				dr += float64(with[i]) - float64(without[i])
				dg += float64(with[i+1]) - float64(without[i+1])
				db += float64(with[i+2]) - float64(without[i+2])
				n++
			}
		}
		return dr / n, dg / n, db / n
	}
	lr, lg, lb := gain(0, w/5)
	rr, rg, rb := gain(4*w/5, w)
	t.Logf("left gains %.1f %.1f %.1f, right gains %.1f %.1f %.1f", lr, lg, lb, rr, rg, rb)
	if lr < 3 || lb < 3 || lg > lb {
		t.Errorf("left: the magenta light should add red and blue over green: %.1f %.1f %.1f", lr, lg, lb)
	}
	if rr < 3 || rb > rr*0.7 {
		t.Errorf("right: the warm lamp should add red over blue: %.1f %.1f %.1f", rr, rg, rb)
	}
	saveWorldPNG(t, "lights-wide", with, w, h)
	saveWorldPNG(t, "lights-off", without, w, h)
}

// In letters mode (no pictures) the town's letters are the terminal's
// own: the people, the signs and the windows show as letters in the
// window's cells, and the town's colours fill the rest.
func TestDesign3DWorldLetters(t *testing.T) {
	atClock(t, 12, 30)
	m := wordsScene(t, 100, 33)
	press(m, "ctrl+f")
	boardRows(m)
	r := &m.ds().room
	r.pos, r.look = r.pos.add(r3vec{-30, -5, -40}), [2]float64{-95, 5}
	r.frame = ""
	rows := room3DText(m)
	if r.pix {
		t.Fatal("want letters mode")
	}
	town := 0
	for i, ch := range r.chars {
		if ch.stamp == r.stamp && ch.face >= 0 && r.faces[ch.face].mat == matWindow {
			town++
			if ch.s == "" || ch.s == " " {
				t.Fatalf("cell %d: an empty town letter", i)
			}
		}
	}
	t.Logf("%d town letters\n%s", town, strings.Join(rows, "\n"))
	if town < 5 {
		t.Fatalf("%d of the town's letters in the window's cells, want several", town)
	}
}

// The sky follows the clock: dark with stars at night, bright by day,
// the sun in the east (left, −x) in the morning and the west in the
// evening, and the daylight through the windows brighter by day.
func TestDesign3DWorldSky(t *testing.T) {
	at := func(hh, mm int) r3sky { return skyAt(time.Date(2026, 6, 21, hh, mm, 0, 0, time.Local)) }
	noon, night, morning, evening := at(12, 30), at(1, 0), at(8, 0), at(19, 0)
	if noon.day < 0.95 || night.day > 0.05 || night.stars < 0.95 || noon.stars > 0.05 {
		t.Fatalf("noon day %.2f stars %.2f, night day %.2f stars %.2f", noon.day, noon.stars, night.day, night.stars)
	}
	if morning.sun.x >= 0 || evening.sun.x <= 0 || morning.sun.y <= 0 || evening.sun.y <= 0 {
		t.Fatalf("sun at 8:00 %+v, at 19:00 %+v: want up, east then west", morning.sun, evening.sun)
	}
	lum := func(c uint32) float64 { r, g, b := chans(c); return 0.3*r + 0.59*g + 0.11*b }
	if lum(noon.top) < 2*lum(night.top) || noon.light[0] < 4*night.light[0] {
		t.Fatalf("noon sky %06x light %v, night sky %06x light %v", noon.top, noon.light, night.top, night.light)
	}
	// In November the sun sets earlier: at 16:50 it is dusk, warm at the horizon.
	dusk := skyAt(time.Date(2026, 11, 14, 16, 50, 0, 0, time.Local))
	if hr, _, hb := chans(dusk.horizon); hr < hb+40 {
		t.Fatalf("dusk horizon %06x: want warm", dusk.horizon)
	}
}

// The glass is not lit by the room's lights (the town is as bright as it
// is outside); the frame is.
func TestDesign3DWorldGlassUnlit(t *testing.T) {
	atClock(t, 12, 30)
	m := worldRoom(t)
	r := &m.ds().room
	worldShot(t, m, r.pos.add(r3vec{60, 0, -30}), -90, 0)
	glass, frame := 0, 0
	for i, fi := range r.face {
		if fi < 0 || r.faces[fi].mat != matWindow {
			continue
		}
		if r.mul[i] == 1 && r.add[i] == 0 && r.world.tint[i] == 0 {
			glass++
		} else {
			frame++
		}
	}
	if glass < len(r.face)/4 || frame == 0 {
		t.Fatalf("%d glass pixels, %d frame pixels of %d", glass, frame, len(r.face))
	}
}

// While a window shows the room is drawn again and again (the people walk
// on); looking away from the windows, it is not.
func TestDesign3DWorldTicks(t *testing.T) {
	atClock(t, 12, 30)
	m := worldRoom(t)
	r := &m.ds().room
	worldShot(t, m, r.pos.add(r3vec{60, 0, -30}), -90, 0)
	if !r.windowShown() {
		t.Fatal("looking at the left wall: no window shown")
	}
	if m.outsideCmd() == nil || !r.world.tick {
		t.Fatal("a window shows: want a tick to draw again")
	}
	if m.outsideCmd() != nil {
		t.Fatal("a second tick while one is on its way")
	}
	if _, cmd := m.Update(room3DOutsideMsg{}); cmd == nil || !r.world.tick {
		t.Fatal("the tick came, the window still shows: want the next one asked for")
	}
	r.world.tick = false
	worldShot(t, m, r.startPos, 0, 0)
	if r.windowShown() || m.outsideCmd() != nil {
		t.Fatal("looking at the back wall: no window, want no tick")
	}
	// The people walk: a second later they are elsewhere.
	w := &r.world
	w.walkers(1000)
	a := w.walk[3].x
	w.walkers(1001)
	if d := absF(w.walk[3].x - a); d < 0.1*w.h || d > w.h {
		t.Fatalf("a walker moved %.2f in a second (a storey is %.1f)", d, w.h)
	}
}

// The town's letters all fit the card's table, every figure is three rows
// of three, and the font has every letter.
func TestDesign3DWorldTables(t *testing.T) {
	if len(outChars) > 64 {
		t.Fatalf("%d letters, the card has room for 64", len(outChars))
	}
	if len(figIdx) != 9*len(walkerFigs) {
		t.Fatalf("figures: %d letters for %d figures", len(figIdx), len(walkerFigs))
	}
	missing := 0
	for i, g := range outGlyphsOnce() {
		if i > 0 && g == nil {
			t.Logf("no letter %q in the font", outChars[i])
			missing++
		}
	}
	if missing > 0 {
		t.Errorf("%d of the town's letters are not in the font", missing)
	}
}
