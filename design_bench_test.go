package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// A full-screen board: twenty structures, a long page and some ink.
func designBenchModel(b *testing.B) (*Model, *dOct) {
	b.Setenv("XDG_DATA_HOME", b.TempDir())
	m, _ := NewModel("", 0)
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	m.setMode(designMode)
	d := m.ds()
	d.camX, d.camY = 0, 0
	for i := 0; i < 20; i++ {
		o := d.newKind(kOct, 0, 0)
		o.X, o.Y, o.Header = 5+(i%5)*35, 2+(i/5)*14, "Structure"
	}
	pg := d.newKind(kPage, 0, 0)
	pg.X, pg.Y, pg.W, pg.H, pg.Header = 120, 5, 60, 40, "Long page"
	pg.Body = strings.Repeat("A line of words that is long enough to wrap around the page once or twice.\n", 3000)
	for x := 0; x < 150; x++ {
		d.marks[[2]int{x, 50}] = d.marks[[2]int{x, 50}]
	}
	d.sel, d.editing = pg.ID, partNone
	d.dirty = true
	d.save()
	return m, pg
}

func BenchmarkDesignPageScroll(b *testing.B) {
	m, pg := designBenchModel(b)
	x, y := pg.X+10, pg.Y+10+m.boardTop()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		btn := tea.MouseWheelDown
		if i%200 >= 100 {
			btn = tea.MouseWheelUp
		}
		m.Update(tea.MouseWheelMsg(tea.Mouse{X: x, Y: y, Button: btn}))
		_ = m.View()
	}
}

func BenchmarkDesignZoomScroll(b *testing.B) {
	m, _ := designBenchModel(b)
	m.ds().sel = -1
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		btn := tea.MouseWheelDown
		if i%8 >= 4 {
			btn = tea.MouseWheelUp
		}
		m.Update(tea.MouseWheelMsg(tea.Mouse{X: 100, Y: 30, Button: btn}))
		_ = m.View()
	}
}

// emptyCell is a board cell with nothing on it.
func emptyCell(b *testing.B, m *Model) (int, int) {
	d := m.ds()
	for y := 2; y < m.boardH()-2; y++ {
		for x := 2; x < m.width-2; x++ {
			if d.hit(x, y).o == nil {
				return x, y + m.boardTop()
			}
		}
	}
	b.Fatal("no empty cell")
	return 0, 0
}

func benchDrag(b *testing.B, m *Model, x, y int) {
	benchDragWith(b, m, x, y, tea.MouseLeft)
}

func benchDragWith(b *testing.B, m *Model, x, y int, btn tea.MouseButton) {
	m.Update(tea.MouseClickMsg(tea.Mouse{X: x, Y: y, Button: btn}))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dx := i%40 - 20
		if (i/40)%2 == 1 {
			dx = 20 - i%40
		}
		m.Update(tea.MouseMotionMsg(tea.Mouse{X: x + dx, Y: y, Button: btn}))
		_ = m.View()
	}
}

func BenchmarkDesignPan(b *testing.B) {
	m, _ := designBenchModel(b)
	m.ds().sel = -1
	x, y := emptyCell(b, m)
	benchDrag(b, m, x, y)
}

func BenchmarkDesignMove(b *testing.B) {
	m, _ := designBenchModel(b)
	d := m.ds()
	x, y, _, _ := d.rect(d.octs[6])
	benchDrag(b, m, x+4, y+4+m.boardTop())
}

func BenchmarkDesignTyping(b *testing.B) {
	m, pg := designBenchModel(b)
	m.ds().editing = partBody
	_ = pg
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Update(tea.KeyPressMsg(tea.Key{Code: 'a', Text: "a"}))
		_ = m.View()
	}
}

// A board with a big photo-like picture on it.
func designPicModel(b *testing.B) *Model {
	m, _ := designBenchModel(b)
	d := m.ds()
	dir := d.imagesDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		b.Fatal(err)
	}
	img := image.NewNRGBA(image.Rect(0, 0, 480, 320))
	for y := 0; y < 320; y++ {
		for x := 0; x < 480; x++ {
			img.Set(x, y, color.NRGBA{uint8(x), uint8(y), uint8(x ^ y), 255})
		}
	}
	f, _ := os.Create(filepath.Join(dir, "photo.png"))
	png.Encode(f, img)
	f.Close()
	o := d.newKind(kOct, 0, 0)
	o.X, o.Y, o.W, o.H, o.Header, o.Image, o.Layout = 20, 6, 90, 40, "Photo", "photo.png", layoutPicture
	d.sel = -1
	_ = m.View()
	return m
}

