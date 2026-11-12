package main

// ---- the 3D room as a picture (kitty) -------------------------------------------------
//
// In kitty the room is not drawn with quarter blocks (four pixels a cell)
// but as a real picture, as many pixels as the terminal's own cells have
// (see room3DDensity). It is shown with kitty's Unicode placeholders: the
// picture is sent once a frame with an escape code (a virtual placement,
// c columns by r rows, never answered: q=2), and the board area is
// written as cells of the letter U+10EEEE, each with two marks above it
// that say its row and column of the picture, in a colour that is the
// picture's number. Those cells are ordinary text, so Bubble Tea's own
// drawing, the menus and help drawn over the board and the redrawing of
// only what changed all work as with any other text. The words stay real
// letters in the cells over the picture (see writeCells).
//
// The escape code cannot go in the View's text: it goes to Bubble Tea as a
// tea.RawMsg, which it writes in order with its frames. A frame is drawn
// in View, so one goroutine (kittyPic) sends the newest picture and drops
// any older one not sent yet. On this computer the pixels go through a
// file kitty reads and deletes (t=t, quick); over SSH they go in the code
// itself, packed (zlib) and in base64 (t=d, o=z).

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"fmt"
	"hash/maphash"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

// room3DPixels: the 3D room is drawn as a kitty picture. Set in main.go
// (kittyWanted), off in tests, which read the quarter blocks.
var room3DPixels = false

// room3DRemote: over SSH, where the pixels go in the escape code itself,
// packed. Fewer of them, then.
var room3DRemote = false

// The most pixels the picture has: still, and while the camera moves (a
// drag, a turn held, walking), when a frame has to be quick. About 150 ms
// after the camera stops, the room is drawn again sharp (room3DSettle).
const (
	room3DStill  = 2_400_000
	room3DMoving = 360_000
	room3DSettle = 150 * time.Millisecond
)

// kittyWanted is whether the 3D room is drawn as a kitty picture, from the
// environment: SY_3D_PIXELS=0 turns it off, =1 on; otherwise it is on in
// kitty.
func kittyWanted(env func(string) string) bool {
	switch env("SY_3D_PIXELS") {
	case "0":
		return false
	case "1":
		return true
	}
	return env("KITTY_WINDOW_ID") != "" || env("TERM") == "xterm-kitty"
}

// kittyRemote is whether sy runs over SSH.
func kittyRemote(env func(string) string) bool {
	return env("SSH_CONNECTION") != "" || env("SSH_TTY") != ""
}

// room3DImageID is the picture's number: kitty's numbers are shared by
// everything in a window, so it is made from sy's process number. It is
// the colour of the placeholder cells (24 bits: red, green, blue).
var room3DImageID = 0x530000 | uint32(os.Getpid()&0xffff)

