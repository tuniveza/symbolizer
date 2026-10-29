package main

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// sy --keys: shows each key exactly as Symbolizer receives it and what it
// does with it. For finding out why a shortcut does not work.

var keyMeaning = map[string]string{
	"alt+left": "move the highlighted text (or the word) left", "alt+right": "move the highlighted text (or the word) right",
	"alt+up": "move the highlighted text (or the line) up", "alt+down": "move the highlighted text (or the line) down",
	"alt+q": "quit to the terminal",
	"alt+l": "load list (sessions, this file from disk)", "alt+o": "open a file (file manager)",
	"ß": "save (Right Alt+S on a UK keyboard)", "ø": "open a file (Right Alt+O on a UK keyboard)", "alt+b": "start of line", "alt+e": "end of file (in the file manager: exit it)", "alt+s": "start of file",
	"ctrl+home": "start of file", "ctrl+end": "end of file", "ctrl+s": "save (a session is asked for a name)",
	"shift+left": "highlight one step left", "shift+right": "highlight one step right",
	"shift+up": "highlight a line up", "shift+down": "highlight a line down",
	"ctrl+left": "cursor left one character (with Up/Down held: diagonal)", "ctrl+right": "cursor right one character (with Up/Down held: diagonal)",
	"ctrl+shift+left": "highlight one character left", "ctrl+shift+right": "highlight one character right",
	"ctrl+shift+up": "highlight a line up", "ctrl+shift+down": "highlight a line down",
	"ctrl+up": "cursor up a line (with Left/Right held: diagonal)", "ctrl+down": "cursor down a line (with Left/Right held: diagonal)",
	"ctrl+c": "copy", "ctrl+v": "paste", "ctrl+p": "paste", "ctrl+f": "find",
	"ctrl+l": "load list (same as Alt+L)",
	"ctrl+o": "file manager (same as Alt+O)", "ctrl+b": "show / hide the file manager", "ctrl+w": "close this file",
	"ctrl+pgup": "previous open file", "ctrl+pgdown": "next open file",
	"alt+shift+left": "move text left", "alt+shift+right": "move text right",
	"ctrl+z": "undo", "ctrl+y": "redo", "ctrl+x": "cut", "ctrl+a": "highlight all",
	"ctrl+d": "duplicate", "ctrl+k": "delete line", "ctrl+/": "comment", "ctrl+g": "go to line",
	"alt+m": "switch modes", "alt+j": "centre lines (or back to the left)", "ŧ": "colour themes (Right Alt+T on a UK keyboard)", "”": "diary: new page (Right Alt+N on a UK keyboard)", "æ": "diary: all pages (Right Alt+A on a UK keyboard)", "alt+t": "colour themes", "alt+n": "new file in a new tab", "alt+c": "close this file",
	"alt+tab": "a real Tab (indent)", "alt+shift+tab": "un-indent", "shift+tab": "un-indent",
	"ctrl+tab": "then Ctrl+Left/Right: set the Tab direction",
	"ctrl+r":   "replace", "ctrl+n": "new session in a new tab", "alt+a": "save as", "f1": "help", "f3": "next match",
	"up": "cursor up", "down": "cursor down", "left": "cursor left one step (word, number, bracket or operator)", "right": "cursor right one step (word, number, bracket or operator)",
	"enter": "new line", "tab": "next file tab (then Left/Right: that way)", "backspace": "delete left", "delete": "delete right",
	"home": "start of line", "end": "end of line", "esc": "clear highlight",
}

type keyCheck struct {
	log      []string
	releases string
	width    int
	height   int
	quitArm  bool
}

func (k *keyCheck) Init() tea.Cmd { return nil }

func (k *keyCheck) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		k.width, k.height = msg.Width, msg.Height
	case tea.KeyboardEnhancementsMsg:
		if msg.SupportsEventTypes() {
			k.releases = "yes (two arrows held together are seen exactly)"
		} else {
			k.releases = "no (two arrows count as held together if pressed within 0.15 s)"
		}
	case tea.PasteMsg:
		k.add(fmt.Sprintf("paste  %q", msg.Content), "pastes this text")
	case tea.KeyReleaseMsg:
		k.add("let go: "+keyName(msg.Key()), "")
	case tea.KeyPressMsg:
		name := keyName(msg.Key())
		if name == "esc" {
			if k.quitArm {
				return k, tea.Quit
			}
			k.quitArm = true
			k.add(name, "press Esc again (or Alt+Q) to close this check")
			return k, nil
		}
		k.quitArm = false
		if name == "alt+q" {
			return k, tea.Quit
		}
		what := keyMeaning[name]
		if what == "" {
			if _, ok := bindings[name]; ok {
				what = "a command"
			} else if t := msg.Key().Text; t != "" {
				what = "types " + fmt.Sprintf("%q", t)
			} else {
				what = "NOTHING (Symbolizer has no command for this key)"
			}
		}
		if msg.Key().IsRepeat {
			name += "  (held, repeating)"
		}
		k.add(name, what)
	}
	return k, nil
}

func (k *keyCheck) add(name, what string) {
	line := stHelpKey.Render(fmt.Sprintf("%-28s", name))
	if what != "" {
		line += stHelpText.Render("→ " + what)
	}
	k.log = append(k.log, line)
	if len(k.log) > 200 {
		k.log = k.log[1:]
	}
}

func (k *keyCheck) View() tea.View {
	head := []string{
		glowText("◆ Symbolizer key check", cBG),
		stHelpText.Render("Press any key. Each line shows the key Symbolizer received, then what it does."),
		stHelpText.Render("Terminal reports keys being let go: " + k.releases),
		stHelpNew.Render("Esc twice closes this."),
		"",
	}
	room := max(k.height-len(head), 1)
	log := k.log
	if len(log) > room {
		log = log[len(log)-room:]
	}
	v := tea.NewView(strings.Join(append(head, log...), "\n"))
	v.AltScreen = true
	v.KeyboardEnhancements.ReportEventTypes = true
	v.KeyboardEnhancements.ReportAllKeysAsEscapeCodes = true
	v.KeyboardEnhancements.ReportAssociatedText = true
	v.BackgroundColor = hex(cBG)
	v.ForegroundColor = hex(cText)
	v.WindowTitle = "Symbolizer key check"
	return v
}