func BenchmarkDesignPicPan(b *testing.B) {
	m := designPicModel(b)
	x, y := emptyCell(b, m)
	benchDrag(b, m, x, y)
}

func BenchmarkDesignPicZoom(b *testing.B) {
	m := designPicModel(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		btn := tea.MouseWheelDown
		if i%8 >= 4 {
			btn = tea.MouseWheelUp
		}
		x, y := emptyCell(b, m)
		m.Update(tea.MouseWheelMsg(tea.Mouse{X: x, Y: y, Button: btn}))
		_ = m.View()
	}
}

type countW struct{ n *int64 }

func (c countW) Write(p []byte) (int, error) { *c.n += int64(len(p)); return len(p), nil }

// What really reaches the terminal while the board is panned: the whole
// program, renderer and all, writing into a counter.
func TestDesignPicTerminalBytes(t *testing.T) {
	if os.Getenv("SY_BYTES") == "" {
		t.Skip("set SY_BYTES=1 to measure")
	}
	b := &testing.B{}
	_ = b
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	for _, pic := range []bool{false, true} {
		m, _ := NewModel("", 0)
		m.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
		m.setMode(designMode)
		d := m.ds()
		d.camX, d.camY = 0, 0
		for i := 0; i < 10; i++ {
			o := d.newKind(kOct, 0, 0)
			o.X, o.Y, o.Header = 5+(i%5)*35, 2+(i/5)*14, "Structure"
		}
		if pic {
			dir := d.imagesDir()
			os.MkdirAll(dir, 0o700)
			img := image.NewNRGBA(image.Rect(0, 0, 480, 320))
			for y := 0; y < 320; y++ {
				for x := 0; x < 480; x++ {
					img.Set(x, y, color.NRGBA{uint8(x), uint8(y), uint8(x ^ y), 255})
				}
			}
			f, _ := os.Create(filepath.Join(dir, "photo.png"))
			png.Encode(f, img)
			f.Close()
			o := d.newKind(kOct, 0, 0)
			o.X, o.Y, o.W, o.H, o.Image, o.Layout = 20, 6, 90, 40, "photo.png", layoutPicture
		}
		d.sel = -1
		var n int64
		p := tea.NewProgram(m, tea.WithInput(nil), tea.WithOutput(countW{&n}), tea.WithWindowSize(200, 60), tea.WithFPS(120))
		done := make(chan struct{})
		go func() { p.Run(); close(done) }()
		time.Sleep(300 * time.Millisecond)
		x, y := 2, 30+m.boardTop()
		start, n0 := time.Now(), n
		p.Send(tea.MouseClickMsg(tea.Mouse{X: x, Y: y, Button: tea.MouseLeft}))
		frames := 0
		for time.Since(start) < 2*time.Second {
			frames++
			p.Send(tea.MouseMotionMsg(tea.Mouse{X: x + frames%40, Y: y, Button: tea.MouseLeft}))
			time.Sleep(8 * time.Millisecond)
		}
		el := time.Since(start)
		p.Quit()
		<-done
		t.Logf("picture %v: %d moves in %v, %.0f KB written, %.0f KB per move", pic, frames, el.Round(time.Millisecond), float64(n-n0)/1024, float64(n-n0)/1024/float64(frames))
	}
}

// The owner's own board, copied to SY_BOARD_HOME (a data folder holding
// symbolizer/design): panned like a hand would.
func BenchmarkDesignOwnBoard(b *testing.B) {
	home := os.Getenv("SY_BOARD_HOME")
	if home == "" {
		b.Skip("set SY_BOARD_HOME")
	}
	b.Setenv("XDG_DATA_HOME", home)
	m, _ := NewModel("", 0)
	m.Update(tea.WindowSizeMsg{Width: 190, Height: 50})
	m.setMode(designMode)
	_ = m.View()
	x, y := emptyCell(b, m)
	benchDrag(b, m, x, y)
}

// The screen size for the owner's-board benchmarks: SY_BENCH_SIZE=240x65.
var benchW, benchH = func() (int, int) {
	w, h := 190, 50
	fmt.Sscanf(os.Getenv("SY_BENCH_SIZE"), "%dx%d", &w, &h)
	return w, h
}()

