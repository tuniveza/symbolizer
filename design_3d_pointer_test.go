package main

import (
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// fakeHyprPtr stands in for hyprctl: it notes the calls, and its
// cursor:invisible starts as theirs.
func fakeHyprPtr(t *testing.T, theirs bool) (calls func() []string) {
	t.Helper()
	var mu sync.Mutex
	var got []string
	oldCtl, oldOn, oldWatch, oldPtr := hyprctl, hyprOn, hideWatch, hyprPtr
	t.Cleanup(func() {
		hyprPtr.show()
		hyprctl, hyprOn, hideWatch, hyprPtr = oldCtl, oldOn, oldWatch, oldPtr
	})
	hyprPtr, hyprOn = &hyprPointer{}, true
	hideWatch = func() {
		mu.Lock()
		got = append(got, "watch")
		mu.Unlock()
	}
	hyprctl = func(args ...string) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, strings.Join(args, " "))
		if args[0] == "getoption" {
			return map[bool]string{true: "bool: true\nset: true\n", false: "bool: false\nset: false\n"}[theirs], nil
		}
		if args[0] == "eval" || args[0] == "dispatch" {
			return "ok", nil
		}
		if args[0] == "j/activewindow" {
			return `{"title": "board — Symbolizer", "at": [100, 50], "size": [1200, 800]}`, nil
		}
		return "unknown request", nil
	}
	return func() []string {
		hyprPtr.done.Wait()
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), got...)
	}
}

// The pointer is hidden while the room is in 3D and shown again as it
// goes flat, as sy loses the keyboard, and as sy ends; a watcher is
// started before it is first hidden.
func TestDesign3DPointerHidden(t *testing.T) {
	calls := fakeHyprPtr(t, false)
	m := room3DModel(t, 120, 40)
	d := m.ds()
	m.Update(tea.MouseMotionMsg(tea.Mouse{X: 5, Y: 5}))
	if got := calls(); len(got) != 0 {
		t.Fatalf("on the flat board the pointer is left alone: %v", got)
	}
	press(m, "ctrl+f")
	if !d.room.on() {
		t.Fatal("Ctrl+F: 3D")
	}
	want := []string{"getoption cursor:invisible", "watch", "eval hl.config({ cursor = { invisible = true } })"}
	if got := calls(); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("into 3D: %v, want %v", got, want)
	}
	m.Update(tea.BlurMsg{})
	m.Update(tea.FocusMsg{})
	press(m, "esc")
	want = append(want, "eval hl.config({ cursor = { invisible = false } })", "eval hl.config({ cursor = { invisible = true } })", "eval hl.config({ cursor = { invisible = false } })")
	if got := calls(); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("another window, back, then the flat board: %v, want %v", got, want)
	}
	press(m, "ctrl+f")
	hyprPtr.show() // sy ending
	want = append(want, "eval hl.config({ cursor = { invisible = true } })", "eval hl.config({ cursor = { invisible = false } })")
	if got := calls(); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("3D again, then sy ends: %v, want %v", got, want)
	}
}

// A pointer the person hid in Hyprland's own settings is left alone.
func TestDesign3DPointerTheirs(t *testing.T) {
	calls := fakeHyprPtr(t, true)
	m := room3DModel(t, 120, 40)
	press(m, "ctrl+f")
	press(m, "esc")
	hyprPtr.show()
	if got := calls(); len(got) != 1 || got[0] != "getoption cursor:invisible" {
		t.Fatalf("their own hidden pointer: only asked about, got %v", got)
	}
}

// In 3D a click acts where the cross is, wherever the mouse is.
func TestDesign3DClickAtCross(t *testing.T) {
	m := room3DModel(t, 120, 40)
	d := m.ds()
	d.room.yaw, d.room.pitch = 20, 15
	o := d.octs[1] // a page
	cell3D(t, m, o.ID, func(sx, sy int) bool { return true })
	d.sel, d.lastClick = -1, time.Time{}
	click(m, 2, m.boardTop()+1, tea.MouseLeft) // the mouse at the top left corner
	if d.sel != o.ID {
		t.Fatalf("a click with the cross on the page chose %d, want %d", d.sel, o.ID)
	}
	if h := d.room.pickAt(2, 1); h.own == o.ID {
		t.Fatal("(the page should not be under the mouse too)")
	}
}

// Far from the window's middle, the hidden pointer is put back there; the
// report of that jump does not turn the camera back.
func TestDesign3DPointerRecenter(t *testing.T) {
	calls := fakeHyprPtr(t, false)
	m := room3DModel(t, 120, 40)
	d := m.ds()
	r := &d.room
	press(m, "ctrl+f")
	calls()
	move := func(x, y int) { m.Update(tea.MouseMotionMsg(tea.Mouse{X: x, Y: y})) }
	move(60, 20)
	move(70, 20) // still near the middle
	if got := calls(); len(got) != 3 {
		t.Fatalf("near the middle the pointer stays: %v", got[3:])
	}
	look := r.look
	for x := 75; x <= 90; x += 5 { // to over a quarter of the width from the middle
		move(x, 20)
	}
	got := calls()
	want := []string{"j/activewindow", "dispatch hl.dsp.cursor.move({ x = 700, y = 450 })"}
	if strings.Join(got[3:], "|") != strings.Join(want, "|") {
		t.Fatalf("far from the middle: %v, want %v", got[3:], want)
	}
	if r.look == look {
		t.Fatal("the moves themselves turn the camera")
	}
	look = r.look
	move(60, 20) // the jump back
	if r.look != look {
		t.Fatalf("the pointer put back turned the camera: %v, was %v", r.look, look)
	}
	move(62, 20)
	if r.look == look {
		t.Fatal("after it, the mouse looks round again")
	}
	if got := calls(); len(got) != 5 {
		t.Fatalf("near the middle again: no more put back: %v", got[5:])
	}
}
