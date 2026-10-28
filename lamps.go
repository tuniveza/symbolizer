package main

import (
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"golang.org/x/sys/unix"
)

// Lock keys (Caps, Num, Scroll, Fn), read from the file the desktop's emberline-locks writes.
// They show two ways:
//
//	lamps      in the status bar, gold when on and ash when off. kitty shows the same lamps on its
//	           own tab bar, so they are left out there; any other terminal (Ghostty) gets them here.
//	the plate  (lockplate.go) flashes up when a lock changes while sy fills the screen. The
//	           desktop's own plate stays away then (emberline-locks asks Hyprland the same thing).
//
// The file is watched with inotify, so a change shows at once; without it, it is read once a second.

type lampsMsg string // the file's text

func runtimeDir() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return dir
	}
	return "/tmp"
}

func lampsFile() string { return filepath.Join(runtimeDir(), "emberline-locks.json") }

var showLamps = !strings.Contains(os.Getenv("TERM"), "kitty") && os.Getenv("KITTY_WINDOW_ID") == ""

func readLampsFile() string {
	data, err := os.ReadFile(lampsFile())
	if err != nil {
		return ""
	}
	return string(data)
}

// lampsWake gets a signal each time the file is written; nil when inotify is not there.
var lampsWake chan struct{}

func watchLamps() {
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC)
	if err != nil {
		return
	}
	// the daemon writes a new file and renames it over the old one; a test just writes it
	if _, err := unix.InotifyAddWatch(fd, runtimeDir(), unix.IN_CLOSE_WRITE|unix.IN_MOVED_TO); err != nil {
		unix.Close(fd)
		return
	}
	wake := make(chan struct{}, 1)
	name := filepath.Base(lampsFile())
	go func() {
		buf := make([]byte, 8192)
		for {
			n, err := unix.Read(fd, buf)
			if err == unix.EINTR {
				continue
			}
			if err != nil {
				return
			}
			for off := 0; off+unix.SizeofInotifyEvent <= n; {
				ev := (*unix.InotifyEvent)(unsafe.Pointer(&buf[off]))
				end := off + unix.SizeofInotifyEvent + int(ev.Len)
				if strings.TrimRight(string(buf[off+unix.SizeofInotifyEvent:min(end, n)]), "\x00") == name {
					select {
					case wake <- struct{}{}:
					default:
					}
				}
				off = end
			}
		}
	}()
	lampsWake = wake
}

// startLamps begins watching the file (once; tests make many models).
func (m *Model) startLamps() tea.Cmd {
	lampsOnce.Do(watchLamps)
	return waitLamps()
}

var lampsOnce sync.Once

// waitLamps reads the file again after the next change (or after a while, in case one was missed).
func waitLamps() tea.Cmd {
	return func() tea.Msg {
		if lampsWake == nil {
			time.Sleep(time.Second)
		} else {
			select {
			case <-lampsWake:
			case <-time.After(30 * time.Second):
			}
		}
		return lampsMsg(readLampsFile())
	}
}

// lockOn says whether a lock is on in the file's text; lockThere whether this machine has that light.
func lockOn(s, key string) bool { return strings.Contains(s, `"`+key+`": true`) }
func lockThere(s, key string) bool {
	return strings.Contains(s, `"`+key+`": `) && !strings.Contains(s, `"`+key+`": null`)
}

// gotLamps takes the new state; when a lock flipped, it asks whether sy fills the screen (plateWanted).
func (m *Model) gotLamps(s string) tea.Cmd {
	old := m.lamps
	m.lamps = s
	cmd := waitLamps()
	if old == "" || s == "" || s == old {
		return cmd
	}
	for _, k := range plateKeys {
		if lockOn(old, k.key) != lockOn(s, k.key) {
			return tea.Batch(cmd, askPlate(k.key, m.focused()))
		}
	}
	return cmd
}

func (m *Model) focused() bool { return !m.blurred }

func (m *Model) lampSign() string {
	if !showLamps || m.lamps == "" {
		return ""
	}
	var parts []string
	for _, l := range []struct{ key, nerd, plain string }{
		{"caps", "\U000f0632", "A"}, {"num", "\U000f03a0", "1"}, {"scroll", "\U000f04e2", "↕"}, {"fn", "\U000f0295", "ƒ"},
	} {
		fg := "#6a2814" // ash: off
		if lockOn(m.lamps, l.key) {
			fg = "#ff8a1a" // gold: on
		}
		parts = append(parts, lipgloss.NewStyle().Background(hex(cStatusBG)).Foreground(hex(fg)).Render(icon(l.nerd, l.plain)))
	}
	return strings.Join(parts, stStatus.Render(" ")) + stStatus.Render("  ")
}

// ---- does sy fill the screen? ----------------------------------------------------------

type plateMsg string // the lock that flipped, when the plate should show

func askPlate(key string, focused bool) tea.Cmd {
	return func() tea.Msg {
		if focused && syFullscreen() && !doNotDisturb() {
			return plateMsg(key)
		}
		return nil
	}
}

// syFullscreen asks Hyprland whether the window being typed in is fullscreen and is this sy:
// its title ends in "Symbolizer" and its program (the terminal) is one sy runs inside.
func syFullscreen() bool {
	sig := os.Getenv("HYPRLAND_INSTANCE_SIGNATURE")
	if sig == "" {
		return false
	}
	c, err := net.DialTimeout("unix", filepath.Join(runtimeDir(), "hypr", sig, ".socket.sock"), 300*time.Millisecond)
	if err != nil {
		return false
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(500 * time.Millisecond))
	if _, err := c.Write([]byte("j/activewindow")); err != nil {
		return false
	}
	data, err := io.ReadAll(c)
	if err != nil {
		return false
	}
	var w struct {
		Pid        int    `json:"pid"`
		Title      string `json:"title"`
		Fullscreen int    `json:"fullscreen"`
	}
	if json.Unmarshal(data, &w) != nil {
		return false
	}
	// fullscreen: 1 = maximised, 2 = fullscreen, 3 = both
	return w.Fullscreen&2 != 0 && strings.HasSuffix(w.Title, "Symbolizer") && runsInside(w.Pid)
}

// runsInside says whether process pid is one of sy's parents (or their parents...).
func runsInside(pid int) bool {
	for p, steps := os.Getppid(), 0; p > 1 && steps < 50; steps++ {
		if p == pid {
			return true
		}
		stat, err := os.ReadFile("/proc/" + strconv.Itoa(p) + "/stat")
		if err != nil {
			return false
		}
		// "pid (name) state ppid ...": the name can hold spaces, so look after its last ")"
		f := strings.Fields(string(stat[strings.LastIndexByte(string(stat), ')')+1:]))
		if len(f) < 2 {
			return false
		}
		p, _ = strconv.Atoi(f[1])
	}
	return false
}

// doNotDisturb follows mako's do-not-disturb mode, as the desktop's plate does.
func doNotDisturb() bool {
	if _, err := exec.LookPath("makoctl"); err != nil {
		return false
	}
	cmd := exec.Command("makoctl", "mode")
	done := make(chan []byte, 1)
	go func() { out, _ := cmd.Output(); done <- out }()
	select {
	case out := <-done:
		return strings.Contains(string(out), "do-not-disturb")
	case <-time.After(time.Second):
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		return false
	}
}
