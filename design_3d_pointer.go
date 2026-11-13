package main

// The mouse pointer is hidden while the room is in 3D: the cross in the
// middle of the view is the pointer there (design_3d.go).
//
// A terminal program cannot hide the pointer: kitty's pointer shapes
// (OSC 22) have no "none". Under Hyprland the compositor can:
// cursor:invisible, set through Hyprland's socket (as hyprctl does), in
// Lua (hl.config, Hyprland with a Lua config) or else as a keyword (older
// Hyprland). It is set while the room is in 3D and sy has the keyboard, and set back when either stops, when sy ends,
// and (a small watcher, see hideWatch) when sy is killed, so the pointer
// never stays hidden. A pointer the person hid in Hyprland's settings is
// left alone. Other desktops keep the pointer.
//
// While it is hidden, a pointer that has gone far from the window's middle
// is put back there (pointerRecenter), as a game holds the mouse, so mouse
// look never meets the window's edge. The report of that jump only notes
// where the pointer is (pointerWarped): the camera does not turn back.

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
)

// hyprPointer hides and shows the pointer through Hyprland, in order, off
// Bubble Tea's loop.
type hyprPointer struct {
	mu      sync.Mutex
	want    bool // hidden is asked for (Bubble Tea's side)
	hidden  bool // and it is (the worker's side)
	theirs  bool // the person's own setting already hides it: left alone
	asked   bool // theirs is known
	watched bool // hideWatch is running
	work    chan func()
	done    sync.WaitGroup
	moveOld bool // Hyprland takes "movecursor x y", not Lua
}

const (
	recenterAt = 0.25                   // the pointer this share of the window's width or height from its middle is put back
	warpNear   = 3.0                    // cells: a jump to this near the middle, after asking, is the pointer put back
	warpWait   = 300 * time.Millisecond // asked longer ago than this, a jump is not taken for it
)

var (
	hyprPtr = &hyprPointer{}
	// hyprctl sends Hyprland a command (args, as hyprctl takes them) and
	// gives its answer (tests set it).
	hyprctl = func(args ...string) (string, error) {
		c, err := net.DialTimeout("unix", filepath.Join(runtimeDir(), "hypr", os.Getenv("HYPRLAND_INSTANCE_SIGNATURE"), ".socket.sock"), 300*time.Millisecond)
		if err != nil {
			return "", err
		}
		defer c.Close()
		c.SetDeadline(time.Now().Add(500 * time.Millisecond))
		if _, err := c.Write([]byte(strings.Join(args, " "))); err != nil {
			return "", err
		}
		out, err := io.ReadAll(c)
		return string(out), err
	}
	// hyprOn is whether sy runs under Hyprland (tests set it).
	hyprOn = os.Getenv("HYPRLAND_INSTANCE_SIGNATURE") != ""
)

// pointerWanted is whether the pointer should be hidden now.
func (m *Model) pointerWanted() bool {
	return hyprOn && !m.blurred && m.inDesign() && m.design != nil && m.design.room.on()
}

// pointerCmd runs after every message: it hides the pointer as the room
// goes 3D and shows it again as it goes flat or sy loses the keyboard.
func (m *Model) pointerCmd() tea.Cmd {
	hyprPtr.set(m.pointerWanted())
	return nil
}

// set asks for the pointer hidden or shown; the worker does it.
func (h *hyprPointer) set(hide bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.want == hide {
		return
	}
	h.want = hide
	h.do(func() { h.apply(hide) }, true)
}

// do gives the worker job, after the ones before it (h.mu held). Unless
// it must be done (hiding, showing), it is dropped if the worker is far
// behind (putting the pointer back).
func (h *hyprPointer) do(job func(), must bool) {
	if h.work == nil {
		h.work = make(chan func(), 16)
		go h.run()
	}
	h.done.Add(1)
	if must {
		h.work <- job
		return
	}
	select {
	case h.work <- job:
	default:
		h.done.Done()
	}
}

// run does the jobs one after another.
func (h *hyprPointer) run() {
	for job := range h.work {
		job()
		h.done.Done()
	}
}

