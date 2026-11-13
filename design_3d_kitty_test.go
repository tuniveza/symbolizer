package main

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"hash/maphash"
	"io"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// withPixels turns the 3D room's kitty picture on for a test, with cells
// cw×ch pixels for a window of the model's size.
func withPixels(t testing.TB, m *Model, cw, ch int) {
	was := room3DPixels
	room3DPixels = true
	termCellAt = struct{ cols, rows, cw, ch int }{m.width, m.height, cw, ch}
	t.Cleanup(func() { room3DPixels, termCellAt = was, struct{ cols, rows, cw, ch int }{} })
}

// A placeholder cell is one cell wide for Bubble Tea's renderer (either
// way it counts), Lip Gloss and the compositor that draws menus over the
// board.
func TestKittyPlaceholderWidth(t *testing.T) {
	cell := string(placeholder(nil, 3, 296))
	line := "\x1b[38;2;1;2;3m" + cell + cell + "\x1b[mAB"
	if w := ansi.StringWidth(cell); w != 1 {
		t.Fatalf("ansi width %d, want 1", w)
	}
	if w := lipgloss.Width(line); w != 4 {
		t.Fatalf("lipgloss width %d, want 4", w)
	}
	for _, method := range []ansi.Method{ansi.WcWidth, ansi.GraphemeWidth} {
		buf := uv.NewScreenBuffer(6, 1)
		buf.Method = method
		uv.NewStyledString(line).Draw(buf, buf.Bounds())
		for x, want := range []string{cell, cell, "A", "B"} {
			if c := buf.CellAt(x, 0); c.Content != want || c.Width != 1 {
				t.Fatalf("method %v, cell %d: %q width %d, want %q width 1", method, x, c.Content, c.Width, want)
			}
		}
	}
	// A menu drawn over it keeps the placeholders round it whole.
	out := lipgloss.NewCompositor(lipgloss.NewLayer(line), lipgloss.NewLayer("X").X(1).Z(1)).Render()
	if !strings.Contains(out, cell) || !strings.Contains(out, "X") || ansi.StringWidth(out) != 4 {
		t.Fatalf("over a menu: %q", out)
	}
}

// Each row of the board area is placeholders that say their row and
// column, in the picture's colour, with the words' letters in between.
func TestKittyPlaceholderRows(t *testing.T) {
	m := wordsScene(t, 100, 33)
	withPixels(t, m, 10, 20)
	r := &m.ds().room
	r.yaw, r.pitch = 20, 10
	r.looked, r.plain = true, false // P: words as letters over the picture
	rows := boardRows(m)
	if !r.pix || r.pw <= 2*m.width {
		t.Fatalf("not a picture: pix %v, %d×%d pixels", r.pix, r.pw, r.ph)
	}
	if len(rows) != m.boardH() {
		t.Fatalf("%d rows, want %d", len(rows), m.boardH())
	}
	id := room3DImageID
	letters := 0
	for y, line := range rows {
		if w := ansi.StringWidth(line); w != m.width {
			t.Fatalf("row %d is %d cells wide, want %d", y, w, m.width)
		}
		buf := uv.NewScreenBuffer(m.width, 1)
		uv.NewStyledString(line).Draw(buf, buf.Bounds())
		for x := 0; x < m.width; x++ {
			c := buf.CellAt(x, 0)
			if !strings.HasPrefix(c.Content, "\U0010EEEE") {
				letters++
				continue
			}
			if c.Content != string(placeholder(nil, y, x)) {
				t.Fatalf("row %d, column %d: %q, want its row and column", y, x, c.Content)
			}
			if got := colour32(c.Style.Fg, 0); got != id {
				t.Fatalf("row %d, column %d: colour %06x, want the picture's number %06x", y, x, got, id)
			}
			if c.Style.Bg != nil {
				t.Fatalf("row %d, column %d: a background on a placeholder", y, x)
			}
		}
	}
	if letters == 0 {
		t.Fatal("no words drawn over the picture")
	}
	if !titleShows(room3DText(m), "Notes") {
		t.Fatal("a title does not show over the picture")
	}
}

