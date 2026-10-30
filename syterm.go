package main

import (
	"os"
	"os/exec"
	"strings"

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