// pointerRecenter puts the hidden pointer back in the window's middle once
// it is far from it (at window cells x, y).
func (m *Model) pointerRecenter(x, y float64) {
	r := &m.ds().room
	w, ht := float64(m.width), float64(m.height)
	if !m.pointerWanted() || r.warpOn && time.Since(r.warpAt) < warpWait ||
		math.Abs(x-w/2) < recenterAt*w && math.Abs(y-ht/2) < recenterAt*ht {
		return
	}
	r.warpOn, r.warpAt = true, time.Now()
	hyprPtr.mu.Lock()
	hyprPtr.do(hyprPtr.recenter, false)
	hyprPtr.mu.Unlock()
}

// pointerWarped is whether a mouse report at window cells (x, y), dx, dy
// from the last, is the pointer put back in the middle: a jump to near it
// soon after asking.
func (m *Model) pointerWarped(x, y, dx, dy float64) bool {
	r := &m.ds().room
	if !r.warpOn {
		return false
	}
	if time.Since(r.warpAt) > warpWait {
		r.warpOn = false
		return false
	}
	if math.Abs(x-float64(m.width)/2) < warpNear && math.Abs(y-float64(m.height)/2) < warpNear &&
		(math.Abs(dx) > warpNear || math.Abs(dy) > warpNear) {
		r.warpOn = false
		return true
	}
	return false
}

// recenter moves the pointer to the middle of the window being typed in,
// if it is sy's (its title ends in "Symbolizer", see syFullscreen).
func (h *hyprPointer) recenter() {
	out, err := hyprctl("j/activewindow")
	if err != nil {
		return
	}
	var w struct {
		Title string `json:"title"`
		At    [2]int `json:"at"`
		Size  [2]int `json:"size"`
	}
	if json.Unmarshal([]byte(out), &w) != nil || !strings.HasSuffix(w.Title, "Symbolizer") || w.Size[0] <= 0 {
		return
	}
	x, y := w.At[0]+w.Size[0]/2, w.At[1]+w.Size[1]/2
	if !h.moveOld {
		if out, err := hyprctl("dispatch", fmt.Sprintf("hl.dsp.cursor.move({ x = %d, y = %d })", x, y)); err == nil && strings.TrimSpace(out) == "ok" {
			return
		}
	}
	if out, err := hyprctl("dispatch", "movecursor", fmt.Sprint(x), fmt.Sprint(y)); err == nil && strings.TrimSpace(out) == "ok" {
		h.moveOld = true
	}
}

// apply hides (or shows) the pointer, unless the person's own setting hides it.
func (h *hyprPointer) apply(hide bool) {
	if !h.asked {
		h.asked = true
		out, _ := hyprctl("getoption", "cursor:invisible")
		h.theirs = strings.Contains(out, "bool: true") || strings.Contains(out, "int: 1")
	}
	if h.theirs || h.hidden == hide {
		return
	}
	if hide && !h.watched {
		h.watched = true
		hideWatch()
	}
	if hyprInvisible(hide) || !hide {
		h.hidden = hide
	}
}

// hyprInvisible sets Hyprland's cursor:invisible, and says whether it took it.
func hyprInvisible(hide bool) bool {
	v := map[bool]string{true: "true", false: "false"}[hide]
	for _, args := range [][]string{
		{"eval", "hl.config({ cursor = { invisible = " + v + " } })"},
		{"keyword", "cursor:invisible", v},
	} {
		if out, err := hyprctl(args...); err == nil && strings.TrimSpace(out) == "ok" {
			return true
		}
	}
	return false
}

// show shows the pointer again and waits until it is shown (sy ending).
func (h *hyprPointer) show() {
	h.set(false)
	h.done.Wait()
}

// hideWatch starts a small watcher, apart from sy and its terminal, that
// shows the pointer again once sy has ended, however it ended (killed, its
// window closed): a pointer left hidden would be very hard to get back.
var hideWatch = func() {
	script := fmt.Sprintf("while kill -0 %d 2>/dev/null; do sleep 1; done; "+
		"hyprctl eval 'hl.config({ cursor = { invisible = false } })' >/dev/null 2>&1; "+
		"hyprctl keyword cursor:invisible false >/dev/null 2>&1", os.Getpid())
	c := exec.Command("sh", "-c", script)
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if c.Start() == nil {
		go c.Wait()
	}
}