// Picking a cell with the mouse finds the same thing whether a cell is
// four pixels (quarter blocks) or 10×20 (a picture).
func TestKittyPickSameCell(t *testing.T) {
	m := wordsScene(t, 100, 33)
	r := &m.ds().room
	w, h := m.width, m.boardH()
	for _, a := range [][2]int{{20, 10}, {-35, 25}, {0, 30}} {
		r.yaw, r.pitch = a[0], a[1]
		room3DPixels = false
		boardRows(m)
		if r.pw != 2*w {
			t.Fatalf("quarter blocks: %d pixels across, want %d", r.pw, 2*w)
		}
		type pick struct {
			own, sx, sy int
			flat        bool
		}
		quad := make([]pick, w*h)
		for i := range quad {
			hit := r.pickAt(i%w, i/w)
			quad[i] = pick{hit.own, hit.sx, hit.sy, hit.flat}
		}
		withPixels(t, m, 10, 20)
		boardRows(m)
		if r.pw != 10*w || r.ph != 20*h {
			t.Fatalf("picture: %d×%d pixels, want %d×%d", r.pw, r.ph, 10*w, 20*h)
		}
		same, blocks := 0, 0
		for i := range quad {
			hit := r.pickAt(i%w, i/w)
			p := pick{hit.own, hit.sx, hit.sy, hit.flat}
			if p == quad[i] {
				same++
			}
			if quad[i].own >= 0 {
				blocks++
			}
		}
		if blocks == 0 {
			t.Fatalf("%v: no block picked", a)
		}
		if float64(same) < 0.98*float64(len(quad)) {
			t.Fatalf("%v: %d of %d cells pick the same, want 98%%", a, same, len(quad))
		}
		room3DPixels = false
	}
	// The middle of a block's front, at both.
	r.yaw, r.pitch = 15, 5
	boardRows(m)
	col, row := cell3D(t, m, m.ds().octs[2].ID, func(int, int) bool { return true })
	q := r.pickAt(col, row)
	withPixels(t, m, 10, 20)
	boardRows(m)
	if p := r.pickAt(col, row); p.own != q.own || p.sx != q.sx || p.sy != q.sy {
		t.Fatalf("the block's cell: %+v as a picture, %+v in quarter blocks", p, q)
	}
}

// Not kitty (or turned off), the room is quarter blocks as before; kitty
// (or turned on) a picture.
func TestKittyWanted(t *testing.T) {
	env := func(kv ...string) func(string) string {
		return func(k string) string {
			for i := 0; i+1 < len(kv); i += 2 {
				if kv[i] == k {
					return kv[i+1]
				}
			}
			return ""
		}
	}
	for _, c := range []struct {
		env  func(string) string
		want bool
	}{
		{env(), false},
		{env("TERM", "xterm-256color"), false},
		{env("TERM", "xterm-kitty"), true},
		{env("KITTY_WINDOW_ID", "3"), true},
		{env("KITTY_WINDOW_ID", "3", "SY_3D_PIXELS", "0"), false},
		{env("TERM", "foot", "SY_3D_PIXELS", "1"), true},
	} {
		if got := kittyWanted(c.env); got != c.want {
			t.Errorf("kittyWanted = %v, want %v", got, c.want)
		}
	}
	if kittyRemote(env()) || !kittyRemote(env("SSH_CONNECTION", "1 2 3 4")) {
		t.Error("kittyRemote")
	}
	m := wordsScene(t, 100, 33)
	m.ds().room.yaw = 25
	room3DPixels = false
	if s := strings.Join(boardRows(m), ""); strings.Contains(s, "\U0010EEEE") || !strings.ContainsAny(s, "▀▄▌▐▘▝▖▗") {
		t.Fatal("not kitty: want quarter blocks, no placeholders")
	}
	withPixels(t, m, 9, 19)
	if s := strings.Join(boardRows(m), ""); !strings.Contains(s, "\U0010EEEE") {
		t.Fatal("kitty: want placeholders")
	}
}

// The picture's size: the terminal's pixels, fewer past the limit, never
// fewer than four a cell.
func TestKittyDensity(t *testing.T) {
	if pw, ph := room3DDensity(100, 27, 10, 20, 1_000_000); pw != 1000 || ph != 540 {
		t.Fatalf("under the limit: %d×%d", pw, ph)
	}
	pw, ph := room3DDensity(100, 27, 10, 20, 135_000)
	if pw*ph > 140_000 || pw*ph < 130_000 || pw*540 != ph*1000 && abs(pw*540-ph*1000) > 1000 {
		t.Fatalf("over the limit: %d×%d", pw, ph)
	}
	if pw, ph := room3DDensity(100, 27, 10, 20, 10); pw != 200 || ph != 54 {
		t.Fatalf("tiny limit: %d×%d, want quarter blocks' 200×54", pw, ph)
	}
}