// kittyMarks are the marks that say a placeholder's row and column: the
// first is 0, the next 1, and so on (kitty's rowcolumn-diacritics.txt).
// The picture can be at most this many cells across and down.
var kittyMarks = []rune{
	0x0305, 0x030D, 0x030E, 0x0310, 0x0312, 0x033D, 0x033E, 0x033F, 0x0346, 0x034A, 0x034B, 0x034C,
	0x0350, 0x0351, 0x0352, 0x0357, 0x035B, 0x0363, 0x0364, 0x0365, 0x0366, 0x0367, 0x0368, 0x0369,
	0x036A, 0x036B, 0x036C, 0x036D, 0x036E, 0x036F, 0x0483, 0x0484, 0x0485, 0x0486, 0x0487, 0x0592,
	0x0593, 0x0594, 0x0595, 0x0597, 0x0598, 0x0599, 0x059C, 0x059D, 0x059E, 0x059F, 0x05A0, 0x05A1,
	0x05A8, 0x05A9, 0x05AB, 0x05AC, 0x05AF, 0x05C4, 0x0610, 0x0611, 0x0612, 0x0613, 0x0614, 0x0615,
	0x0616, 0x0617, 0x0657, 0x0658, 0x0659, 0x065A, 0x065B, 0x065D, 0x065E, 0x06D6, 0x06D7, 0x06D8,
	0x06D9, 0x06DA, 0x06DB, 0x06DC, 0x06DF, 0x06E0, 0x06E1, 0x06E2, 0x06E4, 0x06E7, 0x06E8, 0x06EB,
	0x06EC, 0x0730, 0x0732, 0x0733, 0x0735, 0x0736, 0x073A, 0x073D, 0x073F, 0x0740, 0x0741, 0x0743,
	0x0745, 0x0747, 0x0749, 0x074A, 0x07EB, 0x07EC, 0x07ED, 0x07EE, 0x07EF, 0x07F0, 0x07F1, 0x07F3,
	0x0816, 0x0817, 0x0818, 0x0819, 0x081B, 0x081C, 0x081D, 0x081E, 0x081F, 0x0820, 0x0821, 0x0822,
	0x0823, 0x0825, 0x0826, 0x0827, 0x0829, 0x082A, 0x082B, 0x082C, 0x082D, 0x0951, 0x0953, 0x0954,
	0x0F82, 0x0F83, 0x0F86, 0x0F87, 0x135D, 0x135E, 0x135F, 0x17DD, 0x193A, 0x1A17, 0x1A75, 0x1A76,
	0x1A77, 0x1A78, 0x1A79, 0x1A7A, 0x1A7B, 0x1A7C, 0x1B6B, 0x1B6D, 0x1B6E, 0x1B6F, 0x1B70, 0x1B71,
	0x1B72, 0x1B73, 0x1CD0, 0x1CD1, 0x1CD2, 0x1CDA, 0x1CDB, 0x1CE0, 0x1DC0, 0x1DC1, 0x1DC3, 0x1DC4,
	0x1DC5, 0x1DC6, 0x1DC7, 0x1DC8, 0x1DC9, 0x1DCB, 0x1DCC, 0x1DD1, 0x1DD2, 0x1DD3, 0x1DD4, 0x1DD5,
	0x1DD6, 0x1DD7, 0x1DD8, 0x1DD9, 0x1DDA, 0x1DDB, 0x1DDC, 0x1DDD, 0x1DDE, 0x1DDF, 0x1DE0, 0x1DE1,
	0x1DE2, 0x1DE3, 0x1DE4, 0x1DE5, 0x1DE6, 0x1DFE, 0x20D0, 0x20D1, 0x20D4, 0x20D5, 0x20D6, 0x20D7,
	0x20DB, 0x20DC, 0x20E1, 0x20E7, 0x20E9, 0x20F0, 0x2CEF, 0x2CF0, 0x2CF1, 0x2DE0, 0x2DE1, 0x2DE2,
	0x2DE3, 0x2DE4, 0x2DE5, 0x2DE6, 0x2DE7, 0x2DE8, 0x2DE9, 0x2DEA, 0x2DEB, 0x2DEC, 0x2DED, 0x2DEE,
	0x2DEF, 0x2DF0, 0x2DF1, 0x2DF2, 0x2DF3, 0x2DF4, 0x2DF5, 0x2DF6, 0x2DF7, 0x2DF8, 0x2DF9, 0x2DFA,
	0x2DFB, 0x2DFC, 0x2DFD, 0x2DFE, 0x2DFF, 0xA66F, 0xA67C, 0xA67D, 0xA6F0, 0xA6F1, 0xA8E0, 0xA8E1,
	0xA8E2, 0xA8E3, 0xA8E4, 0xA8E5, 0xA8E6, 0xA8E7, 0xA8E8, 0xA8E9, 0xA8EA, 0xA8EB, 0xA8EC, 0xA8ED,
	0xA8EE, 0xA8EF, 0xA8F0, 0xA8F1, 0xAAB0, 0xAAB2, 0xAAB3, 0xAAB7, 0xAAB8, 0xAABE, 0xAABF, 0xAAC1,
	0xFE20, 0xFE21, 0xFE22, 0xFE23, 0xFE24, 0xFE25, 0xFE26, 0x10A0F, 0x10A38, 0x1D185, 0x1D186, 0x1D187,
	0x1D188, 0x1D189, 0x1D1AA, 0x1D1AB, 0x1D1AC, 0x1D1AD, 0x1D242, 0x1D243, 0x1D244,
}

