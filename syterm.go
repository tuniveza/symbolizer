package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// sy term: ŧ (Right Alt+T on a UK keyboard) shows or hides a real kitty
// terminal in a pane under the editor. kitty does the work: the kitten
// ~/.config/kitty/sy_term.py is mapped to ŧ in panes.conf for Symbolizer
// and for the sy term pane, so normally the key never reaches Symbolizer.
// When it does (kitty's map not loaded yet), ask kitty to run the kitten.

type syTermMsg string // why sy term could not open

func (m *Model) syTerm() tea.Cmd {
	if os.Getenv("KITTY_WINDOW_ID") == "" {
		m.sayErr("sy term needs kitty: it is a kitty pane under the editor.")
		return nil
	}
	return func() tea.Msg {
		out, err := exec.Command("kitten", "@", "action", "kitten", "sy_term.py").CombinedOutput()
		if err != nil {
			why := strings.TrimSpace(string(out))
			if why == "" {
				why = err.Error()
			}
			return syTermMsg("sy term did not open: " + why)
		}
		return nil
	}
}

// tellKittyMode tells kitty which mode Symbolizer is in, as the window's
// user variable sy_mode ("text", "design", …), when it changed. kitty's
// keys in panes.conf use it: Alt+R resizes the split, except in Text mode,
// where it is Emmet's remove tag.
func (m *Model) tellKittyMode() tea.Cmd {
	if os.Getenv("KITTY_WINDOW_ID") == "" {
		return nil
	}
	name := strings.ToLower(modes[m.mode].name)
	if name == m.kittyMode {
		return nil
	}
	m.kittyMode = name
	return tea.Raw("\x1b]1337;SetUserVar=sy_mode=" + base64.StdEncoding.EncodeToString([]byte(name)) + "\x07")
}

// closeIdleSyTerms: Symbolizer is closing. Its sy term panes in this tab
// close with it when all they hold is an idle shell, so the whole kitty
// window goes and the next Left Alt + AltGr opens a new Symbolizer (the
// user asked, 16 Nov 2026). A sy term running something (a program, a
// Claude session …) is left open: closing it would end that.
func closeIdleSyTerms() {
	me := os.Getenv("KITTY_WINDOW_ID")
	if me == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "kitten", "@", "ls", "--match-tab", "window_id:"+me).Output()
	if err != nil {
		return
	}
	for _, id := range idleSyTerms(out, me) {
		_ = exec.CommandContext(ctx, "kitten", "@", "close-window", "--match", "id:"+id).Run()
	}
}

// idleSyTerms are the sy term panes in kitty's ls answer (not window me)
// whose only running program is a shell.
func idleSyTerms(ls []byte, me string) []string {
	var oss []struct {
		Tabs []struct {
			Windows []struct {
				ID   int               `json:"id"`
				Vars map[string]string `json:"user_vars"`
				FG   []struct {
					Cmdline []string `json:"cmdline"`
				} `json:"foreground_processes"`
			} `json:"windows"`
		} `json:"tabs"`
	}
	if json.Unmarshal(ls, &oss) != nil {
		return nil
	}
	shells := map[string]bool{"zsh": true, "bash": true, "fish": true, "sh": true, "dash": true}
	var ids []string
	for _, o := range oss {
		for _, t := range o.Tabs {
			for _, w := range t.Windows {
				if w.Vars["sy_term"] != "1" || itoa(w.ID) == me || len(w.FG) == 0 {
					continue
				}
				idle := true
				for _, p := range w.FG {
					if len(p.Cmdline) == 0 || !shells[strings.TrimPrefix(filepath.Base(p.Cmdline[0]), "-")] {
						idle = false
					}
				}
				if idle {
					ids = append(ids, itoa(w.ID))
				}
			}
		}
	}
	return ids
}