// Over SSH the pixels go in the code, packed, in pieces of 4096 bytes.
func TestKittyDirect(t *testing.T) {
	rgb := make([]byte, 3*120*80)
	x := uint32(1)
	for i := range rgb {
		x = x*1664525 + 1013904223 // noise: it does not pack small
		rgb[i] = byte(x >> 24)
	}
	seq := kittyDirect(&kittyFrame{rgb: rgb, pw: 120, ph: 80, cols: 12, rows: 4})
	if !strings.HasPrefix(seq, syncStart) || !strings.HasSuffix(seq, syncEnd) {
		t.Fatal("want the pieces in one synchronized update")
	}
	seq = strings.TrimSuffix(strings.TrimPrefix(seq, syncStart), syncEnd)
	parts := strings.Split(strings.TrimSuffix(seq, "\x1b\\"), "\x1b\\")
	if len(parts) < 2 {
		t.Fatalf("%d pieces, want several", len(parts))
	}
	var data strings.Builder
	for i, p := range parts {
		keys, payload, _ := strings.Cut(strings.TrimPrefix(p, "\x1b_G"), ";")
		if len(payload) > 4096 {
			t.Fatalf("piece %d: %d bytes", i, len(payload))
		}
		last := i == len(parts)-1
		switch {
		case i == 0 && !strings.Contains(keys, "a=T,U=1,") || i == 0 && !strings.Contains(keys, "o=z"):
			t.Fatalf("first piece: %q", keys)
		case i > 0 && strings.Contains(keys, "a="):
			t.Fatalf("piece %d has more than m and q: %q", i, keys)
		case last != strings.HasSuffix(keys, "m=0"):
			t.Fatalf("piece %d: %q", i, keys)
		}
		data.WriteString(payload)
	}
	packed, err := base64.StdEncoding.DecodeString(data.String())
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zlib.NewReader(bytes.NewReader(packed))
	if err != nil {
		t.Fatal(err)
	}
	back, _ := io.ReadAll(zr)
	if !bytes.Equal(back, rgb) {
		t.Fatal("the pixels do not come back")
	}
}

// The sender sends the newest picture only, and a delete drops a picture
// not sent yet.
func TestKittySender(t *testing.T) {
	var k kittyOut
	sent := make(chan string, 10)
	block := make(chan struct{})
	k.send = func(s string) { <-block; sent <- s }
	k.dir, k.seed = t.TempDir(), maphash.MakeSeed()
	k.done = make(chan struct{})
	k.wake = sync.NewCond(&k.mu)
	go k.loop()
	n := byte(0)
	frame := func() *kittyFrame {
		n++
		return &kittyFrame{rgb: []byte{n, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, pw: 2, ph: 2, cols: 1, rows: 1}
	}
	k.show(frame()) // being sent: held at block
	time.Sleep(20 * time.Millisecond)
	k.show(frame()) // dropped by the next
	k.show(frame())
	close(block)
	first, second := <-sent, <-sent
	if !strings.Contains(first, "t=t") || !strings.Contains(second, "t=t") {
		t.Fatalf("want two pictures: %q %q", first, second)
	}
	select {
	case s := <-sent:
		t.Fatalf("a dropped picture was sent: %q", s)
	case <-time.After(30 * time.Millisecond):
	}
	k.remove()
	if s := <-sent; !strings.Contains(s, "a=d,d=I") {
		t.Fatalf("want a delete: %q", s)
	}
	var out bytes.Buffer
	k.stop(&out)
	if !strings.Contains(out.String(), "a=d,d=I") {
		t.Fatalf("closing: want a delete, got %q", out.String())
	}
}

// pixelBench is a board of 100×27 cells (as the owner's terminal) with
// 10×20 pixel cells, as a picture, turned.
func pixelBench(b *testing.B) *Model {
	m := wordsScene(b, 100, 33)
	for m.boardH() < 27 {
		m.Update(tea.WindowSizeMsg{Width: 100, Height: m.height + 1})
	}
	withPixels(b, m, 10, 20)
	return m
}

// A still frame: all the pixels (1000×540).
func BenchmarkDesign3DPixelsStill(b *testing.B) {
	m := pixelBench(b)
	r := &m.ds().room
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.yaw, r.pitch = 35+i%2, 20
		var sb strings.Builder
		m.designRows(&sb)
	}
	b.ReportMetric(float64(r.cost)/float64(time.Millisecond), "ms/last")
	b.ReportMetric(float64(r.pw*r.ph), "pixels")
}