// The owner's board at every zoom, centred on its picture.
func BenchmarkDesignOwnBoardZoomed(b *testing.B) {
	home := os.Getenv("SY_BOARD_HOME")
	if home == "" {
		b.Skip("set SY_BOARD_HOME")
	}
	for zi := range designZooms {
		b.Run(fmt.Sprint(designZooms[zi]), func(b *testing.B) {
			b.Setenv("XDG_DATA_HOME", home)
			m, _ := NewModel("", 0)
			m.Update(tea.WindowSizeMsg{Width: benchW, Height: benchH})
			m.setMode(designMode)
			d := m.ds()
			var pic *dOct
			for _, o := range d.octs {
				if o.Image != "" {
					pic = o
				}
			}
			d.zoom = zi
			z := d.z()
			d.camX = float64(pic.X+pic.W/2) - float64(benchW/2)/z
			d.camY = float64(pic.Y+pic.H/2) - float64(benchH/2)/z
			_ = m.View()
			benchDragWith(b, m, benchW/2, benchH/2, tea.MouseMiddle)
		})
	}
}

// The owner's board, zoomed in and out with the wheel over its picture
// (each step is a new size, so it is drawn anew).
func BenchmarkDesignOwnBoardWheel(b *testing.B) {
	home := os.Getenv("SY_BOARD_HOME")
	if home == "" {
		b.Skip("set SY_BOARD_HOME")
	}
	for _, lo := range []int{0, 5, 7} {
		b.Run(fmt.Sprint(designZooms[lo], "-", designZooms[lo+2]), func(b *testing.B) {
			b.Setenv("XDG_DATA_HOME", home)
			m, _ := NewModel("", 0)
			m.Update(tea.WindowSizeMsg{Width: 190, Height: 50})
			m.setMode(designMode)
			d := m.ds()
			var pic *dOct
			for _, o := range d.octs {
				if o.Image != "" {
					pic = o
				}
			}
			d.zoom = lo
			z := d.z()
			d.camX = float64(pic.X+pic.W/2) - 95/z
			d.camY = float64(pic.Y+pic.H/2) - 25/z
			_ = m.View()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				btn := tea.MouseWheelUp
				if i%4 >= 2 {
					btn = tea.MouseWheelDown
				}
				m.Update(tea.MouseWheelMsg(tea.Mouse{X: 95, Y: 25 + m.boardTop(), Button: btn}))
				_ = m.View()
			}
		})
	}
}

type doneMsg struct{}

type timedModel struct {
	*Model
	done chan struct{}
}

func (t timedModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(doneMsg); ok {
		close(t.done)
		return t, nil
	}
	_, cmd := t.Model.Update(msg)
	return t, cmd
}

// The whole program, renderer and all, on the owner's board full screen
// and zoomed in on its picture: how long a burst of drags takes to be
// taken in, and how much goes to the terminal.
func TestDesignOwnBoardWholeProgram(t *testing.T) {
	home := os.Getenv("SY_BOARD_HOME")
	if home == "" {
		t.Skip("set SY_BOARD_HOME")
	}
	t.Setenv("XDG_DATA_HOME", home)
	designFrames = os.Getenv("SY_FRAMES") != ""
	defer func() { designFrames = false }()
	for _, zi := range []int{0, 5, 9} {
		for _, size := range [][2]int{{83, 37}, {240, 65}} {
			m, _ := NewModel("", 0)
			m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			m.setMode(designMode)
			d := m.ds()
			var pic *dOct
			for _, o := range d.octs {
				if o.Image != "" {
					pic = o
				}
			}
			d.zoom = zi
			z := d.z()
			d.camX = float64(pic.X+pic.W/2) - float64(size[0]/2)/z
			d.camY = float64(pic.Y+pic.H/2) - float64(size[1]/2)/z
			var n int64
			tm := timedModel{m, make(chan struct{})}
			p := tea.NewProgram(tm, tea.WithInput(nil), tea.WithOutput(countW{&n}), tea.WithWindowSize(size[0], size[1]), tea.WithFPS(120))
			fin := make(chan struct{})
			go func() { p.Run(); close(fin) }()
			time.Sleep(300 * time.Millisecond)
			x, y := size[0]/2, size[1]/2
			n0, start := n, time.Now()
			p.Send(tea.MouseClickMsg(tea.Mouse{X: x, Y: y, Button: tea.MouseMiddle}))
			const moves = 200
			for i := 0; i < moves; i++ {
				p.Send(tea.MouseMotionMsg(tea.Mouse{X: x + i%30 - 15, Y: y + i%10 - 5, Button: tea.MouseMiddle}))
			}
			p.Send(doneMsg{})
			<-tm.done
			el := time.Since(start)
			time.Sleep(100 * time.Millisecond)
			p.Quit()
			<-fin
			if designFrames {
				last := d.frame
				designFrames = false
				var sb strings.Builder
				m.designRows(&sb)
				designFrames = true
				if sb.String() != last {
					t.Errorf("zoom %.2f %v: the last frame drawn is not the board as it ended", designZooms[zi], size)
				}
			}
			t.Logf("zoom %.2f %dx%d: %v per move, %.0f KB to the terminal in all (%.0f KB a frame at most 120/s)", designZooms[zi], size[0], size[1],
				(el / moves).Round(10*time.Microsecond), float64(n-n0)/1024, float64(n-n0)/1024/max(1, el.Seconds()*120))
		}
	}
}

