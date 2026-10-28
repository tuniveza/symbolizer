package main

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

const someLocks = `{"caps": false, "num": true, "scroll": false, "fn": true}`

func plateModel(t *testing.T, w, h int) *Model {
	m, _ := NewModel("", 0)
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	m.lamps = someLocks
	return m
}

func screenText(m *Model) string { return ansi.Strip(m.View().Content) }

func TestPlateShowsAllFourKeys(t *testing.T) {
	m := plateModel(t, 120, 40)
	m.lamps = `{"caps": true, "num": true, "scroll": false, "fn": true}`
	if m.showPlate("caps") == nil || m.plate == nil {
		t.Fatal("no plate")
	}
	s := screenText(m)
	for _, w := range []string{"CAPS", "NUM", "SCROLL", "FN", "⇕"} {
		if !strings.Contains(s, w) {
			t.Errorf("plate has no %q", w)
		}
	}
	if m.plate.small || len(m.plate.sparks) == 0 {
		t.Errorf("want the full plate with sparks, got small=%v sparks=%d", m.plate.small, len(m.plate.sparks))
	}
	// it sits just above the info line, at the right
	if m.plate.y+plateH != m.height-3 || m.plate.x+plateW != m.width-2 {
		t.Errorf("plate at %d,%d", m.plate.x, m.plate.y)
	}
}

func TestPlateGoesAwayByItself(t *testing.T) {
	m := plateModel(t, 120, 40)
	m.showPlate("num")
	gen := m.plate.gen
	m.plate.changed = time.Now().Add(-plateIn - plateHold - plateOut - time.Millisecond)
	if m.plateStep(gen) != nil || m.plate != nil {
		t.Fatal("plate should be gone after its fade-out")
	}
	if strings.Contains(screenText(m), "SCROLL") {
		t.Fatal("plate still drawn")
	}
}

func TestPlateStaleFramesAreDropped(t *testing.T) {
	m := plateModel(t, 120, 40)
	m.showPlate("num")
	old := m.plate.gen
	m.showPlate("caps") // a second press while it shows: same plate, new frames
	if m.plateStep(old) != nil {
		t.Error("an old frame chain kept going")
	}
	if m.plateStep(m.plate.gen) == nil {
		t.Error("the new frame chain stopped")
	}
}

func TestPlateKeepsAwayFromTheCursor(t *testing.T) {
	m := plateModel(t, 120, 40)
	m.buf.Insert(strings.Repeat(strings.Repeat("x", 115)+"\n", 60), "")
	m.buf.Cursor = Pos{m.top + m.textHeight() - 2, 110}
	m.ensureVisible()
	c := m.cursor()
	if c == nil {
		t.Fatal("no cursor")
	}
	m.showPlate("caps")
	p := m.plate
	if c.X >= p.x && c.X < p.x+plateW && c.Y >= p.y && c.Y < p.y+plateH {
		t.Errorf("plate at %d,%d covers the cursor at %d,%d", p.x, p.y, c.X, c.Y)
	}
}

func TestSmallWindowGetsOneLinePlate(t *testing.T) {
	m := plateModel(t, 50, 12)
	m.showPlate("scroll")
	if m.plate == nil || !m.plate.small {
		t.Fatal("want the one-line plate")
	}
	s := screenText(m)
	if !strings.Contains(s, "● CAPS") || !strings.Contains(s, "● FN") {
		t.Errorf("one-line plate not drawn:\n%s", s)
	}
	tiny := plateModel(t, 30, 7)
	if tiny.showPlate("caps") != nil || tiny.plate != nil {
		t.Error("a window too small for any plate should get none")
	}
}

func TestOnlyAFlipAsksForThePlate(t *testing.T) {
	m := plateModel(t, 120, 40)
	m.lamps = ""
	m.gotLamps(someLocks) // the first read: nothing pressed
	if m.lamps != someLocks || m.plate != nil {
		t.Fatal("first read should only store the state")
	}
	m.gotLamps(strings.Replace(someLocks, `"caps": false`, `"caps": true`, 1))
	if !lockOn(m.lamps, "caps") {
		t.Fatal("state not stored")
	}
}

// a pretend Hyprland that answers activewindow
func fakeHypr(t *testing.T, answer string) {
	run := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", run)
	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "test")
	dir := filepath.Join(run, "hypr", "test")
	os.MkdirAll(dir, 0o700)
	l, err := net.Listen("unix", filepath.Join(dir, ".socket.sock"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			buf := make([]byte, 64)
			n, _ := c.Read(buf)
			if string(buf[:n]) == "j/activewindow" {
				c.Write([]byte(answer))
			}
			c.Close()
		}
	}()
}

func TestFullscreenSymbolizerIsSeen(t *testing.T) {
	pid := os.Getppid()
	for _, c := range []struct {
		answer string
		want   bool
	}{
		{`{"pid": PID, "title": "notes.txt — Symbolizer", "fullscreen": 2}`, true},
		{`{"pid": PID, "title": "notes.txt — Symbolizer", "fullscreen": 3}`, true},
		{`{"pid": PID, "title": "notes.txt — Symbolizer", "fullscreen": 1}`, false}, // only maximised
		{`{"pid": PID, "title": "notes.txt — Symbolizer", "fullscreen": 0}`, false},
		{`{"pid": PID, "title": "zsh", "fullscreen": 2}`, false},               // another tab of the terminal
		{`{"pid": 999999, "title": "a — Symbolizer", "fullscreen": 2}`, false}, // another terminal's sy
	} {
		fakeHypr(t, strings.Replace(c.answer, "PID", strconv.Itoa(pid), 1))
		if got := syFullscreen(); got != c.want {
			t.Errorf("%s: got %v", c.answer, got)
		}
	}
	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "")
	if syFullscreen() {
		t.Error("no Hyprland: should be false")
	}
}

func TestBlurredWindowGetsNoPlate(t *testing.T) {
	fakeHypr(t, `{"pid": `+strconv.Itoa(os.Getppid())+`, "title": "x — Symbolizer", "fullscreen": 2}`)
	if askPlate("caps", false)() != nil {
		t.Error("a window without the keyboard asked for the plate")
	}
	if msg, ok := askPlate("caps", true)().(plateMsg); !ok || msg != "caps" {
		t.Errorf("focused fullscreen sy: got %v", msg)
	}
}
