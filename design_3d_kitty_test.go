package main

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"hash/maphash"
	"io"
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
		r.camWas = r.camKey(w, h) // still: all the pixels
		r.movedAt = time.Time{}
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
		r.camWas, r.movedAt = r.camKey(m.width, m.boardH()), time.Time{}
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

// In the program: a picture goes to kitty with each drawing, a RawMsg
// coming back does not draw the room again, a still camera asks for a
// sharp drawing, and going flat deletes the picture.
func TestKittyInTheProgram(t *testing.T) {
	sent := make(chan string, 64)
	startKitty(func(s string) { sent <- s })
	remote := room3DRemote
	room3DRemote = true // the pixels in the code: no files left behind by the test
	t.Cleanup(func() { room3DRemote = remote })
	m := wordsScene(t, 100, 33)
	withPixels(t, m, 10, 20)
	r := &m.ds().room
	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight, Mod: tea.ModCtrl}))
	if !r.on() {
		t.Fatal("not turned")
	}
	if cmd == nil || !r.sharpTick {
		t.Fatal("the camera moved: want a tick for the sharp drawing")
	}
	m.View()
	if !r.pix || !r.lowRes {
		t.Fatalf("moving: want a picture with fewer pixels (pix %v, low %v)", r.pix, r.lowRes)
	}
	select {
	case s := <-sent:
		if !strings.HasPrefix(strings.TrimPrefix(s, syncStart), "\x1b_Ga=T,U=1,") {
			t.Fatalf("want a picture sent: %.40q", s)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no picture sent")
	}
	stamp := r.stamp
	m.Update(tea.RawMsg{Msg: "x"})
	m.View()
	if r.stamp != stamp {
		t.Fatal("a RawMsg drew the room again")
	}
	r.movedAt = time.Now().Add(-time.Second) // still for a while
	m.Update(room3DSharpMsg{})
	m.View()
	if !r.pix || r.lowRes || r.pw != 10*m.width {
		t.Fatalf("still: want all the pixels, got %d across (low %v)", r.pw, r.lowRes)
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
