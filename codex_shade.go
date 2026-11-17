package main

// ---- no desktop shading over the codex --------------------------------------------------
//
// The Emberline desktop runs a screen shader over terminals (the warm glow
// and carved text, emberline-shader-scope). A codex's pages, pictures and
// colours should show exactly as drawn, so while Codex mode is up sy
// writes where its view is (the rows between the top bar and the strip) to
// $XDG_RUNTIME_DIR/sy-clear/<pid>.json; emberline-shader-scope cuts that
// area out of its shading. The bars above and below keep it (the user
// asked, 16 Nov 2026). Leaving Codex mode, or sy, takes the note away.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

func codexShadePath() string {
	run := os.Getenv("XDG_RUNTIME_DIR")
	if run == "" {
		return ""
	}
	return filepath.Join(run, "sy-clear", fmt.Sprint(os.Getpid())+".json")
}

// codexShadeSync writes (or takes away) the note when what it says changed.
func (m *Model) codexShadeSync() {
	c := m.cx()
	key := ""
	if m.inCodex() && !m.help && !m.modesOpen && os.Getenv("KITTY_PID") != "" {
		key = fmt.Sprint(m.width, "x", m.height, "/", m.boardTop(), "+", m.boardH())
	}
	if key == c.shadeKey {
		return
	}
	c.shadeKey = key
	path := codexShadePath()
	if path == "" {
		return
	}
	if key == "" {
		_ = os.Remove(path)
		return
	}
	ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if err != nil || ws.Ypixel == 0 || ws.Row == 0 {
		_ = os.Remove(path)
		return
	}
	if c.shadeTop == 0 {
		c.shadeTop = 1 + map[bool]int{true: 1, false: 0}[syPaneOnTop()]
	}
	note := map[string]any{
		"sy_pid":    os.Getpid(),
		"kitty_pid": os.Getenv("KITTY_PID"),
		"rows":      int(ws.Row),
		"cell_h":    float64(ws.Ypixel) / float64(ws.Row), // screen pixels a row
		"top":       m.boardTop(),                         // the first row of the view
		"bottom":    m.boardTop() + m.boardH(),            // the row after its last
		"on_top":    c.shadeTop == 2,                      // sy's pane is the top of its kitty window
	}
	b, _ := json.Marshal(note)
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path+".part", b, 0o644)
	_ = os.Rename(path+".part", path)
}

// codexShadeDrop takes the note away (sy is closing).
func codexShadeDrop() {
	if p := codexShadePath(); p != "" {
		_ = os.Remove(p)
	}
}

// syPaneOnTop: is sy's pane the first in its kitty tab (so its rows start
// at the window's top)? Its sy term opens under it.
func syPaneOnTop() bool {
	me := os.Getenv("KITTY_WINDOW_ID")
	if me == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "kitten", "@", "ls", "--match-tab", "window_id:"+me).Output()
	if err != nil {
		return true // a window of its own, most likely
	}
	var oss []struct {
		Tabs []struct {
			Windows []struct {
				ID int `json:"id"`
			} `json:"windows"`
		} `json:"tabs"`
	}
	if json.Unmarshal(out, &oss) != nil {
		return true
	}
	for _, o := range oss {
		for _, t := range o.Tabs {
			if len(t.Windows) > 0 {
				return itoa(t.Windows[0].ID) == me
			}
		}
	}
	return true
}