// A frame while the camera moves: fewer pixels.
func BenchmarkDesign3DPixelsMoving(b *testing.B) {
	m := pixelBench(b)
	r := &m.ds().room
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.yaw, r.pitch = 20+i%20, 20
		var sb strings.Builder
		m.designRows(&sb)
	}
	b.ReportMetric(float64(r.cost)/float64(time.Millisecond), "ms/last")
	b.ReportMetric(float64(r.pw*r.ph), "pixels")
}

// In the program: a picture goes to kitty with each drawing, all the
// pixels even while the camera moves, a RawMsg coming back does not draw
// the room again, a key straight after a drawing is drawn a moment later
// (with a tick that asks for it), and going flat deletes the picture.
func TestKittyInTheProgram(t *testing.T) {
	sent := make(chan string, 64)
	startKitty(func(s string) { sent <- s }, func() {})
	remote, pace := room3DRemote, room3DPace
	room3DRemote, room3DPace = true, true // the pixels in the code: no files left behind by the test
	t.Cleanup(func() { room3DRemote, room3DPace = remote, pace })
	m := wordsScene(t, 100, 33)
	withPixels(t, m, 10, 20)
	r := &m.ds().room
	press(m, "ctrl+f")
	if !r.on() {
		t.Fatal("Ctrl+F: not 3D")
	}
	m.View()
	if !r.pix || r.pw != 10*m.width {
		t.Fatalf("moving: want all the pixels, got %d across (pix %v)", r.pw, r.pix)
	}
	select {
	case s := <-sent:
		if !strings.HasPrefix(strings.TrimPrefix(s, syncStart), "\x1b_Ga=T,U=1,") {
			t.Fatalf("want a picture sent: %.40q", s)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no picture sent")
	}
	select { // the first picture goes once more: kitty may have drawn the cells before it came
	case s := <-sent:
		if !strings.HasPrefix(strings.TrimPrefix(s, syncStart), "\x1b_Ga=T,U=1,") {
			t.Fatalf("want the first picture sent again: %.40q", s)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the first picture was not sent again")
	}
	stamp := r.stamp
	m.Update(tea.RawMsg{Msg: "x"})
	m.View()
	if r.stamp != stamp {
		t.Fatal("a RawMsg drew the room again")
	}
	r.drawnAt, r.drawnTill = time.Now(), time.Now() // as if it had just been drawn
	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	if cmd == nil || !r.frameTick {
		t.Fatal("drawn a moment ago: want a tick to draw it again")
	}
	m.View()
	if r.stamp != stamp {
		t.Fatal("drawn again at once, not a moment later")
	}
	time.Sleep(room3DEvery)
	m.Update(room3DFrameMsg{})
	m.View()
	if r.stamp == stamp || r.frameTick {
		t.Fatal("the tick did not draw the room again")
	}
	<-sent
	press(m, "esc")
	if r.on() {
		t.Fatal("Esc: not flat")
	}
	for {
		select {
		case s := <-sent:
			if strings.Contains(s, "a=d,d=I") {
				return
			}
		case <-time.After(2 * time.Second):
			t.Fatal("flat: the picture was not deleted")
		}
	}
}

// A picture waits while kitty still takes in the last one: it goes when
// kitty answers, and sy is told it may draw the next. A terminal that
// never answers is not waited for after a few pictures.
func TestKittyWaitsForKitty(t *testing.T) {
	var k kittyOut
	sent := make(chan string, 10)
	woken := make(chan struct{}, 10)
	k.send = func(s string) { sent <- s }
	k.wakeUp = func() { woken <- struct{}{} }
	k.dir, k.seed = t.TempDir(), maphash.MakeSeed()
	k.done = make(chan struct{})
	k.wake = sync.NewCond(&k.mu)
	go k.loop()
	t.Cleanup(func() { k.stop(io.Discard) })
	n := byte(0)
	frame := func() *kittyFrame {
		n++
		return &kittyFrame{rgb: []byte{n, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, pw: 2, ph: 2, cols: 1, rows: 1}
	}
	k.show(frame())
	<-sent
	k.ack()
	k.show(frame())
	<-sent // kitty answered the first: the second goes at once
	k.show(frame())
	if k.ready() {
		t.Fatal("a picture waits to go: not ready for the next")
	}
	select {
	case s := <-sent:
		t.Fatalf("sent before kitty answered: %q", s)
	case <-time.After(kittyAckWait / 2):
	}
	k.ack()
	select {
	case <-sent:
	case <-time.After(kittyAckWait / 2):
		t.Fatal("kitty answered: the waiting picture did not go")
	}
	select {
	case <-woken:
	case <-time.After(time.Second):
		t.Fatal("sy was not told it may draw")
	}
	if !k.ready() {
		t.Fatal("nothing waits now: ready")
	}

	// A terminal that does not answer: each picture waits kittyAckWait at
	// most, and after kittyUnheard of them, not at all.
	var q kittyOut
	q.send = func(s string) { sent <- s }
	q.dir, q.seed = t.TempDir(), maphash.MakeSeed()
	q.done = make(chan struct{})
	q.wake = sync.NewCond(&q.mu)
	go q.loop()
	t.Cleanup(func() { q.stop(io.Discard) })
	start := time.Now()
	for range kittyUnheard + 4 {
		q.show(frame())
		select {
		case <-sent:
		case <-time.After(2 * kittyAckWait):
			t.Fatal("a picture waited past kittyAckWait")
		}
	}
	if d := time.Since(start); d > time.Duration(kittyUnheard+2)*kittyAckWait {
		t.Fatalf("kept waiting for a terminal that does not answer: %v", d)
	}
}

// While the camera moves, a big window's picture has at most
// room3DMovingPixels; once it has stopped, room3DSharpMsg draws it with
// every pixel.
func TestDesign3DLightWhileMoving(t *testing.T) {
	m := wordsScene(t, 200, 60)
	withPixels(t, m, 14, 28) // 2800 × 1600-odd pixels: a 4K-ish window
	r := &m.ds().room
	press(m, "ctrl+f")
	m.View()
	full := m.width * 14 * m.boardH() * 28
	r.movedAt = time.Now().Add(-time.Second) // still a while
	m.Update(room3DSharpMsg{})
	m.View()
	if r.light || r.pw*r.ph != full {
		t.Fatalf("still: want every pixel (%d), got %d×%d (light %v)", full, r.pw, r.ph, r.light)
	}
	r.look[0] += 5 // the camera turns
	m.ds().hoverOnly, r.frame = false, ""
	m.View()
	if !r.light || r.pw*r.ph > room3DMovingPixels*101/100 { // rounded to whole pixels
		t.Fatalf("moving: want at most %d pixels, got %d×%d (light %v)", room3DMovingPixels, r.pw, r.ph, r.light)
	}
	if a, b := float64(r.pw)/float64(r.ph), float64(m.width*14)/float64(m.boardH()*28); math.Abs(a-b) > 0.01 {
		t.Fatalf("the light picture is out of shape: %v, want %v", a, b)
	}
	_, cmd := m.Update(room3DSharpMsg{}) // too soon: still light, asks again
	m.View()
	if !r.light || cmd == nil || !r.sharpTick {
		t.Fatal("just moved: still light, with a tick to sharpen it later")
	}
	r.movedAt = time.Now().Add(-room3DStill)
	r.sharpTick = false
	m.Update(room3DSharpMsg{})
	m.View()
	if r.light || r.pw*r.ph != full {
		t.Fatalf("stopped: want every pixel again (%d), got %d×%d", full, r.pw, r.ph)
	}

	// A small window has every pixel even while moving.
	m2 := wordsScene(t, 80, 30)
	withPixels(t, m2, 10, 20)
	press(m2, "ctrl+f")
	r2 := &m2.ds().room
	m2.View()
	r2.look[0] += 5
	m2.ds().hoverOnly, r2.frame = false, ""
	m2.View()
	if r2.light || r2.pw != 80*10 {
		t.Fatalf("small window moving: want every pixel, got %d across (light %v)", r2.pw, r2.light)
	}
}