// Bytes one pan step sends to the terminal, full screen over the picture.
func TestDesignOwnBoardFrameBytes(t *testing.T) {
	home := os.Getenv("SY_BOARD_HOME")
	if home == "" {
		t.Skip("set SY_BOARD_HOME")
	}
	t.Setenv("XDG_DATA_HOME", home)
	m, _ := NewModel("", 0)
	m.Update(tea.WindowSizeMsg{Width: 240, Height: 65})
	m.setMode(designMode)
	d := m.ds()
	var pic *dOct
	for _, o := range d.octs {
		if o.Image != "" {
			pic = o
		}
	}
	d.zoom = 9
	if z := os.Getenv("SY_ZOOM"); z != "" {
		fmt.Sscan(z, &d.zoom)
	}
	d.camX, d.camY = float64(pic.X+pic.W/2)-120/d.z(), float64(pic.Y+pic.H/2)-32/d.z()
	var n int64
	p := tea.NewProgram(m, tea.WithInput(nil), tea.WithOutput(countW{&n}), tea.WithWindowSize(240, 65))
	fin := make(chan struct{})
	go func() { p.Run(); close(fin) }()
	time.Sleep(300 * time.Millisecond)
	for _, dx := range []int{1, 1, 3, 0} {
		n0 := n
		p.Send(tea.MouseClickMsg(tea.Mouse{X: 100, Y: 30, Button: tea.MouseMiddle}))
		p.Send(tea.MouseMotionMsg(tea.Mouse{X: 100 + dx, Y: 30 + dx, Button: tea.MouseMiddle}))
		p.Send(tea.MouseReleaseMsg(tea.Mouse{X: 100 + dx, Y: 30 + dx, Button: tea.MouseMiddle}))
		time.Sleep(100 * time.Millisecond)
		t.Logf("pan by %d cells: %.0f KB to the terminal", dx, float64(n-n0)/1024)
	}
	p.Quit()
	<-fin
}

// A large project: 400 structures, 20 pictures, 60 long pages, 10 cabinets
// of 40 pages each, 150 links and 3000 pen marks, on a 240×65 screen.
func bigBoard(b *testing.B) *Model {
	b.Setenv("XDG_DATA_HOME", b.TempDir())
	m, _ := NewModel("", 0)
	m.Update(tea.WindowSizeMsg{Width: 240, Height: 65})
	m.setMode(designMode)
	d := m.ds()
	dir := d.imagesDir()
	os.MkdirAll(dir, 0o700)
	for p := 0; p < 20; p++ {
		img := image.NewNRGBA(image.Rect(0, 0, 480, 320))
		for y := 0; y < 320; y++ {
			for x := 0; x < 480; x++ {
				img.Set(x, y, color.NRGBA{uint8(x + p*13), uint8(y * p), uint8(x ^ y), 255})
			}
		}
		f, _ := os.Create(filepath.Join(dir, fmt.Sprintf("p%d.png", p)))
		png.Encode(f, img)
		f.Close()
	}
	text := strings.Repeat("A line of words in a textual universe, long enough to wrap around.\n", 450) // ~30 KB
	rnd := 0
	next := func(n int) int { rnd = (rnd*1103515245 + 12345) & 0x7fffffff; return rnd % n }
	for i := 0; i < 400; i++ {
		var o *dOct
		switch {
		case i < 20:
			o = d.newKind(kOct, 0, 0)
			o.Image, o.Layout, o.W, o.H = fmt.Sprintf("p%d.png", i), layoutPicture, 60, 40
		case i < 80:
			o = d.newKind(kPage, 0, 0)
			o.Body, o.W, o.H = text, 40, 30
		case i < 90:
			o = d.newKind(kCabinet, 0, 0)
			for j := 0; j < 40; j++ {
				it := &dOct{Kind: kPage, Header: fmt.Sprint("Item ", j), Body: text[:20000]}
				o.Items = append(o.Items, it)
			}
			o.DrawerOpen = true
		default:
			o = d.newKind(kOct, 0, 0)
			o.Header, o.Body = fmt.Sprint("Idea ", i), "Some words about this idea."
		}
		o.X, o.Y = next(3000)-1500, next(1500)-750
	}
	for i := 0; i < 150; i++ {
		d.links = append(d.links, dLink{d.octs[next(400)].ID, d.octs[next(400)].ID})
	}
	for i := 0; i < 3000; i++ {
		d.marks[[2]int{next(3000) - 1500, next(1500) - 750}] = dMark{Pen: int8(next(8))}
	}
	d.sel, d.editing = -1, partNone
	d.dirty = true
	d.save()
	return m
}

