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

import (
	"fmt"
	"io"
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
	work    chan bool
	done    sync.WaitGroup
}

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
	if h.work == nil {
		h.work = make(chan bool, 16)
		go h.run()
	}
	h.done.Add(1)
	h.work <- hide
}

// run hides and shows the pointer as asked, one after another.
func (h *hyprPointer) run() {
	for hide := range h.work {
		h.apply(hide)
		h.done.Done()
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
