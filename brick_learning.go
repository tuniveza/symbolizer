package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"
)

// brick learning in every mode: everything you write in Symbolizer is input
// for brick's imagination neurons. Each file you save is put on a reading
// list (~/.local/share/symbolizer/brick/reading.list) that the training
// daemon (brick/trainer/daemon.py) reads; your diary only while its switch is
// on. It all stays on this PC. F12 turns the daemon on or off in any mode;
// the status bar shows a brick sign while it learns.

func brickDataDir() string { return filepath.Join(filepath.Dir(sessionDir()), "brick") }

var (
	readingMu   sync.Mutex
	readingSeen = map[string]bool{}
)

// noteWritten adds a saved file to brick's reading list (once per file).
func noteWritten(path string) {
	if path == "" {
		return
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return
	}
	readingMu.Lock()
	defer readingMu.Unlock()
	if readingSeen[abs] {
		return
	}
	readingSeen[abs] = true
	dir := brickDataDir()
	if os.MkdirAll(dir, 0o700) != nil {
		return
	}
	list := filepath.Join(dir, "reading.list")
	if old, err := os.ReadFile(list); err == nil {
		for _, l := range strings.Split(string(old), "\n") {
			if l == abs {
				return
			}
		}
	}
	f, err := os.OpenFile(list, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	f.WriteString(abs + "\n")
}

type brickSettings struct {
	Diary string `json:"diary"`
}

func readBrickSettings() brickSettings {
	var s brickSettings
	if b, err := os.ReadFile(filepath.Join(brickDataDir(), "settings.json")); err == nil {
		json.Unmarshal(b, &s)
	}
	return s
}

func brickDiaryOn() bool { return readBrickSettings().Diary == "on" }

func setBrickDiary(on bool) {
	s := readBrickSettings()
	s.Diary = "off"
	if on {
		s.Diary = "on"
	}
	b, _ := json.Marshal(s)
	os.MkdirAll(brickDataDir(), 0o700)
	os.WriteFile(filepath.Join(brickDataDir(), "settings.json"), b, 0o600)
}

// brickLampMsg: whether the training daemon is running (checked at start).
type brickLampMsg bool

func checkBrickLamp() tea.Cmd {
	return func() tea.Msg {
		return brickLampMsg(exec.Command("systemctl", "--user", "is-active", "--quiet", "brick-neurons.service").Run() == nil)
	}
}

// styleColour is a syntax style's colour in the current theme.
func (m *Model) styleColour(s uint8) string {
	if int(s) < len(syntaxStyles) {
		return syntaxStyles[s].fg
	}
	return cText
}