func BenchmarkBigBoardPan(b *testing.B) {
	for _, zi := range []int{0, 2, 5, 9} {
		b.Run(fmt.Sprint(designZooms[zi]), func(b *testing.B) {
			m := bigBoard(b)
			d := m.ds()
			d.zoom = zi
			d.camX, d.camY = -120/d.z(), -32/d.z()
			_ = m.View()
			benchDragWith(b, m, 120, 32, tea.MouseMiddle)
		})
	}
}

func BenchmarkBigBoardTyping(b *testing.B) {
	m := bigBoard(b)
	designSaveLater = true // as in the real program
	defer func() { designSaveLater = false }()
	d := m.ds()
	pg := d.octs[30]
	d.zoom, d.sel, d.editing = 5, pg.ID, partBody
	d.camX, d.camY = float64(pg.X)-20, float64(pg.Y)-10
	_ = m.View()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Update(tea.KeyPressMsg(tea.Key{Code: 'a', Text: "a"}))
		_ = m.View()
	}
}

func BenchmarkBigBoardSave(b *testing.B) {
	m := bigBoard(b)
	d := m.ds()
	b.ResetTimer()
	pg := d.octs[30]
	for i := 0; i < b.N; i++ {
		pg.Body += "a" // one page changes, as when typing
		d.dirty = true
		d.save()
	}
}

// Wheel zooms on the large project: every structure on screen is drawn anew
// at its new size.
func BenchmarkBigBoardZoom(b *testing.B) {
	for _, lo := range []int{0, 3, 7} {
		b.Run(fmt.Sprint(designZooms[lo], "-", designZooms[lo+2]), func(b *testing.B) {
			m := bigBoard(b)
			designSaveLater = true // as in the real program
			defer func() { designSaveLater = false }()
			d := m.ds()
			d.zoom = lo
			d.camX, d.camY = -120/d.z(), -32/d.z()
			_ = m.View()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				btn := tea.MouseWheelUp
				if i%4 >= 2 {
					btn = tea.MouseWheelDown
				}
				m.Update(tea.MouseWheelMsg(tea.Mouse{X: 120, Y: 32 + m.boardTop(), Button: btn}))
				_ = m.View()
			}
		})
	}
}