// placeholder is the text of the placeholder cell at row and column of the
// picture: U+10EEEE with the marks of its row and its column. Each cell
// says both, so a cell after a letter (which stands in for a placeholder)
// needs nothing from the cell before it.
func placeholder(buf []byte, row, col int) []byte {
	buf = append(buf, "\U0010EEEE"...)
	buf = append(buf, string(kittyMarks[row])...)
	return append(buf, string(kittyMarks[col])...)
}

// ---- the picture's size ----------------------------------------------------------------

// termCellAt is the terminal's cell size in pixels, kept for the window
// size it was read at.
var termCellAt struct {
	cols, rows, cw, ch int
}

// termCell is how many pixels wide and tall a cell of the terminal is
// (for a window of cols×rows cells), as the terminal says (TIOCGWINSZ);
// 10×20 when it does not say.
func termCell(cols, rows int) (cw, ch int) {
	t := &termCellAt
	if t.cw > 0 && t.cols == cols && t.rows == rows {
		return t.cw, t.ch
	}
	cw, ch = 10, 20
	for _, f := range []*os.File{os.Stdout, os.Stdin, os.Stderr} {
		ws, err := unix.IoctlGetWinsize(int(f.Fd()), unix.TIOCGWINSZ)
		if err == nil && ws.Col > 0 && ws.Row > 0 && ws.Xpixel > 0 && ws.Ypixel > 0 {
			cw, ch = int(ws.Xpixel)/int(ws.Col), int(ws.Ypixel)/int(ws.Row)
			break
		}
	}
	cw, ch = max(cw, 2), max(ch, 2)
	*t = struct{ cols, rows, cw, ch int }{cols, rows, cw, ch}
	return cw, ch
}

// room3DDensity is the picture's size in pixels for a board area of w×h
// cells, each cw×ch pixels: the terminal's own pixels, or, when that is
// more than limit, fewer in the same proportions (never fewer than the
// quarter blocks' four a cell).
func room3DDensity(w, h, cw, ch, limit int) (pw, ph int) {
	pw, ph = w*cw, h*ch
	if n := pw * ph; n > limit && limit > 0 {
		k := math.Sqrt(float64(limit) / float64(n))
		pw, ph = int(float64(pw)*k+0.5), int(float64(ph)*k+0.5)
	}
	return max(pw, 2*w), max(ph, 2*h)
}

// ---- the board area's cells ------------------------------------------------------------

// writeCells writes the board area as h lines of w cells over the
// picture: a placeholder in each cell, and a word's letter (with its
// colours) where one is drawn. A placeholder's colour is the picture's
// number and never changes, so from frame to frame only the cells whose
// letters change are sent again; the picture itself goes separately.
func (r *room3D) writeCells(w, h int, sb *strings.Builder) {
	buf := make([]byte, 0, w*16)
	id := room3DImageID
	colour := func(buf []byte, lead string, c uint32) []byte {
		buf = append(buf, lead...)
		buf = strconv.AppendUint(buf, uint64(c>>16&255), 10)
		buf = append(buf, ';')
		buf = strconv.AppendUint(buf, uint64(c>>8&255), 10)
		buf = append(buf, ';')
		buf = strconv.AppendUint(buf, uint64(c&255), 10)
		return append(buf, 'm')
	}
	const unset, plain = int64(-1), int64(-2) // no colour set yet; the terminal's own background
	for row := 0; row < h; row++ {
		buf = buf[:0]
		fg, bg := unset, unset
		for col := 0; col < w; col++ {
			if ch := &r.chars[row*w+col]; ch.stamp == r.stamp {
				if ch.cont {
					continue // the second half of a wide letter: drawn with it
				}
				if int64(ch.fg) != fg {
					buf = colour(buf, "\x1b[38;2;", ch.fg)
					fg = int64(ch.fg)
				}
				if int64(ch.bg) != bg {
					buf = colour(buf, "\x1b[48;2;", ch.bg)
					bg = int64(ch.bg)
				}
				buf = append(buf, ch.s...)
				continue
			}
			if fg != int64(id) {
				buf = colour(buf, "\x1b[38;2;", id)
				fg = int64(id)
			}
			if bg != plain {
				buf = append(buf, "\x1b[49m"...)
				bg = plain
			}
			buf = placeholder(buf, row, col)
		}
		buf = append(buf, "\x1b[m\n"...)
		sb.Write(buf)
	}
}

// picture is the room's pixels, lit, as red, green and blue bytes, row by
// row, in buf (made bigger when it is too small).
func (r *room3D) picture(buf []byte) []byte {
	n := r.pw * r.ph
	if cap(buf) < 3*n {
		buf = make([]byte, 3*n)
	}
	buf = buf[:3*n]
	r.bands(r.ph, func(j0, j1 int) {
		for i := j0 * r.pw; i < j1*r.pw; i++ {
			c := r.pixel(i)
			buf[3*i], buf[3*i+1], buf[3*i+2] = byte(c>>16), byte(c>>8), byte(c)
		}
	})
	return buf
}

// ---- sending it -------------------------------------------------------------------------

// kittyFrame is a picture to send: its pixels (see picture), its size, and
// the cells it fills.
type kittyFrame struct {
	rgb        []byte
	pw, ph     int
	cols, rows int
}

// kittyOut sends the pictures, one goroutine for them all.
type kittyOut struct {
	mu      sync.Mutex
	wake    *sync.Cond
	next    *kittyFrame // the newest picture, not sent yet
	del     bool        // the picture is to go (the room went flat)
	stopped bool
	done    chan struct{}
	send    func(string) // to Bubble Tea (a tea.RawMsg)
	shown   bool         // a picture was sent, and not deleted since
	n       int          // files written: each its own name
	pool    sync.Pool    // pixel buffers, used again
	dir     string       // where the files go
	seed    maphash.Seed
	last    uint64 // the last picture sent, summed up (see loop)
}

// kittyPic is the sender of the 3D room's pictures, once main.go starts
// it (startKitty); before that, pictures are drawn but not sent.
var kittyPic kittyOut

// startKitty starts sending pictures with send.
func startKitty(send func(string)) {
	k := &kittyPic
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.send != nil {
		return
	}
	k.wake = sync.NewCond(&k.mu)
	k.send, k.done, k.dir, k.seed = send, make(chan struct{}), kittyDir(), maphash.MakeSeed()
	go k.loop()
}

// kittyDir is where the pictures' files go: memory (/dev/shm) if there
// is such a place, else the computer's folder for passing files.
func kittyDir() string {
	if st, err := os.Stat("/dev/shm"); err == nil && st.IsDir() && unix.Access("/dev/shm", unix.W_OK) == nil {
		return "/dev/shm"
	}
	return os.TempDir()
}

// buffer is a pixel buffer to draw a picture in.
func (k *kittyOut) buffer() []byte {
	if b, ok := k.pool.Get().([]byte); ok {
		return b
	}
	return nil
}

// show sends a picture (newer than any not sent yet, which is dropped).
// Without a sender (tests), it does nothing.
func (k *kittyOut) show(f *kittyFrame) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.send == nil || k.stopped {
		return
	}
	if k.next != nil {
		k.pool.Put(k.next.rgb[:0])
	}
	k.next, k.shown = f, true
	k.wake.Signal()
}

// remove deletes the picture from kitty (the room went flat, or Design
// mode was left), and drops any picture not sent yet.
func (k *kittyOut) remove() {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.send == nil || !k.shown {
		return
	}
	if k.next != nil {
		k.pool.Put(k.next.rgb[:0])
		k.next = nil
	}
	k.del, k.shown = true, false
	k.wake.Signal()
}

// isShown is whether a picture is in kitty now.
func (k *kittyOut) isShown() bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.shown
}