// A very large project: SY_HUGE structures (5000 unless set), a link for
// every two or three, ink everywhere, and cabinets full of pages, spread
// over a wide board. designSaveLater is on, as in the real program.
func hugeBoard(b *testing.B) *Model {
	n := 5000
	if s := os.Getenv("SY_HUGE"); s != "" {
		fmt.Sscan(s, &n)
	}
	b.Setenv("XDG_DATA_HOME", b.TempDir())
	designSaveLater = true
	b.Cleanup(func() { designSaveLater = false })
	m, _ := NewModel("", 0)
	m.Update(tea.WindowSizeMsg{Width: 240, Height: 65})
	m.setMode(designMode)
	d := m.ds()
	rnd := 7
	next := func(k int) int { rnd = (rnd*1103515245 + 12345) & 0x7fffffff; return rnd % k }
	side := 40 * int(math.Sqrt(float64(n)))
	text := strings.Repeat("Words in a large project.\n", 40)
	for i := 0; i < n; i++ {
		var o *dOct
		switch i % 10 {
		case 0:
			o = d.newKind(kPage, 0, 0)
			o.Body = text
		case 1:
			o = d.newKind(kNote, 0, 0)
			o.Body = "A short note."
		case 2:
			if i%50 == 2 {
				o = d.newKind(kCabinet, 0, 0)
				for j := 0; j < 30; j++ {
					o.Items = append(o.Items, &dOct{ID: 1_000_000 + i*100 + j, Kind: kPage, Header: fmt.Sprint("Item ", j), Body: text})
				}
				o.OpenStack = kPage
				break
			}
			fallthrough
		default:
			o = d.newKind(kOct, 0, 0)
			o.Header, o.Body = fmt.Sprint("Idea ", i), "Some words about this idea."
		}
		o.X, o.Y = next(side)-side/2, next(side/2)-side/4
	}
	for i := 0; i < n*2/5; i++ {
		d.links = append(d.links, dLink{d.octs[next(n)].ID, d.octs[next(n)].ID})
	}
	for i := 0; i < n*4; i++ {
		d.marks[[2]int{next(side) - side/2, next(side/2) - side/4}] = dMark{Pen: int8(next(8))}
	}
	d.sel, d.editing = -1, partNone
	d.startUndo()
	return m
}

func BenchmarkHuge(b *testing.B) {
	b.Run("pan100", func(b *testing.B) {
		m := hugeBoard(b)
		d := m.ds()
		d.zoom, d.camX, d.camY = 5, -120, -32
		m.View()
		benchDragWith(b, m, 120, 32, tea.MouseMiddle)
	})
	b.Run("pan25", func(b *testing.B) {
		m := hugeBoard(b)
		d := m.ds()
		d.zoom, d.camX, d.camY = 0, -480, -128
		m.View()
		benchDragWith(b, m, 120, 32, tea.MouseMiddle)
	})
	b.Run("click", func(b *testing.B) { // choose structures: hit, bring forward, save
		m := hugeBoard(b)
		d := m.ds()
		d.zoom, d.camX, d.camY = 5, -120, -32
		m.View()
		var spots [][2]int
		for _, o := range d.octs {
			if x, y, w, h := d.rect(o); x > 2 && y > 2 && x+w < 230 && y+h < 55 {
				spots = append(spots, [2]int{x + w/2, y + h/2})
			}
		}
		if len(spots) == 0 {
			b.Skip("nothing on screen")
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			s := spots[i%len(spots)]
			click(nil2(m), s[0], s[1]+m.boardTop(), tea.MouseLeft)
			m.View()
		}
	})
	b.Run("move", func(b *testing.B) { // drag a structure
		m := hugeBoard(b)
		d := m.ds()
		d.zoom, d.camX, d.camY = 5, -120, -32
		m.View()
		for _, o := range d.octs {
			if x, y, w, h := d.rect(o); x > 40 && y > 10 && x+w < 200 && y+h < 50 && o.Kind == kOct {
				benchDrag(b, m, x+w/2, y+h/2+m.boardTop())
				return
			}
		}
		b.Skip("nothing on screen")
	})
	b.Run("typing", func(b *testing.B) {
		m := hugeBoard(b)
		d := m.ds()
		pg := d.octs[0]
		d.zoom, d.sel, d.editing = 5, pg.ID, partBody
		d.camX, d.camY = float64(pg.X)-20, float64(pg.Y)-10
		m.View()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			m.Update(tea.KeyPressMsg(tea.Key{Code: 'a', Text: "a"}))
			m.View()
		}
	})
	b.Run("zoom", func(b *testing.B) {
		m := hugeBoard(b)
		d := m.ds()
		d.zoom, d.camX, d.camY = 1, -360, -96
		m.View()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			btn := tea.MouseWheelUp
			if i%4 >= 2 {
				btn = tea.MouseWheelDown
			}
			m.Update(tea.MouseWheelMsg(tea.Mouse{X: 120, Y: 32 + m.boardTop(), Button: btn}))
			m.View()
		}
	})
	b.Run("save", func(b *testing.B) { // the background write, one page changed
		m := hugeBoard(b)
		d := m.ds()
		d.dirty = true
		gen, f := d.snapshot()
		d.writeSnapshot(f, gen)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			d.octs[0].Body += "a"
			gen, f := d.snapshot()
			d.writeSnapshot(f, gen)
		}
	})
}

func nil2(m *Model) *Model { return m }