// loop sends the pictures as they come, the newest only.
func (k *kittyOut) loop() {
	defer close(k.done)
	for {
		k.mu.Lock()
		for k.next == nil && !k.del && !k.stopped {
			k.wake.Wait()
		}
		if k.stopped {
			k.mu.Unlock()
			return
		}
		f, del := k.next, k.del
		k.next, k.del = nil, false
		k.n++
		n := k.n
		k.mu.Unlock()
		if del {
			k.send(kittyDelete())
			k.last = 0
		}
		if f != nil {
			// The same picture as the last one sent (the room drawn again
			// for a key let go, a minute gone by): kitty has it already.
			sum := maphash.Bytes(k.seed, f.rgb) ^ uint64(f.pw)<<40 ^ uint64(f.ph)<<20 ^ uint64(f.cols)<<8 ^ uint64(f.rows)
			if sum == k.last {
				k.pool.Put(f.rgb[:0])
				continue
			}
			k.last = sum
			var seq string
			if room3DRemote {
				seq = kittyDirect(f)
			} else {
				seq = kittyFile(f, filepath.Join(k.dir, fmt.Sprintf("sy-%d-%d-tty-graphics-protocol.rgb", os.Getpid(), n)))
			}
			k.pool.Put(f.rgb[:0])
			if seq != "" {
				k.send(seq)
			}
		}
	}
}

// stop ends the sending (sy is closing): it waits for the goroutine, tells
// kitty to delete the picture (written straight to out, Bubble Tea has
// finished) and deletes any file kitty has not read.
func (k *kittyOut) stop(out io.Writer) {
	k.mu.Lock()
	if k.send == nil || k.stopped {
		k.mu.Unlock()
		return
	}
	k.stopped = true
	k.wake.Signal()
	k.mu.Unlock()
	<-k.done
	if k.n > 0 {
		_, _ = io.WriteString(out, kittyDelete())
	}
	old, _ := filepath.Glob(filepath.Join(k.dir, fmt.Sprintf("sy-%d-*-tty-graphics-protocol.rgb", os.Getpid())))
	for _, f := range old {
		_ = os.Remove(f)
	}
}

// kittyHead is the start of the escape code that sends a picture and
// shows it as a virtual placement, c×r cells, for the placeholders.
func kittyHead(f *kittyFrame) string {
	return fmt.Sprintf("\x1b_Ga=T,U=1,i=%d,c=%d,r=%d,f=24,s=%d,v=%d,q=2", room3DImageID, f.cols, f.rows, f.pw, f.ph)
}

// kittyFile writes the pixels to a file kitty reads and then deletes (its
// name has "tty-graphics-protocol" in it), and is the code that says so.
func kittyFile(f *kittyFrame, path string) string {
	if err := os.WriteFile(path, f.rgb, 0o600); err != nil {
		return kittyDirect(f) // no file: the pixels in the code
	}
	return kittyHead(f) + ",t=t;" + base64.StdEncoding.EncodeToString([]byte(path)) + "\x1b\\"
}

// kittyDirect is the code with the pixels in it, packed and in base64, in
// pieces of at most 4096 bytes (m=1: more to come).
func kittyDirect(f *kittyFrame) string {
	var packed bytes.Buffer
	zw, _ := zlib.NewWriterLevel(&packed, zlib.BestSpeed)
	_, _ = zw.Write(f.rgb)
	_ = zw.Close()
	data := base64.StdEncoding.EncodeToString(packed.Bytes())
	const piece = 4096
	var sb strings.Builder
	sb.Grow(len(data) + len(data)/piece*16 + 128)
	sb.WriteString(syncStart)
	for at := 0; at < len(data) || at == 0; at += piece {
		end := min(at+piece, len(data))
		more := 0
		if end < len(data) {
			more = 1
		}
		if at == 0 {
			sb.WriteString(kittyHead(f) + ",t=d,o=z")
		} else {
			sb.WriteString("\x1b_Gq=2")
		}
		sb.WriteString(",m=" + strconv.Itoa(more) + ";")
		sb.WriteString(data[at:end])
		sb.WriteString("\x1b\\")
	}
	sb.WriteString(syncEnd)
	return sb.String()
}

// The pieces go in one synchronized update: the first piece empties the
// old picture, and were kitty to draw the screen before the last piece is
// in, it would find no picture for the placeholders and not look again
// until their cells change.
const syncStart, syncEnd = "\x1b[?2026h", "\x1b[?2026l"

// kittyDelete is the code that deletes the picture, and its placement.
func kittyDelete() string {
	return fmt.Sprintf("\x1b_Ga=d,d=I,i=%d,q=2\x1b\\", room3DImageID)
}
